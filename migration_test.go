// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"strings"
	"testing"
)

// existing builds an exists() predicate over a fixed set of dataset paths.
func existing(paths ...string) func(string) bool {
	present := make(map[string]bool, len(paths))
	for _, p := range paths {
		present[p] = true
	}
	return func(path string) bool { return present[path] }
}

// The reported failure: on host abyss, backing up a dataset named "abyss" made
// the legacy flat path and the hostname namespace container the same dataset,
// and the migration asked ZFS to rename it inside itself:
//
//	cannot rename to 'NIXBACKUPS/abyss/abyss':
//	New dataset name cannot be a descendant of current dataset name
func TestPlanLayoutMigrationNeverRenamesADatasetInsideItself(t *testing.T) {
	plan := planLayoutMigration("NIXBACKUPS", "abyss",
		[]string{"abyss"}, existing("NIXBACKUPS/abyss"))

	if len(plan.Renames) != 0 {
		t.Errorf("a dataset must never be renamed under itself, got %+v", plan.Renames)
	}
	if !reflect.DeepEqual(plan.Ambiguous, []string{"abyss"}) {
		t.Errorf("the collision should be reported as ambiguous, got %+v", plan.Ambiguous)
	}
	if len(plan.Conflicts) != 0 {
		t.Errorf("an ambiguous dataset is not a conflict, got %+v", plan.Conflicts)
	}
}

// Guard the invariant directly, independently of how the plan is reached: no
// rename may ever target a descendant of its own source.
func TestPlanLayoutMigrationRenamesAreNeverDescendantsOfTheirSource(t *testing.T) {
	plan := planLayoutMigration("NIXBACKUPS", "abyss",
		[]string{"abyss", "home", "root"},
		existing("NIXBACKUPS/abyss", "NIXBACKUPS/home", "NIXBACKUPS/root"))

	for _, r := range plan.Renames {
		if r.to == r.from || strings.HasPrefix(r.to, r.from+"/") {
			t.Errorf("rename %s -> %s makes the target a descendant of the source", r.from, r.to)
		}
	}
}

// The ambiguous dataset must not stop the datasets around it migrating.
func TestPlanLayoutMigrationStillMigratesTheOtherDatasets(t *testing.T) {
	plan := planLayoutMigration("NIXBACKUPS", "abyss",
		[]string{"abyss", "home", "root"},
		existing("NIXBACKUPS/abyss", "NIXBACKUPS/home", "NIXBACKUPS/root"))

	want := []layoutRename{
		{from: "NIXBACKUPS/home", to: "NIXBACKUPS/abyss/home"},
		{from: "NIXBACKUPS/root", to: "NIXBACKUPS/abyss/root"},
	}
	if !reflect.DeepEqual(plan.Renames, want) {
		t.Errorf("renames = %+v, want %+v", plan.Renames, want)
	}
}

// A pool already migrated to the namespace layout has nothing at the flat
// paths, so a re-run must be a no-op rather than a second migration.
func TestPlanLayoutMigrationIsANoOpOnAnAlreadyMigratedPool(t *testing.T) {
	plan := planLayoutMigration("NIXBACKUPS", "abyss",
		[]string{"home", "root"},
		existing("NIXBACKUPS/abyss", "NIXBACKUPS/abyss/home", "NIXBACKUPS/abyss/root"))

	if len(plan.Renames) != 0 || len(plan.Conflicts) != 0 || len(plan.Ambiguous) != 0 {
		t.Errorf("re-running on a migrated pool should do nothing, got %+v", plan)
	}
}

// A dataset present at both paths is a genuine conflict: merging could lose
// data, so it is reported rather than guessed at.
func TestPlanLayoutMigrationReportsBothPathsAsAConflict(t *testing.T) {
	plan := planLayoutMigration("NIXBACKUPS", "abyss",
		[]string{"home"},
		existing("NIXBACKUPS/home", "NIXBACKUPS/abyss/home"))

	if !reflect.DeepEqual(plan.Conflicts, []string{"home"}) {
		t.Errorf("conflicts = %+v, want [home]", plan.Conflicts)
	}
	if len(plan.Renames) != 0 {
		t.Errorf("a conflicting dataset must not be renamed, got %+v", plan.Renames)
	}
}

// A dataset named after the host that is not actually present needs nothing
// said about it.
func TestPlanLayoutMigrationIgnoresAnAbsentHostnameDataset(t *testing.T) {
	plan := planLayoutMigration("NIXBACKUPS", "abyss",
		[]string{"abyss"}, existing("NIXBACKUPS/other"))

	if len(plan.Ambiguous) != 0 {
		t.Errorf("nothing exists at the flat path, so there is nothing to report: %+v", plan.Ambiguous)
	}
}

func TestPlanLayoutMigrationLeavesUnbackedDatasetsAlone(t *testing.T) {
	plan := planLayoutMigration("NIXBACKUPS", "abyss",
		[]string{"home"},
		existing("NIXBACKUPS/home", "NIXBACKUPS/somebodyelses-data"))

	for _, r := range plan.Renames {
		if strings.Contains(r.from, "somebodyelses-data") {
			t.Errorf("migration must only touch datasets in scope, got %+v", r)
		}
	}
}
