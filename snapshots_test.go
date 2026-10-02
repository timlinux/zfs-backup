// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestIsBackupSnapshotTag(t *testing.T) {
	cases := map[string]bool{
		"2026-08-13.23h-47-Backup":                true,
		"2026-05-17.22h-55-Backup":                true,
		"autosnap_2026-07-22_22:00:00_hourly":     false,
		"syncoid_abyss_2026-05-19:00:49:53-GMT01": false,
		"blank":                           false,
		"2026-08-13.23h-47-Backup-manual": false,
		"my-2026-08-13.23h-47-Backup":     false,
	}

	for tag, want := range cases {
		if got := isBackupSnapshotTag(tag); got != want {
			t.Errorf("isBackupSnapshotTag(%q) = %v, want %v", tag, got, want)
		}
	}
}

func TestSnapshotTagRoundTrip(t *testing.T) {
	tag := snapshotTagForTime(time.Date(2026, 8, 13, 23, 47, 0, 0, time.UTC))

	if tag != "2026-08-13.23h-47-Backup" {
		t.Fatalf("unexpected tag %q", tag)
	}
	if !isBackupSnapshotTag(tag) {
		t.Errorf("a tag we generate must be recognised as ours: %q", tag)
	}
}

func TestProtectedSnapshotIsNeverOurs(t *testing.T) {
	// NIXROOT/root@blank is rolled back to on every boot on "erase your
	// darlings" installs. Destroying it breaks the system.
	if !isProtectedSnapshotTag("blank") {
		t.Error("@blank must be protected")
	}
	if isBackupSnapshotTag("blank") {
		t.Error("@blank must never be treated as a zfs-backup snapshot")
	}
}

func TestParseSnapshotEntries(t *testing.T) {
	output := strings.Join([]string{
		"NIXROOT/home@2026-08-13.23h-47-Backup\t1755123456\t187433984",
		"NIXROOT/root@blank\t1745123456\t172032",
		"malformed-line-without-at-sign\t1745123456\t0",
		"",
	}, "\n")

	entries := parseSnapshotEntries(output)

	if len(entries) != 2 {
		t.Fatalf("expected 2 parsed entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Dataset != "NIXROOT/home" || entries[0].Tag != "2026-08-13.23h-47-Backup" {
		t.Errorf("unexpected first entry: %+v", entries[0])
	}
	if entries[0].Used != 187433984 {
		t.Errorf("expected used bytes to be parsed, got %d", entries[0].Used)
	}
	if entries[0].Creation.Unix() != 1755123456 {
		t.Errorf("expected creation time to be parsed, got %v", entries[0].Creation)
	}
}

// retentionReferenceTime is the fixed "now" every retention test measures
// against, so results never depend on when the suite happens to run.
var retentionReferenceTime = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

// snapshotFixture builds a snapshot entry n days before retentionReferenceTime.
func snapshotFixture(dataset, tag string, daysAgo int) snapshotEntry {
	return snapshotEntry{
		Name:     dataset + "@" + tag,
		Dataset:  dataset,
		Tag:      tag,
		Creation: retentionReferenceTime.AddDate(0, 0, -daysAgo),
		Used:     1024,
	}
}

// snapshotFixtureAt builds a snapshot entry a given duration before
// retentionReferenceTime, for tests that need same-day granularity.
func snapshotFixtureAt(dataset, tag string, before time.Duration) snapshotEntry {
	return snapshotEntry{
		Name:     dataset + "@" + tag,
		Dataset:  dataset,
		Tag:      tag,
		Creation: retentionReferenceTime.Add(-before),
		Used:     1024,
	}
}

func TestSelectPruneCandidatesIgnoresForeignSnapshots(t *testing.T) {
	all := []snapshotEntry{
		snapshotFixture("NIXROOT/home", "2026-08-14.10h-00-Backup", 0),
		snapshotFixture("NIXROOT/home", "autosnap_2026-08-11_22:00:00_hourly", 3),
		snapshotFixture("NIXROOT/home", "syncoid_abyss_2026-08-10:00:49:53-GMT01:00", 4),
		snapshotFixture("NIXROOT/home", "keep-me-please", 5),
	}

	// filterBackupSnapshots - what pruneLocalSnapshots/pruneDestinationSnapshots
	// actually call before handing entries to the retention policy - must
	// strip everything but zfs-backup's own tag, so retention never even sees
	// a foreign snapshot, let alone destroys one.
	own := filterBackupSnapshots(all)
	if len(own) != 1 || own[0].Tag != "2026-08-14.10h-00-Backup" {
		t.Fatalf("expected only the zfs-backup snapshot to survive filtering, got %+v", own)
	}
}

// TestSelectPruneCandidatesAppliesEveryRetentionTier constructs one snapshot
// per grandfather-father-son tier (plus a same-day loser and a same-year
// loser) and checks selectPruneCandidates keeps exactly one representative
// per tier and prunes the rest.
func TestSelectPruneCandidatesAppliesEveryRetentionTier(t *testing.T) {
	entries := []snapshotEntry{
		// The newest snapshot overall - also today's daily representative.
		snapshotFixtureAt("NIXROOT/home", "daily-newest", 0),
		// Same calendar day as the above, but older - must lose to it.
		snapshotFixtureAt("NIXROOT/home", "daily-loser", 6*time.Hour),
		// 10 days ago: outside the 7-day daily window, inside the 4-week
		// weekly window, and alone in its ISO week.
		snapshotFixture("NIXROOT/home", "weekly-rep", 10),
		// 40 days ago: outside daily and weekly windows, inside the 12-month
		// monthly window, alone in its month.
		snapshotFixture("NIXROOT/home", "monthly-rep", 40),
		// ~400 days ago: outside the monthly window entirely, kept as the
		// newest entry in its calendar year (the yearly-forever tier).
		snapshotFixture("NIXROOT/home", "yearly-rep", 400),
		// ~500 days ago: the same calendar year as yearly-rep, but older -
		// must lose to it in the yearly bucket.
		snapshotFixture("NIXROOT/home", "yearly-loser", 500),
	}

	prune := selectPruneCandidates(entries, retentionReferenceTime, defaultRetentionPolicy)

	var pruned []string
	for _, e := range prune {
		pruned = append(pruned, e.Tag)
	}
	sort.Strings(pruned)
	want := []string{"daily-loser", "yearly-loser"}
	if !reflect.DeepEqual(pruned, want) {
		t.Errorf("pruned %v, want %v", pruned, want)
	}
}

func TestSelectPruneCandidatesNeverPrunesTheOnlyBase(t *testing.T) {
	entries := []snapshotEntry{
		snapshotFixture("NIXROOT/home", "2020-01-01.00h-00-Backup", 3000),
	}

	// A policy of all zeros would be a caller bug; the newest snapshot is
	// the incremental base and must survive regardless.
	if prune := selectPruneCandidates(entries, retentionReferenceTime, retentionPolicy{}); len(prune) != 0 {
		t.Errorf("expected nothing to be pruned, got %+v", prune)
	}
}

// TestCreateDatasetSnapshotsIsNeverRecursive is the regression test for the
// 1.6.0 bug: snapshots must be taken per dataset, never with -r over the pool.
func TestCreateDatasetSnapshotsIsNeverRecursive(t *testing.T) {
	runner := &fakeRunner{}

	created, err := createDatasetSnapshots(context.Background(), runner, "NIXROOT",
		[]string{"home"}, "2026-08-14.10h-00-Backup")
	if err != nil {
		t.Fatalf("createDatasetSnapshots: %v", err)
	}

	want := []string{"NIXROOT/home@2026-08-14.10h-00-Backup"}
	if !reflect.DeepEqual(created, want) {
		t.Errorf("created %v, want %v", created, want)
	}
	for _, line := range runner.commandLines() {
		if strings.Contains(line, "snapshot -r") {
			t.Errorf("recursive snapshot must never be used: %q", line)
		}
	}
	// The pool root dataset itself must not be snapshotted either.
	if runner.ran("zfs snapshot NIXROOT@") {
		t.Error("the pool root dataset must never be snapshotted")
	}
}

// TestCreateDatasetSnapshotsSkipsDatasetsOutOfScope is the other half of the
// invariant: a dataset that is not in the canonical list is never touched.
func TestCreateDatasetSnapshotsSkipsDatasetsOutOfScope(t *testing.T) {
	runner := &fakeRunner{}
	// The pool has home, root, nix and atuin; only home is in scope.
	inScope := []string{"home"}

	if _, err := createDatasetSnapshots(context.Background(), runner, "NIXROOT",
		inScope, "2026-08-14.10h-00-Backup"); err != nil {
		t.Fatalf("createDatasetSnapshots: %v", err)
	}

	for _, outOfScope := range []string{"NIXROOT/root", "NIXROOT/nix", "NIXROOT/atuin"} {
		if runner.mentions(outOfScope) {
			t.Errorf("%s is not in scope and must never be snapshotted", outOfScope)
		}
	}
}

func TestCreateDatasetSnapshotsRollsBackOnFailure(t *testing.T) {
	runner := &fakeRunner{
		respond: func(_ string, args []string) (string, error) {
			if len(args) > 1 && args[0] == "snapshot" && strings.HasPrefix(args[1], "NIXROOT/nix@") {
				return "", fmt.Errorf("dataset is busy")
			}
			return "", nil
		},
	}

	created, err := createDatasetSnapshots(context.Background(), runner, "NIXROOT",
		[]string{"home", "nix"}, "2026-08-14.10h-00-Backup")

	if err == nil {
		t.Fatal("expected an error when a snapshot fails")
	}
	if created != nil {
		t.Errorf("expected no snapshots to be reported as created, got %v", created)
	}
	if !runner.ran("zfs destroy NIXROOT/home@2026-08-14.10h-00-Backup") {
		t.Error("a failed run must destroy the snapshots it already created")
	}
}

func TestCreateDatasetSnapshotsRefusesEmptyScope(t *testing.T) {
	runner := &fakeRunner{}

	if _, err := createDatasetSnapshots(context.Background(), runner, "NIXROOT", nil, "tag"); err == nil {
		t.Error("expected an error when no datasets are in scope")
	}
	if len(runner.calls) != 0 {
		t.Errorf("no commands should run for an empty scope, got %v", runner.commandLines())
	}
}

// pruneFixtureRunner answers snapshot listings for the given datasets.
func pruneFixtureRunner(listings map[string]string) *fakeRunner {
	return &fakeRunner{
		respond: func(_ string, args []string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			if args[0] == "list" {
				target := args[len(args)-1]
				if strings.Contains(strings.Join(args, " "), "-t bookmark") {
					// Bookmark existence check: pretend creation succeeded.
					return target + "\n", nil
				}
				return listings[target], nil
			}
			return "", nil
		},
	}
}

// TestPruneLocalSnapshotsCoversEveryDatasetInScope is the regression test for
// the prune half of the bug: pre-2.0 only POOL/home was ever pruned.
func TestPruneLocalSnapshotsCoversEveryDatasetInScope(t *testing.T) {
	// All 10 snapshots land on the same calendar day, an hour apart, so the
	// daily tier alone keeps just the newest one per dataset - unambiguous
	// without needing to reason about ISO week boundaries.
	reference := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	listing := func(dataset string) string {
		var b strings.Builder
		for hour := 0; hour <= 9; hour++ {
			fmt.Fprintf(&b, "%s@2026-08-10.%02dh-00-Backup\t%d\t1024\n",
				dataset, hour, time.Date(2026, 8, 10, hour, 0, 0, 0, time.UTC).Unix())
		}
		return b.String()
	}
	runner := pruneFixtureRunner(map[string]string{
		"NIXROOT/home":  listing("NIXROOT/home"),
		"NIXROOT/atuin": listing("NIXROOT/atuin"),
	})

	result := pruneLocalSnapshots(context.Background(), runner, "NIXROOT",
		[]string{"home", "atuin"}, defaultRetentionPolicy, reference)

	if len(result.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
	// 10 snapshots per dataset, same day, keeping only the newest, leaves 9
	// to prune on each.
	if len(result.Pruned) != 18 {
		t.Fatalf("expected 18 pruned snapshots across both datasets, got %d: %v",
			len(result.Pruned), result.Pruned)
	}
	for _, dataset := range []string{"NIXROOT/home", "NIXROOT/atuin"} {
		if !runner.ran("zfs destroy " + dataset + "@") {
			t.Errorf("%s should have been pruned", dataset)
		}
		if !runner.ran("zfs bookmark " + dataset + "@") {
			t.Errorf("%s should have been bookmarked before pruning", dataset)
		}
		if runner.ran("zfs destroy " + dataset + "@2026-08-10.09h-00-Backup") {
			t.Errorf("%s: the newest snapshot of the day must survive", dataset)
		}
	}
}

func TestBookmarkAndDestroyRefusesToDestroyWithoutBookmark(t *testing.T) {
	runner := &fakeRunner{
		respond: func(_ string, args []string) (string, error) {
			if len(args) > 0 && args[0] == "bookmark" {
				return "", fmt.Errorf("permission denied")
			}
			if len(args) > 0 && args[0] == "list" {
				return "", fmt.Errorf("dataset does not exist")
			}
			return "", nil
		},
	}

	err := bookmarkAndDestroy(context.Background(), runner, "NIXROOT/home@2026-08-14.10h-00-Backup")

	if err == nil {
		t.Fatal("expected an error when the bookmark could not be created")
	}
	if runner.ran("zfs destroy") {
		t.Error("the snapshot must not be destroyed when its bookmark is missing")
	}
}

func TestBookmarkAndDestroyRefusesProtectedSnapshots(t *testing.T) {
	runner := &fakeRunner{}

	if err := bookmarkAndDestroy(context.Background(), runner, "NIXROOT/root@blank"); err == nil {
		t.Error("expected an error for a protected snapshot")
	}
	if len(runner.calls) != 0 {
		t.Errorf("no commands should run for a protected snapshot, got %v", runner.commandLines())
	}
}

func TestDestroySnapshotsSkipsProtected(t *testing.T) {
	runner := &fakeRunner{}

	destroySnapshots(context.Background(), runner, []string{
		"NIXROOT/root@blank",
		"NIXROOT/home@2026-08-14.10h-00-Backup",
	})

	if runner.mentions("@blank") {
		t.Error("@blank must never be destroyed")
	}
	if !runner.ran("zfs destroy NIXROOT/home@2026-08-14.10h-00-Backup") {
		t.Error("expected the zfs-backup snapshot to be destroyed")
	}
}

func TestSnapshotsForDatasets(t *testing.T) {
	snapshots := []string{
		"NIXROOT/home@2026-08-14.10h-00-Backup",
		"NIXROOT/nix@2026-08-14.10h-00-Backup",
	}

	got := snapshotsForDatasets(snapshots, []string{"NIXROOT/nix"})

	if !reflect.DeepEqual(got, []string{"NIXROOT/nix@2026-08-14.10h-00-Backup"}) {
		t.Errorf("snapshotsForDatasets = %v", got)
	}
}

// TestSyncoidBaseArgsAlwaysDisablesSyncSnapshots guards the second half of the
// orphan problem: syncoid's own sync-snapshots are orphaned on failed sends.
func TestSyncoidBaseArgsAlwaysDisablesSyncSnapshots(t *testing.T) {
	args := syncoidBaseArgs("NIXROOT/home", "NIXBACKUPS/abyss/home")

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--no-sync-snap") {
		t.Errorf("--no-sync-snap must always be passed, got %q", joined)
	}
	if args[len(args)-2] != "NIXROOT/home" || args[len(args)-1] != "NIXBACKUPS/abyss/home" {
		t.Errorf("source and destination must come last, got %q", joined)
	}

	withExtra := syncoidBaseArgs("a", "b", "--force-delete")
	if !strings.Contains(strings.Join(withExtra, " "), "--force-delete") {
		t.Errorf("extra flags must be preserved, got %v", withExtra)
	}
}
