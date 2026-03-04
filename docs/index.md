# Kartoza ZFS Backup Tool

A beautiful TUI (Terminal User Interface) for managing ZFS backups, built with Go and the Charm libraries.

![Kartoza ZFS Backup Screenshot](assets/images/kartoza-zfs-backup.png)

## Overview

Kartoza ZFS Backup Tool provides an intuitive terminal interface for managing ZFS backups to external drives. It leverages the power of ZFS snapshots and incremental send/receive to efficiently backup your data while keeping you informed about what's happening at every step.

## Key Features

<div class="grid cards" markdown>

-   :material-package-variant: **Incremental Backups**

    ---

    Efficient snapshots with syncoid integration. Only changed data is transferred, making backups fast and bandwidth-efficient.

-   :material-fire: **Force Backup**

    ---

    Destructive backup option for when incremental chains are broken. Resets the backup to match your current source state.

-   :material-download: **Restore Files**

    ---

    Dual-panel file explorer for browsing snapshots and restoring files. Vim/yazi-style keybindings for efficient navigation.

-   :material-information: **Pool Information**

    ---

    View comprehensive pool details including structure, health, datasets, and snapshots in a scrollable display.

-   :material-wrench: **Pool Maintenance**

    ---

    Start, stop, and monitor scrub operations for data integrity verification with real-time progress.

-   :material-shield-lock: **Device Preparation**

    ---

    Create encrypted ZFS pools with AES-256-GCM on new external drives with a simple guided workflow.

-   :material-power-plug: **Safe Unmounting**

    ---

    Properly export pools and power off USB drives to prevent data corruption.

-   :material-console: **CLI Mode**

    ---

    Command-line arguments for automation and scripting in headless environments.

</div>

## Quick Start

```bash
# Run with sudo for full ZFS access
sudo zfs-backup

# Or use CLI mode for automation
sudo zfs-backup --backup
```

## How It Works

The tool uses ZFS's built-in snapshot and send/receive capabilities to create efficient, incremental backups:

1. **Snapshots** capture the exact state of your data at a point in time
2. **Incremental sends** transfer only the changes since the last backup
3. **Bookmarks** replace old local snapshots to save space while maintaining the backup chain
4. **Encryption** keeps your backup data secure at rest

## Requirements

- Linux with ZFS filesystem
- At least one ZFS pool (source)
- External drive with encrypted ZFS pool (destination)
- [syncoid](https://github.com/jimsalterjrs/sanoid) from the sanoid package
- Root privileges or ZFS delegation configured

## Support

Having issues? Check the [User Guide](user-guide/getting-started.md) or open an issue on [GitHub](https://github.com/kartoza/zfs-backup/issues).

---

Made with :heart: by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/kartoza/zfs-backup)
