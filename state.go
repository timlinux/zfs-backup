package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// =============================================================================
// Remote Host Profiles
// =============================================================================

// RemoteHost represents a saved remote host connection profile
type RemoteHost struct {
	Name    string `json:"name"`    // Display name (e.g., "Office Server")
	SSHHost string `json:"ssh_host"` // SSH connection string (user@host)
	Dataset string `json:"dataset"`  // Remote dataset (e.g., NIXROOT/home)
}

// RemoteHostConfig holds all saved remote host profiles
type RemoteHostConfig struct {
	Hosts []RemoteHost `json:"hosts"`
}

// getConfigDir returns the config directory path, creating it if needed
func getConfigDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(home, ".config")
	}

	appDir := filepath.Join(configDir, "zfs-backup")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", err
	}

	return appDir, nil
}

// getHostsFilePath returns the path to the hosts config file
func getHostsFilePath() (string, error) {
	dir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hosts.json"), nil
}

// LoadRemoteHosts loads saved remote host profiles from disk
func LoadRemoteHosts() (*RemoteHostConfig, error) {
	hostsPath, err := getHostsFilePath()
	if err != nil {
		return &RemoteHostConfig{}, err
	}

	data, err := os.ReadFile(hostsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &RemoteHostConfig{}, nil
		}
		return &RemoteHostConfig{}, err
	}

	var config RemoteHostConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return &RemoteHostConfig{}, err
	}

	return &config, nil
}

// SaveRemoteHosts saves remote host profiles to disk
func SaveRemoteHosts(config *RemoteHostConfig) error {
	hostsPath, err := getHostsFilePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(hostsPath, data, 0644)
}

// AddRemoteHost adds a new remote host profile (or updates existing by SSHHost)
func AddRemoteHost(sshHost, dataset string) error {
	config, err := LoadRemoteHosts()
	if err != nil {
		config = &RemoteHostConfig{}
	}

	// Check if this host already exists - update it
	for i, h := range config.Hosts {
		if h.SSHHost == sshHost {
			config.Hosts[i].Dataset = dataset
			return SaveRemoteHosts(config)
		}
	}

	// Extract a display name from the SSH host
	name := sshHost
	if parts := strings.Split(sshHost, "@"); len(parts) > 1 {
		name = parts[1]
	}

	config.Hosts = append(config.Hosts, RemoteHost{
		Name:    name,
		SSHHost: sshHost,
		Dataset: dataset,
	})

	return SaveRemoteHosts(config)
}

// RemoveRemoteHost removes a remote host profile by index
func RemoveRemoteHost(index int) error {
	config, err := LoadRemoteHosts()
	if err != nil {
		return err
	}

	if index < 0 || index >= len(config.Hosts) {
		return nil
	}

	config.Hosts = append(config.Hosts[:index], config.Hosts[index+1:]...)
	return SaveRemoteHosts(config)
}

// =============================================================================
// Backup State
// =============================================================================

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
