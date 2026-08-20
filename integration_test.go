// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

//go:build integration

// Integration tests that exercise the snapshot, prune and cleanup logic
// against a real, throwaway, file-backed ZFS pool. They verify that the
// commands we build are actually valid ZFS, which the unit tests cannot.
//
// These tests create and destroy a pool, so they are opt-in and require root:
//
//	sudo -E env "PATH=$PATH" go test -tags integration -run TestIntegration -v ./...
//
// They refuse to run unless ZFS_BACKUP_INTEGRATION=1 is set, and they refuse
// to touch a pool that already exists.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testPoolName is deliberately unlikely to collide with a real pool.
const testPoolName = "zfsbackuptestpool"

// requireIntegrationEnv skips unless the caller explicitly opted in and can
// actually create pools.
func requireIntegrationEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("ZFS_BACKUP_INTEGRATION") != "1" {
		t.Skip("set ZFS_BACKUP_INTEGRATION=1 to run integration tests")
	}
	if os.Geteuid() != 0 {
		t.Skip("integration tests need root to create a ZFS pool")
	}
	if _, err := exec.LookPath("zpool"); err != nil {
		t.Skip("zpool not available")
	}
}

// newTestPool creates a file-backed pool with the given child datasets and
// registers its destruction with the test.
func newTestPool(t *testing.T, datasets ...string) string {
	t.Helper()

	if err := exec.Command("zpool", "list", testPoolName).Run(); err == nil {
		t.Fatalf("pool %s already exists - refusing to touch it", testPoolName)
	}

	dir := t.TempDir()
	vdev := filepath.Join(dir, "vdev.img")
	if err := exec.Command("truncate", "-s", "256M", vdev).Run(); err != nil {
		t.Fatalf("could not create vdev file: %v", err)
	}

	out, err := exec.Command("zpool", "create", "-m", "none", testPoolName, vdev).CombinedOutput()
	if err != nil {
		t.Fatalf("zpool create failed: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("zpool", "destroy", "-f", testPoolName).Run()
	})

	for _, ds := range datasets {
		out, err := exec.Command("zfs", "create", testPoolName+"/"+ds).CombinedOutput()
		if err != nil {
			t.Fatalf("zfs create %s failed: %v\n%s", ds, err, out)
		}
	}

	return testPoolName
}

// snapshotCount returns how many snapshots exist on exactly the given dataset.
func snapshotCount(t *testing.T, dataset string) int {
	t.Helper()
	out, err := exec.Command("zfs", "list", "-H", "-o", "name", "-t", "snapshot", "-d", "1", dataset).Output()
	if err != nil {
		t.Fatalf("listing snapshots of %s failed: %v", dataset, err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

func snapshotExists(t *testing.T, snapshot string) bool {
	t.Helper()
	return exec.Command("zfs", "list", "-H", "-t", "snapshot", snapshot).Run() == nil
}

// TestIntegrationSnapshotScopeLeavesOtherDatasetsUntouched is the bug brief's
// scenario 1: configure one dataset, run the snapshot phase, and assert the
// other datasets carry no snapshots at all.
func TestIntegrationSnapshotScopeLeavesOtherDatasetsUntouched(t *testing.T) {
	requireIntegrationEnv(t)
	pool := newTestPool(t, "home", "root", "nix")
	ctx := context.Background()

	tag := snapshotTagForTime(time.Now())
	if _, err := createDatasetSnapshots(ctx, defaultRunner, pool, []string{"home"}, tag); err != nil {
		t.Fatalf("createDatasetSnapshots: %v", err)
	}

	if got := snapshotCount(t, pool+"/home"); got != 1 {
		t.Errorf("expected 1 snapshot on %s/home, got %d", pool, got)
	}
	for _, ds := range []string{"root", "nix"} {
		if got := snapshotCount(t, pool+"/"+ds); got != 0 {
			t.Errorf("%s/%s must have no snapshots, got %d", pool, ds, got)
		}
	}
	if got := snapshotCount(t, pool); got != 0 {
		t.Errorf("the pool root dataset must have no snapshots, got %d", got)
	}
}

// TestIntegrationRepeatedRunsStayBounded is scenario 4: snapshot counts must
// not grow without limit across many runs, on every dataset in scope.
func TestIntegrationRepeatedRunsStayBounded(t *testing.T) {
	requireIntegrationEnv(t)
	pool := newTestPool(t, "home", "atuin")
	ctx := context.Background()
	datasets := []string{"home", "atuin"}

	for run := 0; run < 12; run++ {
		// Distinct tags, oldest first, so pruning has something to do.
		tag := snapshotTagForTime(time.Date(2026, 1, 1+run, 10, 0, 0, 0, time.UTC))
		if _, err := createDatasetSnapshots(ctx, defaultRunner, pool, datasets, tag); err != nil {
			t.Fatalf("run %d: createDatasetSnapshots: %v", run, err)
		}
		result := pruneLocalSnapshots(ctx, defaultRunner, pool, datasets, localBackupSnapshotsKept)
		if len(result.Warnings) > 0 {
			t.Fatalf("run %d: prune warnings: %v", run, result.Warnings)
		}
	}

	for _, ds := range datasets {
		if got := snapshotCount(t, pool+"/"+ds); got != localBackupSnapshotsKept {
			t.Errorf("%s/%s should hold %d snapshots after 12 runs, got %d",
				pool, ds, localBackupSnapshotsKept, got)
		}
		// Pruned snapshots must survive as bookmarks so incrementals still work.
		out, err := exec.Command("zfs", "list", "-H", "-o", "name", "-t", "bookmark", pool+"/"+ds).Output()
		if err != nil {
			t.Fatalf("listing bookmarks failed: %v", err)
		}
		if strings.TrimSpace(string(out)) == "" {
			t.Errorf("%s/%s should have bookmarks for the pruned snapshots", pool, ds)
		}
	}
}

// TestIntegrationDoctorDetectsPlantedOrphans is scenario 5, plus the mandatory
// safety checks: a hand-planted orphan is found, and @blank never is.
func TestIntegrationDoctorDetectsPlantedOrphans(t *testing.T) {
	requireIntegrationEnv(t)
	pool := newTestPool(t, "home", "root")
	ctx := context.Background()

	planted := []string{
		pool + "/root@2026-01-01.00h-00-Backup",
		pool + "/root@syncoid_testhost_2026-01-01:00:00:00-GMT00:00",
		pool + "/root@blank",
		pool + "/home@2026-01-01.00h-00-Backup",
	}
	for _, snap := range planted {
		if out, err := exec.Command("zfs", "snapshot", snap).CombinedOutput(); err != nil {
			t.Fatalf("planting %s failed: %v\n%s", snap, err, out)
		}
	}

	entries, err := listSnapshotEntries(ctx, defaultRunner, pool, 0)
	if err != nil {
		t.Fatalf("listSnapshotEntries: %v", err)
	}

	// "home" is in scope; "root" is not.
	orphans := scanOrphans(entries, pool, []string{"home"}, time.Now(), minSyncoidOrphanAge)

	found := map[string]bool{}
	for _, o := range orphans {
		found[o.Name] = true
	}
	if !found[pool+"/root@2026-01-01.00h-00-Backup"] {
		t.Error("the planted out-of-scope backup snapshot should have been detected")
	}
	if !found[pool+"/root@syncoid_testhost_2026-01-01:00:00:00-GMT00:00"] {
		t.Error("the planted syncoid leftover should have been detected")
	}
	if found[pool+"/root@blank"] {
		t.Error("@blank must never be reported as an orphan")
	}
	if found[pool+"/home@2026-01-01.00h-00-Backup"] {
		t.Error("a snapshot on an in-scope dataset is managed, not an orphan")
	}

	// Now destroy them for real and confirm the safety net held.
	safe := safeToDestroy(vetOrphans(ctx, defaultRunner, orphans))
	if failed := destroySnapshots(ctx, defaultRunner, safe); len(failed) > 0 {
		t.Fatalf("failed to destroy %v", failed)
	}
	if snapshotExists(t, pool+"/root@2026-01-01.00h-00-Backup") {
		t.Error("the orphan should have been destroyed")
	}
	if !snapshotExists(t, pool+"/root@blank") {
		t.Error("@blank must survive cleanup")
	}
	if !snapshotExists(t, pool+"/home@2026-01-01.00h-00-Backup") {
		t.Error("in-scope snapshots must survive cleanup")
	}
}

// TestIntegrationHeldSnapshotsAreNeverDestroyed exercises the hold pre-flight
// check against real ZFS holds.
func TestIntegrationHeldSnapshotsAreNeverDestroyed(t *testing.T) {
	requireIntegrationEnv(t)
	pool := newTestPool(t, "root")
	ctx := context.Background()

	snapshot := pool + "/root@2026-01-01.00h-00-Backup"
	if out, err := exec.Command("zfs", "snapshot", snapshot).CombinedOutput(); err != nil {
		t.Fatalf("snapshot failed: %v\n%s", err, out)
	}
	if out, err := exec.Command("zfs", "hold", "keepme", snapshot).CombinedOutput(); err != nil {
		t.Fatalf("hold failed: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("zfs", "release", "keepme", snapshot).Run() })

	entries, err := listSnapshotEntries(ctx, defaultRunner, pool, 0)
	if err != nil {
		t.Fatalf("listSnapshotEntries: %v", err)
	}
	orphans := scanOrphans(entries, pool, nil, time.Now(), minSyncoidOrphanAge)
	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(orphans))
	}

	if safe := safeToDestroy(vetOrphans(ctx, defaultRunner, orphans)); len(safe) != 0 {
		t.Errorf("a held snapshot must never be cleared for destruction, got %v", safe)
	}
	if !snapshotExists(t, snapshot) {
		t.Error("the held snapshot must still exist")
	}
}

// TestIntegrationBookmarkAndDestroyRoundTrip proves the prune path builds valid
// ZFS commands: a bookmark is created and the snapshot then removed.
func TestIntegrationBookmarkAndDestroyRoundTrip(t *testing.T) {
	requireIntegrationEnv(t)
	pool := newTestPool(t, "home")
	ctx := context.Background()

	snapshot := fmt.Sprintf("%s/home@2026-01-01.00h-00-Backup", pool)
	if out, err := exec.Command("zfs", "snapshot", snapshot).CombinedOutput(); err != nil {
		t.Fatalf("snapshot failed: %v\n%s", err, out)
	}

	if err := bookmarkAndDestroy(ctx, defaultRunner, snapshot); err != nil {
		t.Fatalf("bookmarkAndDestroy: %v", err)
	}

	if snapshotExists(t, snapshot) {
		t.Error("the snapshot should have been destroyed")
	}
	bookmark := strings.Replace(snapshot, "@", "#", 1)
	if err := exec.Command("zfs", "list", "-H", "-t", "bookmark", bookmark).Run(); err != nil {
		t.Errorf("the bookmark %s should exist", bookmark)
	}
}
