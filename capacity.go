// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// =============================================================================
// Pre-flight capacity check
// =============================================================================
//
// A sync stage that runs out of destination space partway through does not
// fail cleanly: syncoid dies mid-stream, `zfs receive` is left holding a
// partial-receive resume token, and the next run either resumes into the
// same wall or - if the source snapshot it was resuming from has since
// rotated away - hits the unrelated "cannot resume send" failure that
// runSyncoidWithResumeRecovery exists to clean up after. It is cheaper and
// far more honest to size the run before it starts: `zfs send -nP` is a dry
// run, so every number here comes from ZFS's own accounting, not a guess.

// datasetAvailableBytes returns a dataset's `available` property in bytes -
// the space still free for new writes, accounting for reservations exactly
// as ZFS itself does. ok is false if the property could not be read.
func datasetAvailableBytes(ctx context.Context, r commandRunner, dataset string) (int64, bool) {
	out, err := r.Output(ctx, "zfs", "get", "-H", "-p", "-o", "value", "available", dataset)
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// datasetRequirement is one dataset's estimated contribution to a backup
// run's destination space requirement.
type datasetRequirement struct {
	Dataset       string // suffix, e.g. "home"
	RequiredBytes int64
}

// estimateDatasetSendBytes sizes one dataset's contribution to a backup run,
// mirroring the estimate trackSyncProgress computes at the start of that
// dataset's own sync - but callable up front, before any dataset's sync
// begins, so a pre-flight check can size every dataset in one pass. snaps is
// the source's snapshot listing (local or remote - the caller decides how to
// fetch it); listDest reports which of those tags have already reached the
// destination.
func estimateDatasetSendBytes(ctx context.Context, r commandRunner, sourceDataset string, snaps []snapshotInfo, listDest func() map[string]bool) int64 {
	dots := makeSnapshotDots(snaps, SnapPending)
	applySnapshotProgress(dots, listDest(), true)
	fallback, _ := datasetUsedBytes(ctx, r, sourceDataset)
	return estimateSendBytes(ctx, r, sourceDataset, dots, fallback)
}

// CapacityCheck compares what a backup run is estimated to need on the
// destination against what is actually free there right now.
type CapacityCheck struct {
	Requirements   []datasetRequirement
	RequiredBytes  int64
	AvailableBytes int64
	AvailableKnown bool // false if the destination's available space could not be read
}

// Sufficient reports whether the destination has enough free space for the
// estimated requirement. An unknown available figure is treated as
// insufficient - proceeding on a number we could not read is exactly the
// silent-crash behaviour this check exists to replace.
func (c CapacityCheck) Sufficient() bool {
	return c.AvailableKnown && c.AvailableBytes >= c.RequiredBytes
}

// ShortfallBytes is how much more space the run needs than is free, or 0 if
// there is enough (or the available figure is unknown, in which case there
// is nothing meaningful to report as a shortfall amount).
func (c CapacityCheck) ShortfallBytes() int64 {
	if !c.AvailableKnown || c.AvailableBytes >= c.RequiredBytes {
		return 0
	}
	return c.RequiredBytes - c.AvailableBytes
}

// capacityShortfallSummary renders a one-line explanation of an insufficient
// CapacityCheck, suitable as (or folded into) an error message. The full
// capacityAdvisory is written to the run log separately, where its
// per-dataset breakdown and options list have room to be useful.
func capacityShortfallSummary(check CapacityCheck) string {
	if !check.AvailableKnown {
		return fmt.Sprintf("could not determine free space on the destination (need an estimated %s)", formatSize(check.RequiredBytes))
	}
	return fmt.Sprintf("need %s, have %s free (short by %s)",
		formatSize(check.RequiredBytes), formatSize(check.AvailableBytes), formatSize(check.ShortfallBytes()))
}

// checkBackupCapacity totals requirements and compares them against the
// destination pool's free space.
func checkBackupCapacity(ctx context.Context, destRunner commandRunner, destPool string, requirements []datasetRequirement) CapacityCheck {
	var total int64
	for _, req := range requirements {
		total += req.RequiredBytes
	}
	available, ok := datasetAvailableBytes(ctx, destRunner, destPool)
	return CapacityCheck{
		Requirements:   requirements,
		RequiredBytes:  total,
		AvailableBytes: available,
		AvailableKnown: ok,
	}
}

// capacityAdvisory renders a clear, actionable breakdown of a capacity
// shortfall: how much this run needs, how much is free, a per-dataset
// breakdown so the user can see where the space is going, and what to do
// about it. reclaimedBytes is what a proactive retention prune already
// freed before this advisory was produced (0 if none was attempted or
// nothing was reclaimed).
func capacityAdvisory(check CapacityCheck, reclaimedBytes int64) string {
	var b strings.Builder

	if !check.AvailableKnown {
		fmt.Fprintf(&b, "This backup needs an estimated %s, but the free space on the destination could not be read.\n", formatSize(check.RequiredBytes))
	} else {
		fmt.Fprintf(&b, "This backup needs an estimated %s, but only %s is free on the destination.\n",
			formatSize(check.RequiredBytes), formatSize(check.AvailableBytes))
		fmt.Fprintf(&b, "Shortfall: %s\n", formatSize(check.ShortfallBytes()))
	}

	if reclaimedBytes > 0 {
		fmt.Fprintf(&b, "\nPruning old backup snapshots under the retention policy freed %s just now - still not enough.\n", formatSize(reclaimedBytes))
	}

	if len(check.Requirements) > 0 {
		b.WriteString("\nPer-dataset estimate:\n")
		for _, req := range check.Requirements {
			fmt.Fprintf(&b, "  %-24s %s\n", req.Dataset, formatSize(req.RequiredBytes))
		}
	}

	b.WriteString("\nOptions:\n")
	b.WriteString("  - Use a larger backup disk\n")
	b.WriteString("  - Narrow the backup scope (Backup Scope in the menu) to exclude a large dataset\n")
	b.WriteString("  - Free space manually on the destination pool, then try again\n")

	return b.String()
}
