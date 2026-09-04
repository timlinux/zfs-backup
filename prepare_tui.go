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
		m.devicePickNote = ""
		return m, nil
	case "down", "j":
		if m.deviceIndex < len(m.deviceCandidates)-1 {
			m.deviceIndex++
		}
		m.devicePickNote = ""
		return m, nil
	case "enter":
		if m.deviceIndex >= len(m.deviceCandidates) {
			return m, nil
		}
		candidate := m.deviceCandidates[m.deviceIndex]
		if !candidate.Selectable() {
			// Silence on enter is a dead end; say why this disk is refused.
			m.devicePickNote = fmt.Sprintf("%s cannot be erased: %s.",
				candidate.Device.Path(), candidate.Vetoes[0].Reason)
			return m, nil
		}
		m.devicePickNote = ""
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

// devicePickBlockWidth is the fixed inner width of the picker list. The list
// is laid out left-aligned inside this block and the block is centred as a
// whole - centring row by row would put the size column at a different
// offset on every line, on the one screen where reading the right row is
// everything.
const devicePickBlockWidth = 66

// renderDevicePickContent draws the disk picker.
func (m model) renderDevicePickContent(width int) string {
	centre := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	if !m.devicePickReady {
		return centre.Render(m.spinner.View() + " Inspecting attached disks...")
	}

	var b strings.Builder
	b.WriteString(selectedItemStyle.Render("Choose the disk to prepare"))
	b.WriteString("\n\n")
	b.WriteString(warningStyle.Render("The chosen disk will be completely erased."))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Disks marked ✗ are in use and cannot be chosen."))
	b.WriteString("\n\n")

	if len(m.deviceCandidates) == 0 {
		b.WriteString(warningStyle.Render("No disks found. Attach the backup drive and press r."))
		b.WriteString("\n")
	}

	for i, candidate := range m.deviceCandidates {
		marker := "  "
		if !candidate.Selectable() {
			marker = "✗ "
		}
		cursor := "  "
		if i == m.deviceIndex {
			cursor = "▌ "
		}

		model := strings.TrimSpace(candidate.Device.Model)
		if model == "" {
			model = "unknown model"
		}
		kind := "internal"
		if candidate.Device.Removable {
			kind = "removable"
		}
		line := fmt.Sprintf("%s%s%-14s %9s  %s (%s)",
			cursor, marker, candidate.Device.Path(),
			formatSize(candidate.Device.Size), model, kind)

		switch {
		case !candidate.Selectable():
			b.WriteString(mutedStyle.Render(line))
		case i == m.deviceIndex:
			b.WriteString(selectedItemStyle.Render(line))
		default:
			b.WriteString(infoStyle.Render(line))
		}
		b.WriteString("\n")

		// The refusal reason hangs indented under its own disk, wrapped to
		// the block, so it can never detach from the row it belongs to.
		for _, veto := range candidate.Vetoes {
			reason := lipgloss.NewStyle().Width(devicePickBlockWidth - 8).
				Render(veto.Reason)
			for _, rl := range strings.Split(reason, "\n") {
				b.WriteString(mutedStyle.Render("        " + rl))
				b.WriteString("\n")
			}
		}
	}

	if m.devicePickNote != "" {
		b.WriteString("\n")
		b.WriteString(warningStyle.Render(m.devicePickNote))
		b.WriteString("\n")
	}

	block := lipgloss.NewStyle().Width(devicePickBlockWidth).Render(b.String())
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, block)
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
	// The instruction line names the word; the field must not also show it,
	// or the confirmation reads as pre-filled and becomes "press enter twice".
	m.input.Placeholder = ""
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
		m.input.Blur()
		m.input.SetValue("")
		if m.operation == "prepare" {
			// Back one stage: re-open the pool-name input with the chosen
			// name still in place.
			m.state = stateInput
			m.preparePhase = 1
			m.input.Placeholder = "NIXBACKUPS"
			m.input.SetValue(m.destPool)
			m.input.Focus()
			return m, textinput.Blink
		}
		m.state = stateMenu
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

// renderTypedConfirmContent draws the typed-confirmation screen. The chrome
// is deliberately flat: the default warning and report boxes spend six rows
// each on borders, padding and margins, which pushed the banner - the
// loudest safety signal on the most dangerous screen - off an 80x24
// terminal.
func (m model) renderTypedConfirmContent(width int) string {
	centre := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	banner := dangerBanner(m.typedConfirmTitle)
	detailBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAlert).
		Padding(0, 2).
		Render(m.typedConfirmDetail)

	var b strings.Builder
	b.WriteString(centre.Render(banner))
	b.WriteString("\n")
	b.WriteString(centre.Render(detailBox))
	b.WriteString("\n")
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
			"Every existing backup snapshot on %s will be deleted and\n"+
			"the backup rebuilt from the current source state. Older file\n"+
			"versions held only by those snapshots are lost. Use this only\n"+
			"when the incremental chain is broken - the ordinary backup\n"+
			"never needs it.",
		destPool, destPool)
}

// dangerBanner renders the one-line destructive warning. One row, high
// contrast - the boxed variant spent six rows on chrome and pushed the
// content it was warning about off small terminals.
func dangerBanner(text string) string {
	return lipgloss.NewStyle().
		// Dark text, not white: white on #D9695C is 3.43:1, below AA.
		Foreground(colorBackground).
		Background(colorAlert).
		Bold(true).
		Padding(0, 2).
		Render(text)
}
