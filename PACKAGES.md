# ZFS Backup Tool - Packages

This document provides an annotated list of all packages in the software architecture.

## Go Dependencies

### TUI Framework

| Package | Purpose |
|---------|---------|
| github.com/charmbracelet/bubbletea | Elm-inspired TUI framework providing the Model-Update-View architecture |
| github.com/charmbracelet/bubbles | Pre-built TUI components (spinner, progress bar, text input) |
| github.com/charmbracelet/bubbles/viewport | Scrollable viewport component for large content display |
| github.com/charmbracelet/lipgloss | Terminal styling and layout with CSS-like syntax |

### URL Handling

| Package | Purpose |
|---------|---------|
| github.com/skratchdot/open-golang/open | Cross-platform URL opening for external links (docs, GitHub, donate) |

## Standard Library Usage

| Package | Purpose |
|---------|---------|
| bufio | Buffered I/O for reading command output |
| context | Context management for cancellable operations |
| encoding/json | State file serialization/deserialization |
| fmt | Formatted I/O operations |
| io | Basic I/O interfaces for file copying |
| os | File operations, environment variables |
| os/exec | External command execution (zfs, zpool, syncoid) |
| os/user | Current user information for home directory |
| path/filepath | File path manipulation |
| regexp | Regular expression matching for output parsing |
| sort | Sorting file entries by name, size, date |
| strconv | String to number conversions |
| strings | String manipulation |
| syscall | Low-level system calls for file ownership (UID/GID) |
| time | Time handling for timestamps and snapshots |

## System Dependencies

| Package | Purpose | Source |
|---------|---------|--------|
| zfsutils | ZFS filesystem utilities (zfs, zpool commands) | NixOS/system package |
| sanoid | syncoid tool for efficient ZFS send/receive | NixOS/system package |
| udisks2 | USB drive power management (udisksctl) | NixOS/system package |
| coreutils | Basic utilities (cp, mkdir) | NixOS/system package |

## Nix Packages

| File | Purpose |
|------|---------|
| flake.nix | Nix flake configuration with inputs, outputs, and development shell |
| package.nix | Package definition with Go build instructions |
| module.nix | NixOS module for system integration |

## Development Dependencies

Available via `nix develop`:

| Package | Purpose |
|---------|---------|
| go | Go compiler and toolchain |
| gopls | Go language server for IDE integration |
| golangci-lint | Go linter for code quality |
| mkdocs | Documentation site generator |
| mkdocs-material | Material theme for mkdocs |

---

Made with :heart: by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/kartoza/zfs-backup)
