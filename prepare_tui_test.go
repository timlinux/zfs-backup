// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// swapRunner replaces the global runner for one test.
func swapRunner(t *testing.T, r commandRunner) {
	t.Helper()
	previous := defaultRunner
	defaultRunner = r
	t.Cleanup(func() { defaultRunner = previous })
}

// abyssRunner answers lsblk and zpool status with the workstation fixture.
func abyssRunner() *fakeRunner {
	return &fakeRunner{
		respond: func(name string, _ []string) (string, error) {
			if name == "lsblk" {
				return abyssLsblk, nil
			}
			return abyssZpoolStatus, nil
		},
	}
}

// The last line of defence: performPrepare itself refuses a disk the system
// is using, regardless of how it was reached. Under the old code the exact
// call performPrepare("/dev/nvme0n1", ...) would have wiped the system disk.
func TestPerformPrepareRefusesTheSystemDisk(t *testing.T) {
	runner := abyssRunner()
	swapRunner(t, runner)

	_, err := performPrepare("/dev/nvme0n1", "NIXBACKUPS", "passphrase")

	if err == nil {
		t.Fatal("preparing the system disk must be refused")
	}
	if !strings.Contains(err.Error(), "refusing to prepare") {
		t.Errorf("the refusal should be explicit: %v", err)
	}
	for _, call := range runner.commandLines() {
		for _, banned := range []string{"wipefs", "sgdisk", "labelclear", "zpool create"} {
			if strings.Contains(call, banned) {
				t.Errorf("a refused disk must never see %q: %s", banned, call)
			}
		}
	}
}

func TestPerformPrepareRefusesADiskThatIsNotAttached(t *testing.T) {
	swapRunner(t, abyssRunner())

	if _, err := performPrepare("/dev/sdz", "NIXBACKUPS", "passphrase"); err == nil {
		t.Fatal("a disk that does not exist must be refused, not wiped by name")
	}
}

func TestPerformPrepareRefusesWhenTheSystemCannotBeInspected(t *testing.T) {
	swapRunner(t, &fakeRunner{
		respond: func(string, []string) (string, error) {
			return "", strings.NewReader("").UnreadByte() // any error
		},
	})

	if _, err := performPrepare("/dev/sdb", "NIXBACKUPS", "passphrase"); err == nil {
		t.Fatal("not knowing what the system is using must refuse the wipe")
	}
}

// typedConfirmModel parks a model on the typed-confirm screen for prepare.
func typedConfirmModel() model {
	m := model{operation: "prepare", devicePath: "/dev/sdb", input: textinput.New(), passwordInput: textinput.New()}
	return m.startTypedConfirm("THIS DISK WILL BE ERASED",
		prepareConfirmDetail(deviceCandidate{Device: blockDevice{Name: "sdb", Type: "disk", Size: 2000398934016, Model: "Seagate Expansion"}}, "NIXBACKUPS"),
		wipeConfirmationWord("/dev/sdb"))
}

// The confirmation is the disk's own name. Everything else bounces.
func TestTypedConfirmRejectsEverythingButTheDeviceName(t *testing.T) {
	for _, wrong := range []string{"DESTROY", "yes", "y", "sda", "SDB", " "} {
		m := typedConfirmModel()
		m.input.SetValue(wrong)

		m, cmd := m.updateTypedConfirmScreen(tea.KeyMsg{Type: tea.KeyEnter})

		if m.state != stateTypedConfirm || cmd != nil {
			t.Errorf("%q must not pass the confirmation", wrong)
		}
		if m.typedConfirmError == "" {
			t.Errorf("rejecting %q should say why", wrong)
		}
		if m.input.Value() != "" {
			t.Errorf("the rejected answer must be cleared, not left to edit into place")
		}
	}
}

func TestTypedConfirmAcceptsTheDeviceNameAndAsksForThePassphrase(t *testing.T) {
	m := typedConfirmModel()
	m.input.SetValue("sdb")

	m, cmd := m.updateTypedConfirmScreen(tea.KeyMsg{Type: tea.KeyEnter})

	if m.state != statePassword {
		t.Errorf("prepare should move on to the passphrase, got state %v", m.state)
	}
	if cmd == nil {
		t.Error("focusing the passphrase input should blink")
	}
}

func TestTypedConfirmEscBacksOutCompletely(t *testing.T) {
	m := typedConfirmModel()
	m.input.SetValue("sd") // half-typed

	m, cmd := m.updateTypedConfirmScreen(tea.KeyMsg{Type: tea.KeyEsc})

	if m.state != stateMenu || cmd != nil {
		t.Error("esc must return to the menu without side effects")
	}
	if m.input.Value() != "" {
		t.Error("the half-typed word must not survive")
	}
}

// Force-backup shares the screen but demands DESTROY and continues into pool
// access rather than a passphrase.
func TestTypedConfirmForForceBackupContinuesToThePool(t *testing.T) {
	m := model{operation: "force-backup", destPool: "NIXBACKUPS", input: textinput.New(), passwordInput: textinput.New()}
	m = m.startTypedConfirm("BACKUP HISTORY WILL BE DELETED",
		forceBackupConfirmDetail("NIXBACKUPS"), destroyConfirmationWord)
	m.input.SetValue(destroyConfirmationWord)

	m, cmd := m.updateTypedConfirmScreen(tea.KeyMsg{Type: tea.KeyEnter})

	if m.state != stateMenu {
		t.Errorf("force-backup continues via poolReadyMsg from the menu state, got %v", m.state)
	}
	if cmd == nil {
		t.Error("confirming should start pool access")
	}
}

// The picker refuses to act on a vetoed disk even if the user presses enter.
func TestDevicePickerWillNotSelectAVetoedDisk(t *testing.T) {
	m := model{
		state:           stateDevicePick,
		operation:       "prepare",
		devicePickReady: true,
		input:           textinput.New(),
		deviceCandidates: []deviceCandidate{
			{Device: blockDevice{Name: "nvme0n1", Type: "disk"},
				Vetoes: []deviceVeto{{Reason: "this disk holds the running system (/)"}}},
			{Device: blockDevice{Name: "sdb", Type: "disk", Removable: true}},
		},
	}

	m, cmd := m.updateDevicePickScreen(tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != stateDevicePick || cmd != nil || m.devicePath != "" {
		t.Error("enter on a vetoed disk must do nothing")
	}

	m.deviceIndex = 1
	m, _ = m.updateDevicePickScreen(tea.KeyMsg{Type: tea.KeyEnter})
	if m.devicePath != "/dev/sdb" || m.state != stateInput {
		t.Errorf("enter on a clean disk should choose it and ask for the pool name, got %q state %v",
			m.devicePath, m.state)
	}
}

// The detail shown before wiping must name the disk, its size and the cost.
func TestPrepareConfirmDetailNamesWhatDies(t *testing.T) {
	detail := prepareConfirmDetail(deviceCandidate{
		Device: blockDevice{Name: "sdb", Type: "disk", Size: 2000398934016, Model: "Seagate Expansion"},
	}, "NIXBACKUPS")

	for _, want := range []string{"/dev/sdb", "Seagate Expansion", "erased", "NIXBACKUPS", "cannot be undone"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail is missing %q:\n%s", want, detail)
		}
	}
}

func TestForceBackupConfirmDetailNamesTheCost(t *testing.T) {
	detail := forceBackupConfirmDetail("NIXBACKUPS")

	for _, want := range []string{"NIXBACKUPS", "deleted", "Older\nversions", "lost"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail is missing %q:\n%s", want, detail)
		}
	}
}
