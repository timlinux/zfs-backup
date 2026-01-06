package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// runBackup performs an incremental backup
func runBackup(password string) tea.Cmd {
	return func() tea.Msg {
		msg, err := performBackup(password)
		return operationResultMsg{message: msg, err: err}
	}
}

// runForceBackup performs a destructive force backup
func runForceBackup(password string) tea.Cmd {
	return func() tea.Msg {
		msg, err := performForceBackup(password)
		return operationResultMsg{message: msg, err: err}
	}
}

// runPrepare prepares a new backup device
func runPrepare(device string) tea.Cmd {
	return func() tea.Msg {
		msg, err := performPrepare(device)
		return operationResultMsg{message: msg, err: err}
	}
}

// runUnmount unmounts the backup disk
func runUnmount() tea.Cmd {
	return func() tea.Msg {
		msg, err := performUnmount()
		return operationResultMsg{message: msg, err: err}
	}
}

// Synchronous versions for CLI mode
func runBackupSync() {
	// Prompt for password
	fmt.Print("Enter encryption password for NIXBACKUPS: ")
	var password string
	fmt.Scanln(&password)

	msg, err := performBackup(password)
	if err != nil {
		fmt.Println(errorStyle.Render("❌ " + err.Error()))
		return
	}
	fmt.Println(statusStyle.Render(msg))
}

func runForceBackupSync() {
	// Prompt for password
	fmt.Print("Enter encryption password for NIXBACKUPS: ")
	var password string
	fmt.Scanln(&password)

	msg, err := performForceBackup(password)
	if err != nil {
		fmt.Println(errorStyle.Render("❌ " + err.Error()))
		return
	}
	fmt.Println(statusStyle.Render(msg))
}

func runUnmountSync() {
	msg, err := performUnmount()
	if err != nil {
		fmt.Println(errorStyle.Render("❌ " + err.Error()))
		return
	}
	fmt.Println(statusStyle.Render(msg))
}

func performBackup(password string) (string, error) {
	var output strings.Builder

	// Check if NIXBACKUPS is already imported
	output.WriteString("🐴 Checking if NIXBACKUPS is already imported...\n")
	imported, err := isPoolImported("NIXBACKUPS")
	if err != nil {
		return "", fmt.Errorf("failed to check pool status: %w", err)
	}

	if !imported {
		output.WriteString("🔌 Importing NIXBACKUPS volume from USB drive\n")
		if err := runCommand("zpool", "import", "NIXBACKUPS"); err != nil {
			return "", fmt.Errorf("failed to import pool: %w", err)
		}

		output.WriteString("🔓 Loading encryption key for NIXBACKUPS\n")
		if err := loadZFSKey("NIXBACKUPS", password); err != nil {
			return "", fmt.Errorf("failed to load encryption key: %w", err)
		}
	} else {
		output.WriteString("✅ NIXBACKUPS is already imported\n")

		// Check if key is loaded
		keyStatus, err := getKeyStatus("NIXBACKUPS")
		if err != nil {
			return "", fmt.Errorf("failed to check key status: %w", err)
		}

		if keyStatus != "available" {
			output.WriteString(fmt.Sprintf("🔓 Loading encryption key for NIXBACKUPS (key status: %s)\n", keyStatus))
			if err := loadZFSKey("NIXBACKUPS", password); err != nil {
				return "", fmt.Errorf("failed to load encryption key: %w", err)
			}
		} else {
			output.WriteString("✅ Encryption key is already loaded\n")
		}
	}

	// Create snapshot with timestamp
	timestamp := time.Now().Format("2006-01-02.15h-04")
	snapshotName := fmt.Sprintf("NIXROOT/home@%s-Home", timestamp)

	output.WriteString(fmt.Sprintf("🗓️  Preparing a snapshot for %s\n", timestamp))
	output.WriteString(fmt.Sprintf("📸 Creating local snapshot: %s\n", snapshotName))

	if err := runCommand("zfs", "snapshot", snapshotName); err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	output.WriteString("📨 Sending snapshots incrementally to backup disk\n")
	if err := runCommand("syncoid", "--create-bookmark", "NIXROOT/home", "NIXBACKUPS/home"); err != nil {
		return "", fmt.Errorf("syncoid failed: %w", err)
	}

	output.WriteString("🔖 Creating bookmarks for snapshots older than 7 days and deleting snapshots\n")
	if err := pruneOldLocalSnapshots(); err != nil {
		output.WriteString(fmt.Sprintf("⚠️  Warning: failed to prune local snapshots: %v\n", err))
	}

	output.WriteString("🧹 Pruning old snapshots on backup disk (keeping monthly archives)\n")
	if err := pruneBackupSnapshots(); err != nil {
		output.WriteString(fmt.Sprintf("⚠️  Warning: failed to prune backup snapshots: %v\n", err))
	}

	// Generate report
	report, err := generateBackupReport()
	if err != nil {
		output.WriteString(fmt.Sprintf("⚠️  Warning: failed to generate report: %v\n", err))
	} else {
		output.WriteString("\n" + report + "\n")
	}

	output.WriteString("🔌 Exporting the backup zpool\n")

	// Get device before exporting
	device, err := getBackupDevice()
	if err == nil {
		if err := runCommand("zpool", "export", "NIXBACKUPS"); err != nil {
			return "", fmt.Errorf("failed to export pool: %w", err)
		}

		output.WriteString(fmt.Sprintf("⚡️ Powering off the USB drive (%s)\n", device))
		if err := runCommand("udisksctl", "power-off", "-b", device); err != nil {
			output.WriteString(fmt.Sprintf("⚠️  Warning: failed to power off device: %v\n", err))
		}
	} else {
		output.WriteString("⚠️  Skipping device power-off due to device detection failure\n")
		if err := runCommand("zpool", "export", "NIXBACKUPS"); err != nil {
			return "", fmt.Errorf("failed to export pool: %w", err)
		}
	}

	output.WriteString("\n✅ Backup completed successfully!")
	return output.String(), nil
}

func performForceBackup(password string) (string, error) {
	var output strings.Builder

	timestamp := time.Now().Format("2006-01-02.15h-04")

	output.WriteString("🔌 Mounting NIXBACKUPS volume from USB drive\n")
	if err := runCommand("zpool", "import", "NIXBACKUPS"); err != nil {
		return "", fmt.Errorf("failed to import pool: %w", err)
	}

	output.WriteString("🔓 Loading encryption key for NIXBACKUPS\n")
	if err := loadZFSKey("NIXBACKUPS", password); err != nil {
		return "", fmt.Errorf("failed to load encryption key: %w", err)
	}

	output.WriteString(fmt.Sprintf("🗓️  Preparing a snapshot for %s\n", timestamp))
	output.WriteString("📸 Taking a snapshot\n")

	snapshotName := fmt.Sprintf("NIXROOT/home@%s-Home", timestamp)
	if err := runCommand("zfs", "snapshot", snapshotName); err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	output.WriteString("📨 Force sending the snapshots to the external USB disk\n")
	if err := runCommand("syncoid", "--force-delete", "NIXROOT/home", "NIXBACKUPS/home"); err != nil {
		return "", fmt.Errorf("syncoid failed: %w", err)
	}

	output.WriteString("📝 Listing the snapshots now that it is copied to the USB disk\n")
	snapshots, err := listSnapshots()
	if err != nil {
		output.WriteString(fmt.Sprintf("⚠️  Warning: failed to list snapshots: %v\n", err))
	} else {
		output.WriteString(snapshots + "\n")
	}

	output.WriteString("\n✅ Force backup completed successfully!")
	return output.String(), nil
}

func performPrepare(device string) (string, error) {
	var output strings.Builder

	output.WriteString(fmt.Sprintf("🔧 Preparing backup device: %s\n", device))
	output.WriteString("⚠️  Creating encrypted ZFS pool NIXBACKUPS\n")

	// Note: This will prompt for passphrase interactively
	if err := runCommand("zpool", "create",
		"-O", "encryption=aes-256-gcm",
		"-O", "keyformat=passphrase",
		"-O", "keylocation=prompt",
		"-O", "compression=zstd",
		"-O", "atime=off",
		"NIXBACKUPS",
		device); err != nil {
		return "", fmt.Errorf("failed to create pool: %w", err)
	}

	output.WriteString(fmt.Sprintf("✅ Backup device %s prepared as encrypted ZFS pool NIXBACKUPS\n", device))
	return output.String(), nil
}

func performUnmount() (string, error) {
	var output strings.Builder

	output.WriteString("🔌 Unmounting the backup zpool\n\n")
	output.WriteString("📊 BEFORE STATE:\n")
	output.WriteString("================\n")

	pools, err := runCommandOutput("zpool", "list")
	if err == nil {
		output.WriteString("ZFS Pools:\n" + pools + "\n\n")
	}

	filesystems, err := runCommandOutput("zfs", "list")
	if err == nil {
		output.WriteString("ZFS Filesystems:\n" + filesystems + "\n\n")
	}

	// Get device before exporting
	device, err := getBackupDevice()
	if err == nil {
		output.WriteString("🔓 Exporting NIXBACKUPS pool...\n")
		if err := runCommand("zpool", "export", "NIXBACKUPS"); err != nil {
			return "", fmt.Errorf("failed to export pool: %w", err)
		}

		output.WriteString(fmt.Sprintf("⚡️ Powering off the USB drive (%s)\n", device))
		if err := runCommand("udisksctl", "power-off", "-b", device); err != nil {
			output.WriteString(fmt.Sprintf("⚠️  Warning: failed to power off device: %v\n", err))
		}
	} else {
		output.WriteString("⚠️  Skipping device power-off due to device detection failure\n")
		// Try to export anyway
		_ = runCommand("zpool", "export", "NIXBACKUPS")
	}

	output.WriteString("\n📊 AFTER STATE:\n")
	output.WriteString("===============\n")

	pools, err = runCommandOutput("zpool", "list")
	if err == nil {
		output.WriteString("ZFS Pools:\n" + pools + "\n\n")
	}

	filesystems, err = runCommandOutput("zfs", "list")
	if err == nil {
		output.WriteString("ZFS Filesystems:\n" + filesystems + "\n\n")
	}

	output.WriteString("✅ Safe to unplug the external drive")
	return output.String(), nil
}

// Helper functions

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w\nOutput: %s", name, err, string(output))
	}
	return nil
}

func runCommandWithStdin(stdin string, name string, args ...string) error {
	cmd := exec.Command(name, args...)

	// Create a pipe for stdin
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	// Capture stdout and stderr
	outputBytes := &bytes.Buffer{}
	cmd.Stdout = outputBytes
	cmd.Stderr = outputBytes

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s failed to start: %w", name, err)
	}

	// Write the password to stdin and close it
	_, err = stdinPipe.Write([]byte(stdin + "\n"))
	if err != nil {
		return fmt.Errorf("failed to write to stdin: %w", err)
	}
	stdinPipe.Close()

	// Wait for command to complete
	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("%s failed: %w\nOutput: %s", name, err, outputBytes.String())
	}

	return nil
}

// loadZFSKey loads a ZFS encryption key by passing password via stdin
func loadZFSKey(dataset, password string) error {
	cmd := exec.Command("zfs", "load-key", dataset)

	// Set stdin to read from our password string
	cmd.Stdin = strings.NewReader(password + "\n")

	// Capture output for error reporting
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("zfs load-key failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func runCommandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w", name, err)
	}
	return string(output), nil
}

func isPoolImported(poolName string) (bool, error) {
	output, err := runCommandOutput("zpool", "list")
	if err != nil {
		return false, err
	}
	return strings.Contains(output, poolName), nil
}

func getKeyStatus(dataset string) (string, error) {
	output, err := runCommandOutput("zfs", "get", "-H", "-o", "value", "keystatus", dataset)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func getBackupDevice() (string, error) {
	output, err := runCommandOutput("zpool", "status", "NIXBACKUPS")
	if err != nil {
		return "", fmt.Errorf("could not get pool status: %w", err)
	}

	// Look for device lines (indented with tabs/spaces, no colons)
	re := regexp.MustCompile(`^\s+(sd[a-z]+|nvme[0-9]+n[0-9]+)\s+ONLINE`)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if matches := re.FindStringSubmatch(line); len(matches) > 1 {
			device := matches[1]
			// Remove trailing digits
			device = regexp.MustCompile(`[0-9]+$`).ReplaceAllString(device, "")
			// Add /dev/ prefix if not present
			if !strings.HasPrefix(device, "/dev/") {
				device = "/dev/" + device
			}
			return device, nil
		}
	}

	return "", fmt.Errorf("could not detect backup device")
}

func pruneOldLocalSnapshots() error {
	// Get snapshots older than 7 days
	output, err := runCommandOutput("zfs", "list", "-H", "-o", "name", "-t", "snapshot", "-S", "creation")
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var nixrootSnapshots []string
	for _, line := range lines {
		if strings.HasPrefix(line, "NIXROOT/home@") {
			nixrootSnapshots = append(nixrootSnapshots, line)
		}
	}

	// Keep first 7, bookmark and delete the rest
	if len(nixrootSnapshots) > 7 {
		for _, snap := range nixrootSnapshots[7:] {
			bookmark := strings.Replace(snap, "@", "#", 1)
			_ = runCommand("zfs", "bookmark", snap, bookmark)
			_ = runCommand("zfs", "destroy", snap)
		}
	}

	return nil
}

func pruneBackupSnapshots() error {
	now := time.Now()
	keepMonths := []string{
		now.Format("2006-01"),
		now.AddDate(0, -1, 0).Format("2006-01"),
		now.AddDate(0, -2, 0).Format("2006-01"),
	}

	output, err := runCommandOutput("zfs", "list", "-H", "-o", "name", "-t", "snapshot")
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "NIXBACKUPS/home@") {
			continue
		}

		shouldKeep := false
		for _, month := range keepMonths {
			if strings.Contains(line, month) {
				shouldKeep = true
				break
			}
		}

		if !shouldKeep {
			bookmark := strings.Replace(line, "@", "#", 1)
			_ = runCommand("zfs", "bookmark", line, bookmark)
			_ = runCommand("zfs", "destroy", line)
		}
	}

	return nil
}

func listSnapshots() (string, error) {
	return runCommandOutput("zfs", "list", "-t", "snapshot")
}

func generateBackupReport() (string, error) {
	var report bytes.Buffer

	report.WriteString("📊 Backup Report Summary\n")
	report.WriteString(strings.Repeat("─", 50) + "\n")

	// Get oldest snapshot
	output, err := runCommandOutput("zfs", "list", "-t", "snapshot", "-o", "name,creation", "-s", "creation")
	if err == nil {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "NIXBACKUPS/home") {
				report.WriteString(fmt.Sprintf("• Oldest snapshot: %s\n", line))
				break
			}
		}
	}

	// Count local snapshots
	localCount := 0
	output, err = runCommandOutput("zfs", "list", "-H", "-t", "snapshot", "-o", "name")
	if err == nil {
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, "NIXROOT/home@") {
				localCount++
			}
		}
	}
	report.WriteString(fmt.Sprintf("• Snapshots on local: %d\n", localCount))

	// Count backup snapshots
	backupCount := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "NIXBACKUPS/home@") {
			backupCount++
		}
	}
	report.WriteString(fmt.Sprintf("• Snapshots on backup: %d\n", backupCount))

	// Missing snapshots
	missing := 0
	if backupCount < localCount {
		missing = localCount - backupCount
	}
	report.WriteString(fmt.Sprintf("• Missing snapshots: %d\n", missing))

	// Free space
	output, err = runCommandOutput("zfs", "list", "-H", "-o", "available", "NIXROOT")
	if err == nil {
		report.WriteString(fmt.Sprintf("• Free space on local: %s\n", strings.TrimSpace(output)))
	}

	output, err = runCommandOutput("zfs", "list", "-H", "-o", "available", "NIXBACKUPS")
	if err == nil {
		report.WriteString(fmt.Sprintf("• Free space on backup: %s\n", strings.TrimSpace(output)))
	}

	return reportBoxStyle.Render(report.String()), nil
}
