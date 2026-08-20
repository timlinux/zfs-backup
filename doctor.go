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
// cleanup-orphans subcommand
// =============================================================================

// cleanupOptions controls the cleanup-orphans subcommand.
type cleanupOptions struct {
	Pool    string
	Dataset string // optional: restrict to a single dataset
	Confirm bool   // --yes: actually destroy (dry run is the default)
	Force   bool   // --force: skip the typed confirmation prompt
}

// runCleanupOrphans reports, and optionally destroys, orphaned snapshots. Dry
// run is the default; destroying requires --yes plus a typed confirmation.
// It returns the process exit code.
func runCleanupOrphans(ctx context.Context, r commandRunner, opts cleanupOptions, confirmFn func(string) bool) int {
	fmt.Println()
	fmt.Println(titleStyle.Render("zfs-backup cleanup-orphans"))
	fmt.Println(interstitialStyle.Render(strings.Repeat("─", 60)))
	fmt.Println()

	scan, err := collectOrphanScan(ctx, r, opts.Pool)
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return 1
	}

	orphans := scan.Orphans
	if opts.Dataset != "" {
		var filtered []orphanSnapshot
		for _, o := range orphans {
			if o.Dataset == opts.Dataset {
				filtered = append(filtered, o)
			}
		}
		orphans = filtered
	}

	if len(orphans) == 0 {
		fmt.Println(statusStyle.Render("[OK] Nothing to clean up."))
		fmt.Println()
		return 0
	}

	fmt.Println(infoStyle.Render(describeScope(scan.Pool, scan.InScope, scan.Missing)))
	fmt.Println(infoStyle.Render("Datasets in scope are never touched by this command."))
	fmt.Println()

	decisions := vetOrphans(ctx, r, orphans)
	printCleanupPlan(decisions)

	targets := safeToDestroy(decisions)
	if len(targets) == 0 {
		fmt.Println(warningStyle.Render("Every candidate was skipped by a safety check. Nothing to do."))
		fmt.Println()
		return 0
	}

	if !opts.Confirm {
		fmt.Println(infoStyle.Render("Dry run - nothing has been destroyed."))
		fmt.Println()
		for _, name := range targets {
			if out, err := r.Output(ctx, "zfs", "destroy", "-nv", name); err == nil {
				trimmed := strings.TrimSpace(out)
				if trimmed != "" {
					fmt.Printf("  %s\n", trimmed)
				}
			} else {
				fmt.Println(warningStyle.Render(fmt.Sprintf("  %s: dry run failed: %v", name, err)))
			}
		}
		fmt.Println()
		fmt.Println(infoStyle.Render(fmt.Sprintf(
			"Re-run with --yes to destroy these %d snapshot(s).", len(targets))))
		fmt.Println()
		return 0
	}

	fmt.Println(destructiveWarningStyle.Render(
		"  DESTROYING SNAPSHOTS IS IRREVERSIBLE  "))
	fmt.Println()
	fmt.Println(warningStyle.Render(fmt.Sprintf(
		"About to destroy %d snapshot(s) on pool %s.", len(targets), scan.Pool)))
	fmt.Println(infoStyle.Render(
		"Space is only reclaimed once every snapshot pinning a block is gone, so\n" +
			"usage may barely move until the last few are destroyed."))
	fmt.Println()

	if !opts.Force && !confirmFn("Type DESTROY to continue: ") {
		fmt.Println(statusStyle.Render("Aborted. Nothing was destroyed."))
		fmt.Println()
		return 1
	}

	destroyed := 0
	var failures []string
	for _, name := range targets {
		// One snapshot at a time - never a range expression, which would
		// happily take out snapshots that did not match the pattern.
		if err := r.Run(ctx, "zfs", "destroy", name); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		destroyed++
		fmt.Printf("  destroyed %s\n", name)
	}

	fmt.Println()
	fmt.Println(statusStyle.Render(fmt.Sprintf("Destroyed %d of %d snapshot(s).", destroyed, len(targets))))
	for _, f := range failures {
		fmt.Println(errorStyle.Render("  failed: " + f))
	}

	if usage, err := listDatasetUsage(ctx, r, scan.Pool); err == nil {
		fmt.Println()
		fmt.Println(labelStyle.Render("Space after cleanup:"))
		for _, u := range usage {
			fmt.Printf("  %-32s used %10s  snapshots %10s\n",
				u.Name, formatSize(u.Used), formatSize(u.UsedBySnapshots))
		}
	}
	fmt.Println()

	if len(failures) > 0 {
		return 1
	}
	return 0
}

// printCleanupPlan renders the per-dataset summary of what cleanup would do.
func printCleanupPlan(decisions []destroyDecision) {
	byDataset := map[string][]destroyDecision{}
	var order []string
	for _, d := range decisions {
		ds := d.Orphan.Dataset
		if _, seen := byDataset[ds]; !seen {
			order = append(order, ds)
		}
		byDataset[ds] = append(byDataset[ds], d)
	}
	sort.Strings(order)

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
		fmt.Printf("  %s\n", labelStyle.Render(ds))
		fmt.Printf("    %d snapshot(s) to destroy, %s uniquely referenced\n", safe, formatSize(unique))
		if skipped > 0 {
			for _, d := range byDataset[ds] {
				if !d.Safe {
					fmt.Println(warningStyle.Render(fmt.Sprintf(
						"    skipping %s (%s)", d.Orphan.Name, d.SkipReason)))
				}
			}
		}
	}
	fmt.Println()
}
