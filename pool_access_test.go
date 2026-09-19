// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// preparePoolAccess used to import a missing pool with a bare exec.Command,
// no timeout, no fake-ability - a wedged or absent pool left the caller
// stuck on "Initializing..." forever with no error and no way out. It now
// goes through defaultRunner like every other command, so these run
// instantly against a fake instead of needing a real (or hung) zpool.

func TestPreparePoolAccessSkipsImportWhenAlreadyImported(t *testing.T) {
	runner := &fakeRunner{respond: func(name string, args []string) (string, error) {
		if name == "zpool" && len(args) > 0 && args[0] == "list" {
			return "NIXBACKUP\n", nil
		}
		return "", nil
	}}
	swapRunner(t, runner)

	msg := emptyModel().preparePoolAccess("NIXBACKUP")().(poolReadyMsg)

	if msg.err != nil {
		t.Fatalf("an already-imported pool should not error, got %v", msg.err)
	}
	if runner.ran("import") {
		t.Error("an already-imported pool must not be re-imported")
	}
}

func TestPreparePoolAccessImportsAMissingPool(t *testing.T) {
	runner := &fakeRunner{respond: func(name string, args []string) (string, error) {
		if name == "zpool" && len(args) > 0 && args[0] == "list" {
			return "NIXROOT\n", nil // NIXBACKUP is not in the list
		}
		return "", nil // import succeeds
	}}
	swapRunner(t, runner)

	msg := emptyModel().preparePoolAccess("NIXBACKUP")().(poolReadyMsg)

	if msg.err != nil {
		t.Fatalf("a successful import should not error, got %v", msg.err)
	}
	if !runner.ran("zpool", "import", "NIXBACKUP") {
		t.Errorf("expected an import of NIXBACKUP, got %v", runner.commandLines())
	}
}

// The regression: a pool that cannot be imported must fail promptly with a
// clear error, not hang. Because the import now runs through defaultRunner,
// this test completes instantly instead of needing a real hung command.
func TestPreparePoolAccessFailsFastWhenImportIsRefused(t *testing.T) {
	runner := &fakeRunner{respond: func(name string, args []string) (string, error) {
		if name == "zpool" && len(args) > 0 && args[0] == "list" {
			return "NIXROOT\n", nil // NIXBACKUP is not in the list
		}
		return "", errors.New("cannot import 'NIXBACKUP': no such pool available")
	}}
	swapRunner(t, runner)

	msg := emptyModel().preparePoolAccess("NIXBACKUP")().(poolReadyMsg)

	if msg.err == nil {
		t.Fatal("a refused import must be reported as an error, not silently succeed")
	}
	if !strings.Contains(msg.err.Error(), "import") {
		t.Errorf("the error should say the import failed, got %q", msg.err)
	}
}

// emptyModel returns a zero-value model - preparePoolAccess is a plain method that
// does not read any model state, so an empty one is all these tests need.
func emptyModel() model { return model{} }

// The real bug behind "perpetually initialising with no feedback": submitting
// the passphrase for an operation that reads pool contents directly -
// cleanup, doctor, scope - fell through to startOperation(), which has no
// case for any of them. That set state to stateRunning and issued a Cmd
// batch with no actual work Cmd in it (just a spinner and a progress
// listener on a channel nobody ever writes to) - the screen span forever.
// These prove the password handler now routes each to its own unlock-and-
// load step instead, so a real message (even an error, since there is no
// real zfs pool in this test) comes back and state never gets stuck on
// stateRunning.
func TestPasswordSubmitRoutesPoolReadingOperationsToTheirOwnLoader(t *testing.T) {
	cases := []struct {
		operation string
		wantMsg   string
	}{
		{"cleanup", "main.cleanupPlanMsg"},
		{"doctor", "main.doctorLoadedMsg"},
		{"scope", "main.scopeLoadedMsg"},
	}

	for _, tc := range cases {
		t.Run(tc.operation, func(t *testing.T) {
			input := textinput.New()
			input.SetValue("hunter2")
			m := model{
				state:         statePassword,
				operation:     tc.operation,
				passwordInput: input,
				cleanupPool:   "NIXBACKUP",
				doctorPool:    "NIXBACKUP",
				scopePool:     "NIXBACKUP",
			}

			newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			nm := newModel.(model)

			if nm.state == stateRunning {
				t.Fatalf("operation %q must not fall through to the generic running screen "+
					"(startOperation has no case for it, so it would spin forever)", tc.operation)
			}
			if cmd == nil {
				t.Fatalf("operation %q should have issued its own unlock-and-load command", tc.operation)
			}

			msg := cmd()
			gotType := fmt.Sprintf("%T", msg)
			if gotType != tc.wantMsg {
				t.Errorf("operation %q produced %s, want %s (the dedicated loader)",
					tc.operation, gotType, tc.wantMsg)
			}
		})
	}
}

// Reported bug: choosing "Clean Up Orphaned Snapshots" landed on a screen
// titled "Backup in Progress" (wrong - it isn't a backup) with a status line
// reading "Running: cleanup" (an internal code, not what was chosen). Both
// must now name the actual menu item.
func TestRunningScreenNamesTheChosenOperationNotABareCode(t *testing.T) {
	m := model{state: stateRunning, operation: "cleanup", width: 80, height: 24}

	if got := m.getStatusText(); got != "Running: Clean Up Orphaned Snapshots" {
		t.Errorf("status text = %q, want it to name the chosen menu item", got)
	}

	body := m.renderRunningContent(80)
	if strings.Contains(body, "Backup in Progress") {
		t.Error("a cleanup must not be labelled as a backup")
	}
	if !strings.Contains(body, "Clean Up Orphaned Snapshots") {
		t.Errorf("the progress screen should name the operation, got:\n%s", body)
	}
}
