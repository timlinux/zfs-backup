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
// Backup snapshot browser - core logic
// =============================================================================
//
// The browser answers one question the doctor cannot: what is sitting on the
// BACKUP pool, and does each snapshot there still correspond to anything on
// the source pool? A backup snapshot whose source dataset is gone, or whose
// source dataset is no longer replicated, will never be pruned by any backup
// run - it is debris that only a human can decide to remove. This file
// classifies every snapshot on the destination so the TUI can show the user
// exactly which ones those are, and why.

// browseClass says what a destination snapshot means relative to the source.
type browseClass int

const (
	// browseSynced - the same tag exists on the source dataset (as a snapshot
	// or bookmark). Healthy replicated history.
	browseSynced browseClass = iota
	// browseBase - the newest tag shared with the source. This is the
	// incremental base for the next backup: destroying it forces a full
	// re-send, so it is flagged as protected, never as a candidate.
	browseBase
	// browseRetained - the source has pruned its copy but the destination
	// keeps it by design (backup retention holds more history than the
	// source). Normal, not a problem.
	browseRetained
	// browseOrphan - orphaned from the source: the source dataset is gone,
	// or is no longer in the backup scope, or this is stale syncoid debris.
	// Nothing will ever prune it. These are the deletion candidates.
	browseOrphan
	// browseRecent - a syncoid sync-snapshot young enough that it may belong
	// to a send still in flight. Left alone until it ages.
	browseRecent
	// browseProtected - never touched under any circumstance (e.g. @blank).
	browseProtected
	// browseForeign - not zfs-backup's naming pattern. Someone else made it;
	// it is shown for completeness and left strictly alone.
	browseForeign
	// browseRemoteNS - lives under another host's namespace on the backup
	// pool. Its source is on that machine, so it cannot be judged from here.
	browseRemoteNS
)

// browseSnapshot is one destination snapshot with its verdict.
type browseSnapshot struct {
	snapshotEntry
	Class  browseClass
	Reason string
}

// browseDataset is one dataset on the backup pool with its mapped source and
// classified snapshots.
type browseDataset struct {
	Name         string // full destination dataset, e.g. NIXBACKUP/abyss/home
	Source       string // mapped source dataset, "" when none applies
	SourceExists bool   // does the mapped source dataset still exist?
	InScope      bool   // is the mapped source in the configured backup scope?
	RemoteHost   string // non-empty when namespaced under another host
	Snapshots    []browseSnapshot
	OrphanCount  int   // snapshots classified browseOrphan
	OrphanBytes  int64 // bytes uniquely referenced by those orphans (a floor)
}

// backupBrowse is the whole classified picture of one backup pool.
type backupBrowse struct {
	SourcePool  string
	DestPool    string
	Hostname    string
	ScopeKnown  bool // false when the backup scope could not be resolved
	ScanTime    time.Time
	Datasets    []browseDataset
	OrphanCount int
	OrphanBytes int64
}

// orphanFromSource marks a destination snapshot with no source counterpart.
const orphanFromSource orphanKind = "orphaned from source"

// mapDestinationToSource maps a destination dataset back to the source
// dataset it mirrors, honouring both the hostname-namespaced layout
// (DESTPOOL/<hostname>/home) and the legacy flat layout (DESTPOOL/home).
//
// A first-level child that is neither this host's namespace nor a name found
// on the source pool is treated as another machine's namespace: its source of
// truth lives on that machine and cannot be judged from here.
func mapDestinationToSource(destDataset, destPool, sourcePool, hostname string, sourceDatasets map[string]bool) (source, remoteHost string) {
	if destDataset == destPool {
		return sourcePool, ""
	}
	rel := strings.TrimPrefix(destDataset, destPool+"/")
	parts := strings.SplitN(rel, "/", 2)

	if parts[0] == hostname {
		if len(parts) == 1 {
			return "", "" // the namespace container itself mirrors nothing
		}
		return sourcePool + "/" + parts[1], ""
	}

	// Legacy flat layout: NIXBACKUP/home mirrors NIXROOT/home. Recognised
	// when the first path element names a dataset that exists (or a top-level
	// child that exists) on the source pool.
	if sourceDatasets[sourcePool+"/"+rel] || sourceDatasets[sourcePool+"/"+parts[0]] {
		return sourcePool + "/" + rel, ""
	}

	return "", parts[0]
}

// tagSetsFromEntries indexes snapshot tags by dataset.
func tagSetsFromEntries(entries []snapshotEntry) map[string]map[string]bool {
	sets := map[string]map[string]bool{}
	for _, e := range entries {
		if sets[e.Dataset] == nil {
			sets[e.Dataset] = map[string]bool{}
		}
		sets[e.Dataset][e.Tag] = true
	}
	return sets
}

// parseBookmarkTags parses `zfs list -H -t bookmark -o name` output into a
// dataset -> tag set index. Unparseable lines are skipped.
func parseBookmarkTags(output string) map[string]map[string]bool {
	sets := map[string]map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		idx := strings.Index(line, "#")
		if idx <= 0 || idx == len(line)-1 {
			continue
		}
		ds, tag := line[:idx], line[idx+1:]
		if sets[ds] == nil {
			sets[ds] = map[string]bool{}
		}
		sets[ds][tag] = true
	}
	return sets
}

// listBookmarkTags lists every bookmark under a pool, indexed by dataset.
// Pools without bookmark support or without bookmarks yield an empty index
// rather than an error - a missing bookmark only makes classification more
// conservative, never destructive.
func listBookmarkTags(ctx context.Context, r commandRunner, pool string) map[string]map[string]bool {
	output, err := r.Output(ctx, "zfs", "list", "-H", "-t", "bookmark", "-o", "name", "-r", pool)
	if err != nil {
		return map[string]map[string]bool{}
	}
	return parseBookmarkTags(output)
}

// listDatasetNames lists every dataset under a pool, including the pool root.
func listDatasetNames(ctx context.Context, r commandRunner, pool string) ([]string, error) {
	output, err := r.Output(ctx, "zfs", "list", "-H", "-o", "name", "-r", pool)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// classifyDestinationSnapshots is the pure heart of the browser: given the
// destination's snapshots and the source's snapshot/bookmark tags, it decides
// what every destination snapshot means. No commands are run and nothing is
// ever modified - this function only forms opinions.
func classifyDestinationSnapshots(
	destDatasets []string,
	destEntries []snapshotEntry,
	destPool, sourcePool, hostname string,
	sourceTags map[string]map[string]bool,
	sourceDatasets map[string]bool,
	inScope map[string]bool,
	scopeKnown bool,
	now time.Time,
) []browseDataset {
	byDataset := map[string][]snapshotEntry{}
	// backupishRoots are the first-level children whose subtree actually
	// carries backup or syncoid snapshots. Only those may be presumed to be
	// another host's namespace; a first-level child with no such snapshots
	// is just an unrecognised dataset, and calling it "remote" would hide
	// real debris behind a reassuring label.
	backupishRoots := map[string]bool{}
	for _, e := range destEntries {
		byDataset[e.Dataset] = append(byDataset[e.Dataset], e)
		if isBackupSnapshotTag(e.Tag) || isSyncoidSnapshotTag(e.Tag) {
			if rel, ok := strings.CutPrefix(e.Dataset, destPool+"/"); ok {
				backupishRoots[strings.SplitN(rel, "/", 2)[0]] = true
			}
		}
	}

	// Every dataset that exists or carries snapshots is browsable.
	seen := map[string]bool{}
	var names []string
	for _, ds := range destDatasets {
		if !seen[ds] {
			seen[ds] = true
			names = append(names, ds)
		}
	}
	for ds := range byDataset {
		if !seen[ds] {
			seen[ds] = true
			names = append(names, ds)
		}
	}
	sort.Strings(names)

	var result []browseDataset
	for _, name := range names {
		source, remoteHost := mapDestinationToSource(name, destPool, sourcePool, hostname, sourceDatasets)
		if remoteHost != "" && !backupishRoots[remoteHost] {
			remoteHost = "" // unrecognised, not provably another host's backups
		}
		ds := browseDataset{
			Name:         name,
			Source:       source,
			SourceExists: source != "" && sourceDatasets[source],
			InScope:      source != "" && inScope[source],
			RemoteHost:   remoteHost,
		}

		entries := byDataset[name]
		sortSnapshotsNewestFirst(entries)

		// The incremental base is the newest destination snapshot whose tag
		// the source still has (as a snapshot or bookmark). Entries are
		// newest-first, so the first shared tag is the base.
		baseName := ""
		if ds.SourceExists {
			for _, e := range entries {
				if sourceTags[source][e.Tag] {
					baseName = e.Name
					break
				}
			}
		}

		for _, e := range entries {
			ds.Snapshots = append(ds.Snapshots, classifyOne(e, ds, baseName, sourceTags, scopeKnown, now))
		}

		for _, s := range ds.Snapshots {
			if s.Class == browseOrphan {
				ds.OrphanCount++
				if s.Used > 0 {
					ds.OrphanBytes += s.Used
				}
			}
		}
		result = append(result, ds)
	}
	return result
}

// classifyOne forms the verdict for a single destination snapshot.
func classifyOne(e snapshotEntry, ds browseDataset, baseName string, sourceTags map[string]map[string]bool, scopeKnown bool, now time.Time) browseSnapshot {
	verdict := func(class browseClass, reason string) browseSnapshot {
		return browseSnapshot{snapshotEntry: e, Class: class, Reason: reason}
	}

	if isProtectedSnapshotTag(e.Tag) {
		return verdict(browseProtected, "protected - never touched by zfs-backup")
	}
	if ds.RemoteHost != "" {
		return verdict(browseRemoteNS, fmt.Sprintf(
			"belongs to %s's backups - judge it on that machine", ds.RemoteHost))
	}

	sharedWithSource := ds.SourceExists && sourceTags[ds.Source][e.Tag]
	if sharedWithSource {
		if e.Name == baseName {
			return verdict(browseBase, fmt.Sprintf(
				"newest snapshot shared with %s - keep it, or the next backup has to re-send the whole dataset", ds.Source))
		}
		return verdict(browseSynced, fmt.Sprintf("also present on %s", ds.Source))
	}

	switch {
	case isBackupSnapshotTag(e.Tag):
		if ds.Source == "" || !ds.SourceExists {
			return verdict(browseOrphan,
				"no matching dataset on the source pool - nothing will ever prune this")
		}
		if scopeKnown && !ds.InScope {
			return verdict(browseOrphan, fmt.Sprintf(
				"%s is no longer in the backup scope - nothing will ever prune this", ds.Source))
		}
		return verdict(browseRetained, fmt.Sprintf(
			"pruned on %s, kept here by the backup retention policy", ds.Source))
	case isSyncoidSnapshotTag(e.Tag):
		if !e.Creation.IsZero() && now.Sub(e.Creation) < minSyncoidOrphanAge {
			return verdict(browseRecent, "young syncoid sync-snapshot - a send may still be in flight")
		}
		return verdict(browseOrphan, "stale syncoid sync-snapshot with no source counterpart - debris from a failed send")
	default:
		return verdict(browseForeign, "not created by zfs-backup - left strictly alone")
	}
}

// collectBackupBrowse gathers and classifies everything the browser shows.
// Read-only: it lists snapshots, bookmarks and datasets, and forms opinions.
func collectBackupBrowse(ctx context.Context, r commandRunner, sourcePool, destPool, hostname string, inScopeChildren []string, scopeKnown bool) (*backupBrowse, error) {
	destEntries, err := listSnapshotEntries(ctx, r, destPool, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots on %s: %w", destPool, err)
	}
	destDatasets, err := listDatasetNames(ctx, r, destPool)
	if err != nil {
		return nil, fmt.Errorf("failed to list datasets on %s: %w", destPool, err)
	}
	sourceEntries, err := listSnapshotEntries(ctx, r, sourcePool, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots on %s: %w", sourcePool, err)
	}
	sourceNames, err := listDatasetNames(ctx, r, sourcePool)
	if err != nil {
		return nil, fmt.Errorf("failed to list datasets on %s: %w", sourcePool, err)
	}

	sourceTags := tagSetsFromEntries(sourceEntries)
	for ds, tags := range listBookmarkTags(ctx, r, sourcePool) {
		if sourceTags[ds] == nil {
			sourceTags[ds] = map[string]bool{}
		}
		for tag := range tags {
			sourceTags[ds][tag] = true
		}
	}

	sourceDatasets := map[string]bool{}
	for _, name := range sourceNames {
		sourceDatasets[name] = true
	}
	inScope := map[string]bool{}
	for _, child := range inScopeChildren {
		inScope[sourcePool+"/"+child] = true
	}

	browse := &backupBrowse{
		SourcePool: sourcePool,
		DestPool:   destPool,
		Hostname:   hostname,
		ScopeKnown: scopeKnown,
		ScanTime:   time.Now(),
		Datasets: classifyDestinationSnapshots(
			destDatasets, destEntries, destPool, sourcePool, hostname,
			sourceTags, sourceDatasets, inScope, scopeKnown, time.Now()),
	}
	for _, ds := range browse.Datasets {
		browse.OrphanCount += ds.OrphanCount
		browse.OrphanBytes += ds.OrphanBytes
	}
	return browse, nil
}

// =============================================================================
// Destination cleanup plan
// =============================================================================

// destCleanupPlan is the vetted answer to "what would destroying the browser's
// candidates actually remove?". Building it performs no destructive work.
type destCleanupPlan struct {
	DestPool  string
	Decisions []destroyDecision // every candidate, safe or held back
	Targets   []string          // the subset cleared for destruction
}

// buildDestCleanupPlan wraps the browser's orphan candidates in the same
// vetting the source-side cleanup uses - holds, clones and protected tags are
// re-checked against live ZFS state - so a snapshot the doctor would refuse
// to touch is equally untouchable from the browser.
func buildDestCleanupPlan(ctx context.Context, r commandRunner, browse *backupBrowse) *destCleanupPlan {
	var candidates []orphanSnapshot
	for _, ds := range browse.Datasets {
		for _, s := range ds.Snapshots {
			if s.Class == browseOrphan {
				candidates = append(candidates, orphanSnapshot{
					snapshotEntry: s.snapshotEntry,
					Kind:          orphanFromSource,
					Reason:        s.Reason,
				})
			}
		}
	}
	decisions := vetOrphans(ctx, r, candidates)
	return &destCleanupPlan{
		DestPool:  browse.DestPool,
		Decisions: decisions,
		Targets:   safeToDestroy(decisions),
	}
}

// uniqueBytes totals the space uniquely held by the cleared targets. A floor,
// not a promise: blocks shared with surviving snapshots count against neither.
func (p *destCleanupPlan) uniqueBytes() int64 {
	var total int64
	for _, d := range p.Decisions {
		if d.Safe && d.Orphan.Used > 0 {
			total += d.Orphan.Used
		}
	}
	return total
}
