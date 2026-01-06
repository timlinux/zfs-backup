package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F1C069")).
			Background(lipgloss.Color("#1F1F1F")).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8E3BD")).
			MarginBottom(1)

	menuStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#F1C069")).
			Padding(1, 2).
			MarginTop(1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7DCE82")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFB86C")).
			Bold(true)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8BE9FD"))

	reportBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("#F1C069")).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F1C069")).
				Bold(true)
)

type sessionState int

const (
	stateMenu sessionState = iota
	stateConfirm
	stateInput
	statePassword
	stateRunning
	stateResult
	stateHelp
)

type menuItem struct {
	title       string
	description string
	icon        string
}

func (i menuItem) Title() string       { return i.icon + " " + i.title }
func (i menuItem) Description() string { return i.description }
func (i menuItem) FilterValue() string { return i.title }

type model struct {
	state         sessionState
	list          list.Model
	spinner       spinner.Model
	input         textinput.Model
	passwordInput textinput.Model
	operation     string
	message       string
	err           error
	width         int
	height        int
	confirmMsg    string
	confirmYes    bool
	quitting      bool
	showingHelp   bool
	password      string
	devicePath    string
}

func initialModel() model {
	items := []list.Item{
		menuItem{
			title:       "Backup ZFS (incremental)",
			description: "Run incremental backup of NIXROOT to NIXBACKUPS",
			icon:        "📦",
		},
		menuItem{
			title:       "Force Backup ZFS (destructive)",
			description: "Force backup - deletes old snapshots on backup disk",
			icon:        "🔥",
		},
		menuItem{
			title:       "Prepare Backup Device",
			description: "Create encrypted ZFS pool on new backup device",
			icon:        "🔧",
		},
		menuItem{
			title:       "Unmount Backup Disk",
			description: "Safely unmount and power off backup disk",
			icon:        "🔌",
		},
		menuItem{
			title:       "Help",
			description: "Show help information",
			icon:        "❓",
		},
		menuItem{
			title:       "Exit",
			description: "Exit the application",
			icon:        "❌",
		},
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "🗄️  ZFS Backup Management Tool"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1C069"))

	ti := textinput.New()
	ti.Placeholder = "/dev/sda"
	ti.Focus()
	ti.CharLimit = 50
	ti.Width = 40

	pi := textinput.New()
	pi.Placeholder = "Enter encryption password"
	pi.EchoMode = textinput.EchoPassword
	pi.EchoCharacter = '•'
	pi.CharLimit = 256
	pi.Width = 40

	return model{
		state:         stateMenu,
		list:          l,
		spinner:       s,
		input:         ti,
		passwordInput: pi,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width-4, msg.Height-8)
		return m, nil

	case tea.KeyMsg:
		if m.state == stateMenu {
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit

			case "enter":
				i, ok := m.list.SelectedItem().(menuItem)
				if ok {
					switch i.title {
					case "Backup ZFS (incremental)":
						m.operation = "backup"
						m.state = statePassword
						m.passwordInput.SetValue("")
						m.passwordInput.Focus()
						return m, textinput.Blink
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
						m.operation = "unmount"
						return m.startOperation()
					case "Help":
						m.showingHelp = true
						return m, nil
					case "Exit":
						m.quitting = true
						return m, tea.Quit
					}
				}
			}
		} else if m.state == stateConfirm {
			switch msg.String() {
			case "y", "Y":
				m.confirmYes = true
				// Check if this operation needs password
				if m.operation == "force-backup" {
					m.state = statePassword
					m.passwordInput.SetValue("")
					m.passwordInput.Focus()
					return m, textinput.Blink
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
		} else if m.state == stateResult || m.showingHelp {
			switch msg.String() {
			case "enter", "esc", "q":
				m.state = stateMenu
				m.showingHelp = false
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

	case operationResultMsg:
		m.state = stateResult
		m.message = msg.message
		m.err = msg.err
		return m, nil
	}

	var cmd tea.Cmd
	switch m.state {
	case stateMenu:
		m.list, cmd = m.list.Update(msg)
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

	if m.showingHelp {
		return m.renderHelp()
	}

	switch m.state {
	case stateMenu:
		header := titleStyle.Render("🗄️  ZFS Backup Management Tool") + "\n"
		subtitle := subtitleStyle.Render("Manage your NIXROOT to NIXBACKUPS backup operations") + "\n\n"
		return header + subtitle + m.list.View() + "\n\n" +
			infoStyle.Render("Press 'q' or Ctrl+C to quit")

	case stateConfirm:
		var b strings.Builder
		b.WriteString(titleStyle.Render("⚠️  Confirmation Required") + "\n\n")
		b.WriteString(warningStyle.Render(m.confirmMsg) + "\n\n")
		b.WriteString(infoStyle.Render("Press 'y' to confirm, 'n' to cancel") + "\n")
		return menuStyle.Render(b.String())

	case stateInput:
		var b strings.Builder
		b.WriteString(titleStyle.Render("🔧 Prepare Backup Device") + "\n\n")
		b.WriteString(infoStyle.Render("Enter the device path to use for backup:") + "\n\n")
		b.WriteString(m.input.View() + "\n\n")
		b.WriteString(subtitleStyle.Render("Example: /dev/sda") + "\n\n")
		b.WriteString(infoStyle.Render("Press Enter to continue, Esc to cancel") + "\n")
		return menuStyle.Render(b.String())

	case statePassword:
		var b strings.Builder
		b.WriteString(titleStyle.Render("🔐 Encryption Password") + "\n\n")
		b.WriteString(infoStyle.Render("Enter the encryption password for NIXBACKUPS:") + "\n\n")
		b.WriteString(m.passwordInput.View() + "\n\n")
		b.WriteString(infoStyle.Render("Press Enter to continue, Esc to cancel") + "\n")
		return menuStyle.Render(b.String())

	case stateRunning:
		var b strings.Builder
		b.WriteString(titleStyle.Render("⚙️  Working...") + "\n\n")
		b.WriteString(m.spinner.View() + " " + m.operation + "\n\n")
		b.WriteString(infoStyle.Render("Please wait while the operation completes...") + "\n")
		return menuStyle.Render(b.String())

	case stateResult:
		var b strings.Builder
		if m.err != nil {
			b.WriteString(titleStyle.Render("❌ Operation Failed") + "\n\n")
			b.WriteString(errorStyle.Render(m.err.Error()) + "\n\n")
		} else {
			b.WriteString(titleStyle.Render("✅ Operation Completed") + "\n\n")
			b.WriteString(statusStyle.Render(m.message) + "\n\n")
		}
		b.WriteString(infoStyle.Render("Press Enter or Esc to return to menu") + "\n")
		return menuStyle.Render(b.String())
	}

	return ""
}

func (m model) renderHelp() string {
	help := `
🗄️  ZFS Backup Management Tool

DESCRIPTION:
  A beautiful TUI for managing ZFS backups from NIXROOT to NIXBACKUPS.

OPERATIONS:

  📦 Backup ZFS (incremental)
     Performs an incremental backup using syncoid. Creates a timestamped
     snapshot, syncs to the external backup pool, and prunes old snapshots
     while keeping monthly archives. You will be prompted for the encryption
     password.

  🔥 Force Backup ZFS (destructive)
     Forces a complete backup by deleting previous snapshots on the backup
     disk. Use this when local and backup are out of sync. You will be
     prompted for the encryption password.

  🔧 Prepare Backup Device
     Creates an encrypted ZFS pool on a new external drive. This will
     erase all data on the specified device and create NIXBACKUPS pool
     with AES-256-GCM encryption.

  🔌 Unmount Backup Disk
     Safely exports the NIXBACKUPS pool and powers off the USB drive.
     Always use this before unplugging the backup drive.

REQUIREMENTS:
  - syncoid installed (from sanoid package)
  - ZFS filesystem with NIXROOT pool
  - External drive for NIXBACKUPS pool
  - Root privileges (sudo) OR ZFS delegation configured
  - Encryption password for NIXBACKUPS pool

KEYBOARD SHORTCUTS:
  ↑/↓      Navigate menu
  Enter    Select option
  y/n      Confirm/Cancel
  Esc      Go back
  q        Quit application
  Ctrl+C   Force quit

Press Enter or Esc to return to menu
`

	return reportBoxStyle.Render(help)
}

type operationResultMsg struct {
	message string
	err     error
}

func (m model) startOperation() (model, tea.Cmd) {
	m.state = stateRunning
	var cmd tea.Cmd

	switch m.operation {
	case "backup":
		cmd = runBackup(m.password)
	case "force-backup":
		cmd = runForceBackup(m.password)
	case "prepare":
		cmd = runPrepare(m.devicePath)
	case "unmount":
		cmd = runUnmount()
	}

	// Clear password from memory
	m.password = ""
	m.passwordInput.SetValue("")

	return m, tea.Batch(m.spinner.Tick, cmd)
}

func checkPermissions() error {
	// Check if we can run zfs commands by testing a simple read-only operation
	cmd := exec.Command("zfs", "list", "-H", "-o", "name")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("insufficient permissions to run ZFS commands.\nPlease run with: sudo zfs-backup\nOr configure ZFS delegation for your user")
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

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
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
	help := `
🗄️  ZFS Backup Management Tool

Usage: zfs-backup [OPTIONS]

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

Note: If you have ZFS delegation configured for your user, you can omit sudo.
`
	fmt.Println(reportBoxStyle.Render(help))
}
