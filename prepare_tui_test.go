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

// esc steps BACK one stage in the prepare wizard - it must not throw away
// the disk selection and pool name and dump the user at the menu.
func TestTypedConfirmEscStepsBackToThePoolName(t *testing.T) {
	m := typedConfirmModel()
	m.destPool = "NIXBACKUPS"
	m.input.SetValue("sd") // half-typed

	m, _ = m.updateTypedConfirmScreen(tea.KeyMsg{Type: tea.KeyEsc})

	if m.state != stateInput || m.preparePhase != 1 {
		t.Errorf("esc should reopen the pool-name input, got state %v phase %d", m.state, m.preparePhase)
	}
	if m.input.Value() != "NIXBACKUPS" {
		t.Errorf("the chosen pool name should be preserved, got %q", m.input.Value())
	}
	if m.devicePath != "/dev/sdb" {
		t.Error("the disk selection must survive the back-step")
	}
}

// For force-backup there is no earlier wizard stage, so esc cancels cleanly.
func TestTypedConfirmEscCancelsForceBackup(t *testing.T) {
	m := model{operation: "force-backup", destPool: "NIXBACKUPS", input: textinput.New(), passwordInput: textinput.New()}
	m = m.startTypedConfirm("BACKUP HISTORY WILL BE DELETED",
		forceBackupConfirmDetail("NIXBACKUPS"), destroyConfirmationWord)
	m.input.SetValue("DESTRO")

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

	for _, want := range []string{"NIXBACKUPS", "deleted", "Older file\nversions", "lost"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail is missing %q:\n%s", want, detail)
		}
	}
}

// Blocker from review: the field must not display the word being demanded,
// or the confirmation reads as pre-filled.
func TestTypedConfirmPlaceholderNeverRevealsTheWord(t *testing.T) {
	m := typedConfirmModel()
	if m.input.Placeholder == m.typedConfirmWord || m.input.Placeholder == "sdb" {
		t.Errorf("placeholder %q reveals the confirmation word", m.input.Placeholder)
	}
}

// Blocker from review: enter on a refused disk must explain, not stay silent.
func TestDevicePickerExplainsARefusedDisk(t *testing.T) {
	m := model{
		state:           stateDevicePick,
		operation:       "prepare",
		devicePickReady: true,
		input:           textinput.New(),
		deviceCandidates: []deviceCandidate{
			{Device: blockDevice{Name: "nvme0n1", Type: "disk"},
				Vetoes: []deviceVeto{{Reason: "this disk holds the running system (/)"}}},
		},
	}

	m, _ = m.updateDevicePickScreen(tea.KeyMsg{Type: tea.KeyEnter})

	if !strings.Contains(m.devicePickNote, "cannot be erased") ||
		!strings.Contains(m.devicePickNote, "running system") {
		t.Errorf("expected an explanation, got %q", m.devicePickNote)
	}
	if !strings.Contains(m.renderDevicePickContent(100), "cannot be erased") {
		t.Error("the explanation must be rendered")
	}
}

// Blocker from review: refused disks carry a non-colour marker.
func TestDevicePickerMarksRefusedDisksWithoutColour(t *testing.T) {
	m := model{
		devicePickReady: true,
		deviceCandidates: []deviceCandidate{
			{Device: blockDevice{Name: "nvme0n1", Type: "disk"},
				Vetoes: []deviceVeto{{Reason: "mounted at /"}}},
			{Device: blockDevice{Name: "sdb", Type: "disk", Removable: true}},
		},
	}
	out := m.renderDevicePickContent(100)
	if !strings.Contains(out, "✗ /dev/nvme0n1") {
		t.Errorf("refused disk should carry the ✗ marker:\n%s", out)
	}
	if strings.Contains(out, "✗ /dev/sdb") {
		t.Error("a selectable disk must not be marked refused")
	}
}

// Blocker from review: the size column must sit at one offset for every row.
func TestDevicePickerColumnsAlign(t *testing.T) {
	m := model{
		devicePickReady: true,
		deviceCandidates: []deviceCandidate{
			{Device: blockDevice{Name: "nvme0n1", Type: "disk", Size: 1024209543168, Model: "Samsung SSD 990"},
				Vetoes: []deviceVeto{{Reason: "this disk holds the running system (/boot)"}}},
			{Device: blockDevice{Name: "sda", Type: "disk", Size: 4000787030016, Model: "WDC WD40EFRX"},
				Vetoes: []deviceVeto{{Reason: "mounted at /mnt/data"}}},
			{Device: blockDevice{Name: "sdb", Type: "disk", Size: 2000398934016, Model: "Seagate Expansion", Removable: true}},
		},
	}
	out := m.renderDevicePickContent(100)

	var offsets []int
	for _, line := range strings.Split(out, "\n") {
		if idx := strings.Index(line, "/dev/"); idx >= 0 {
			// Visible column, not byte offset - the cursor and marker are
			// multi-byte glyphs.
			offsets = append(offsets, lipglossWidth(line[:idx]))
		}
	}
	if len(offsets) != 3 {
		t.Fatalf("expected 3 device rows, found %d:\n%s", len(offsets), out)
	}
	for _, o := range offsets[1:] {
		if o != offsets[0] {
			t.Errorf("device paths not column-aligned (offsets %v):\n%s", offsets, out)
		}
	}
}

// Blocker from review: q killed the app from the pool-name and passphrase
// screens, making any value containing q untypeable.
func TestQIsAnOrdinaryCharacterInTextFields(t *testing.T) {
	base := model{width: 80, height: 24, input: textinput.New(), passwordInput: textinput.New()}

	inputM := base
	inputM.state = stateInput
	inputM.operation = "prepare"
	inputM.preparePhase = 1
	inputM.input.Focus()
	next, _ := inputM.Update(keyRunes("q"))
	m := next.(model)
	if m.quitting {
		t.Fatal("q must not quit from a text input")
	}
	if m.input.Value() != "q" {
		t.Errorf("q should be typed into the field, got %q", m.input.Value())
	}

	passM := base
	passM.state = statePassword
	passM.operation = "prepare"
	passM.passwordInput.Focus()
	next, _ = passM.Update(keyRunes("q"))
	m = next.(model)
	if m.quitting {
		t.Fatal("q must not quit from the passphrase field")
	}
	if m.passwordInput.Value() != "q" {
		t.Errorf("q should be typed into the passphrase, got %q", m.passwordInput.Value())
	}
}

// The banner is the loudest safety signal on the most dangerous screen; the
// whole confirmation must seat on an 80x24 terminal with the banner on it.
func TestDiskWipeConfirmFitsWithBannerVisible(t *testing.T) {
	m := typedConfirmModel()
	m.width, m.height, m.state = 80, 24, stateTypedConfirm

	view := m.View()
	lines := strings.Split(view, "\n")

	if len(lines) > 25 {
		t.Errorf("typed confirm emits %d lines on a 24-row terminal", len(lines))
	}
	if !strings.Contains(view, "THIS DISK WILL BE ERASED") {
		t.Errorf("the banner must be on screen:\n%s", view)
	}
	if !strings.Contains(view, "/dev/sdb") {
		t.Error("the disk identity must be on screen")
	}
}
