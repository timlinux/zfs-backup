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

// stateCleanup is the orphaned-snapshot cleanup screen, reached from the main
// menu or from the health check. It walks three phases in one screen rather
// than throwing the user into a popup: review the plan, type the confirmation,
// read the outcome.
const stateCleanup sessionState = 103

// cleanupPhase tracks where the user is within the cleanup screen.
type cleanupPhase int

const (
	// cleanupPhasePlan shows the vetted dry run. Nothing has been destroyed.
	cleanupPhasePlan cleanupPhase = iota
	// cleanupPhaseConfirm asks the user to type DESTROY.
	cleanupPhaseConfirm
	// cleanupPhaseRunning is destroying snapshots.
	cleanupPhaseRunning
	// cleanupPhaseDone reports what actually happened.
	cleanupPhaseDone
)

// destroyConfirmationWord is what the user must type to proceed. Identical to
// the word the CLI demands, so the two paths are equally hard to fat-finger.
const destroyConfirmationWord = "DESTROY"

// =============================================================================
// Messages and commands
// =============================================================================

// cleanupPlanMsg carries a vetted cleanup plan and its per-snapshot preview.
type cleanupPlanMsg struct {
	pool    string
	plan    *cleanupPlan
	preview []string
	err     error
}

// cleanupDoneMsg carries the outcome of an actual destroy run.
type cleanupDoneMsg struct {
	outcome cleanupOutcome
	usage   []datasetUsage
}

// loadCleanupPlan builds the plan and asks ZFS what each destroy would free.
// Read-only: this command never destroys anything.
func loadCleanupPlan(pool string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		plan, err := buildCleanupPlan(ctx, defaultRunner, pool, "")
		if err != nil {
			return cleanupPlanMsg{pool: pool, err: err}
		}
		return cleanupPlanMsg{
			pool:    pool,
			plan:    plan,
			preview: previewDestroy(ctx, defaultRunner, plan.Targets),
		}
	}
}

// performCleanup destroys the vetted targets and re-reads space usage so the
// screen can show what the cleanup actually bought.
func performCleanup(pool string, targets []string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		outcome := destroyPlannedSnapshots(ctx, defaultRunner, targets, nil)
		usage, err := listDatasetUsage(ctx, defaultRunner, pool)
		if err != nil {
			usage = nil
		}
		return cleanupDoneMsg{outcome: outcome, usage: usage}
	}
}

// =============================================================================
// Key handling
// =============================================================================

// updateCleanupScreen handles keys for the cleanup screen.
func (m model) updateCleanupScreen(msg tea.KeyMsg) (model, tea.Cmd) {
	// Destroying is not interruptible - a half-cancelled destroy loop is
	// harder to reason about than letting it finish - and that includes
	// ctrl+c, or the promise would be one keystroke deep.
	if m.cleanupPhase == cleanupPhaseRunning {
		return m, nil
	}
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	if m.cleanupPhase == cleanupPhaseConfirm {
		return m.updateCleanupConfirm(msg)
	}

	switch msg.String() {
	case "esc", "q":
		m.state = stateMenu
		m.cleanupReady = false
		m.cleanupMessage = ""
		return m, nil
	case "r":
		m.cleanupReady = false
		m.cleanupPhase = cleanupPhasePlan
		m.cleanupMessage = ""
		return m, tea.Batch(m.spinner.Tick, loadCleanupPlan(m.cleanupPool))
	case "d":
		if m.cleanupPhase != cleanupPhasePlan || m.cleanupPlan == nil {
			return m, nil
		}
		if len(m.cleanupPlan.Targets) == 0 {
			m.cleanupMessage = "Nothing is cleared for destruction, so there is nothing to confirm."
			return m, nil
		}
		m.cleanupPhase = cleanupPhaseConfirm
		m.cleanupMessage = ""
		// The confirm furniture (banner, prompt, input) costs ~8 rows below
		// the viewport, and what must be visible while DESTROY is typed is
		// the list of what dies - so swap in a compact body sized to fit.
		m.cleanupViewport = newShorterViewport(m.width, m.height, 4, confirmCleanupBody(m.cleanupPlan))
		m.input.SetValue("")
		m.input.Placeholder = ""
		m.input.Focus()
		return m, textinput.Blink
	default:
		var cmd tea.Cmd
		m.cleanupViewport, cmd = m.cleanupViewport.Update(msg)
		return m, cmd
	}
}

// updateCleanupConfirm handles the typed-confirmation phase.
func (m model) updateCleanupConfirm(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "up", "down", "pgup", "pgdown":
		// The list of what dies must stay reachable at the moment of
		// commitment - navigation scrolls the plan, it never types.
		var cmd tea.Cmd
		m.cleanupViewport, cmd = m.cleanupViewport.Update(msg)
		return m, cmd
	case "esc":
		m.cleanupPhase = cleanupPhasePlan
		m.cleanupMessage = "Aborted. Nothing was destroyed."
		m.cleanupViewport = newReportViewport(m.width, m.height, m.cleanupPlanBody)
		m.input.SetValue("")
		m.input.Blur()
		return m, nil
	case "enter":
		if strings.TrimSpace(m.input.Value()) != destroyConfirmationWord {
			m.cleanupMessage = fmt.Sprintf(
				"Type %s exactly to continue, or press esc to back out.", destroyConfirmationWord)
			m.input.SetValue("")
			return m, nil
		}
		m.input.Blur()
		m.input.SetValue("")
		m.cleanupPhase = cleanupPhaseRunning
		m.cleanupMessage = ""
		return m, tea.Batch(m.spinner.Tick, performCleanup(m.cleanupPool, m.cleanupPlan.Targets))
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// =============================================================================
// Rendering
// =============================================================================

// renderCleanupContent draws the cleanup screen for the current phase.
func (m model) renderCleanupContent(width int) string {
	if !m.cleanupReady {
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(m.spinner.View() + " Looking for orphaned snapshots...")
	}

	var b strings.Builder

	title := selectedItemStyle.Render(fmt.Sprintf("Clean Up Orphaned Snapshots: %s", m.cleanupPool))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(title))
	b.WriteString("\n\n")

	b.WriteString(m.cleanupViewport.View())
	b.WriteString("\n")

	centre := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	switch m.cleanupPhase {
	case cleanupPhasePlan:
		b.WriteString(centre.Render(m.renderCleanupPlanFooter()))
	case cleanupPhaseConfirm:
		if !m.cleanupViewport.AtBottom() {
			b.WriteString(centre.Render(mutedStyle.Render(fmt.Sprintf(
				"▼ more below | %d%%", int(m.cleanupViewport.ScrollPercent()*100)))))
			b.WriteString("\n")
		}
		b.WriteString(centre.Render(dangerBanner("DESTROYING SNAPSHOTS IS IRREVERSIBLE")))
		b.WriteString("\n")
		b.WriteString(centre.Render(warningStyle.Render(fmt.Sprintf(
			"Type %s to destroy %s on %s",
			destroyConfirmationWord, pluralise(len(m.cleanupPlan.Targets), "snapshot", "snapshots"), m.cleanupPool))))
		b.WriteString("\n")
		b.WriteString(centre.Render(m.input.View()))
	case cleanupPhaseRunning:
		b.WriteString(centre.Render(m.spinner.View() + " Destroying snapshots..."))
	case cleanupPhaseDone:
		b.WriteString(centre.Render(m.renderCleanupDoneFooter()))
	}
	b.WriteString("\n")

	if m.cleanupMessage != "" {
		b.WriteString(centre.Render(warningStyle.Render(m.cleanupMessage)))
		b.WriteString("\n")
	}

	return b.String()
}

// renderCleanupPlanFooter summarises the dry run and the key that acts on it.
func (m model) renderCleanupPlanFooter() string {
	if m.cleanupPlan == nil || len(m.cleanupPlan.Targets) == 0 {
		return statusStyle.Render("Nothing to clean up - this pool is healthy.")
	}
	return infoStyle.Render(fmt.Sprintf(
		"Dry run: %s, at least %s would be freed. Press d to destroy them.",
		pluralise(len(m.cleanupPlan.Targets), "snapshot", "snapshots"), formatSize(m.cleanupPlan.uniqueBytes())))
}

// renderCleanupDoneFooter summarises what the destroy run achieved.
func (m model) renderCleanupDoneFooter() string {
	destroyed := len(m.cleanupOutcome.Destroyed)
	if len(m.cleanupOutcome.Failures) > 0 {
		return errorStyle.Render(fmt.Sprintf(
			"Destroyed %s, %d failed - see the report above, then press r to re-scan.",
			pluralise(destroyed, "snapshot", "snapshots"), len(m.cleanupOutcome.Failures)))
	}
	return statusStyle.Render(fmt.Sprintf("Destroyed %s.", pluralise(destroyed, "snapshot", "snapshots")))
}

// buildCleanupPlanView renders the dry-run body shown in the viewport.
func buildCleanupPlanView(plan *cleanupPlan, preview []string) string {
	var b strings.Builder

	if notice := unsetScopeNotice(plan.Scan); notice != "" {
		b.WriteString(warningStyle.Render(notice))
		b.WriteString("\n")
	}
	b.WriteString(infoStyle.Render(describeScope(plan.Scan.Pool, plan.Scan.InScope, plan.Scan.Missing)))
	b.WriteString("\n")
	b.WriteString(infoStyle.Render("Datasets in scope are never touched by this screen."))
	b.WriteString("\n\n")

	if len(plan.Decisions) == 0 {
		b.WriteString(statusStyle.Render("No orphaned snapshots found. Nothing to clean up."))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(labelStyle.Render("What would be destroyed"))
	b.WriteString("\n\n")
	b.WriteString(renderCleanupPlan(plan))

	if len(plan.Targets) == 0 {
		b.WriteString("\n")
		b.WriteString(warningStyle.Render(
			"Every candidate was held back by a safety check. Nothing to do."))
		b.WriteString("\n")
		return b.String()
	}

	if len(preview) > 0 {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("ZFS dry run"))
		b.WriteString("\n\n")
		for _, line := range preview {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}

	b.WriteString("\n")
	b.WriteString(infoStyle.Render(reclaimCaveat))
	b.WriteString("\n")

	return b.String()
}

// buildCleanupResultView renders the outcome shown in the viewport after a run.
func buildCleanupResultView(pool string, outcome cleanupOutcome, usage []datasetUsage) string {
	var b strings.Builder

	b.WriteString(labelStyle.Render(fmt.Sprintf("Cleanup complete on %s", pool)))
	b.WriteString("\n\n")

	for _, name := range outcome.Destroyed {
		fmt.Fprintf(&b, "  %s %s\n", statusStyle.Render("destroyed"), name)
	}
	for _, f := range outcome.Failures {
		b.WriteString(errorStyle.Render("  failed: " + f))
		b.WriteString("\n")
	}

	if len(usage) > 0 {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Space after cleanup"))
		b.WriteString("\n\n")
		for _, u := range usage {
			fmt.Fprintf(&b, "  %-32s used %10s  snapshots %10s\n",
				u.Name, formatSize(u.Used), formatSize(u.UsedBySnapshots))
		}
	}

	return b.String()
}

// cleanupHotkeys returns the footer hotkeys for the current cleanup phase, so
// the destroy key is only advertised when it would actually do something.
func (m model) cleanupHotkeys() string {
	switch m.cleanupPhase {
	case cleanupPhaseConfirm:
		return fmt.Sprintf("scroll ↑/↓ • type %s • enter confirm • esc back", destroyConfirmationWord)
	case cleanupPhaseRunning:
		return "destroying - please wait"
	case cleanupPhaseDone:
		return "scroll up/down • r rescan • esc return"
	default:
		if m.cleanupPlan != nil && len(m.cleanupPlan.Targets) > 0 {
			return "scroll up/down • d destroy • r refresh • esc return"
		}
		return "scroll up/down • r refresh • esc return"
	}
}

// confirmCleanupBody is what the viewport shows while DESTROY is typed: the
// exact list of what dies, nothing else. The scope advisory and dry-run
// detail belong to the plan phase; at the moment of commitment the names are
// the only thing that matters.
func confirmCleanupBody(plan *cleanupPlan) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render(fmt.Sprintf(
		"These %s will be destroyed on %s:",
		pluralise(len(plan.Targets), "snapshot", "snapshots"), plan.Scan.Pool)))
	b.WriteString("\n\n")
	for _, name := range plan.Targets {
		b.WriteString("  " + name + "\n")
	}
	b.WriteString("\n")
	b.WriteString(infoStyle.Render(fmt.Sprintf(
		"At least %s is freed once they are gone.", formatSize(plan.uniqueBytes()))))
	b.WriteString("\n")
	return b.String()
}
