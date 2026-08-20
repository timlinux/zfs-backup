// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Screens that live outside the original sessionState enum, following the
// existing statePoolSelect convention.
const (
	// stateScope is the backup scope editor: which datasets are backed up.
	stateScope sessionState = 101
	// stateDoctor is the read-only health report.
	stateDoctor sessionState = 102
)

// =============================================================================
// Backup scope editor
// =============================================================================

// scopeLoadedMsg carries the datasets of a pool and the current selection.
type scopeLoadedMsg struct {
	pool     string
	datasets []string
	selected map[string]bool
	missing  []string
	err      error
}

// scopeSavedMsg reports the outcome of saving the scope.
type scopeSavedMsg struct {
	err error
}

// loadBackupScope reads the pool's datasets and the saved scope.
func loadBackupScope(pool string) tea.Cmd {
	return func() tea.Msg {
		datasets, err := getChildDatasets(pool)
		if err != nil {
			return scopeLoadedMsg{pool: pool, err: err}
		}
		selected, missing, err := resolveBackupDatasets(pool)
		if err != nil {
			return scopeLoadedMsg{pool: pool, err: err}
		}

		chosen := make(map[string]bool, len(selected))
		for _, ds := range selected {
			chosen[ds] = true
		}
		return scopeLoadedMsg{pool: pool, datasets: datasets, selected: chosen, missing: missing}
	}
}

// saveBackupScope persists the selection. Selecting every dataset clears the
// restriction, so a pool that gains a dataset later still backs it up.
func saveBackupScope(pool string, datasets []string, selected map[string]bool) tea.Cmd {
	return func() tea.Msg {
		var chosen []string
		for _, ds := range datasets {
			if selected[ds] {
				chosen = append(chosen, ds)
			}
		}
		if len(chosen) == len(datasets) {
			chosen = nil // no restriction
		}
		return scopeSavedMsg{err: SetPoolScope(pool, chosen)}
	}
}

// updateScopeScreen handles keys for the backup scope editor.
func (m model) updateScopeScreen(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "q":
		m.state = stateMenu
		m.scopeMessage = ""
		return m, nil
	case "up", "k":
		if m.scopeIndex > 0 {
			m.scopeIndex--
		}
		return m, nil
	case "down", "j":
		if m.scopeIndex < len(m.scopeDatasets)-1 {
			m.scopeIndex++
		}
		return m, nil
	case " ", "x":
		if len(m.scopeDatasets) > 0 {
			ds := m.scopeDatasets[m.scopeIndex]
			m.scopeSelected[ds] = !m.scopeSelected[ds]
			m.scopeMessage = ""
		}
		return m, nil
	case "a":
		for _, ds := range m.scopeDatasets {
			m.scopeSelected[ds] = true
		}
		m.scopeMessage = ""
		return m, nil
	case "n":
		for _, ds := range m.scopeDatasets {
			m.scopeSelected[ds] = false
		}
		m.scopeMessage = ""
		return m, nil
	case "enter":
		chosen := 0
		for _, ds := range m.scopeDatasets {
			if m.scopeSelected[ds] {
				chosen++
			}
		}
		if chosen == 0 {
			m.scopeMessage = "Select at least one dataset - a backup of nothing is not a backup."
			return m, nil
		}
		return m, saveBackupScope(m.scopePool, m.scopeDatasets, m.scopeSelected)
	}
	return m, nil
}

// renderScopeContent draws the backup scope editor.
func (m model) renderScopeContent(width int) string {
	var b strings.Builder

	title := selectedItemStyle.Render(fmt.Sprintf("Backup Scope: %s", m.scopePool))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(title))
	b.WriteString("\n\n")

	b.WriteString(subtitleStyle.Render(
		"  Only the datasets you tick are snapshotted, replicated and pruned."))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(
		"  Everything else is left completely untouched."))
	b.WriteString("\n\n")

	if len(m.scopeDatasets) == 0 {
		b.WriteString(warningStyle.Render("  No datasets found on this pool."))
		b.WriteString("\n")
	}

	for i, ds := range m.scopeDatasets {
		cursor := "  "
		if i == m.scopeIndex {
			cursor = "> "
		}
		box := "[ ]"
		line := ds
		if m.scopeSelected[ds] {
			box = "[x]"
		}
		rendered := fmt.Sprintf("%s%s %s", cursor, box, line)
		if i == m.scopeIndex {
			b.WriteString(selectedItemStyle.Render(rendered))
		} else if m.scopeSelected[ds] {
			b.WriteString(statusStyle.Render(rendered))
		} else {
			b.WriteString(infoStyle.Render(rendered))
		}
		b.WriteString("\n")
	}

	for _, ds := range m.scopeMissing {
		b.WriteString(warningStyle.Render(
			fmt.Sprintf("  !   %s (configured but no longer exists)", ds)))
		b.WriteString("\n")
	}

	if m.scopeMessage != "" {
		b.WriteString("\n")
		b.WriteString(warningStyle.Render("  " + m.scopeMessage))
		b.WriteString("\n")
	}

	return b.String()
}

// =============================================================================
// Health check screen
// =============================================================================

// doctorLoadedMsg carries a rendered health report.
type doctorLoadedMsg struct {
	pool     string
	content  string
	problems int
	err      error
}

// loadDoctorReport runs the read-only health check for a pool.
func loadDoctorReport(pool string) tea.Cmd {
	return func() tea.Msg {
		scan, err := collectOrphanScan(context.Background(), defaultRunner, pool)
		if err != nil {
			return doctorLoadedMsg{pool: pool, err: err}
		}
		content, problems := renderDoctorReport(scan)
		return doctorLoadedMsg{pool: pool, content: content, problems: problems}
	}
}

// updateDoctorScreen handles keys for the health report screen.
func (m model) updateDoctorScreen(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "q":
		m.state = stateMenu
		m.doctorReady = false
		return m, nil
	case "r":
		m.doctorReady = false
		return m, loadDoctorReport(m.doctorPool)
	default:
		var cmd tea.Cmd
		m.doctorViewport, cmd = m.doctorViewport.Update(msg)
		return m, cmd
	}
}

// renderDoctorContent draws the health report screen.
func (m model) renderDoctorContent(width int) string {
	if !m.doctorReady {
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(m.spinner.View() + " Checking backup health...")
	}

	var b strings.Builder

	title := selectedItemStyle.Render(fmt.Sprintf("Backup Health: %s", m.doctorPool))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(title))
	b.WriteString("\n\n")

	b.WriteString(m.doctorViewport.View())
	b.WriteString("\n")

	verdict := statusStyle.Render("Healthy - nothing to clean up")
	if m.doctorProblems > 0 {
		verdict = warningStyle.Render(fmt.Sprintf(
			"%d issue group(s) - run 'sudo zfs-backup cleanup-orphans' to reclaim space",
			m.doctorProblems))
	}
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(verdict))
	b.WriteString("\n")

	scrollInfo := subtitleStyle.Render(fmt.Sprintf(
		"Scroll: j/k or arrows | %d%% | r refresh | esc/q to return",
		int(m.doctorViewport.ScrollPercent()*100)))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(scrollInfo))

	return b.String()
}

// newReportViewport builds a viewport sized to the current terminal.
func newReportViewport(width, height int, content string) viewport.Model {
	viewportHeight := height - 14
	if viewportHeight < 5 {
		viewportHeight = 5
	}
	viewportWidth := width - 8
	if viewportWidth < 40 {
		viewportWidth = 40
	}

	vp := viewport.New(viewportWidth, viewportHeight)
	vp.SetContent(content)
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorHighlight2).
		Padding(0, 1)
	return vp
}
