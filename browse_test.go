// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Destination -> source mapping
// =============================================================================

func TestMapDestinationToSource(t *testing.T) {
	sourceDatasets := map[string]bool{
		"NIXROOT":      true,
		"NIXROOT/home": true,
		"NIXROOT/nix":  true,
	}

	cases := []struct {
		name       string
		dest       string
		wantSource string
		wantRemote string
	}{
		{"hostname namespace", "NIXBACKUP/abyss/home", "NIXROOT/home", ""},
		{"hostname namespace, gone on source", "NIXBACKUP/abyss/overflow", "NIXROOT/overflow", ""},
		{"namespace container itself", "NIXBACKUP/abyss", "", ""},
		{"legacy flat layout", "NIXBACKUP/home", "NIXROOT/home", ""},
		{"legacy flat nested", "NIXBACKUP/home/sub", "NIXROOT/home/sub", ""},
		{"another host's namespace", "NIXBACKUP/otherbox/home", "", "otherbox"},
		{"pool root", "NIXBACKUP", "NIXROOT", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, remote := mapDestinationToSource(tc.dest, "NIXBACKUP", "NIXROOT", "abyss", sourceDatasets)
			if source != tc.wantSource || remote != tc.wantRemote {
				t.Fatalf("mapDestinationToSource(%s) = (%q, %q), want (%q, %q)",
					tc.dest, source, remote, tc.wantSource, tc.wantRemote)
			}
		})
	}
}

// =============================================================================
// Bookmark parsing
// =============================================================================

func TestParseBookmarkTags(t *testing.T) {
	output := strings.Join([]string{
		"NIXROOT/home#2026-05-17.22h-55-Backup",
		"NIXROOT/home#autosnap_2026-07-22_22:00:00_hourly",
		"garbage-without-hash",
		"",
	}, "\n")

	tags := parseBookmarkTags(output)
	if !tags["NIXROOT/home"]["2026-05-17.22h-55-Backup"] {
		t.Error("expected the -Backup bookmark tag to be indexed")
	}
	if !tags["NIXROOT/home"]["autosnap_2026-07-22_22:00:00_hourly"] {
		t.Error("expected the autosnap bookmark tag to be indexed")
	}
	if len(tags) != 1 {
		t.Errorf("expected exactly one dataset in the index, got %d", len(tags))
	}
}

// =============================================================================
// Classification
// =============================================================================

// browseFixtureNow is a fixed reference time so verdicts are deterministic.
var browseFixtureNow = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func snap(name string, age time.Duration, used int64) snapshotEntry {
	ds, tag, _ := splitSnapshot(name)
	return snapshotEntry{
		Name:     name,
		Dataset:  ds,
		Tag:      tag,
		Creation: browseFixtureNow.Add(-age),
		Used:     used,
	}
}

// classFor digs the verdict for one snapshot out of a classified result.
func classFor(t *testing.T, datasets []browseDataset, name string) browseSnapshot {
	t.Helper()
	for _, ds := range datasets {
		for _, s := range ds.Snapshots {
			if s.Name == name {
				return s
			}
		}
	}
	t.Fatalf("snapshot %s not found in classification", name)
	return browseSnapshot{}
}

func classifyFixture(t *testing.T, scopeKnown bool) []browseDataset {
	t.Helper()
	destDatasets := []string{
		"NIXBACKUP",
		"NIXBACKUP/abyss",
		"NIXBACKUP/abyss/home",
		"NIXBACKUP/abyss/overflow",
		"NIXBACKUP/abyss/root",
		"NIXBACKUP/otherbox/home",
		"NIXBACKUP/empty",
	}
	destEntries := []snapshotEntry{
		// home: base + older synced + retained history + a user snapshot
		snap("NIXBACKUP/abyss/home@2026-09-09.22h-00-Backup", 14*time.Hour, 100),
		snap("NIXBACKUP/abyss/home@2026-09-01.22h-00-Backup", 9*24*time.Hour, 200),
		snap("NIXBACKUP/abyss/home@2026-06-01.22h-00-Backup", 101*24*time.Hour, 300),
		snap("NIXBACKUP/abyss/home@my-manual-snapshot", 5*24*time.Hour, 10),
		// overflow: source dataset no longer exists - all orphans
		snap("NIXBACKUP/abyss/overflow@2026-05-17.22h-55-Backup", 116*24*time.Hour, 400),
		snap("NIXBACKUP/abyss/overflow@2026-05-19.00h-24-Backup", 114*24*time.Hour, 500),
		// root: exists on source but out of scope, plus stale + young syncoid
		snap("NIXBACKUP/abyss/root@2026-05-17.22h-55-Backup", 116*24*time.Hour, 600),
		snap("NIXBACKUP/abyss/root@syncoid_abyss_2026-05-19:00:49:53-GMT01:00", 114*24*time.Hour, 50),
		snap("NIXBACKUP/abyss/root@syncoid_abyss_2026-09-10:11:30:00-GMT01:00", 30*time.Minute, 5),
		snap("NIXBACKUP/abyss/root@blank", 200*24*time.Hour, 1),
		// another machine's namespace - never judged from here
		snap("NIXBACKUP/otherbox/home@2026-01-01.00h-00-Backup", 250*24*time.Hour, 700),
	}
	sourceTags := map[string]map[string]bool{
		"NIXROOT/home": {
			"2026-09-09.22h-00-Backup": true, // snapshot still on source
			"2026-09-01.22h-00-Backup": true, // bookmark on source
		},
	}
	sourceDatasets := map[string]bool{
		"NIXROOT":      true,
		"NIXROOT/home": true,
		"NIXROOT/root": true,
	}
	inScope := map[string]bool{"NIXROOT/home": true}

	return classifyDestinationSnapshots(
		destDatasets, destEntries, "NIXBACKUP", "NIXROOT", "abyss",
		sourceTags, sourceDatasets, inScope, scopeKnown, browseFixtureNow)
}

func TestClassifyDestinationSnapshots(t *testing.T) {
	datasets := classifyFixture(t, true)

	expect := map[string]browseClass{
		"NIXBACKUP/abyss/home@2026-09-09.22h-00-Backup":                   browseBase,
		"NIXBACKUP/abyss/home@2026-09-01.22h-00-Backup":                   browseSynced,
		"NIXBACKUP/abyss/home@2026-06-01.22h-00-Backup":                   browseRetained,
		"NIXBACKUP/abyss/home@my-manual-snapshot":                         browseForeign,
		"NIXBACKUP/abyss/overflow@2026-05-17.22h-55-Backup":               browseOrphan,
		"NIXBACKUP/abyss/overflow@2026-05-19.00h-24-Backup":               browseOrphan,
		"NIXBACKUP/abyss/root@2026-05-17.22h-55-Backup":                   browseOrphan,
		"NIXBACKUP/abyss/root@syncoid_abyss_2026-05-19:00:49:53-GMT01:00": browseOrphan,
		"NIXBACKUP/abyss/root@syncoid_abyss_2026-09-10:11:30:00-GMT01:00": browseRecent,
		"NIXBACKUP/abyss/root@blank":                                      browseProtected,
		"NIXBACKUP/otherbox/home@2026-01-01.00h-00-Backup":                browseRemoteNS,
	}
	for name, wantClass := range expect {
		got := classFor(t, datasets, name)
		if got.Class != wantClass {
			t.Errorf("%s: class = %v, want %v (reason: %s)", name, got.Class, wantClass, got.Reason)
		}
	}
}

func TestClassifyOrphanTotals(t *testing.T) {
	datasets := classifyFixture(t, true)

	var orphans int
	var bytes int64
	for _, ds := range datasets {
		orphans += ds.OrphanCount
		bytes += ds.OrphanBytes
	}
	// overflow: 400+500, root: 600 + stale syncoid 50
	if orphans != 4 {
		t.Errorf("orphan count = %d, want 4", orphans)
	}
	if bytes != 1550 {
		t.Errorf("orphan bytes = %d, want 1550", bytes)
	}
}

func TestClassifyUnknownScopeIsConservative(t *testing.T) {
	datasets := classifyFixture(t, false)

	// With the scope unreadable, an out-of-scope verdict cannot be formed:
	// root's -Backup snapshot must degrade to retained, not orphan. The
	// missing-dataset and stale-syncoid verdicts do not depend on scope.
	got := classFor(t, datasets, "NIXBACKUP/abyss/root@2026-05-17.22h-55-Backup")
	if got.Class != browseRetained {
		t.Errorf("unknown scope: root backup snapshot = %v, want browseRetained", got.Class)
	}
	still := classFor(t, datasets, "NIXBACKUP/abyss/overflow@2026-05-17.22h-55-Backup")
	if still.Class != browseOrphan {
		t.Errorf("unknown scope: missing-source snapshot = %v, want browseOrphan", still.Class)
	}
}

func TestClassifyEmptyDatasetIsListed(t *testing.T) {
	datasets := classifyFixture(t, true)
	for _, ds := range datasets {
		if ds.Name == "NIXBACKUP/empty" {
			if len(ds.Snapshots) != 0 {
				t.Errorf("empty dataset should list no snapshots, got %d", len(ds.Snapshots))
			}
			return
		}
	}
	t.Error("dataset with no snapshots should still be browsable")
}

// The incremental base must never be classified a candidate, even when it is
// the only snapshot shared with the source - destroying it would force a full
// re-send of the dataset.
func TestBaseIsNeverACandidate(t *testing.T) {
	datasets := classifyFixture(t, true)
	base := classFor(t, datasets, "NIXBACKUP/abyss/home@2026-09-09.22h-00-Backup")
	if base.Class == browseOrphan {
		t.Fatal("the incremental base was classified as a deletion candidate")
	}
}

// =============================================================================
// Orphan navigation
// =============================================================================

func TestBrowseJumpToNextOrphan(t *testing.T) {
	datasets := classifyFixture(t, true)

	d, s, ordinal, found := browseJumpToNextOrphan(datasets, 0, 0, false)
	if !found {
		t.Fatal("expected to find an orphan")
	}
	first := datasets[d].Snapshots[s]
	if first.Class != browseOrphan {
		t.Fatalf("jump landed on %s (%v), not an orphan", first.Name, first.Class)
	}
	if ordinal != 1 {
		t.Errorf("first jump should report candidate 1, got %d", ordinal)
	}

	// Jumping repeatedly must visit every orphan exactly once before wrapping.
	visited := map[string]bool{first.Name: true}
	for range 3 {
		d, s, _, found = browseJumpToNextOrphan(datasets, d, s, true)
		if !found {
			t.Fatal("expected wrap-around to keep finding orphans")
		}
		visited[datasets[d].Snapshots[s].Name] = true
	}
	if len(visited) != 4 {
		t.Errorf("visited %d distinct orphans, want 4", len(visited))
	}
}

// With the cursor parked on a dataset in the left pane, o must land on that
// dataset's own first candidate - not skip past it and only come back after
// wrapping the whole pool.
func TestBrowseJumpFindsTheCandidateUnderTheCursor(t *testing.T) {
	datasets := classifyFixture(t, true)
	overflowIdx := -1
	for i, ds := range datasets {
		if ds.Name == "NIXBACKUP/abyss/overflow" {
			overflowIdx = i
		}
	}
	if overflowIdx < 0 {
		t.Fatal("fixture is missing the overflow dataset")
	}

	d, s, _, found := browseJumpToNextOrphan(datasets, overflowIdx, 0, false)
	if !found {
		t.Fatal("expected to find an orphan")
	}
	if d != overflowIdx || s != 0 {
		t.Errorf("jump from the dataset pane landed on (%d,%d), want overflow's first snapshot (%d,0)",
			d, s, overflowIdx)
	}
}

// A pool holding only snapshot-less datasets must answer "none found", not
// spin - this exact shape used to hang the first implementation.
func TestBrowseJumpToNextOrphanAllDatasetsEmpty(t *testing.T) {
	datasets := []browseDataset{
		{Name: "NIXBACKUP/a"},
		{Name: "NIXBACKUP/b"},
	}
	if _, _, _, found := browseJumpToNextOrphan(datasets, 0, 0, false); found {
		t.Error("empty datasets hold no orphans")
	}
}

func TestBrowseJumpToNextOrphanNoneFound(t *testing.T) {
	datasets := []browseDataset{{
		Name:      "NIXBACKUP/home",
		Snapshots: []browseSnapshot{{snapshotEntry: snap("NIXBACKUP/home@x-Backup", time.Hour, 1), Class: browseSynced}},
	}}
	if _, _, _, found := browseJumpToNextOrphan(datasets, 0, 0, true); found {
		t.Error("no orphans exist, so none should be found")
	}
}

// =============================================================================
// End-to-end collection with a fake runner
// =============================================================================

// browseFakeResponder serves the canned pool state for collect tests.
func browseFakeResponder(name string, args []string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	switch {
	case strings.Contains(line, "-t snapshot") && strings.Contains(line, "NIXBACKUP"):
		return strings.Join([]string{
			"NIXBACKUP/abyss/home@2026-09-09.22h-00-Backup\t1757455200\t100",
			"NIXBACKUP/abyss/overflow@2026-05-17.22h-55-Backup\t1747518900\t400",
		}, "\n"), nil
	case strings.Contains(line, "-t snapshot") && strings.Contains(line, "NIXROOT"):
		return "NIXROOT/home@2026-09-09.22h-00-Backup\t1757455200\t100", nil
	case strings.Contains(line, "-t bookmark"):
		return "NIXROOT/home#2026-09-01.22h-00-Backup", nil
	case strings.Contains(line, "zfs list -H -o name -r NIXBACKUP"):
		return "NIXBACKUP\nNIXBACKUP/abyss\nNIXBACKUP/abyss/home\nNIXBACKUP/abyss/overflow", nil
	case strings.Contains(line, "zfs list -H -o name -r NIXROOT"):
		return "NIXROOT\nNIXROOT/home", nil
	}
	return "", nil
}

func TestCollectBackupBrowseIsReadOnly(t *testing.T) {
	runner := &fakeRunner{respond: browseFakeResponder}

	browse, err := collectBackupBrowse(context.Background(), runner,
		"NIXROOT", "NIXBACKUP", "abyss", []string{"home"}, true)
	if err != nil {
		t.Fatalf("collectBackupBrowse: %v", err)
	}

	for _, call := range runner.commandLines() {
		for _, verb := range []string{"zfs destroy", "zfs snapshot", "zfs rollback",
			"zfs receive", "zfs create", "zfs set", "zfs bookmark"} {
			if strings.HasPrefix(call, verb) {
				t.Errorf("browsing must be read-only, but ran: %s", call)
			}
		}
	}

	if browse.OrphanCount != 1 {
		t.Fatalf("orphan count = %d, want 1 (the overflow snapshot)", browse.OrphanCount)
	}
	orphan := classFor(t, browse.Datasets, "NIXBACKUP/abyss/overflow@2026-05-17.22h-55-Backup")
	if orphan.Class != browseOrphan {
		t.Errorf("overflow snapshot = %v, want browseOrphan", orphan.Class)
	}
	base := classFor(t, browse.Datasets, "NIXBACKUP/abyss/home@2026-09-09.22h-00-Backup")
	if base.Class != browseBase {
		t.Errorf("home snapshot = %v, want browseBase (tag still on source)", base.Class)
	}
}

// =============================================================================
// Destination cleanup plan
// =============================================================================

func TestBuildDestCleanupPlanVetsCandidates(t *testing.T) {
	browse := &backupBrowse{
		DestPool: "NIXBACKUP",
		Datasets: []browseDataset{{
			Name: "NIXBACKUP/abyss/overflow",
			Snapshots: []browseSnapshot{
				{snapshotEntry: snap("NIXBACKUP/abyss/overflow@2026-05-17.22h-55-Backup", time.Hour, 400), Class: browseOrphan, Reason: "no source"},
				{snapshotEntry: snap("NIXBACKUP/abyss/overflow@2026-05-19.00h-24-Backup", time.Hour, 500), Class: browseOrphan, Reason: "no source"},
				{snapshotEntry: snap("NIXBACKUP/abyss/overflow@keep-me", time.Hour, 5), Class: browseForeign, Reason: "user snapshot"},
			},
		}},
	}

	// The first candidate carries a hold; it must be held back.
	runner := &fakeRunner{respond: func(name string, args []string) (string, error) {
		line := name + " " + strings.Join(args, " ")
		if strings.Contains(line, "holds") && strings.Contains(line, "2026-05-17") {
			return "NIXBACKUP/abyss/overflow@2026-05-17.22h-55-Backup\tkeepme\tThu Sep 10 12:00 2026", nil
		}
		if strings.Contains(line, "clones") {
			return "-", nil
		}
		return "", nil
	}}

	plan := buildDestCleanupPlan(context.Background(), runner, browse)

	if len(plan.Decisions) != 2 {
		t.Fatalf("decisions = %d, want 2 (only orphan candidates are considered)", len(plan.Decisions))
	}
	if len(plan.Targets) != 1 || plan.Targets[0] != "NIXBACKUP/abyss/overflow@2026-05-19.00h-24-Backup" {
		t.Fatalf("targets = %v, want only the un-held snapshot", plan.Targets)
	}
	if plan.uniqueBytes() != 500 {
		t.Errorf("uniqueBytes = %d, want 500", plan.uniqueBytes())
	}
	for _, target := range plan.Targets {
		if strings.Contains(target, "keep-me") {
			t.Error("a foreign snapshot leaked into the destroy targets")
		}
	}
}
