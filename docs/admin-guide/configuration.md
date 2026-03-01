# Configuration

This guide covers configuration options for Kartoza ZFS Backup Tool.

## Pool Structure

The tool works with any ZFS pool structure. When you run a backup, you'll be prompted to select:

1. **Source Pool** - The pool containing data to backup
2. **Destination Pool** - The external backup pool

### Example Structures

```
Source Pool (NIXROOT):
├── NIXROOT/home      ← Backed up
├── NIXROOT/root
└── NIXROOT/var

Destination Pool (NIXBACKUPS):
└── NIXBACKUPS/home   ← Backup destination
```

---

## Encryption

### Recommended Setup

We recommend using ZFS native encryption for backup pools:

```bash
# Create encrypted pool (done via "Prepare Backup Device" menu)
zpool create \
  -O encryption=aes-256-gcm \
  -O keyformat=passphrase \
  -O keylocation=prompt \
  -O compression=zstd \
  -O atime=off \
  NIXBACKUPS /dev/sdX
```

### Encryption Properties

| Property | Value | Description |
|----------|-------|-------------|
| encryption | aes-256-gcm | Strong authenticated encryption |
| keyformat | passphrase | Password-based key |
| keylocation | prompt | Ask for password on load |

---

## Retention Policy

### Local Snapshots

By default, the tool keeps the **last 7 snapshots** on your local system. Older snapshots are converted to bookmarks.

### Backup Snapshots

The backup retention policy keeps:

- All recent snapshots
- Monthly snapshots for the last **3 months**

### Customizing Retention

Currently, retention policies are hardcoded. Future versions will support configuration via:

- Command-line flags
- Configuration file
- Environment variables

---

## CLI Mode

For automation and scripting, use CLI mode:

```bash
# Run incremental backup
sudo zfs-backup --backup

# Force backup (destructive)
sudo zfs-backup --force-backup

# Unmount backup disk
sudo zfs-backup --unmount

# Show help
zfs-backup --help
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ZFS_BACKUP_SOURCE` | Default source pool (future) |
| `ZFS_BACKUP_DEST` | Default destination pool (future) |

---

## Systemd Integration

### Creating a Backup Service

```ini
# /etc/systemd/system/zfs-backup.service
[Unit]
Description=ZFS Backup
After=local-fs.target

[Service]
Type=oneshot
ExecStart=/usr/bin/zfs-backup --backup
StandardInput=tty
TTYPath=/dev/tty1

[Install]
WantedBy=multi-user.target
```

### Creating a Timer

```ini
# /etc/systemd/system/zfs-backup.timer
[Unit]
Description=Run ZFS backup weekly

[Timer]
OnCalendar=weekly
Persistent=true

[Install]
WantedBy=timers.target
```

!!! warning "Interactive Password"
    The backup requires an encryption password. For fully automated backups, consider using a keyfile instead of passphrase.

---

## Logging

The tool maintains a state file for resume functionality:

```
~/.cache/zfs-backup/backup-state.json
```

This file contains:

- Current operation
- Completed stages
- Stage timings
- Snapshot names

### Clearing State

If you need to clear the resume state:

```bash
rm ~/.cache/zfs-backup/backup-state.json
```

---

## Troubleshooting

### Permission Denied

If you see permission errors, ensure you're running with sudo:

```bash
sudo zfs-backup
```

Or configure [ZFS Delegation](zfs-delegation.md).

### Pool Not Found

If your external drive's pool isn't detected:

1. Ensure the drive is connected
2. Run with sudo for pool scanning
3. Check if the pool needs to be imported manually:
   ```bash
   sudo zpool import
   ```

### Sync Already in Progress

If you see "already target of a zfs receive process":

1. Wait for the existing process to complete, OR
2. Kill the existing process:
   ```bash
   sudo pkill -f "zfs receive"
   ```

---

Made with :heart: by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/kartoza/zfs-backup)
