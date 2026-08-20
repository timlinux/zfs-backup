// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// abyssFixture reproduces the state reported on host abyss: sanoid snapshots
// on NIXROOT/home (expected), plus zfs-backup and syncoid snapshots left on
// datasets that were never in the backup scope.
func abyssFixture(now time.Time) []snapshotEntry {
	entry := func(dataset, tag string, daysAgo int) snapshotEntry {
		return snapshotEntry{
			Name:     dataset + "@" + tag,
			Dataset:  dataset,
			Tag:      tag,
			Creation: now.AddDate(0, 0, -daysAgo),
			Used:     1024,
		}
	}
	return []snapshotEntry{
		entry("NIXROOT/home", "autosnap_2026-08-13_22:00:00_hourly", 1),
		entry("NIXROOT/home", "2026-08-13.23h-47-Backup", 1),
		entry("NIXROOT/root", "blank", 115),
		entry("NIXROOT/root", "2026-08-13.23h-47-Backup", 1),
		entry("NIXROOT/root", "syncoid_abyss_2026-05-29:00:49:53-GMT01:00", 77),
		entry("NIXROOT/nix", "2026-08-13.23h-47-Backup", 1),
		entry("NIXROOT", "2026-08-13.23h-47-Backup", 1),
		entry("NIXROOT/home/nested", "2026-08-13.23h-47-Backup", 1),
	}
}

func TestScanOrphansFindsOutOfScopeBackupSnapshots(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

	orphans := scanOrphans(abyssFixture(now), "NIXROOT", []string{"home"}, now, minSyncoidOrphanAge)

	var names []string
	for _, o := range orphans {
		names = append(names, o.Name)
	}
	want := []string{
		"NIXROOT/root@2026-08-13.23h-47-Backup",
		"NIXROOT/root@syncoid_abyss_2026-05-29:00:49:53-GMT01:00",
		"NIXROOT/nix@2026-08-13.23h-47-Backup",
		"NIXROOT@2026-08-13.23h-47-Backup",
		"NIXROOT/home/nested@2026-08-13.23h-47-Backup",
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("scanOrphans found\n  %v\nwant\n  %v", names, want)
	}
}

func TestScanOrphansNeverReportsProtectedOrForeignSnapshots(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

	orphans := scanOrphans(abyssFixture(now), "NIXROOT", []string{"home"}, now, minSyncoidOrphanAge)

	for _, o := range orphans {
		if o.Tag == "blank" {
			t.Error("@blank must never be reported as an orphan")
		}
		if strings.HasPrefix(o.Tag, "autosnap_") {
			t.Errorf("sanoid snapshots are not ours to clean: %s", o.Name)
		}
	}
}

func TestScanOrphansIgnoresDatasetsInScope(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

	orphans := scanOrphans(abyssFixture(now), "NIXROOT",
		[]string{"home", "root", "nix"}, now, minSyncoidOrphanAge)

	for _, o := range orphans {
		if o.Kind == orphanOutOfScope && (o.Dataset == "NIXROOT/root" || o.Dataset == "NIXROOT/nix") {
			t.Errorf("%s is in scope and is pruned normally, so it is not an orphan", o.Name)
		}
	}
}

func TestScanOrphansLeavesRecentSyncoidSnapshotsAlone(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	entries := []snapshotEntry{{
		Name:     "NIXROOT/root@syncoid_abyss_2026-08-14:11:00:00-GMT01:00",
		Dataset:  "NIXROOT/root",
		Tag:      "syncoid_abyss_2026-08-14:11:00:00-GMT01:00",
		Creation: now.Add(-1 * time.Hour),
	}}

	// A send may still be in flight, so a fresh sync-snapshot is not debris.
	if orphans := scanOrphans(entries, "NIXROOT", nil, now, minSyncoidOrphanAge); len(orphans) != 0 {
		t.Errorf("expected no orphans for a one-hour-old sync snapshot, got %+v", orphans)
	}
}

func TestParseDatasetUsageHandlesUnsetProperties(t *testing.T) {
	output := strings.Join([]string{
		"NIXROOT/root\t21692354560\t20615843840\t32212254720\t-",
		"NIXROOT/home\t100000\t1000\t-\t-",
		"",
	}, "\n")

	usages := parseDatasetUsage(output)

	if len(usages) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(usages))
	}
	if usages[0].Quota != 32212254720 {
		t.Errorf("quota not parsed: %+v", usages[0])
	}
	if usages[1].Quota != 0 {
		t.Errorf("an unset quota should read as 0, got %d", usages[1].Quota)
	}
}

func TestFlagQuotaPressureFlagsSnapshotDominatedDatasets(t *testing.T) {
	usages := []datasetUsage{
		// The reported case: 20.2G used, 19.2G snapshots, 30G quota.
		{Name: "NIXROOT/root", Used: 21692354560, UsedBySnapshots: 20615843840, Quota: 32212254720},
		{Name: "NIXROOT/home", Used: 21692354560, UsedBySnapshots: 1073741824, Quota: 322122547200},
		{Name: "NIXROOT/nix", Used: 0, UsedBySnapshots: 0},
	}

	flagged := flagQuotaPressure(usages, quotaPressureThreshold)

	if len(flagged) != 1 || flagged[0].Name != "NIXROOT/root" {
		t.Errorf("expected only NIXROOT/root to be flagged, got %+v", flagged)
	}
}

func TestVetOrphansAppliesSafetyChecks(t *testing.T) {
	held := "NIXROOT/root@2026-01-01.00h-00-Backup"
	cloned := "NIXROOT/root@2026-01-02.00h-00-Backup"
	clean := "NIXROOT/root@2026-01-03.00h-00-Backup"

	runner := &fakeRunner{
		respond: func(_ string, args []string) (string, error) {
			joined := strings.Join(args, " ")
			switch {
			case strings.HasPrefix(joined, "holds") && strings.Contains(joined, held):
				return "NIXROOT/root@2026-01-01.00h-00-Backup\tkeepme\t-\n", nil
			case strings.HasPrefix(joined, "holds"):
				return "", nil
			case strings.Contains(joined, "clones") && strings.Contains(joined, cloned):
				return "NIXROOT/clone-of-root\n", nil
			case strings.Contains(joined, "clones"):
				return "-\n", nil
			}
			return "", nil
		},
	}

	orphans := []orphanSnapshot{}
	for _, name := range []string{held, cloned, clean} {
		dataset, tag, _ := splitSnapshot(name)
		orphans = append(orphans, orphanSnapshot{
			snapshotEntry: snapshotEntry{Name: name, Dataset: dataset, Tag: tag},
			Kind:          orphanOutOfScope,
		})
	}

	safe := safeToDestroy(vetOrphans(context.Background(), runner, orphans))

	if !reflect.DeepEqual(safe, []string{clean}) {
		t.Errorf("expected only the unencumbered snapshot to be destroyable, got %v", safe)
	}
}

func TestVetOrphansTreatsUnknownStateAsUnsafe(t *testing.T) {
	runner := &fakeRunner{
		respond: func(_ string, args []string) (string, error) {
			return "", fmt.Errorf("zfs unavailable")
		},
	}
	orphans := []orphanSnapshot{{
		snapshotEntry: snapshotEntry{
			Name:    "NIXROOT/root@2026-01-01.00h-00-Backup",
			Dataset: "NIXROOT/root",
			Tag:     "2026-01-01.00h-00-Backup",
		},
	}}

	if safe := safeToDestroy(vetOrphans(context.Background(), runner, orphans)); len(safe) != 0 {
		t.Errorf("a snapshot we cannot prove is safe must not be destroyed, got %v", safe)
	}
}

func TestGroupOrphansByDataset(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	orphans := scanOrphans(abyssFixture(now), "NIXROOT", []string{"home"}, now, minSyncoidOrphanAge)

	datasets, grouped := groupOrphansByDataset(orphans)

	if len(datasets) != 4 {
		t.Fatalf("expected 4 affected datasets, got %v", datasets)
	}
	if len(grouped["NIXROOT/root"]) != 2 {
		t.Errorf("expected 2 orphans on NIXROOT/root, got %d", len(grouped["NIXROOT/root"]))
	}
}
