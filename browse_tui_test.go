// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// browseModel builds a model parked on a ready browser screen with the
// standard classification fixture loaded.
func browseModel(t *testing.T) model {
	t.Helper()
	datasets := classifyFixture(t, true)
	browse := &backupBrowse{
		SourcePool: "NIXROOT",
		DestPool:   "NIXBACKUP",
		Hostname:   "abyss",
		ScopeKnown: true,
		Datasets:   datasets,
	}
	for _, ds := range datasets {
		browse.OrphanCount += ds.OrphanCount
		browse.OrphanBytes += ds.OrphanBytes
	}
	return model{
		state:          stateBrowse,
		browseSrcPool:  "NIXROOT",
		browseDestPool: "NIXBACKUP",
		browseData:     browse,
		browseReady:    true,
		browsePhase:    browsePhaseList,
		input:          textinput.New(),
		width:          120,
		height:         40,
	}
}

// browsePlanFixture is a vetted destination plan with one cleared target.
func browsePlanFixture() *destCleanupPlan {
	decisions := []destroyDecision{
		{
			Orphan: orphanSnapshot{
				snapshotEntry: snapshotEntry{
					Name:    "NIXBACKUP/abyss/overflow@2026-05-17.22h-55-Backup",
					Dataset: "NIXBACKUP/abyss/overflow",
					Tag:     "2026-05-17.22h-55-Backup",
					Used:    400,
				},
				Kind:   orphanFromSource,
				Reason: "no matching dataset on the source pool",
			},
			Safe: true,
		},
	}
	return &destCleanupPlan{
		DestPool:  "NIXBACKUP",
		Decisions: decisions,
		Targets:   safeToDestroy(decisions),
	}
}

// No single keystroke may destroy anything from the browser either.
func TestBrowseScreenRequiresTheTypedConfirmation(t *testing.T) {
	m := browseModel(t)
	m.browsePhase = browsePhaseConfirm
	m.browsePlan = browsePlanFixture()

	m.input.SetValue("destroy")
	m, cmd := m.updateBrowseScreen(tea.KeyMsg{Type: tea.KeyEnter})
	if m.browsePhase != browsePhaseConfirm {
		t.Errorf("lowercase confirmation must not proceed, got phase %v", m.browsePhase)
	}
	if cmd != nil {
		t.Error("a rejected confirmation must not issue a destroy command")
	}
	if m.browseMessage == "" {
		t.Error("a rejected confirmation must say why")
	}

	m.input.SetValue(destroyConfirmationWord)
	m, cmd = m.updateBrowseScreen(tea.KeyMsg{Type: tea.KeyEnter})
	if m.browsePhase != browsePhaseRunning {
		t.Errorf("the exact word should proceed, got phase %v", m.browsePhase)
	}
	if cmd == nil {
		t.Error("proceeding should issue the destroy command")
	}
}

// While the plan is still being vetted, enter must be inert - there is no
// approved list to act on yet.
func TestBrowseConfirmIgnoresEnterWhileVetting(t *testing.T) {
	m := browseModel(t)
	m.browsePhase = browsePhaseConfirm
	m.browsePlan = nil
	m.input.SetValue(destroyConfirmationWord)

	m, cmd := m.updateBrowseScreen(tea.KeyMsg{Type: tea.KeyEnter})
	if m.browsePhase != browsePhaseConfirm || cmd != nil {
		t.Error("enter must do nothing until the vetted plan has arrived")
	}
}

func TestBrowseEscapeAbortsWithoutDestroying(t *testing.T) {
	m := browseModel(t)
	m.browsePhase = browsePhaseConfirm
	m.browsePlan = browsePlanFixture()
	m.input.SetValue(destroyConfirmationWord)

	m, cmd := m.updateBrowseScreen(tea.KeyMsg{Type: tea.KeyEsc})
	if m.browsePhase != browsePhaseList {
		t.Errorf("esc should return to the browser, got phase %v", m.browsePhase)
	}
	if cmd != nil {
		t.Error("esc must not issue a destroy command")
	}
	if !strings.Contains(m.browseMessage, "Nothing was destroyed") {
		t.Errorf("esc should say nothing was destroyed, got %q", m.browseMessage)
	}
	if m.input.Value() != "" {
		t.Error("a typed DESTROY must not survive an abort")
	}
}

func TestBrowseScreenIgnoresKeysWhileDestroying(t *testing.T) {
	m := browseModel(t)
	m.browsePhase = browsePhaseRunning

	for _, key := range []tea.KeyMsg{keyRunes("c"), keyRunes("q"), keyRunes("r"),
		{Type: tea.KeyEsc}, {Type: tea.KeyEnter}, {Type: tea.KeyCtrlC}} {
		next, cmd := m.updateBrowseScreen(key)
		if next.browsePhase != browsePhaseRunning || next.state != stateBrowse || cmd != nil {
			t.Errorf("key %v should be ignored mid-destroy", key)
		}
	}
}

func TestBrowseCtrlCQuitsOutsideADestroy(t *testing.T) {
	for _, phase := range []browsePhase{browsePhaseList, browsePhaseConfirm, browsePhaseDone} {
		m := browseModel(t)
		m.browsePhase = phase
		next, _ := m.updateBrowseScreen(tea.KeyMsg{Type: tea.KeyCtrlC})
		if !next.quitting {
			t.Errorf("ctrl+c should quit from phase %v", phase)
		}
	}
}

func TestBrowseNavigationSwitchesPanes(t *testing.T) {
	m := browseModel(t)

	m, _ = m.updateBrowseScreen(keyRunes("j"))
	if m.browseDatasetIdx != 1 || m.browseFocusSnaps {
		t.Fatalf("j should move the dataset cursor, got idx %d focus %v",
			m.browseDatasetIdx, m.browseFocusSnaps)
	}

	// Move onto a dataset with snapshots, then enter its snapshot pane.
	for m.browseSelectedDataset() != nil && len(m.browseSelectedDataset().Snapshots) == 0 {
		m, _ = m.updateBrowseScreen(keyRunes("j"))
	}
	m, _ = m.updateBrowseScreen(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.browseFocusSnaps {
		t.Fatal("enter on a dataset with snapshots should focus the snapshot pane")
	}

	m, _ = m.updateBrowseScreen(tea.KeyMsg{Type: tea.KeyEsc})
	if m.browseFocusSnaps {
		t.Fatal("esc from the snapshot pane should return to the dataset pane")
	}
	if m.state != stateBrowse {
		t.Fatal("the first esc must not leave the browser")
	}

	m, _ = m.updateBrowseScreen(tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stateMenu {
		t.Fatal("esc from the dataset pane should return to the menu")
	}
}

func TestBrowseOrphanJumpKeyLandsOnACandidate(t *testing.T) {
	m := browseModel(t)

	m, _ = m.updateBrowseScreen(keyRunes("o"))
	ds := m.browseSelectedDataset()
	if ds == nil || !m.browseFocusSnaps {
		t.Fatal("o should focus the snapshot pane on a candidate")
	}
	snapshot := ds.Snapshots[m.browseSnapIdx]
	if snapshot.Class != browseOrphan {
		t.Fatalf("o landed on %s (%v), want an orphan", snapshot.Name, snapshot.Class)
	}
}

func TestBrowseOrphanJumpExplainsWhenThereAreNone(t *testing.T) {
	m := browseModel(t)
	// Strip the fixture down to healthy datasets only.
	var healthy []browseDataset
	for _, ds := range m.browseData.Datasets {
		if ds.OrphanCount == 0 {
			healthy = append(healthy, ds)
		}
	}
	m.browseData.Datasets = healthy
	m.browseData.OrphanCount = 0
	m.browseDatasetIdx = 0

	m, _ = m.updateBrowseScreen(keyRunes("o"))
	if m.browseMessage == "" {
		t.Error("o with no candidates should say the pool is clean")
	}
}

func TestBrowseCleanupNeedsCandidates(t *testing.T) {
	m := browseModel(t)
	m.browseData.OrphanCount = 0

	m, cmd := m.updateBrowseScreen(keyRunes("c"))
	if m.browsePhase != browsePhaseList || cmd != nil {
		t.Error("c must do nothing when no snapshot is orphaned")
	}
	if m.browseMessage == "" {
		t.Error("c with nothing to do should say so")
	}
	if strings.Contains(m.browseHotkeys(), "c clean up") {
		t.Errorf("the cleanup key must not be advertised, got %q", m.browseHotkeys())
	}
}

func TestBrowseCleanupKeyStartsVetting(t *testing.T) {
	m := browseModel(t)

	m, cmd := m.updateBrowseScreen(keyRunes("c"))
	if m.browsePhase != browsePhaseConfirm {
		t.Errorf("c should enter the confirm phase, got %v", m.browsePhase)
	}
	if cmd == nil {
		t.Error("c should issue the vetting command")
	}
	if m.browsePlan != nil {
		t.Error("the plan must come from vetting, never be presumed")
	}
}

func TestBrowseDoneKeysRescanOrReturn(t *testing.T) {
	m := browseModel(t)
	m.browsePhase = browsePhaseDone

	next, cmd := m.updateBrowseScreen(keyRunes("r"))
	if next.browsePhase != browsePhaseList || next.browseReady || cmd == nil {
		t.Error("r after a destroy should rescan the pool")
	}

	m.browsePhase = browsePhaseDone
	next, cmd = m.updateBrowseScreen(tea.KeyMsg{Type: tea.KeyEsc})
	if next.state != stateMenu || cmd != nil {
		t.Error("esc after a destroy should return to the menu")
	}
}

func TestBrowseConfirmBodyListsWhatDiesAndWhatIsKept(t *testing.T) {
	plan := browsePlanFixture()
	plan.Decisions = append(plan.Decisions, destroyDecision{
		Orphan: orphanSnapshot{
			snapshotEntry: snapshotEntry{
				Name:    "NIXBACKUP/abyss/overflow@2026-05-19.00h-24-Backup",
				Dataset: "NIXBACKUP/abyss/overflow",
				Tag:     "2026-05-19.00h-24-Backup",
				Used:    500,
			},
			Kind: orphanFromSource,
		},
		SkipReason: "snapshot has a hold",
	})

	body := stripANSI(buildBrowseConfirmBody(plan, []string{"would destroy ..."}))
	if !strings.Contains(body, "2026-05-17.22h-55-Backup") {
		t.Error("the confirm body must name what dies")
	}
	if !strings.Contains(body, "snapshot has a hold") {
		t.Error("the confirm body must explain what was held back and why")
	}
	if !strings.Contains(body, "current contents are kept") {
		t.Error("the confirm body must say the datasets themselves survive")
	}
}

// When every candidate is held back there is nothing to confirm, so the body
// must not promise "0 snapshots will be destroyed" above a live input.
func TestBrowseConfirmBodyWithNoTargetsLeadsWithWhatWasKept(t *testing.T) {
	plan := &destCleanupPlan{
		DestPool: "NIXBACKUP",
		Decisions: []destroyDecision{{
			Orphan: orphanSnapshot{snapshotEntry: snapshotEntry{
				Name: "NIXBACKUP/abyss/overflow@2026-05-17.22h-55-Backup"}},
			SkipReason: "snapshot has a hold",
		}},
	}
	body := stripANSI(buildBrowseConfirmBody(plan, nil))
	if strings.Contains(body, "will be destroyed") || strings.Contains(body, "0 snapshots") {
		t.Errorf("a plan with no targets must not promise destruction, got:\n%s", body)
	}
	if !strings.Contains(body, "snapshot has a hold") {
		t.Error("the held-back reason must lead the body")
	}
}

// The zero-target confirm footer must not invite typing DESTROY.
func TestBrowseHotkeysWithNoTargetsOfferOnlyEscape(t *testing.T) {
	m := browseModel(t)
	m.browsePhase = browsePhaseConfirm
	m.browsePlan = &destCleanupPlan{DestPool: "NIXBACKUP"}
	if strings.Contains(m.browseHotkeys(), destroyConfirmationWord) {
		t.Errorf("no targets means no DESTROY prompt, got %q", m.browseHotkeys())
	}
}

// The reason shown for the highlighted snapshot is the browser's whole value:
// it must survive the round trip into the rendered screen.
func TestBrowseListShowsTheReasonForTheHighlightedSnapshot(t *testing.T) {
	m := browseModel(t)
	m, _ = m.updateBrowseScreen(keyRunes("o"))

	view := stripANSI(m.renderBrowseContent(m.width))
	if !strings.Contains(view, "prune") {
		t.Errorf("the rendered browser should explain the highlighted orphan, got:\n%s", view)
	}
}

// The whole rendered view - header, content, footer - must fit the terminal
// even on a realistic pool, or bubbletea scrolls the header and the cursor
// out of sight. A fixture-sized pool will not expose this, so inflate it.
func TestBrowseViewFitsTheTerminal(t *testing.T) {
	m := browseModel(t)
	for i := range 24 {
		count := 1
		if i == 22 {
			count = 60 // one deep dataset so the cursor can sit far down a long list
		}
		snaps := make([]browseSnapshot, 0, count)
		for j := range count {
			snaps = append(snaps, browseSnapshot{
				snapshotEntry: snap(fmt.Sprintf("NIXBACKUP/abyss/extra%02d@2026-0%d-01.2%dh-00-Backup", i, j%9+1, j%4),
					time.Duration(9+j)*24*time.Hour, 100),
				Class:  browseRetained,
				Reason: "pruned on the source, kept here by the backup retention policy for this dataset",
			})
		}
		m.browseData.Datasets = append(m.browseData.Datasets, browseDataset{
			Name:      fmt.Sprintf("NIXBACKUP/abyss/extra%02d", i),
			Snapshots: snaps,
		})
	}

	for _, size := range []struct{ w, h int }{
		{60, 24}, {80, 24}, {100, 24}, {120, 24}, {100, 30}, {120, 40},
	} {
		m.width, m.height = size.w, size.h
		// Worst case: cursor deep in the long dataset near the end of the
		// pool, so both scroll indicators, a wrapping reason line and an
		// inline message are all on screen at once.
		m.browseFocusSnaps = true
		m.browseDatasetIdx = len(m.browseData.Datasets) - 2
		m.browseSnapIdx = len(m.browseData.Datasets[m.browseDatasetIdx].Snapshots) - 1
		m.browseMessage = "Candidate 1 of 4."

		view := m.View()
		if rows := displayRows(view, size.w); rows > size.h {
			t.Errorf("%dx%d: rendered view occupies %d display rows - overflows the terminal by %d",
				size.w, size.h, rows, rows-size.h)
		}
	}
}

// A wide terminal must never show less than a narrow one: the size column
// has to survive the two-pane layout's width cap at every two-pane width,
// not just the comfortable ones.
func TestBrowseWideModeKeepsTheSizeColumn(t *testing.T) {
	m := browseModel(t)
	m.browseFocusSnaps = true
	for i, ds := range m.browseData.Datasets {
		if ds.Name == "NIXBACKUP/abyss/overflow" {
			m.browseDatasetIdx = i
		}
	}
	m.browseSnapIdx = 0

	for _, width := range []int{95, 100, 105, 120} {
		m.width, m.height = width, 40
		view := stripANSI(m.renderBrowseContent(m.width))
		if !strings.Contains(view, "500 B") {
			t.Errorf("width %d: the per-snapshot size must be visible, got:\n%s", width, view)
		}
	}
}

// Whatever the layout decides to show, no rendered line may ever exceed the
// terminal width - an over-wide line wraps and shears the layout apart.
func TestBrowseNoLineExceedsTheTerminalWidth(t *testing.T) {
	m := browseModel(t)
	m.browseFocusSnaps = true
	m.browseSnapIdx = 0
	for _, width := range []int{60, 80, 95, 100, 105, 120} {
		m.width, m.height = width, 24
		for _, line := range strings.Split(m.renderBrowseContent(width), "\n") {
			if w := lipglossWidth(line); w > width {
				t.Errorf("width %d: a rendered line is %d cells wide:\n%s", width, w, stripANSI(line))
			}
		}
	}
}

// stripANSI removes colour escape codes so tests can assert on plain text.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case inEscape:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
		case r == '\x1b':
			inEscape = true
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Guard against the fixture drifting: the shared time base must stay fixed or
// the young-syncoid verdicts above become flaky.
func TestBrowseFixtureNowIsFixed(t *testing.T) {
	if browseFixtureNow.IsZero() || !browseFixtureNow.Equal(time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)) {
		t.Error("browseFixtureNow must stay a fixed instant")
	}
}
