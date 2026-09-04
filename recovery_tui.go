// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// stateRecoverPool is the guided pool recovery screen: it diagnoses a pool
// that has stopped responding and works through the remedies itself.
const stateRecoverPool sessionState = 104

// recoverPhase tracks where the user is within the recovery screen.
type recoverPhase int

const (
	// recoverPhaseChecking is waiting on a health check.
	recoverPhaseChecking recoverPhase = iota
	// recoverPhaseReady is showing the diagnosis, with a remedy to offer.
	recoverPhaseReady
	// recoverPhaseConfirm is asking before a forceful remedy.
	recoverPhaseConfirm
	// recoverPhaseWorking is running a remedy.
	recoverPhaseWorking
	// recoverPhaseRecovered is the pool coming back.
	recoverPhaseRecovered
	// recoverPhaseExhausted is every remedy tried, pool still down.
	recoverPhaseExhausted
)

// =============================================================================
// Messages and commands
// =============================================================================

// poolHealthMsg carries the result of a health check.
type poolHealthMsg struct {
	health poolHealth
}

// remedyDoneMsg carries the result of one remedy.
type remedyDoneMsg struct {
	result remedyResult
}

// checkPool runs a health check for the recovery screen.
func checkPool(pool string) tea.Cmd {
	return func() tea.Msg {
		return poolHealthMsg{health: checkPoolHealth(context.Background(), defaultRunner, pool)}
	}
}

// runRemedy applies one remedy and re-checks the pool.
func runRemedy(pool string, remedy poolRemedy) tea.Cmd {
	return func() tea.Msg {
		return remedyDoneMsg{result: applyRemedy(context.Background(), defaultRunner, pool, remedy)}
	}
}

// =============================================================================
// Key handling
// =============================================================================

// updateRecoverPoolScreen handles keys for the guided recovery screen.
func (m model) updateRecoverPoolScreen(msg tea.KeyMsg) (model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	switch m.recoverPhase {
	case recoverPhaseWorking, recoverPhaseChecking:
		// A half-cancelled zpool command leaves a worse mess than waiting.
		return m, nil

	case recoverPhaseConfirm:
		switch msg.String() {
		case "y", "Y", "enter":
			remedy := m.recoverRemedies[m.recoverIndex]
			m.recoverPhase = recoverPhaseWorking
			m.recoverMessage = ""
			return m, tea.Batch(m.spinner.Tick, runRemedy(m.recoverPool, remedy))
		case "n", "N", "esc":
			m.recoverPhase = recoverPhaseReady
			m.recoverMessage = "Left alone. Nothing was changed."
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "esc", "q":
		m.state = stateMenu
		m.recoverReady = false
		m.recoverMessage = ""
		return m, nil

	case "r":
		m.recoverPhase = recoverPhaseChecking
		m.recoverMessage = ""
		return m, tea.Batch(m.spinner.Tick, checkPool(m.recoverPool))

	case "enter":
		if m.recoverPhase != recoverPhaseReady || m.recoverIndex >= len(m.recoverRemedies) {
			return m, nil
		}
		remedy := m.recoverRemedies[m.recoverIndex]
		if remedy.NeedsConfirm {
			m.recoverPhase = recoverPhaseConfirm
			m.recoverMessage = ""
			return m, nil
		}
		m.recoverPhase = recoverPhaseWorking
		m.recoverMessage = ""
		return m, tea.Batch(m.spinner.Tick, runRemedy(m.recoverPool, remedy))

	default:
		var cmd tea.Cmd
		m.recoverViewport, cmd = m.recoverViewport.Update(msg)
		return m, cmd
	}
}

// afterRemedy advances the ladder once a remedy has reported back.
func (m model) afterRemedy(result remedyResult) model {
	m.recoverLog = append(m.recoverLog, "")
	m.recoverLog = append(m.recoverLog, result.Title)
	m.recoverLog = append(m.recoverLog, result.Log...)

	if result.TimedOut {
		m.recoverLog = append(m.recoverLog,
			"  The command did not return. The pool is wedged in the kernel.")
	}

	m.recoverHealth = result.Health
	m.recoverIndex++

	switch {
	case result.Health.Usable():
		m.recoverPhase = recoverPhaseRecovered
	case m.recoverIndex >= len(m.recoverRemedies):
		m.recoverPhase = recoverPhaseExhausted
	default:
		m.recoverPhase = recoverPhaseReady
	}

	m.recoverViewport = newReportViewport(m.width, m.height, m.recoverBody())
	m.recoverReady = true
	return m
}

// =============================================================================
// Rendering
// =============================================================================

// recoverBody builds the scrollable part of the recovery screen.
func (m model) recoverBody() string {
	var b strings.Builder

	b.WriteString(labelStyle.Render("Diagnosis"))
	b.WriteString("\n\n  ")
	if m.recoverHealth.Usable() {
		b.WriteString(statusStyle.Render(m.recoverHealth.Summary()))
	} else {
		b.WriteString(warningStyle.Render(m.recoverHealth.Summary()))
	}
	b.WriteString("\n")

	if len(m.recoverLog) > 0 {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("What has been tried"))
		b.WriteString("\n")
		for _, line := range m.recoverLog {
			b.WriteString(line + "\n")
		}
	}

	switch m.recoverPhase {
	case recoverPhaseRecovered:
		b.WriteString("\n")
		b.WriteString(statusStyle.Render("The pool is responding again. You can back up now."))
		b.WriteString("\n")
	case recoverPhaseExhausted:
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("What is left to try"))
		b.WriteString("\n\n")
		b.WriteString(exhaustedGuidance)
		b.WriteString("\n")
	default:
		if m.recoverIndex < len(m.recoverRemedies) {
			next := m.recoverRemedies[m.recoverIndex]
			b.WriteString("\n")
			b.WriteString(labelStyle.Render(fmt.Sprintf("Next step %d of %d: %s",
				m.recoverIndex+1, len(m.recoverRemedies), next.Title)))
			b.WriteString("\n\n")
			b.WriteString(next.Explanation)
			b.WriteString("\n\n")
			for _, command := range next.Commands {
				b.WriteString(infoStyle.Render("  " + strings.Join(command, " ")))
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}

// renderRecoverPoolContent draws the guided recovery screen.
func (m model) renderRecoverPoolContent(width int) string {
	centre := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	if !m.recoverReady {
		return centre.Render(m.spinner.View() +
			fmt.Sprintf(" Checking %s... (up to %s if the pool is wedged)",
				m.recoverPool, poolCommandTimeout))
	}

	var b strings.Builder
	b.WriteString(centre.Render(selectedItemStyle.Render("Pool Recovery: " + m.recoverPool)))
	b.WriteString("\n\n")
	b.WriteString(m.recoverViewport.View())
	b.WriteString("\n")

	switch m.recoverPhase {
	case recoverPhaseChecking:
		b.WriteString(centre.Render(m.spinner.View() + " Re-checking the pool..."))
	case recoverPhaseWorking:
		b.WriteString(centre.Render(m.spinner.View() + " Working on the pool - this can take a moment..."))
	case recoverPhaseConfirm:
		b.WriteString(centre.Render(warningStyle.Render(
			m.recoverRemedies[m.recoverIndex].Title + " - proceed? (y/n)")))
	case recoverPhaseRecovered:
		b.WriteString(centre.Render(statusStyle.Render("Recovered. Press esc to return to the menu.")))
	case recoverPhaseExhausted:
		b.WriteString(centre.Render(warningStyle.Render(
			"zfs-backup cannot take this further - see the notes above.")))
	default:
		b.WriteString(centre.Render(warningStyle.Render("Press enter to run the next step.")))
	}
	b.WriteString("\n")

	if m.recoverMessage != "" {
		b.WriteString(centre.Render(infoStyle.Render(m.recoverMessage)))
		b.WriteString("\n")
	}

	return b.String()
}

// recoverHotkeys returns the footer hotkeys for the current recovery phase.
func (m model) recoverHotkeys() string {
	switch m.recoverPhase {
	case recoverPhaseConfirm:
		return "y proceed • n cancel • esc back"
	case recoverPhaseWorking, recoverPhaseChecking:
		return "working - please wait"
	case recoverPhaseRecovered, recoverPhaseExhausted:
		return "scroll up/down • r re-check • esc return"
	default:
		return "enter run next step • scroll up/down • r re-check • esc return"
	}
}

// startPoolRecovery parks the model on a fresh recovery run for a pool.
func (m model) startPoolRecovery(pool string) model {
	m.state = stateRecoverPool
	m.recoverPool = pool
	m.recoverRemedies = poolRemedies(pool)
	m.recoverIndex = 0
	m.recoverLog = nil
	m.recoverPhase = recoverPhaseChecking
	m.recoverMessage = ""
	m.recoverReady = false
	m.recoverHealth = poolHealth{Pool: pool, State: poolStateUnknown}
	return m
}

// recoverablePoolFromError names the pool a failure is about, when the failure
// is one the recovery screen can actually work on. Returns "" otherwise, so
// the offer only appears where it means something.
func recoverablePoolFromError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if !suspendedPoolPattern.MatchString(text) {
		return ""
	}
	if m := quotedNamePattern.FindStringSubmatch(text); len(m) > 1 {
		return strings.SplitN(m[1], "/", 2)[0]
	}
	return ""
}
