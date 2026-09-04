// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// stateDevicePick is the vetted disk picker for "Prepare Backup Device". The
// user chooses from what is actually attached; disks the system is using are
// shown, but refused, with the reason on the line.
const stateDevicePick sessionState = 105

// stateTypedConfirm is the shared typed-confirmation screen for the two
// destructive flows. A single keystroke must never wipe a disk or delete
// backup history; here the user types the name of what they are giving up.
const stateTypedConfirm sessionState = 106

// =============================================================================
// Device picker
// =============================================================================

// devicesLoadedMsg carries the vetted disk list.
type devicesLoadedMsg struct {
	candidates []deviceCandidate
	err        error
}

// loadDeviceCandidates inspects the attached disks. Read-only.
func loadDeviceCandidates() tea.Cmd {
	return func() tea.Msg {
		candidates, err := collectDeviceCandidates(context.Background(), defaultRunner)
		return devicesLoadedMsg{candidates: candidates, err: err}
	}
}

// updateDevicePickScreen handles keys for the disk picker.
func (m model) updateDevicePickScreen(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "q":
		m.state = stateMenu
		m.devicePickReady = false
		return m, nil
	case "r":
		m.devicePickReady = false
		return m, tea.Batch(m.spinner.Tick, loadDeviceCandidates())
	case "up", "k":
		if m.deviceIndex > 0 {
			m.deviceIndex--
		}
		return m, nil
	case "down", "j":
		if m.deviceIndex < len(m.deviceCandidates)-1 {
			m.deviceIndex++
		}
		return m, nil
	case "enter":
		if m.deviceIndex >= len(m.deviceCandidates) {
			return m, nil
		}
		candidate := m.deviceCandidates[m.deviceIndex]
		if !candidate.Selectable() {
			// The refusal is the feature: the reason is already on the line.
			return m, nil
		}
		m.devicePath = candidate.Device.Path()
		// On to the pool name, then the typed confirmation.
		m.state = stateInput
		m.preparePhase = 1
		m.input.Placeholder = "NIXBACKUPS"
		m.input.SetValue("NIXBACKUPS")
		m.input.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// renderDevicePickContent draws the disk picker.
func (m model) renderDevicePickContent(width int) string {
	centre := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	if !m.devicePickReady {
		return centre.Render(m.spinner.View() + " Inspecting attached disks...")
	}

	var b strings.Builder
	b.WriteString(centre.Render(selectedItemStyle.Render("Choose the disk to prepare")))
	b.WriteString("\n\n")
	b.WriteString(centre.Render(warningStyle.Render(
		"The chosen disk will be completely erased.")))
	b.WriteString("\n")
	b.WriteString(centre.Render(subtitleStyle.Render(
		"Disks the system is using are listed but cannot be chosen.")))
	b.WriteString("\n\n")

	if len(m.deviceCandidates) == 0 {
		b.WriteString(centre.Render(warningStyle.Render(
			"No disks found. Attach the backup drive and press r.")))
		b.WriteString("\n")
		return b.String()
	}

	for i, candidate := range m.deviceCandidates {
		cursor := "  "
		if i == m.deviceIndex {
			cursor = "> "
		}
		line := cursor + candidate.Describe()
		switch {
		case !candidate.Selectable():
			line = subtitleStyle.Render(line)
		case i == m.deviceIndex:
			line = selectedItemStyle.Render(line)
		default:
			line = infoStyle.Render(line)
		}
		b.WriteString(centre.Render(line))
		b.WriteString("\n")
	}

	return b.String()
}

// =============================================================================
// Typed confirmation
// =============================================================================

// startTypedConfirm parks the model on the typed-confirmation screen.
func (m model) startTypedConfirm(title, detail, word string) model {
	m.state = stateTypedConfirm
	m.typedConfirmTitle = title
	m.typedConfirmDetail = detail
	m.typedConfirmWord = word
	m.typedConfirmError = ""
	m.input.SetValue("")
	m.input.Placeholder = word
	m.input.Focus()
	return m
}

// updateTypedConfirmScreen handles the typed-confirmation keys.
func (m model) updateTypedConfirmScreen(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.state = stateMenu
		m.input.Blur()
		m.input.SetValue("")
		return m, nil
	case "enter":
		if strings.TrimSpace(m.input.Value()) != m.typedConfirmWord {
			m.typedConfirmError = fmt.Sprintf(
				"Type %s exactly to continue, or press esc to back out.", m.typedConfirmWord)
			m.input.SetValue("")
			return m, nil
		}
		m.input.Blur()
		m.input.SetValue("")
		return m.afterTypedConfirm()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// afterTypedConfirm routes a successful confirmation onward.
func (m model) afterTypedConfirm() (model, tea.Cmd) {
	switch m.operation {
	case "prepare":
		// The passphrase for the new encrypted pool is the last thing needed.
		m.state = statePassword
		m.passwordInput.SetValue("")
		m.passwordInput.Focus()
		return m, textinput.Blink
	case "force-backup":
		m.state = stateMenu
		return m, m.preparePoolAccess(m.destPool)
	}
	m.state = stateMenu
	return m, nil
}

// renderTypedConfirmContent draws the typed-confirmation screen.
func (m model) renderTypedConfirmContent(width int) string {
	centre := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	var b strings.Builder
	b.WriteString(centre.Render(destructiveWarningStyle.Render("  " + m.typedConfirmTitle + "  ")))
	b.WriteString("\n\n")
	b.WriteString(centre.Render(reportBoxStyle.Render(m.typedConfirmDetail)))
	b.WriteString("\n\n")
	b.WriteString(centre.Render(warningStyle.Render(
		fmt.Sprintf("Type %s and press enter to continue.", m.typedConfirmWord))))
	b.WriteString("\n")
	b.WriteString(centre.Render(m.input.View()))
	b.WriteString("\n")
	if m.typedConfirmError != "" {
		b.WriteString(centre.Render(errorStyle.Render(m.typedConfirmError)))
		b.WriteString("\n")
	}
	return b.String()
}

// prepareConfirmDetail describes exactly what preparing a disk will do.
func prepareConfirmDetail(candidate deviceCandidate, poolName string) string {
	model := strings.TrimSpace(candidate.Device.Model)
	if model == "" {
		model = "unknown model"
	}
	return fmt.Sprintf(
		"Disk:  %s\nSize:  %s\nModel: %s\n\n"+
			"Everything on this disk will be erased and replaced by the\n"+
			"encrypted ZFS pool %q. This cannot be undone.",
		candidate.Device.Path(), formatSize(candidate.Device.Size), model, poolName)
}

// forceBackupConfirmDetail describes what a force backup deletes.
func forceBackupConfirmDetail(destPool string) string {
	return fmt.Sprintf(
		"Pool:  %s\n\n"+
			"Every existing backup snapshot on %s will be deleted and the\n"+
			"backup rebuilt from the current state of the source. Older\n"+
			"versions of your files, held only by those snapshots, are lost.\n\n"+
			"Use this only when the incremental chain is broken - the\n"+
			"ordinary backup never needs it.",
		destPool, destPool)
}
