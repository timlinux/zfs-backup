package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Application constants
const (
	appName    = "Kartoza ZFS Backup"
	appTagline = "Keep your ZFS Backed Up!"
	appVersion = "1.0.0"

	// URLs for footer links
	kartozaURL  = "https://kartoza.com"
	donateURL   = "https://github.com/sponsors/kartoza"
	githubURL   = "https://github.com/kartoza/zfs-backup"
	docsURL     = "https://kartoza.github.io/zfs-backup"
)

// Kartoza brand colors
var (
	colorHighlight1 = lipgloss.Color("#DF9E2F") // Primary gold/orange
	colorHighlight2 = lipgloss.Color("#569FC6") // Blue
	colorHighlight3 = lipgloss.Color("#8A8B8B") // Gray
	colorHighlight4 = lipgloss.Color("#06969A") // Teal
	colorAlert      = lipgloss.Color("#CC0403") // Red alert
	colorBackground = lipgloss.Color("#1E1E1E") // Dark background
	colorForeground = lipgloss.Color("#FFFFFF") // White text
)

// Styles using Kartoza brand colors
var (
	// Title style - primary brand color
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHighlight1).
			Align(lipgloss.Center)

	// Tagline style
	taglineStyle = lipgloss.NewStyle().
			Foreground(colorHighlight2).
			Italic(true).
			Align(lipgloss.Center)

	// Interstitial line style
	interstitialStyle = lipgloss.NewStyle().
				Foreground(colorHighlight3).
				Align(lipgloss.Center)

	// Status line style
	statusLineStyle = lipgloss.NewStyle().
			Foreground(colorHighlight4).
			Align(lipgloss.Center)

	// Content frame with rounded corners
	contentFrameStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorHighlight2).
				Padding(1, 2)

	// Header section with pool selection
	headerBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorHighlight3).
			Padding(1, 2).
			MarginBottom(1)

	// Central content frame
	centralFrameStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorHighlight2).
				Padding(1, 3).
				MarginTop(1).
				MarginBottom(1)

	// Menu container
	menuStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorHighlight4).
			Padding(1, 2)

	// Individual menu option
	menuItemStyle = lipgloss.NewStyle().
			Padding(0, 1).
			MarginBottom(1)

	// Destructive operation warning
	destructiveWarningStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAlert).
				Foreground(colorAlert).
				Padding(1, 2).
				Bold(true).
				MarginTop(1).
				MarginBottom(1)

	// Safe operation indicator
	safeOperationStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorHighlight4).
				Padding(1, 2).
				MarginTop(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorHighlight3).
			Italic(true).
			MarginBottom(1)

	statusStyle = lipgloss.NewStyle().
			Foreground(colorHighlight4).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorAlert).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(colorHighlight1).
			Bold(true)

	infoStyle = lipgloss.NewStyle().
			Foreground(colorHighlight2)

	reportBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorHighlight4).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(colorHighlight1).
				Bold(true)

	// Pool selector styles
	poolSelectorStyle = lipgloss.NewStyle().
				Foreground(colorHighlight2).
				Bold(true)

	poolSelectorActiveStyle = lipgloss.NewStyle().
					Foreground(colorHighlight1).
					Bold(true).
					Padding(0, 1)

	labelStyle = lipgloss.NewStyle().
			Foreground(colorHighlight3).
			Bold(true)

	// Footer credit style
	footerCreditStyle = lipgloss.NewStyle().
				Foreground(colorHighlight3).
				Align(lipgloss.Center)

	// Hotkey style
	hotkeyStyle = lipgloss.NewStyle().
			Foreground(colorHighlight2)

	// Pagination dots style
	paginationActiveStyle = lipgloss.NewStyle().
				Foreground(colorHighlight1)

	paginationInactiveStyle = lipgloss.NewStyle().
				Foreground(colorHighlight3)
)

// =============================================================================
// DRY Header and Footer Components
// =============================================================================

// renderHeader renders the standard Kartoza header with title, tagline, and status
func renderHeader(width int, status string) string {
	var b strings.Builder

	// Create interstitial line
	lineWidth := width - 4
	if lineWidth < 20 {
		lineWidth = 20
	}
	interstitial := strings.Repeat("─", lineWidth)

	// Top interstitial line
	line0 := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(interstitialStyle.Render(interstitial))
	b.WriteString(line0 + "\n")

	// Kartoza brand line - "Kartoza - ZFS Backup Tool"
	brandLine := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(titleStyle.Render("Kartoza - ZFS Backup Tool"))
	b.WriteString(brandLine + "\n")

	// Tagline - centered
	tagline := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(taglineStyle.Render(appTagline))
	b.WriteString(tagline + "\n")

	// Interstitial line
	line1 := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(interstitialStyle.Render(interstitial))
	b.WriteString(line1 + "\n")

	// Status line - centered
	statusText := fmt.Sprintf("Version %s │ Status: %s", appVersion, status)
	statusLine := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(statusLineStyle.Render(statusText))
	b.WriteString(statusLine + "\n")

	// Interstitial line
	line2 := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(interstitialStyle.Render(interstitial))
	b.WriteString(line2 + "\n")

	return b.String()
}

// renderFooter renders the standard Kartoza footer with pagination, hotkeys, and credits
func renderFooter(width int, hotkeys string, currentPage, totalPages int) string {
	var b strings.Builder

	// Create interstitial line
	lineWidth := width - 4
	if lineWidth < 20 {
		lineWidth = 20
	}
	interstitial := strings.Repeat("─", lineWidth)

	// Interstitial line
	line1 := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(interstitialStyle.Render(interstitial))
	b.WriteString(line1 + "\n")

	// Pagination dots - centered
	var paginationDots string
	if totalPages > 1 {
		for i := 0; i < totalPages; i++ {
			if i == currentPage {
				paginationDots += paginationActiveStyle.Render("●")
			} else {
				paginationDots += paginationInactiveStyle.Render("○")
			}
			if i < totalPages-1 {
				paginationDots += " "
			}
		}
	} else {
		paginationDots = paginationActiveStyle.Render("●")
	}
	pagination := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(paginationDots)
	b.WriteString(pagination + "\n")

	// Hotkeys bar - centered
	hotkeysLine := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(hotkeyStyle.Render(hotkeys))
	b.WriteString(hotkeysLine + "\n")

	// Interstitial line
	line2 := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(interstitialStyle.Render(interstitial))
	b.WriteString(line2 + "\n")

	// Kartoza credit line with hotkeys - centered
	// Format: Made with 💗 by [K]artoza │ D[o]nate! │ [G]itHub
	kartoza := selectedItemStyle.Render("K") + footerCreditStyle.Render("artoza")
	donate := footerCreditStyle.Render("D") + selectedItemStyle.Render("o") + footerCreditStyle.Render("nate!")
	github := selectedItemStyle.Render("G") + footerCreditStyle.Render("itHub")
	credit := footerCreditStyle.Render("Made with 💗 by ") + kartoza +
		footerCreditStyle.Render(" │ ") + donate +
		footerCreditStyle.Render(" │ ") + github

	creditLine := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(credit)
	b.WriteString(creditLine + "\n")

	return b.String()
}

// getStatusText returns the current status text based on application state
func (m model) getStatusText() string {
	if m.selectingPool {
		if m.selectingSource {
			return "Select Source Pool"
		}
		return "Select Destination Pool"
	}

	switch m.state {
	case stateMenu:
		return "Ready"
	case stateConfirm:
		return "Awaiting Confirmation"
	case stateInput:
		return "Input Required"
	case statePassword:
		return "Enter Password"
	case stateRunning:
		return "Running: " + m.operation
	case stateResult:
		return "Complete"
	case stateHelp:
		return "Help"
	case stateResume:
		return "Resume Available"
	default:
		return "Idle"
	}
}

// getHotkeys returns the appropriate hotkeys for the current state
func (m model) getHotkeys() string {
	if m.selectingPool {
		return "↑/k up • ↓/j down • enter select • esc cancel"
	}

	switch m.state {
	case stateMenu:
		return "↑/k up • ↓/j down • enter select • ? help • q quit"
	case stateConfirm:
		return "y confirm • n cancel • esc back"
	case stateInput, statePassword:
		return "enter submit • esc cancel"
	case stateRunning:
		return "ctrl+c cancel (resumable)"
	case stateResult, stateHelp:
		return "enter/esc return to menu"
	case stateResume:
		return "y resume • n start fresh • esc back"
	default:
		return "q quit"
	}
}

// =============================================================================
// Application State Types
// =============================================================================

type sessionState int

const (
	stateMenu sessionState = iota
	stateConfirm
	stateInput
	statePassword
	stateRunning
	stateResult
	stateHelp
	stateResume
	stateRestore
)

type menuItem struct {
	title       string
	description string
	icon        string
}

// Simple menu items for main menu
var mainMenuItems = []menuItem{
	{title: "Backup ZFS (incremental)", description: "Run incremental backup from source to destination pool using syncoid", icon: "📦"},
	{title: "Restore Files", description: "Browse snapshots and restore files to any location", icon: "📥"},
	{title: "Unmount Backup Disk", description: "Safely export the backup pool and power off the USB drive", icon: "🔌"},
	{title: "Help", description: "Show detailed help information about all operations", icon: "❓"},
	{title: "Exit", description: "Exit the application", icon: "❌"},
	{title: "---", description: "Danger Zone", icon: ""},  // Separator
	{title: "Prepare Backup Device", description: "⚠️ DESTRUCTIVE - Create an encrypted ZFS pool on a new external backup device", icon: "🔧"},
	{title: "Force Backup ZFS (destructive)", description: "⚠️ DESTRUCTIVE - Deletes old snapshots on backup disk and forces full sync", icon: "🔥"},
}

type model struct {
	state            sessionState
	menuIndex        int // Current menu selection index
	spinner          spinner.Model
	input            textinput.Model
	passwordInput    textinput.Model
	progress         progress.Model
	operation        string
	message          string
	err              error
	width            int
	height           int
	confirmMsg       string
	confirmYes       bool
	quitting         bool
	showingHelp      bool
	password         string
	devicePath       string
	backupState      *BackupState
	currentStage     string
	cancelFunc       context.CancelFunc
	resumeState      *BackupState
	totalStages      int
	eta              time.Duration
	// Pool selection (shown after backup option selected)
	availablePools   []string
	sourcePool       string
	destPool         string
	selectingPool    bool      // Are we in pool selection mode?
	poolSelectIndex  int       // Current pool selection index
	selectingSource  bool      // true = selecting source, false = selecting dest
	// Progress channel for real-time updates
	progressChan     chan progressUpdate
	// Restore mode
	restoreModel     RestoreModel
}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorHighlight1)

	ti := textinput.New()
	ti.Placeholder = "/dev/sda"
	ti.Focus()
	ti.CharLimit = 50
	ti.Width = 40
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorHighlight2)
	ti.TextStyle = lipgloss.NewStyle().Foreground(colorHighlight1)

	pi := textinput.New()
	pi.Placeholder = "Enter encryption password"
	pi.EchoMode = textinput.EchoPassword
	pi.EchoCharacter = '•'
	pi.CharLimit = 256
	pi.Width = 40
	pi.PromptStyle = lipgloss.NewStyle().Foreground(colorHighlight2)
	pi.TextStyle = lipgloss.NewStyle().Foreground(colorHighlight1)

	// Progress bar with Kartoza brand colors
	prog := progress.New(
		progress.WithGradient(string(colorHighlight2), string(colorHighlight1)),
	)
	prog.Width = 60

	// Get available pools (including locked/not-imported ones)
	pools := getAllPools()

	// Try to detect defaults (NIXROOT and NIXBACKUPS)
	sourcePool := "NIXROOT"
	destPool := "NIXBACKUPS"

	// Validate defaults exist in available pools
	sourceFound := false
	destFound := false
	for _, p := range pools {
		if p == sourcePool {
			sourceFound = true
		}
		if p == destPool {
			destFound = true
		}
	}

	// If defaults not found, use first available or empty
	if !sourceFound && len(pools) > 0 {
		sourcePool = pools[0]
	} else if !sourceFound {
		sourcePool = ""
	}

	if !destFound {
		destPool = ""
	}

	return model{
		state:          stateMenu,
		menuIndex:      0,
		spinner:        s,
		input:          ti,
		passwordInput:  pi,
		progress:       prog,
		availablePools: pools,
		sourcePool:     sourcePool,
		destPool:       destPool,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

// getAvailablePools returns a list of currently imported ZFS pools
func getAvailablePools() []string {
	output, err := runCommandOutput("zpool", "list", "-H", "-o", "name")
	if err != nil {
		return []string{}
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var pools []string
	for _, line := range lines {
		if line != "" {
			pools = append(pools, line)
		}
	}
	return pools
}

// getAllPools returns all ZFS pools including those that can be imported
func getAllPools() []string {
	// First get imported pools
	pools := getAvailablePools()
	poolSet := make(map[string]bool)
	for _, p := range pools {
		poolSet[p] = true
	}

	// Then get importable pools (try with sudo first, then without)
	output, err := runCommandOutput("sudo", "zpool", "import")
	if err != nil {
		// Try without sudo as fallback
		output, err = runCommandOutput("zpool", "import")
	}
	if err == nil {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "pool:") {
				poolName := strings.TrimSpace(strings.TrimPrefix(line, "pool:"))
				if poolName != "" && !poolSet[poolName] {
					pools = append(pools, poolName)
					poolSet[poolName] = true
				}
			}
		}
	}

	return pools
}

// statePoolSelect is a new state for pool selection
const statePoolSelect sessionState = 100

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Pass to restore model if in restore state
		if m.state == stateRestore {
			m.restoreModel.width = msg.Width
			m.restoreModel.height = msg.Height
		}
		return m, nil

	case tea.KeyMsg:
		// Handle restore mode - delegate to restore model
		if m.state == stateRestore {
			var cmd tea.Cmd
			m.restoreModel, cmd = m.restoreModel.Update(msg)
			// Check if restore mode wants to exit
			if m.restoreModel.done {
				m.state = stateMenu
				m.restoreModel = RestoreModel{}
				if !m.restoreModel.returnToMenu {
					return m, tea.Quit
				}
				return m, nil
			}
			return m, cmd
		}

		// Handle pool selection state
		if m.selectingPool {
			switch msg.String() {
			case "up", "k":
				if m.poolSelectIndex > 0 {
					m.poolSelectIndex--
				}
				return m, nil
			case "down", "j":
				if m.poolSelectIndex < len(m.availablePools)-1 {
					m.poolSelectIndex++
				}
				return m, nil
			case "enter":
				if len(m.availablePools) > 0 {
					selectedPool := m.availablePools[m.poolSelectIndex]
					if m.selectingSource {
						m.sourcePool = selectedPool
						// Now select destination
						m.selectingSource = false
						m.poolSelectIndex = 0
						// Try to default to a different pool
						for i, p := range m.availablePools {
							if p != m.sourcePool {
								m.poolSelectIndex = i
								break
							}
						}
					} else {
						m.destPool = selectedPool
						m.selectingPool = false
						// Proceed to password
						m.state = statePassword
						m.passwordInput.SetValue("")
						m.passwordInput.Focus()
						return m, textinput.Blink
					}
				}
				return m, nil
			case "esc":
				m.selectingPool = false
				m.state = stateMenu
				return m, nil
			}
			return m, nil
		}

		// Handle help screen first (before menu)
		if m.showingHelp {
			switch msg.String() {
			case "enter", "esc", "q":
				m.showingHelp = false
				return m, nil
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "D":
				// Open online documentation
				return m, openURL(docsURL)
			}
			return m, nil
		}

		if m.state == stateMenu {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit

			case "up", "k":
				if m.menuIndex > 0 {
					m.menuIndex--
					// Skip separator
					if mainMenuItems[m.menuIndex].title == "---" {
						if m.menuIndex > 0 {
							m.menuIndex--
						} else {
							m.menuIndex++
						}
					}
				}
				return m, nil

			case "down", "j":
				if m.menuIndex < len(mainMenuItems)-1 {
					m.menuIndex++
					// Skip separator
					if mainMenuItems[m.menuIndex].title == "---" {
						if m.menuIndex < len(mainMenuItems)-1 {
							m.menuIndex++
						} else {
							m.menuIndex--
						}
					}
				}
				return m, nil

			case "?":
				// Show help
				m.showingHelp = true
				return m, nil

			case "K":
				// Open Kartoza website
				return m, openURL(kartozaURL)

			case "O":
				// Open Donate page
				return m, openURL(donateURL)

			case "G":
				// Open GitHub page
				return m, openURL(githubURL)

			case "enter":
				selected := mainMenuItems[m.menuIndex]
				switch selected.title {
				case "Backup ZFS (incremental)":
					m.operation = "backup"
					// Refresh pool list and start pool selection
					m.availablePools = getAllPools()
					m.selectingPool = true
					m.selectingSource = true
					m.poolSelectIndex = 0
					return m, nil
				case "Restore Files":
					// Enter restore mode
					m.state = stateRestore
					m.restoreModel = NewRestoreModel()
					m.restoreModel.width = m.width
					m.restoreModel.height = m.height
					return m, nil
				case "Force Backup ZFS (destructive)":
					m.state = stateConfirm
					m.confirmMsg = "⚠️  This will delete all previous snapshots on the backup disk.\nAre you sure you want to continue?"
					m.operation = "force-backup"
					m.confirmYes = false
					return m, nil
				case "Prepare Backup Device":
					m.state = stateInput
					m.operation = "prepare"
					m.input.SetValue("")
					return m, textinput.Blink
				case "Unmount Backup Disk":
					// For unmount, go to pool selection for destination only
					m.operation = "unmount"
					m.availablePools = getAllPools()
					m.selectingPool = true
					m.selectingSource = false // Only select dest pool
					m.poolSelectIndex = 0
					return m, nil
				case "Help":
					m.showingHelp = true
					return m, nil
				case "Exit":
					m.quitting = true
					return m, tea.Quit
				}
			}
		} else if m.state == stateConfirm {
			switch msg.String() {
			case "y", "Y":
				m.confirmYes = true
				// Check if this operation needs pool selection then password
				if m.operation == "force-backup" {
					// Go to pool selection first
					m.availablePools = getAllPools()
					m.selectingPool = true
					m.selectingSource = true
					m.poolSelectIndex = 0
					return m, nil
				} else if m.operation == "prepare" {
					// For prepare, use the stored device path
					m.devicePath = m.input.Value()
				}
				return m.startOperation()
			case "n", "N", "esc":
				m.state = stateMenu
				return m, nil
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			}
		} else if m.state == stateInput {
			switch msg.String() {
			case "enter":
				if m.input.Value() != "" {
					m.devicePath = m.input.Value()
					m.state = stateConfirm
					m.confirmMsg = fmt.Sprintf("⚠️  WARNING: You are about to erase all data on %s.\nThis action is irreversible!\nAre you absolutely sure?", m.input.Value())
					m.confirmYes = false
					return m, nil
				}
			case "esc":
				m.state = stateMenu
				return m, nil
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			}
		} else if m.state == statePassword {
			switch msg.String() {
			case "enter":
				if m.passwordInput.Value() != "" {
					m.password = m.passwordInput.Value()
					return m.startOperation()
				}
			case "esc":
				m.state = stateMenu
				m.password = ""
				m.passwordInput.SetValue("")
				return m, nil
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			}
		} else if m.state == stateRunning {
			switch msg.String() {
			case "ctrl+c":
				// Graceful cancellation
				if m.cancelFunc != nil {
					m.cancelFunc()
				}
				if m.backupState != nil {
					m.backupState.Cancelled = true
					_ = SaveBackupState(m.backupState)
				}
				m.state = stateResult
				m.message = "⚠️  Backup cancelled. You can resume later by starting a new backup."
				m.err = nil
				m.cancelFunc = nil
				return m, nil
			}
		} else if m.state == stateResume {
			switch msg.String() {
			case "y", "Y":
				// Resume the backup
				m.state = statePassword
				m.operation = m.resumeState.Operation
				m.passwordInput.SetValue("")
				m.passwordInput.Focus()
				return m, textinput.Blink
			case "n", "N", "esc":
				// Start fresh, clear state
				_ = ClearBackupState()
				m.resumeState = nil
				m.state = stateMenu
				return m, nil
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			}
		} else if m.state == stateResult {
			switch msg.String() {
			case "enter", "esc", "q":
				m.state = stateMenu
				return m, nil
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			}
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	// Handle restore mode messages
	case snapshotsLoadedMsg, filesLoadedMsg, snapshotMountedMsg, copyProgressMsg, poolUnlockedMsg, unmountCompleteMsg:
		if m.state == stateRestore {
			var cmd tea.Cmd
			m.restoreModel, cmd = m.restoreModel.Update(msg)
			if m.restoreModel.done {
				m.state = stateMenu
				m.restoreModel = RestoreModel{}
			}
			return m, cmd
		}

	case progressMsg:
		m.currentStage = msg.stage
		m.totalStages = msg.totalStages
		m.eta = msg.eta

		var cmd tea.Cmd
		if msg.progress >= 1.0 {
			cmd = m.progress.SetPercent(1.0)
		} else {
			cmd = m.progress.SetPercent(msg.progress)
		}
		return m, cmd

	case progressUpdateMsg:
		// Handle real-time progress updates from backup operations
		m.currentStage = msg.stage
		m.totalStages = msg.totalStages
		m.backupState = msg.state

		// Calculate progress percentage
		progress := float64(msg.stageNum) / float64(msg.totalStages)

		// Continue listening for more progress updates
		var cmds []tea.Cmd
		cmds = append(cmds, m.progress.SetPercent(progress))
		if m.progressChan != nil {
			cmds = append(cmds, listenForProgress(m.progressChan))
		}
		return m, tea.Batch(cmds...)

	case tickMsg:
		// Update ETA periodically
		if m.state == stateRunning && m.backupState != nil {
			m.eta = m.backupState.EstimateTimeRemaining(m.totalStages)
		}
		return m, tickEvery()

	case progress.FrameMsg:
		// Handle progress for restore mode too
		if m.state == stateRestore {
			var cmd tea.Cmd
			m.restoreModel, cmd = m.restoreModel.Update(msg)
			return m, cmd
		}
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case operationResultMsg:
		// Cancel any ongoing operations
		if m.cancelFunc != nil {
			m.cancelFunc()
			m.cancelFunc = nil
		}

		// Close progress channel
		if m.progressChan != nil {
			close(m.progressChan)
			m.progressChan = nil
		}

		m.state = stateResult
		m.message = msg.message
		m.err = msg.err
		m.backupState = nil

		// Clear state file if successful
		if msg.err == nil {
			_ = ClearBackupState()
		}

		return m, nil
	}

	var cmd tea.Cmd
	switch m.state {
	case stateInput:
		m.input, cmd = m.input.Update(msg)
	case statePassword:
		m.passwordInput, cmd = m.passwordInput.Update(msg)
	}

	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return statusStyle.Render("👋 Goodbye!\n")
	}

	// Restore mode has its own full view
	if m.state == stateRestore {
		return m.restoreModel.View()
	}

	// Use a reasonable default if width/height is not set
	width := m.width
	if width == 0 {
		width = 80
	}
	height := m.height
	if height == 0 {
		height = 24
	}

	// Get status and hotkeys based on current state
	status := m.getStatusText()
	hotkeys := m.getHotkeys()

	// Override for help state
	if m.showingHelp {
		status = "Help"
		hotkeys = "enter/esc return to menu"
	}

	// Render components
	header := renderHeader(width, status)
	footer := renderFooter(width, hotkeys, 0, 1)
	var content string
	if m.showingHelp {
		content = m.renderHelpContent(width)
	} else {
		content = m.renderContentNopad(width)
	}

	// Count lines used
	headerLines := strings.Count(header, "\n")
	contentLines := strings.Count(content, "\n")
	footerLines := strings.Count(footer, "\n")

	// Calculate padding needed to push footer to bottom
	usedLines := headerLines + contentLines + footerLines
	paddingLines := height - usedLines
	if paddingLines < 0 {
		paddingLines = 0
	}

	// Build the view
	var view strings.Builder
	view.WriteString(header)
	view.WriteString(content)

	// Add padding to push footer to bottom
	for i := 0; i < paddingLines; i++ {
		view.WriteString("\n")
	}

	view.WriteString(footer)

	return view.String()
}

// renderContentNopad renders the main content area without padding
func (m model) renderContentNopad(width int) string {
	var content strings.Builder

	switch m.state {
	case stateMenu:
		content.WriteString(m.renderMenuContent(width, 0))
	case stateConfirm:
		content.WriteString(m.renderConfirmContent(width))
	case stateInput:
		content.WriteString(m.renderInputContent(width))
	case statePassword:
		content.WriteString(m.renderPasswordContent(width))
	case stateRunning:
		content.WriteString(m.renderRunningContent(width))
	case stateResume:
		content.WriteString(m.renderResumeContent(width))
	case stateResult:
		content.WriteString(m.renderResultContent(width))
	case stateHelp:
		content.WriteString(m.renderHelpContent(width))
	case stateRestore:
		// Restore mode has its own full view, handled in View()
	}

	return content.String()
}

// renderMenuContent renders the main menu view
func (m model) renderMenuContent(width, height int) string {
	var b strings.Builder

	// If we're selecting a pool, show pool selection UI
	if m.selectingPool {
		return m.renderPoolSelectionContent(width)
	}

	// Render simple menu items - one line each
	for i, item := range mainMenuItems {
		var line string
		if item.title == "---" {
			// Separator - render danger zone header
			separator := warningStyle.Render("──── ⚠️  Danger Zone ⚠️  ────")
			centered := lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Render(separator)
			b.WriteString("\n" + centered + "\n")
			continue
		}
		if i == m.menuIndex {
			// Selected item - highlighted
			line = selectedItemStyle.Render(fmt.Sprintf("  ▶ %s %s", item.icon, item.title))
		} else {
			// Normal item
			line = fmt.Sprintf("    %s %s", item.icon, item.title)
		}
		centered := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(line)
		b.WriteString(centered + "\n")
	}

	// Add spacing before description
	b.WriteString("\n")

	// Show description of selected item at the bottom
	selectedItem := mainMenuItems[m.menuIndex]
	descBox := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(subtitleStyle.Render(selectedItem.description))
	b.WriteString(descBox + "\n")

	return b.String()
}

// renderPoolSelectionContent renders the pool selection UI
func (m model) renderPoolSelectionContent(width int) string {
	var b strings.Builder

	// Title based on what we're selecting
	var title string
	if m.selectingSource {
		title = "Select Source Pool"
	} else {
		title = "Select Destination Pool"
	}

	titleLine := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(selectedItemStyle.Render(title))
	b.WriteString(titleLine + "\n\n")

	// Show already selected source if selecting dest
	if !m.selectingSource && m.sourcePool != "" {
		sourceLine := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(infoStyle.Render(fmt.Sprintf("Source: %s", m.sourcePool)))
		b.WriteString(sourceLine + "\n\n")
	}

	// List pools
	if len(m.availablePools) == 0 {
		noPoolsLine := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(warningStyle.Render("No pools found!"))
		b.WriteString(noPoolsLine + "\n\n")

		hintLine := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(subtitleStyle.Render("Try running with: sudo zfs-backup"))
		b.WriteString(hintLine + "\n")
	} else if len(m.availablePools) == 1 && !m.selectingSource {
		// Only one pool and selecting destination - warn user
		warnLine := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(warningStyle.Render("Only imported pools shown. External drives may need sudo."))
		b.WriteString(warnLine + "\n\n")

		for i, pool := range m.availablePools {
			var line string
			if i == m.poolSelectIndex {
				line = selectedItemStyle.Render(fmt.Sprintf("  ▶ %s", pool))
			} else {
				line = fmt.Sprintf("    %s", pool)
			}
			centered := lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Render(line)
			b.WriteString(centered + "\n")
		}
	} else {
		for i, pool := range m.availablePools {
			var line string
			if i == m.poolSelectIndex {
				line = selectedItemStyle.Render(fmt.Sprintf("  ▶ %s", pool))
			} else {
				line = fmt.Sprintf("    %s", pool)
			}
			centered := lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Render(line)
			b.WriteString(centered + "\n")
		}
	}

	return b.String()
}

// renderConfirmContent renders the confirmation dialog
func (m model) renderConfirmContent(width int) string {
	var b strings.Builder

	// Check if this is a destructive operation
	isDestructive := m.operation == "force-backup" || m.operation == "prepare"

	if isDestructive {
		// Show strong warning for destructive operations
		warningBox := lipgloss.NewStyle().
			Width(width - 4).
			Align(lipgloss.Center).
			Render(destructiveWarningStyle.Render(
				"⚠️  DESTRUCTIVE OPERATION WARNING ⚠️\n\n" +
					m.confirmMsg + "\n\n" +
					"THIS ACTION CANNOT BE UNDONE!"))
		b.WriteString(warningBox + "\n\n")
	} else {
		safeBox := lipgloss.NewStyle().
			Width(width - 4).
			Align(lipgloss.Center).
			Render(safeOperationStyle.Render(m.confirmMsg))
		b.WriteString(safeBox + "\n\n")
	}

	return b.String()
}

// renderInputContent renders the device input form
func (m model) renderInputContent(width int) string {
	var b strings.Builder

	contentTitle := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(selectedItemStyle.Render("🔧 Prepare Backup Device"))
	b.WriteString(contentTitle + "\n\n")

	prompt := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(infoStyle.Render("Enter the device path to use for backup:"))
	b.WriteString(prompt + "\n\n")

	// Input field centered
	inputBox := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(m.input.View())
	b.WriteString(inputBox + "\n\n")

	hint := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(subtitleStyle.Render("Example: /dev/sda"))
	b.WriteString(hint + "\n")

	return b.String()
}

// renderPasswordContent renders the password input form
func (m model) renderPasswordContent(width int) string {
	var b strings.Builder

	contentTitle := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(selectedItemStyle.Render("🔐 Encryption Password"))
	b.WriteString(contentTitle + "\n\n")

	poolInfo := m.destPool
	if poolInfo == "" {
		poolInfo = "the backup pool"
	}

	prompt := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(infoStyle.Render(fmt.Sprintf("Enter the encryption password for %s:", poolInfo)))
	b.WriteString(prompt + "\n\n")

	// Password field centered
	passBox := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(m.passwordInput.View())
	b.WriteString(passBox + "\n\n")

	hint := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(subtitleStyle.Render("The password will be used to unlock the encrypted ZFS pool"))
	b.WriteString(hint + "\n")

	return b.String()
}

// renderRunningContent renders the operation progress view
func (m model) renderRunningContent(width int) string {
	var b strings.Builder

	contentTitle := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(selectedItemStyle.Render("⚙️  Backup in Progress"))
	b.WriteString(contentTitle + "\n\n")

	// Current stage with spinner
	var stageText string
	if m.currentStage != "" {
		stageText = m.spinner.View() + " " + m.currentStage
	} else {
		stageText = m.spinner.View() + " " + m.operation
	}
	stageLine := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(stageText)
	b.WriteString(stageLine + "\n\n")

	// Progress bar
	if m.backupState != nil && m.totalStages > 0 {
		percent := float64(len(m.backupState.CompletedStages)) / float64(m.totalStages)
		progressBar := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(m.progress.ViewAs(percent))
		b.WriteString(progressBar + "\n\n")

		// Stage count
		completed := len(m.backupState.CompletedStages)
		stageCount := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(infoStyle.Render(fmt.Sprintf("Stage %d of %d", completed+1, m.totalStages)))
		b.WriteString(stageCount + "\n")

		// ETA
		if m.eta > 0 {
			etaLine := lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Render(infoStyle.Render(fmt.Sprintf("Estimated time remaining: %s", formatDuration(m.eta))))
			b.WriteString(etaLine + "\n")
		}

		// Elapsed time
		elapsed := time.Since(m.backupState.StartTime)
		elapsedLine := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(infoStyle.Render(fmt.Sprintf("Elapsed: %s", formatDuration(elapsed))))
		b.WriteString(elapsedLine + "\n\n")
	} else {
		initLine := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(infoStyle.Render("Initializing..."))
		b.WriteString(initLine + "\n\n")
	}

	return b.String()
}

// renderResumeContent renders the resume prompt
func (m model) renderResumeContent(width int) string {
	var b strings.Builder

	contentTitle := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(selectedItemStyle.Render("📥 Resume Previous Backup"))
	b.WriteString(contentTitle + "\n\n")

	if m.resumeState != nil {
		info1 := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(infoStyle.Render(fmt.Sprintf("Found incomplete %s operation from:", m.resumeState.Operation)))
		b.WriteString(info1 + "\n")

		info2 := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(infoStyle.Render(fmt.Sprintf("Started: %s", m.resumeState.StartTime.Format("2006-01-02 15:04:05"))))
		b.WriteString(info2 + "\n")

		info3 := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(infoStyle.Render(fmt.Sprintf("Completed stages: %d", len(m.resumeState.CompletedStages))))
		b.WriteString(info3 + "\n\n")

		var warningText string
		if m.resumeState.Cancelled {
			warningText = "This backup was cancelled."
		} else {
			warningText = "This backup was interrupted."
		}
		warning := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(warningStyle.Render(warningText))
		b.WriteString(warning + "\n\n")
	}

	question := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(infoStyle.Render("Would you like to resume from where it left off?"))
	b.WriteString(question + "\n")

	return b.String()
}

// renderResultContent renders the operation result
func (m model) renderResultContent(width int) string {
	var b strings.Builder

	if m.err != nil {
		contentTitle := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(errorStyle.Render("❌ Operation Failed"))
		b.WriteString(contentTitle + "\n\n")

		errMsg := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(errorStyle.Render(m.err.Error()))
		b.WriteString(errMsg + "\n")
	} else {
		contentTitle := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(statusStyle.Render("✅ Operation Completed"))
		b.WriteString(contentTitle + "\n\n")

		msg := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(statusStyle.Render(m.message))
		b.WriteString(msg + "\n")
	}

	return b.String()
}

// renderHelpContent renders the help screen
func (m model) renderHelpContent(width int) string {
	help := `DESCRIPTION
  A beautiful TUI for managing ZFS backups.

OPERATIONS
  📦 Backup ZFS (incremental)
     Performs an incremental backup using syncoid.

  🔥 Force Backup ZFS (destructive)
     Forces a complete backup by deleting previous snapshots.

  📥 Restore Files
     Browse snapshots and restore files to any location.

  🔧 Prepare Backup Device
     Creates an encrypted ZFS pool on a new external drive.

  🔌 Unmount Backup Disk
     Safely exports the pool and powers off the USB drive.

REQUIREMENTS
  • syncoid installed (from sanoid package)
  • ZFS filesystem with source pool
  • External drive for backup pool
  • Root privileges (sudo) OR ZFS delegation configured

DOCUMENTATION
  Press D to open online documentation
  https://kartoza.github.io/zfs-backup

Press esc/enter/q to return to menu`

	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(reportBoxStyle.Render(help))
}

// openURL opens a URL in the default browser
func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		// Try xdg-open first (Linux), then open (macOS), then start (Windows)
		cmd = exec.Command("xdg-open", url)
		if err := cmd.Start(); err != nil {
			// Fallback attempts could be added here
			return nil
		}
		return nil
	}
}

type operationResultMsg struct {
	message string
	err     error
}

type progressMsg struct {
	stage       string
	progress    float64
	totalStages int
	eta         time.Duration
}

type tickMsg time.Time

func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	} else if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func (m model) startOperation() (model, tea.Cmd) {
	m.state = stateRunning
	var cmds []tea.Cmd

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	// Create progress channel for real-time updates
	m.progressChan = make(chan progressUpdate, 10)

	// Check if resuming
	var resumeFrom *BackupState
	if m.resumeState != nil {
		resumeFrom = m.resumeState
		m.resumeState = nil
	}

	switch m.operation {
	case "backup":
		cmds = append(cmds, runBackup(ctx, m.password, m.sourcePool, m.destPool, resumeFrom, m.progressChan))
	case "force-backup":
		cmds = append(cmds, runForceBackup(ctx, m.password, m.sourcePool, m.destPool, resumeFrom, m.progressChan))
	case "prepare":
		cmds = append(cmds, runPrepare(m.devicePath, m.destPool))
	case "unmount":
		cmds = append(cmds, runUnmount(m.destPool))
	}

	// Add command to listen for progress updates
	cmds = append(cmds, listenForProgress(m.progressChan))

	// Clear password from memory
	m.password = ""
	m.passwordInput.SetValue("")

	cmds = append(cmds, m.spinner.Tick, tickEvery())
	return m, tea.Batch(cmds...)
}

// listenForProgress creates a command that listens for progress updates
func listenForProgress(ch <-chan progressUpdate) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return nil
		}
		return progressUpdateMsg(update)
	}
}

// progressUpdateMsg wraps progressUpdate for the tea message system
type progressUpdateMsg progressUpdate

func isSudo() bool {
	// Check if running as root (effective user ID 0)
	return os.Geteuid() == 0
}

func checkPermissions() error {
	// First check if running with sudo
	if !isSudo() {
		var b strings.Builder
		b.WriteString("\n╔═══════════════════════════════════════════════════════╗\n")
		b.WriteString("║                  ⚠️  NOTICE  ⚠️                         ║\n")
		b.WriteString("╚═══════════════════════════════════════════════════════╝\n\n")
		b.WriteString("For the best experience, please run this application with sudo:\n\n")
		b.WriteString("    sudo zfs-backup\n\n")
		b.WriteString("This ensures proper permissions for ZFS operations.\n\n")
		b.WriteString("Alternatively, configure ZFS delegation for your user:\n")
		b.WriteString("    sudo zfs allow -u $USER create,destroy,snapshot,mount,send,receive <pool>\n\n")

		fmt.Fprintln(os.Stderr, warningStyle.Render(b.String()))
	}

	// Check if we can run zfs commands by testing a simple read-only operation
	cmd := exec.Command("zfs", "list", "-H", "-o", "name")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("insufficient permissions to run ZFS commands")
	}
	return nil
}

func main() {
	// Check permissions first
	if err := checkPermissions(); err != nil {
		fmt.Fprintln(os.Stderr, errorStyle.Render("⚠️  "+err.Error()))
		os.Exit(1)
	}

	// Handle command-line arguments
	if len(os.Args) > 1 {
		handleCLI()
		return
	}

	// Check for incomplete backup
	m := initialModel()
	if state, err := LoadBackupState(); err == nil && state != nil {
		// Found incomplete backup
		m.resumeState = state
		m.state = stateResume
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func handleCLI() {
	arg := os.Args[1]
	switch arg {
	case "--backup", "-b":
		fmt.Println(statusStyle.Render("📦 Running incremental backup..."))
		runBackupSync()
	case "--force-backup", "-f":
		fmt.Println(warningStyle.Render("🔥 Running force backup..."))
		runForceBackupSync()
	case "--unmount", "-u":
		fmt.Println(infoStyle.Render("🔌 Unmounting backup disk..."))
		runUnmountSync()
	case "--help", "-h":
		showCLIHelp()
	default:
		fmt.Fprintf(os.Stderr, errorStyle.Render("❌ Unknown option: %s\n"), arg)
		fmt.Fprintln(os.Stderr, "Run 'zfs-backup --help' for usage information")
		os.Exit(1)
	}
}

func showCLIHelp() {
	// Header
	fmt.Println()
	fmt.Println(titleStyle.Render(appName))
	fmt.Println(taglineStyle.Render(appTagline))
	fmt.Println(interstitialStyle.Render(strings.Repeat("─", 50)))
	fmt.Println(statusLineStyle.Render(fmt.Sprintf("Version %s", appVersion)))
	fmt.Println(interstitialStyle.Render(strings.Repeat("─", 50)))
	fmt.Println()

	help := `Usage: zfs-backup [OPTIONS]

Options:
  -b, --backup          Run incremental backup
  -f, --force-backup    Force backup (destructive)
  -u, --unmount         Unmount and power off backup disk
  -h, --help            Show this help message

If no options are provided, an interactive TUI menu will be displayed.

Examples:
  sudo zfs-backup              # Show interactive menu
  sudo zfs-backup --backup     # Run incremental backup
  sudo zfs-backup --unmount    # Unmount backup disk

Note: If you have ZFS delegation configured for your user, you can omit sudo.`

	fmt.Println(reportBoxStyle.Render(help))

	// Footer
	fmt.Println()
	fmt.Println(interstitialStyle.Render(strings.Repeat("─", 50)))
	fmt.Println(footerCreditStyle.Render("Made with 💗 by Kartoza │ Donate! │ GitHub"))
	fmt.Println()
}
