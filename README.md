# Kartoza ZFS Backup Tool

A beautiful TUI (Terminal User Interface) for managing ZFS backups, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss).

[![Release](https://img.shields.io/github/v/release/timlinux/zfs-backup)](https://github.com/timlinux/zfs-backup/releases)
[![License](https://img.shields.io/github/license/timlinux/zfs-backup)](LICENSE)
[![Documentation](https://img.shields.io/badge/docs-online-blue)](https://timlinux.github.io/zfs-backup/)

📖 **[Full Documentation](https://timlinux.github.io/zfs-backup/)**

## Features

- **Incremental Backups** - Efficient snapshots with syncoid integration
- **Force Backup** - Destructive backup option for out-of-sync scenarios
- **Restore Files** - Dual-panel file explorer to browse snapshots and restore files
- **Pool Information** - View detailed pool structure, health, datasets, and snapshots
- **Pool Maintenance** - Start, stop, and monitor scrub operations
- **Device Preparation** - Create encrypted ZFS pools with AES-256-GCM
- **Safe Unmounting** - Properly export pools and power off USB drives
- **CLI Mode** - Command-line arguments for automation and scripting

## Quick Start

### Install

```bash
# Download binary (Linux x86_64)
curl -L https://github.com/timlinux/zfs-backup/releases/latest/download/zfs-backup-linux-amd64 -o zfs-backup
chmod +x zfs-backup
sudo mv zfs-backup /usr/local/bin/

# Or with Nix
nix run github:timlinux/zfs-backup
```

See [Installation Guide](https://timlinux.github.io/zfs-backup/admin-guide/installation/) for more options including NixOS, Arch (AUR), Debian, Fedora, Snap, and Flatpak.

### Run

```bash
# Interactive mode
sudo zfs-backup

# CLI mode
sudo zfs-backup --backup      # Run incremental backup
sudo zfs-backup --unmount     # Safely unmount backup drive
sudo zfs-backup --help        # Show help
```

## Requirements

- Linux with ZFS filesystem
- [syncoid](https://github.com/jimsalterjrs/sanoid) (from sanoid package)
- Root privileges or ZFS delegation configured
- External drive with encrypted ZFS pool (for backups)

## Menu Options

| Option | Description |
|--------|-------------|
| Backup ZFS (incremental) | Run efficient incremental backup using syncoid |
| Restore Files | Browse snapshots and restore individual files |
| Show zpool info | View pool structure, health, datasets, and snapshots |
| Pool Maintenance | Start/stop scrubs, monitor pool health |
| Unmount Backup Disk | Safely export pool and power off USB drive |
| Prepare Backup Device | Create new encrypted ZFS pool on external drive |
| Force Backup (destructive) | Reset backup when incremental chain is broken |

## Keyboard Shortcuts

### Navigation
| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate |
| `Enter` | Select |
| `Esc` | Go back |
| `q` | Quit |

### Scrollable Views (Pool Info, Maintenance, Results)
| Key | Action |
|-----|--------|
| `j/k` | Scroll line |
| `Ctrl+u/d` | Page up/down |
| `g/G` | Top/bottom |

### Restore Mode
| Key | Action |
|-----|--------|
| `Tab` or `h/l` | Switch panels |
| `Space` | Toggle selection |
| `y` | Copy selected files |
| `/` | Search |
| `m` | Create directory |

See [Full Keyboard Reference](https://timlinux.github.io/zfs-backup/user-guide/keyboard-shortcuts/).

## Documentation

- [Getting Started](https://timlinux.github.io/zfs-backup/user-guide/getting-started/)
- [Backup Operations](https://timlinux.github.io/zfs-backup/user-guide/backup-operations/)
- [Restore Files](https://timlinux.github.io/zfs-backup/user-guide/restore-files/)
- [Pool Information](https://timlinux.github.io/zfs-backup/user-guide/pool-info/)
- [Pool Maintenance](https://timlinux.github.io/zfs-backup/user-guide/pool-maintenance/)
- [Installation Guide](https://timlinux.github.io/zfs-backup/admin-guide/installation/)
- [ZFS Delegation](https://timlinux.github.io/zfs-backup/admin-guide/zfs-delegation/)

## Architecture

```
zfs-backup/
├── main.go       # Bubble Tea TUI and main application logic
├── zfs.go        # ZFS operations (backup, prepare, unmount)
├── state.go      # Backup state management for resume
├── restore.go    # Restore mode with dual-panel explorer
├── flake.nix     # Nix flake configuration
└── docs/         # MkDocs documentation
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

See [Contributing Guide](https://timlinux.github.io/zfs-backup/developer-guide/contributing/).

## License

MIT License - see [LICENSE](LICENSE) file for details.

---

Made with <3 by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/timlinux/zfs-backup)
