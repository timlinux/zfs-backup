// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// stateBrowse is the backup snapshot browser: a two-pane walk of everything
// on the backup pool, with every snapshot judged against the source pool so
// the problematic ones - orphaned from the source, never to be pruned - are
// visible and reachable without scrolling through the healthy ones.
const stateBrowse sessionState = 107

// browsePhase tracks where the user is within the browser screen.
type browsePhase int

const (
	// browsePhaseList is the two-pane browser. Strictly read-only.
	browsePhaseList browsePhase = iota
	// browsePhaseConfirm shows the vetted candidates and asks for DESTROY.
	browsePhaseConfirm
	// browsePhaseRunning is destroying the confirmed candidates.
	browsePhaseRunning
	// browsePhaseDone reports what actually happened.
	browsePhaseDone
)

// Layout constants. The two-pane threshold is derived from the panes, not
// guessed: below it the joined block would exceed the terminal and wrap,
// shearing the verdict badges off the right edge.
const (
	// browseListWidth is the dataset pane's fixed width.
	browseListWidth = 40
	// browseBadgeWidth is room for the widest verdict badge ("protected").
	browseBadgeWidth = 9
	// browseSnapChrome is the non-tag furniture in one snapshot row: the
	// cursor gutter (3), the gap before the size column (1), the size
	// column (8) and the gap before the badge (2).
	browseSnapChrome = 3 + 1 + 8 + 2
	// browseMaxTagWidth caps the tag column so huge terminals do not
	// stretch rows into unreadable ribbons.
	browseMaxTagWidth = 44
	// browseMinTagWidth keeps tags recognisable at the narrow extreme.
	browseMinTagWidth = 16
	// browseTwoPaneMinWidth is the narrowest terminal that fits both panes
	// side by side: list + gap + cursor/size/badge chrome + a usable tag
	// column + centring slack.
	browseTwoPaneMinWidth = browseListWidth + 2 + browseSnapChrome + browseBadgeWidth + 24 + 6
)

// =============================================================================
// Messages and commands
// =============================================================================

// browseLoadedMsg carries a classified scan of the backup pool.
type browseLoadedMsg struct {
	browse *backupBrowse
	err    error
	// unlockFailed marks the failure as a wrong passphrase, which is
	// recoverable at the password prompt - not at the browser's retry key.
	unlockFailed bool
}

// browsePlanMsg carries the vetted destroy plan for the orphan candidates.
type browsePlanMsg struct {
	plan    *destCleanupPlan
	preview []string
}

// browseDestroyProgressMsg reports one snapshot destroyed mid-run.
type browseDestroyProgressMsg struct{}

// browseCleanupDoneMsg carries the outcome of an actual destroy run.
type browseCleanupDoneMsg struct {
	outcome cleanupOutcome
}

// loadBrowseData scans both pools and classifies every destination snapshot.
// Read-only: this command never modifies anything.
func loadBrowseData(sourcePool, destPool string) tea.Cmd {
	return func() tea.Msg {
		return loadBrowseDataSync(sourcePool, destPool)
	}
}

// loadBrowseDataSync is the body of loadBrowseData, shared with the unlock
// path. The backup scope is resolved best-effort: when it cannot be read the
// browser still works, it just stops calling out-of-scope snapshots orphans.
func loadBrowseDataSync(sourcePool, destPool string) tea.Msg {
	ctx := context.Background()
	inScope, _, err := resolveBackupDatasets(sourcePool)
	scopeKnown := err == nil
	browse, err := collectBackupBrowse(ctx, defaultRunner, sourcePool, destPool,
		getLocalHostname(), inScope, scopeKnown)
	return browseLoadedMsg{browse: browse, err: err}
}

// unlockAndLoadBrowse unlocks the backup pool with the entered passphrase and
// then loads the browser data. A failed unlock is reported as such so the
// user lands back at the password prompt, not at a dead-end error screen.
func (m model) unlockAndLoadBrowse() tea.Cmd {
	sourcePool, destPool, password := m.browseSrcPool, m.browseDestPool, m.password
	return func() tea.Msg {
		cmd := exec.Command("zfs", "load-key", destPool)
		cmd.Stdin = strings.NewReader(password + "\n")
		if err := cmd.Run(); err != nil {
			return browseLoadedMsg{
				err:          fmt.Errorf("the passphrase did not unlock %s", destPool),
				unlockFailed: true,
			}
		}
		return loadBrowseDataSync(sourcePool, destPool)
	}
}

// buildBrowsePlan vets the browser's candidates and dry-runs each destroy.
// Read-only: nothing is destroyed until the user has typed DESTROY.
func buildBrowsePlan(browse *backupBrowse) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		plan := buildDestCleanupPlan(ctx, defaultRunner, browse)
		return browsePlanMsg{
			plan:    plan,
			preview: previewDestroy(ctx, defaultRunner, plan.Targets),
		}
	}
}

// performBrowseCleanup destroys the vetted candidates, one snapshot at a
// time, reporting each destroy through the progress channel so the running
// view can count along instead of sitting on a bare spinner.
func performBrowseCleanup(targets []string, progress chan struct{}) tea.Cmd {
	return func() tea.Msg {
		outcome := destroyPlannedSnapshots(context.Background(), defaultRunner, targets,
			func(string) { progress <- struct{}{} })
		close(progress)
		return browseCleanupDoneMsg{outcome: outcome}
	}
}

// listenBrowseDestroyProgress relays one progress tick from the destroy run.
func listenBrowseDestroyProgress(progress chan struct{}) tea.Cmd {
	return func() tea.Msg {
		if _, ok := <-progress; !ok {
			return nil
		}
		return browseDestroyProgressMsg{}
	}
}

// =============================================================================
// Navigation
// =============================================================================

// browseSelectedDataset returns the dataset under the cursor, if any.
func (m model) browseSelectedDataset() *browseDataset {
	if m.browseData == nil || m.browseDatasetIdx < 0 || m.browseDatasetIdx >= len(m.browseData.Datasets) {
		return nil
	}
	return &m.browseData.Datasets[m.browseDatasetIdx]
}

// browseJumpToNextOrphan finds the next orphan candidate after the cursor,
// wrapping around, so pressing o repeatedly walks every problematic snapshot
// on the pool. With the cursor on the dataset pane the search starts at that
// dataset's own first snapshot, so a candidate right under the cursor is
// found first, not last. Returns the target position and the candidate's
// ordinal among all candidates (in walk order).
func browseJumpToNextOrphan(datasets []browseDataset, dsIdx, snapIdx int, focusSnaps bool) (d, s, ordinal int, found bool) {
	type position struct{ dataset, snapshot int }
	var flat []position
	current := -1
	for di, ds := range datasets {
		for si := range ds.Snapshots {
			if di == dsIdx {
				if focusSnaps && si == snapIdx {
					current = len(flat)
				} else if !focusSnaps && si == 0 {
					// Start just before this dataset's first snapshot.
					current = len(flat) - 1
				}
			}
			flat = append(flat, position{di, si})
		}
	}
	if len(flat) == 0 {
		return dsIdx, snapIdx, 0, false
	}
	for i := 1; i <= len(flat); i++ {
		idx := ((current+i)%len(flat) + len(flat)) % len(flat)
		p := flat[idx]
		if datasets[p.dataset].Snapshots[p.snapshot].Class == browseOrphan {
			ordinal := 1
			for _, q := range flat[:idx] {
				if datasets[q.dataset].Snapshots[q.snapshot].Class == browseOrphan {
					ordinal++
				}
			}
			return p.dataset, p.snapshot, ordinal, true
		}
	}
	return dsIdx, snapIdx, 0, false
}

// =============================================================================
// Key handling
// =============================================================================

// updateBrowseScreen handles keys for the browser screen.
func (m model) updateBrowseScreen(msg tea.KeyMsg) (model, tea.Cmd) {
	// Destroying is not interruptible, same contract as the cleanup screen.
	if m.browsePhase == browsePhaseRunning {
		return m, nil
	}
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	if m.browsePhase == browsePhaseConfirm {
		return m.updateBrowseConfirm(msg)
	}
	if m.browsePhase == browsePhaseDone {
		switch msg.String() {
		case "r", "enter":
			// The pool changed under us - anything shown before the destroy
			// would be a stale verdict, so going back means rescanning.
			return m.rescanBrowse()
		case "esc", "q":
			m.state = stateMenu
			m.browseReady = false
			m.browsePhase = browsePhaseList
			m.browseMessage = ""
			return m, nil
		default:
			var cmd tea.Cmd
			m.browseViewport, cmd = m.browseViewport.Update(msg)
			return m, cmd
		}
	}

	// A failed scan leaves nothing to navigate; only retry and escape mean
	// anything, and the other keys must not pretend to know the pool - but
	// they answer, because a keypress that does nothing at all reads as a
	// hang.
	if m.browseErr != nil || m.browseData == nil {
		switch msg.String() {
		case "esc", "q":
			m.state = stateMenu
			m.browseReady = false
			m.browseMessage = ""
			return m, nil
		case "r":
			return m.rescanBrowse()
		}
		m.browseMessage = "The pool has not been scanned yet - press r to retry."
		m.browseMessagePositive = false
		return m, nil
	}

	ds := m.browseSelectedDataset()
	budget := m.browsePaneBudget()
	switch msg.String() {
	case "esc", "q":
		if m.browseFocusSnaps {
			m.browseFocusSnaps = false
			m.browseMessage = ""
			return m, nil
		}
		m.state = stateMenu
		m.browseReady = false
		m.browseMessage = ""
		return m, nil
	case "up", "k":
		return m.browseMoveCursor(-1, ds), nil
	case "down", "j":
		return m.browseMoveCursor(1, ds), nil
	case "pgup":
		return m.browseMoveCursor(-budget, ds), nil
	case "pgdown":
		return m.browseMoveCursor(budget, ds), nil
	case "home", "g":
		return m.browseMoveCursor(-1<<30, ds), nil
	case "end", "G":
		return m.browseMoveCursor(1<<30, ds), nil
	case "enter", "right", "l", "tab":
		m.browseMessage = ""
		if !m.browseFocusSnaps && ds != nil {
			if len(ds.Snapshots) == 0 {
				m.browseMessage = "This dataset has no snapshots."
				m.browseMessagePositive = false
				return m, nil
			}
			m.browseFocusSnaps = true
			m.browseSnapIdx = 0
		} else if m.browseFocusSnaps {
			m.browseFocusSnaps = false
		}
		return m, nil
	case "left", "h":
		m.browseFocusSnaps = false
		m.browseMessage = ""
		return m, nil
	case "o", "O":
		d, s, ordinal, found := browseJumpToNextOrphan(
			m.browseData.Datasets, m.browseDatasetIdx, m.browseSnapIdx, m.browseFocusSnaps)
		if !found {
			m.browseMessage = "No deletion candidates on this pool - everything lines up with the source."
			m.browseMessagePositive = true
			return m, nil
		}
		m.browseDatasetIdx, m.browseSnapIdx = d, s
		m.browseFocusSnaps = true
		m.browseMessage = fmt.Sprintf("Candidate %d of %d.", ordinal, m.browseData.OrphanCount)
		m.browseMessagePositive = false
		return m, nil
	case "r":
		return m.rescanBrowse()
	case "c":
		if m.browseData.OrphanCount == 0 {
			m.browseMessage = "Nothing to clean up - no snapshot here is orphaned from the source."
			m.browseMessagePositive = true
			return m, nil
		}
		m.browsePhase = browsePhaseConfirm
		m.browsePlan = nil // still being vetted; confirm renders a spinner until browsePlanMsg
		m.browseMessage = ""
		return m, tea.Batch(m.spinner.Tick, buildBrowsePlan(m.browseData))
	}
	return m, nil
}

// browseMoveCursor moves the cursor in the focused pane by delta, clamped.
func (m model) browseMoveCursor(delta int, ds *browseDataset) model {
	m.browseMessage = ""
	if m.browseFocusSnaps {
		if ds == nil {
			return m
		}
		m.browseSnapIdx = min(max(m.browseSnapIdx+delta, 0), len(ds.Snapshots)-1)
		return m
	}
	previous := m.browseDatasetIdx
	m.browseDatasetIdx = min(max(m.browseDatasetIdx+delta, 0), len(m.browseData.Datasets)-1)
	if m.browseDatasetIdx != previous {
		m.browseSnapIdx = 0
	}
	return m
}

// rescanBrowse throws the current verdicts away and reads both pools again.
func (m model) rescanBrowse() (model, tea.Cmd) {
	m.browsePhase = browsePhaseList
	m.browseReady = false
	m.browseMessage = ""
	m.browseFocusSnaps = false
	m.browseSnapIdx = 0
	return m, tea.Batch(m.spinner.Tick, loadBrowseData(m.browseSrcPool, m.browseDestPool))
}

// updateBrowseConfirm handles the typed-confirmation phase.
func (m model) updateBrowseConfirm(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "up", "down", "pgup", "pgdown":
		// The list of what dies stays reachable at the moment of commitment.
		var cmd tea.Cmd
		m.browseViewport, cmd = m.browseViewport.Update(msg)
		return m, cmd
	case "esc":
		m.browsePhase = browsePhaseList
		m.browseMessage = "Aborted. Nothing was destroyed."
		m.browseMessagePositive = false
		m.input.SetValue("")
		m.input.Blur()
		return m, nil
	case "enter":
		if m.browsePlan == nil || len(m.browsePlan.Targets) == 0 {
			return m, nil
		}
		if strings.TrimSpace(m.input.Value()) != destroyConfirmationWord {
			m.browseMessage = fmt.Sprintf(
				"Type %s exactly to continue, or press esc to back out.", destroyConfirmationWord)
			m.browseMessagePositive = false
			m.input.SetValue("")
			return m, nil
		}
		m.input.Blur()
		m.input.SetValue("")
		m.browsePhase = browsePhaseRunning
		m.browseMessage = ""
		m.browseDestroyTotal = len(m.browsePlan.Targets)
		m.browseDestroyDone = 0
		m.browseDestroyProgress = make(chan struct{}, 1)
		return m, tea.Batch(m.spinner.Tick,
			performBrowseCleanup(m.browsePlan.Targets, m.browseDestroyProgress),
			listenBrowseDestroyProgress(m.browseDestroyProgress))
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// =============================================================================
// Rendering
// =============================================================================

// browseClassBadge renders the one-word verdict for a snapshot. The badge
// that must survive - the incremental base - is the loudest of the safe
// verdicts; the badge that must go is the only red one.
func browseClassBadge(class browseClass) string {
	switch class {
	case browseBase:
		return statusStyle.Render("⚓ base")
	case browseSynced:
		return statusStyle.Render("synced")
	case browseRetained:
		return infoStyle.Render("retained")
	case browseOrphan:
		return errorStyle.Render("ORPHAN")
	case browseRecent:
		return warningStyle.Render("recent")
	case browseProtected:
		return mutedStyle.Render("protected")
	case browseRemoteNS:
		return mutedStyle.Render("remote")
	default:
		return mutedStyle.Render("foreign")
	}
}

// browsePaneBudget is how many rows each pane may spend on its list.
func (m model) browsePaneBudget() int {
	budget := m.height - 12
	if budget < 5 {
		budget = 5
	}
	return budget
}

// browseTagWidth computes the snapshot tag column for a given pane width.
func browseTagWidth(paneWidth int, withSize bool) int {
	chrome := browseSnapChrome + browseBadgeWidth
	if !withSize {
		chrome = 3 + 2 + browseBadgeWidth
	}
	return min(max(paneWidth-chrome, browseMinTagWidth), browseMaxTagWidth)
}

// renderBrowseContent draws the browser for the current phase.
func (m model) renderBrowseContent(width int) string {
	centre := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	if !m.browseReady {
		return centre.Render(m.spinner.View() + " Reading both pools and comparing snapshots...")
	}
	if m.browseErr != nil {
		var b strings.Builder
		b.WriteString(centre.Render(errorStyle.Render("Could not scan the backup pool")))
		b.WriteString("\n\n")
		b.WriteString(centre.Render(m.browseErr.Error()))
		b.WriteString("\n\n")
		b.WriteString(centre.Render(mutedStyle.Render("r retry • esc return to menu")))
		if m.browseMessage != "" {
			b.WriteString("\n")
			b.WriteString(centre.Render(warningStyle.Render(m.browseMessage)))
		}
		return b.String()
	}

	switch m.browsePhase {
	case browsePhaseConfirm:
		return m.renderBrowseConfirm(width)
	case browsePhaseRunning:
		return centre.Render(fmt.Sprintf("%s Destroying snapshot %d of %d...",
			m.spinner.View(), min(m.browseDestroyDone+1, m.browseDestroyTotal), m.browseDestroyTotal))
	case browsePhaseDone:
		return m.renderBrowseDone(width)
	}
	return m.renderBrowseList(width)
}

// renderBrowseList draws the browser: two panes side by side when the
// terminal fits them, otherwise the focused pane alone. The block is
// composed, measured against the real header and footer for this width and
// shrunk until it fits - wrapped summary or reason lines steal rows from the
// panes, never from the frame, so the Kartoza header and the cursor can
// never scroll off screen. If even a three-row pane cannot fit, the legend
// is sacrificed before any list row is.
func (m model) renderBrowseList(width int) string {
	// Header and footer both end in a newline, so their newline counts are
	// exactly their row counts.
	frame := strings.Count(renderHeader(width, m.getStatusText()), "\n") +
		strings.Count(renderFooter(width, m.browseHotkeys(), 0, 1), "\n")
	avail := max(m.height-frame, 1)

	budget := m.browsePaneBudget()
	legend, compact := true, false
	out := m.composeBrowseList(width, budget, legend, compact)
	for displayRows(out, width) > avail {
		switch {
		case budget > 3:
			budget--
		case legend:
			legend = false
		case !compact:
			compact = true // give up the blank separator rows too
		default:
			// Physically too small even at the bottom rung: say so on one
			// line instead of emitting a block that scrolls the header away.
			return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).
				Render(warningStyle.Render("Terminal too small - the browser needs at least 80x24."))
		}
		out = m.composeBrowseList(width, budget, legend, compact)
	}
	return out
}

// displayRows counts the rows a block occupies on a terminal of the given
// width - what the terminal counts, not what strings.Count sees: a logical
// line wider than the terminal wraps and costs more than one row.
func displayRows(s string, width int) int {
	if width < 1 {
		width = 1
	}
	rows := 0
	for _, line := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		rows += max(1, (lipgloss.Width(line)+width-1)/width)
	}
	return rows
}

// composeBrowseList renders the list phase with a given pane row budget.
// compact drops the blank separator rows, the last resort before admitting
// the terminal is too small.
func (m model) composeBrowseList(width, budget int, showLegend, compact bool) string {
	b := &strings.Builder{}
	browse := m.browseData
	centre := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	wide := width >= browseTwoPaneMinWidth

	title := selectedItemStyle.Render(fmt.Sprintf(
		"Backup Snapshot Browser: %s judged against %s", browse.DestPool, browse.SourcePool))
	b.WriteString(centre.Render(title))
	b.WriteString("\n")

	// In compact mode every prose line is truncated to a single row before
	// styling - a wrapped line costs the row that was just reclaimed.
	if compact {
		summary := fmt.Sprintf("%s • %s orphaned",
			pluralise(len(browse.Datasets), "dataset", "datasets"),
			pluralise(browse.OrphanCount, "snapshot", "snapshots"))
		// The scope caveat explains why removed-from-scope datasets still
		// read "retained", so it outranks the reclaimable figure when the
		// truncation has to choose.
		if !browse.ScopeKnown {
			summary += " • scope unknown"
		}
		summary += fmt.Sprintf(" • %s reclaimable", formatSize(browse.OrphanBytes))
		style := statusStyle
		if browse.OrphanCount > 0 {
			style = errorStyle
		}
		b.WriteString(centre.Render(style.Render(truncateRunes(summary, width-2))))
	} else {
		summary := fmt.Sprintf("%s browsed", pluralise(len(browse.Datasets), "dataset", "datasets"))
		if browse.OrphanCount > 0 {
			summary += " • " + errorStyle.Render(fmt.Sprintf(
				"%s orphaned from the source (at least %s reclaimable)",
				pluralise(browse.OrphanCount, "snapshot", "snapshots"), formatSize(browse.OrphanBytes)))
		} else {
			summary += " • " + statusStyle.Render("no orphans - everything lines up with the source")
		}
		if !browse.ScopeKnown {
			summary += " • " + warningStyle.Render("backup scope unreadable - scope checks skipped")
		}
		b.WriteString(centre.Render(summary))
	}
	b.WriteString("\n")

	// A first visit should not need the manual: say what the verdict that
	// drives decisions actually means. The longer clauses only appear where
	// the whole legend still fits on one line.
	if showLegend {
		legend := "ORPHAN = nothing will ever prune it, safe to remove"
		if width >= 130 {
			legend += " • ⚓ base = keeps the next backup incremental • retained = older history kept on purpose"
		}
		b.WriteString(centre.Render(mutedStyle.Render(legend)))
		b.WriteString("\n")
	}
	if !compact {
		b.WriteString("\n")
	}

	if !wide {
		// Narrow terminal: one pane at a time, following the focus, as a
		// fixed-width block so rows stay left-aligned within it.
		paneWidth := min(width-2, 76)
		var pane string
		if m.browseFocusSnaps {
			pane = m.renderBrowseSnapshotPane(budget, paneWidth)
		} else {
			pane = m.renderBrowseDatasetPane(budget, false)
		}
		b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center,
			lipgloss.NewStyle().Width(paneWidth).Render(pane)))
	} else {
		rightWidth := min(width-browseListWidth-6, browseSnapChrome+browseBadgeWidth+browseMaxTagWidth)
		joined := lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(browseListWidth).Render(m.renderBrowseDatasetPane(budget, true)),
			"  ",
			lipgloss.NewStyle().Width(rightWidth).Render(m.renderBrowseSnapshotPane(budget, rightWidth)))
		b.WriteString(lipgloss.PlaceHorizontal(width, lipgloss.Center, joined))
	}
	// The pane block must always be terminated or the next line fuses onto
	// its trailing padded row into one double-width line; compact only ever
	// drops the blank separator that follows.
	b.WriteString("\n")
	if !compact {
		b.WriteString("\n")
	}

	// The reason line explains the highlighted snapshot in plain language.
	if ds := m.browseSelectedDataset(); ds != nil && m.browseFocusSnaps &&
		m.browseSnapIdx >= 0 && m.browseSnapIdx < len(ds.Snapshots) {
		reason := ds.Snapshots[m.browseSnapIdx].Reason
		if compact {
			reason = truncateRunes(reason, width-2)
		}
		b.WriteString(centre.Render(infoStyle.Render(reason)))
		b.WriteString("\n")
	}

	if m.browseMessage != "" {
		style := warningStyle
		if m.browseMessagePositive {
			style = statusStyle
		}
		message := m.browseMessage
		if compact {
			message = truncateRunes(message, width-2)
		}
		b.WriteString(centre.Render(style.Render(message)))
		b.WriteString("\n")
	}
	return b.String()
}

// renderBrowseDatasetPane draws the dataset pane: every dataset on the
// backup pool, with its orphan count as the eye-catcher.
func (m model) renderBrowseDatasetPane(budget int, wide bool) string {
	var b strings.Builder
	datasets := m.browseData.Datasets

	start := 0
	if m.browseDatasetIdx >= budget {
		start = m.browseDatasetIdx - budget + 1
	}
	end := min(len(datasets), start+budget)

	if start > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf(" ▲ %d above", start)))
		b.WriteString("\n")
	}
	for i := start; i < end; i++ {
		ds := datasets[i]
		label := ds.Name
		if rest, ok := strings.CutPrefix(label, m.browseData.DestPool+"/"); ok {
			label = rest
		}
		suffix := ""
		switch {
		case ds.OrphanCount > 0 && wide:
			suffix = errorStyle.Render("● " + pluralise(ds.OrphanCount, "orphan", "orphans"))
		case ds.OrphanCount > 0:
			suffix = errorStyle.Render(fmt.Sprintf("● %d", ds.OrphanCount))
		case ds.RemoteHost != "":
			suffix = mutedStyle.Render("(remote)")
		case len(ds.Snapshots) == 0:
			suffix = mutedStyle.Render("(no snapshots)")
		}
		// Truncate the name, never the badge: the count is the eye-catcher.
		label = truncateRunes(label, max(browseListWidth-4-lipgloss.Width(suffix)-1, 8))
		line := label
		if suffix != "" {
			line += " " + suffix
		}
		if i == m.browseDatasetIdx && !m.browseFocusSnaps {
			b.WriteString(selectedItemStyle.Render(" ▌ ") + selectedItemStyle.Render(label))
			if suffix != "" {
				b.WriteString(" " + suffix)
			}
		} else if i == m.browseDatasetIdx {
			b.WriteString(infoStyle.Render(" ▌ ") + line)
		} else {
			b.WriteString("   " + line)
		}
		b.WriteString("\n")
	}
	if end < len(datasets) {
		b.WriteString(mutedStyle.Render(fmt.Sprintf(" ▼ %d below", len(datasets)-end)))
		b.WriteString("\n")
	}
	return b.String()
}

// renderBrowseSnapshotPane draws the snapshot pane: the selected dataset's
// snapshots, newest first, each with its verdict. Row geometry adapts to the
// pane width so the badge - the point of the screen - never wraps off.
func (m model) renderBrowseSnapshotPane(budget, paneWidth int) string {
	ds := m.browseSelectedDataset()
	if ds == nil {
		return mutedStyle.Render("  Nothing selected.")
	}
	var b strings.Builder

	head := labelStyle.Render(truncateRunes(ds.Name, paneWidth))
	arrow := ""
	switch {
	case ds.RemoteHost != "":
		arrow = mutedStyle.Render("→ backups pulled from " + ds.RemoteHost)
	case ds.Source == "":
		arrow = mutedStyle.Render("→ mirrors nothing on " + m.browseData.SourcePool)
	case !ds.SourceExists:
		arrow = errorStyle.Render("→ " + ds.Source + " no longer exists")
	default:
		arrow = mutedStyle.Render("→ " + ds.Source)
	}
	b.WriteString(head)
	b.WriteString("\n")
	b.WriteString(arrow)
	b.WriteString("\n\n")

	if len(ds.Snapshots) == 0 {
		b.WriteString(mutedStyle.Render("  No snapshots on this dataset."))
		b.WriteString("\n")
		return b.String()
	}

	// The size column decides between candidates, so it is shown wherever a
	// readable tag column can coexist with it - the exact fit condition,
	// not a round number, so no width can ever show less than a narrower
	// one does.
	withSize := paneWidth >= browseMinTagWidth+browseSnapChrome+browseBadgeWidth
	tagWidth := browseTagWidth(paneWidth, withSize)

	listBudget := max(budget-3, 3)
	start := 0
	if m.browseSnapIdx >= listBudget {
		start = m.browseSnapIdx - listBudget + 1
	}
	end := min(len(ds.Snapshots), start+listBudget)

	if start > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ▲ %d newer", start)))
		b.WriteString("\n")
	}
	for i := start; i < end; i++ {
		snap := ds.Snapshots[i]
		tag := truncateRunes(snap.Tag, tagWidth)
		line := tag + strings.Repeat(" ", tagWidth-lipgloss.Width(tag))
		if withSize {
			size := "        "
			if snap.Used >= 0 {
				size = fmt.Sprintf("%8s", formatSize(snap.Used))
			}
			line += " " + size
		}
		line += "  " + browseClassBadge(snap.Class)
		if i == m.browseSnapIdx && m.browseFocusSnaps {
			b.WriteString(selectedItemStyle.Render(" ▌ ") + line)
		} else {
			b.WriteString("   " + line)
		}
		b.WriteString("\n")
	}
	if end < len(ds.Snapshots) {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ▼ %d older", len(ds.Snapshots)-end)))
		b.WriteString("\n")
	}
	return b.String()
}

// truncateRunes shortens a string to limit display cells, rune-safely.
func truncateRunes(s string, limit int) string {
	if lipgloss.Width(s) <= limit {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > limit {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// renderBrowseConfirm draws the typed-confirmation phase.
func (m model) renderBrowseConfirm(width int) string {
	centre := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	if m.browsePlan == nil {
		return centre.Render(m.spinner.View() + " Vetting candidates against holds, clones and dry runs...")
	}

	var b strings.Builder
	b.WriteString(centre.Render(selectedItemStyle.Render(
		fmt.Sprintf("Destroy Orphaned Backup Snapshots: %s", m.browsePlan.DestPool))))
	b.WriteString("\n\n")
	b.WriteString(m.browseViewport.View())
	b.WriteString("\n")

	if len(m.browsePlan.Targets) == 0 {
		b.WriteString(centre.Render(warningStyle.Render(
			"Every candidate was held back by a safety check. Nothing to do.")))
		b.WriteString("\n")
		b.WriteString(centre.Render(mutedStyle.Render("esc return to the browser")))
		return b.String()
	}

	if !m.browseViewport.AtBottom() {
		b.WriteString(centre.Render(mutedStyle.Render(fmt.Sprintf(
			"▼ more below | %d%%", int(m.browseViewport.ScrollPercent()*100)))))
		b.WriteString("\n")
	}
	b.WriteString(centre.Render(dangerBanner("DESTROYING SNAPSHOTS IS IRREVERSIBLE")))
	b.WriteString("\n")
	b.WriteString(centre.Render(warningStyle.Render(fmt.Sprintf(
		"Type %s to destroy %s on %s",
		destroyConfirmationWord, pluralise(len(m.browsePlan.Targets), "snapshot", "snapshots"), m.browsePlan.DestPool))))
	b.WriteString("\n")
	b.WriteString(centre.Render(m.input.View()))
	b.WriteString("\n")
	if m.browseMessage != "" {
		style := warningStyle
		if m.browseMessagePositive {
			style = statusStyle
		}
		b.WriteString(centre.Render(style.Render(m.browseMessage)))
		b.WriteString("\n")
	}
	return b.String()
}

// renderBrowseDone draws the destroy outcome. The keys live in the footer,
// like every other screen - not duplicated here.
func (m model) renderBrowseDone(width int) string {
	centre := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	var b strings.Builder
	b.WriteString(centre.Render(selectedItemStyle.Render("Cleanup complete")))
	b.WriteString("\n\n")
	b.WriteString(m.browseViewport.View())
	b.WriteString("\n")
	destroyed := len(m.browseOutcome.Destroyed)
	if len(m.browseOutcome.Failures) > 0 {
		b.WriteString(centre.Render(errorStyle.Render(fmt.Sprintf(
			"Destroyed %s, %d failed - see above.",
			pluralise(destroyed, "snapshot", "snapshots"), len(m.browseOutcome.Failures)))))
	} else {
		b.WriteString(centre.Render(statusStyle.Render(fmt.Sprintf(
			"Destroyed %s.", pluralise(destroyed, "snapshot", "snapshots")))))
	}
	b.WriteString("\n")
	return b.String()
}

// buildBrowseConfirmBody is what the confirm viewport shows: exactly what
// dies, why each candidate was flagged, and what was held back. When every
// candidate was held back there is nothing to confirm, so the held-back list
// leads and the destroy furniture is omitted entirely.
func buildBrowseConfirmBody(plan *destCleanupPlan, preview []string) string {
	var b strings.Builder

	if len(plan.Targets) == 0 {
		b.WriteString(labelStyle.Render("Every candidate was held back by a safety check"))
		b.WriteString("\n\n")
		for _, d := range planSkipped(plan) {
			b.WriteString(warningStyle.Render(fmt.Sprintf("  keeping %s (%s)", d.Orphan.Name, d.SkipReason)))
			b.WriteString("\n")
		}
		return b.String()
	}

	b.WriteString(labelStyle.Render(fmt.Sprintf(
		"These %s will be destroyed on %s:",
		pluralise(len(plan.Targets), "snapshot", "snapshots"), plan.DestPool)))
	b.WriteString("\n\n")
	for _, d := range plan.Decisions {
		if d.Safe {
			fmt.Fprintf(&b, "  %s\n    %s\n", d.Orphan.Name, mutedStyle.Render(d.Orphan.Reason))
		}
	}
	if skipped := planSkipped(plan); len(skipped) > 0 {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Held back by safety checks"))
		b.WriteString("\n\n")
		for _, d := range skipped {
			b.WriteString(warningStyle.Render(fmt.Sprintf("  keeping %s (%s)", d.Orphan.Name, d.SkipReason)))
			b.WriteString("\n")
		}
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
	b.WriteString(infoStyle.Render(fmt.Sprintf(
		"At least %s is freed once they are gone. The datasets themselves and their current contents are kept.",
		formatSize(plan.uniqueBytes()))))
	b.WriteString("\n")
	b.WriteString(infoStyle.Render(reclaimCaveat))
	b.WriteString("\n")
	return b.String()
}

// planSkipped returns the decisions a safety check held back.
func planSkipped(plan *destCleanupPlan) []destroyDecision {
	var out []destroyDecision
	for _, d := range plan.Decisions {
		if !d.Safe {
			out = append(out, d)
		}
	}
	return out
}

// buildBrowseResultBody renders the destroy outcome for the viewport.
func buildBrowseResultBody(outcome cleanupOutcome) string {
	var b strings.Builder
	for _, name := range outcome.Destroyed {
		fmt.Fprintf(&b, "  %s %s\n", statusStyle.Render("destroyed"), name)
	}
	for _, f := range outcome.Failures {
		b.WriteString(errorStyle.Render("  failed: " + f))
		b.WriteString("\n")
	}
	return b.String()
}

// browseHotkeys returns the footer hotkeys for the current browser phase.
// Keys that would do nothing in the current state are not advertised.
func (m model) browseHotkeys() string {
	switch m.browsePhase {
	case browsePhaseConfirm:
		if m.browsePlan != nil && len(m.browsePlan.Targets) == 0 {
			return "esc back"
		}
		return fmt.Sprintf("scroll ↑/↓ • type %s • enter confirm • esc back", destroyConfirmationWord)
	case browsePhaseRunning:
		return "destroying - please wait"
	case browsePhaseDone:
		return "scroll up/down • r rescan • esc return"
	default:
		if m.browseErr != nil || m.browseData == nil {
			return "r retry • esc return"
		}
		keys := "↑/↓ move • pgup/pgdn page • enter/tab switch pane • o next orphan"
		if m.browseData.OrphanCount > 0 {
			keys += " • c clean up"
		}
		return keys + " • r rescan • esc return"
	}
}
