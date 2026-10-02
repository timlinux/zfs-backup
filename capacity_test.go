// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDatasetAvailableBytes(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, error) {
		return "5000000\n", nil
	}}
	n, ok := datasetAvailableBytes(context.Background(), r, "NIXBACKUPS")
	if !ok || n != 5000000 {
		t.Errorf("datasetAvailableBytes() = (%d, %v), want (5000000, true)", n, ok)
	}

	failing := &fakeRunner{respond: func(name string, args []string) (string, error) {
		return "", errors.New("pool does not exist")
	}}
	if _, ok := datasetAvailableBytes(context.Background(), failing, "GONE"); ok {
		t.Error("expected ok=false when the pool lookup fails")
	}
}

func TestCheckBackupCapacitySufficient(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, error) {
		return "1000\n", nil
	}}
	reqs := []datasetRequirement{{Dataset: "home", RequiredBytes: 400}, {Dataset: "docs", RequiredBytes: 500}}

	check := checkBackupCapacity(context.Background(), r, "NIXBACKUPS", reqs)

	if check.RequiredBytes != 900 {
		t.Errorf("RequiredBytes = %d, want 900", check.RequiredBytes)
	}
	if !check.Sufficient() {
		t.Errorf("expected 1000 available to be sufficient for 900 required")
	}
	if check.ShortfallBytes() != 0 {
		t.Errorf("expected no shortfall, got %d", check.ShortfallBytes())
	}
}

func TestCheckBackupCapacityInsufficient(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, error) {
		return "100\n", nil
	}}
	reqs := []datasetRequirement{{Dataset: "home", RequiredBytes: 900}}

	check := checkBackupCapacity(context.Background(), r, "NIXBACKUPS", reqs)

	if check.Sufficient() {
		t.Error("100 available must not be sufficient for 900 required")
	}
	if got := check.ShortfallBytes(); got != 800 {
		t.Errorf("ShortfallBytes() = %d, want 800", got)
	}
}

func TestCheckBackupCapacityUnknownAvailableIsTreatedAsInsufficient(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, error) {
		return "", errors.New("pool does not exist")
	}}
	reqs := []datasetRequirement{{Dataset: "home", RequiredBytes: 100}}

	check := checkBackupCapacity(context.Background(), r, "NIXBACKUPS", reqs)

	if check.Sufficient() {
		t.Error("an unreadable available figure must never be treated as sufficient")
	}
	// Nothing meaningful to report as a shortfall amount when we don't know
	// the available figure - the advisory falls back to saying so instead.
	if got := check.ShortfallBytes(); got != 0 {
		t.Errorf("ShortfallBytes() = %d, want 0 when available is unknown", got)
	}
}

func TestEstimateDatasetSendBytesUsesDryRunSize(t *testing.T) {
	r := &fakeRunner{respond: func(name string, args []string) (string, error) {
		if len(args) > 0 && args[0] == "send" {
			return "size\t2048\n", nil
		}
		return "0\n", nil
	}}
	snaps := []snapshotInfo{{Tag: "a", Size: "1K"}}

	got := estimateDatasetSendBytes(context.Background(), r, "tank/home", snaps, func() map[string]bool { return map[string]bool{} })

	if got != 2048 {
		t.Errorf("estimateDatasetSendBytes() = %d, want 2048", got)
	}
}

func TestCapacityAdvisoryMentionsShortfallAndOptions(t *testing.T) {
	check := CapacityCheck{
		Requirements:   []datasetRequirement{{Dataset: "home", RequiredBytes: 5 * 1024 * 1024 * 1024}},
		RequiredBytes:  5 * 1024 * 1024 * 1024,
		AvailableBytes: 1 * 1024 * 1024 * 1024,
		AvailableKnown: true,
	}

	advisory := capacityAdvisory(check, 0)

	for _, want := range []string{"5.0 GB", "1.0 GB", "home", "larger backup disk", "Backup Scope"} {
		if !strings.Contains(advisory, want) {
			t.Errorf("advisory missing %q:\n%s", want, advisory)
		}
	}
}

func TestCapacityAdvisoryReportsUnknownAvailable(t *testing.T) {
	check := CapacityCheck{RequiredBytes: 1024, AvailableKnown: false}

	advisory := capacityAdvisory(check, 0)

	if !strings.Contains(advisory, "could not be read") {
		t.Errorf("expected the advisory to explain the unknown available figure, got:\n%s", advisory)
	}
}

func TestCapacityAdvisoryMentionsReclaimedSpace(t *testing.T) {
	check := CapacityCheck{RequiredBytes: 1024, AvailableBytes: 100, AvailableKnown: true}

	advisory := capacityAdvisory(check, 50*1024*1024)

	if !strings.Contains(advisory, "50.0 MB") {
		t.Errorf("expected the advisory to mention what pruning reclaimed, got:\n%s", advisory)
	}
}
