package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// BackupStage represents a stage in the backup process
type BackupStage string

const (
	StageImportPool     BackupStage = "import_pool"
	StageLoadKey        BackupStage = "load_key"
	StageCreateSnapshot BackupStage = "create_snapshot"
	StageSyncData       BackupStage = "sync_data"
	StagePruneLocal     BackupStage = "prune_local"
	StagePruneBackup    BackupStage = "prune_backup"
	StageExportPool     BackupStage = "export_pool"
	StagePowerOff       BackupStage = "power_off"
)

// BackupState represents the current state of a backup operation
type BackupState struct {
	Operation      string                 `json:"operation"`       // "backup" or "force-backup"
	StartTime      time.Time              `json:"start_time"`
	CompletedStages map[BackupStage]bool  `json:"completed_stages"`
	CurrentStage   BackupStage            `json:"current_stage"`
	SnapshotName   string                 `json:"snapshot_name,omitempty"`
	Cancelled      bool                   `json:"cancelled"`
	LastUpdate     time.Time              `json:"last_update"`
	StageTimings   map[BackupStage]time.Duration `json:"stage_timings"` // Historical timings
}

// getStateFilePath returns the path to the state file
func getStateFilePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		// Fallback to home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cacheDir = filepath.Join(home, ".cache")
	}

	appDir := filepath.Join(cacheDir, "zfs-backup")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", err
	}

	return filepath.Join(appDir, "backup-state.json"), nil
}

// SaveBackupState saves the current backup state to disk
func SaveBackupState(state *BackupState) error {
	statePath, err := getStateFilePath()
	if err != nil {
		return err
	}

	state.LastUpdate = time.Now()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statePath, data, 0644)
}

// LoadBackupState loads the backup state from disk
func LoadBackupState() (*BackupState, error) {
	statePath, err := getStateFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No state file exists
		}
		return nil, err
	}

	var state BackupState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// ClearBackupState removes the state file
func ClearBackupState() error {
	statePath, err := getStateFilePath()
	if err != nil {
		return err
	}

	err = os.Remove(statePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// NewBackupState creates a new backup state
func NewBackupState(operation string) *BackupState {
	return &BackupState{
		Operation:       operation,
		StartTime:       time.Now(),
		CompletedStages: make(map[BackupStage]bool),
		StageTimings:    make(map[BackupStage]time.Duration),
		LastUpdate:      time.Now(),
	}
}

// IsStageCompleted checks if a stage has been completed
func (s *BackupState) IsStageCompleted(stage BackupStage) bool {
	return s.CompletedStages[stage]
}

// MarkStageCompleted marks a stage as completed
func (s *BackupState) MarkStageCompleted(stage BackupStage, duration time.Duration) {
	s.CompletedStages[stage] = true
	s.StageTimings[stage] = duration
}

// GetProgress returns the current progress (0-100)
func (s *BackupState) GetProgress(totalStages int) float64 {
	if totalStages == 0 {
		return 0
	}
	return float64(len(s.CompletedStages)) / float64(totalStages) * 100
}

// EstimateTimeRemaining estimates time remaining based on average stage duration
func (s *BackupState) EstimateTimeRemaining(totalStages int) time.Duration {
	if len(s.CompletedStages) == 0 {
		return 0
	}

	elapsed := time.Since(s.StartTime)
	avgTimePerStage := elapsed / time.Duration(len(s.CompletedStages))
	remainingStages := totalStages - len(s.CompletedStages)

	return avgTimePerStage * time.Duration(remainingStages)
}
