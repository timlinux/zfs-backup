# Restore Files

The Restore Files feature provides a dual-panel file explorer for browsing ZFS snapshots and restoring individual files or directories.

## Overview

The restore interface is inspired by classic file managers like Midnight Commander:

- **Left panel**: Browse snapshots and their contents
- **Right panel**: Browse the destination filesystem
- Navigate with vim-style keybindings
- Select multiple files and copy them to any location

## Accessing Restore Mode

From the main menu, select **Restore Files**.

## Pool Selection

1. **Select Source Pool**: Choose the pool containing the snapshots you want to restore from
2. **Enter Password**: If the pool is encrypted, enter the encryption password

## Interface Layout

```
┌─ Snapshots (NIXROOT) ─────────────────┬─ /home/user/restore ──────────────┐
│ ▶ autosnap_2026-03-04_00:00:00_daily │ ^ ..                               │
│   autosnap_2026-03-03_00:00:00_daily │ / Documents                        │
│   autosnap_2026-03-02_00:00:00_daily │ / Downloads                        │
│   autosnap_2026-03-01_00:00:00_daily │   file.txt              1.2 KB     │
│   autosnap_2026-02-28_00:00:00_daily │                                    │
│                                       │                                    │
└───────────────────────────────────────┴────────────────────────────────────┘
 0 files selected (0 B) │ Sort: Name ↑ │ Search:
```

## Navigation

### Panel Navigation

| Key | Action |
|-----|--------|
| ++tab++ or ++h++ / ++l++ | Switch between panels |
| ++j++ / ++arrow-down++ | Move down |
| ++k++ / ++arrow-up++ | Move up |
| ++g++ | Go to first item |
| ++shift+g++ | Go to last item |
| ++ctrl+u++ | Page up |
| ++ctrl+d++ | Page down |

### Browsing

| Key | Action |
|-----|--------|
| ++enter++ | Enter snapshot/directory |
| ++escape++ or ++period++ | Go to parent directory |
| ++slash++ | Search in current directory |

### File Operations

| Key | Action |
|-----|--------|
| ++space++ | Toggle file selection |
| ++y++ | Yank (copy) selected files |
| ++m++ | Create new directory (right panel) |

### Other

| Key | Action |
|-----|--------|
| ++s++ | Cycle sort mode (name/size/date) |
| ++u++ | Unmount source pool and return |
| ++question++ | Show help |
| ++q++ | Return to main menu |

## Restoring Files

### Step 1: Select a Snapshot

In the left panel, navigate to the snapshot you want to restore from and press ++enter++.

### Step 2: Navigate to Files

Browse into the snapshot's directory structure to find the files you want to restore.

### Step 3: Select Files

Press ++space++ to toggle selection on files or directories. Selected items show a checkmark (✓).

The status bar at the bottom shows:
- Number of files selected
- Total size of selected files

### Step 4: Choose Destination

Use ++tab++ to switch to the right panel and navigate to where you want to restore the files. You can create new directories with ++m++.

### Step 5: Copy Files

Press ++y++ to yank (copy) the selected files.

### Restore Mode Options

When you press ++y++, you'll be asked to choose:

1. **Restore to original location**: Files are restored to their original paths
2. **Restore to current folder**: Files are copied to the directory shown in the right panel

### File Attributes

When restoring files, the tool preserves:

- Original file permissions
- Original ownership (UID/GID)
- Original modification timestamps
- Symlinks (as symlinks, not as copied files)

!!! note "Permissions Required"
    You need appropriate permissions to set ownership. Run with `sudo` if restoring files owned by other users.

## Handling Existing Files

If files already exist at the destination, you'll be shown a confirmation dialog listing the conflicting files. Choose whether to:

- Overwrite existing files
- Cancel the operation

## Tips

!!! tip "Quick Navigation"
    Use ++g++ and ++shift+g++ to jump to the beginning and end of long lists.

!!! tip "Search"
    Press ++slash++ and type to filter files in the current directory.

!!! tip "Sorting"
    Press ++s++ to cycle through sort modes: Name, Size, Date.

---

Made with <3 by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/timlinux/zfs-backup)
