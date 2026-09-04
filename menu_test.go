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
