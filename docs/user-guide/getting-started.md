# Getting Started

This guide will help you get up and running with Kartoza ZFS Backup Tool.

## Prerequisites

Before using this tool, ensure you have:

- A Linux system with ZFS installed
- At least one ZFS pool configured as your source (e.g., your main system pool)
- An external drive prepared with an encrypted ZFS pool (or use the tool to prepare one)
- The `syncoid` utility installed (from the sanoid package)

## Launching the Application

### Interactive Mode (Recommended)

Simply run the application with sudo:

```bash
sudo zfs-backup
```

You'll be greeted with the main menu:

![Main Menu](../assets/images/kartoza-zfs-backup.png)

### Navigation

Use these keys to navigate:

| Key | Action |
|-----|--------|
| ++arrow-up++ / ++k++ | Move up |
| ++arrow-down++ / ++j++ | Move down |
| ++enter++ | Select option |
| ++escape++ | Go back |
| ++q++ | Quit |

## Your First Backup

### Step 1: Select Backup Operation

From the main menu, select **Backup ZFS (incremental)**.

### Step 2: Choose Source Pool

The tool will scan for available ZFS pools and prompt you to select a source pool. This is the pool containing the data you want to backup.

### Step 3: Choose Destination Pool

Next, select the destination pool. This should be your external backup drive. The tool will show both imported pools and pools that can be imported from external drives.

!!! tip "External Drives"
    If your external drive isn't showing, make sure it's connected and try running with `sudo` to allow pool detection.

### Step 4: Enter Encryption Password

If your destination pool is encrypted (recommended), you'll be prompted for the encryption password.

### Step 5: Watch the Progress

The backup will proceed through several stages:

1. **Import Pool** - Makes the external drive's pool available
2. **Load Encryption Key** - Unlocks the encrypted pool
3. **Create Snapshot** - Captures current state of your data
4. **Sync Data** - Transfers changes to backup drive
5. **Prune Local Snapshots** - Converts old snapshots to bookmarks
6. **Prune Backup Snapshots** - Maintains retention policy
7. **Export & Power Off** - Safely ejects the drive

Each stage includes explanatory text so you understand what's happening.

## After the Backup

Once complete, you'll see a summary report showing:

- Oldest and newest snapshots
- Number of snapshots on local and backup
- Free space remaining

The external drive will be safely powered off and you can physically disconnect it.

## Next Steps

- Learn about [Backup Operations](backup-operations.md) in detail
- Review [Keyboard Shortcuts](keyboard-shortcuts.md)
- Set up [ZFS Delegation](../admin-guide/zfs-delegation.md) to run without sudo

---

Made with :heart: by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/kartoza/zfs-backup)
