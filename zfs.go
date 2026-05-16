package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// getLocalHostname returns the local machine's hostname
func getLocalHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "localhost"
	}
	return hostname
}

// getRemoteHostname extracts the hostname from a user@host SSH string
func getRemoteHostname(sshHost string) string {
	// Extract just the hostname from user@host
	parts := strings.Split(sshHost, "@")
	host := parts[len(parts)-1]
	// Remove port if present
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return host
}

// getHostnameDatasetPath returns the dataset path namespaced by hostname.
// For example: NIXBACKUPS/<hostname>/home
func getHostnameDatasetPath(destPool, hostname, datasetSuffix string) string {
	return fmt.Sprintf("%s/%s/%s", destPool, hostname, datasetSuffix)
}

// resolveBackupDestination determines the correct destination dataset path.
// For backward compatibility: if the old flat path (e.g., DESTPOOL/home) exists
// and no hostname-namespaced path exists, use the flat path.
// For new setups or when hostname namespace already exists, use DESTPOOL/<hostname>/home.
func resolveBackupDestination(destPool, hostname, datasetSuffix string) string {
	namespacedPath := getHostnameDatasetPath(destPool, hostname, datasetSuffix)
	flatPath := fmt.Sprintf("%s/%s", destPool, datasetSuffix)

	// Check if namespaced path already exists (preferred)
	if _, err := runCommandOutput("zfs", "list", "-H", namespacedPath); err == nil {
		return namespacedPath
	}

	// Check if flat path exists (backward compat)
	if _, err := runCommandOutput("zfs", "list", "-H", flatPath); err == nil {
		return flatPath
	}

	// Neither exists - use hostname-namespaced path for new setups
	return namespacedPath
}

// getChildDatasets returns the child dataset names (suffixes) of a pool.
// For example, for pool NIXROOT with datasets NIXROOT/home, NIXROOT/nix, NIXROOT/root,
// it returns ["home", "nix", "root"]. The pool root dataset itself is excluded.
func getChildDatasets(pool string) ([]string, error) {
	output, err := runCommandOutput("zfs", "list", "-H", "-o", "name", "-r", pool)
	if err != nil {
		return nil, err
	}

	var datasets []string
	prefix := pool + "/"
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == pool {
			continue
		}
		// Only include direct children (not nested like pool/a/b)
		if strings.HasPrefix(line, prefix) {
			suffix := strings.TrimPrefix(line, prefix)
			if !strings.Contains(suffix, "/") {
				datasets = append(datasets, suffix)
			}
		}
	}

	return datasets, nil
}

// getRemoteChildDatasets returns child dataset names from a remote host via SSH.
func getRemoteChildDatasets(sshHost, pool string) ([]string, error) {
	output, err := runCommandOutput("ssh", sshHost, "zfs", "list", "-H", "-o", "name", "-r", pool)
	if err != nil {
		return nil, err
	}

	var datasets []string
	prefix := pool + "/"
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == pool {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			suffix := strings.TrimPrefix(line, prefix)
			if !strings.Contains(suffix, "/") {
				datasets = append(datasets, suffix)
			}
		}
	}

	return datasets, nil
}

// progressUpdate is sent to update the UI during backup
type progressUpdate struct {
	stage       string
	stageNum    int
	totalStages int
	state       *BackupState
}

// runBackup performs an incremental backup with progress updates
func runBackup(ctx context.Context, password, sourcePool, destPool string, resumeFrom *BackupState, progressChan chan<- progressUpdate) tea.Cmd {
	return func() tea.Msg {
		msg, err := performBackup(ctx, password, sourcePool, destPool, resumeFrom, progressChan)
		return operationResultMsg{message: msg, err: err}
	}
}

// runForceBackup performs a destructive force backup with progress updates
func runForceBackup(ctx context.Context, password, sourcePool, destPool string, resumeFrom *BackupState, progressChan chan<- progressUpdate) tea.Cmd {
	return func() tea.Msg {
		msg, err := performForceBackup(ctx, password, sourcePool, destPool, resumeFrom, progressChan)
		return operationResultMsg{message: msg, err: err}
	}
}

// runPrepare prepares a new backup device
func runPrepare(device, poolName, password string) tea.Cmd {
	return func() tea.Msg {
		msg, err := performPrepare(device, poolName, password)
		return operationResultMsg{message: msg, err: err}
	}
}

// runUnmount unmounts the backup disk
func runUnmount(poolName string) tea.Cmd {
	return func() tea.Msg {
		msg, err := performUnmount(poolName)
		return operationResultMsg{message: msg, err: err}
	}
}

// runRecover attempts to recover from an abnormally ended backup
func runRecover(ctx context.Context, password, sourcePool, destPool string, progressChan chan<- progressUpdate) tea.Cmd {
	return func() tea.Msg {
		msg, err := performRecover(ctx, password, sourcePool, destPool, progressChan)
		return operationResultMsg{message: msg, err: err}
	}
}

// runPushBackup performs a push backup to a remote server via SSH
func runPushBackup(ctx context.Context, password, sourcePool, remoteHost, remoteDestPool string, resumeFrom *BackupState, progressChan chan<- progressUpdate) tea.Cmd {
	return func() tea.Msg {
		msg, err := performPushBackup(ctx, password, sourcePool, remoteHost, remoteDestPool, resumeFrom, progressChan)
		return operationResultMsg{message: msg, err: err}
	}
}

// runRemoteBackup performs a remote backup via SSH with progress updates
func runRemoteBackup(ctx context.Context, password, remoteHost, remoteDataset, destPool string, resumeFrom *BackupState, progressChan chan<- progressUpdate) tea.Cmd {
	return func() tea.Msg {
		msg, err := performRemoteBackup(ctx, password, remoteHost, remoteDataset, destPool, resumeFrom, progressChan)
		return operationResultMsg{message: msg, err: err}
	}
}

// Synchronous versions for CLI mode
func runBackupSync() {
	// Prompt for password
	fmt.Print("Enter encryption password for NIXBACKUPS: ")
	var password string
	fmt.Scanln(&password)

	ctx := context.Background()
	msg, err := performBackup(ctx, password, "NIXROOT", "NIXBACKUPS", nil, nil)
	if err != nil {
		fmt.Println(errorStyle.Render("Error:" + err.Error()))
		return
	}
	fmt.Println(statusStyle.Render(msg))
}

func runForceBackupSync() {
	// Prompt for password
	fmt.Print("Enter encryption password for NIXBACKUPS: ")
	var password string
	fmt.Scanln(&password)

	ctx := context.Background()
	msg, err := performForceBackup(ctx, password, "NIXROOT", "NIXBACKUPS", nil, nil)
	if err != nil {
		fmt.Println(errorStyle.Render("Error:" + err.Error()))
		return
	}
	fmt.Println(statusStyle.Render(msg))
}

func runUnmountSync() {
	msg, err := performUnmount("NIXBACKUPS")
	if err != nil {
		fmt.Println(errorStyle.Render("Error:" + err.Error()))
		return
	}
	fmt.Println(statusStyle.Render(msg))
}

func performBackup(ctx context.Context, password, sourcePool, destPool string, resumeFrom *BackupState, progressChan chan<- progressUpdate) (string, error) {
	var output strings.Builder

	// Validate pool names
	if sourcePool == "" {
		return "", fmt.Errorf("source pool not selected")
	}
	if destPool == "" {
		return "", fmt.Errorf("destination pool not selected")
	}

	output.WriteString(fmt.Sprintf("Backing up %s → %s\n\n", sourcePool, destPool))

	// Initialize or load backup state
	var state *BackupState
	if resumeFrom != nil {
		state = resumeFrom
		output.WriteString("[RESUME]Resuming backup from previous session...\n\n")
	} else {
		state = NewBackupState("backup")
	}

	// Save initial state
	if err := SaveBackupState(state); err != nil {
		return "", fmt.Errorf("failed to save state: %w", err)
	}

	totalStages := 7
	currentStage := 1

	// Helper to send progress updates
	sendProgress := func(stage string, stageEnum BackupStage) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			output.WriteString(fmt.Sprintf("[%d/%d] %s\n", currentStage, totalStages, stage))
			// Send progress to UI
			if progressChan != nil {
				progressChan <- progressUpdate{
					stage:       stage,
					stageNum:    currentStage,
					totalStages: totalStages,
					state:       state,
				}
			}
			currentStage++
			return nil
		}
	}

	// Helper to execute stage with progress tracking
	executeStage := func(stageEnum BackupStage, stageName string, fn func() error) error {
		if state.IsStageCompleted(stageEnum) {
			output.WriteString(fmt.Sprintf("✓ Skipping completed stage: %s\n", stageName))
			currentStage++
			return nil
		}

		if err := sendProgress(stageName, stageEnum); err != nil {
			return err
		}

		stageStart := time.Now()
		state.CurrentStage = stageEnum
		_ = SaveBackupState(state)

		if err := fn(); err != nil {
			return err
		}

		duration := time.Since(stageStart)
		state.MarkStageCompleted(stageEnum, duration)
		_ = SaveBackupState(state)

		return nil
	}

	// Stage 1: Import pool
	err := executeStage(StageImportPool, fmt.Sprintf("[POOL]Importing %s pool", destPool), func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 IMPORT POOL\n")
		output.WriteString("   ZFS pools on external drives must be 'imported' before use.\n")
		output.WriteString("   This makes the pool available to the system without mounting\n")
		output.WriteString("   individual filesystems yet.\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		imported, err := isPoolImported(destPool)
		if err != nil {
			return fmt.Errorf("failed to check pool status: %w", err)
		}

		if !imported {
			output.WriteString(fmt.Sprintf("Importing %s volume from USB drive\n", destPool))
			if err := runCommandWithContext(ctx, "zpool", "import", destPool); err != nil {
				return fmt.Errorf("failed to import pool: %w", err)
			}
		} else {
			output.WriteString(fmt.Sprintf("[OK]%s is already imported\n", destPool))
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 2: Load encryption key
	err = executeStage(StageLoadKey, "🔓 Loading encryption key", func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 ENCRYPTION KEY\n")
		output.WriteString("   Your backup pool uses ZFS native encryption (AES-256-GCM).\n")
		output.WriteString("   The encryption key must be loaded before any data can be\n")
		output.WriteString("   read or written. Data at rest remains encrypted on disk.\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		keyStatus, err := getKeyStatus(destPool)
		if err != nil {
			return fmt.Errorf("failed to check key status: %w", err)
		}

		if keyStatus != "available" {
			output.WriteString(fmt.Sprintf("Loading encryption key (status: %s)\n", keyStatus))
			if err := loadZFSKey(destPool, password); err != nil {
				return fmt.Errorf("failed to load encryption key: %w", err)
			}
		} else {
			output.WriteString("[OK]Encryption key is already loaded\n")
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 3: Create recursive snapshot (all datasets)
	err = executeStage(StageCreateSnapshot, "[SNAP]Creating snapshot", func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 CREATE SNAPSHOT\n")
		output.WriteString("   A ZFS snapshot captures the exact state of your data at this\n")
		output.WriteString("   moment. Snapshots are instant and space-efficient - they only\n")
		output.WriteString("   store the differences (deltas) from the live filesystem.\n")
		output.WriteString("   Using -r to snapshot all datasets under the source pool.\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		timestamp := time.Now().Format("2006-01-02.15h-04")
		snapshotTag := fmt.Sprintf("%s-Backup", timestamp)
		snapshotName := fmt.Sprintf("%s@%s", sourcePool, snapshotTag)
		state.SnapshotName = snapshotName
		_ = SaveBackupState(state)

		output.WriteString(fmt.Sprintf("Creating recursive snapshot: %s\n", snapshotName))
		if err := runCommandWithContext(ctx, "zfs", "snapshot", "-r", snapshotName); err != nil {
			return fmt.Errorf("failed to create snapshot: %w", err)
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 4: Sync data (all datasets)
	err = executeStage(StageSyncData, "📨 Syncing data to backup disk", func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 INCREMENTAL SYNC (using syncoid)\n")
		output.WriteString("   Syncoid uses ZFS send/receive to transfer only the changes\n")
		output.WriteString("   (deltas) since the last backup. This is much faster than\n")
		output.WriteString("   copying all files. The data stream is sent directly from\n")
		output.WriteString("   source to destination at the block level.\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		hostname := getLocalHostname()

		// Get all child datasets of the source pool
		datasets, err := getChildDatasets(sourcePool)
		if err != nil {
			return fmt.Errorf("failed to list source datasets: %w", err)
		}

		if len(datasets) == 0 {
			output.WriteString("Warning: No child datasets found, nothing to sync\n")
			return nil
		}

		output.WriteString(fmt.Sprintf("Found %d dataset(s) to sync: %s\n\n", len(datasets), strings.Join(datasets, ", ")))

		for i, ds := range datasets {
			syncDest := resolveBackupDestination(destPool, hostname, ds)
			syncSrc := fmt.Sprintf("%s/%s", sourcePool, ds)

			output.WriteString(fmt.Sprintf("[Dataset %d/%d] %s -> %s\n", i+1, len(datasets), syncSrc, syncDest))

			// Check for existing zfs receive process and wait for it
			if err := waitForZFSReceive(ctx, syncDest, &output); err != nil {
				return fmt.Errorf("error waiting for existing receive on %s: %w", ds, err)
			}

			if err := runCommandWithContext(ctx, "syncoid", "--create-bookmark", syncSrc, syncDest); err != nil {
				output.WriteString(fmt.Sprintf("Warning: syncoid failed for %s: %v (continuing...)\n", ds, err))
			} else {
				output.WriteString(fmt.Sprintf("[OK] %s synced successfully\n", ds))
			}
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 5: Prune local snapshots
	err = executeStage(StagePruneLocal, "🔖 Pruning local snapshots", func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 PRUNE LOCAL SNAPSHOTS → BOOKMARKS\n")
		output.WriteString("   Old snapshots on your local system are converted to bookmarks.\n")
		output.WriteString("   Bookmarks are tiny markers that allow future incremental sends\n")
		output.WriteString("   without keeping the full snapshot data locally. This saves\n")
		output.WriteString("   disk space while preserving backup continuity.\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		output.WriteString("Creating bookmarks and pruning old snapshots...\n")
		if err := pruneOldLocalSnapshots(sourcePool); err != nil {
			output.WriteString(fmt.Sprintf("Warning:Warning: %v\n", err))
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 6: Prune backup snapshots
	err = executeStage(StagePruneBackup, "🧹 Pruning backup snapshots", func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 PRUNE BACKUP SNAPSHOTS\n")
		output.WriteString("   Old snapshots on the backup drive are pruned to save space.\n")
		output.WriteString("   We keep recent snapshots plus monthly archives for the last\n")
		output.WriteString("   3 months. Pruned snapshots are converted to bookmarks first\n")
		output.WriteString("   to maintain the incremental backup chain.\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		output.WriteString("Keeping monthly archives...\n")
		if err := pruneBackupSnapshots(destPool); err != nil {
			output.WriteString(fmt.Sprintf("Warning:Warning: %v\n", err))
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 7: Export and power off
	err = executeStage(StageExportPool, "[POOL]Exporting pool and powering off", func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 EXPORT & POWER OFF\n")
		output.WriteString("   Exporting the pool ensures all data is flushed to disk and\n")
		output.WriteString("   the pool metadata is cleanly written. The USB drive is then\n")
		output.WriteString("   powered off safely, allowing you to physically disconnect it.\n")
		output.WriteString("   Never unplug without exporting first - this prevents corruption!\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		// Generate report
		report, err := generateBackupReport(sourcePool, destPool)
		if err != nil {
			output.WriteString(fmt.Sprintf("Warning:Warning: failed to generate report: %v\n", err))
		} else {
			output.WriteString("\n" + report + "\n")
		}

		output.WriteString("Exporting the backup zpool\n")
		device, err := getBackupDevice(destPool)
		if err == nil {
			if err := runCommandWithContext(ctx, "zpool", "export", destPool); err != nil {
				return fmt.Errorf("failed to export pool: %w", err)
			}

			output.WriteString(fmt.Sprintf("⚡️ Powering off USB drive (%s)\n", device))
			if err := runCommandWithContext(ctx, "udisksctl", "power-off", "-b", device); err != nil {
				output.WriteString(fmt.Sprintf("Warning:Warning: failed to power off device: %v\n", err))
			}
		} else {
			output.WriteString("Warning:Skipping device power-off\n")
			if err := runCommandWithContext(ctx, "zpool", "export", destPool); err != nil {
				return fmt.Errorf("failed to export pool: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	output.WriteString("\n[OK]Backup completed successfully!")
	return output.String(), nil
}

func performForceBackup(ctx context.Context, password, sourcePool, destPool string, resumeFrom *BackupState, progressChan chan<- progressUpdate) (string, error) {
	var output strings.Builder

	// Validate pool names
	if sourcePool == "" {
		return "", fmt.Errorf("source pool not selected")
	}
	if destPool == "" {
		return "", fmt.Errorf("destination pool not selected")
	}

	output.WriteString(fmt.Sprintf("Force backing up %s → %s\n\n", sourcePool, destPool))

	// Initialize or load backup state
	var state *BackupState
	if resumeFrom != nil {
		state = resumeFrom
		output.WriteString("[RESUME]Resuming force backup from previous session...\n\n")
	} else {
		state = NewBackupState("force-backup")
	}

	// Save initial state
	if err := SaveBackupState(state); err != nil {
		return "", fmt.Errorf("failed to save state: %w", err)
	}

	totalStages := 5
	currentStage := 1

	// Helper to send progress updates
	sendProgress := func(stage string, stageEnum BackupStage) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			output.WriteString(fmt.Sprintf("[%d/%d] %s\n", currentStage, totalStages, stage))
			// Send progress to UI
			if progressChan != nil {
				progressChan <- progressUpdate{
					stage:       stage,
					stageNum:    currentStage,
					totalStages: totalStages,
					state:       state,
				}
			}
			currentStage++
			return nil
		}
	}

	// Helper to execute stage with progress tracking
	executeStage := func(stageEnum BackupStage, stageName string, fn func() error) error {
		if state.IsStageCompleted(stageEnum) {
			output.WriteString(fmt.Sprintf("✓ Skipping completed stage: %s\n", stageName))
			currentStage++
			return nil
		}

		if err := sendProgress(stageName, stageEnum); err != nil {
			return err
		}

		stageStart := time.Now()
		state.CurrentStage = stageEnum
		_ = SaveBackupState(state)

		if err := fn(); err != nil {
			return err
		}

		duration := time.Since(stageStart)
		state.MarkStageCompleted(stageEnum, duration)
		_ = SaveBackupState(state)

		return nil
	}

	// Stage 1: Import pool
	err := executeStage(StageImportPool, fmt.Sprintf("[POOL]Importing %s pool", destPool), func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 IMPORT POOL (Force Backup)\n")
		output.WriteString("   Importing the external backup pool to make it available.\n")
		output.WriteString("   Warning:Force backup will DELETE existing snapshots on the target!\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		output.WriteString(fmt.Sprintf("Mounting %s volume from USB drive\n", destPool))
		if err := runCommandWithContext(ctx, "zpool", "import", destPool); err != nil {
			return fmt.Errorf("failed to import pool: %w", err)
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 2: Load encryption key
	err = executeStage(StageLoadKey, "🔓 Loading encryption key", func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 ENCRYPTION KEY\n")
		output.WriteString("   Loading the encryption key to access the backup pool.\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		output.WriteString(fmt.Sprintf("Loading encryption key for %s\n", destPool))
		if err := loadZFSKey(destPool, password); err != nil {
			return fmt.Errorf("failed to load encryption key: %w", err)
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 3: Create recursive snapshot
	err = executeStage(StageCreateSnapshot, "[SNAP]Creating snapshot", func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 CREATE SNAPSHOT\n")
		output.WriteString("   Creating recursive snapshot of all local datasets.\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		timestamp := time.Now().Format("2006-01-02.15h-04")
		snapshotTag := fmt.Sprintf("%s-Backup", timestamp)
		snapshotName := fmt.Sprintf("%s@%s", sourcePool, snapshotTag)
		state.SnapshotName = snapshotName
		_ = SaveBackupState(state)

		output.WriteString(fmt.Sprintf("Creating recursive snapshot: %s\n", snapshotName))
		if err := runCommandWithContext(ctx, "zfs", "snapshot", "-r", snapshotName); err != nil {
			return fmt.Errorf("failed to create snapshot: %w", err)
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 4: Force sync data (all datasets)
	err = executeStage(StageSyncData, "📨 Force syncing to backup disk", func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 FORCE SYNC (DESTRUCTIVE)\n")
		output.WriteString("   Warning:Using --force-delete to remove old snapshots on backup.\n")
		output.WriteString("   This resets the backup to match your current source state.\n")
		output.WriteString("   Use this when the incremental chain is broken or corrupted.\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		hostname := getLocalHostname()

		datasets, err := getChildDatasets(sourcePool)
		if err != nil {
			return fmt.Errorf("failed to list source datasets: %w", err)
		}

		if len(datasets) == 0 {
			output.WriteString("Warning: No child datasets found, nothing to sync\n")
			return nil
		}

		output.WriteString(fmt.Sprintf("Found %d dataset(s) to force sync: %s\n\n", len(datasets), strings.Join(datasets, ", ")))

		for i, ds := range datasets {
			syncDest := resolveBackupDestination(destPool, hostname, ds)
			syncSrc := fmt.Sprintf("%s/%s", sourcePool, ds)

			output.WriteString(fmt.Sprintf("[Dataset %d/%d] %s -> %s\n", i+1, len(datasets), syncSrc, syncDest))

			if err := waitForZFSReceive(ctx, syncDest, &output); err != nil {
				return fmt.Errorf("error waiting for existing receive on %s: %w", ds, err)
			}

			if err := runCommandWithContext(ctx, "syncoid", "--force-delete", syncSrc, syncDest); err != nil {
				output.WriteString(fmt.Sprintf("Warning: syncoid failed for %s: %v (continuing...)\n", ds, err))
			} else {
				output.WriteString(fmt.Sprintf("[OK] %s force synced successfully\n", ds))
			}
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 5: List snapshots
	err = executeStage(StagePruneBackup, "📝 Listing snapshots", func() error {
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		output.WriteString("📖 SNAPSHOT SUMMARY\n")
		output.WriteString("   Listing all snapshots on the backup disk after force sync.\n")
		output.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		output.WriteString("Listing the snapshots on backup disk\n")
		snapshots, err := listSnapshots()
		if err != nil {
			output.WriteString(fmt.Sprintf("Warning:Warning: failed to list snapshots: %v\n", err))
		} else {
			output.WriteString(snapshots + "\n")
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	output.WriteString("\n[OK]Force backup completed successfully!")
	return output.String(), nil
}

func performPrepare(device, poolName, password string) (string, error) {
	var output strings.Builder

	if poolName == "" {
		return "", fmt.Errorf("pool name not specified")
	}

	if device == "" {
		return "", fmt.Errorf("device path not specified")
	}

	if password == "" {
		return "", fmt.Errorf("encryption passphrase not specified")
	}

	output.WriteString(fmt.Sprintf("[SETUP]Preparing backup device: %s\n", device))

	// Step 1: Clear any existing ZFS labels on the device
	output.WriteString("Clearing existing ZFS labels...\n")
	// zpool labelclear may fail if no labels exist, so we ignore errors
	_ = runCommand("zpool", "labelclear", "-f", device)

	// Step 2: Wipe partition table using wipefs (removes all filesystem signatures)
	output.WriteString("Wiping filesystem signatures...\n")
	if err := runCommand("wipefs", "-a", device); err != nil {
		output.WriteString(fmt.Sprintf("Warning:wipefs warning: %v (continuing anyway)\n", err))
	}

	// Step 3: Create GPT partition table
	output.WriteString("Creating GPT partition table...\n")
	if err := runCommand("sgdisk", "-Z", device); err != nil {
		output.WriteString(fmt.Sprintf("Warning:sgdisk warning: %v (continuing anyway)\n", err))
	}

	output.WriteString(fmt.Sprintf("Creating encrypted ZFS pool %s...\n", poolName))

	// Step 4: Create the encrypted ZFS pool with -f flag to force
	// Pass password via stdin using keylocation=file:///dev/stdin
	if err := runCommandWithStdin(password, "zpool", "create",
		"-f", // Force creation even if device appears to be in use
		"-O", "encryption=aes-256-gcm",
		"-O", "keyformat=passphrase",
		"-O", "keylocation=file:///dev/stdin",
		"-O", "compression=zstd",
		"-O", "atime=off",
		poolName,
		device); err != nil {
		return "", fmt.Errorf("failed to create pool: %w", err)
	}

	// Update keylocation to prompt for future unlocks
	output.WriteString("Updating key location for future unlocks...\n")
	if err := runCommand("zfs", "set", "keylocation=prompt", poolName); err != nil {
		output.WriteString(fmt.Sprintf("Warning:Could not update keylocation: %v\n", err))
	}

	output.WriteString(fmt.Sprintf("[OK]Backup device %s prepared as encrypted ZFS pool %s\n", device, poolName))
	return output.String(), nil
}

func performUnmount(poolName string) (string, error) {
	var output strings.Builder

	if poolName == "" {
		return "", fmt.Errorf("pool name not specified")
	}

	output.WriteString(fmt.Sprintf("[POOL]Unmounting the %s zpool\n\n", poolName))
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
	device, err := getBackupDevice(poolName)
	if err == nil {
		output.WriteString(fmt.Sprintf("🔓 Exporting %s pool...\n", poolName))
		if err := runCommand("zpool", "export", poolName); err != nil {
			return "", fmt.Errorf("failed to export pool: %w", err)
		}

		output.WriteString(fmt.Sprintf("⚡️ Powering off the USB drive (%s)\n", device))
		if err := runCommand("udisksctl", "power-off", "-b", device); err != nil {
			output.WriteString(fmt.Sprintf("Warning:Warning: failed to power off device: %v\n", err))
		}
	} else {
		output.WriteString("Warning:Skipping device power-off due to device detection failure\n")
		// Try to export anyway
		_ = runCommand("zpool", "export", poolName)
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

	output.WriteString("[OK]Safe to unplug the external drive")
	return output.String(), nil
}

func performRecover(ctx context.Context, password, sourcePool, destPool string, progressChan chan<- progressUpdate) (string, error) {
	var output strings.Builder

	// Validate pool names
	if sourcePool == "" {
		return "", fmt.Errorf("source pool not selected")
	}
	if destPool == "" {
		return "", fmt.Errorf("destination pool not selected")
	}

	output.WriteString(fmt.Sprintf("Recovering backup sync: %s -> %s\n\n", sourcePool, destPool))

	totalStages := 4
	currentStage := 1

	// Helper to send progress updates
	sendProgress := func(stage string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			output.WriteString(fmt.Sprintf("[%d/%d] %s\n", currentStage, totalStages, stage))
			if progressChan != nil {
				progressChan <- progressUpdate{
					stage:       stage,
					stageNum:    currentStage,
					totalStages: totalStages,
					state:       nil,
				}
			}
			currentStage++
			return nil
		}
	}

	// Stage 1: Import pool
	if err := sendProgress(fmt.Sprintf("Importing %s pool", destPool)); err != nil {
		return output.String(), err
	}
	output.WriteString("-----------------------------------------------------------\n")
	output.WriteString("IMPORT POOL\n")
	output.WriteString("   Importing the backup pool to access its state.\n")
	output.WriteString("-----------------------------------------------------------\n\n")

	imported, err := isPoolImported(destPool)
	if err != nil {
		return output.String(), fmt.Errorf("failed to check pool status: %w", err)
	}

	if !imported {
		output.WriteString(fmt.Sprintf("Importing %s volume from USB drive\n", destPool))
		if err := runCommandWithContext(ctx, "zpool", "import", destPool); err != nil {
			return output.String(), fmt.Errorf("failed to import pool: %w", err)
		}
	} else {
		output.WriteString(fmt.Sprintf("[OK] %s is already imported\n", destPool))
	}

	// Stage 2: Load encryption key
	if err := sendProgress("Loading encryption key"); err != nil {
		return output.String(), err
	}
	output.WriteString("-----------------------------------------------------------\n")
	output.WriteString("ENCRYPTION KEY\n")
	output.WriteString("   Loading the encryption key to access the backup pool.\n")
	output.WriteString("-----------------------------------------------------------\n\n")

	keyStatus, err := getKeyStatus(destPool)
	if err != nil {
		return output.String(), fmt.Errorf("failed to check key status: %w", err)
	}

	if keyStatus != "available" {
		output.WriteString(fmt.Sprintf("Loading encryption key (status: %s)\n", keyStatus))
		if err := loadZFSKey(destPool, password); err != nil {
			return output.String(), fmt.Errorf("failed to load encryption key: %w", err)
		}
	} else {
		output.WriteString("[OK] Encryption key is already loaded\n")
	}

	// Stage 3: Abort partial receive state
	if err := sendProgress("Clearing partial receive state"); err != nil {
		return output.String(), err
	}
	output.WriteString("-----------------------------------------------------------\n")
	output.WriteString("CLEAR PARTIAL RECEIVE STATE\n")
	output.WriteString("   When a ZFS send/receive is interrupted, the target dataset\n")
	output.WriteString("   retains a partial receive state. This must be cleared before\n")
	output.WriteString("   a new sync can begin. Using 'zfs receive -A' to abort.\n")
	output.WriteString("-----------------------------------------------------------\n\n")

	hostname := getLocalHostname()
	destDataset := resolveBackupDestination(destPool, hostname, "home")

	// Check if there's a partial receive state
	receiveResumeToken, _ := runCommandOutput("zfs", "get", "-H", "-o", "value", "receive_resume_token", destDataset)
	receiveResumeToken = strings.TrimSpace(receiveResumeToken)

	if receiveResumeToken != "" && receiveResumeToken != "-" {
		output.WriteString(fmt.Sprintf("Found partial receive state on %s\n", destDataset))
		output.WriteString("Aborting partial receive...\n")

		// Abort the partial receive
		abortCmd := exec.Command("zfs", "receive", "-A", destDataset)
		abortOutput, err := abortCmd.CombinedOutput()
		if err != nil {
			output.WriteString(fmt.Sprintf("Warning: zfs receive -A returned: %v\n", err))
			output.WriteString(fmt.Sprintf("Output: %s\n", string(abortOutput)))
		} else {
			output.WriteString("[OK] Partial receive state cleared successfully\n")
		}
	} else {
		output.WriteString("[OK] No partial receive state found (already clean)\n")
	}

	// Stage 4: Find common snapshots
	if err := sendProgress("Analyzing snapshots"); err != nil {
		return output.String(), err
	}
	output.WriteString("-----------------------------------------------------------\n")
	output.WriteString("SNAPSHOT ANALYSIS\n")
	output.WriteString("   Comparing snapshots between source and destination to find\n")
	output.WriteString("   a common point for incremental sync.\n")
	output.WriteString("-----------------------------------------------------------\n\n")

	sourceDataset := fmt.Sprintf("%s/home", sourcePool)

	// Get source snapshots
	sourceSnaps, err := runCommandOutput("zfs", "list", "-H", "-o", "name", "-t", "snapshot", "-r", sourceDataset)
	if err != nil {
		output.WriteString(fmt.Sprintf("Warning: Could not list source snapshots: %v\n", err))
	}

	// Get destination snapshots
	destSnaps, err := runCommandOutput("zfs", "list", "-H", "-o", "name", "-t", "snapshot", "-r", destDataset)
	if err != nil {
		output.WriteString(fmt.Sprintf("Warning: Could not list destination snapshots: %v\n", err))
	}

	// Parse snapshots into sets for comparison
	sourceSnapSet := make(map[string]bool)
	sourceSnapList := strings.Split(strings.TrimSpace(sourceSnaps), "\n")
	for _, snap := range sourceSnapList {
		if snap != "" {
			// Extract just the snapshot name (after @)
			parts := strings.SplitN(snap, "@", 2)
			if len(parts) == 2 {
				sourceSnapSet[parts[1]] = true
			}
		}
	}

	destSnapSet := make(map[string]bool)
	destSnapList := strings.Split(strings.TrimSpace(destSnaps), "\n")
	for _, snap := range destSnapList {
		if snap != "" {
			// Extract just the snapshot name (after @)
			parts := strings.SplitN(snap, "@", 2)
			if len(parts) == 2 {
				destSnapSet[parts[1]] = true
			}
		}
	}

	// Find common snapshots
	var commonSnapshots []string
	for snapName := range sourceSnapSet {
		if destSnapSet[snapName] {
			commonSnapshots = append(commonSnapshots, snapName)
		}
	}

	output.WriteString("SOURCE SNAPSHOTS:\n")
	if len(sourceSnapList) > 0 && sourceSnapList[0] != "" {
		for _, snap := range sourceSnapList {
			output.WriteString(fmt.Sprintf("  - %s\n", snap))
		}
	} else {
		output.WriteString("  (none found)\n")
	}
	output.WriteString("\n")

	output.WriteString("DESTINATION SNAPSHOTS:\n")
	if len(destSnapList) > 0 && destSnapList[0] != "" {
		for _, snap := range destSnapList {
			output.WriteString(fmt.Sprintf("  - %s\n", snap))
		}
	} else {
		output.WriteString("  (none found)\n")
	}
	output.WriteString("\n")

	output.WriteString("-----------------------------------------------------------\n")
	output.WriteString("RECOVERY RESULT\n")
	output.WriteString("-----------------------------------------------------------\n\n")

	if len(commonSnapshots) > 0 {
		output.WriteString("[OK] GOOD NEWS: Found common snapshot(s)!\n\n")
		output.WriteString("Common snapshots:\n")
		for _, snap := range commonSnapshots {
			output.WriteString(fmt.Sprintf("  - @%s\n", snap))
		}
		output.WriteString("\n")
		output.WriteString("NEXT STEPS:\n")
		output.WriteString("  1. You can now run a normal 'Backup ZFS (incremental)'\n")
		output.WriteString("  2. Syncoid will use the common snapshot as a base\n")
		output.WriteString("  3. Only changes since that snapshot will be transferred\n")
	} else {
		output.WriteString("[!] WARNING: No common snapshots found!\n\n")
		output.WriteString("This means incremental sync is not possible.\n\n")
		output.WriteString("NEXT STEPS:\n")
		output.WriteString("  1. Use 'Force Backup ZFS (destructive)' to reset the backup\n")
		output.WriteString("  2. This will delete all snapshots on the backup disk\n")
		output.WriteString("  3. A full backup will be performed (slower, but complete)\n")
	}

	output.WriteString("\n[OK] Recovery analysis completed!")
	return output.String(), nil
}

func performRemoteBackup(ctx context.Context, password, remoteHost, remoteDataset, destPool string, resumeFrom *BackupState, progressChan chan<- progressUpdate) (string, error) {
	var output strings.Builder

	if remoteHost == "" {
		return "", fmt.Errorf("remote host not specified")
	}
	if remoteDataset == "" {
		return "", fmt.Errorf("remote dataset not specified")
	}
	if destPool == "" {
		return "", fmt.Errorf("destination pool not selected")
	}

	// Extract hostname for namespacing
	hostname := getRemoteHostname(remoteHost)

	// Parse remote dataset - could be a pool (NIXROOT) or a specific dataset (NIXROOT/home)
	remotePool := remoteDataset
	if idx := strings.Index(remoteDataset, "/"); idx >= 0 {
		remotePool = remoteDataset[:idx]
	}

	output.WriteString(fmt.Sprintf("Remote backup: %s:%s -> %s/%s/\n\n", remoteHost, remoteDataset, destPool, hostname))

	// Initialize or load backup state
	var state *BackupState
	if resumeFrom != nil {
		state = resumeFrom
		output.WriteString("[RESUME] Resuming remote backup from previous session...\n\n")
	} else {
		state = NewBackupState("remote-backup")
	}

	if err := SaveBackupState(state); err != nil {
		return "", fmt.Errorf("failed to save state: %w", err)
	}

	totalStages := 5
	currentStage := 1

	sendProgress := func(stage string, stageEnum BackupStage) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			output.WriteString(fmt.Sprintf("[%d/%d] %s\n", currentStage, totalStages, stage))
			if progressChan != nil {
				progressChan <- progressUpdate{
					stage:       stage,
					stageNum:    currentStage,
					totalStages: totalStages,
					state:       state,
				}
			}
			currentStage++
			return nil
		}
	}

	executeStage := func(stageEnum BackupStage, stageName string, fn func() error) error {
		if state.IsStageCompleted(stageEnum) {
			output.WriteString(fmt.Sprintf("Skipping completed stage: %s\n", stageName))
			currentStage++
			return nil
		}

		if err := sendProgress(stageName, stageEnum); err != nil {
			return err
		}

		stageStart := time.Now()
		state.CurrentStage = stageEnum
		_ = SaveBackupState(state)

		if err := fn(); err != nil {
			return err
		}

		duration := time.Since(stageStart)
		state.MarkStageCompleted(stageEnum, duration)
		_ = SaveBackupState(state)
		return nil
	}

	// Stage 1: Import destination pool
	err := executeStage(StageImportPool, fmt.Sprintf("Importing %s pool", destPool), func() error {
		output.WriteString("-----------------------------------------------------------\n")
		output.WriteString("IMPORT POOL\n")
		output.WriteString("   Importing the external backup pool.\n")
		output.WriteString("-----------------------------------------------------------\n\n")

		imported, err := isPoolImported(destPool)
		if err != nil {
			return fmt.Errorf("failed to check pool status: %w", err)
		}

		if !imported {
			output.WriteString(fmt.Sprintf("Importing %s volume from USB drive\n", destPool))
			if err := runCommandWithContext(ctx, "zpool", "import", destPool); err != nil {
				return fmt.Errorf("failed to import pool: %w", err)
			}
		} else {
			output.WriteString(fmt.Sprintf("[OK] %s is already imported\n", destPool))
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 2: Load encryption key
	err = executeStage(StageLoadKey, "Loading encryption key", func() error {
		output.WriteString("-----------------------------------------------------------\n")
		output.WriteString("ENCRYPTION KEY\n")
		output.WriteString("   Loading the encryption key for the backup pool.\n")
		output.WriteString("-----------------------------------------------------------\n\n")

		keyStatus, err := getKeyStatus(destPool)
		if err != nil {
			return fmt.Errorf("failed to check key status: %w", err)
		}

		if keyStatus != "available" {
			output.WriteString(fmt.Sprintf("Loading encryption key (status: %s)\n", keyStatus))
			if err := loadZFSKey(destPool, password); err != nil {
				return fmt.Errorf("failed to load encryption key: %w", err)
			}
		} else {
			output.WriteString("[OK] Encryption key is already loaded\n")
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 3: Ensure hostname dataset exists
	err = executeStage(StageCreateSnapshot, fmt.Sprintf("Preparing %s namespace", hostname), func() error {
		output.WriteString("-----------------------------------------------------------\n")
		output.WriteString("HOSTNAME NAMESPACE\n")
		output.WriteString("   Creating hostname-based dataset namespace on the backup.\n")
		output.WriteString("   This allows multiple hosts to share one backup drive.\n")
		output.WriteString("-----------------------------------------------------------\n\n")

		// Check if the hostname dataset exists
		hostDataset := fmt.Sprintf("%s/%s", destPool, hostname)
		if _, err := runCommandOutput("zfs", "list", "-H", hostDataset); err != nil {
			output.WriteString(fmt.Sprintf("Creating namespace dataset: %s\n", hostDataset))
			if err := runCommandWithContext(ctx, "zfs", "create", "-p", hostDataset); err != nil {
				return fmt.Errorf("failed to create hostname dataset: %w", err)
			}
		} else {
			output.WriteString(fmt.Sprintf("[OK] Namespace %s already exists\n", hostDataset))
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 4: Remote sync via syncoid (all datasets)
	err = executeStage(StageSyncData, "Syncing data from remote host", func() error {
		output.WriteString("-----------------------------------------------------------\n")
		output.WriteString("REMOTE SYNC (using syncoid via SSH)\n")
		output.WriteString("   Pulling data from remote host using syncoid over SSH.\n")
		output.WriteString("   Only changes since the last backup are transferred.\n")
		output.WriteString("-----------------------------------------------------------\n\n")

		// Determine datasets to sync
		var datasetsToSync []string
		if strings.Contains(remoteDataset, "/") {
			// Specific dataset given (e.g., NIXROOT/home) - sync just this one
			datasetsToSync = []string{remoteDataset}
		} else {
			// Pool name given - discover all child datasets via SSH
			children, err := getRemoteChildDatasets(remoteHost, remotePool)
			if err != nil {
				output.WriteString(fmt.Sprintf("Warning: Could not discover remote datasets: %v\n", err))
				output.WriteString("Falling back to syncing pool root dataset\n")
				datasetsToSync = []string{remoteDataset}
			} else {
				for _, child := range children {
					datasetsToSync = append(datasetsToSync, fmt.Sprintf("%s/%s", remotePool, child))
				}
			}
		}

		if len(datasetsToSync) == 0 {
			output.WriteString("Warning: No datasets found to sync\n")
			return nil
		}

		output.WriteString(fmt.Sprintf("Syncing %d dataset(s) from %s\n\n", len(datasetsToSync), remoteHost))

		for i, ds := range datasetsToSync {
			// Extract suffix for destination path
			suffix := ds
			if idx := strings.Index(ds, "/"); idx >= 0 {
				suffix = ds[idx+1:]
			}

			syncDest := getHostnameDatasetPath(destPool, hostname, suffix)
			syncSrc := fmt.Sprintf("%s:%s", remoteHost, ds)

			output.WriteString(fmt.Sprintf("[Dataset %d/%d] %s -> %s\n", i+1, len(datasetsToSync), syncSrc, syncDest))

			if err := waitForZFSReceive(ctx, syncDest, &output); err != nil {
				return fmt.Errorf("error waiting for existing receive on %s: %w", ds, err)
			}

			if err := runCommandWithContext(ctx, "syncoid", "--create-bookmark", syncSrc, syncDest); err != nil {
				output.WriteString(fmt.Sprintf("Warning: syncoid failed for %s: %v (continuing...)\n", ds, err))
			} else {
				output.WriteString(fmt.Sprintf("[OK] %s synced successfully\n", ds))
			}
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 5: Export and power off
	err = executeStage(StageExportPool, "Exporting pool and powering off", func() error {
		output.WriteString("-----------------------------------------------------------\n")
		output.WriteString("EXPORT & POWER OFF\n")
		output.WriteString("   Safely exporting the pool and powering off the drive.\n")
		output.WriteString("-----------------------------------------------------------\n\n")

		device, err := getBackupDevice(destPool)
		if err == nil {
			if err := runCommandWithContext(ctx, "zpool", "export", destPool); err != nil {
				return fmt.Errorf("failed to export pool: %w", err)
			}

			output.WriteString(fmt.Sprintf("Powering off USB drive (%s)\n", device))
			if err := runCommandWithContext(ctx, "udisksctl", "power-off", "-b", device); err != nil {
				output.WriteString(fmt.Sprintf("Warning: failed to power off device: %v\n", err))
			}
		} else {
			output.WriteString("Warning: Skipping device power-off\n")
			if err := runCommandWithContext(ctx, "zpool", "export", destPool); err != nil {
				return fmt.Errorf("failed to export pool: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	output.WriteString("\n[OK] Remote backup completed successfully!")
	return output.String(), nil
}

func performPushBackup(ctx context.Context, password, sourcePool, remoteHost, remoteDestPool string, resumeFrom *BackupState, progressChan chan<- progressUpdate) (string, error) {
	var output strings.Builder

	if sourcePool == "" {
		return "", fmt.Errorf("source pool not selected")
	}
	if remoteHost == "" {
		return "", fmt.Errorf("remote host not specified")
	}
	if remoteDestPool == "" {
		return "", fmt.Errorf("remote destination pool not specified")
	}

	hostname := getLocalHostname()

	output.WriteString(fmt.Sprintf("Push backup: %s -> %s:%s/%s/\n\n", sourcePool, remoteHost, remoteDestPool, hostname))

	var state *BackupState
	if resumeFrom != nil {
		state = resumeFrom
		output.WriteString("[RESUME] Resuming push backup from previous session...\n\n")
	} else {
		state = NewBackupState("push-backup")
	}

	if err := SaveBackupState(state); err != nil {
		return "", fmt.Errorf("failed to save state: %w", err)
	}

	totalStages := 3
	currentStage := 1

	sendProgress := func(stage string, stageEnum BackupStage) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			output.WriteString(fmt.Sprintf("[%d/%d] %s\n", currentStage, totalStages, stage))
			if progressChan != nil {
				progressChan <- progressUpdate{
					stage:       stage,
					stageNum:    currentStage,
					totalStages: totalStages,
					state:       state,
				}
			}
			currentStage++
			return nil
		}
	}

	executeStage := func(stageEnum BackupStage, stageName string, fn func() error) error {
		if state.IsStageCompleted(stageEnum) {
			output.WriteString(fmt.Sprintf("Skipping completed stage: %s\n", stageName))
			currentStage++
			return nil
		}

		if err := sendProgress(stageName, stageEnum); err != nil {
			return err
		}

		stageStart := time.Now()
		state.CurrentStage = stageEnum
		_ = SaveBackupState(state)

		if err := fn(); err != nil {
			return err
		}

		duration := time.Since(stageStart)
		state.MarkStageCompleted(stageEnum, duration)
		_ = SaveBackupState(state)
		return nil
	}

	// Stage 1: Create recursive snapshot locally
	err := executeStage(StageCreateSnapshot, "Creating local snapshot", func() error {
		output.WriteString("-----------------------------------------------------------\n")
		output.WriteString("CREATE SNAPSHOT\n")
		output.WriteString("   Creating recursive snapshot of all local datasets.\n")
		output.WriteString("-----------------------------------------------------------\n\n")

		timestamp := time.Now().Format("2006-01-02.15h-04")
		snapshotTag := fmt.Sprintf("%s-Backup", timestamp)
		snapshotName := fmt.Sprintf("%s@%s", sourcePool, snapshotTag)
		state.SnapshotName = snapshotName
		_ = SaveBackupState(state)

		output.WriteString(fmt.Sprintf("Creating recursive snapshot: %s\n", snapshotName))
		if err := runCommandWithContext(ctx, "zfs", "snapshot", "-r", snapshotName); err != nil {
			return fmt.Errorf("failed to create snapshot: %w", err)
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 2: Push all datasets to remote via syncoid
	err = executeStage(StageSyncData, "Pushing data to remote host", func() error {
		output.WriteString("-----------------------------------------------------------\n")
		output.WriteString("PUSH SYNC (using syncoid via SSH)\n")
		output.WriteString("   Pushing local data to remote backup server over SSH.\n")
		output.WriteString("   Datasets are namespaced by local hostname on the remote.\n")
		output.WriteString("-----------------------------------------------------------\n\n")

		datasets, err := getChildDatasets(sourcePool)
		if err != nil {
			return fmt.Errorf("failed to list source datasets: %w", err)
		}

		if len(datasets) == 0 {
			output.WriteString("Warning: No child datasets found, nothing to sync\n")
			return nil
		}

		output.WriteString(fmt.Sprintf("Pushing %d dataset(s) to %s\n\n", len(datasets), remoteHost))

		for i, ds := range datasets {
			syncSrc := fmt.Sprintf("%s/%s", sourcePool, ds)
			// Push to remote: syncoid local_src user@host:DESTPOOL/hostname/dataset
			remoteDest := fmt.Sprintf("%s:%s/%s/%s", remoteHost, remoteDestPool, hostname, ds)

			output.WriteString(fmt.Sprintf("[Dataset %d/%d] %s -> %s\n", i+1, len(datasets), syncSrc, remoteDest))

			if err := runCommandWithContext(ctx, "syncoid", "--create-bookmark", syncSrc, remoteDest); err != nil {
				output.WriteString(fmt.Sprintf("Warning: syncoid failed for %s: %v (continuing...)\n", ds, err))
			} else {
				output.WriteString(fmt.Sprintf("[OK] %s pushed successfully\n", ds))
			}
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	// Stage 3: Prune local snapshots
	err = executeStage(StagePruneLocal, "Pruning local snapshots", func() error {
		output.WriteString("-----------------------------------------------------------\n")
		output.WriteString("PRUNE LOCAL SNAPSHOTS\n")
		output.WriteString("   Cleaning up old local snapshots to save space.\n")
		output.WriteString("-----------------------------------------------------------\n\n")

		if err := pruneOldLocalSnapshots(sourcePool); err != nil {
			output.WriteString(fmt.Sprintf("Warning: %v\n", err))
		}
		return nil
	})
	if err != nil {
		return output.String(), err
	}

	output.WriteString("\n[OK] Push backup completed successfully!")
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

func runCommandWithContext(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if cancelled
		if ctx.Err() != nil {
			return fmt.Errorf("operation cancelled")
		}
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

// isZFSReceiveRunning checks if there's an active zfs receive process for the given dataset
func isZFSReceiveRunning(dataset string) (bool, int, error) {
	// Use pgrep to find zfs receive processes
	output, err := runCommandOutput("pgrep", "-af", "zfs receive")
	if err != nil {
		// pgrep returns error if no processes found, which is fine
		return false, 0, nil
	}

	// Check if any of the processes are for our dataset
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, dataset) {
			// Extract PID from the line (first field)
			fields := strings.Fields(line)
			if len(fields) > 0 {
				var pid int
				fmt.Sscanf(fields[0], "%d", &pid)
				return true, pid, nil
			}
		}
	}

	return false, 0, nil
}

// waitForZFSReceive waits for an existing zfs receive process to complete
func waitForZFSReceive(ctx context.Context, dataset string, output *strings.Builder) error {
	running, pid, err := isZFSReceiveRunning(dataset)
	if err != nil {
		return err
	}

	if !running {
		return nil
	}

	output.WriteString(fmt.Sprintf("⏳ Found existing zfs receive process (PID %d) for %s\n", pid, dataset))
	output.WriteString("   Waiting for it to complete...\n")

	// Poll until the process completes or context is cancelled
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			running, _, err := isZFSReceiveRunning(dataset)
			if err != nil {
				return err
			}
			if !running {
				output.WriteString("   [OK]Previous receive process completed\n")
				return nil
			}
			output.WriteString("   Still waiting...\n")
		}
	}
}

func getKeyStatus(dataset string) (string, error) {
	output, err := runCommandOutput("zfs", "get", "-H", "-o", "value", "keystatus", dataset)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func getBackupDevice(poolName string) (string, error) {
	output, err := runCommandOutput("zpool", "status", poolName)
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

func pruneOldLocalSnapshots(sourcePool string) error {
	// Get snapshots older than 7 days
	output, err := runCommandOutput("zfs", "list", "-H", "-o", "name", "-t", "snapshot", "-S", "creation")
	if err != nil {
		return err
	}

	prefix := sourcePool + "/home@"
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var localSnapshots []string
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			localSnapshots = append(localSnapshots, line)
		}
	}

	// Keep first 7, bookmark and delete the rest
	if len(localSnapshots) > 7 {
		for _, snap := range localSnapshots[7:] {
			bookmark := strings.Replace(snap, "@", "#", 1)
			_ = runCommand("zfs", "bookmark", snap, bookmark)
			_ = runCommand("zfs", "destroy", snap)
		}
	}

	return nil
}

func pruneBackupSnapshots(destPool string) error {
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

	prefix := destPool + "/home@"
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, prefix) {
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

func generateBackupReport(sourcePool, destPool string) (string, error) {
	var report bytes.Buffer

	report.WriteString("📊 Backup Report Summary\n")
	report.WriteString(strings.Repeat("─", 50) + "\n")

	sourcePrefix := sourcePool + "/home@"
	destPrefix := destPool + "/home"

	// Get oldest snapshot
	output, err := runCommandOutput("zfs", "list", "-t", "snapshot", "-o", "name,creation", "-s", "creation")
	if err == nil {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, destPrefix) {
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
			if strings.HasPrefix(line, sourcePrefix) {
				localCount++
			}
		}
	}
	report.WriteString(fmt.Sprintf("• Snapshots on local: %d\n", localCount))

	// Count backup snapshots
	backupCount := 0
	destSnapshotPrefix := destPool + "/home@"
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, destSnapshotPrefix) {
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
	output, err = runCommandOutput("zfs", "list", "-H", "-o", "available", sourcePool)
	if err == nil {
		report.WriteString(fmt.Sprintf("• Free space on local: %s\n", strings.TrimSpace(output)))
	}

	output, err = runCommandOutput("zfs", "list", "-H", "-o", "available", destPool)
	if err == nil {
		report.WriteString(fmt.Sprintf("• Free space on backup: %s\n", strings.TrimSpace(output)))
	}

	return reportBoxStyle.Render(report.String()), nil
}
