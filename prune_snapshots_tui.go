// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// statePruneSnapshots is the source-retention prune screen, reached from the
// main menu. It mirrors stateCleanup's three-phase flow (plan, confirm,
// running, done) and reuses its cleanupPhase enum and destroyConfirmationWord
// - the shape of "review a dry run, type DESTROY, watch it happen, read the
// outcome" is identical, only what is being pruned differs.
const statePruneSnapshots sessionState = 108

// =============================================================================
// Messages and commands
// =============================================================================

// sourcePrunePlanMsg carries a vetted source-retention plan and its
// per-snapshot preview.
type sourcePrunePlanMsg struct {
	pool    string
	plan    *sourcePrunePlan
	preview []string
	err     error
}

// sourcePruneDoneMsg carries the outcome of an actual prune run.
type sourcePruneDoneMsg struct {
	outcome cleanupOutcome
	usage   []datasetUsage
}

// sourcePruneDestroyProgressMsg reports one snapshot pruned mid-run.
type sourcePruneDestroyProgressMsg struct{}

// listenSourcePruneDestroyProgress relays one progress tick from the prune run.
func listenSourcePruneDestroyProgress(progress chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-progress; !ok {
			return nil
		}
		return sourcePruneDestroyProgressMsg{}
	}
}

// loadSourcePrunePlan builds the plan and asks ZFS what each destroy would
// free. Read-only: this command never destroys anything.
func loadSourcePrunePlan(pool string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		plan, err := buildSourcePrunePlan(ctx, defaultRunner, pool, time.Now(), defaultRetentionPolicy)
		if err != nil {
			return sourcePrunePlanMsg{pool: pool, err: err}
		}
		names := make([]string, len(plan.Targets))
		for i, t := range plan.Targets {
			names[i] = t.Name
		}
		return sourcePrunePlanMsg{
			pool:    pool,
			plan:    plan,
			preview: previewDestroy(ctx, defaultRunner, names),
		}
	}
}

// performSourcePrune destroys the vetted targets, reporting each one through
// the progress channel so the running view can count along, then re-reads
// space usage so the screen can show what the prune actually bought.
func performSourcePrune(pool string, targets []sourcePruneCandidate, progress chan struct{}) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		outcome := destroySourcePruneCandidates(ctx, defaultRunner, targets,
			func(string) { progress <- struct{}{} })
		close(progress)
		usage, err := listDatasetUsage(ctx, defaultRunner, pool)
		if err != nil {
			usage = nil
		}
		return sourcePruneDoneMsg{outcome: outcome, usage: usage}
	}
}

// =============================================================================
// Key handling
// =============================================================================

// updatePruneSnapshotsScreen handles keys for the prune-snapshots screen.
func (m model) updatePruneSnapshotsScreen(msg tea.KeyMsg) (model, tea.Cmd) {
	// Destroying is not interruptible, for the same reason stateCleanup's
	// running phase is not: a half-finished prune loop is harder to reason
	// about than letting it finish.
	if m.sourcePrunePhase == cleanupPhaseRunning {
		return m, nil
	}
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	if m.sourcePrunePhase == cleanupPhaseConfirm {
		return m.updatePruneSnapshotsConfirm(msg)
	}

	switch msg.String() {
	case "esc", "q":
		m.state = stateMenu
		m.sourcePruneReady = false
		m.sourcePruneMessage = ""
		return m, nil
	case "r":
		m.sourcePruneReady = false
		m.sourcePrunePhase = cleanupPhasePlan
		m.sourcePruneMessage = ""
		return m, tea.Batch(m.spinner.Tick, loadSourcePrunePlan(m.sourcePrunePool))
	case "d":
		if m.sourcePrunePhase != cleanupPhasePlan || m.sourcePrunePlan == nil {
			return m, nil
		}
		if len(m.sourcePrunePlan.Targets) == 0 {
			m.sourcePruneMessage = "Nothing is cleared for destruction, so there is nothing to confirm."
			return m, nil
		}
		m.sourcePrunePhase = cleanupPhaseConfirm
		m.sourcePruneMessage = ""
		m.sourcePruneViewport = newShorterViewport(m.width, m.height, 4, confirmPruneBody(m.sourcePrunePlan))
		m.input.SetValue("")
		m.input.Placeholder = ""
		m.input.Focus()
		return m, textinput.Blink
	default:
		var cmd tea.Cmd
		m.sourcePruneViewport, cmd = m.sourcePruneViewport.Update(msg)
		return m, cmd
	}
}

// updatePruneSnapshotsConfirm handles the typed-confirmation phase.
func (m model) updatePruneSnapshotsConfirm(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "up", "down", "pgup", "pgdown":
		var cmd tea.Cmd
		m.sourcePruneViewport, cmd = m.sourcePruneViewport.Update(msg)
		return m, cmd
	case "esc":
		m.sourcePrunePhase = cleanupPhasePlan
		m.sourcePruneMessage = "Aborted. Nothing was destroyed."
		m.sourcePruneViewport = newReportViewport(m.width, m.height, m.sourcePrunePlanBody)
		m.input.SetValue("")
		m.input.Blur()
		return m, nil
	case "enter":
		if strings.TrimSpace(m.input.Value()) != destroyConfirmationWord {
			m.sourcePruneMessage = fmt.Sprintf(
				"Type %s exactly to continue, or press esc to back out.", destroyConfirmationWord)
			m.input.SetValue("")
			return m, nil
		}
		m.input.Blur()
		m.input.SetValue("")
		m.sourcePrunePhase = cleanupPhaseRunning
		m.sourcePruneMessage = ""
		m.sourcePruneDestroyTotal = len(m.sourcePrunePlan.Targets)
		m.sourcePruneDestroyDone = 0
		m.sourcePruneDestroyProgress = make(chan struct{}, 1)
		return m, tea.Batch(m.spinner.Tick,
			performSourcePrune(m.sourcePrunePool, m.sourcePrunePlan.Targets, m.sourcePruneDestroyProgress),
			listenSourcePruneDestroyProgress(m.sourcePruneDestroyProgress))
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// =============================================================================
// Rendering
// =============================================================================

// renderPruneSnapshotsContent draws the prune-snapshots screen for the
// current phase.
func (m model) renderPruneSnapshotsContent(width int) string {
	if !m.sourcePruneReady {
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(m.spinner.View() + " Scanning source-pool snapshot history...")
	}

	var b strings.Builder

	title := selectedItemStyle.Render(fmt.Sprintf("Prune Historic Snapshots: %s", m.sourcePrunePool))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(title))
	b.WriteString("\n\n")

	b.WriteString(m.sourcePruneViewport.View())
	b.WriteString("\n")

	centre := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	switch m.sourcePrunePhase {
	case cleanupPhasePlan:
		b.WriteString(centre.Render(m.renderPrunePlanFooter()))
	case cleanupPhaseConfirm:
		if !m.sourcePruneViewport.AtBottom() {
			b.WriteString(centre.Render(mutedStyle.Render(fmt.Sprintf(
				"▼ more below | %d%%", int(m.sourcePruneViewport.ScrollPercent()*100)))))
			b.WriteString("\n")
		}
		b.WriteString(centre.Render(dangerBanner("DESTROYING SNAPSHOTS IS IRREVERSIBLE")))
		b.WriteString("\n")
		b.WriteString(centre.Render(warningStyle.Render(fmt.Sprintf(
			"Type %s to destroy %s on %s",
			destroyConfirmationWord, pluralise(len(m.sourcePrunePlan.Targets), "snapshot", "snapshots"), m.sourcePrunePool))))
		b.WriteString("\n")
		b.WriteString(centre.Render(m.input.View()))
	case cleanupPhaseRunning:
		b.WriteString(centre.Render(fmt.Sprintf("%s Pruning snapshot %d of %d...",
			m.spinner.View(), min(m.sourcePruneDestroyDone+1, m.sourcePruneDestroyTotal), m.sourcePruneDestroyTotal)))
	case cleanupPhaseDone:
		b.WriteString(centre.Render(m.renderPruneDoneFooter()))
	}
	b.WriteString("\n")

	if m.sourcePruneMessage != "" {
		b.WriteString(centre.Render(warningStyle.Render(m.sourcePruneMessage)))
		b.WriteString("\n")
	}

	return b.String()
}

// renderPrunePlanFooter summarises the dry run and the key that acts on it.
func (m model) renderPrunePlanFooter() string {
	if m.sourcePrunePlan == nil || len(m.sourcePrunePlan.Targets) == 0 {
		return statusStyle.Render("Nothing to prune - every snapshot is within the retention policy.")
	}
	return infoStyle.Render(fmt.Sprintf(
		"Dry run: %s, at least %s would be freed. Press d to destroy them.",
		pluralise(len(m.sourcePrunePlan.Targets), "snapshot", "snapshots"), formatSize(m.sourcePrunePlan.uniqueBytes())))
}

// renderPruneDoneFooter summarises what the destroy run achieved.
func (m model) renderPruneDoneFooter() string {
	destroyed := len(m.sourcePruneOutcome.Destroyed)
	if len(m.sourcePruneOutcome.Failures) > 0 {
		return errorStyle.Render(fmt.Sprintf(
			"Destroyed %s, %d failed - see the report above, then press r to re-scan.",
			pluralise(destroyed, "snapshot", "snapshots"), len(m.sourcePruneOutcome.Failures)))
	}
	return statusStyle.Render(fmt.Sprintf("Destroyed %s.", pluralise(destroyed, "snapshot", "snapshots")))
}

// buildPrunePlanView renders the dry-run body shown in the viewport.
func buildPrunePlanView(plan *sourcePrunePlan, preview []string) string {
	var b strings.Builder

	b.WriteString(infoStyle.Render(describeScope(plan.Pool, plan.InScope, plan.Missing)))
	b.WriteString("\n")
	b.WriteString(infoStyle.Render(retentionPolicyDescription(defaultRetentionPolicy)))
	b.WriteString("\n")
	b.WriteString(infoStyle.Render("Only datasets already backed up by zfs-backup are ever touched."))
	b.WriteString("\n\n")

	if len(plan.Decisions) == 0 {
		b.WriteString(statusStyle.Render("Every snapshot is within the retention policy. Nothing to prune."))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(labelStyle.Render("What would be pruned"))
	b.WriteString("\n\n")
	b.WriteString(renderSourcePrunePlan(plan))

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
	b.WriteString(infoStyle.Render(
		"zfs-backup's own snapshots survive as bookmarks, so incremental backups keep working.\nPruned sanoid snapshots do not - they are gone."))
	b.WriteString("\n")

	return b.String()
}

// buildPruneResultView renders the outcome shown in the viewport after a run.
func buildPruneResultView(pool string, outcome cleanupOutcome, usage []datasetUsage) string {
	var b strings.Builder

	b.WriteString(labelStyle.Render(fmt.Sprintf("Pruning complete on %s", pool)))
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
		b.WriteString(labelStyle.Render("Space after pruning"))
		b.WriteString("\n\n")
		for _, u := range usage {
			fmt.Fprintf(&b, "  %-32s used %10s  snapshots %10s\n",
				u.Name, formatSize(u.Used), formatSize(u.UsedBySnapshots))
		}
	}

	return b.String()
}

// prunePlanHotkeys returns the footer hotkeys for the current phase, so the
// destroy key is only advertised when it would actually do something.
func (m model) prunePlanHotkeys() string {
	switch m.sourcePrunePhase {
	case cleanupPhaseConfirm:
		return fmt.Sprintf("scroll ↑/↓ • type %s • enter confirm • esc back", destroyConfirmationWord)
	case cleanupPhaseRunning:
		return "pruning - please wait"
	case cleanupPhaseDone:
		return "scroll up/down • r rescan • esc return"
	default:
		if m.sourcePrunePlan != nil && len(m.sourcePrunePlan.Targets) > 0 {
			return "scroll up/down • d prune • r refresh • esc return"
		}
		return "scroll up/down • r refresh • esc return"
	}
}

// confirmPruneBody is what the viewport shows while DESTROY is typed: the
// exact list of what dies, nothing else - mirroring confirmCleanupBody.
func confirmPruneBody(plan *sourcePrunePlan) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render(fmt.Sprintf(
		"These %s will be destroyed on %s:",
		pluralise(len(plan.Targets), "snapshot", "snapshots"), plan.Pool)))
	b.WriteString("\n\n")
	for _, t := range plan.Targets {
		b.WriteString("  " + t.Name + "\n")
	}
	b.WriteString("\n")
	b.WriteString(infoStyle.Render(fmt.Sprintf(
		"At least %s is freed once they are gone.", formatSize(plan.uniqueBytes()))))
	b.WriteString("\n")
	return b.String()
}
