# ZFS Backup Tool - Specification

This document provides a complete technical treatment of the Kartoza ZFS Backup Tool including architecture, user stories, functional requirements, and testing requirements.

## Overview

A Terminal User Interface (TUI) application for managing ZFS backups to external drives, built with Go and the Charm libraries (Bubble Tea, Bubbles, Lipgloss).

## Architecture

### Component Diagram

```mermaid
graph TB
    subgraph "User Interface"
        TUI[Bubble Tea TUI]
        CLI[CLI Parser]
    end

    subgraph "Business Logic"
        BM[Backup Manager]
        SM[State Manager]
        PM[Progress Manager]
    end

    subgraph "ZFS Operations"
        ZO[ZFS Commands]
        SY[Syncoid Integration]
    end

    subgraph "External"
        ZFS[ZFS Filesystem]
        USB[USB Drive Control]
    end

    TUI --> BM
    CLI --> BM
    BM --> SM
    BM --> PM
    BM --> ZO
    BM --> SY
    ZO --> ZFS
    SY --> ZFS
    BM --> USB
```

### File Structure

| File | Purpose |
|------|---------|
| main.go | TUI application, views, state machine, and main logic |
| zfs.go | ZFS operations (backup, prepare, unmount) |
| state.go | Backup state management for resume functionality |
| restore.go | Restore mode with dual-panel file explorer |
| package.nix | Nix package definition |
| module.nix | NixOS module |
| flake.nix | Nix flake configuration |

### State Machine

```mermaid
stateDiagram-v2
    [*] --> Menu
    Menu --> PoolSelection: Select Backup
    PoolSelection --> Password: Pools Selected
    Password --> Running: Password Entered
    Running --> Result: Complete
    Running --> Result: Error
    Result --> Menu: Dismiss
    Menu --> Confirm: Destructive Op
    Confirm --> PoolSelection: Confirmed
    Confirm --> Menu: Cancelled
    Menu --> Restore: Select Restore
    Restore --> Menu: Done/Cancel
    Menu --> [*]: Quit
```

## User Stories

### US-001: Incremental Backup
**As a** system administrator
**I want to** perform incremental backups of my ZFS filesystems
**So that** I can efficiently protect my data with minimal storage and time overhead

**Acceptance Criteria:**
- Imports and unlocks the encrypted backup pool
- Creates timestamped snapshots
- Uses syncoid for incremental data transfer
- Prunes old snapshots automatically
- Generates backup health report
- Safely exports pool on completion

### US-002: Force Backup
**As a** system administrator
**I want to** perform a force backup when incremental chains are broken
**So that** I can reset the backup state and continue protecting my data

**Acceptance Criteria:**
- Requires explicit confirmation (destructive operation)
- Deletes existing snapshots on backup disk
- Performs full backup from current state
- Warns user about data loss implications

### US-003: Restore Files
**As a** user
**I want to** browse ZFS snapshots and restore individual files
**So that** I can recover specific files without restoring entire datasets

**Acceptance Criteria:**
- Dual-panel Midnight Commander-style interface
- Left panel shows snapshots and their contents
- Right panel shows filesystem for destination selection
- Navigate with vim/yazi keybindings (hjkl, g/G, Ctrl+u/d)
- Select files with spacebar
- Copy selected files with 'y' (yank)
- Preserve file ownership, permissions, and timestamps
- Two restore modes: original location or current folder
- Create directories in destination panel with 'm'

### US-004: Prepare Backup Device
**As a** system administrator
**I want to** prepare new external drives for encrypted ZFS backups
**So that** I can add new backup media to my rotation

**Acceptance Criteria:**
- Prompts for device path
- Requires double confirmation (destructive operation)
- Creates encrypted ZFS pool with AES-256-GCM
- Configures ZSTD compression
- Optimizes atime settings

### US-005: Safe Unmount
**As a** user
**I want to** safely unmount and power off backup drives
**So that** I can physically disconnect drives without data corruption

**Acceptance Criteria:**
- Exports ZFS pool properly
- Powers off USB drive
- Confirms successful completion

### US-007: View Pool Information
**As a** system administrator
**I want to** view detailed ZFS pool information
**So that** I can monitor pool health, structure, and usage

**Acceptance Criteria:**
- Prompt user to select which pool to view
- Import pool if not already imported
- Unlock encrypted pools if needed (prompt for password)
- Display zpool status (structure, state, health)
- Display zpool list with usage information
- Display all datasets with usage and mountpoints
- Display snapshots with usage and creation dates
- Scrollable viewport that respects header/footer bounds
- Keyboard navigation (j/k, arrows, page up/down)

### US-008: Pool Maintenance
**As a** system administrator
**I want to** perform maintenance operations on ZFS pools
**So that** I can ensure data integrity through regular scrubs

**Acceptance Criteria:**
- Prompt user to select which pool to maintain
- Import pool if not already imported
- Unlock encrypted pools if needed (prompt for password)
- Display current pool status including any ongoing scrub/resilver
- Start a new scrub with 's' key
- Stop an in-progress scrub with 'x' key
- Refresh status display with 'r' key
- Show pool health metrics (size, allocated, free, fragmentation)
- Scrollable viewport for detailed status information

### US-006: Resume Interrupted Backup
**As a** system administrator
**I want to** resume interrupted backups from where they stopped
**So that** I don't lose progress due to interruptions

**Acceptance Criteria:**
- State saved to ~/.cache/zfs-backup/backup-state.json
- Prompts to resume on startup if interrupted state exists
- Continues from the interrupted stage

## Functional Requirements

### FR-001: Main Menu Structure
The main menu shall display items in this order:
1. Backup ZFS (incremental)
2. Restore Files
3. Show zpool info
4. Pool Maintenance
5. Unmount Backup Disk
6. Help
7. Exit
8. --- Danger Zone ---
9. Prepare Backup Device
10. Force Backup ZFS (destructive)

Navigation skips the separator when using up/down keys.

### FR-002: Pool Selection
- Display all available ZFS pools
- Allow selecting source and destination pools
- Show pool names and encryption status
- Support both interactive and CLI modes

### FR-003: Password Handling
- Prompt for encryption passphrase when needed
- Mask password input
- Support separate passphrases for source and destination pools
- Validate pool unlock before proceeding

### FR-004: Progress Display
- Show current stage and total stages
- Display progress percentage where applicable
- Show spinner for indeterminate operations
- Display operation output in real-time

### FR-005: Restore File Browser
- Snapshot list with scrolling support
- File browser within snapshots
- ".." entry at top for parent navigation
- Sort modes: name, size, date
- Search functionality with '/'
- File selection with spacebar
- Multi-file selection support
- Directory creation with 'm' key

### FR-006: File Preservation
When restoring files:
- Preserve original UID/GID ownership
- Preserve original file permissions
- Preserve original modification timestamps
- Handle symlinks correctly (use lchown)

### FR-007: Keyboard Shortcuts

#### Main Menu
| Key | Action |
|-----|--------|
| ↑/k | Navigate up |
| ↓/j | Navigate down |
| Enter | Select option |
| ? | Show help |
| q | Quit application |
| Ctrl+C | Force quit |
| K | Open Kartoza website |
| O | Open Donate page |
| G | Open GitHub page |

#### Restore Mode
| Key | Action |
|-----|--------|
| h/l or Tab | Switch panels |
| j/k | Navigate up/down |
| g/G | Go to top/bottom |
| Ctrl+u/d | Page up/down |
| Enter | Enter directory/snapshot |
| Space | Toggle selection |
| y | Yank (copy) selected files |
| / | Search |
| s | Cycle sort mode |
| m | Create directory |
| u | Unmount and power off |
| ? | Show help |
| Esc | Go back one level |
| q | Return to menu |

### FR-008: Escape Navigation
- Escape should navigate back through states, not immediately exit
- From restore explorer: if in snapshot, go to snapshot list; else go to source selection
- From pool selection: return to menu
- From password entry: return to previous state
- Only exit application from top-level menu

### FR-009: CLI Mode
Support command-line flags for automation:
- `--backup`: Run incremental backup
- `--force-backup`: Run force backup (destructive)
- `--unmount`: Unmount backup disk
- `--help`: Show help

## Non-Functional Requirements

### NFR-001: Visual Design
- Kartoza brand colors (Gold #DF9E2F, Blue #569FC6, Teal #06969A, Red #CC0403)
- Responsive layout adapting to terminal size
- Fixed header and footer
- Scrollable content area
- Minimal, text-based UI without emojis for better terminal compatibility

### NFR-002: Error Handling
- Clear error messages displayed to user
- Errors saved to state for resume functionality
- Non-destructive operations recover gracefully

### NFR-003: Dependencies
- Go with Bubble Tea, Bubbles, Lipgloss
- ZFS utilities (zpool, zfs commands)
- syncoid (from sanoid package)
- udisks2 for USB drive control

## Testing Requirements

### TR-001: Unit Tests
- State management (save/load/clear)
- File operations (copy with attributes)
- Menu navigation logic

### TR-002: Integration Tests
- ZFS pool operations (mock or test pools)
- Backup workflow stages
- Restore file selection and copy

### TR-003: Manual Testing
- Full backup cycle on test system
- Restore individual files and verify attributes
- Device preparation on test drive
- Resume after interruption

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.2.0 | 2026-03 | Added "Pool Maintenance" with scrub control; fixed pool import/unlock flow; scrollable result reports |
| 1.1.0 | 2026 | Added "Show zpool info" feature; simplified UI by removing emojis |
| 1.0.0 | 2025 | Initial release with backup, restore, prepare, unmount |

---

Made with :heart: by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/kartoza/zfs-backup)
