// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// =============================================================================
// prune-snapshots subcommand: retention-based pruning of the SOURCE pool
// =============================================================================
//
// Unlike cleanup-orphans (which only ever removes debris nothing manages),
// this tool trims snapshot HISTORY that something legitimately keeps adding
// to: zfs-backup's own -Backup snapshots (already thinned automatically on
// every backup run - this just gives a way to do it without waiting for one)
// and sanoid's autosnap_* snapshots (which zfs-backup has never touched -
// sanoid manages its own retention, but that retention is not always tight
// enough to keep a quota'd dataset out of trouble). Both families are
// thinned under the same grandfather-father-son policy as the backup
// destination, evaluated SEPARATELY per dataset and per family: sanoid runs
// far more often than zfs-backup, so bucketing the two families together
// would let a merely-newer sanoid snapshot outcompete zfs-backup's own
// snapshot for a retention slot.
//
// Pruned zfs-backup snapshots are converted to bookmarks first, exactly like
// the automatic prune stages, so the incremental chain survives. Pruned
// sanoid snapshots are destroyed outright: zfs-backup never uses them as a
// send base, so a bookmark would serve no purpose but clutter.

// sourcePruneKind records which family a source-prune candidate belongs to,
// which decides how it is destroyed.
type sourcePruneKind string

const (
	sourcePruneOwn    sourcePruneKind = "zfs-backup"
	sourcePruneSanoid sourcePruneKind = "sanoid autosnap"
)

// sourcePruneCandidate is one snapshot the retention policy no longer wants
// kept on the source pool.
type sourcePruneCandidate struct {
	snapshotEntry
	Kind sourcePruneKind
}

// sourcePruneCandidatesForDataset returns the retention-policy prune
// candidates for one dataset's snapshot listing. The zfs-backup and sanoid
// families are bucketed independently - see the package comment above for
// why mixing them would let a merely-newer sanoid snapshot cost the
// zfs-backup family a retention slot it should have kept.
func sourcePruneCandidatesForDataset(entries []snapshotEntry, now time.Time, policy retentionPolicy) []sourcePruneCandidate {
	var own, sanoid []snapshotEntry
	for _, e := range filterManagedSourceSnapshots(entries) {
		switch {
		case isBackupSnapshotTag(e.Tag):
			own = append(own, e)
		case isSanoidAutosnapTag(e.Tag):
			sanoid = append(sanoid, e)
		}
	}

	var candidates []sourcePruneCandidate
	for _, e := range selectPruneCandidates(own, now, policy) {
		candidates = append(candidates, sourcePruneCandidate{snapshotEntry: e, Kind: sourcePruneOwn})
	}
	for _, e := range selectPruneCandidates(sanoid, now, policy) {
		candidates = append(candidates, sourcePruneCandidate{snapshotEntry: e, Kind: sourcePruneSanoid})
	}
	return candidates
}

// sourcePruneDecision records whether one source-retention candidate may be
// destroyed.
type sourcePruneDecision struct {
	Candidate  sourcePruneCandidate
	Safe       bool
	SkipReason string
}

// vetSourcePruneCandidates applies the same mandatory checks vetOrphans does:
// never touch a protected, held, or cloned snapshot.
func vetSourcePruneCandidates(ctx context.Context, r commandRunner, candidates []sourcePruneCandidate) []sourcePruneDecision {
	decisions := make([]sourcePruneDecision, 0, len(candidates))
	for _, c := range candidates {
		safe, reason := snapshotDestroySafety(ctx, r, c.Tag, c.Name)
		decisions = append(decisions, sourcePruneDecision{Candidate: c, Safe: safe, SkipReason: reason})
	}
	return decisions
}

// safeSourcePruneTargets returns the candidates vetSourcePruneCandidates
// cleared for destruction.
func safeSourcePruneTargets(decisions []sourcePruneDecision) []sourcePruneCandidate {
	var targets []sourcePruneCandidate
	for _, d := range decisions {
		if d.Safe {
			targets = append(targets, d.Candidate)
		}
	}
	return targets
}

// sourcePrunePlan is the vetted answer to "what would prune-snapshots
// destroy?". Building a plan performs no destructive work whatsoever - it
// only reads.
type sourcePrunePlan struct {
	Pool      string
	InScope   []string
	Missing   []string
	Scoped    bool // mirrors orphanScan.ScopeConfigured
	Decisions []sourcePruneDecision
	Targets   []sourcePruneCandidate
}

// buildSourcePrunePlan scans every in-scope dataset on pool and vets every
// retention-policy prune candidate it finds. Only datasets zfs-backup already
// backs up are ever touched - see the snapshot scope invariant in
// datasets.go - so this tool can never reach a dataset the rest of zfs-backup
// does not already know about.
func buildSourcePrunePlan(ctx context.Context, r commandRunner, pool string, now time.Time, policy retentionPolicy) (*sourcePrunePlan, error) {
	inScope, missing, err := resolveBackupDatasets(pool)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve backup scope for %s: %w", pool, err)
	}
	configured, err := IsPoolScopeConfigured(pool)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup scope for %s: %w", pool, err)
	}

	var candidates []sourcePruneCandidate
	for _, ds := range inScope {
		fullDS := fmt.Sprintf("%s/%s", pool, ds)
		entries, err := listSnapshotEntries(ctx, r, fullDS, 1)
		if err != nil {
			continue // a dataset that vanished mid-scan has nothing to prune
		}
		candidates = append(candidates, sourcePruneCandidatesForDataset(entries, now, policy)...)
	}

	decisions := vetSourcePruneCandidates(ctx, r, candidates)
	return &sourcePrunePlan{
		Pool:      pool,
		InScope:   inScope,
		Missing:   missing,
		Scoped:    configured,
		Decisions: decisions,
		Targets:   safeSourcePruneTargets(decisions),
	}, nil
}

// uniqueBytes totals the space uniquely held by the candidates cleared for
// destruction. Like cleanupPlan.uniqueBytes, this is a floor, not a promise.
func (p *sourcePrunePlan) uniqueBytes() int64 {
	var total int64
	for _, d := range p.Decisions {
		if d.Safe && d.Candidate.Used > 0 {
			total += d.Candidate.Used
		}
	}
	return total
}

// renderSourcePrunePlan describes a plan as text, grouped by dataset with a
// zfs-backup/sanoid breakdown, mirroring renderCleanupPlan.
func renderSourcePrunePlan(plan *sourcePrunePlan) string {
	byDataset := map[string][]sourcePruneDecision{}
	var order []string
	for _, d := range plan.Decisions {
		ds := d.Candidate.Dataset
		if _, seen := byDataset[ds]; !seen {
			order = append(order, ds)
		}
		byDataset[ds] = append(byDataset[ds], d)
	}
	sort.Strings(order)

	var b strings.Builder
	for _, ds := range order {
		var ownCount, sanoidCount int
		var unique int64
		var skipped []sourcePruneDecision
		for _, d := range byDataset[ds] {
			if !d.Safe {
				skipped = append(skipped, d)
				continue
			}
			if d.Candidate.Kind == sourcePruneOwn {
				ownCount++
			} else {
				sanoidCount++
			}
			if d.Candidate.Used > 0 {
				unique += d.Candidate.Used
			}
		}
		fmt.Fprintf(&b, "  %s\n", labelStyle.Render(ds))
		fmt.Fprintf(&b, "    %d zfs-backup snapshot(s), %d sanoid autosnap(s) to prune, %s uniquely referenced\n",
			ownCount, sanoidCount, formatSize(unique))
		for _, d := range skipped {
			b.WriteString(warningStyle.Render(fmt.Sprintf(
				"    skipping %s (%s)", d.Candidate.Name, d.SkipReason)))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// destroySourcePruneCandidates destroys the vetted targets one at a time:
// zfs-backup's own snapshots are bookmarked first so the incremental chain
// survives, sanoid's autosnap_* snapshots are destroyed outright since
// zfs-backup never uses them as a send base. progress, if non-nil, is called
// after each snapshot so a UI can show movement.
func destroySourcePruneCandidates(ctx context.Context, r commandRunner, targets []sourcePruneCandidate, progress func(string)) cleanupOutcome {
	var outcome cleanupOutcome
	for _, t := range targets {
		var err error
		if t.Kind == sourcePruneOwn {
			err = bookmarkAndDestroy(ctx, r, t.Name)
		} else {
			err = r.Run(ctx, "zfs", "destroy", t.Name)
		}
		if err != nil {
			outcome.Failures = append(outcome.Failures, fmt.Sprintf("%s: %s", t.Name, explainDestroyFailure(err)))
			continue
		}
		outcome.Destroyed = append(outcome.Destroyed, t.Name)
		if progress != nil {
			progress(t.Name)
		}
	}
	return outcome
}

// sourcePruneOptions controls the prune-snapshots subcommand.
type sourcePruneOptions struct {
	Pool    string
	Confirm bool // --yes: actually destroy (dry run is the default)
	Force   bool // --force: skip the typed confirmation prompt
}

// runPruneSourceSnapshots reports, and optionally destroys, source-pool
// snapshots the retention policy no longer wants kept. Dry run is the
// default; destroying requires --yes plus a typed confirmation. It returns
// the process exit code.
func runPruneSourceSnapshots(ctx context.Context, r commandRunner, opts sourcePruneOptions, confirmFn func(string) bool) int {
	fmt.Println()
	fmt.Println(titleStyle.Render("zfs-backup prune-snapshots"))
	fmt.Println(interstitialStyle.Render(strings.Repeat("─", 60)))
	fmt.Println()

	plan, err := buildSourcePrunePlan(ctx, r, opts.Pool, time.Now(), defaultRetentionPolicy)
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return 1
	}

	if len(plan.Decisions) == 0 {
		fmt.Println(statusStyle.Render("[OK] Nothing to prune - every snapshot is within the retention policy."))
		fmt.Println()
		return 0
	}

	fmt.Println(infoStyle.Render(describeScope(plan.Pool, plan.InScope, plan.Missing)))
	fmt.Println(infoStyle.Render(retentionPolicyDescription(defaultRetentionPolicy)))
	fmt.Println(infoStyle.Render("Only datasets already backed up by zfs-backup are ever touched."))
	fmt.Println()
	fmt.Print(renderSourcePrunePlan(plan))
	fmt.Println()

	if len(plan.Targets) == 0 {
		fmt.Println(warningStyle.Render("Every candidate was skipped by a safety check. Nothing to do."))
		fmt.Println()
		return 0
	}

	if !opts.Confirm {
		fmt.Println(infoStyle.Render("Dry run - nothing has been destroyed."))
		fmt.Println()
		names := make([]string, len(plan.Targets))
		for i, t := range plan.Targets {
			names[i] = t.Name
		}
		for _, line := range previewDestroy(ctx, r, names) {
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
		"About to destroy %d snapshot(s) on pool %s.", len(plan.Targets), plan.Pool)))
	fmt.Println(infoStyle.Render(reclaimCaveat))
	fmt.Println(infoStyle.Render(
		"zfs-backup's own snapshots survive as bookmarks, so incremental sends keep working. Pruned sanoid snapshots do not - they are gone."))
	fmt.Println()

	if !opts.Force && !confirmFn("Type DESTROY to continue: ") {
		fmt.Println(statusStyle.Render("Aborted. Nothing was destroyed."))
		fmt.Println()
		return 1
	}

	outcome := destroySourcePruneCandidates(ctx, r, plan.Targets, func(name string) {
		fmt.Printf("  destroyed %s\n", name)
	})

	fmt.Println()
	fmt.Println(statusStyle.Render(fmt.Sprintf("Destroyed %d of %d snapshot(s).",
		len(outcome.Destroyed), len(plan.Targets))))
	for _, f := range outcome.Failures {
		fmt.Println(errorStyle.Render("  failed: " + f))
	}

	if usage, err := listDatasetUsage(ctx, r, plan.Pool); err == nil {
		fmt.Println()
		fmt.Println(labelStyle.Render("Space after pruning:"))
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
