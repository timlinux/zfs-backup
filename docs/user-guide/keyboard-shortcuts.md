# Keyboard Shortcuts

Quick reference for all keyboard shortcuts in Kartoza ZFS Backup Tool.

## Global Shortcuts

| Key | Action |
|-----|--------|
| ++q++ | Quit application |
| ++ctrl+c++ | Force quit / Cancel operation |
| ++question++ | Show help |

## Navigation

| Key | Action |
|-----|--------|
| ++arrow-up++ / ++k++ | Move up |
| ++arrow-down++ / ++j++ | Move down |
| ++enter++ | Select / Confirm |
| ++escape++ | Go back / Cancel |

## Confirmation Dialogs

| Key | Action |
|-----|--------|
| ++y++ | Yes / Confirm |
| ++n++ | No / Cancel |
| ++escape++ | Cancel and go back |

## During Operations

| Key | Action |
|-----|--------|
| ++ctrl+c++ | Cancel operation (resumable) |

!!! tip "Resumable Operations"
    If you cancel a backup with ++ctrl+c++, your progress is saved. The next time you start a backup, you'll be prompted to resume from where you left off.

## Footer Links

| Key | Action |
|-----|--------|
| ++shift+k++ | Open Kartoza website |
| ++shift+o++ | Open Donate page |
| ++shift+g++ | Open GitHub repository |

## Vim-Style Navigation

The tool supports vim-style navigation:

| Vim Key | Equivalent |
|---------|------------|
| ++j++ | ++arrow-down++ |
| ++k++ | ++arrow-up++ |

## Pool Info / Maintenance Shortcuts

In the scrollable information views:

| Key | Action |
|-----|--------|
| ++j++ / ++arrow-down++ | Scroll down |
| ++k++ / ++arrow-up++ | Scroll up |
| ++ctrl+d++ | Page down |
| ++ctrl+u++ | Page up |
| ++g++ | Go to top |
| ++shift+g++ | Go to bottom |
| ++escape++ / ++q++ | Return to menu |

### Maintenance-Specific

| Key | Action |
|-----|--------|
| ++s++ | Start scrub |
| ++x++ | Stop scrub |
| ++r++ | Refresh status |

## Restore Mode Shortcuts

In the dual-panel restore explorer, additional shortcuts are available:

### Panel Navigation

| Key | Action |
|-----|--------|
| ++h++ / ++arrow-left++ | Focus left panel |
| ++l++ / ++arrow-right++ | Focus right panel |
| ++tab++ | Switch panel focus |
| ++enter++ | Enter directory/snapshot |
| ++backspace++ / ++-++ | Go up / back |

### File Selection

| Key | Action |
|-----|--------|
| ++space++ | Toggle file selection |
| ++a++ | Select all files |
| ++c++ | Clear selection |
| ++y++ | Yank (copy) selected files |

### Browse & Sort

| Key | Action |
|-----|--------|
| ++slash++ | Search files |
| ++s++ | Cycle sort mode (Name/Date/Size) |
| ++r++ | Reverse sort order |
| ++g++ | Go to top |
| ++shift+g++ | Go to bottom |
| ++ctrl+u++ | Page up |
| ++ctrl+d++ | Page down |

### Exit

| Key | Action |
|-----|--------|
| ++u++ | Unmount and power off |
| ++q++ / ++escape++ | Return to menu |

---

Made with :heart: by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/kartoza/zfs-backup)
