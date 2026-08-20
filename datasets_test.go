// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"testing"
)

func TestApplyScopeDefaultsToEverything(t *testing.T) {
	available := []string{"home", "nix", "root"}

	selected, missing := applyScope(available, nil)

	if !reflect.DeepEqual(selected, available) {
		t.Errorf("an unconfigured pool should back up every dataset, got %v", selected)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing datasets, got %v", missing)
	}
}

func TestApplyScopeRestrictsToConfiguredDatasets(t *testing.T) {
	available := []string{"atuin", "home", "nix", "overflow", "root"}

	selected, missing := applyScope(available, []string{"home"})

	if !reflect.DeepEqual(selected, []string{"home"}) {
		t.Errorf("expected only home in scope, got %v", selected)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing datasets, got %v", missing)
	}
}

func TestApplyScopeReportsConfiguredButMissingDatasets(t *testing.T) {
	selected, missing := applyScope([]string{"home"}, []string{"home", "photos"})

	if !reflect.DeepEqual(selected, []string{"home"}) {
		t.Errorf("expected home in scope, got %v", selected)
	}
	if !reflect.DeepEqual(missing, []string{"photos"}) {
		t.Errorf("expected photos reported as missing, got %v", missing)
	}
}

func TestApplyScopeFollowsOnDiskOrder(t *testing.T) {
	selected, _ := applyScope([]string{"atuin", "home", "nix"}, []string{"nix", "atuin"})

	if !reflect.DeepEqual(selected, []string{"atuin", "nix"}) {
		t.Errorf("expected on-disk ordering, got %v", selected)
	}
}

func TestQualifyDatasets(t *testing.T) {
	got := qualifyDatasets("NIXROOT", []string{"home", "atuin"})
	want := []string{"NIXROOT/home", "NIXROOT/atuin"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("qualifyDatasets = %v, want %v", got, want)
	}
}
