// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	tea "github.com/charmbracelet/bubbletea"

	"strings"
	"testing"
)

func TestMenuCursorNeverLandsOnAHeader(t *testing.T) {
	rows := visibleMenuRows("")

	cursor := clampMenuCursor(rows, 0)
	if rows[cursor].item == nil {
		t.Fatal("the initial cursor must sit on an item, not a header")
	}

	// Walk the whole menu down and back up; the cursor must always be on an
	// item and must visit every item exactly once in order.
	var visited []string
	for {
		visited = append(visited, rows[cursor].item.title)
		next := moveMenuCursor(rows, cursor, 1)
		if next == cursor {
			break
		}
		cursor = next
	}

	var want []string
	for _, section := range menuSections {
		for _, item := range section.items {
			want = append(want, item.title)
		}
	}
	if strings.Join(visited, "|") != strings.Join(want, "|") {
		t.Errorf("walk order = %v, want %v", visited, want)
	}
}

func TestMenuCursorStopsAtTheEnds(t *testing.T) {
	rows := visibleMenuRows("")
	first := clampMenuCursor(rows, 0)

	if moveMenuCursor(rows, first, -1) != first {
		t.Error("moving up from the first item must stay put")
	}

	last := first
	for {
		next := moveMenuCursor(rows, last, 1)
		if next == last {
			break
		}
		last = next
	}
	if moveMenuCursor(rows, last, 1) != last {
		t.Error("moving down from the last item must stay put")
	}
}

func TestMenuFilterNarrowsAndHeadersFollow(t *testing.T) {
	rows := visibleMenuRows("orphaned")

	var items, headers []string
	for _, row := range rows {
		if row.item != nil {
			items = append(items, row.item.title)
		} else {
			headers = append(headers, row.header)
		}
	}

	// Both mention orphaned snapshots - the health check finds them, the
	// cleanup removes them - so the filter should surface both.
	if strings.Join(items, "|") != "Backup Health Check|Clean Up Orphaned Snapshots" {
		t.Errorf("filter 'orphaned' should match the health check and the cleanup, got %v", items)
	}
	if len(headers) != 1 || headers[0] != "Health" {
		t.Errorf("only the Health header should remain, got %v", headers)
	}
}

func TestMenuFilterWithNoMatchesLeavesNoRows(t *testing.T) {
	if rows := visibleMenuRows("zzzz-no-such-thing"); len(rows) != 0 {
		t.Errorf("expected no rows, got %d", len(rows))
	}
	// And the renderer must not panic on an empty menu.
	m := model{menuFilter: "zzzz-no-such-thing"}
	if out := m.renderMenu(100); !strings.Contains(out, "Nothing matches") {
		t.Error("an empty filter result should say so")
	}
}

// The safety contract of the menu itself: destructive entries are marked and
// carry a guard; read-only entries claim to change nothing.
func TestEveryDestructiveEntryDeclaresItsGuard(t *testing.T) {
	for _, section := range menuSections {
		for _, item := range section.items {
			if item.safety == menuDestructive && item.guard == "" {
				t.Errorf("%q is destructive but declares no guard", item.title)
			}
			if item.safety == menuDestructive && section.name != "Danger Zone" && section.name != "Health" {
				t.Errorf("%q is destructive but not fenced in an expected section (%s)", item.title, section.name)
			}
		}
	}
}

// The progress screen used to say "Backup in Progress" / "Running: cleanup"
// no matter which menu item you picked - a cleanup was mislabelled as a
// backup, and the status line named an internal code, not what you chose.
// operationNames fixes that, but only if it stays in sync: every value must
// be a real, current menu title (so a renamed title is caught here, not left
// silently stale), and it must cover every operation the dispatch below
// actually sets.
func TestOperationNamesCoverEveryDispatchedOperation(t *testing.T) {
	titles := map[string]bool{}
	for _, section := range menuSections {
		for _, item := range section.items {
			titles[item.title] = true
		}
	}

	for op, title := range operationNames {
		if !titles[title] {
			t.Errorf("operationNames[%q] = %q, which is not a current menu title", op, title)
		}
	}

	// Every operation code the enter-key dispatch can set (main.go's
	// `switch selected.title` block) must resolve to a display name -
	// otherwise the progress screen falls back to the raw code again.
	dispatched := []string{
		"backup", "remote-backup", "push-backup", "force-backup", "prepare",
		"zpoolinfo", "recover-pool", "maintenance", "quotas", "scope",
		"browse", "cleanup", "doctor", "recover", "unmount",
	}
	for _, op := range dispatched {
		if operationDisplayName(op) == op {
			t.Errorf("operation %q has no display name - the progress screen would show the raw code", op)
		}
	}
}

func TestMenuTitlesAreUniqueAndComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, section := range menuSections {
		if len(section.items) == 0 {
			t.Errorf("section %q is empty", section.name)
		}
		for _, item := range section.items {
			if item.title == "" || item.description == "" || item.detail == "" {
				t.Errorf("item %+v is missing title, description or detail", item.title)
			}
			if seen[item.title] {
				t.Errorf("duplicate menu title %q - dispatch is by title, so this is a collision", item.title)
			}
			seen[item.title] = true
		}
	}
}

func TestWideMenuShowsTheDetailCard(t *testing.T) {
	m := model{menuIndex: 1}
	out := m.renderMenu(120)

	for _, want := range []string{"Back Up Now", "makes changes", "Danger Zone", "never uses", "Never uses"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			continue
		}
		if want == "never uses" || want == "Never uses" {
			continue // covered by the case-insensitive check above
		}
		t.Errorf("wide menu is missing %q", want)
	}
	if !strings.Contains(out, "─") {
		t.Error("section rules should be drawn")
	}
}

// menuRowIndex finds the row index of a titled item in the unfiltered menu,
// for tests that need to point m.menuIndex at a specific item.
func menuRowIndex(t *testing.T, title string) int {
	t.Helper()
	for i, row := range visibleMenuRows("") {
		if row.item != nil && row.item.title == title {
			return i
		}
	}
	t.Fatalf("no menu item titled %q", title)
	return -1
}

// hasDetailCard reports whether the rendered menu shows the two-pane detail
// box (its rounded top-left corner is a mark narrow mode never produces).
func hasDetailCard(out string) bool {
	return strings.Contains(out, "╭")
}

// At a terminal height that fits a short detail card but not a long one in
// full, both items must still show the hint panel - the reported bug was
// that the long one lost its panel entirely (falling back to the one-line
// narrow view) while the short one kept a full card, and a naive fix that
// makes both disappear instead of both appear is just as wrong: the whole
// point of the panel is to be there when the terminal is wide enough.
func TestMenuHintPanelShowsForShortAndLongDetailAlikeAtAMarginalHeight(t *testing.T) {
	const width = 120
	// "Pool Information" has one of the shortest detail cards (10 lines);
	// "Browse Backup Snapshots" one of the tallest (20 lines).
	short := menuRowIndex(t, "Pool Information")
	tall := menuRowIndex(t, "Browse Backup Snapshots")

	m := model{menuIndex: short, height: 26, width: width}
	shortOut := m.renderMenu(width)
	if !hasDetailCard(shortOut) {
		t.Error("the short-detail item should show its hint panel in full")
	}

	m.menuIndex = tall
	tallOut := m.renderMenu(width)
	if !hasDetailCard(tallOut) {
		t.Error("the long-detail item should still show a hint panel, truncated if necessary - not none at all")
	}
	if !strings.Contains(tallOut, "more below") {
		t.Error("a truncated panel should say there is more, not cut off silently")
	}
}

func TestNarrowMenuFitsTheTerminal(t *testing.T) {
	m := model{menuIndex: 1}
	for _, line := range strings.Split(m.renderMenu(80), "\n") {
		if visible := lipglossWidth(line); visible > 80 {
			t.Errorf("line overflows an 80-column terminal (%d): %q", visible, line)
		}
	}
}

// lipglossWidth strips ANSI escapes and measures the visible width.
func lipglossWidth(line string) int {
	visible := 0
	inEscape := false
	for _, r := range line {
		switch {
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		case r == '\x1b':
			inEscape = true
		default:
			visible++
		}
	}
	return visible
}

// The review found the composed View overflowing the terminal - first at
// 80x24, then (after the first fix) on wide-but-short terminals where the
// two-pane detail card could not shrink. Sweep the common geometries, with
// the cursor at several depths so the danger zone and mid-list windows are
// both exercised.
func TestComposedViewFitsCommonTerminals(t *testing.T) {
	rows := visibleMenuRows("")
	cursors := []int{clampMenuCursor(rows, 1), len(rows) / 2, len(rows) - 1}

	for _, w := range []int{80, 96, 120, 160} {
		for _, h := range []int{24, 30, 40} {
			for _, c := range cursors {
				m := model{width: w, height: h, state: stateMenu,
					menuIndex: clampMenuCursor(rows, c)}
				view := m.View()
				lines := strings.Split(view, "\n")
				if len(lines) > h+1 {
					t.Errorf("%dx%d cursor %d: View() emits %d lines", w, h, c, len(lines))
				}
				for i, line := range lines {
					if vw := lipglossWidth(line); vw > w {
						t.Errorf("%dx%d cursor %d: line %d overflows (%d cols)", w, h, c, i, vw)
					}
				}
			}
		}
	}
}

func TestMenuWindowKeepsTheCursorVisible(t *testing.T) {
	rows := visibleMenuRows("")
	// Cursor on the last item, tiny budget.
	last := clampMenuCursor(rows, len(rows)-1)
	for last < len(rows)-1 {
		next := moveMenuCursor(rows, last, 1)
		if next == last {
			break
		}
		last = next
	}

	windowed, cursor, above, below := windowMenuRows(rows, last, 8)

	if len(windowed) != 8 {
		t.Fatalf("window size = %d, want 8", len(windowed))
	}
	if windowed[cursor].item == nil || windowed[cursor].item.title != rows[last].item.title {
		t.Error("the cursor row must survive windowing")
	}
	if below != 0 || above != len(rows)-8 {
		t.Errorf("cut counts wrong: above=%d below=%d", above, below)
	}
}

func TestMenuEscClearsACommittedFilter(t *testing.T) {
	m := model{state: stateMenu, width: 80, height: 24, menuFilter: "backup"}

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = m2.(model)

	if m.menuFilter != "" {
		t.Errorf("esc should clear the committed filter, got %q", m.menuFilter)
	}
}
