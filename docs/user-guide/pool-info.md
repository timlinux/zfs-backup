# Pool Information

The Pool Information feature provides a comprehensive view of any ZFS pool's structure, health, and usage.

## Accessing Pool Information

From the main menu, select **Show zpool info**.

## Pool Selection

You'll be prompted to select which pool to view:

1. The tool shows all available pools (both imported and importable)
2. Use ++arrow-up++ / ++arrow-down++ or ++j++ / ++k++ to navigate
3. Press ++enter++ to select

!!! note "Unimported Pools"
    If you select a pool that isn't currently imported, the tool will automatically import it for you.

## Encryption Handling

If the selected pool is encrypted and locked, you'll be prompted for the encryption password before viewing the information.

## Information Displayed

The pool information view shows four sections:

### Status

Output from `zpool status`, including:

- Pool state (ONLINE, DEGRADED, etc.)
- Device structure (mirrors, raidz, etc.)
- Scrub/resilver status
- Any errors detected

### Usage

Output from `zpool list -v`, showing:

- Total size
- Allocated space
- Free space
- Fragmentation percentage
- Capacity percentage
- Individual device usage

### Datasets

All datasets within the pool:

- Name
- Used space
- Available space
- Referenced space
- Mountpoint

### Snapshots

All snapshots within the pool:

- Snapshot name
- Used space
- Referenced space
- Creation date

## Navigation

The information is displayed in a scrollable viewport:

| Key | Action |
|-----|--------|
| ++j++ / ++arrow-down++ | Scroll down |
| ++k++ / ++arrow-up++ | Scroll up |
| ++ctrl+d++ | Page down |
| ++ctrl+u++ | Page up |
| ++g++ | Go to top |
| ++shift+g++ | Go to bottom |
| ++escape++ / ++q++ | Return to menu |

The scroll percentage is shown at the bottom of the screen.

## Example Output

```
Pool: NIXROOT
======================================================================

STATUS
----------------------------------------------------------------------
  pool: NIXROOT
 state: ONLINE
  scan: scrub repaired 0B in 00:27:41 with 0 errors on Sun Mar  1 00:54:59 2026
config:

        NAME                                                 STATE
        NIXROOT                                              ONLINE
          raidz1-0                                           ONLINE
            nvme-eui.e8238fa6bf530001001b448b40f0effc-part2  ONLINE
            nvme-eui.e8238fa6bf530001001b444a41037afa-part2  ONLINE

errors: No known data errors

USAGE
----------------------------------------------------------------------
NAME      SIZE  ALLOC   FREE  FRAG   CAP
NIXROOT  1.82T  1.40T   371G    8%   77%

DATASETS
----------------------------------------------------------------------
NAME               USED  AVAIL  REFER  MOUNTPOINT
NIXROOT           1.40T   371G   192K  none
NIXROOT/home      1.19T   319G  1.17T  none
NIXROOT/nix        159G   141G   159G  /nix
NIXROOT/root      9.78G   371G  9.78G  /
```

---

Made with <3 by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/timlinux/zfs-backup)
