// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// =============================================================================
// Orphan detection
// =============================================================================

// orphanKind classifies why a snapshot is considered debris.
type orphanKind string

const (
	// orphanOutOfScope is a zfs-backup snapshot on a dataset that zfs-backup
	// does not replicate, so nothing will ever prune it.
	orphanOutOfScope orphanKind = "out-of-scope"
	// orphanSyncoid is a syncoid sync-snapshot left behind by a failed send.
	orphanSyncoid orphanKind = "syncoid leftover"
)

// orphanSnapshot is a snapshot that no part of zfs-backup will ever clean up.
type orphanSnapshot struct {
	snapshotEntry
	Kind   orphanKind
	Reason string
}

// minSyncoidOrphanAge is how old a syncoid sync-snapshot must be before it is
// treated as debris. A younger one may belong to a send that is still running.
const minSyncoidOrphanAge = 24 * time.Hour

// scanOrphans finds snapshots under a pool that no phase of zfs-backup will
// ever clean up: its own `-Backup` snapshots sitting on datasets outside the
// configured scope (including the pool root and nested descendants left by the
// pre-2.0 recursive snapshot), and stale syncoid sync-snapshots.
//
// Snapshots the user or another tool created are never reported - only
// zfs-backup's own naming patterns are matched, and protected snapshots such
// as POOL/root@blank are excluded outright.
func scanOrphans(entries []snapshotEntry, pool string, inScope []string, now time.Time, minSyncoidAge time.Duration) []orphanSnapshot {
	scoped := make(map[string]bool, len(inScope))
	for _, ds := range inScope {
		scoped[fmt.Sprintf("%s/%s", pool, ds)] = true
	}

	var orphans []orphanSnapshot
	for _, entry := range entries {
		if isProtectedSnapshotTag(entry.Tag) {
			continue
		}

		switch {
		case isBackupSnapshotTag(entry.Tag):
			if scoped[entry.Dataset] {
				continue // managed: the prune phase covers this dataset
			}
			orphans = append(orphans, orphanSnapshot{
				snapshotEntry: entry,
				Kind:          orphanOutOfScope,
				Reason:        fmt.Sprintf("%s is not in the backup scope, so nothing prunes it", entry.Dataset),
			})
		case isSyncoidSnapshotTag(entry.Tag):
			if !entry.Creation.IsZero() && now.Sub(entry.Creation) < minSyncoidAge {
				continue // a send may still be in flight
			}
			orphans = append(orphans, orphanSnapshot{
				snapshotEntry: entry,
				Kind:          orphanSyncoid,
				Reason:        "syncoid sync-snapshot left behind by a failed send",
			})
		}
	}

	return orphans
}

// groupOrphansByDataset groups orphans by dataset, preserving a stable order.
func groupOrphansByDataset(orphans []orphanSnapshot) ([]string, map[string][]orphanSnapshot) {
	grouped := map[string][]orphanSnapshot{}
	for _, o := range orphans {
		grouped[o.Dataset] = append(grouped[o.Dataset], o)
	}

	datasets := make([]string, 0, len(grouped))
	for ds := range grouped {
		datasets = append(datasets, ds)
	}
	sort.Strings(datasets)

	for _, ds := range datasets {
		sort.SliceStable(grouped[ds], func(i, j int) bool {
			return grouped[ds][i].Name < grouped[ds][j].Name
		})
	}

	return datasets, grouped
}

// =============================================================================
// Quota pressure
// =============================================================================

// datasetUsage is the space accounting for one dataset.
type datasetUsage struct {
	Name            string
	Used            int64
	UsedBySnapshots int64
	Quota           int64
	RefQuota        int64
}

// quotaPressureThreshold is the fraction of a dataset's quota that may be
// consumed by snapshots before it is flagged.
const quotaPressureThreshold = 0.5

// parseDatasetUsage parses the output of
// `zfs list -H -p -o name,used,usedbysnapshots,quota,refquota`.
func parseDatasetUsage(output string) []datasetUsage {
	var usages []datasetUsage
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) < 5 || fields[0] == "" {
			continue
		}
		parse := func(s string) int64 {
			v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if err != nil {
				return 0 // ZFS renders "none"/"-" for unset properties
			}
			return v
		}
		usages = append(usages, datasetUsage{
			Name:            strings.TrimSpace(fields[0]),
			Used:            parse(fields[1]),
			UsedBySnapshots: parse(fields[2]),
			Quota:           parse(fields[3]),
			RefQuota:        parse(fields[4]),
		})
	}
	return usages
}

// flagQuotaPressure returns the datasets whose snapshots consume more than the
// given fraction of their quota. Datasets without a quota are flagged when
// snapshots dominate their usage, since those silently eat pool free space.
func flagQuotaPressure(usages []datasetUsage, threshold float64) []datasetUsage {
	var flagged []datasetUsage
	for _, u := range usages {
		if u.UsedBySnapshots == 0 {
			continue
		}
		switch {
		case u.Quota > 0:
			if float64(u.UsedBySnapshots)/float64(u.Quota) > threshold {
				flagged = append(flagged, u)
			}
		case u.Used > 0:
			if float64(u.UsedBySnapshots)/float64(u.Used) > 0.75 {
				flagged = append(flagged, u)
			}
		}
	}
	return flagged
}

// listDatasetUsage reads space accounting for a pool and its descendants.
func listDatasetUsage(ctx context.Context, r commandRunner, pool string) ([]datasetUsage, error) {
	output, err := r.Output(ctx, "zfs", "list", "-H", "-p", "-r",
		"-o", "name,used,usedbysnapshots,quota,refquota", pool)
	if err != nil {
		return nil, err
	}
	return parseDatasetUsage(output), nil
}

// =============================================================================
// Destroy-safety checks
// =============================================================================

// snapshotHasHolds reports whether a snapshot carries a user hold. A held
// snapshot cannot be destroyed and signals that something else depends on it.
func snapshotHasHolds(ctx context.Context, r commandRunner, snapshot string) bool {
	output, err := r.Output(ctx, "zfs", "holds", "-H", snapshot)
	if err != nil {
		return true // cannot prove it is safe, so treat it as held
	}
	return strings.TrimSpace(output) != ""
}

// snapshotHasClones reports whether a snapshot has dependent clones. A cloned
// snapshot must never be destroyed.
func snapshotHasClones(ctx context.Context, r commandRunner, snapshot string) bool {
	output, err := r.Output(ctx, "zfs", "get", "-H", "-o", "value", "clones", snapshot)
	if err != nil {
		return true // cannot prove it is safe
	}
	value := strings.TrimSpace(output)
	return value != "" && value != "-"
}

// destroyDecision records whether one orphan may be destroyed.
type destroyDecision struct {
	Orphan     orphanSnapshot
	Safe       bool
	SkipReason string
}

// vetOrphans applies the mandatory pre-flight checks from the cleanup
// procedure: never touch protected snapshots, never touch held snapshots and
// never touch snapshots with dependent clones.
func vetOrphans(ctx context.Context, r commandRunner, orphans []orphanSnapshot) []destroyDecision {
	decisions := make([]destroyDecision, 0, len(orphans))
	for _, o := range orphans {
		switch {
		case isProtectedSnapshotTag(o.Tag):
			decisions = append(decisions, destroyDecision{Orphan: o, SkipReason: "protected snapshot"})
		case snapshotHasHolds(ctx, r, o.Name):
			decisions = append(decisions, destroyDecision{Orphan: o, SkipReason: "snapshot has a hold"})
		case snapshotHasClones(ctx, r, o.Name):
			decisions = append(decisions, destroyDecision{Orphan: o, SkipReason: "snapshot has dependent clones"})
		default:
			decisions = append(decisions, destroyDecision{Orphan: o, Safe: true})
		}
	}
	return decisions
}

// safeToDestroy returns the snapshot names cleared by vetOrphans.
func safeToDestroy(decisions []destroyDecision) []string {
	var names []string
	for _, d := range decisions {
		if d.Safe {
			names = append(names, d.Orphan.Name)
		}
	}
	return names
}

// =============================================================================
// Shared collection step
// =============================================================================

// orphanScan is everything doctor and cleanup-orphans need about one pool.
type orphanScan struct {
	Pool     string
	InScope  []string
	Missing  []string
	Orphans  []orphanSnapshot
	Usage    []datasetUsage
	ScanTime time.Time
}

// collectOrphanScan performs the read-only inspection shared by the doctor and
// cleanup-orphans subcommands.
func collectOrphanScan(ctx context.Context, r commandRunner, pool string) (*orphanScan, error) {
	inScope, missing, err := resolveBackupDatasets(pool)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve backup scope for %s: %w", pool, err)
	}

	entries, err := listSnapshotEntries(ctx, r, pool, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots on %s: %w", pool, err)
	}

	usage, err := listDatasetUsage(ctx, r, pool)
	if err != nil {
		return nil, fmt.Errorf("failed to read space usage for %s: %w", pool, err)
	}

	now := time.Now()
	return &orphanScan{
		Pool:     pool,
		InScope:  inScope,
		Missing:  missing,
		Orphans:  scanOrphans(entries, pool, inScope, now, minSyncoidOrphanAge),
		Usage:    usage,
		ScanTime: now,
	}, nil
}

// =============================================================================
// doctor subcommand
// =============================================================================

// renderDoctorReport renders a scan as a human-readable report and returns how
// many issue groups it found. Shared by the CLI subcommand and the TUI health
// screen so both always say exactly the same thing.
func renderDoctorReport(scan *orphanScan) (string, int) {
	var b strings.Builder
	problems := 0

	b.WriteString(describeScope(scan.Pool, scan.InScope, scan.Missing) + "\n")
	if len(scan.Missing) > 0 {
		b.WriteString("  These datasets are configured for backup but no longer exist.\n")
	}
	b.WriteString("\n")

	datasets, grouped := groupOrphansByDataset(scan.Orphans)
	if len(datasets) == 0 {
		b.WriteString("[OK] No orphaned zfs-backup or syncoid snapshots found.\n")
	} else {
		problems++
		b.WriteString(fmt.Sprintf("[!] %d orphaned snapshot(s) on %d dataset(s):\n\n",
			len(scan.Orphans), len(datasets)))
		for _, ds := range datasets {
			var backupCount, syncoidCount int
			var unique int64
			var oldest, newest time.Time
			for _, o := range grouped[ds] {
				if o.Kind == orphanSyncoid {
					syncoidCount++
				} else {
					backupCount++
				}
				if o.Used > 0 {
					unique += o.Used
				}
				if oldest.IsZero() || o.Creation.Before(oldest) {
					oldest = o.Creation
				}
				if o.Creation.After(newest) {
					newest = o.Creation
				}
			}
			b.WriteString(fmt.Sprintf("  %s\n", ds))
			b.WriteString(fmt.Sprintf("    %d zfs-backup snapshot(s), %d syncoid leftover(s)\n",
				backupCount, syncoidCount))
			if !oldest.IsZero() {
				b.WriteString(fmt.Sprintf("    spanning %s → %s\n",
					oldest.Format("2006-01-02"), newest.Format("2006-01-02")))
			}
			b.WriteString(fmt.Sprintf("    %s uniquely referenced (shared blocks are not counted here)\n",
				formatSize(unique)))
		}
		b.WriteString("\n  Reclaim them with: sudo zfs-backup cleanup-orphans --pool " + scan.Pool + "\n")
	}
	b.WriteString("\n")

	flagged := flagQuotaPressure(scan.Usage, quotaPressureThreshold)
	if len(flagged) == 0 {
		b.WriteString("[OK] No dataset is dominated by snapshot usage.\n")
	} else {
		problems++
		b.WriteString("[!] Snapshots dominate space usage on:\n\n")
		for _, u := range flagged {
			quota := "none"
			if u.Quota > 0 {
				quota = formatSize(u.Quota)
			}
			b.WriteString(fmt.Sprintf("  %s\n", u.Name))
			b.WriteString(fmt.Sprintf("    used %s, of which %s is snapshots (quota %s)\n",
				formatSize(u.Used), formatSize(u.UsedBySnapshots), quota))
		}
		b.WriteString("\n  Note: `quota` counts snapshots against the limit, `refquota` does not.\n")
		b.WriteString("  A dataset with a `quota` starts failing writes once snapshots fill it.\n")
	}

	b.WriteString("\n")
	if problems == 0 {
		b.WriteString("Verdict: healthy.\n")
	} else {
		b.WriteString(fmt.Sprintf("Verdict: %d issue group(s) need attention.\n", problems))
	}

	return b.String(), problems
}

// runDoctor prints a read-only health report for a pool. It returns the
// process exit code: 0 when the pool is clean, 1 when problems were found.
func runDoctor(ctx context.Context, r commandRunner, pool string) int {
	fmt.Println()
	fmt.Println(titleStyle.Render("zfs-backup doctor"))
	fmt.Println(interstitialStyle.Render(strings.Repeat("─", 60)))
	fmt.Println()

	scan, err := collectOrphanScan(ctx, r, pool)
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return 1
	}

	report, problems := renderDoctorReport(scan)
	fmt.Println(report)

	if problems == 0 {
		return 0
	}
	return 1
}

// =============================================================================
// Cleanup planning - shared by the TUI screen and the CLI subcommand
// =============================================================================

// cleanupOptions controls the cleanup-orphans subcommand.
type cleanupOptions struct {
	Pool    string
	Dataset string // optional: restrict to a single dataset
	Confirm bool   // --yes: actually destroy (dry run is the default)
	Force   bool   // --force: skip the typed confirmation prompt
}

// cleanupPlan is the vetted answer to "what would cleanup destroy?". Building a
// plan performs no destructive work whatsoever - it only reads.
type cleanupPlan struct {
	Scan      *orphanScan
	Dataset   string            // the dataset filter that was applied, if any
	Decisions []destroyDecision // every candidate, safe or skipped
	Targets   []string          // the subset cleared for destruction
}

// buildCleanupPlan scans a pool and vets every orphan it finds. Both the TUI
// cleanup screen and the cleanup-orphans subcommand go through this one
// function, so a snapshot the CLI would refuse to touch is equally untouchable
// from the menu.
func buildCleanupPlan(ctx context.Context, r commandRunner, pool, dataset string) (*cleanupPlan, error) {
	scan, err := collectOrphanScan(ctx, r, pool)
	if err != nil {
		return nil, err
	}

	orphans := scan.Orphans
	if dataset != "" {
		var filtered []orphanSnapshot
		for _, o := range orphans {
			if o.Dataset == dataset {
				filtered = append(filtered, o)
			}
		}
		orphans = filtered
	}

	decisions := vetOrphans(ctx, r, orphans)
	return &cleanupPlan{
		Scan:      scan,
		Dataset:   dataset,
		Decisions: decisions,
		Targets:   safeToDestroy(decisions),
	}, nil
}

// uniqueBytes totals the space uniquely held by the snapshots cleared for
// destruction. It is a floor, not a promise: blocks shared with a snapshot that
// survives are counted against neither, so the real saving is usually larger.
func (p *cleanupPlan) uniqueBytes() int64 {
	var total int64
	for _, d := range p.Decisions {
		if d.Safe && d.Orphan.Used > 0 {
			total += d.Orphan.Used
		}
	}
	return total
}

// skipped returns the candidates a safety check refused to destroy.
func (p *cleanupPlan) skipped() []destroyDecision {
	var out []destroyDecision
	for _, d := range p.Decisions {
		if !d.Safe {
			out = append(out, d)
		}
	}
	return out
}

// renderCleanupPlan describes a plan as text: what would go, per dataset, and
// what a safety check held back. Returned as a string so the TUI can put it in
// a viewport and the CLI can print it verbatim.
func renderCleanupPlan(plan *cleanupPlan) string {
	byDataset := map[string][]destroyDecision{}
	var order []string
	for _, d := range plan.Decisions {
		ds := d.Orphan.Dataset
		if _, seen := byDataset[ds]; !seen {
			order = append(order, ds)
		}
		byDataset[ds] = append(byDataset[ds], d)
	}
	sort.Strings(order)

	var b strings.Builder
	for _, ds := range order {
		var safe, skipped int
		var unique int64
		for _, d := range byDataset[ds] {
			if d.Safe {
				safe++
				if d.Orphan.Used > 0 {
					unique += d.Orphan.Used
				}
			} else {
				skipped++
			}
		}
		fmt.Fprintf(&b, "  %s\n", labelStyle.Render(ds))
		fmt.Fprintf(&b, "    %d snapshot(s) to destroy, %s uniquely referenced\n",
			safe, formatSize(unique))
		if skipped > 0 {
			for _, d := range byDataset[ds] {
				if !d.Safe {
					b.WriteString(warningStyle.Render(fmt.Sprintf(
						"    skipping %s (%s)", d.Orphan.Name, d.SkipReason)))
					b.WriteString("\n")
				}
			}
		}
	}
	return b.String()
}

// previewDestroy asks ZFS what each destroy would free, without destroying
// anything. Returns one line per snapshot.
func previewDestroy(ctx context.Context, r commandRunner, targets []string) []string {
	lines := make([]string, 0, len(targets))
	for _, name := range targets {
		out, err := r.Output(ctx, "zfs", "destroy", "-nv", name)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s: dry run failed: %v", name, err))
			continue
		}
		if trimmed := strings.TrimSpace(out); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// cleanupOutcome records what actually happened during a destroy run.
type cleanupOutcome struct {
	Destroyed []string
	Failures  []string
}

// destroyPlannedSnapshots destroys the vetted targets one at a time. progress,
// if non-nil, is called after each snapshot so a UI can show movement.
//
// One snapshot per call - never a range expression, which would happily take
// out snapshots that never appeared in the plan.
func destroyPlannedSnapshots(ctx context.Context, r commandRunner, targets []string, progress func(string)) cleanupOutcome {
	var outcome cleanupOutcome
	for _, name := range targets {
		if err := r.Run(ctx, "zfs", "destroy", name); err != nil {
			outcome.Failures = append(outcome.Failures, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		outcome.Destroyed = append(outcome.Destroyed, name)
		if progress != nil {
			progress(name)
		}
	}
	return outcome
}

// =============================================================================
// cleanup-orphans subcommand
// =============================================================================

// runCleanupOrphans reports, and optionally destroys, orphaned snapshots. Dry
// run is the default; destroying requires --yes plus a typed confirmation.
// It returns the process exit code.
func runCleanupOrphans(ctx context.Context, r commandRunner, opts cleanupOptions, confirmFn func(string) bool) int {
	fmt.Println()
	fmt.Println(titleStyle.Render("zfs-backup cleanup-orphans"))
	fmt.Println(interstitialStyle.Render(strings.Repeat("─", 60)))
	fmt.Println()

	plan, err := buildCleanupPlan(ctx, r, opts.Pool, opts.Dataset)
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return 1
	}

	if len(plan.Decisions) == 0 {
		fmt.Println(statusStyle.Render("[OK] Nothing to clean up."))
		fmt.Println()
		return 0
	}

	fmt.Println(infoStyle.Render(describeScope(plan.Scan.Pool, plan.Scan.InScope, plan.Scan.Missing)))
	fmt.Println(infoStyle.Render("Datasets in scope are never touched by this command."))
	fmt.Println()
	fmt.Print(renderCleanupPlan(plan))
	fmt.Println()

	if len(plan.Targets) == 0 {
		fmt.Println(warningStyle.Render("Every candidate was skipped by a safety check. Nothing to do."))
		fmt.Println()
		return 0
	}

	if !opts.Confirm {
		fmt.Println(infoStyle.Render("Dry run - nothing has been destroyed."))
		fmt.Println()
		for _, line := range previewDestroy(ctx, r, plan.Targets) {
			fmt.Printf("  %s\n", line)
		}
		fmt.Println()
		fmt.Println(infoStyle.Render(fmt.Sprintf(
			"Re-run with --yes to destroy these %d snapshot(s).", len(plan.Targets))))
		fmt.Println()
		return 0
	}

	fmt.Println(destructiveWarningStyle.Render(
		"  DESTROYING SNAPSHOTS IS IRREVERSIBLE  "))
	fmt.Println()
	fmt.Println(warningStyle.Render(fmt.Sprintf(
		"About to destroy %d snapshot(s) on pool %s.", len(plan.Targets), plan.Scan.Pool)))
	fmt.Println(infoStyle.Render(reclaimCaveat))
	fmt.Println()

	if !opts.Force && !confirmFn("Type DESTROY to continue: ") {
		fmt.Println(statusStyle.Render("Aborted. Nothing was destroyed."))
		fmt.Println()
		return 1
	}

	outcome := destroyPlannedSnapshots(ctx, r, plan.Targets, func(name string) {
		fmt.Printf("  destroyed %s\n", name)
	})

	fmt.Println()
	fmt.Println(statusStyle.Render(fmt.Sprintf("Destroyed %d of %d snapshot(s).",
		len(outcome.Destroyed), len(plan.Targets))))
	for _, f := range outcome.Failures {
		fmt.Println(errorStyle.Render("  failed: " + f))
	}

	if usage, err := listDatasetUsage(ctx, r, plan.Scan.Pool); err == nil {
		fmt.Println()
		fmt.Println(labelStyle.Render("Space after cleanup:"))
		for _, u := range usage {
			fmt.Printf("  %-32s used %10s  snapshots %10s\n",
				u.Name, formatSize(u.Used), formatSize(u.UsedBySnapshots))
		}
	}
	fmt.Println()

	if len(outcome.Failures) > 0 {
		return 1
	}
	return 0
}

// reclaimCaveat explains why freed space often looks disappointing at first.
// Shown identically by the CLI and the TUI cleanup screen.
const reclaimCaveat = "Space is only reclaimed once every snapshot pinning a block is gone, so\n" +
	"usage may barely move until the last few are destroyed."
