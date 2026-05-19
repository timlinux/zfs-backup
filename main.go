package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// appVersion is set at build time via -ldflags "-X main.appVersion=..."
// Source of truth: VERSION file in the repo root.
var appVersion = "dev"

// Application constants
const (
	appName    = "Kartoza ZFS Backup"
	appTagline = "Keep your ZFS Backed Up!"

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
	// Format: Made with <3 by [K]artoza | D[o]nate! | [G]itHub
	kartoza := selectedItemStyle.Render("K") + footerCreditStyle.Render("artoza")
	donate := footerCreditStyle.Render("D") + selectedItemStyle.Render("o") + footerCreditStyle.Render("nate!")
	github := selectedItemStyle.Render("G") + footerCreditStyle.Render("itHub")
	credit := footerCreditStyle.Render("Made with <3 by ") + kartoza +
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
	if m.selectingSavedHost {
		return "Select Remote Host"
	}
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
	case stateZpoolInfo:
		return "Pool Information"
	case stateMaintenance:
		return "Pool Maintenance"
	case stateQuotaManage:
		return "Dataset Manager"
	case stateReports:
		if m.reportViewing {
			return "Viewing Report"
		}
		return "Browse Reports"
	default:
		return "Idle"
	}
}

// getHotkeys returns the appropriate hotkeys for the current state
func (m model) getHotkeys() string {
	if m.selectingSavedHost {
		return "↑/k up • ↓/j down • enter select • d delete • esc cancel"
	}
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
	case stateResult:
		if m.lastReportPdf != "" || m.lastReportMd != "" {
			return "p open report • enter/esc return to menu"
		}
		return "enter/esc return to menu"
	case stateHelp, stateZpoolInfo:
		return "enter/esc return to menu"
	case stateMaintenance:
		return "s start scrub • x stop scrub • r refresh • esc return"
	case stateQuotaManage:
		return "↑/k up • ↓/j down • e edit quota • c create • d delete • n no quota • esc return"
	case stateReports:
		if m.reportViewing {
			return "scroll up/down • esc back to list"
		}
		return "↑/k up • ↓/j down • enter view • p open report • d delete • esc return"
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
	stateZpoolInfo
	stateMaintenance
	stateQuotaManage
	stateReports
)

type menuItem struct {
	title       string
	description string
	icon        string
}

// Simple menu items for main menu
var mainMenuItems = []menuItem{
	{title: "Backup ZFS (incremental)", description: "Run incremental backup from local source to destination pool using syncoid", icon: ""},
	{title: "Pull Remote Backup", description: "Pull incremental backup from a remote host via SSH to local backup pool", icon: ""},
	{title: "Push Backup to Remote", description: "Push local ZFS snapshots to a backup pool on a remote server via SSH", icon: ""},
	{title: "Restore Files", description: "Browse snapshots and restore files to any location", icon: ""},
	{title: "Show zpool info", description: "Show detailed information about ZFS pool structure, status and health", icon: ""},
	{title: "Pool Maintenance", description: "Start, stop, or monitor scrub operations for data integrity verification", icon: ""},
	{title: "Manage Datasets", description: "View/edit quotas, create and delete ZFS datasets", icon: ""},
	{title: "Browse Reports", description: "View previous backup reports with timings, sizes, and error details", icon: ""},
	{title: "Recover Failed Backup", description: "Fix broken sync state when backup was interrupted or snapshot was deleted", icon: ""},
	{title: "Unmount Backup Disk", description: "Safely export the backup pool and power off the USB drive", icon: ""},
	{title: "Help", description: "Show detailed help information about all operations", icon: ""},
	{title: "Exit", description: "Exit the application", icon: ""},
	{title: "---", description: "Danger Zone", icon: ""}, // Separator
	{title: "Prepare Backup Device", description: "DESTRUCTIVE - Create an encrypted ZFS pool on a new external backup device", icon: ""},
	{title: "Force Backup ZFS (destructive)", description: "DESTRUCTIVE - Deletes old snapshots on backup disk and forces full sync", icon: ""},
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
	progressChan       chan progressUpdate
	// Per-dataset sync progress (populated during sync stage)
	datasetProgress    []DatasetProgress
	currentDataset     int // Index of currently syncing dataset (-1 if none)
	operationStartTime time.Time // When the current operation started
	// Restore mode
	restoreModel     RestoreModel
	// Zpool info viewer
	zpoolInfoPool    string           // Selected pool for info display
	zpoolViewport    viewport.Model   // Scrollable viewport for info
	zpoolInfoReady   bool             // Is the viewport content ready?
	// Maintenance
	maintenancePool    string         // Selected pool for maintenance
	maintenanceAction  string         // Current maintenance action
	scrubProgress      string         // Current scrub progress info
	maintenanceReady   bool           // Is maintenance info ready?
	// Result viewport
	resultViewport     viewport.Model // Scrollable viewport for result content
	resultReady        bool           // Is result viewport ready?
	// Prepare operation phases
	preparePhase       int            // 0 = device path input, 1 = pool name input
	// Remote backup
	remoteHost         string         // SSH host for remote backup (user@host)
	remoteDataset      string         // Remote dataset path (e.g., NIXROOT/home)
	remoteInputPhase   int            // 0 = host input, 1 = dataset input
	isRemote           bool           // Whether current operation is remote
	// Saved remote hosts
	savedHosts         []RemoteHost   // Loaded from config
	savedHostIndex     int            // Selection cursor in saved host list
	selectingSavedHost bool           // Are we showing the saved host picker?
	// Quota/Dataset management
	quotaDatasets      []quotaEntry   // Datasets with quota info
	quotaIndex         int            // Current row cursor
	quotaEditing       bool           // Are we editing a quota value?
	quotaInput         textinput.Model // Input for editing quota
	quotaPool          string         // Pool being managed
	quotaPoolSize      string         // Total pool size
	quotaPoolFree      string         // Free space on pool
	// Dataset creation form
	datasetCreating    bool           // Are we in the create dataset form?
	datasetFormField   int            // Current field in create form
	datasetForm        datasetCreateForm // Form data
	// Dataset deletion
	datasetDeleting    bool           // Are we confirming a delete?
	// Report browser
	reportFiles        []reportEntry  // List of report files
	reportIndex        int            // Selection cursor in report list
	reportViewport     viewport.Model // Viewport for viewing a report
	reportViewing      bool           // Are we viewing a report (vs listing)?
	// Last generated report (for opening from result screen)
	lastReportMd       string         // Path to last generated markdown report
	lastReportPdf      string         // Path to last generated PDF report
}

// =============================================================================
// Quota Management Types
// =============================================================================

// quotaEntry holds quota information for a single dataset
// reportEntry represents a report file in the report browser
type reportEntry struct {
	Name    string    // Filename (without path)
	Path    string    // Full path to the .md file
	PdfPath string    // Full path to the .pdf file (may not exist)
	ModTime time.Time // Last modified time
}

// reportsLoadedMsg is sent when report files are loaded
type reportsLoadedMsg struct {
	reports []reportEntry
	err     error
}

// reportContentMsg is sent when a report's content is loaded
type reportContentMsg struct {
	content string
	err     error
}

// loadReportFiles scans the reports directory and returns entries
func loadReportFiles() tea.Cmd {
	return func() tea.Msg {
		dir, err := getReportsDir()
		if err != nil {
			return reportsLoadedMsg{err: err}
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return reportsLoadedMsg{err: err}
		}

		var reports []reportEntry
		seen := make(map[string]bool)
		// Walk in reverse so newest files are first
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			baseName := strings.TrimSuffix(name, ".md")
			if seen[baseName] {
				continue
			}
			seen[baseName] = true

			info, _ := e.Info()
			modTime := time.Time{}
			if info != nil {
				modTime = info.ModTime()
			}

			pdfPath := filepath.Join(dir, baseName+".pdf")
			if _, err := os.Stat(pdfPath); err != nil {
				pdfPath = ""
			}

			reports = append(reports, reportEntry{
				Name:    baseName,
				Path:    filepath.Join(dir, name),
				PdfPath: pdfPath,
				ModTime: modTime,
			})
		}

		return reportsLoadedMsg{reports: reports}
	}
}

// loadReportContent reads a report file for display
func loadReportContent(path string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return reportContentMsg{err: err}
		}
		return reportContentMsg{content: string(data)}
	}
}

type quotaEntry struct {
	Name        string // Dataset name (e.g., NIXROOT/home)
	Type        string // "filesystem" or "volume"
	Quota       string // Current quota (e.g., "3T", "none")
	Used        string // Current usage
	Available   string // Available space
	SupportsQuota bool // Whether this dataset type supports quotas
}

// datasetCreateForm holds the form state for creating a new dataset
type datasetCreateForm struct {
	Name        string // Dataset name (relative to pool)
	Type        string // "filesystem" or "volume"
	Quota       string // Optional quota
	RecordSize  string // Record size (e.g., 128K, 1M)
	Compression string // Compression algorithm
	Atime       string // "on" or "off"
	VolumeSize  string // Volume size (only for volumes)
}

// datasetCreateFormFields are the field labels for the create form
var datasetCreateFormFields = []string{
	"Name",
	"Type (filesystem/volume)",
	"Quota",
	"Record Size",
	"Compression",
	"Atime (on/off)",
	"Volume Size (volumes only)",
}

// datasetCreateFormDefaults returns sensible defaults for a new dataset
func datasetCreateFormDefaults() datasetCreateForm {
	return datasetCreateForm{
		Type:        "filesystem",
		RecordSize:  "128K",
		Compression: "zstd",
		Atime:       "off",
	}
}

// datasetDeletedMsg is sent when a dataset is deleted
type datasetDeletedMsg struct {
	err error
}

// createDataset creates a new ZFS dataset with the given properties
func createDataset(pool string, form datasetCreateForm) tea.Cmd {
	return func() tea.Msg {
		fullName := fmt.Sprintf("%s/%s", pool, form.Name)

		args := []string{"create"}

		// Set properties
		if form.Quota != "" && form.Quota != "none" {
			args = append(args, "-o", fmt.Sprintf("quota=%s", form.Quota))
		}
		if form.RecordSize != "" {
			args = append(args, "-o", fmt.Sprintf("recordsize=%s", form.RecordSize))
		}
		if form.Compression != "" {
			args = append(args, "-o", fmt.Sprintf("compression=%s", form.Compression))
		}
		if form.Atime != "" {
			args = append(args, "-o", fmt.Sprintf("atime=%s", form.Atime))
		}

		if form.Type == "volume" {
			// Volumes need -V flag with size
			if form.VolumeSize == "" {
				return quotaSetMsg{err: fmt.Errorf("volume size is required for volume type")}
			}
			args = append(args, "-V", form.VolumeSize, fullName)
		} else {
			args = append(args, fullName)
		}

		if err := runCommand("zfs", args...); err != nil {
			return quotaSetMsg{err: fmt.Errorf("failed to create dataset: %w", err)}
		}
		return quotaSetMsg{}
	}
}

// deleteDataset destroys a ZFS dataset after safety checks
func deleteDataset(name string) tea.Cmd {
	return func() tea.Msg {
		// Safety check: ensure no child datasets
		output, err := runCommandOutput("zfs", "list", "-H", "-r", "-o", "name", name)
		if err != nil {
			return datasetDeletedMsg{err: fmt.Errorf("failed to check dataset: %w", err)}
		}
		children := 0
		for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
			if line != "" && line != name {
				children++
			}
		}
		if children > 0 {
			return datasetDeletedMsg{err: fmt.Errorf("dataset has %d child dataset(s) - delete children first", children)}
		}

		// Safety check: check for snapshots
		snapOutput, err := runCommandOutput("zfs", "list", "-H", "-t", "snapshot", "-o", "name", "-r", name)
		if err == nil {
			snapCount := 0
			for _, line := range strings.Split(strings.TrimSpace(snapOutput), "\n") {
				if line != "" {
					snapCount++
				}
			}
			if snapCount > 0 {
				return datasetDeletedMsg{err: fmt.Errorf("dataset has %d snapshot(s) - destroy snapshots first or use 'zfs destroy -r'", snapCount)}
			}
		}

		// Destroy the dataset
		if err := runCommand("zfs", "destroy", name); err != nil {
			return datasetDeletedMsg{err: fmt.Errorf("failed to destroy dataset: %w", err)}
		}
		return datasetDeletedMsg{}
	}
}

// quotaLoadedMsg is sent when quota data is loaded
type quotaLoadedMsg struct {
	datasets []quotaEntry
	poolSize string
	poolFree string
	err      error
}

// quotaSetMsg is sent when a quota is set
type quotaSetMsg struct {
	err error
}

// loadQuotaData fetches dataset quota information for a pool
func loadQuotaData(pool string) tea.Cmd {
	return func() tea.Msg {
		// Get pool size info
		poolInfo, err := runCommandOutput("zpool", "list", "-H", "-o", "size,free", pool)
		if err != nil {
			return quotaLoadedMsg{err: fmt.Errorf("failed to get pool info: %w", err)}
		}
		fields := strings.Fields(strings.TrimSpace(poolInfo))
		var poolSize, poolFree string
		if len(fields) >= 2 {
			poolSize = fields[0]
			poolFree = fields[1]
		}

		// Get all datasets with quota info
		output, err := runCommandOutput("zfs", "list", "-H", "-r", "-o", "name,type,quota,used,avail", pool)
		if err != nil {
			return quotaLoadedMsg{err: fmt.Errorf("failed to list datasets: %w", err)}
		}

		var datasets []quotaEntry
		for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 5 {
				continue
			}

			dsType := parts[1]
			supportsQuota := dsType == "filesystem"

			quota := parts[2]
			if quota == "-" {
				quota = "none"
			}

			datasets = append(datasets, quotaEntry{
				Name:          parts[0],
				Type:          dsType,
				Quota:         quota,
				Used:          parts[3],
				Available:     parts[4],
				SupportsQuota: supportsQuota,
			})
		}

		return quotaLoadedMsg{
			datasets: datasets,
			poolSize: poolSize,
			poolFree: poolFree,
		}
	}
}

// setQuota applies a quota to a dataset
func setQuota(dataset, value string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if value == "" || strings.ToLower(value) == "none" {
			err = runCommand("zfs", "set", "quota=none", dataset)
		} else {
			err = runCommand("zfs", "set", fmt.Sprintf("quota=%s", value), dataset)
		}
		if err != nil {
			return quotaSetMsg{err: err}
		}
		return quotaSetMsg{}
	}
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

	// Auto-detect source and destination pools:
	// - Source: first pool that does NOT contain "BACKUP" (case-insensitive)
	// - Destination: first pool that DOES contain "BACKUP" (case-insensitive)
	var sourcePool, destPool string
	for _, p := range pools {
		upper := strings.ToUpper(p)
		if strings.Contains(upper, "BACKUP") {
			if destPool == "" {
				destPool = p
			}
		} else {
			if sourcePool == "" {
				sourcePool = p
			}
		}
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

// startPoolSelection sets up the model for pool selection with smart defaults.
// If selectSource is true, it starts with source selection (prefers non-BACKUP pool).
// If selectSource is false, it only selects a destination (prefers BACKUP pool).
func (m *model) startPoolSelection(selectSource bool) {
	m.availablePools = getAllPools()
	m.selectingPool = true
	m.selectingSource = selectSource
	m.poolSelectIndex = 0

	// Pre-select a smart default based on BACKUP keyword
	if selectSource {
		// Source: prefer a pool WITHOUT "BACKUP" in its name
		for i, p := range m.availablePools {
			if !strings.Contains(strings.ToUpper(p), "BACKUP") {
				m.poolSelectIndex = i
				break
			}
		}
	} else {
		// Destination: prefer a pool WITH "BACKUP" in its name
		for i, p := range m.availablePools {
			if strings.Contains(strings.ToUpper(p), "BACKUP") {
				m.poolSelectIndex = i
				break
			}
		}
	}
}

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
		// Update viewport size if in zpool info state
		if m.state == stateZpoolInfo && m.zpoolInfoReady {
			viewportHeight := msg.Height - 12
			if viewportHeight < 5 {
				viewportHeight = 5
			}
			viewportWidth := msg.Width - 8
			if viewportWidth < 40 {
				viewportWidth = 40
			}
			m.zpoolViewport.Width = viewportWidth
			m.zpoolViewport.Height = viewportHeight
		}
		// Update viewport size if viewing a report
		if m.state == stateReports && m.reportViewing {
			viewportHeight := msg.Height - 12
			if viewportHeight < 5 {
				viewportHeight = 5
			}
			viewportWidth := msg.Width - 8
			if viewportWidth < 40 {
				viewportWidth = 40
			}
			m.reportViewport.Width = viewportWidth
			m.reportViewport.Height = viewportHeight
		}
		// Update viewport size if in result state
		if m.state == stateResult && m.resultReady {
			viewportHeight := msg.Height - 12
			if viewportHeight < 5 {
				viewportHeight = 5
			}
			viewportWidth := msg.Width - 8
			if viewportWidth < 40 {
				viewportWidth = 40
			}
			m.resultViewport.Width = viewportWidth
			m.resultViewport.Height = viewportHeight
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

		// Handle saved remote host selection
		if m.selectingSavedHost {
			// Last item is always "+ Add new host"
			totalItems := len(m.savedHosts) + 1
			switch msg.String() {
			case "up", "k":
				if m.savedHostIndex > 0 {
					m.savedHostIndex--
				}
				return m, nil
			case "down", "j":
				if m.savedHostIndex < totalItems-1 {
					m.savedHostIndex++
				}
				return m, nil
			case "enter":
				m.selectingSavedHost = false
				if m.savedHostIndex < len(m.savedHosts) {
					// Selected a saved host - use its details
					host := m.savedHosts[m.savedHostIndex]
					m.remoteHost = host.SSHHost
					m.remoteDataset = host.Dataset
					if m.operation == "push-backup" {
						// For push: select local source pool
						m.startPoolSelection(true)
					} else {
						// For pull: select local destination pool
						m.startPoolSelection(false)
					}
				} else {
					// "+ Add new host" selected
					m.state = stateInput
					m.remoteInputPhase = 0
					m.input.Placeholder = "user@hostname"
					m.input.SetValue("")
					return m, textinput.Blink
				}
				return m, nil
			case "d", "D":
				// Delete selected host
				if m.savedHostIndex < len(m.savedHosts) {
					_ = RemoveRemoteHost(m.savedHostIndex)
					// Reload
					if config, err := LoadRemoteHosts(); err == nil {
						m.savedHosts = config.Hosts
					}
					if m.savedHostIndex >= len(m.savedHosts)+1 {
						m.savedHostIndex = len(m.savedHosts)
					}
				}
				return m, nil
			case "esc":
				m.selectingSavedHost = false
				m.state = stateMenu
				return m, nil
			}
			return m, nil
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

						// For push-backup, skip dest selection (dest is remote)
						if m.operation == "push-backup" {
							m.selectingPool = false
							// Check if pool needs import/unlock before prompting
							return m, m.preparePoolAccess(selectedPool)
						}

						// Now select destination
						m.selectingSource = false
						m.poolSelectIndex = 0
						// Default to a pool with BACKUP in its name
						for i, p := range m.availablePools {
							if p != m.sourcePool && strings.Contains(strings.ToUpper(p), "BACKUP") {
								m.poolSelectIndex = i
								break
							}
						}
						// Fallback: first pool different from source
						if m.poolSelectIndex == 0 && len(m.availablePools) > 0 && m.availablePools[0] == m.sourcePool {
							for i, p := range m.availablePools {
								if p != m.sourcePool {
									m.poolSelectIndex = i
									break
								}
							}
						}
					} else {
						m.destPool = selectedPool
						m.selectingPool = false

						// Set up operation-specific pool references
						if m.operation == "zpoolinfo" {
							m.zpoolInfoPool = selectedPool
						} else if m.operation == "maintenance" {
							m.maintenancePool = selectedPool
						} else if m.operation == "quotas" {
							m.quotaPool = selectedPool
						}

						// Smart pool access: import if needed, skip password if already unlocked
						return m, m.preparePoolAccess(selectedPool)
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

			case "o", "O":
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
					m.isRemote = false
					m.startPoolSelection(true)
					return m, nil
				case "Pull Remote Backup":
					m.operation = "remote-backup"
					m.isRemote = true
					// Load saved hosts
					if config, err := LoadRemoteHosts(); err == nil && len(config.Hosts) > 0 {
						m.savedHosts = config.Hosts
						m.savedHostIndex = 0
						m.selectingSavedHost = true
					} else {
						m.state = stateInput
						m.remoteInputPhase = 0
						m.input.Placeholder = "user@hostname"
						m.input.SetValue("")
						return m, textinput.Blink
					}
					return m, nil
				case "Push Backup to Remote":
					m.operation = "push-backup"
					m.isRemote = true
					// Load saved hosts for push target
					if config, err := LoadRemoteHosts(); err == nil && len(config.Hosts) > 0 {
						m.savedHosts = config.Hosts
						m.savedHostIndex = 0
						m.selectingSavedHost = true
					} else {
						m.state = stateInput
						m.remoteInputPhase = 0
						m.input.Placeholder = "user@hostname"
						m.input.SetValue("")
						return m, textinput.Blink
					}
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
					m.confirmMsg = "WARNING: This will delete all previous snapshots on the backup disk.\nAre you sure you want to continue?"
					m.operation = "force-backup"
					m.confirmYes = false
					return m, nil
				case "Prepare Backup Device":
					m.state = stateInput
					m.operation = "prepare"
					m.preparePhase = 0 // Start with device path input
					m.input.Placeholder = "/dev/sda"
					m.input.SetValue("")
					return m, textinput.Blink
				case "Show zpool info":
					m.operation = "zpoolinfo"
					m.startPoolSelection(false)
					return m, nil
				case "Pool Maintenance":
					m.operation = "maintenance"
					m.startPoolSelection(false)
					return m, nil
				case "Manage Datasets":
					m.operation = "quotas"
					m.startPoolSelection(false)
					return m, nil
				case "Browse Reports":
					m.state = stateReports
					m.reportViewing = false
					m.reportIndex = 0
					return m, loadReportFiles()
				case "Recover Failed Backup":
					m.operation = "recover"
					m.startPoolSelection(true)
					return m, nil
				case "Unmount Backup Disk":
					m.operation = "unmount"
					m.startPoolSelection(false)
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
					// Go to pool selection - set state to stateMenu
					// so the pool selection UI renders correctly
					m.state = stateMenu
					m.startPoolSelection(true)
					return m, nil
				} else if m.operation == "prepare" {
					// For prepare, go to password state to collect encryption passphrase
					m.state = statePassword
					m.passwordInput.SetValue("")
					m.passwordInput.Focus()
					return m, textinput.Blink
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
					if (m.operation == "remote-backup" || m.operation == "push-backup") && m.remoteInputPhase == 0 {
						// Phase 0: got remote host, now ask for dataset/pool
						m.remoteHost = m.input.Value()
						m.remoteInputPhase = 1
						if m.operation == "push-backup" {
							m.input.Placeholder = "NIXBACKUPS"
							m.input.SetValue("NIXBACKUPS")
						} else {
							m.input.Placeholder = "NIXROOT/home"
							m.input.SetValue("NIXROOT/home")
						}
						return m, nil
					} else if (m.operation == "remote-backup" || m.operation == "push-backup") && m.remoteInputPhase == 1 {
						// Phase 1: got dataset, save host profile
						m.remoteDataset = m.input.Value()
						_ = AddRemoteHost(m.remoteHost, m.remoteDataset)
						if m.operation == "push-backup" {
							// Push: select local source pool
							m.startPoolSelection(true)
						} else {
							// Pull: select local destination pool
							m.startPoolSelection(false)
						}
						return m, nil
					} else if m.operation == "prepare" && m.preparePhase == 0 {
						// Phase 0: got device path, now ask for pool name
						m.devicePath = m.input.Value()
						m.preparePhase = 1
						m.input.Placeholder = "NIXBACKUPS"
						m.input.SetValue("NIXBACKUPS") // Default to NIXBACKUPS
						return m, nil
					} else if m.operation == "prepare" && m.preparePhase == 1 {
						// Phase 1: got pool name, go to confirm
						m.destPool = m.input.Value()
						m.state = stateConfirm
						m.confirmMsg = fmt.Sprintf("WARNING: You are about to erase all data on %s.\nThis will create encrypted ZFS pool '%s'.\nThis action is irreversible!\nAre you absolutely sure?", m.devicePath, m.destPool)
						m.confirmYes = false
						return m, nil
					}
					// Original behavior for other operations
					m.devicePath = m.input.Value()
					m.state = stateConfirm
					m.confirmMsg = fmt.Sprintf("WARNING: You are about to erase all data on %s.\nThis action is irreversible!\nAre you absolutely sure?", m.input.Value())
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
					// Handle zpoolinfo, maintenance, and quotas specially
					if m.operation == "zpoolinfo" {
						return m, m.unlockAndLoadZpoolInfo()
					}
					if m.operation == "maintenance" {
						return m, m.unlockAndLoadMaintenance()
					}
					if m.operation == "quotas" {
						// Unlock then load quotas
						return m, m.unlockAndLoadQuotas()
					}
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
				m.message = "Backup cancelled. You can resume later by starting a new backup."
				m.err = nil
				m.cancelFunc = nil
				return m, nil
			}
		} else if m.state == stateResume {
			switch msg.String() {
			case "y", "Y":
				// Resume the backup - check if pool still needs unlock
				m.operation = m.resumeState.Operation
				return m, m.preparePoolAccess(m.destPool)
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
				m.resultReady = false
				return m, nil
			case "p":
				// Open report (PDF or markdown fallback)
				if m.lastReportPdf != "" {
					_ = openFileForUser(m.lastReportPdf)
				} else if m.lastReportMd != "" {
					_ = openFileForUser(m.lastReportMd)
				}
				return m, nil
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			default:
				// Pass other keys to viewport for scrolling
				if m.resultReady {
					var cmd tea.Cmd
					m.resultViewport, cmd = m.resultViewport.Update(msg)
					return m, cmd
				}
			}
		} else if m.state == stateZpoolInfo {
			switch msg.String() {
			case "esc", "q":
				m.state = stateMenu
				m.zpoolInfoReady = false
				return m, nil
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			default:
				// Pass other keys to viewport for scrolling
				var cmd tea.Cmd
				m.zpoolViewport, cmd = m.zpoolViewport.Update(msg)
				return m, cmd
			}
		} else if m.state == stateReports {
			if m.reportViewing {
				// Viewing a report - scroll or go back
				switch msg.String() {
				case "esc", "q":
					m.reportViewing = false
					return m, nil
				case "ctrl+c":
					m.quitting = true
					return m, tea.Quit
				default:
					var cmd tea.Cmd
					m.reportViewport, cmd = m.reportViewport.Update(msg)
					return m, cmd
				}
			} else {
				// Browsing report list
				switch msg.String() {
				case "up", "k":
					if m.reportIndex > 0 {
						m.reportIndex--
					}
					return m, nil
				case "down", "j":
					if m.reportIndex < len(m.reportFiles)-1 {
						m.reportIndex++
					}
					return m, nil
				case "enter":
					if len(m.reportFiles) > 0 {
						return m, loadReportContent(m.reportFiles[m.reportIndex].Path)
					}
					return m, nil
				case "p":
					// Open report (PDF or markdown fallback)
					if len(m.reportFiles) > 0 {
						entry := m.reportFiles[m.reportIndex]
						if entry.PdfPath != "" {
							_ = openFileForUser(entry.PdfPath)
						} else {
							_ = openFileForUser(entry.Path)
						}
					}
					return m, nil
				case "d":
					// Delete selected report
					if len(m.reportFiles) > 0 {
						entry := m.reportFiles[m.reportIndex]
						_ = os.Remove(entry.Path)
						if entry.PdfPath != "" {
							_ = os.Remove(entry.PdfPath)
						}
						if m.reportIndex >= len(m.reportFiles)-1 && m.reportIndex > 0 {
							m.reportIndex--
						}
						return m, loadReportFiles()
					}
					return m, nil
				case "esc", "q":
					m.state = stateMenu
					return m, nil
				case "ctrl+c":
					m.quitting = true
					return m, tea.Quit
				}
				return m, nil
			}
		} else if m.state == stateMaintenance {
			switch msg.String() {
			case "esc", "q":
				m.state = stateMenu
				m.maintenanceReady = false
				return m, nil
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "s", "S":
				// Start scrub
				return m, m.startScrub()
			case "x", "X":
				// Stop scrub
				return m, m.stopScrub()
			case "r", "R":
				// Refresh status
				return m, m.loadMaintenanceStatus()
			default:
				// Pass other keys to viewport for scrolling
				var cmd tea.Cmd
				m.zpoolViewport, cmd = m.zpoolViewport.Update(msg)
				return m, cmd
			}
		} else if m.state == stateQuotaManage {
			// Handle dataset creation form
			if m.datasetCreating {
				return m.handleDatasetCreateForm(msg)
			}
			// Handle delete confirmation
			if m.datasetDeleting {
				switch msg.String() {
				case "y", "Y":
					m.datasetDeleting = false
					dataset := m.quotaDatasets[m.quotaIndex].Name
					return m, deleteDataset(dataset)
				case "n", "N", "esc":
					m.datasetDeleting = false
					return m, nil
				}
				return m, nil
			}
			// Handle quota editing
			if m.quotaEditing {
				switch msg.String() {
				case "enter":
					newQuota := m.quotaInput.Value()
					dataset := m.quotaDatasets[m.quotaIndex].Name
					m.quotaEditing = false
					return m, setQuota(dataset, newQuota)
				case "esc":
					m.quotaEditing = false
					return m, nil
				default:
					var cmd tea.Cmd
					m.quotaInput, cmd = m.quotaInput.Update(msg)
					return m, cmd
				}
			}
			// Normal navigation
			switch msg.String() {
			case "esc", "q":
				m.state = stateMenu
				m.quotaDatasets = nil
				return m, nil
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			case "up", "k":
				if m.quotaIndex > 0 {
					m.quotaIndex--
				}
				return m, nil
			case "down", "j":
				if m.quotaIndex < len(m.quotaDatasets)-1 {
					m.quotaIndex++
				}
				return m, nil
			case "enter", "e":
				// Edit quota for current dataset
				if m.quotaIndex < len(m.quotaDatasets) && m.quotaDatasets[m.quotaIndex].SupportsQuota {
					m.quotaEditing = true
					current := m.quotaDatasets[m.quotaIndex].Quota
					if current == "none" || current == "0" {
						m.quotaInput.SetValue("")
					} else {
						m.quotaInput.SetValue(current)
					}
					m.quotaInput.Focus()
					return m, textinput.Blink
				}
				return m, nil
			case "n":
				// Set quota to none (remove quota)
				if m.quotaIndex < len(m.quotaDatasets) && m.quotaDatasets[m.quotaIndex].SupportsQuota {
					dataset := m.quotaDatasets[m.quotaIndex].Name
					return m, setQuota(dataset, "none")
				}
				return m, nil
			case "c":
				// Create new dataset
				m.datasetCreating = true
				m.datasetFormField = 0
				m.datasetForm = datasetCreateFormDefaults()
				m.quotaInput.SetValue("")
				m.quotaInput.Placeholder = "dataset-name"
				m.quotaInput.Focus()
				return m, textinput.Blink
			case "d", "D":
				// Delete dataset (with confirmation)
				if m.quotaIndex < len(m.quotaDatasets) {
					ds := m.quotaDatasets[m.quotaIndex]
					// Don't allow deleting the pool root
					if ds.Name != m.quotaPool {
						m.datasetDeleting = true
					}
				}
				return m, nil
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
		m.datasetProgress = msg.datasets
		m.currentDataset = msg.currentDataset

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

	case zpoolInfoLoadedMsg:
		if msg.err != nil {
			m.state = stateResult
			m.err = msg.err
			m.message = ""
			return m, nil
		}
		// Set up viewport with the content
		m.state = stateZpoolInfo
		m.zpoolInfoReady = true
		// Calculate viewport height (leave room for header, title, and footer)
		viewportHeight := m.height - 12
		if viewportHeight < 5 {
			viewportHeight = 5
		}
		viewportWidth := m.width - 8
		if viewportWidth < 40 {
			viewportWidth = 40
		}
		m.zpoolViewport = viewport.New(viewportWidth, viewportHeight)
		m.zpoolViewport.SetContent(msg.content)
		m.zpoolViewport.Style = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorHighlight4).
			Padding(0, 1)
		return m, nil

	case poolReadyMsg:
		if msg.err != nil {
			m.state = stateResult
			m.err = msg.err
			m.message = ""
			return m, nil
		}
		// Pool is imported, now check if it needs unlocking
		if needsUnlock, _ := poolNeedsUnlock(msg.pool); needsUnlock {
			m.state = statePassword
			m.passwordInput.SetValue("")
			m.passwordInput.Focus()
			return m, textinput.Blink
		}
		// No password needed - route to the appropriate action
		switch m.operation {
		case "zpoolinfo":
			return m, m.loadZpoolInfo()
		case "quotas":
			return m, loadQuotaData(m.quotaPool)
		case "maintenance":
			return m, m.loadMaintenanceStatus()
		case "backup", "force-backup", "recover", "remote-backup", "push-backup":
			// Pool is already imported and unlocked - start the operation directly
			m.password = ""
			var newM model
			var cmd tea.Cmd
			newM, cmd = m.startOperation()
			return newM, cmd
		default:
			// Unmount, prepare, etc. - start directly
			m.password = ""
			var newM model
			var cmd tea.Cmd
			newM, cmd = m.startOperation()
			return newM, cmd
		}

	case maintenanceStatusMsg:
		if msg.err != nil {
			m.state = stateResult
			m.err = msg.err
			m.message = ""
			return m, nil
		}
		// Set up viewport with the content
		m.state = stateMaintenance
		m.maintenanceReady = true
		m.scrubProgress = msg.content
		// Calculate viewport height
		viewportHeight := m.height - 14
		if viewportHeight < 5 {
			viewportHeight = 5
		}
		viewportWidth := m.width - 8
		if viewportWidth < 40 {
			viewportWidth = 40
		}
		m.zpoolViewport = viewport.New(viewportWidth, viewportHeight)
		m.zpoolViewport.SetContent(msg.content)
		m.zpoolViewport.Style = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorHighlight4).
			Padding(0, 1)
		return m, nil

	case scrubActionMsg:
		if msg.err != nil {
			m.state = stateResult
			m.err = msg.err
			m.message = ""
			return m, nil
		}
		// Refresh the maintenance status after scrub action
		return m, m.loadMaintenanceStatus()

	case quotaLoadedMsg:
		if msg.err != nil {
			m.state = stateResult
			m.err = msg.err
			m.message = ""
			return m, nil
		}
		m.state = stateQuotaManage
		m.quotaDatasets = msg.datasets
		m.quotaPoolSize = msg.poolSize
		m.quotaPoolFree = msg.poolFree
		m.quotaIndex = 0
		m.quotaEditing = false
		// Initialize quota input
		qi := textinput.New()
		qi.Placeholder = "e.g., 500G, 2T, none"
		qi.CharLimit = 20
		qi.Width = 20
		qi.PromptStyle = lipgloss.NewStyle().Foreground(colorHighlight2)
		qi.TextStyle = lipgloss.NewStyle().Foreground(colorHighlight1)
		m.quotaInput = qi
		return m, nil

	case quotaSetMsg:
		if msg.err != nil {
			m.state = stateResult
			m.err = msg.err
			m.message = ""
			return m, nil
		}
		// Refresh quota data after change
		return m, loadQuotaData(m.quotaPool)

	case datasetDeletedMsg:
		if msg.err != nil {
			m.state = stateResult
			m.err = msg.err
			m.message = ""
			return m, nil
		}
		// Refresh after delete
		m.quotaIndex = 0
		return m, loadQuotaData(m.quotaPool)

	case reportsLoadedMsg:
		if msg.err != nil {
			m.state = stateResult
			m.err = msg.err
			m.message = ""
			return m, nil
		}
		m.reportFiles = msg.reports
		m.reportIndex = 0
		m.reportViewing = false
		return m, nil

	case reportContentMsg:
		if msg.err != nil {
			m.state = stateResult
			m.err = msg.err
			m.message = ""
			return m, nil
		}
		viewportHeight := m.height - 12
		if viewportHeight < 5 {
			viewportHeight = 5
		}
		viewportWidth := m.width - 8
		if viewportWidth < 40 {
			viewportWidth = 40
		}
		m.reportViewport = viewport.New(viewportWidth, viewportHeight)
		m.reportViewport.SetContent(msg.content)
		m.reportViewport.Style = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorHighlight4).
			Padding(0, 1)
		m.reportViewing = true
		return m, nil

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
		// Keep datasetProgress for the final report (don't clear it)
		m.currentDataset = -1

		// Set up viewport for scrollable result display
		viewportHeight := m.height - 12
		if viewportHeight < 5 {
			viewportHeight = 5
		}
		viewportWidth := m.width - 8
		if viewportWidth < 40 {
			viewportWidth = 40
		}

		// Build result content with dataset dashboard appended
		resultContent := msg.message
		if len(m.datasetProgress) > 0 {
			resultContent += "\n" + m.renderDatasetReport(viewportWidth)
		}

		// Write report to markdown and PDF for all operations
		reportInfo := ReportInfo{
			Operation:       m.operation,
			SourcePool:      m.sourcePool,
			DestPool:        m.destPool,
			RemoteHost:      m.remoteHost,
			StartTime:       m.operationStartTime,
			EndTime:         time.Now(),
			DatasetProgress: m.datasetProgress,
			Success:         msg.err == nil,
			OperationLog:    msg.message,
		}
		if msg.err != nil {
			reportInfo.ErrorMessage = msg.err.Error()
		}
		mdPath, pdfPath, reportErr := writeBackupReport(reportInfo)
		m.lastReportMd = ""
		m.lastReportPdf = ""
		if reportErr == nil {
			m.lastReportMd = mdPath
			m.lastReportPdf = pdfPath
			resultContent += "\n\nReport saved: " + mdPath
			if pdfPath != "" {
				resultContent += "\nPDF report:   " + pdfPath
				resultContent += "\n\nPress 'p' to open the report"
			}
		} else {
			resultContent += "\n\nWarning: could not write report: " + reportErr.Error()
		}

		m.resultViewport = viewport.New(viewportWidth, viewportHeight)
		m.resultViewport.SetContent(resultContent)
		m.resultViewport.Style = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorHighlight4).
			Padding(0, 1)
		m.resultReady = true
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
	case stateQuotaManage:
		if m.quotaEditing || m.datasetCreating {
			m.quotaInput, cmd = m.quotaInput.Update(msg)
		}
	}

	return m, cmd
}

func (m model) View() string {
	if m.quitting {
		return statusStyle.Render("Goodbye!\n")
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
	case stateZpoolInfo:
		content.WriteString(m.renderZpoolInfoContent(width))
	case stateMaintenance:
		content.WriteString(m.renderMaintenanceContent(width))
	case stateQuotaManage:
		content.WriteString(m.renderQuotaContent(width))
	case stateReports:
		content.WriteString(m.renderReportsContent(width))
	case stateRestore:
		// Restore mode has its own full view, handled in View()
	}

	return content.String()
}

// renderMenuContent renders the main menu view
func (m model) renderMenuContent(width, height int) string {
	var b strings.Builder

	// If we're selecting a saved remote host, show that UI
	if m.selectingSavedHost {
		return m.renderSavedHostSelection(width)
	}

	// If we're selecting a pool, show pool selection UI
	if m.selectingPool {
		return m.renderPoolSelectionContent(width)
	}

	// Render simple menu items - one line each
	for i, item := range mainMenuItems {
		var line string
		if item.title == "---" {
			// Separator - render danger zone header
			separator := warningStyle.Render("──── Danger Zone ────")
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

// renderSavedHostSelection renders the saved remote host picker
func (m model) renderSavedHostSelection(width int) string {
	var b strings.Builder

	titleLine := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(selectedItemStyle.Render("Remote Backup - Select Host"))
	b.WriteString(titleLine + "\n\n")

	// List saved hosts
	for i, host := range m.savedHosts {
		var line string
		detail := fmt.Sprintf("%s (%s)", host.SSHHost, host.Dataset)
		if i == m.savedHostIndex {
			line = selectedItemStyle.Render(fmt.Sprintf("  ▶ %s", detail))
		} else {
			line = fmt.Sprintf("    %s", detail)
		}
		centered := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(line)
		b.WriteString(centered + "\n")
	}

	// "+ Add new host" option
	addIdx := len(m.savedHosts)
	var addLine string
	if m.savedHostIndex == addIdx {
		addLine = selectedItemStyle.Render("  ▶ + Add new host...")
	} else {
		addLine = "    + Add new host..."
	}
	addCentered := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(addLine)
	b.WriteString(addCentered + "\n\n")

	// Hint
	hint := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(subtitleStyle.Render("enter select • d delete • esc cancel"))
	b.WriteString(hint + "\n")

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
				"DESTRUCTIVE OPERATION WARNING\n\n" +
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

	var titleText, promptText, hintText string

	if m.operation == "remote-backup" || m.operation == "push-backup" {
		if m.operation == "push-backup" {
			titleText = "Push Backup to Remote"
		} else {
			titleText = "Pull Remote Backup"
		}
		if m.remoteInputPhase == 0 {
			promptText = "Enter the remote host (SSH connection string):"
			hintText = "Example: root@myserver or user@192.168.1.100"
		} else if m.operation == "push-backup" {
			promptText = fmt.Sprintf("Enter the remote destination pool (host: %s):", m.remoteHost)
			hintText = "Example: NIXBACKUPS (the pool on the remote server)"
		} else {
			promptText = fmt.Sprintf("Enter the remote dataset to back up (host: %s):", m.remoteHost)
			hintText = "Example: NIXROOT/home or NIXROOT (for all datasets)"
		}
	} else {
		titleText = "Prepare Backup Device"
		if m.preparePhase == 0 {
			promptText = "Enter the device path to use for backup:"
			hintText = "Example: /dev/sda"
		} else {
			promptText = fmt.Sprintf("Enter the pool name (device: %s):", m.devicePath)
			hintText = "Default: NIXBACKUPS (press Enter to use default)"
		}
	}

	contentTitle := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(selectedItemStyle.Render(titleText))
	b.WriteString(contentTitle + "\n\n")

	prompt := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(infoStyle.Render(promptText))
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
		Render(subtitleStyle.Render(hintText))
	b.WriteString(hint + "\n")

	return b.String()
}

// renderPasswordContent renders the password input form
func (m model) renderPasswordContent(width int) string {
	var b strings.Builder

	var title, promptText, hintText string
	poolInfo := m.destPool
	if poolInfo == "" {
		poolInfo = "the backup pool"
	}

	if m.operation == "prepare" {
		title = "Set Encryption Password"
		promptText = fmt.Sprintf("Create an encryption password for %s:", poolInfo)
		hintText = "IMPORTANT: Remember this password! You will need it to access your backups."
	} else {
		title = "Encryption Password"
		promptText = fmt.Sprintf("Enter the encryption password for %s:", poolInfo)
		hintText = "The password will be used to unlock the encrypted ZFS pool"
	}

	contentTitle := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(selectedItemStyle.Render(title))
	b.WriteString(contentTitle + "\n\n")

	prompt := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(infoStyle.Render(promptText))
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
		Render(subtitleStyle.Render(hintText))
	b.WriteString(hint + "\n")

	return b.String()
}

// renderRunningContent renders the operation progress view with per-dataset dot grid
func (m model) renderRunningContent(width int) string {
	var b strings.Builder

	contentTitle := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(selectedItemStyle.Render("Backup in Progress"))
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

	// Progress bar and stage info
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

		// Per-dataset progress grid (shown during sync stage)
		if len(m.datasetProgress) > 0 {
			b.WriteString(m.renderDatasetGrid(width))
		}
	} else {
		initLine := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(infoStyle.Render("Initializing..."))
		b.WriteString(initLine + "\n\n")
	}

	return b.String()
}

// renderDatasetGrid renders per-dataset progress bars and a snapshot dot matrix
// for the currently syncing dataset
func (m model) renderDatasetGrid(width int) string {
	var b strings.Builder

	// Styles
	dotPending := lipgloss.NewStyle().Foreground(colorHighlight3) // Gray
	dotSyncing := lipgloss.NewStyle().Foreground(colorHighlight1) // Orange
	dotDone := lipgloss.NewStyle().Foreground(colorHighlight2)    // Blue
	dotError := lipgloss.NewStyle().Foreground(colorAlert)        // Red
	labelDim := lipgloss.NewStyle().Foreground(colorHighlight3)
	labelActive := lipgloss.NewStyle().Foreground(colorHighlight1).Bold(true)
	labelDone := lipgloss.NewStyle().Foreground(colorHighlight2)
	labelError := lipgloss.NewStyle().Foreground(colorAlert)

	// Count completed datasets for the dataset-level progress bar
	doneCount := 0
	errorCount := 0
	for _, ds := range m.datasetProgress {
		switch ds.Status {
		case DatasetDone:
			doneCount++
		case DatasetError, DatasetSkipped:
			doneCount++ // Count towards progress (attempted)
			errorCount++
		}
	}

	// Dataset-level progress bar
	dsPercent := float64(0)
	if len(m.datasetProgress) > 0 {
		dsPercent = float64(doneCount) / float64(len(m.datasetProgress))
	}
	dsProgressBar := progress.New(
		progress.WithGradient(string(colorHighlight4), string(colorHighlight2)),
	)
	barWidth := width - 20
	if barWidth < 30 {
		barWidth = 30
	}
	if barWidth > 60 {
		barWidth = 60
	}
	dsProgressBar.Width = barWidth

	syncHeader := fmt.Sprintf("Datasets %d/%d", doneCount, len(m.datasetProgress))
	if errorCount > 0 {
		syncHeader += fmt.Sprintf(" (%d failed)", errorCount)
	}
	headerLine := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(infoStyle.Render(syncHeader))
	b.WriteString(headerLine + "\n")

	dsBar := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(dsProgressBar.ViewAs(dsPercent))
	b.WriteString(dsBar + "\n\n")

	// Find the longest dataset name for alignment
	maxNameLen := 0
	for _, ds := range m.datasetProgress {
		if len(ds.Name) > maxNameLen {
			maxNameLen = len(ds.Name)
		}
	}

	// Render each dataset as a compact status line
	for i, ds := range m.datasetProgress {
		var statusIcon string
		var label string

		switch ds.Status {
		case DatasetPending:
			statusIcon = dotPending.Render("○")
			label = labelDim.Render(ds.Name)
		case DatasetSyncing:
			statusIcon = dotSyncing.Render(m.spinner.View())
			label = labelActive.Render(ds.Name)
		case DatasetDone:
			statusIcon = dotDone.Render("●")
			label = labelDone.Render(ds.Name)
		case DatasetError:
			statusIcon = dotError.Render("●")
			label = labelError.Render(ds.Name)
		case DatasetSkipped:
			statusIcon = dotError.Render("○")
			label = labelError.Render(ds.Name)
		}

		row := fmt.Sprintf("  %s %s", statusIcon, label)

		line := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(row)
		b.WriteString(line + "\n")

		// Show snapshot dot matrix for the currently syncing dataset
		if i == m.currentDataset && len(ds.Snapshots) > 0 {
			b.WriteString(m.renderSnapshotMatrix(ds.Snapshots, width))
		}
	}

	b.WriteString("\n")

	// Legend
	legend := fmt.Sprintf("%s pending  %s syncing  %s done  %s error",
		dotPending.Render("○"),
		dotSyncing.Render("●"),
		dotDone.Render("●"),
		dotError.Render("●"),
	)
	legendLine := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(labelDim.Render(legend))
	b.WriteString(legendLine + "\n")

	return b.String()
}

// renderSnapshotMatrix renders a rectangular grid of dots representing snapshots
// for a single dataset. Each dot is colored by its sync status.
func (m model) renderSnapshotMatrix(snapshots []SnapshotDot, width int) string {
	var b strings.Builder

	dotPending := lipgloss.NewStyle().Foreground(colorHighlight3) // Gray
	dotSyncing := lipgloss.NewStyle().Foreground(colorHighlight1) // Orange
	dotDone := lipgloss.NewStyle().Foreground(colorHighlight2)    // Blue
	dotError := lipgloss.NewStyle().Foreground(colorAlert)        // Red

	// Calculate grid dimensions
	// Each dot takes 2 chars (dot + space), leave margins for centering
	maxDotsPerRow := (width - 12) / 2
	if maxDotsPerRow < 10 {
		maxDotsPerRow = 10
	}
	if maxDotsPerRow > 40 {
		maxDotsPerRow = 40
	}

	// Build rows of dots
	var row strings.Builder
	for i, snap := range snapshots {
		if i > 0 && i%maxDotsPerRow == 0 {
			// Flush current row
			line := lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Render(row.String())
			b.WriteString(line + "\n")
			row.Reset()
		}

		var dot string
		switch snap.Status {
		case SnapPending:
			dot = dotPending.Render("○")
		case SnapSyncing:
			dot = dotSyncing.Render("●")
		case SnapDone:
			dot = dotDone.Render("●")
		case SnapError:
			dot = dotError.Render("●")
		}

		if i%maxDotsPerRow > 0 {
			row.WriteString(" ")
		}
		row.WriteString(dot)
	}

	// Flush last row
	if row.Len() > 0 {
		line := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(row.String())
		b.WriteString(line + "\n")
	}

	// Show snapshot count
	countText := fmt.Sprintf("%d snapshots", len(snapshots))
	countLine := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(lipgloss.NewStyle().Foreground(colorHighlight3).Render(countText))
	b.WriteString(countLine + "\n")

	return b.String()
}

// renderDatasetReport renders the final dataset sync report with timings, sizes,
// errors, and snapshot dot grids. Shown in the result view after backup completes.
func (m model) renderDatasetReport(width int) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", width) + "\n")
	b.WriteString("  DATASET SYNC REPORT\n")
	b.WriteString(strings.Repeat("─", width) + "\n\n")

	doneCount := 0
	errorCount := 0
	var totalDuration time.Duration
	for _, ds := range m.datasetProgress {
		totalDuration += ds.Duration
		switch ds.Status {
		case DatasetDone:
			doneCount++
		case DatasetError, DatasetSkipped:
			errorCount++
		}
	}

	b.WriteString(fmt.Sprintf("  Total datasets: %d  |  Synced: %d  |  Failed: %d\n", len(m.datasetProgress), doneCount, errorCount))
	b.WriteString(fmt.Sprintf("  Total sync time: %s\n\n", formatDuration(totalDuration)))

	for _, ds := range m.datasetProgress {
		// Status indicator
		var statusStr string
		switch ds.Status {
		case DatasetDone:
			statusStr = "[OK]"
		case DatasetError:
			statusStr = "[FAIL]"
		case DatasetSkipped:
			statusStr = "[SKIP]"
		case DatasetPending:
			statusStr = "[--]"
		default:
			statusStr = "[??]"
		}

		// Dataset header line: status name (size) duration
		line := fmt.Sprintf("  %s %s", statusStr, ds.Name)
		if ds.Size != "" && ds.Size != "?" {
			line += fmt.Sprintf(" (%s)", ds.Size)
		}
		if ds.Duration > 0 {
			line += fmt.Sprintf("  %s", formatDuration(ds.Duration))
		}
		line += fmt.Sprintf("  %d snapshots", len(ds.Snapshots))
		b.WriteString(line + "\n")

		// Show snapshot dot matrix
		if len(ds.Snapshots) > 0 {
			b.WriteString(m.renderSnapshotMatrixPlain(ds.Snapshots, width))
		}

		// Show error details for failed datasets
		if ds.ErrorMsg != "" {
			b.WriteString(fmt.Sprintf("    Error: %s\n", ds.ErrorMsg))
		}

		b.WriteString("\n")
	}

	// Legend
	b.WriteString(fmt.Sprintf("  Legend: ○ pending  ● synced  ● error\n"))

	return b.String()
}

// renderSnapshotMatrixPlain renders snapshot dots without lipgloss styling (for the
// scrollable result viewport which doesn't support ANSI well in all terminals).
// Uses plain Unicode characters.
func (m model) renderSnapshotMatrixPlain(snapshots []SnapshotDot, width int) string {
	var b strings.Builder

	maxDotsPerRow := (width - 8) / 2
	if maxDotsPerRow < 10 {
		maxDotsPerRow = 10
	}
	if maxDotsPerRow > 40 {
		maxDotsPerRow = 40
	}

	var row strings.Builder
	row.WriteString("    ")
	for i, snap := range snapshots {
		if i > 0 && i%maxDotsPerRow == 0 {
			b.WriteString(row.String() + "\n")
			row.Reset()
			row.WriteString("    ")
		}
		if i%maxDotsPerRow > 0 {
			row.WriteString(" ")
		}
		switch snap.Status {
		case SnapDone:
			row.WriteString("●")
		case SnapError:
			row.WriteString("✕")
		default:
			row.WriteString("○")
		}
	}
	if row.Len() > 4 {
		b.WriteString(row.String() + "\n")
	}

	return b.String()
}

// renderResumeContent renders the resume prompt
// renderReportsContent renders the report browser view
func (m model) renderReportsContent(width int) string {
	var b strings.Builder

	if m.reportViewing {
		// Show the report content in a scrollable viewport
		contentTitle := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(selectedItemStyle.Render("Backup Report"))
		b.WriteString(contentTitle + "\n\n")
		b.WriteString(m.reportViewport.View())
		return b.String()
	}

	// Report list view
	contentTitle := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(selectedItemStyle.Render("Backup Reports"))
	b.WriteString(contentTitle + "\n\n")

	if len(m.reportFiles) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(infoStyle.Render("No reports found. Reports are generated after each backup run."))
		b.WriteString(emptyMsg + "\n\n")

		pathMsg := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(subtitleStyle.Render("Reports are saved to ~/.local/share/zfs-backup/reports/"))
		b.WriteString(pathMsg + "\n")
		return b.String()
	}

	countMsg := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(infoStyle.Render(fmt.Sprintf("%d report(s) found", len(m.reportFiles))))
	b.WriteString(countMsg + "\n\n")

	// Show report list
	for i, r := range m.reportFiles {
		cursor := "  "
		style := subtitleStyle
		if i == m.reportIndex {
			cursor = "> "
			style = selectedItemStyle
		}

		// Format: name + date + PDF indicator
		line := cursor + r.Name
		if !r.ModTime.IsZero() {
			line += "  " + r.ModTime.Format("02 Jan 15:04")
		}
		if r.PdfPath != "" {
			line += "  [PDF]"
		}

		row := lipgloss.NewStyle().
			Width(width).
			Render(style.Render(line))
		b.WriteString(row + "\n")
	}

	return b.String()
}

func (m model) renderResumeContent(width int) string {
	var b strings.Builder

	contentTitle := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(selectedItemStyle.Render("Resume Previous Backup"))
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

// renderResultContent renders the operation result in a scrollable viewport
func (m model) renderResultContent(width int) string {
	var b strings.Builder

	if m.err != nil {
		// Error display - simple centered message
		contentTitle := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(errorStyle.Render("Operation Failed"))
		b.WriteString(contentTitle + "\n\n")

		errMsg := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(errorStyle.Render(m.err.Error()))
		b.WriteString(errMsg + "\n\n")

		hint := subtitleStyle.Render("Press enter/esc/q to return to menu")
		b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(hint))
	} else {
		// Success display - scrollable viewport for long reports
		contentTitle := lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(statusStyle.Render("Operation Completed"))
		b.WriteString(contentTitle + "\n\n")

		if m.resultReady {
			// Viewport with scrollable content
			b.WriteString(m.resultViewport.View())
			b.WriteString("\n")

			// Footer with scroll info
			scrollInfo := subtitleStyle.Render(fmt.Sprintf(
				"Scroll: j/k or arrows | %d%% | enter/esc/q to return",
				int(m.resultViewport.ScrollPercent()*100)))
			b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(scrollInfo))
		} else {
			msg := lipgloss.NewStyle().
				Width(width).
				Align(lipgloss.Center).
				Render(statusStyle.Render(m.message))
			b.WriteString(msg + "\n")
		}
	}

	return b.String()
}

// renderHelpContent renders the help screen
func (m model) renderHelpContent(width int) string {
	help := `DESCRIPTION
  A beautiful TUI for managing ZFS backups.

OPERATIONS
  Backup ZFS (incremental)
     Performs an incremental backup using syncoid.

  Pull Remote Backup
     Pulls incremental backup from a remote host via SSH.
     Requires SSH key-based auth to the remote host.
     Backups are namespaced by hostname on the backup drive.

  Push Backup to Remote
     Pushes local ZFS snapshots to a remote backup server via SSH.
     Datasets are namespaced by local hostname on the remote pool.

  Force Backup ZFS (destructive)
     Forces a complete backup by deleting previous snapshots.

  Restore Files
     Browse snapshots and restore files to any location.

  Show zpool info
     Displays detailed pool structure, status and health.

  Pool Maintenance
     Start, stop, or monitor scrub operations for data integrity.

  Manage Quotas
     View and edit dataset quotas. Units: T, G, M, K.

  Prepare Backup Device
     Creates an encrypted ZFS pool on a new external drive.

  Unmount Backup Disk
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

// loadZpoolInfo returns a command that fetches zpool info asynchronously
func (m model) loadZpoolInfo() tea.Cmd {
	pool := m.zpoolInfoPool
	return func() tea.Msg {
		var info strings.Builder

		// Header with pool name
		info.WriteString(fmt.Sprintf("Pool: %s\n", pool))
		info.WriteString(strings.Repeat("=", 70) + "\n\n")

		// Get zpool status for this specific pool
		info.WriteString("STATUS\n")
		info.WriteString(strings.Repeat("-", 70) + "\n")
		statusCmd := exec.Command("zpool", "status", pool)
		statusOutput, err := statusCmd.Output()
		if err != nil {
			info.WriteString("Error: " + err.Error() + "\n")
		} else {
			info.WriteString(string(statusOutput))
		}

		// Get zpool list for this specific pool
		info.WriteString("\nUSAGE\n")
		info.WriteString(strings.Repeat("-", 70) + "\n")
		listCmd := exec.Command("zpool", "list", "-v", pool)
		listOutput, err := listCmd.Output()
		if err != nil {
			info.WriteString("Error: " + err.Error() + "\n")
		} else {
			info.WriteString(string(listOutput))
		}

		// Get datasets for this pool
		info.WriteString("\nDATASETS\n")
		info.WriteString(strings.Repeat("-", 70) + "\n")
		zfsCmd := exec.Command("zfs", "list", "-r", "-o", "name,used,avail,refer,mountpoint", pool)
		zfsOutput, err := zfsCmd.Output()
		if err != nil {
			info.WriteString("Error: " + err.Error() + "\n")
		} else {
			info.WriteString(string(zfsOutput))
		}

		// Get snapshots for this pool
		info.WriteString("\nSNAPSHOTS\n")
		info.WriteString(strings.Repeat("-", 70) + "\n")
		snapCmd := exec.Command("zfs", "list", "-r", "-t", "snapshot", "-o", "name,used,refer,creation", pool)
		snapOutput, err := snapCmd.Output()
		if err != nil {
			info.WriteString("No snapshots or error: " + err.Error() + "\n")
		} else {
			output := strings.TrimSpace(string(snapOutput))
			if output == "" {
				info.WriteString("No snapshots found.\n")
			} else {
				info.WriteString(output + "\n")
			}
		}

		return zpoolInfoLoadedMsg{content: info.String()}
	}
}

// unlockAndLoadZpoolInfo unlocks the pool and then loads the info
func (m model) unlockAndLoadZpoolInfo() tea.Cmd {
	pool := m.zpoolInfoPool
	password := m.password
	return func() tea.Msg {
		// Try to unlock the pool
		cmd := exec.Command("zfs", "load-key", pool)
		cmd.Stdin = strings.NewReader(password + "\n")
		if err := cmd.Run(); err != nil {
			return zpoolInfoLoadedMsg{err: fmt.Errorf("failed to unlock pool: %w", err)}
		}

		// Now fetch the info
		var info strings.Builder

		// Header with pool name
		info.WriteString(fmt.Sprintf("Pool: %s\n", pool))
		info.WriteString(strings.Repeat("=", 70) + "\n\n")

		// Get zpool status for this specific pool
		info.WriteString("STATUS\n")
		info.WriteString(strings.Repeat("-", 70) + "\n")
		statusCmd := exec.Command("zpool", "status", pool)
		statusOutput, err := statusCmd.Output()
		if err != nil {
			info.WriteString("Error: " + err.Error() + "\n")
		} else {
			info.WriteString(string(statusOutput))
		}

		// Get zpool list for this specific pool
		info.WriteString("\nUSAGE\n")
		info.WriteString(strings.Repeat("-", 70) + "\n")
		listCmd := exec.Command("zpool", "list", "-v", pool)
		listOutput, err := listCmd.Output()
		if err != nil {
			info.WriteString("Error: " + err.Error() + "\n")
		} else {
			info.WriteString(string(listOutput))
		}

		// Get datasets for this pool
		info.WriteString("\nDATASETS\n")
		info.WriteString(strings.Repeat("-", 70) + "\n")
		zfsCmd := exec.Command("zfs", "list", "-r", "-o", "name,used,avail,refer,mountpoint", pool)
		zfsOutput, err := zfsCmd.Output()
		if err != nil {
			info.WriteString("Error: " + err.Error() + "\n")
		} else {
			info.WriteString(string(zfsOutput))
		}

		// Get snapshots for this pool
		info.WriteString("\nSNAPSHOTS\n")
		info.WriteString(strings.Repeat("-", 70) + "\n")
		snapCmd := exec.Command("zfs", "list", "-r", "-t", "snapshot", "-o", "name,used,refer,creation", pool)
		snapOutput, err := snapCmd.Output()
		if err != nil {
			info.WriteString("No snapshots or error: " + err.Error() + "\n")
		} else {
			output := strings.TrimSpace(string(snapOutput))
			if output == "" {
				info.WriteString("No snapshots found.\n")
			} else {
				info.WriteString(output + "\n")
			}
		}

		return zpoolInfoLoadedMsg{content: info.String()}
	}
}

// renderZpoolInfoContent renders detailed zpool information in a scrollable viewport
func (m model) renderZpoolInfoContent(width int) string {
	if !m.zpoolInfoReady {
		// Show loading spinner
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(m.spinner.View() + " Loading pool information...")
	}

	var b strings.Builder

	// Title
	title := selectedItemStyle.Render(fmt.Sprintf("Pool Information: %s", m.zpoolInfoPool))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(title))
	b.WriteString("\n\n")

	// Viewport with content
	b.WriteString(m.zpoolViewport.View())
	b.WriteString("\n")

	// Footer with scroll info
	scrollInfo := subtitleStyle.Render(fmt.Sprintf(
		"Scroll: j/k or arrows | %d%% | esc/q to return",
		int(m.zpoolViewport.ScrollPercent()*100)))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(scrollInfo))

	return b.String()
}

// preparePoolAccess imports pool if needed and checks if unlock is required
func (m model) preparePoolAccess(pool string) tea.Cmd {
	return func() tea.Msg {
		// Check if pool is imported
		imported, err := isPoolImported(pool)
		if err != nil {
			return poolReadyMsg{pool: pool, err: fmt.Errorf("failed to check pool status: %w", err)}
		}

		if !imported {
			// Try to import the pool
			cmd := exec.Command("sudo", "zpool", "import", pool)
			if err := cmd.Run(); err != nil {
				// Try without sudo
				cmd = exec.Command("zpool", "import", pool)
				if err := cmd.Run(); err != nil {
					return poolReadyMsg{pool: pool, err: fmt.Errorf("failed to import pool: %w", err)}
				}
			}
		}

		return poolReadyMsg{pool: pool}
	}
}

// unlockAndLoadMaintenance unlocks the pool and loads maintenance status
func (m model) unlockAndLoadMaintenance() tea.Cmd {
	pool := m.maintenancePool
	password := m.password
	return func() tea.Msg {
		// Try to unlock the pool
		cmd := exec.Command("zfs", "load-key", pool)
		cmd.Stdin = strings.NewReader(password + "\n")
		if err := cmd.Run(); err != nil {
			return maintenanceStatusMsg{err: fmt.Errorf("failed to unlock pool: %w", err)}
		}

		// Now load the status
		return loadMaintenanceStatusSync(pool)
	}
}

// unlockAndLoadQuotas unlocks the pool and loads quota data
func (m model) unlockAndLoadQuotas() tea.Cmd {
	pool := m.quotaPool
	password := m.password
	return func() tea.Msg {
		cmd := exec.Command("zfs", "load-key", pool)
		cmd.Stdin = strings.NewReader(password + "\n")
		if err := cmd.Run(); err != nil {
			return quotaLoadedMsg{err: fmt.Errorf("failed to unlock pool: %w", err)}
		}
		// Reuse the loadQuotaData command's inner logic
		return loadQuotaData(pool)().(tea.Msg)
	}
}

// loadMaintenanceStatus returns a command that fetches maintenance status
func (m model) loadMaintenanceStatus() tea.Cmd {
	pool := m.maintenancePool
	return func() tea.Msg {
		return loadMaintenanceStatusSync(pool)
	}
}

// loadMaintenanceStatusSync synchronously loads maintenance status
func loadMaintenanceStatusSync(pool string) maintenanceStatusMsg {
	var info strings.Builder

	info.WriteString(fmt.Sprintf("Pool: %s\n", pool))
	info.WriteString(strings.Repeat("=", 70) + "\n\n")

	// Get pool status which includes scrub/resilver info
	info.WriteString("POOL STATUS\n")
	info.WriteString(strings.Repeat("-", 70) + "\n")
	statusCmd := exec.Command("zpool", "status", pool)
	statusOutput, err := statusCmd.Output()
	if err != nil {
		info.WriteString("Error: " + err.Error() + "\n")
	} else {
		info.WriteString(string(statusOutput))
	}

	// Get pool health
	info.WriteString("\nPOOL HEALTH\n")
	info.WriteString(strings.Repeat("-", 70) + "\n")
	healthCmd := exec.Command("zpool", "list", "-H", "-o", "name,health,size,alloc,free,frag,cap", pool)
	healthOutput, err := healthCmd.Output()
	if err != nil {
		info.WriteString("Error: " + err.Error() + "\n")
	} else {
		// Format the health info nicely
		fields := strings.Fields(strings.TrimSpace(string(healthOutput)))
		if len(fields) >= 7 {
			info.WriteString(fmt.Sprintf("  Health:        %s\n", fields[1]))
			info.WriteString(fmt.Sprintf("  Size:          %s\n", fields[2]))
			info.WriteString(fmt.Sprintf("  Allocated:     %s\n", fields[3]))
			info.WriteString(fmt.Sprintf("  Free:          %s\n", fields[4]))
			info.WriteString(fmt.Sprintf("  Fragmentation: %s\n", fields[5]))
			info.WriteString(fmt.Sprintf("  Capacity:      %s\n", fields[6]))
		} else {
			info.WriteString(string(healthOutput))
		}
	}

	// Check for ongoing scrub/resilver
	isScubbing := false
	var progress float64
	statusStr := string(statusOutput)
	if strings.Contains(statusStr, "scrub in progress") || strings.Contains(statusStr, "resilver in progress") {
		isScubbing = true
		// Try to extract progress percentage
		lines := strings.Split(statusStr, "\n")
		for _, line := range lines {
			if strings.Contains(line, "% done") {
				// Extract percentage
				parts := strings.Fields(line)
				for i, p := range parts {
					if strings.HasSuffix(p, "%") {
						fmt.Sscanf(parts[i], "%f%%", &progress)
						break
					}
				}
			}
		}
	}

	info.WriteString("\n\nACTIONS\n")
	info.WriteString(strings.Repeat("-", 70) + "\n")
	if isScubbing {
		info.WriteString("  [x] Stop Scrub - Cancel the current scrub operation\n")
		info.WriteString("  [r] Refresh    - Update the status display\n")
	} else {
		info.WriteString("  [s] Start Scrub - Begin data integrity verification\n")
		info.WriteString("  [r] Refresh     - Update the status display\n")
	}
	info.WriteString("  [q] Return      - Go back to main menu\n")

	return maintenanceStatusMsg{
		content:    info.String(),
		isScubbing: isScubbing,
		progress:   progress,
	}
}

// startScrub starts a scrub operation on the maintenance pool
func (m model) startScrub() tea.Cmd {
	pool := m.maintenancePool
	return func() tea.Msg {
		cmd := exec.Command("sudo", "zpool", "scrub", pool)
		if err := cmd.Run(); err != nil {
			// Try without sudo
			cmd = exec.Command("zpool", "scrub", pool)
			if err := cmd.Run(); err != nil {
				return scrubActionMsg{action: "start", err: fmt.Errorf("failed to start scrub: %w", err)}
			}
		}
		return scrubActionMsg{action: "start", success: true, message: "Scrub started"}
	}
}

// stopScrub stops a scrub operation on the maintenance pool
func (m model) stopScrub() tea.Cmd {
	pool := m.maintenancePool
	return func() tea.Msg {
		cmd := exec.Command("sudo", "zpool", "scrub", "-s", pool)
		if err := cmd.Run(); err != nil {
			// Try without sudo
			cmd = exec.Command("zpool", "scrub", "-s", pool)
			if err := cmd.Run(); err != nil {
				return scrubActionMsg{action: "stop", err: fmt.Errorf("failed to stop scrub: %w", err)}
			}
		}
		return scrubActionMsg{action: "stop", success: true, message: "Scrub stopped"}
	}
}

// renderMaintenanceContent renders the pool maintenance view
func (m model) renderMaintenanceContent(width int) string {
	if !m.maintenanceReady {
		// Show loading spinner
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center).
			Render(m.spinner.View() + " Loading maintenance status...")
	}

	var b strings.Builder

	// Title
	title := selectedItemStyle.Render(fmt.Sprintf("Pool Maintenance: %s", m.maintenancePool))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(title))
	b.WriteString("\n\n")

	// Viewport with content
	b.WriteString(m.zpoolViewport.View())
	b.WriteString("\n")

	// Footer with scroll info and hotkeys
	scrollInfo := subtitleStyle.Render(fmt.Sprintf(
		"Scroll: j/k | %d%% | s=scrub x=stop r=refresh q=return",
		int(m.zpoolViewport.ScrollPercent()*100)))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(scrollInfo))

	return b.String()
}

// handleDatasetCreateForm handles keyboard input for the dataset creation form
func (m model) handleDatasetCreateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.datasetCreating = false
		return m, nil
	case "enter":
		// Save current field value and advance
		val := m.quotaInput.Value()
		switch m.datasetFormField {
		case 0:
			m.datasetForm.Name = val
		case 1:
			if val == "" {
				val = "filesystem"
			}
			m.datasetForm.Type = val
		case 2:
			m.datasetForm.Quota = val
		case 3:
			if val == "" {
				val = "128K"
			}
			m.datasetForm.RecordSize = val
		case 4:
			if val == "" {
				val = "zstd"
			}
			m.datasetForm.Compression = val
		case 5:
			if val == "" {
				val = "off"
			}
			m.datasetForm.Atime = val
		case 6:
			m.datasetForm.VolumeSize = val
		}

		m.datasetFormField++

		// Skip volume size field if type is filesystem
		if m.datasetFormField == 6 && m.datasetForm.Type != "volume" {
			m.datasetFormField++
		}

		// If we've passed the last field, create the dataset
		if m.datasetFormField >= len(datasetCreateFormFields) {
			if m.datasetForm.Name == "" {
				m.datasetCreating = false
				return m, nil
			}
			m.datasetCreating = false
			return m, createDataset(m.quotaPool, m.datasetForm)
		}

		// Set up next field
		m.quotaInput.SetValue(m.getCreateFormDefault())
		m.quotaInput.Placeholder = m.getCreateFormPlaceholder()
		return m, nil
	case "tab":
		// Same as enter - advance to next field
		return m.handleDatasetCreateForm(tea.KeyMsg{Type: tea.KeyEnter})
	default:
		var cmd tea.Cmd
		m.quotaInput, cmd = m.quotaInput.Update(msg)
		return m, cmd
	}
}

// getCreateFormDefault returns the default value for the current create form field
func (m model) getCreateFormDefault() string {
	switch m.datasetFormField {
	case 1:
		return m.datasetForm.Type
	case 3:
		return m.datasetForm.RecordSize
	case 4:
		return m.datasetForm.Compression
	case 5:
		return m.datasetForm.Atime
	default:
		return ""
	}
}

// getCreateFormPlaceholder returns the placeholder for the current create form field
func (m model) getCreateFormPlaceholder() string {
	switch m.datasetFormField {
	case 0:
		return "dataset-name"
	case 1:
		return "filesystem"
	case 2:
		return "e.g., 500G, 2T, or leave empty"
	case 3:
		return "128K"
	case 4:
		return "zstd"
	case 5:
		return "off"
	case 6:
		return "e.g., 10G (required for volumes)"
	default:
		return ""
	}
}

// renderQuotaContent renders the quota management table
func (m model) renderQuotaContent(width int) string {
	var b strings.Builder

	// Show create form overlay
	if m.datasetCreating {
		return m.renderDatasetCreateForm(width)
	}

	// Show delete confirmation overlay
	if m.datasetDeleting && m.quotaIndex < len(m.quotaDatasets) {
		return m.renderDatasetDeleteConfirm(width)
	}

	// Title
	title := selectedItemStyle.Render(fmt.Sprintf("Dataset Manager: %s", m.quotaPool))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(title))
	b.WriteString("\n")

	// Pool info
	poolInfo := infoStyle.Render(fmt.Sprintf("Pool Size: %s | Free: %s", m.quotaPoolSize, m.quotaPoolFree))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(poolInfo))
	b.WriteString("\n\n")

	// Table header
	header := fmt.Sprintf("  %-30s %-12s %-10s %-10s %-10s",
		"DATASET", "TYPE", "QUOTA", "USED", "AVAIL")
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).
		Render(labelStyle.Render(header)))
	b.WriteString("\n")

	// Separator
	sep := strings.Repeat("─", 76)
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).
		Render(interstitialStyle.Render(sep)))
	b.WriteString("\n")

	// Dataset rows
	for i, ds := range m.quotaDatasets {
		name := ds.Name
		if len(name) > 28 {
			name = "..." + name[len(name)-25:]
		}

		quotaDisplay := ds.Quota
		if !ds.SupportsQuota {
			quotaDisplay = "-"
		}

		var row string
		if i == m.quotaIndex && m.quotaEditing {
			row = fmt.Sprintf("▶ %-28s %-12s ", name, ds.Type)
			row += m.quotaInput.View()
		} else if i == m.quotaIndex {
			marker := "▶"
			if !ds.SupportsQuota {
				marker = "  "
			}
			row = selectedItemStyle.Render(fmt.Sprintf("%s %-28s %-12s %-10s %-10s %-10s",
				marker, name, ds.Type, quotaDisplay, ds.Used, ds.Available))
		} else {
			row = fmt.Sprintf("  %-28s %-12s %-10s %-10s %-10s",
				name, ds.Type, quotaDisplay, ds.Used, ds.Available)
			if !ds.SupportsQuota {
				row = subtitleStyle.Render(row)
			}
		}

		centered := lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(row)
		b.WriteString(centered + "\n")
	}

	// Help text
	b.WriteString("\n")
	helpSep := strings.Repeat("─", 76)
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).
		Render(interstitialStyle.Render(helpSep)))
	b.WriteString("\n")

	helpLines := []string{
		"QUOTA UNITS: Use T (terabytes), G (gigabytes), M (megabytes), K (kilobytes)",
		"Examples: 500G = 500 gigabytes, 2T = 2 terabytes, 100M = 100 megabytes",
		"e/enter edit quota • n remove quota • c create dataset • d delete • q return",
	}
	for _, line := range helpLines {
		b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).
			Render(subtitleStyle.Render(line)))
		b.WriteString("\n")
	}

	return b.String()
}

// renderDatasetCreateForm renders the dataset creation form
func (m model) renderDatasetCreateForm(width int) string {
	var b strings.Builder

	title := selectedItemStyle.Render("Create New Dataset")
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(title))
	b.WriteString("\n")

	poolInfo := infoStyle.Render(fmt.Sprintf("Parent pool: %s | Free: %s", m.quotaPool, m.quotaPoolFree))
	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(poolInfo))
	b.WriteString("\n\n")

	// Show completed fields
	formValues := []string{
		m.datasetForm.Name,
		m.datasetForm.Type,
		m.datasetForm.Quota,
		m.datasetForm.RecordSize,
		m.datasetForm.Compression,
		m.datasetForm.Atime,
		m.datasetForm.VolumeSize,
	}

	for i, label := range datasetCreateFormFields {
		// Skip volume size if filesystem
		if i == 6 && m.datasetForm.Type != "volume" {
			continue
		}

		var line string
		if i < m.datasetFormField {
			// Completed field
			val := formValues[i]
			if val == "" {
				val = "(default)"
			}
			line = infoStyle.Render(fmt.Sprintf("  %-28s: %s", label, val))
		} else if i == m.datasetFormField {
			// Active field - show input
			line = selectedItemStyle.Render(fmt.Sprintf("▶ %-28s: ", label)) + m.quotaInput.View()
		} else {
			// Future field
			line = subtitleStyle.Render(fmt.Sprintf("  %-28s: ...", label))
		}

		centered := lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(line)
		b.WriteString(centered + "\n")
	}

	b.WriteString("\n")

	// Help
	helpLines := []string{
		"enter/tab next field • esc cancel",
		"",
		"RECORD SIZE: 4K (databases), 16K (VMs), 128K (general), 1M (large files)",
		"COMPRESSION: zstd (best), lz4 (fast), gzip, off",
		"TYPE: filesystem (normal) or volume (block device for VMs/databases)",
	}
	for _, line := range helpLines {
		b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).
			Render(subtitleStyle.Render(line)))
		b.WriteString("\n")
	}

	return b.String()
}

// renderDatasetDeleteConfirm renders the delete confirmation
func (m model) renderDatasetDeleteConfirm(width int) string {
	var b strings.Builder

	ds := m.quotaDatasets[m.quotaIndex]

	warning := destructiveWarningStyle.Render(fmt.Sprintf(
		"DELETE DATASET\n\n"+
			"Are you sure you want to destroy:\n\n"+
			"  %s\n\n"+
			"Type: %s | Used: %s\n\n"+
			"This action CANNOT be undone!\n"+
			"All data in this dataset will be permanently lost.",
		ds.Name, ds.Type, ds.Used))

	b.WriteString(lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(warning))
	b.WriteString("\n\n")

	hint := lipgloss.NewStyle().Width(width).Align(lipgloss.Center).
		Render(infoStyle.Render("Press 'y' to confirm deletion or 'n'/esc to cancel"))
	b.WriteString(hint)

	return b.String()
}

// openURL opens a URL in the default browser, running as the real user if under sudo
func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		_ = openFileForUser(url)
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

type zpoolInfoLoadedMsg struct {
	content string
	err     error
}

type poolReadyMsg struct {
	pool string
	err  error
}

type maintenanceStatusMsg struct {
	content    string
	isScubbing bool
	progress   float64
	err        error
}

type scrubActionMsg struct {
	action  string // "start" or "stop"
	success bool
	message string
	err     error
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
	m.datasetProgress = nil
	m.currentDataset = -1
	m.operationStartTime = time.Now()

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
		cmds = append(cmds, runPrepare(m.devicePath, m.destPool, m.password))
	case "unmount":
		cmds = append(cmds, runUnmount(m.destPool))
	case "recover":
		cmds = append(cmds, runRecover(ctx, m.password, m.sourcePool, m.destPool, m.progressChan))
	case "remote-backup":
		cmds = append(cmds, runRemoteBackup(ctx, m.password, m.remoteHost, m.remoteDataset, m.destPool, resumeFrom, m.progressChan))
	case "push-backup":
		cmds = append(cmds, runPushBackup(ctx, m.password, m.sourcePool, m.remoteHost, m.remoteDataset, resumeFrom, m.progressChan))
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
		b.WriteString("║                     NOTICE                            ║\n")
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
	// Handle --version and --help before permission checks
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "--version" || arg == "-v" {
			fmt.Println(appVersion)
			return
		}
		if arg == "--help" || arg == "-h" {
			showCLIHelp()
			return
		}
	}

	// Check permissions first
	if err := checkPermissions(); err != nil {
		fmt.Fprintln(os.Stderr, errorStyle.Render("Error: "+err.Error()))
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
		fmt.Println(statusStyle.Render("Running incremental backup..."))
		runBackupSync()
	case "--force-backup", "-f":
		fmt.Println(warningStyle.Render("Running force backup..."))
		runForceBackupSync()
	case "--unmount", "-u":
		fmt.Println(infoStyle.Render("Unmounting backup disk..."))
		runUnmountSync()
	case "--version", "-v":
		fmt.Println(appVersion)
	case "--help", "-h":
		showCLIHelp()
	default:
		fmt.Fprintf(os.Stderr, errorStyle.Render("Unknown option: %s\n"), arg)
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
	fmt.Println(footerCreditStyle.Render("Made with <3 by Kartoza | Donate! | GitHub"))
	fmt.Println()
}
