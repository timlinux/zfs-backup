// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// =============================================================================
// Main menu - sections, safety levels, filtering, two-pane rendering
// =============================================================================
//
// The menu is the map of the whole application, so it carries the safety
// story: every entry declares whether it reads, changes, or destroys, and the
// detail pane says exactly what it will and will not touch before the user
// commits to anything.

// menuSafety is how much damage an operation can do.
type menuSafety int

const (
	// menuReadOnly operations change nothing.
	menuReadOnly menuSafety = iota
	// menuWrites operations create or change data in the ordinary course of
	// their job - snapshots, replication, configuration.
	menuWrites
	// menuDestructive operations delete data and are gated behind typed
	// confirmations.
	menuDestructive
)

// menuItem is one operation on the menu.
type menuItem struct {
	title       string
	description string // one-liner under the list on narrow terminals
	detail      string // the honest contract, shown in the detail pane
	guard       string // what stands between enter and the damage
	safety      menuSafety
}

// menuSection groups related operations.
type menuSection struct {
	name  string
	items []menuItem
}

// menuSections is the whole menu. Order is workflow order: the things done
// daily first, the things done once at the bottom, behind a marked fence.
var menuSections = []menuSection{
	{
		name: "Back Up",
		items: []menuItem{
			{
				title:       "Back Up Now",
				description: "Incremental backup of the datasets in scope to the backup pool",
				detail:      "Snapshots each dataset in your backup scope, replicates the changes to the backup pool with syncoid, and prunes old backup snapshots on both sides.\n\nTouches only the datasets in your backup scope. Never uses recursive snapshots. Never deletes anything of yours - only its own dated -Backup snapshots.",
				safety:      menuWrites,
			},
			{
				title:       "Push Backup to Remote",
				description: "Send local snapshots to a backup pool on another machine via SSH",
				detail:      "Replicates the datasets in scope to a pool on a remote server, namespaced by this machine's hostname.\n\nNeeds SSH key access to the remote host. The remote pool is written to; your local data is only read.",
				safety:      menuWrites,
			},
			{
				title:       "Pull Backup From Remote",
				description: "Fetch another machine's datasets onto the local backup pool",
				detail:      "Pulls snapshots from a remote host onto the local backup pool, namespaced by the remote hostname.\n\nThe remote machine is only read. syncoid's own sync snapshot is kept there as the replication base.",
				safety:      menuWrites,
			},
		},
	},
	{
		name: "Restore",
		items: []menuItem{
			{
				title:       "Restore Files",
				description: "Browse snapshots and copy files back out",
				detail:      "Mounts snapshots read-only, lets you browse them side by side with the live filesystem, and copies files where you ask.\n\nSnapshots are never modified. Existing files at the destination are only overwritten after you confirm each conflict.",
				safety:      menuWrites,
			},
			{
				title:       "Browse Backup Reports",
				description: "Read previous backup reports: timings, sizes, errors",
				detail:      "Opens the reports previous runs wrote. Changes nothing.",
				safety:      menuReadOnly,
			},
		},
	},
	{
		name: "Health",
		items: []menuItem{
			{
				title:       "Backup Health Check",
				description: "Find orphaned snapshots and quota pressure",
				detail:      "Scans for snapshots nothing will ever prune - debris from versions before 2.0 - and datasets whose quota is filling with snapshots.\n\nCompletely read-only. Press c on the report to go straight to the cleanup.",
				safety:      menuReadOnly,
			},
			{
				title:       "Clean Up Orphaned Snapshots",
				description: "Reclaim space from snapshots older versions left behind",
				detail:      "Destroys the orphaned snapshots the health check finds, after showing a full dry run.\n\nNever touches datasets in your backup scope, snapshots with holds or clones, @blank, or anything that is not zfs-backup's own naming pattern.",
				guard:       "dry run first, then typed DESTROY",
				safety:      menuDestructive,
			},
			{
				title:       "Recover Failed Backup",
				description: "Fix broken sync state after an interrupted backup",
				detail:      "Clears partial receive state and re-establishes the incremental chain after a backup was interrupted.\n\nWorks on the backup pool; the source is only read.",
				safety:      menuWrites,
			},
			{
				title:       "Fix a Pool That Stopped Responding",
				description: "Recover a pool ZFS has suspended - usually a dropped drive",
				detail:      "Diagnoses a suspended pool and walks the remedies for you, gentlest first, re-checking after each.\n\nNothing in the ladder destroys data. The forceful step asks before it runs.",
				safety:      menuWrites,
			},
		},
	},
	{
		name: "Pools",
		items: []menuItem{
			{
				title:       "Backup Scope",
				description: "Choose which datasets are backed up",
				detail:      "Tick the datasets you want backed up. Only those are ever snapshotted, replicated and pruned - everything else on the pool is left completely untouched.",
				safety:      menuWrites,
			},
			{
				title:       "Pool Information",
				description: "Structure, status and health of a pool",
				detail:      "Shows zpool status, properties, and layout. Changes nothing.",
				safety:      menuReadOnly,
			},
			{
				title:       "Pool Maintenance",
				description: "Start, stop or monitor scrubs",
				detail:      "Runs ZFS scrubs, which verify every block against its checksum and repair from redundancy where possible.\n\nScrubs read the whole pool but only ever repair; they never discard data.",
				safety:      menuWrites,
			},
			{
				title:       "Manage Datasets",
				description: "View and edit quotas, create and delete datasets",
				detail:      "Edit dataset quotas and create datasets. Deleting a dataset is confirmed per dataset before anything happens.",
				safety:      menuWrites,
			},
			{
				title:       "Unmount Backup Disk",
				description: "Export the backup pool and power the drive off",
				detail:      "Cleanly exports the backup pool and spins the USB drive down so it can be unplugged. Nothing is deleted.",
				safety:      menuWrites,
			},
		},
	},
	{
		name: "Danger Zone",
		items: []menuItem{
			{
				title:       "Prepare Backup Device",
				description: "Erase a disk and create a new encrypted backup pool on it",
				detail:      "Completely erases a disk and builds a new encrypted ZFS pool on it.\n\nThe disk is chosen from a vetted picker: anything mounted, in use by an imported pool, or holding the running system is listed but cannot be chosen.",
				guard:       "vetted picker, then type the disk's own name",
				safety:      menuDestructive,
			},
			{
				title:       "Force Full Backup",
				description: "Delete backup history and rebuild from current state",
				detail:      "Deletes every existing backup snapshot on the backup pool and replicates everything again from the current source state.\n\nOlder versions of files held only by those snapshots are lost. Only for when the incremental chain is broken.",
				guard:       "typed DESTROY once the pool is chosen",
				safety:      menuDestructive,
			},
		},
	},
}

// menuRow is one visible line of the menu: a section header or an item.
type menuRow struct {
	header string
	item   *menuItem
}

// visibleMenuRows flattens the sections into rows, applying the filter. With
// a filter, only matching items (and the headers of sections that still have
// matches) remain.
func visibleMenuRows(filter string) []menuRow {
	needle := strings.ToLower(strings.TrimSpace(filter))
	var rows []menuRow
	for s := range menuSections {
		section := &menuSections[s]
		var matched []*menuItem
		for i := range section.items {
			item := &section.items[i]
			if needle == "" ||
				strings.Contains(strings.ToLower(item.title), needle) ||
				strings.Contains(strings.ToLower(item.description), needle) {
				matched = append(matched, item)
			}
		}
		if len(matched) == 0 {
			continue
		}
		rows = append(rows, menuRow{header: section.name})
		for _, item := range matched {
			rows = append(rows, menuRow{item: item})
		}
	}
	return rows
}

// moveMenuCursor moves the cursor by delta, skipping headers. If there is no
// selectable row in that direction it stays put.
func moveMenuCursor(rows []menuRow, current, delta int) int {
	i := current + delta
	for i >= 0 && i < len(rows) && rows[i].item == nil {
		i += delta
	}
	if i < 0 || i >= len(rows) || rows[i].item == nil {
		return current
	}
	return i
}

// clampMenuCursor puts an out-of-range or header-pointing cursor onto the
// first selectable row.
func clampMenuCursor(rows []menuRow, current int) int {
	if current >= 0 && current < len(rows) && rows[current].item != nil {
		return current
	}
	for i, row := range rows {
		if row.item != nil {
			return i
		}
	}
	return 0
}

// currentMenuItem returns the item under the cursor, if any.
func (m model) currentMenuItem() (menuItem, bool) {
	rows := visibleMenuRows(m.menuFilter)
	idx := clampMenuCursor(rows, m.menuIndex)
	if idx < len(rows) && rows[idx].item != nil {
		return *rows[idx].item, true
	}
	return menuItem{}, false
}

// safetyBadge renders the safety level as a small labelled chip.
func safetyBadge(s menuSafety) string {
	switch s {
	case menuReadOnly:
		return statusStyle.Render("● read-only")
	case menuDestructive:
		return errorStyle.Render("● destructive")
	default:
		return infoStyle.Render("● makes changes")
	}
}

// menuListWidth is the left pane width in two-pane mode.
const menuListWidth = 44

// twoPaneMinWidth is the narrowest terminal that gets the detail pane.
const twoPaneMinWidth = 96

// renderMenu draws the menu: two panes when the terminal allows, a single
// grouped list when it does not. Never overflows horizontally.
func (m model) renderMenu(width int) string {
	rows := visibleMenuRows(m.menuFilter)
	cursor := clampMenuCursor(rows, m.menuIndex)

	var list strings.Builder

	if m.menuFiltering || m.menuFilter != "" {
		list.WriteString(labelStyle.Render(" / " + m.menuFilter))
		if m.menuFiltering {
			list.WriteString(selectedItemStyle.Render("▎"))
		}
		list.WriteString("\n\n")
	}

	if len(rows) == 0 {
		list.WriteString(subtitleStyle.Render("  Nothing matches. Esc clears the filter."))
		list.WriteString("\n")
	}

	for i, row := range rows {
		if row.item == nil {
			if i > 0 {
				list.WriteString("\n")
			}
			header := row.header
			style := labelStyle
			if header == "Danger Zone" {
				style = errorStyle
			}
			list.WriteString(style.Render(" " + header))
			list.WriteString(" " + subtitleStyle.Render(strings.Repeat("─", max(2, menuListWidth-len(header)-3))))
			list.WriteString("\n")
			continue
		}

		marker := "  "
		line := row.item.title
		if row.item.safety == menuDestructive {
			line += " " + errorStyle.Render("!")
		}
		if i == cursor {
			list.WriteString(selectedItemStyle.Render(" ▌ " + line))
		} else {
			list.WriteString(marker + " " + infoStyle.Render(line))
		}
		list.WriteString("\n")
	}

	if width < twoPaneMinWidth {
		// Narrow terminal: grouped list plus the one-line description.
		var b strings.Builder
		b.WriteString(list.String())
		if item, ok := m.currentMenuItem(); ok {
			b.WriteString("\n")
			b.WriteString(safetyBadge(item.safety))
			b.WriteString("\n")
			b.WriteString(subtitleStyle.Render(item.description))
			b.WriteString("\n")
		}
		return lipgloss.PlaceHorizontal(width, lipgloss.Center,
			lipgloss.NewStyle().Width(menuListWidth+8).Render(b.String()))
	}

	// Detail pane for the highlighted item.
	var card strings.Builder
	if item, ok := m.currentMenuItem(); ok {
		card.WriteString(selectedItemStyle.Render(item.title))
		card.WriteString("\n")
		card.WriteString(safetyBadge(item.safety))
		card.WriteString("\n\n")
		card.WriteString(item.detail)
		card.WriteString("\n")
		if item.guard != "" {
			card.WriteString("\n")
			card.WriteString(warningStyle.Render("Guard: " + item.guard))
			card.WriteString("\n")
		}
	}

	detailWidth := min(64, width-menuListWidth-8)
	left := lipgloss.NewStyle().Width(menuListWidth).Render(list.String())
	right := reportBoxStyle.Width(detailWidth).Render(card.String())
	joined := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)

	return lipgloss.PlaceHorizontal(width, lipgloss.Center, joined)
}

// menuHotkeys is the footer for the menu state.
func (m model) menuHotkeys() string {
	if m.menuFiltering {
		return "type to filter • ↑/↓ move • enter select • esc clear"
	}
	return "↑/k up • ↓/j down • enter select • / filter • ? help • q quit"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
