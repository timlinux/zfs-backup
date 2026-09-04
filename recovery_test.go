// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The reported failure, classified.
func TestParsePoolHealthRecognisesASuspendedPool(t *testing.T) {
	health := parsePoolHealth("NIXBACKUPS",
		"cannot open 'NIXBACKUPS': pool I/O is currently suspended", errors.New("exit status 1"))

	if health.State != poolSuspended {
		t.Errorf("state = %q, want %q", health.State, poolSuspended)
	}
	if health.Usable() {
		t.Error("a suspended pool must not be reported as usable")
	}
	if !strings.Contains(health.Summary(), "SUSPENDED") {
		t.Errorf("summary should name the condition: %q", health.Summary())
	}
}

func TestParsePoolHealthClassifiesTheStatesWeActdOn(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   poolState
		usable bool
	}{
		{"online", "  pool: TANK\n state: ONLINE\n", poolOnline, true},
		// Degraded means redundancy is gone, not the data - a backup can proceed.
		{"degraded", "  pool: TANK\n state: DEGRADED\n", poolDegraded, true},
		{"unavailable", "  pool: TANK\n state: UNAVAIL\n", poolUnavailable, false},
		{"faulted", "  pool: TANK\n state: FAULTED\n", poolUnavailable, false},
		{"suspended state line", "  pool: TANK\n state: SUSPENDED\n", poolSuspended, false},
		{"not imported", "cannot open 'TANK': no such pool", poolNotImported, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			health := parsePoolHealth("TANK", tc.output, nil)
			if health.State != tc.want {
				t.Errorf("state = %q, want %q", health.State, tc.want)
			}
			if health.Usable() != tc.usable {
				t.Errorf("Usable() = %v, want %v", health.Usable(), tc.usable)
			}
		})
	}
}

// Recovery runs commands on the user's behalf, so the ladder must contain
// nothing that can lose data. zpool destroy, labelclear and a non-forced
// import of someone else's pool have no place here.
func TestRemediesNeverRunSomethingDestructive(t *testing.T) {
	banned := []string{"destroy", "labelclear", "split", "remove", "replace", "zfs"}

	for _, remedy := range poolRemedies("NIXBACKUPS") {
		for _, command := range remedy.Commands {
			line := strings.Join(command, " ")
			if command[0] != "zpool" {
				t.Errorf("recovery should only drive zpool, got %q", line)
			}
			for _, word := range banned {
				for _, arg := range command[1:] {
					if arg == word {
						t.Errorf("remedy %q runs a destructive subcommand: %q", remedy.Title, line)
					}
				}
			}
		}
	}
}

// The gentlest remedy must come first, and the forceful one must ask.
func TestRemediesEscalateGentlestFirst(t *testing.T) {
	remedies := poolRemedies("NIXBACKUPS")

	if len(remedies) < 2 {
		t.Fatalf("expected an escalation ladder, got %d step(s)", len(remedies))
	}
	if got := strings.Join(remedies[0].Commands[0], " "); got != "zpool clear NIXBACKUPS" {
		t.Errorf("first remedy = %q, want the gentlest (zpool clear)", got)
	}
	if remedies[0].NeedsConfirm {
		t.Error("zpool clear is safe and should not need confirmation")
	}
	if !remedies[1].NeedsConfirm {
		t.Error("a forced export interrupts I/O and must be confirmed")
	}
}

func TestApplyRemedyStopsAtTheFirstFailure(t *testing.T) {
	runner := &fakeRunner{
		respond: func(name string, args []string) (string, error) {
			if len(args) > 0 && args[0] == "export" {
				return "", errors.New("pool is busy")
			}
			return "  pool: NIXBACKUPS\n state: SUSPENDED\n", nil
		},
	}
	forced := poolRemedies("NIXBACKUPS")[1]

	result := applyRemedy(context.Background(), runner, "NIXBACKUPS", forced)

	if result.Err == nil {
		t.Fatal("the failure should be reported")
	}
	if runner.ran("zpool import") {
		t.Error("import must not run after export failed - that would be guesswork")
	}
	if len(result.Log) != 1 || !strings.Contains(result.Log[0], "failed") {
		t.Errorf("log should record the failed command, got %v", result.Log)
	}
}

// After every remedy the pool is re-checked, so the screen never claims
// success it has not verified.
func TestApplyRemedyAlwaysRechecksThePool(t *testing.T) {
	runner := &fakeRunner{
		respond: func(_ string, _ []string) (string, error) {
			return "  pool: NIXBACKUPS\n state: ONLINE\n", nil
		},
	}

	result := applyRemedy(context.Background(), runner, "NIXBACKUPS", poolRemedies("NIXBACKUPS")[0])

	if !runner.ran("zpool status NIXBACKUPS") {
		t.Error("the pool must be re-checked after a remedy")
	}
	if !result.Health.Usable() {
		t.Errorf("the re-check should have found the pool online, got %q", result.Health.State)
	}
}

// The offer to fix must only appear for failures recovery can actually act on.
func TestRecoverablePoolFromErrorOnlyOffersWhereItCanHelp(t *testing.T) {
	suspended := errors.New(
		"failed to check key status for NIXBACKUPS: cannot open 'NIXBACKUPS': pool I/O is currently suspended")
	if got := recoverablePoolFromError(suspended); got != "NIXBACKUPS" {
		t.Errorf("recoverablePoolFromError = %q, want NIXBACKUPS", got)
	}

	for _, other := range []error{
		nil,
		errors.New("source pool not selected"),
		errors.New("cannot open 'NIXBACKUPS': dataset does not exist"),
	} {
		if got := recoverablePoolFromError(other); got != "" {
			t.Errorf("no offer expected for %v, got %q", other, got)
		}
	}
}

func TestRecoverablePoolFromErrorNamesThePoolNotTheDataset(t *testing.T) {
	err := errors.New("cannot open 'NIXBACKUPS/abyss/home': pool I/O is currently suspended")

	if got := recoverablePoolFromError(err); got != "NIXBACKUPS" {
		t.Errorf("recoverablePoolFromError = %q, want NIXBACKUPS", got)
	}
}

// recoverModel parks a model on a recovery screen with a diagnosed pool.
func recoverModel() model {
	m := model{}.startPoolRecovery("NIXBACKUPS")
	m.recoverHealth = poolHealth{Pool: "NIXBACKUPS", State: poolSuspended}
	m.recoverPhase = recoverPhaseReady
	m.recoverReady = true
	return m
}

// The forceful remedy must not run on a single keypress.
func TestRecoveryScreenConfirmsBeforeForcingTheExport(t *testing.T) {
	m := recoverModel()
	m.recoverIndex = 1 // the forced export/import

	m, cmd := m.updateRecoverPoolScreen(tea.KeyMsg{Type: tea.KeyEnter})

	if m.recoverPhase != recoverPhaseConfirm {
		t.Fatalf("enter should ask first, got phase %v", m.recoverPhase)
	}
	if cmd != nil {
		t.Error("nothing should run until the user confirms")
	}

	m, cmd = m.updateRecoverPoolScreen(keyRunes("n"))
	if m.recoverPhase != recoverPhaseReady || cmd != nil {
		t.Error("declining must return to the diagnosis without running anything")
	}
}

// The gentle remedy is allowed to run straight away.
func TestRecoveryScreenRunsTheSafeRemedyWithoutAsking(t *testing.T) {
	m := recoverModel()

	m, cmd := m.updateRecoverPoolScreen(tea.KeyMsg{Type: tea.KeyEnter})

	if m.recoverPhase != recoverPhaseWorking {
		t.Errorf("zpool clear should run on enter, got phase %v", m.recoverPhase)
	}
	if cmd == nil {
		t.Error("enter should start the remedy")
	}
}

func TestRecoveryScreenIgnoresKeysWhileWorking(t *testing.T) {
	m := recoverModel()
	m.recoverPhase = recoverPhaseWorking

	for _, key := range []tea.KeyMsg{keyRunes("q"), keyRunes("r"), {Type: tea.KeyEnter}, {Type: tea.KeyEsc}} {
		next, cmd := m.updateRecoverPoolScreen(key)
		if next.recoverPhase != recoverPhaseWorking || next.state == stateMenu || cmd != nil {
			t.Errorf("key %v should be ignored mid-remedy", key)
		}
	}
}

// Success is only ever claimed on the strength of a re-check.
func TestRecoveryReportsSuccessOnlyWhenThePoolIsBack(t *testing.T) {
	m := recoverModel()

	stillDown := m.afterRemedy(remedyResult{
		Title:  "Ask ZFS to retry the failed I/O",
		Health: poolHealth{Pool: "NIXBACKUPS", State: poolSuspended},
	})
	if stillDown.recoverPhase == recoverPhaseRecovered {
		t.Error("the pool is still suspended - recovery must not claim success")
	}

	backUp := m.afterRemedy(remedyResult{
		Title:  "Ask ZFS to retry the failed I/O",
		Health: poolHealth{Pool: "NIXBACKUPS", State: poolOnline},
	})
	if backUp.recoverPhase != recoverPhaseRecovered {
		t.Errorf("the pool is online, expected recovered, got phase %v", backUp.recoverPhase)
	}
}

// Once the ladder is exhausted the screen says so instead of offering a step
// that does not exist.
func TestRecoveryStopsWhenTheLadderIsExhausted(t *testing.T) {
	m := recoverModel()
	m.recoverIndex = len(m.recoverRemedies) - 1

	m = m.afterRemedy(remedyResult{
		Title:  "Force the pool out and back in",
		Health: poolHealth{Pool: "NIXBACKUPS", State: poolSuspended},
	})

	if m.recoverPhase != recoverPhaseExhausted {
		t.Fatalf("expected exhausted, got phase %v", m.recoverPhase)
	}
	if !strings.Contains(m.recoverBody(), "cable") {
		t.Error("the exhausted screen should point at the hardware")
	}
	if strings.Contains(m.recoverHotkeys(), "enter run next step") {
		t.Errorf("no next step should be offered: %q", m.recoverHotkeys())
	}
}

// A timeout is a finding, not a crash: commands against a wedged pool can
// block in the kernel forever.
func TestRecoveryRecordsATimeoutAsAFinding(t *testing.T) {
	m := recoverModel()

	m = m.afterRemedy(remedyResult{
		Title:    "Ask ZFS to retry the failed I/O",
		TimedOut: true,
		Health:   poolHealth{Pool: "NIXBACKUPS", State: poolSuspended},
	})

	if !strings.Contains(m.recoverBody(), "wedged in the kernel") {
		t.Errorf("a timeout should be explained:\n%s", m.recoverBody())
	}
}
