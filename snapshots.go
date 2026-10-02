// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// =============================================================================
// Snapshot naming
// =============================================================================

// snapshotTimeLayout is the timestamp layout used in zfs-backup snapshot tags,
// e.g. 2026-08-13.23h-47.
const snapshotTimeLayout = "2006-01-02.15h-04"

// backupSnapshotSuffix marks a snapshot as created by zfs-backup.
const backupSnapshotSuffix = "-Backup"

// backupSnapshotPattern matches snapshot tags this tool creates, e.g.
// 2026-08-13.23h-47-Backup. Only snapshots matching this pattern are ever
// pruned or destroyed by zfs-backup - anything the user or another tool
// created is left strictly alone.
var backupSnapshotPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.\d{2}h-\d{2}` + backupSnapshotSuffix + `$`)

// syncoidSnapshotPattern matches syncoid's own sync-snapshots, e.g.
// syncoid_abyss_2026-05-19:00:49:53-GMT01:00. zfs-backup passes
// --no-sync-snap so it no longer creates these, but older runs left them
// behind on failed sends.
var syncoidSnapshotPattern = regexp.MustCompile(`^syncoid_.+`)

// sanoidAutosnapPattern matches sanoid's own periodic snapshots, e.g.
// autosnap_2026-07-22_22:00:00_hourly. sanoid is a separate service that
// manages its own creation schedule; zfs-backup never creates these, but the
// source-retention tool (prune-snapshots) is allowed to thin them out under
// the same GFS policy as its own snapshots, since sanoid's own retention is
// not always tight enough to keep a quota'd dataset out of trouble.
var sanoidAutosnapPattern = regexp.MustCompile(`^autosnap_.+`)

// protectedSnapshotTags are never touched under any circumstance. On NixOS
// "erase your darlings" installs, POOL/root@blank is rolled back to on every
// boot - destroying it breaks the system.
var protectedSnapshotTags = map[string]bool{
	"blank": true,
}

// snapshotTagForTime builds the snapshot tag zfs-backup uses for a run.
func snapshotTagForTime(t time.Time) string {
	return t.Format(snapshotTimeLayout) + backupSnapshotSuffix
}

// isBackupSnapshotTag reports whether a snapshot tag was created by zfs-backup.
func isBackupSnapshotTag(tag string) bool {
	return backupSnapshotPattern.MatchString(tag)
}

// isSyncoidSnapshotTag reports whether a snapshot tag is a syncoid sync-snapshot.
func isSyncoidSnapshotTag(tag string) bool {
	return syncoidSnapshotPattern.MatchString(tag)
}

// isSanoidAutosnapTag reports whether a snapshot tag is one of sanoid's own
// periodic snapshots.
func isSanoidAutosnapTag(tag string) bool {
	return sanoidAutosnapPattern.MatchString(tag)
}

// isProtectedSnapshotTag reports whether a snapshot must never be destroyed.
func isProtectedSnapshotTag(tag string) bool {
	return protectedSnapshotTags[tag]
}

// splitSnapshot splits a full snapshot name into its dataset and tag.
func splitSnapshot(name string) (dataset, tag string, ok bool) {
	idx := strings.Index(name, "@")
	if idx <= 0 || idx == len(name)-1 {
		return "", "", false
	}
	return name[:idx], name[idx+1:], true
}

// =============================================================================
// Snapshot listing
// =============================================================================

// snapshotEntry is one snapshot with the metadata the prune and orphan logic
// needs.
type snapshotEntry struct {
	Name     string    // full name, e.g. NIXROOT/home@2026-08-13.23h-47-Backup
	Dataset  string    // NIXROOT/home
	Tag      string    // 2026-08-13.23h-47-Backup
	Creation time.Time // creation time
	Used     int64     // bytes uniquely referenced by this snapshot (-1 if unknown)
}

// parseSnapshotEntries parses the output of
// `zfs list -H -p -t snapshot -o name,creation,used`. Lines that cannot be
// parsed are skipped rather than failing the whole listing.
func parseSnapshotEntries(output string) []snapshotEntry {
	var entries []snapshotEntry
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 1 {
			continue
		}

		dataset, tag, ok := splitSnapshot(strings.TrimSpace(fields[0]))
		if !ok {
			continue
		}

		entry := snapshotEntry{
			Name:    strings.TrimSpace(fields[0]),
			Dataset: dataset,
			Tag:     tag,
			Used:    -1,
		}
		if len(fields) > 1 {
			if secs, err := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64); err == nil {
				entry.Creation = time.Unix(secs, 0)
			}
		}
		if len(fields) > 2 {
			if used, err := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64); err == nil {
				entry.Used = used
			}
		}

		entries = append(entries, entry)
	}
	return entries
}

// listSnapshotEntries lists snapshots at or below the given target. depth of 1
// restricts the listing to the target dataset's own snapshots; depth of 0
// recurses through all descendants.
func listSnapshotEntries(ctx context.Context, r commandRunner, target string, depth int) ([]snapshotEntry, error) {
	args := []string{"list", "-H", "-p", "-t", "snapshot", "-o", "name,creation,used"}
	if depth > 0 {
		args = append(args, "-d", strconv.Itoa(depth))
	} else {
		args = append(args, "-r")
	}
	args = append(args, target)

	output, err := r.Output(ctx, "zfs", args...)
	if err != nil {
		return nil, err
	}
	return parseSnapshotEntries(output), nil
}

// sortSnapshotsNewestFirst orders snapshots by creation time, newest first,
// falling back to the name so the order is deterministic when two snapshots
// share a creation second.
func sortSnapshotsNewestFirst(entries []snapshotEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Creation.Equal(entries[j].Creation) {
			return entries[i].Name > entries[j].Name
		}
		return entries[i].Creation.After(entries[j].Creation)
	})
}

// filterBackupSnapshots keeps only the snapshots zfs-backup itself created.
func filterBackupSnapshots(entries []snapshotEntry) []snapshotEntry {
	var own []snapshotEntry
	for _, e := range entries {
		if isProtectedSnapshotTag(e.Tag) {
			continue
		}
		if isBackupSnapshotTag(e.Tag) {
			own = append(own, e)
		}
	}
	return own
}

// filterManagedSourceSnapshots keeps the snapshots the source-retention tool
// (prune-snapshots) is allowed to thin out: zfs-backup's own `-Backup`
// snapshots and sanoid's `autosnap_*` snapshots. Anything else - a syncoid
// leftover, a snapshot the user made by hand, @blank - is left strictly
// alone, exactly as filterBackupSnapshots leaves everything but its own
// pattern alone.
func filterManagedSourceSnapshots(entries []snapshotEntry) []snapshotEntry {
	var managed []snapshotEntry
	for _, e := range entries {
		if isProtectedSnapshotTag(e.Tag) {
			continue
		}
		if isBackupSnapshotTag(e.Tag) || isSanoidAutosnapTag(e.Tag) {
			managed = append(managed, e)
		}
	}
	return managed
}

// =============================================================================
// Snapshot creation
// =============================================================================

// createDatasetSnapshots snapshots exactly the datasets it is given - one
// `zfs snapshot` per dataset, never `zfs snapshot -r`.
//
// This upholds the snapshot scope invariant documented in datasets.go: a
// recursive snapshot would also cover the pool root and any nested
// descendants, which the replication and prune phases never visit, leaving
// snapshots that accumulate forever.
//
// If any dataset fails, the snapshots already created by this call are
// destroyed again so a failed run leaves no residue.
func createDatasetSnapshots(ctx context.Context, r commandRunner, pool string, datasets []string, tag string) ([]string, error) {
	if len(datasets) == 0 {
		return nil, fmt.Errorf("no datasets in scope for pool %s - nothing to snapshot", pool)
	}

	created := make([]string, 0, len(datasets))
	for _, ds := range datasets {
		name := fmt.Sprintf("%s/%s@%s", pool, ds, tag)
		if err := r.Run(ctx, "zfs", "snapshot", name); err != nil {
			// Roll back this run's snapshots so nothing is orphaned.
			destroySnapshots(ctx, r, created)
			return nil, fmt.Errorf("failed to create snapshot %s: %w", name, err)
		}
		created = append(created, name)
	}

	return created, nil
}

// destroySnapshots destroys the given snapshots, skipping protected ones. It
// returns the names it could not destroy. Errors are not fatal: this is used
// on cleanup paths where the caller is already reporting a failure.
func destroySnapshots(ctx context.Context, r commandRunner, names []string) []string {
	var failed []string
	for _, name := range names {
		if _, tag, ok := splitSnapshot(name); ok && isProtectedSnapshotTag(tag) {
			continue
		}
		if err := r.Run(ctx, "zfs", "destroy", name); err != nil {
			failed = append(failed, name)
		}
	}
	return failed
}

// snapshotsForDatasets returns the subset of snapshot names belonging to any of
// the given fully qualified datasets. Used to undo the snapshots of datasets
// whose replication failed.
func snapshotsForDatasets(snapshots []string, datasets []string) []string {
	wanted := make(map[string]bool, len(datasets))
	for _, ds := range datasets {
		wanted[ds] = true
	}

	var matched []string
	for _, snap := range snapshots {
		if dataset, _, ok := splitSnapshot(snap); ok && wanted[dataset] {
			matched = append(matched, snap)
		}
	}
	return matched
}

// =============================================================================
// Retention policy (grandfather-father-son)
// =============================================================================

// retentionPolicy is a grandfather-father-son retention schedule: keep the
// newest snapshot per calendar day for the last Daily days, per ISO week for
// the last Weekly weeks, per calendar month for the last Monthly months, and
// per calendar year for the last Yearly years (0 means forever - never age
// out yearly archives). The single newest snapshot overall is always kept
// regardless of policy, since it is the incremental base for the next run.
type retentionPolicy struct {
	Daily   int
	Weekly  int
	Monthly int
	Yearly  int // 0 = keep one snapshot per year forever
}

// defaultRetentionPolicy is zfs-backup's standard schedule: a daily snapshot
// for the last week, a weekly one for the last month, a monthly one for the
// last year, and one a year forever after that.
var defaultRetentionPolicy = retentionPolicy{Daily: 7, Weekly: 4, Monthly: 12, Yearly: 0}

// retentionPolicyDescription renders a policy as one human-readable line.
func retentionPolicyDescription(p retentionPolicy) string {
	yearly := fmt.Sprintf("one per year for the last %d years", p.Yearly)
	if p.Yearly <= 0 {
		yearly = "one per year forever"
	}
	return fmt.Sprintf("   policy: one per day for %d days, one per week for %d weeks,\n"+
		"   one per month for %d months, %s.", p.Daily, p.Weekly, p.Monthly, yearly)
}

// selectRetained returns the full snapshot names selectPruneCandidates
// decided to keep, per defaultRetentionPolicy's rules. Entries are assumed to
// already be filtered to one "owned" naming family (e.g. filterBackupSnapshots
// or filterManagedSourceSnapshots) - retention policy never looks at
// snapshots it was not told belong to it.
func selectRetained(entries []snapshotEntry, now time.Time, policy retentionPolicy) map[string]bool {
	keep := map[string]bool{}
	if len(entries) == 0 {
		return keep
	}

	sorted := append([]snapshotEntry(nil), entries...)
	sortSnapshotsNewestFirst(sorted)
	keep[sorted[0].Name] = true // always keep the newest: the incremental base

	keepNewestPerBucket := func(windowStart time.Time, bucketKey func(time.Time) string) {
		best := map[string]snapshotEntry{}
		for _, e := range sorted {
			if e.Creation.Before(windowStart) {
				continue
			}
			key := bucketKey(e.Creation)
			if cur, ok := best[key]; !ok || e.Creation.After(cur.Creation) {
				best[key] = e
			}
		}
		for _, e := range best {
			keep[e.Name] = true
		}
	}

	if policy.Daily > 0 {
		keepNewestPerBucket(now.AddDate(0, 0, -policy.Daily), func(t time.Time) string {
			return t.Format("2006-01-02")
		})
	}
	if policy.Weekly > 0 {
		keepNewestPerBucket(now.AddDate(0, 0, -policy.Weekly*7), func(t time.Time) string {
			year, week := t.ISOWeek()
			return fmt.Sprintf("%d-W%02d", year, week)
		})
	}
	if policy.Monthly > 0 {
		keepNewestPerBucket(now.AddDate(0, -policy.Monthly, 0), func(t time.Time) string {
			return t.Format("2006-01")
		})
	}
	// Yearly: a zero windowStart (year 1) is always in the past, so a
	// Yearly of 0 naturally means "forever" without a special case.
	var yearlyWindowStart time.Time
	if policy.Yearly > 0 {
		yearlyWindowStart = now.AddDate(-policy.Yearly, 0, 0)
	}
	keepNewestPerBucket(yearlyWindowStart, func(t time.Time) string {
		return t.Format("2006")
	})

	return keep
}

// selectPruneCandidates returns the entries selectRetained did not keep - the
// ones to convert to bookmarks and destroy.
func selectPruneCandidates(entries []snapshotEntry, now time.Time, policy retentionPolicy) []snapshotEntry {
	keep := selectRetained(entries, now, policy)
	var prune []snapshotEntry
	for _, e := range entries {
		if !keep[e.Name] {
			prune = append(prune, e)
		}
	}
	return prune
}

// =============================================================================
// Pruning
// =============================================================================

// bookmarkAndDestroy converts a snapshot to a bookmark of the same name and
// then destroys the snapshot. The destroy only happens once the bookmark is
// confirmed to exist, so a failed bookmark can never cost us the incremental
// base.
func bookmarkAndDestroy(ctx context.Context, r commandRunner, snapshot string) error {
	dataset, tag, ok := splitSnapshot(snapshot)
	if !ok {
		return fmt.Errorf("not a snapshot name: %s", snapshot)
	}
	if isProtectedSnapshotTag(tag) {
		return fmt.Errorf("refusing to touch protected snapshot %s", snapshot)
	}

	bookmark := dataset + "#" + tag
	// A bookmark may already exist from a previous run; that is fine, so the
	// error is only fatal if the bookmark is still absent afterwards.
	bookmarkErr := r.Run(ctx, "zfs", "bookmark", snapshot, bookmark)
	if _, err := r.Output(ctx, "zfs", "list", "-H", "-o", "name", "-t", "bookmark", bookmark); err != nil {
		if bookmarkErr != nil {
			return fmt.Errorf("failed to bookmark %s: %w", snapshot, bookmarkErr)
		}
		return fmt.Errorf("bookmark %s missing after creation", bookmark)
	}

	if err := r.Run(ctx, "zfs", "destroy", snapshot); err != nil {
		return fmt.Errorf("failed to destroy %s: %w", snapshot, err)
	}
	return nil
}

// pruneResult summarises one prune pass.
type pruneResult struct {
	Pruned   []string
	Warnings []string
}

// pruneLocalSnapshots converts old zfs-backup snapshots on the source pool to
// bookmarks, one dataset at a time over the canonical dataset list, keeping
// what policy's grandfather-father-son schedule calls for. Unlike the pre-2.0
// implementation it covers every dataset it snapshots rather than only
// POOL/home, which is what allowed snapshots to pile up elsewhere.
func pruneLocalSnapshots(ctx context.Context, r commandRunner, pool string, datasets []string, policy retentionPolicy, now time.Time) pruneResult {
	var result pruneResult

	for _, ds := range datasets {
		fullDS := fmt.Sprintf("%s/%s", pool, ds)
		entries, err := listSnapshotEntries(ctx, r, fullDS, 1)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: could not list snapshots: %v", fullDS, err))
			continue
		}

		own := filterBackupSnapshots(entries)
		for _, entry := range selectPruneCandidates(own, now, policy) {
			if err := bookmarkAndDestroy(ctx, r, entry.Name); err != nil {
				result.Warnings = append(result.Warnings, err.Error())
				continue
			}
			result.Pruned = append(result.Pruned, entry.Name)
		}
	}

	return result
}

// pruneDestinationSnapshots prunes zfs-backup's own snapshots on the backup
// pool under policy's grandfather-father-son schedule. destinations are fully
// qualified dataset names on the backup pool.
func pruneDestinationSnapshots(ctx context.Context, r commandRunner, destinations []string, policy retentionPolicy, now time.Time) pruneResult {
	var result pruneResult

	for _, dest := range destinations {
		entries, err := listSnapshotEntries(ctx, r, dest, 1)
		if err != nil {
			// A destination that does not exist yet is not an error worth
			// shouting about - it simply has nothing to prune.
			continue
		}

		own := filterBackupSnapshots(entries)
		for _, entry := range selectPruneCandidates(own, now, policy) {
			if err := bookmarkAndDestroy(ctx, r, entry.Name); err != nil {
				result.Warnings = append(result.Warnings, err.Error())
				continue
			}
			result.Pruned = append(result.Pruned, entry.Name)
		}
	}

	return result
}
