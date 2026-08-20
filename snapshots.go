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
// Pruning
// =============================================================================

// localBackupSnapshotsKept is how many of zfs-backup's own snapshots stay on
// the source pool per dataset. Older ones become bookmarks.
const localBackupSnapshotsKept = 7

// selectSnapshotsToPrune returns the snapshots to convert to bookmarks and
// destroy, keeping the newest `keep` entries. Only zfs-backup's own snapshots
// are ever considered - sanoid autosnaps, syncoid sync-snapshots and anything
// the user made are left untouched.
func selectSnapshotsToPrune(entries []snapshotEntry, keep int) []snapshotEntry {
	own := filterBackupSnapshots(entries)
	sortSnapshotsNewestFirst(own)

	if keep < 1 {
		keep = 1 // never prune the newest: it is the incremental base
	}
	if len(own) <= keep {
		return nil
	}
	return own[keep:]
}

// selectDestinationSnapshotsToPrune returns the destination snapshots to prune,
// keeping monthly archives for the given months plus the newest snapshot,
// which is the base for the next incremental send.
func selectDestinationSnapshotsToPrune(entries []snapshotEntry, keepMonths []string) []snapshotEntry {
	own := filterBackupSnapshots(entries)
	sortSnapshotsNewestFirst(own)

	var prune []snapshotEntry
	for i, e := range own {
		if i == 0 {
			continue // always keep the newest - it is the incremental base
		}
		keep := false
		for _, month := range keepMonths {
			if strings.HasPrefix(e.Tag, month) {
				keep = true
				break
			}
		}
		if !keep {
			prune = append(prune, e)
		}
	}
	return prune
}

// recentMonths returns the year-month prefixes to retain on the backup pool.
func recentMonths(now time.Time, count int) []string {
	months := make([]string, 0, count)
	for i := 0; i < count; i++ {
		months = append(months, now.AddDate(0, -i, 0).Format("2006-01"))
	}
	return months
}

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
// bookmarks, one dataset at a time over the canonical dataset list. Unlike the
// pre-2.0 implementation it covers every dataset it snapshots rather than only
// POOL/home, which is what allowed snapshots to pile up elsewhere.
func pruneLocalSnapshots(ctx context.Context, r commandRunner, pool string, datasets []string, keep int) pruneResult {
	var result pruneResult

	for _, ds := range datasets {
		fullDS := fmt.Sprintf("%s/%s", pool, ds)
		entries, err := listSnapshotEntries(ctx, r, fullDS, 1)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: could not list snapshots: %v", fullDS, err))
			continue
		}

		for _, entry := range selectSnapshotsToPrune(entries, keep) {
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
// pool, keeping monthly archives for the last three months plus the newest
// snapshot of each dataset. destinations are fully qualified dataset names on
// the backup pool.
func pruneDestinationSnapshots(ctx context.Context, r commandRunner, destinations []string, now time.Time) pruneResult {
	var result pruneResult
	keepMonths := recentMonths(now, 3)

	for _, dest := range destinations {
		entries, err := listSnapshotEntries(ctx, r, dest, 1)
		if err != nil {
			// A destination that does not exist yet is not an error worth
			// shouting about - it simply has nothing to prune.
			continue
		}

		for _, entry := range selectDestinationSnapshotsToPrune(entries, keepMonths) {
			if err := bookmarkAndDestroy(ctx, r, entry.Name); err != nil {
				result.Warnings = append(result.Warnings, err.Error())
				continue
			}
			result.Pruned = append(result.Pruned, entry.Name)
		}
	}

	return result
}
