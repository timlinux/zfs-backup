package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// =============================================================================
// Restore Mode Types
// =============================================================================

// RestoreState represents the current state within restore mode
type RestoreState int

const (
	restoreSelectSource RestoreState = iota
	restorePasswordSource
	restoreExplorer
	restoreCopying
	restoreConfirmOverwrite
	restoreComplete
)

// PanelFocus indicates which panel has focus
type PanelFocus int

const (
	focusLeft PanelFocus = iota
	focusRight
)

// SortMode for snapshot/file listing
type SortMode int

const (
	sortByName SortMode = iota
	sortByDate
	sortBySize
)

// FileEntry represents a file or directory in the browser
type FileEntry struct {
	Name     string
	Path     string
	Size     int64
	ModTime  time.Time
	IsDir    bool
	Selected bool
}

// SnapshotEntry represents a ZFS snapshot
type SnapshotEntry struct {
	Name      string
	Dataset   string
	Creation  time.Time
	Used      string
	Referenced string
}

// RestoreModel holds the state for restore mode
type RestoreModel struct {
	state           RestoreState
	sourcePool      string
	sourcePassword  string
	passwordInput   textinput.Model

	// Explorer state
	focus           PanelFocus
	snapshots       []SnapshotEntry
	snapshotIndex   int
	snapshotOffset  int  // For scrolling snapshot list
	currentSnapshot string
	snapshotMounted bool
	mountPoint      string

	// Left panel (source/snapshot browser)
	leftPath        string
	leftEntries     []FileEntry
	leftIndex       int
	leftOffset      int  // For scrolling

	// Right panel (destination browser)
	rightPath       string
	rightEntries    []FileEntry
	rightIndex      int
	rightOffset     int

	// Selection
	selectedFiles   []FileEntry

	// Search/filter
	searchMode      bool
	searchInput     textinput.Model
	searchQuery     string

	// Mkdir mode
	mkdirMode       bool
	mkdirInput      textinput.Model
	focusAfterLoad  string  // Name of file/directory to focus after reload

	// Restore mode selection
	restoreModeSelect bool  // true when showing restore mode dialog
	restoreToOriginal bool  // true = restore to original location, false = restore to current folder

	// Sort
	sortMode        SortMode
	sortAscending   bool

	// Copy progress
	copyProgress    progress.Model
	copyingFile     string
	copyTotal       int64
	copyCurrent     int64
	copyQueue       []FileEntry

	// Confirmation
	confirmFiles    []FileEntry
	confirmYes      bool

	// Dimensions
	width           int
	height          int

	// For returning to main menu
	done            bool
	returnToMenu    bool
}

// =============================================================================
// Restore Mode Styles
// =============================================================================

var (
	// Panel styles
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorHighlight3).
			Padding(0, 1)

	panelActiveStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorHighlight1).
				Padding(0, 1)

	panelHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorHighlight2).
				Background(lipgloss.Color("#2A2A2A")).
				Padding(0, 1)

	// File entry styles
	fileEntryStyle = lipgloss.NewStyle().
			Foreground(colorForeground)

	fileEntrySelectedStyle = lipgloss.NewStyle().
				Foreground(colorHighlight1).
				Bold(true)

	fileEntryCursorStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#3A3A3A"))

	fileEntryDirStyle = lipgloss.NewStyle().
				Foreground(colorHighlight2).
				Bold(true)

	fileEntryMarkedStyle = lipgloss.NewStyle().
				Foreground(colorHighlight4)

	// Snapshot styles
	snapshotStyle = lipgloss.NewStyle().
			Foreground(colorHighlight2)

	snapshotSelectedStyle = lipgloss.NewStyle().
				Foreground(colorHighlight1).
				Bold(true)

	// Status bar
	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorHighlight3).
			Background(lipgloss.Color("#2A2A2A")).
			Padding(0, 1)

	// Dialog box style
	dialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorHighlight1).
			Background(lipgloss.Color("#1A1A1A")).
			Padding(1, 2)

	// Subtle text style
	subtleStyle = lipgloss.NewStyle().
			Foreground(colorHighlight3)

	// Size column
	sizeStyle = lipgloss.NewStyle().
			Foreground(colorHighlight3).
			Width(10).
			Align(lipgloss.Right)

	// Date column
	dateStyle = lipgloss.NewStyle().
			Foreground(colorHighlight3).
			Width(16)
)

// =============================================================================
// Restore Model Initialization
// =============================================================================

// NewRestoreModel creates a new restore model
func NewRestoreModel() RestoreModel {
	pi := textinput.New()
	pi.Placeholder = "Enter encryption password"
	pi.EchoMode = textinput.EchoPassword
	pi.EchoCharacter = '•'
	pi.CharLimit = 256
	pi.Width = 40

	si := textinput.New()
	si.Placeholder = "Search..."
	si.CharLimit = 100
	si.Width = 30

	mi := textinput.New()
	mi.Placeholder = "New folder name"
	mi.CharLimit = 255
	mi.Width = 40

	prog := progress.New(
		progress.WithGradient(string(colorHighlight2), string(colorHighlight1)),
	)

	// Start right panel at user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/"
	}

	return RestoreModel{
		state:         restoreSelectSource,
		passwordInput: pi,
		searchInput:   si,
		mkdirInput:    mi,
		copyProgress:  prog,
		sortMode:      sortByDate,
		sortAscending: false,
		focus:         focusLeft,
		leftPath:      "/",
		rightPath:     homeDir,
	}
}

// =============================================================================
// Restore Mode Update
// =============================================================================

// RestoreUpdate handles updates for restore mode
func (m RestoreModel) Update(msg tea.Msg) (RestoreModel, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.copyProgress.Width = msg.Width / 2
		return m, nil

	case tea.KeyMsg:
		// Global keys
		switch msg.String() {
		case "ctrl+c":
			m.done = true
			m.returnToMenu = false
			return m, tea.Quit
		case "esc":
			// Handle esc for various modes - go back one level at a time
			if m.searchMode {
				m.searchMode = false
				m.searchQuery = ""
				return m, nil
			}
			if m.mkdirMode {
				m.mkdirMode = false
				return m, nil
			}
			if m.restoreModeSelect {
				m.restoreModeSelect = false
				return m, nil
			}
			// Handle esc based on current state
			switch m.state {
			case restoreSelectSource:
				// At top level of restore - return to main menu
				m.done = true
				m.returnToMenu = true
				return m, nil
			case restorePasswordSource:
				// Go back to pool selection
				m.state = restoreSelectSource
				m.passwordInput.SetValue("")
				return m, nil
			case restoreExplorer:
				// If inside a snapshot, go back to snapshot list
				if m.currentSnapshot != "" {
					m.currentSnapshot = ""
					m.snapshotMounted = false
					m.leftEntries = nil
					m.leftIndex = 0
					m.leftOffset = 0
					m.leftPath = "/"
					m.selectedFiles = nil
					return m, m.unmountSnapshot()
				}
				// Otherwise go back to pool selection
				m.state = restoreSelectSource
				m.snapshotIndex = 0
				m.snapshotOffset = 0
				m.snapshots = nil
				return m, nil
			case restoreConfirmOverwrite:
				// Go back to explorer
				m.state = restoreExplorer
				m.confirmFiles = nil
				return m, nil
			case restoreComplete:
				// Return to main menu
				m.done = true
				m.returnToMenu = true
				return m, nil
			default:
				// Default: return to main menu
				if m.snapshotMounted && m.mountPoint != "" {
					go unmountZFSSnapshot(m.mountPoint)
				}
				m.done = true
				m.returnToMenu = true
				return m, nil
			}
		}

		// Handle mkdir mode
		if m.mkdirMode {
			return m.updateMkdirInput(msg)
		}

		// Handle restore mode selection
		if m.restoreModeSelect {
			return m.updateRestoreModeSelect(msg)
		}

		// State-specific handling
		switch m.state {
		case restoreSelectSource:
			return m.updatePoolSelection(msg)
		case restorePasswordSource:
			return m.updatePasswordInput(msg)
		case restoreExplorer:
			return m.updateExplorer(msg)
		case restoreCopying:
			// Can't interact during copy
			return m, nil
		case restoreConfirmOverwrite:
			return m.updateConfirmOverwrite(msg)
		case restoreComplete:
			switch msg.String() {
			case "enter", "q":
				m.done = true
				m.returnToMenu = true
			case "u":
				// Unmount option
				return m, m.unmountAndPowerOff()
			}
		}

	case snapshotsLoadedMsg:
		m.snapshots = msg.snapshots
		return m, nil

	case filesLoadedMsg:
		if msg.isLeft {
			m.leftEntries = msg.entries
			m.leftIndex = 0
			m.leftOffset = 0
		} else {
			m.rightEntries = msg.entries
			m.rightIndex = 0
			m.rightOffset = 0
			// Focus on specific file if requested (e.g., after mkdir)
			if m.focusAfterLoad != "" {
				for i, entry := range msg.entries {
					if entry.Name == m.focusAfterLoad {
						m.rightIndex = i
						// Adjust offset if needed to keep focused item in view
						visibleHeight := m.height - 10 // approximate visible area
						if m.rightIndex >= visibleHeight {
							m.rightOffset = m.rightIndex - visibleHeight/2
						}
						break
					}
				}
				m.focusAfterLoad = ""
			}
		}
		return m, nil

	case snapshotMountedMsg:
		m.snapshotMounted = true
		m.mountPoint = msg.mountPoint
		m.leftPath = msg.mountPoint
		return m, m.loadFiles(m.leftPath, true)

	case copyProgressMsg:
		m.copyingFile = msg.file
		m.copyCurrent = msg.current
		m.copyTotal = msg.total
		if msg.done {
			// Return to explorer view and refresh right panel
			m.state = restoreExplorer
			m.selectedFiles = nil  // Clear selections
			m.copyProgress = progress.New(progress.WithDefaultGradient())
			// Refresh the right panel to show restored files
			return m, m.loadFiles(m.rightPath, false)
		}
		cmds = append(cmds, m.copyProgress.SetPercent(float64(msg.current)/float64(msg.total)))
		return m, tea.Batch(cmds...)

	case progress.FrameMsg:
		progressModel, cmd := m.copyProgress.Update(msg)
		m.copyProgress = progressModel.(progress.Model)
		return m, cmd

	case poolUnlockedMsg:
		if m.state == restorePasswordSource {
			// Go directly to explorer after unlocking source pool
			m.state = restoreExplorer
			m.snapshotIndex = 0
			m.snapshotOffset = 0
			return m, tea.Batch(m.loadSnapshots(), m.loadFiles(m.rightPath, false))
		}
		return m, nil

	case unmountCompleteMsg:
		// Just continue
		return m, nil
	}

	// Update text inputs
	if m.state == restorePasswordSource {
		m.passwordInput, cmd = m.passwordInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.searchMode {
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.mkdirMode {
		m.mkdirInput, cmd = m.mkdirInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// updateMkdirInput handles mkdir input
func (m RestoreModel) updateMkdirInput(msg tea.KeyMsg) (RestoreModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		dirname := m.mkdirInput.Value()
		if dirname != "" {
			newPath := filepath.Join(m.rightPath, dirname)
			err := os.MkdirAll(newPath, 0755)
			if err == nil {
				// Refresh directory listing and focus on new folder
				m.mkdirMode = false
				m.focusAfterLoad = dirname
				return m, m.loadFiles(m.rightPath, false)
			}
		}
		m.mkdirMode = false
		return m, nil
	default:
		// Pass all other keys to the text input
		var cmd tea.Cmd
		m.mkdirInput, cmd = m.mkdirInput.Update(msg)
		return m, cmd
	}
}

// updateRestoreModeSelect handles restore mode selection dialog
func (m RestoreModel) updateRestoreModeSelect(msg tea.KeyMsg) (RestoreModel, tea.Cmd) {
	switch msg.String() {
	case "up", "k", "left", "h":
		m.restoreToOriginal = true
	case "down", "j", "right", "l":
		m.restoreToOriginal = false
	case "o":
		m.restoreToOriginal = true
		m.restoreModeSelect = false
		return m.startCopy()
	case "c":
		m.restoreToOriginal = false
		m.restoreModeSelect = false
		return m.startCopy()
	case "enter":
		m.restoreModeSelect = false
		return m.startCopy()
	}
	return m, nil
}

// updatePoolSelection handles pool selection state (source pool only)
func (m RestoreModel) updatePoolSelection(msg tea.KeyMsg) (RestoreModel, tea.Cmd) {
	pools := getAllPools()

	switch msg.String() {
	case "up", "k":
		if m.snapshotIndex > 0 {
			m.snapshotIndex--
		}
	case "down", "j":
		if m.snapshotIndex < len(pools)-1 {
			m.snapshotIndex++
		}
	case "enter":
		if len(pools) > 0 {
			selectedPool := pools[m.snapshotIndex]
			m.sourcePool = selectedPool
			// Check if pool needs unlocking
			if needsUnlock, _ := poolNeedsUnlock(selectedPool); needsUnlock {
				m.state = restorePasswordSource
				m.passwordInput.SetValue("")
				m.passwordInput.Focus()
				return m, textinput.Blink
			}
			// Go directly to explorer - right panel shows local filesystem
			m.state = restoreExplorer
			m.snapshotIndex = 0
			m.snapshotOffset = 0
			return m, tea.Batch(m.loadSnapshots(), m.loadFiles(m.rightPath, false))
		}
	}
	return m, nil
}

// updatePasswordInput handles password entry
func (m RestoreModel) updatePasswordInput(msg tea.KeyMsg) (RestoreModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		password := m.passwordInput.Value()
		if password != "" {
			m.sourcePassword = password
			return m, m.unlockPool(m.sourcePool, password)
		}
	}
	return m, nil
}

// updateExplorer handles the dual-panel explorer
func (m RestoreModel) updateExplorer(msg tea.KeyMsg) (RestoreModel, tea.Cmd) {
	switch msg.String() {
	// Navigation
	case "up", "k":
		m.navigateUp()
	case "down", "j":
		m.navigateDown()
	case "left", "h":
		m.focus = focusLeft
	case "right", "l":
		m.focus = focusRight
	case "tab":
		if m.focus == focusLeft {
			m.focus = focusRight
		} else {
			m.focus = focusLeft
		}

	// Page navigation
	case "ctrl+u", "pgup":
		m.pageUp()
	case "ctrl+d", "pgdown":
		m.pageDown()
	case "g":
		m.goToTop()
	case "G":
		m.goToBottom()

	// Enter directory/snapshot
	case "enter":
		return m.handleEnter()

	// Go back
	case "backspace", "-":
		return m.handleBack()

	// Selection
	case " ":
		m.toggleSelection()

	// Yank (copy) - show restore mode selection
	case "y":
		if len(m.selectedFiles) > 0 || (m.focus == focusLeft && len(m.leftEntries) > 0 && m.leftEntries[m.leftIndex].Name != "..") {
			// If no selection, select current item
			if len(m.selectedFiles) == 0 {
				entry := m.leftEntries[m.leftIndex]
				if !entry.IsDir {
					m.selectedFiles = []FileEntry{entry}
				}
			}
			if len(m.selectedFiles) > 0 {
				m.restoreModeSelect = true
				m.restoreToOriginal = true // Default to original location
				return m, nil
			}
		}

	// Select all
	case "a":
		m.selectAll()

	// Clear selection
	case "c":
		m.clearSelection()

	// Search
	case "/":
		m.searchMode = true
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		return m, textinput.Blink

	// Make directory (only in right panel)
	case "m":
		if m.focus == focusRight {
			m.mkdirMode = true
			m.mkdirInput.SetValue("")
			m.mkdirInput.Focus()
			return m, textinput.Blink
		}

	// Sort
	case "s":
		m.cycleSortMode()
		m.sortEntries()

	case "r":
		m.sortAscending = !m.sortAscending
		m.sortEntries()

	// Unmount and quit
	case "u":
		return m, m.unmountAndPowerOff()

	case "q":
		m.done = true
		m.returnToMenu = true
	}

	return m, nil
}

// updateConfirmOverwrite handles overwrite confirmation
func (m RestoreModel) updateConfirmOverwrite(msg tea.KeyMsg) (RestoreModel, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.confirmYes = true
		m.state = restoreCopying
		return m, m.executeCopy()
	case "n", "N", "esc":
		m.confirmYes = false
		m.state = restoreExplorer
		m.confirmFiles = nil
	}
	return m, nil
}

// =============================================================================
// Navigation Helpers
// =============================================================================

func (m *RestoreModel) navigateUp() {
	if m.focus == focusLeft {
		if m.currentSnapshot == "" {
			// Snapshot list
			if m.snapshotIndex > 0 {
				m.snapshotIndex--
				// Scroll up if cursor goes above visible area
				if m.snapshotIndex < m.snapshotOffset {
					m.snapshotOffset = m.snapshotIndex
				}
			}
		} else {
			// File list
			if m.leftIndex > 0 {
				m.leftIndex--
				if m.leftIndex < m.leftOffset {
					m.leftOffset = m.leftIndex
				}
			}
		}
	} else {
		if m.rightIndex > 0 {
			m.rightIndex--
			if m.rightIndex < m.rightOffset {
				m.rightOffset = m.rightIndex
			}
		}
	}
}

func (m *RestoreModel) navigateDown() {
	visibleRows := m.getPanelHeight() - 4 // Account for borders and header

	if m.focus == focusLeft {
		if m.currentSnapshot == "" {
			// Snapshot list
			if m.snapshotIndex < len(m.snapshots)-1 {
				m.snapshotIndex++
				// Scroll down if cursor goes below visible area
				if m.snapshotIndex >= m.snapshotOffset+visibleRows {
					m.snapshotOffset = m.snapshotIndex - visibleRows + 1
				}
			}
		} else {
			// File list
			if m.leftIndex < len(m.leftEntries)-1 {
				m.leftIndex++
				if m.leftIndex >= m.leftOffset+visibleRows {
					m.leftOffset = m.leftIndex - visibleRows + 1
				}
			}
		}
	} else {
		if m.rightIndex < len(m.rightEntries)-1 {
			m.rightIndex++
			if m.rightIndex >= m.rightOffset+visibleRows {
				m.rightOffset = m.rightIndex - visibleRows + 1
			}
		}
	}
}

func (m *RestoreModel) pageUp() {
	visibleRows := m.getPanelHeight() - 4
	if m.focus == focusLeft {
		if m.currentSnapshot == "" {
			// Snapshot list
			m.snapshotIndex -= visibleRows
			if m.snapshotIndex < 0 {
				m.snapshotIndex = 0
			}
			m.snapshotOffset = m.snapshotIndex
		} else {
			// File list
			m.leftIndex -= visibleRows
			if m.leftIndex < 0 {
				m.leftIndex = 0
			}
			m.leftOffset = m.leftIndex
		}
	} else {
		m.rightIndex -= visibleRows
		if m.rightIndex < 0 {
			m.rightIndex = 0
		}
		m.rightOffset = m.rightIndex
	}
}

func (m *RestoreModel) pageDown() {
	visibleRows := m.getPanelHeight() - 4
	if m.focus == focusLeft {
		if m.currentSnapshot == "" {
			// Snapshot list
			m.snapshotIndex += visibleRows
			if m.snapshotIndex >= len(m.snapshots) {
				m.snapshotIndex = len(m.snapshots) - 1
			}
			if m.snapshotIndex < 0 {
				m.snapshotIndex = 0
			}
			// Adjust offset to keep cursor visible
			if m.snapshotIndex >= m.snapshotOffset+visibleRows {
				m.snapshotOffset = m.snapshotIndex - visibleRows + 1
			}
		} else {
			// File list
			m.leftIndex += visibleRows
			if m.leftIndex >= len(m.leftEntries) {
				m.leftIndex = len(m.leftEntries) - 1
			}
			if m.leftIndex < 0 {
				m.leftIndex = 0
			}
			m.leftOffset = m.leftIndex
		}
	} else {
		m.rightIndex += visibleRows
		if m.rightIndex >= len(m.rightEntries) {
			m.rightIndex = len(m.rightEntries) - 1
		}
		if m.rightIndex < 0 {
			m.rightIndex = 0
		}
		m.rightOffset = m.rightIndex
	}
}

func (m *RestoreModel) goToTop() {
	if m.focus == focusLeft {
		if m.currentSnapshot == "" {
			// Snapshot list
			m.snapshotIndex = 0
			m.snapshotOffset = 0
		} else {
			// File list
			m.leftIndex = 0
			m.leftOffset = 0
		}
	} else {
		m.rightIndex = 0
		m.rightOffset = 0
	}
}

func (m *RestoreModel) goToBottom() {
	visibleRows := m.getPanelHeight() - 4
	if m.focus == focusLeft {
		if m.currentSnapshot == "" {
			// Snapshot list
			m.snapshotIndex = len(m.snapshots) - 1
			if m.snapshotIndex < 0 {
				m.snapshotIndex = 0
			}
			// Adjust offset to show cursor at bottom of visible area
			if m.snapshotIndex >= visibleRows {
				m.snapshotOffset = m.snapshotIndex - visibleRows + 1
			}
		} else {
			// File list
			m.leftIndex = len(m.leftEntries) - 1
			if m.leftIndex < 0 {
				m.leftIndex = 0
			}
			// Adjust offset to show cursor at bottom of visible area
			if m.leftIndex >= visibleRows {
				m.leftOffset = m.leftIndex - visibleRows + 1
			}
		}
	} else {
		m.rightIndex = len(m.rightEntries) - 1
		if m.rightIndex < 0 {
			m.rightIndex = 0
		}
		// Adjust offset to show cursor at bottom of visible area
		if m.rightIndex >= visibleRows {
			m.rightOffset = m.rightIndex - visibleRows + 1
		}
	}
}

func (m *RestoreModel) getPanelHeight() int {
	// Total height minus header, footer, status bar
	return m.height - 12
}

func (m *RestoreModel) getPanelWidth() int {
	// Half width minus borders and padding
	return (m.width / 2) - 4
}

// =============================================================================
// Selection Helpers
// =============================================================================

func (m *RestoreModel) toggleSelection() {
	if m.focus != focusLeft || m.currentSnapshot == "" {
		return
	}
	if m.leftIndex < len(m.leftEntries) {
		entry := &m.leftEntries[m.leftIndex]

		// Don't allow selecting ".." entry
		if entry.Name == ".." {
			m.navigateDown()
			return
		}

		entry.Selected = !entry.Selected

		if entry.Selected {
			m.selectedFiles = append(m.selectedFiles, *entry)
		} else {
			// Remove from selection
			for i, f := range m.selectedFiles {
				if f.Path == entry.Path {
					m.selectedFiles = append(m.selectedFiles[:i], m.selectedFiles[i+1:]...)
					break
				}
			}
		}

		// Move down after selection
		m.navigateDown()
	}
}

func (m *RestoreModel) selectAll() {
	if m.focus != focusLeft || m.currentSnapshot == "" {
		return
	}
	m.selectedFiles = nil
	for i := range m.leftEntries {
		// Skip ".." and directories
		if m.leftEntries[i].Name == ".." || m.leftEntries[i].IsDir {
			continue
		}
		m.leftEntries[i].Selected = true
		m.selectedFiles = append(m.selectedFiles, m.leftEntries[i])
	}
}

func (m *RestoreModel) clearSelection() {
	for i := range m.leftEntries {
		m.leftEntries[i].Selected = false
	}
	m.selectedFiles = nil
}

// =============================================================================
// Sort Helpers
// =============================================================================

func (m *RestoreModel) cycleSortMode() {
	m.sortMode = (m.sortMode + 1) % 3
}

func (m *RestoreModel) sortEntries() {
	sortFunc := func(entries []FileEntry) {
		sort.Slice(entries, func(i, j int) bool {
			// ".." always first
			if entries[i].Name == ".." {
				return true
			}
			if entries[j].Name == ".." {
				return false
			}

			// Directories always before files
			if entries[i].IsDir != entries[j].IsDir {
				return entries[i].IsDir
			}

			var result bool
			switch m.sortMode {
			case sortByName:
				result = strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
			case sortByDate:
				result = entries[i].ModTime.After(entries[j].ModTime)
			case sortBySize:
				result = entries[i].Size > entries[j].Size
			}

			if !m.sortAscending {
				result = !result
			}
			return result
		})
	}

	if m.focus == focusLeft {
		sortFunc(m.leftEntries)
	} else {
		sortFunc(m.rightEntries)
	}
}

// =============================================================================
// Action Handlers
// =============================================================================

func (m RestoreModel) handleEnter() (RestoreModel, tea.Cmd) {
	if m.focus == focusLeft {
		if m.currentSnapshot == "" {
			// Enter snapshot
			if m.snapshotIndex < len(m.snapshots) {
				snap := m.snapshots[m.snapshotIndex]
				m.currentSnapshot = snap.Name
				return m, m.mountSnapshot(snap.Name)
			}
		} else {
			// Enter directory
			if m.leftIndex < len(m.leftEntries) {
				entry := m.leftEntries[m.leftIndex]
				if entry.IsDir {
					m.leftPath = entry.Path
					m.leftIndex = 0
					m.leftOffset = 0
					return m, m.loadFiles(m.leftPath, true)
				}
			}
		}
	} else {
		// Right panel - enter directory
		if m.rightIndex < len(m.rightEntries) {
			entry := m.rightEntries[m.rightIndex]
			if entry.IsDir {
				m.rightPath = entry.Path
				m.rightIndex = 0
				m.rightOffset = 0
				return m, m.loadFiles(m.rightPath, false)
			}
		}
	}
	return m, nil
}

func (m RestoreModel) handleBack() (RestoreModel, tea.Cmd) {
	if m.focus == focusLeft {
		if m.currentSnapshot != "" {
			// Go up in file tree or back to snapshot list
			parent := filepath.Dir(m.leftPath)
			if parent != m.leftPath && strings.HasPrefix(parent, m.mountPoint) {
				m.leftPath = parent
				m.leftIndex = 0
				m.leftOffset = 0
				return m, m.loadFiles(m.leftPath, true)
			} else {
				// Back to snapshot list
				m.currentSnapshot = ""
				m.snapshotMounted = false
				m.leftEntries = nil
				m.leftPath = "/"
				return m, m.unmountSnapshot()
			}
		}
	} else {
		// Right panel - go up
		parent := filepath.Dir(m.rightPath)
		if parent != m.rightPath {
			m.rightPath = parent
			m.rightIndex = 0
			m.rightOffset = 0
			return m, m.loadFiles(m.rightPath, false)
		}
	}
	return m, nil
}

func (m RestoreModel) startCopy() (RestoreModel, tea.Cmd) {
	if len(m.selectedFiles) == 0 {
		return m, nil
	}

	// Check for existing files at destination
	var conflicts []FileEntry
	for _, f := range m.selectedFiles {
		var destPath string
		if m.restoreToOriginal {
			// Restore to original location - extract path relative to snapshot mount
			destPath = m.getOriginalDestPath(f)
		} else {
			// Restore to current folder
			destPath = filepath.Join(m.rightPath, f.Name)
		}
		if _, err := os.Stat(destPath); err == nil {
			conflicts = append(conflicts, f)
		}
	}

	if len(conflicts) > 0 {
		m.confirmFiles = conflicts
		m.state = restoreConfirmOverwrite
		return m, nil
	}

	m.state = restoreCopying
	return m, m.executeCopy()
}

// getOriginalDestPath computes the original path for a file being restored
func (m RestoreModel) getOriginalDestPath(f FileEntry) string {
	// The file path is something like /tmp/zfs-restore-xxx/subdir/file.txt
	// We need to strip the mount point prefix and use the rest as the destination
	if m.mountPoint != "" && strings.HasPrefix(f.Path, m.mountPoint) {
		relPath := strings.TrimPrefix(f.Path, m.mountPoint)
		// The dataset name determines the base path
		// For now, assume the snapshot was from a dataset mounted at root
		// e.g., NIXBACKUPS/home -> /home
		parts := strings.Split(m.currentSnapshot, "@")
		if len(parts) > 0 {
			datasetPath := getDatasetMountPoint(parts[0])
			if datasetPath != "" {
				return filepath.Join(datasetPath, relPath)
			}
		}
		// Fallback: use the relative path from root
		return relPath
	}
	return f.Path
}

// =============================================================================
// Commands (ZFS and File Operations)
// =============================================================================

type snapshotsLoadedMsg struct {
	snapshots []SnapshotEntry
}

type filesLoadedMsg struct {
	entries []FileEntry
	isLeft  bool
}

type snapshotMountedMsg struct {
	mountPoint string
}

type copyProgressMsg struct {
	file    string
	current int64
	total   int64
	done    bool
}

type poolUnlockedMsg struct {
	pool string
}

type unmountCompleteMsg struct{}

func (m RestoreModel) loadSnapshots() tea.Cmd {
	return func() tea.Msg {
		snapshots, err := getPoolSnapshots(m.sourcePool)
		if err != nil {
			return snapshotsLoadedMsg{snapshots: nil}
		}
		return snapshotsLoadedMsg{snapshots: snapshots}
	}
}

func (m RestoreModel) loadFiles(path string, isLeft bool) tea.Cmd {
	return func() tea.Msg {
		entries, err := readDirectory(path)
		if err != nil {
			return filesLoadedMsg{entries: nil, isLeft: isLeft}
		}
		return filesLoadedMsg{entries: entries, isLeft: isLeft}
	}
}

func (m RestoreModel) mountSnapshot(snapshotName string) tea.Cmd {
	return func() tea.Msg {
		mountPoint, err := mountZFSSnapshot(snapshotName)
		if err != nil {
			return snapshotMountedMsg{mountPoint: ""}
		}
		return snapshotMountedMsg{mountPoint: mountPoint}
	}
}

func (m RestoreModel) unmountSnapshot() tea.Cmd {
	return func() tea.Msg {
		if m.mountPoint != "" {
			unmountZFSSnapshot(m.mountPoint)
		}
		return unmountCompleteMsg{}
	}
}

func (m RestoreModel) unlockPool(pool, password string) tea.Cmd {
	return func() tea.Msg {
		err := loadZFSKey(pool, password)
		if err != nil {
			return poolUnlockedMsg{pool: ""}
		}
		return poolUnlockedMsg{pool: pool}
	}
}

func (m RestoreModel) executeCopy() tea.Cmd {
	restoreToOriginal := m.restoreToOriginal
	rightPath := m.rightPath
	mountPoint := m.mountPoint
	currentSnapshot := m.currentSnapshot
	selectedFiles := m.selectedFiles

	return func() tea.Msg {
		var totalSize int64
		for _, f := range selectedFiles {
			totalSize += f.Size
		}

		var copiedSize int64
		for _, f := range selectedFiles {
			var destPath string
			if restoreToOriginal {
				// Restore to original location
				destPath = getOriginalDestPathStatic(f, mountPoint, currentSnapshot)
			} else {
				// Restore to current folder
				destPath = filepath.Join(rightPath, f.Name)
			}

			// Ensure parent directory exists
			destDir := filepath.Dir(destPath)
			if err := os.MkdirAll(destDir, 0755); err != nil {
				continue
			}

			err := copyFile(f.Path, destPath, func(written int64) {
				// Progress callback - would need channel for real-time updates
			})
			if err != nil {
				// Continue with other files
			}
			copiedSize += f.Size
		}

		return copyProgressMsg{
			file:    "",
			current: totalSize,
			total:   totalSize,
			done:    true,
		}
	}
}

// getOriginalDestPathStatic is a static version for use in goroutines
func getOriginalDestPathStatic(f FileEntry, mountPoint, currentSnapshot string) string {
	if mountPoint != "" && strings.HasPrefix(f.Path, mountPoint) {
		relPath := strings.TrimPrefix(f.Path, mountPoint)
		parts := strings.Split(currentSnapshot, "@")
		if len(parts) > 0 {
			datasetPath := getDatasetMountPoint(parts[0])
			if datasetPath != "" {
				return filepath.Join(datasetPath, relPath)
			}
		}
		return relPath
	}
	return f.Path
}

func (m RestoreModel) unmountAndPowerOff() tea.Cmd {
	return func() tea.Msg {
		// Unmount snapshot if mounted
		if m.mountPoint != "" {
			unmountZFSSnapshot(m.mountPoint)
		}

		// Export and power off source pool
		if m.sourcePool != "" {
			device, err := getBackupDevice(m.sourcePool)
			if err == nil {
				_ = runCommand("zpool", "export", m.sourcePool)
				_ = runCommand("udisksctl", "power-off", "-b", device)
			}
		}

		return unmountCompleteMsg{}
	}
}

// =============================================================================
// ZFS Helper Functions
// =============================================================================

func poolNeedsUnlock(pool string) (bool, error) {
	keyStatus, err := getKeyStatus(pool)
	if err != nil {
		// Pool might not be encrypted
		return false, nil
	}
	return keyStatus != "available", nil
}

func getPoolMountPoint(pool string) string {
	output, err := runCommandOutput("zfs", "get", "-H", "-o", "value", "mountpoint", pool)
	if err != nil {
		return "/"
	}
	return strings.TrimSpace(output)
}

func getPoolSnapshots(pool string) ([]SnapshotEntry, error) {
	output, err := runCommandOutput("zfs", "list", "-H", "-t", "snapshot", "-o", "name,creation,used,refer", "-s", "creation", "-r", pool)
	if err != nil {
		return nil, err
	}

	var snapshots []SnapshotEntry
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			// Parse creation time
			creationStr := fields[1] + " " + fields[2] + " " + fields[3]
			creation, _ := time.Parse("Mon Jan 2 15:04 2006", creationStr)

			snapshots = append(snapshots, SnapshotEntry{
				Name:       fields[0],
				Dataset:    strings.Split(fields[0], "@")[0],
				Creation:   creation,
				Used:       fields[len(fields)-2],
				Referenced: fields[len(fields)-1],
			})
		}
	}

	// Reverse to show newest first
	for i, j := 0, len(snapshots)-1; i < j; i, j = i+1, j-1 {
		snapshots[i], snapshots[j] = snapshots[j], snapshots[i]
	}

	return snapshots, nil
}

func mountZFSSnapshot(snapshotName string) (string, error) {
	// Create temporary mount point
	mountPoint := fmt.Sprintf("/tmp/zfs-restore-%d", time.Now().Unix())
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return "", err
	}

	// Mount the snapshot read-only
	err := runCommand("mount", "-t", "zfs", snapshotName, mountPoint)
	if err != nil {
		os.Remove(mountPoint)
		return "", err
	}

	return mountPoint, nil
}

func unmountZFSSnapshot(mountPoint string) error {
	_ = runCommand("umount", mountPoint)
	os.Remove(mountPoint)
	return nil
}

func readDirectory(path string) ([]FileEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var result []FileEntry

	// Add parent directory entry (..) if not at root
	if path != "/" {
		parentPath := filepath.Dir(path)
		result = append(result, FileEntry{
			Name:  "..",
			Path:  parentPath,
			IsDir: true,
		})
	}

	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		result = append(result, FileEntry{
			Name:    e.Name(),
			Path:    filepath.Join(path, e.Name()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   e.IsDir(),
		})
	}

	return result, nil
}

func copyFile(src, dst string, progressFn func(int64)) error {
	// Get source file info first
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}

	// Handle symlinks
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(src)
		if err != nil {
			return err
		}
		// Remove destination if it exists
		os.Remove(dst)
		if err := os.Symlink(linkTarget, dst); err != nil {
			return err
		}
		// Try to preserve symlink ownership (requires root)
		if stat, ok := srcInfo.Sys().(*syscall.Stat_t); ok {
			_ = os.Lchown(dst, int(stat.Uid), int(stat.Gid))
		}
		return nil
	}

	// Handle directories
	if srcInfo.IsDir() {
		if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
			return err
		}
		// Preserve ownership and timestamps for directory
		if stat, ok := srcInfo.Sys().(*syscall.Stat_t); ok {
			_ = os.Chown(dst, int(stat.Uid), int(stat.Gid))
		}
		_ = os.Chmod(dst, srcInfo.Mode().Perm())
		_ = os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime())
		return nil
	}

	// Regular file copy
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Create destination with same permissions
	destFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode().Perm())
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy with progress
	buf := make([]byte, 1024*1024) // 1MB buffer
	var written int64
	for {
		n, err := sourceFile.Read(buf)
		if n > 0 {
			_, writeErr := destFile.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			written += int64(n)
			if progressFn != nil {
				progressFn(written)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	// Ensure data is written to disk
	destFile.Sync()

	// Preserve ownership (requires root privileges)
	if stat, ok := srcInfo.Sys().(*syscall.Stat_t); ok {
		_ = os.Chown(dst, int(stat.Uid), int(stat.Gid))
	}

	// Preserve permissions
	_ = os.Chmod(dst, srcInfo.Mode().Perm())

	// Preserve modification and access times
	_ = os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime())

	return nil
}

// =============================================================================
// Restore Mode View
// =============================================================================

func (m RestoreModel) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var content string

	switch m.state {
	case restoreSelectSource:
		content = m.renderPoolSelection()
	case restorePasswordSource:
		content = m.renderPasswordEntry()
	case restoreExplorer:
		content = m.renderExplorer()
	case restoreCopying:
		content = m.renderCopying()
	case restoreConfirmOverwrite:
		content = m.renderConfirmOverwrite()
	case restoreComplete:
		content = m.renderComplete()
	}

	// Build full view with header and footer
	header := renderHeader(m.width, m.getStatusText())
	footer := renderFooter(m.width, m.getHotkeys(), 0, 1)

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (m RestoreModel) getStatusText() string {
	switch m.state {
	case restoreSelectSource:
		return "Select Source Pool"
	case restorePasswordSource:
		return "Enter Password"
	case restoreExplorer:
		return fmt.Sprintf("Restore Explorer | %d selected", len(m.selectedFiles))
	case restoreCopying:
		return "Copying Files..."
	case restoreConfirmOverwrite:
		return "Confirm Overwrite"
	case restoreComplete:
		return "Restore Complete"
	}
	return "Restore Mode"
}

func (m RestoreModel) getHotkeys() string {
	switch m.state {
	case restoreSelectSource:
		return "↑/k up • ↓/j down • enter select • esc cancel"
	case restorePasswordSource:
		return "enter submit • esc cancel"
	case restoreExplorer:
		if m.mkdirMode {
			return "enter create • esc cancel"
		}
		if m.restoreModeSelect {
			return "↑/↓ select • enter confirm • o original • c current • esc cancel"
		}
		return "hjkl nav • tab switch • space select • y yank • m mkdir • / search • u unmount • esc menu"
	case restoreConfirmOverwrite:
		return "y confirm • n cancel"
	case restoreComplete:
		return "u unmount drive • enter/q return to menu"
	}
	return "esc back"
}

func (m RestoreModel) renderPoolSelection() string {
	var b strings.Builder
	pools := getAllPools()

	title := "Select Source Pool (containing snapshots to restore from)"
	b.WriteString(selectedItemStyle.Render(title) + "\n\n")

	for i, pool := range pools {
		var line string
		if i == m.snapshotIndex {
			line = selectedItemStyle.Render(fmt.Sprintf("  ▶ %s", pool))
		} else {
			line = fmt.Sprintf("    %s", pool)
		}
		b.WriteString(line + "\n")
	}

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(b.String())
}

func (m RestoreModel) renderPasswordEntry() string {
	var b strings.Builder

	poolName := m.sourcePool

	b.WriteString(selectedItemStyle.Render("Encryption Password") + "\n\n")
	b.WriteString(infoStyle.Render(fmt.Sprintf("Enter password for %s:", poolName)) + "\n\n")
	b.WriteString(m.passwordInput.View() + "\n")

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(b.String())
}

func (m RestoreModel) renderExplorer() string {
	panelWidth := m.getPanelWidth()
	panelHeight := m.getPanelHeight()

	// Left panel
	leftPanel := m.renderLeftPanel(panelWidth, panelHeight)

	// Right panel
	rightPanel := m.renderRightPanel(panelWidth, panelHeight)

	// Join panels horizontally
	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// Status bar
	statusBar := m.renderStatusBar()

	result := lipgloss.JoinVertical(lipgloss.Left, panels, statusBar)

	// Overlay mkdir dialog
	if m.mkdirMode {
		dialog := m.renderMkdirDialog()
		result = m.overlayDialog(result, dialog)
	}

	// Overlay restore mode selection
	if m.restoreModeSelect {
		dialog := m.renderRestoreModeDialog()
		result = m.overlayDialog(result, dialog)
	}

	return result
}

func (m RestoreModel) renderMkdirDialog() string {
	var b strings.Builder

	b.WriteString(selectedItemStyle.Render("Create New Folder") + "\n\n")
	b.WriteString(infoStyle.Render(fmt.Sprintf("In: %s", m.rightPath)) + "\n\n")
	b.WriteString(m.mkdirInput.View() + "\n\n")
	b.WriteString(subtleStyle.Render("Enter to create • Esc to cancel"))

	return dialogBoxStyle.Render(b.String())
}

func (m RestoreModel) renderRestoreModeDialog() string {
	var b strings.Builder

	b.WriteString(selectedItemStyle.Render("Restore Mode") + "\n\n")
	b.WriteString(infoStyle.Render(fmt.Sprintf("Restoring %d file(s)", len(m.selectedFiles))) + "\n\n")

	// Option 1: Original location
	opt1 := "  Restore to original location"
	if m.restoreToOriginal {
		opt1 = "▶ Restore to original location"
		b.WriteString(selectedItemStyle.Render(opt1) + "\n")
	} else {
		b.WriteString(opt1 + "\n")
	}

	// Option 2: Current folder
	opt2 := fmt.Sprintf("  Restore to current folder (%s)", m.rightPath)
	if !m.restoreToOriginal {
		opt2 = fmt.Sprintf("▶ Restore to current folder (%s)", m.rightPath)
		b.WriteString(selectedItemStyle.Render(opt2) + "\n")
	} else {
		b.WriteString(opt2 + "\n")
	}

	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("↑/↓ select • Enter confirm • o original • c current • Esc cancel"))

	return dialogBoxStyle.Render(b.String())
}

func (m RestoreModel) overlayDialog(background, dialog string) string {
	// Simple overlay - just center the dialog
	return lipgloss.Place(
		m.width,
		m.height-8, // Account for header/footer
		lipgloss.Center,
		lipgloss.Center,
		dialog,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("0")),
	)
}

func (m RestoreModel) renderLeftPanel(width, height int) string {
	var style lipgloss.Style
	if m.focus == focusLeft {
		style = panelActiveStyle.Width(width).Height(height)
	} else {
		style = panelStyle.Width(width).Height(height)
	}

	var content strings.Builder

	// Header
	var headerText string
	if m.currentSnapshot == "" {
		headerText = fmt.Sprintf(" Snapshots (%s) ", m.sourcePool)
	} else {
		headerText = fmt.Sprintf(" %s ", m.leftPath)
	}
	header := panelHeaderStyle.Width(width - 2).Render(headerText)
	content.WriteString(header + "\n")

	visibleRows := height - 3

	if m.currentSnapshot == "" {
		// Show snapshot list with scrolling
		for i := 0; i < visibleRows; i++ {
			idx := i + m.snapshotOffset
			if idx >= len(m.snapshots) {
				break
			}
			snap := m.snapshots[idx]

			// Format: name | date | size
			name := snap.Name
			if len(name) > width-30 {
				name = "..." + name[len(name)-width+33:]
			}

			var line string
			if idx == m.snapshotIndex && m.focus == focusLeft {
				line = snapshotSelectedStyle.Render(fmt.Sprintf("▶ %s", name))
			} else {
				line = snapshotStyle.Render(fmt.Sprintf("  %s", name))
			}
			content.WriteString(line + "\n")
		}
	} else {
		// Show file list
		for i := 0; i < visibleRows && i+m.leftOffset < len(m.leftEntries); i++ {
			idx := i + m.leftOffset
			entry := m.leftEntries[idx]

			line := m.renderFileEntry(entry, idx == m.leftIndex && m.focus == focusLeft, width-4)
			content.WriteString(line + "\n")
		}
	}

	return style.Render(content.String())
}

func (m RestoreModel) renderRightPanel(width, height int) string {
	var style lipgloss.Style
	if m.focus == focusRight {
		style = panelActiveStyle.Width(width).Height(height)
	} else {
		style = panelStyle.Width(width).Height(height)
	}

	var content strings.Builder

	// Header
	headerText := fmt.Sprintf(" %s ", m.rightPath)
	header := panelHeaderStyle.Width(width - 2).Render(headerText)
	content.WriteString(header + "\n")

	visibleRows := height - 3

	for i := 0; i < visibleRows && i+m.rightOffset < len(m.rightEntries); i++ {
		idx := i + m.rightOffset
		entry := m.rightEntries[idx]

		line := m.renderFileEntry(entry, idx == m.rightIndex && m.focus == focusRight, width-4)
		content.WriteString(line + "\n")
	}

	return style.Render(content.String())
}

func (m RestoreModel) renderFileEntry(entry FileEntry, isCursor bool, width int) string {
	// Icon - using simple ASCII characters
	var icon string
	if entry.Name == ".." {
		icon = "^"
	} else if entry.IsDir {
		icon = "/"
	} else {
		icon = " "
	}

	// Selection marker (don't allow selecting ..)
	var marker string
	if entry.Name == ".." {
		marker = "  "
	} else if entry.Selected {
		marker = "✓ "
	} else {
		marker = "  "
	}

	// Size and date (skip for ..)
	var sizeStr, dateStr string
	if entry.Name == ".." {
		sizeStr = ""
		dateStr = "(parent)"
	} else {
		sizeStr = formatSize(entry.Size)
		dateStr = entry.ModTime.Format("Jan 02 15:04")
	}

	// Name (truncate if needed)
	nameWidth := width - 30
	name := entry.Name
	if len(name) > nameWidth {
		name = name[:nameWidth-3] + "..."
	}

	// Build line
	line := fmt.Sprintf("%s%s %s  %s  %s",
		marker,
		icon,
		lipgloss.NewStyle().Width(nameWidth).Render(name),
		sizeStyle.Render(sizeStr),
		dateStyle.Render(dateStr),
	)

	// Apply style
	var style lipgloss.Style
	if isCursor {
		style = fileEntryCursorStyle
	} else if entry.Selected {
		style = fileEntryMarkedStyle
	} else if entry.IsDir {
		style = fileEntryDirStyle
	} else {
		style = fileEntryStyle
	}

	return style.Render(line)
}

func (m RestoreModel) renderStatusBar() string {
	var parts []string

	// Selection info
	if len(m.selectedFiles) > 0 {
		var totalSize int64
		for _, f := range m.selectedFiles {
			totalSize += f.Size
		}
		parts = append(parts, fmt.Sprintf("%d files selected (%s)", len(m.selectedFiles), formatSize(totalSize)))
	}

	// Sort info
	var sortStr string
	switch m.sortMode {
	case sortByName:
		sortStr = "Name"
	case sortByDate:
		sortStr = "Date"
	case sortBySize:
		sortStr = "Size"
	}
	if m.sortAscending {
		sortStr += " ↑"
	} else {
		sortStr += " ↓"
	}
	parts = append(parts, fmt.Sprintf("Sort: %s", sortStr))

	// Search
	if m.searchMode {
		parts = append(parts, fmt.Sprintf("Search: %s", m.searchInput.View()))
	}

	status := strings.Join(parts, " │ ")
	return statusBarStyle.Width(m.width).Render(status)
}

func (m RestoreModel) renderCopying() string {
	var b strings.Builder

	b.WriteString(selectedItemStyle.Render("Copying Files...") + "\n\n")

	if m.copyingFile != "" {
		b.WriteString(infoStyle.Render(m.copyingFile) + "\n\n")
	}

	b.WriteString(m.copyProgress.View() + "\n\n")

	if m.copyTotal > 0 {
		percent := float64(m.copyCurrent) / float64(m.copyTotal) * 100
		b.WriteString(infoStyle.Render(fmt.Sprintf("%s / %s (%.1f%%)",
			formatSize(m.copyCurrent),
			formatSize(m.copyTotal),
			percent)) + "\n")
	}

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(b.String())
}

func (m RestoreModel) renderConfirmOverwrite() string {
	var b strings.Builder

	b.WriteString(warningStyle.Render("Files Already Exist") + "\n\n")
	b.WriteString(infoStyle.Render("The following files will be overwritten:") + "\n\n")

	for i, f := range m.confirmFiles {
		if i >= 10 {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(m.confirmFiles)-10))
			break
		}
		b.WriteString(fmt.Sprintf("  • %s\n", f.Name))
	}

	b.WriteString("\n")
	b.WriteString(warningStyle.Render("This cannot be undone!") + "\n\n")
	b.WriteString(infoStyle.Render("Press 'y' to confirm or 'n' to cancel") + "\n")

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(b.String())
}

func (m RestoreModel) renderComplete() string {
	var b strings.Builder

	b.WriteString(statusStyle.Render("Restore Complete") + "\n\n")
	b.WriteString(infoStyle.Render(fmt.Sprintf("Restored %d files to %s", len(m.selectedFiles), m.rightPath)) + "\n\n")

	b.WriteString(infoStyle.Render("Press 'u' to safely unmount the source drive") + "\n")
	b.WriteString(infoStyle.Render("Press 'enter' or 'q' to return to menu") + "\n")

	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(b.String())
}

// =============================================================================
// Utility Functions
// =============================================================================

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// getDatasetMountPoint returns the mount point for a ZFS dataset
func getDatasetMountPoint(dataset string) string {
	output, err := exec.Command("zfs", "get", "-H", "-o", "value", "mountpoint", dataset).Output()
	if err != nil {
		// Try to infer from dataset name (e.g., NIXBACKUPS/home -> /home)
		parts := strings.SplitN(dataset, "/", 2)
		if len(parts) > 1 {
			return "/" + parts[1]
		}
		return ""
	}
	return strings.TrimSpace(string(output))
}
