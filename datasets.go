// SPDX-FileCopyrightText: Tim Sutton / Kartoza
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// =============================================================================
// Backup scope - the single source of truth for which datasets we touch
// =============================================================================
//
// THE SNAPSHOT SCOPE INVARIANT
//
//	zfs-backup must never create a snapshot on a dataset it is not going to
//	replicate and subsequently prune.
//
// Every phase of a run - snapshot, replicate, prune, bookmark - is driven from
// the one list returned by resolveBackupDatasets. Nothing else may derive its
// own dataset list, and no phase may use `zfs snapshot -r`: a recursive
// snapshot covers the pool root and nested descendants that the replication
// and prune phases never visit, so those snapshots accumulate forever and
// silently consume the dataset's quota.

// PoolScope records which direct children of a pool zfs-backup may touch.
type PoolScope struct {
	// Datasets holds direct-child suffixes, e.g. ["home", "atuin"].
	// An empty slice means "every direct child of the pool".
	Datasets []string `json:"datasets"`
}

// BackupScope is the on-disk scope configuration, keyed by pool name.
type BackupScope struct {
	Pools map[string]PoolScope `json:"pools"`
}

// scopeFileName is the config file holding the backup scope.
const scopeFileName = "scope.json"

// getScopeFilePath returns the path to the backup scope config file.
func getScopeFilePath() (string, error) {
	dir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, scopeFileName), nil
}

// LoadBackupScope reads the saved backup scope. A missing file is not an
// error: it simply means no pool has been narrowed and every direct child of
// each pool is in scope.
func LoadBackupScope() (*BackupScope, error) {
	scopePath, err := getScopeFilePath()
	if err != nil {
		return &BackupScope{Pools: map[string]PoolScope{}}, err
	}

	data, err := os.ReadFile(scopePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &BackupScope{Pools: map[string]PoolScope{}}, nil
		}
		return &BackupScope{Pools: map[string]PoolScope{}}, err
	}

	var scope BackupScope
	if err := json.Unmarshal(data, &scope); err != nil {
		return &BackupScope{Pools: map[string]PoolScope{}}, err
	}
	if scope.Pools == nil {
		scope.Pools = map[string]PoolScope{}
	}

	return &scope, nil
}

// SaveBackupScope writes the backup scope to disk.
func SaveBackupScope(scope *BackupScope) error {
	scopePath, err := getScopeFilePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(scope, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(scopePath, data, 0644); err != nil {
		return err
	}
	chownToRealUser(scopePath)

	return nil
}

// SetPoolScope restricts a pool to the given direct-child dataset suffixes.
// Passing an empty list clears the restriction, returning the pool to "every
// direct child".
func SetPoolScope(pool string, datasets []string) error {
	scope, err := LoadBackupScope()
	if err != nil {
		return err
	}

	if len(datasets) == 0 {
		delete(scope.Pools, pool)
	} else {
		cleaned := make([]string, 0, len(datasets))
		seen := map[string]bool{}
		for _, ds := range datasets {
			ds = strings.TrimSpace(strings.Trim(ds, "/"))
			if ds == "" || seen[ds] {
				continue
			}
			seen[ds] = true
			cleaned = append(cleaned, ds)
		}
		sort.Strings(cleaned)
		scope.Pools[pool] = PoolScope{Datasets: cleaned}
	}

	return SaveBackupScope(scope)
}

// applyScope intersects the datasets that actually exist under a pool with the
// configured selection. It returns the datasets to process (in the order they
// appear on disk) and any configured datasets that no longer exist, so the
// caller can warn about a stale configuration. A nil or empty selection means
// "all available datasets".
//
// This is the one place the canonical list is decided; keep it pure so it can
// be tested without a pool.
func applyScope(available, configured []string) (selected, missing []string) {
	if len(configured) == 0 {
		return append([]string(nil), available...), nil
	}

	wanted := make(map[string]bool, len(configured))
	for _, ds := range configured {
		wanted[ds] = true
	}

	present := make(map[string]bool, len(available))
	for _, ds := range available {
		present[ds] = true
		if wanted[ds] {
			selected = append(selected, ds)
		}
	}

	for _, ds := range configured {
		if !present[ds] {
			missing = append(missing, ds)
		}
	}

	return selected, missing
}

// resolveBackupDatasets returns the canonical list of dataset suffixes that
// every phase of a backup run must use, together with any configured datasets
// that are missing from the pool.
//
// Callers MUST use this list for snapshotting as well as replication and
// pruning - see the snapshot scope invariant above.
func resolveBackupDatasets(pool string) (selected, missing []string, err error) {
	available, err := getChildDatasets(pool)
	if err != nil {
		return nil, nil, err
	}

	scope, scopeErr := LoadBackupScope()
	if scopeErr != nil {
		// A broken or unreadable scope file must not silently widen the scope
		// to the whole pool, so surface it.
		return nil, nil, fmt.Errorf("failed to read backup scope: %w", scopeErr)
	}

	poolScope, configured := scope.Pools[pool]
	if !configured {
		return available, nil, nil
	}

	selected, missing = applyScope(available, poolScope.Datasets)
	return selected, missing, nil
}

// resolveRemoteBackupDatasets is the remote-host counterpart of
// resolveBackupDatasets, used by the pull-from-remote flow.
func resolveRemoteBackupDatasets(sshHost, pool string) (selected, missing []string, err error) {
	available, err := getRemoteChildDatasets(sshHost, pool)
	if err != nil {
		return nil, nil, err
	}

	scope, scopeErr := LoadBackupScope()
	if scopeErr != nil {
		return nil, nil, fmt.Errorf("failed to read backup scope: %w", scopeErr)
	}

	poolScope, configured := scope.Pools[sshHost+":"+pool]
	if !configured {
		return available, nil, nil
	}

	selected, missing = applyScope(available, poolScope.Datasets)
	return selected, missing, nil
}

// qualifyDatasets turns dataset suffixes into fully qualified dataset names.
func qualifyDatasets(pool string, datasets []string) []string {
	qualified := make([]string, 0, len(datasets))
	for _, ds := range datasets {
		qualified = append(qualified, fmt.Sprintf("%s/%s", pool, ds))
	}
	return qualified
}

// describeScope renders a one-line human-readable summary of the scope, for
// the run log and the doctor report.
func describeScope(pool string, selected, missing []string) string {
	var b strings.Builder
	if len(selected) == 0 {
		b.WriteString(fmt.Sprintf("No datasets in scope for %s", pool))
	} else {
		b.WriteString(fmt.Sprintf("Datasets in scope for %s: %s", pool, strings.Join(selected, ", ")))
	}
	if len(missing) > 0 {
		b.WriteString(fmt.Sprintf(" (configured but missing: %s)", strings.Join(missing, ", ")))
	}
	return b.String()
}
