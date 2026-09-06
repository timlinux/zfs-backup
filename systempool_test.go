// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"
)

// nixrootSystemListing is what `zfs list -H -o name,mountpoint,mounted -r
// NIXROOT` reports on a NixOS system pool: the root filesystem, /home, /nix,
// and an unmounted application dataset.
const nixrootSystemListing = "NIXROOT\tnone\tno\n" +
	"NIXROOT/home\t/home\tyes\n" +
	"NIXROOT/nix\t/nix\tyes\n" +
	"NIXROOT/root\t/\tyes\n" +
	"NIXROOT/atuin\t-\tno\n"

// backupPoolListing is a healthy external backup pool: nothing mounted at a
// system path even though received datasets inherited system-ish mountpoints.
const backupPoolListing = "NIXBACKUPS\t/mnt/NIXBACKUPS\tyes\n" +
	"NIXBACKUPS/abyss\t/mnt/NIXBACKUPS/abyss\tyes\n" +
	"NIXBACKUPS/abyss/home\t/home\tno\n" +
	"NIXBACKUPS/abyss/root\t/\tno\n"

func TestSystemDatasetsFromList(t *testing.T) {
	system := systemDatasetsFromList(nixrootSystemListing)
	if len(system) != 3 {
		t.Fatalf("expected 3 system datasets, got %d: %v", len(system), system)
	}
	joined := strings.Join(system, "; ")
	for _, want := range []string{"NIXROOT/root (mounted at /)", "NIXROOT/home (mounted at /home)", "NIXROOT/nix (mounted at /nix)"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in %q", want, joined)
		}
	}
}

func TestSystemDatasetsFromListIgnoresUnmountedSystemMountpoints(t *testing.T) {
	// A backup pool carries datasets whose mountpoint property says /home or
	// even / - but they are not mounted, so the pool is not a system pool.
	if system := systemDatasetsFromList(backupPoolListing); len(system) != 0 {
		t.Fatalf("backup pool wrongly flagged as system pool: %v", system)
	}
}

// scriptedRunner builds a fakeRunner whose responder answers each command
// line (name and args joined by spaces) via the supplied function.
func scriptedRunner(respond func(line string) (string, error)) *fakeRunner {
	return &fakeRunner{respond: func(name string, args []string) (string, error) {
		return respond(strings.Join(append([]string{name}, args...), " "))
	}}
}

// listingError marks a command as failed, standing in for zfs complaining.
type listingError struct{}

func (listingError) Error() string { return "cannot open: dataset does not exist" }

func TestVetLocalBackupDestinationRefusesSystemPool(t *testing.T) {
	r := scriptedRunner(func(line string) (string, error) {
		if strings.HasPrefix(line, "zfs list") && strings.Contains(line, "NIXROOT") {
			return nixrootSystemListing, nil
		}
		return "", listingError{}
	})

	err := vetLocalBackupDestination(context.Background(), r, "NIXROOT")
	if err == nil {
		t.Fatal("system pool accepted as backup destination")
	}
	for _, want := range []string{"NIXROOT", "running system", "Pull Remote Backup"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

func TestVetLocalBackupDestinationAllowsBackupPool(t *testing.T) {
	r := scriptedRunner(func(line string) (string, error) {
		return backupPoolListing, nil
	})
	if err := vetLocalBackupDestination(context.Background(), r, "NIXBACKUPS"); err != nil {
		t.Fatalf("healthy backup pool refused: %v", err)
	}
}

func TestVetLocalBackupDestinationAllowsUnimportedPool(t *testing.T) {
	// The destination is typically an external drive that is not imported
	// yet; the running system's pool is always imported, so an unlistable
	// pool passes the vet.
	r := scriptedRunner(func(line string) (string, error) {
		return "", listingError{}
	})
	if err := vetLocalBackupDestination(context.Background(), r, "NIXBACKUPS"); err != nil {
		t.Fatalf("unimported pool refused: %v", err)
	}
}

func TestPerformBackupRefusesSystemPoolDestinationBeforeTouchingAnything(t *testing.T) {
	r := scriptedRunner(func(line string) (string, error) {
		if strings.Contains(line, "NIXROOT") {
			return nixrootSystemListing, nil
		}
		return "", listingError{}
	})
	previous := defaultRunner
	defaultRunner = r
	t.Cleanup(func() { defaultRunner = previous })

	_, err := performBackup(context.Background(), "", "TANK", "NIXROOT", nil, nil)
	if err == nil {
		t.Fatal("performBackup accepted a system pool as destination")
	}
	if !strings.Contains(err.Error(), "running system") {
		t.Errorf("unexpected error: %v", err)
	}
	for _, forbidden := range []string{"snapshot", "rename", "destroy", "syncoid", "zpool import"} {
		if r.mentions(forbidden) {
			t.Errorf("guard ran too late: a %q command was issued", forbidden)
		}
	}
}

func TestPerformForceBackupRefusesSystemPoolDestination(t *testing.T) {
	r := scriptedRunner(func(line string) (string, error) {
		if strings.Contains(line, "NIXROOT") {
			return nixrootSystemListing, nil
		}
		return "", listingError{}
	})
	previous := defaultRunner
	defaultRunner = r
	t.Cleanup(func() { defaultRunner = previous })

	_, err := performForceBackup(context.Background(), "", "TANK", "NIXROOT", nil, nil)
	if err == nil {
		t.Fatal("performForceBackup accepted a system pool as destination")
	}
	if !strings.Contains(err.Error(), "running system") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMigrateLegacyLayoutRefusesSystemPool(t *testing.T) {
	r := scriptedRunner(func(line string) (string, error) {
		if strings.Contains(line, "NIXROOT") {
			return nixrootSystemListing, nil
		}
		return "", listingError{}
	})
	previous := defaultRunner
	defaultRunner = r
	t.Cleanup(func() { defaultRunner = previous })

	var output strings.Builder
	err := migrateLegacyDestinationLayout(context.Background(), "NIXROOT", "abyss",
		[]string{"home", "nix", "root", "atuin"}, &output)
	if err == nil {
		t.Fatal("migration ran against the pool holding the running system")
	}
	if !strings.Contains(err.Error(), "running system") {
		t.Errorf("unexpected error: %v", err)
	}
	if r.mentions("rename") {
		t.Error("migration issued a rename on a system pool")
	}
}

func TestVetDestPoolSelection(t *testing.T) {
	m := model{
		systemPools: map[string]string{"NIXROOT": "NIXROOT/root (mounted at /)"},
	}

	cases := []struct {
		name            string
		operation       string
		selectingSource bool
		pool            string
		refused         bool
	}{
		{"backup dest refuses system pool", "backup", false, "NIXROOT", true},
		{"force backup dest refuses system pool", "force-backup", false, "NIXROOT", true},
		{"backup dest allows backup pool", "backup", false, "NIXBACKUPS", false},
		{"pull may target system pool", "remote-backup", false, "NIXROOT", false},
		{"source selection unrestricted", "backup", true, "NIXROOT", false},
		{"info screens unrestricted", "zpoolinfo", false, "NIXROOT", false},
	}
	for _, tc := range cases {
		m.operation = tc.operation
		m.selectingSource = tc.selectingSource
		if got := m.vetDestPoolSelection(tc.pool); (got != "") != tc.refused {
			t.Errorf("%s: got %q, refused=%v", tc.name, got, tc.refused)
		}
	}
}

func TestReplicationFailureErrorNamesEveryDatasetWithReason(t *testing.T) {
	err := replicationFailureError("backup",
		[]string{"home", "nix"},
		map[string]string{
			"home": "syncoid failed: CRITICAL ERROR: Target exists but has no snapshots matching with NIXROOT/home!",
			"nix":  "",
		})
	msg := err.Error()
	if !strings.HasPrefix(msg, "backup incomplete: 2 dataset(s) failed to replicate") {
		t.Errorf("unexpected headline: %q", msg)
	}
	if !strings.Contains(msg, "home: syncoid failed") || !strings.Contains(msg, "Target exists") {
		t.Errorf("reason missing from error: %q", msg)
	}
	if !strings.Contains(msg, "\nnix") {
		t.Errorf("reasonless dataset should still be named: %q", msg)
	}
}

func TestFailureLineCompressesAndTruncates(t *testing.T) {
	multi := "line one\n   line two\n\nline three"
	if got := failureLine(multi); got != "line one line two line three" {
		t.Errorf("whitespace not collapsed: %q", got)
	}
	long := strings.Repeat("x", 500)
	if got := failureLine(long); len(got) != 203 || !strings.HasSuffix(got, "...") {
		t.Errorf("long reason not truncated: len=%d", len(got))
	}
}
