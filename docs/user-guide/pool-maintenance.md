# Pool Maintenance

The Pool Maintenance feature allows you to monitor pool health and control scrub operations for data integrity verification.

## What is a Scrub?

A ZFS scrub reads all data in the pool and verifies it against stored checksums. This process:

- Detects silent data corruption (bit rot)
- Automatically repairs corrupted data from redundant copies (mirrors, raidz)
- Reports any unrepairable errors

!!! tip "Regular Scrubs"
    It's recommended to run a scrub at least monthly, or weekly for critical data.

## Accessing Pool Maintenance

From the main menu, select **Pool Maintenance**.

## Pool Selection

1. Select the pool you want to maintain
2. Unimported pools will be automatically imported
3. Encrypted pools will prompt for password

## Maintenance View

The maintenance screen shows:

### Pool Status

Current state of the pool including any ongoing operations:

```
  pool: NIXROOT
 state: ONLINE
  scan: scrub in progress since Wed Mar  4 10:30:00 2026
        1.20T scanned at 150M/s, 800G issued at 100M/s
        0B repaired, 57.14% done, 01:30:00 to go
```

### Pool Health

Key metrics at a glance:

- Health status (ONLINE, DEGRADED, FAULTED)
- Total size
- Allocated space
- Free space
- Fragmentation percentage
- Capacity percentage

### Available Actions

The actions available depend on the current state:

**When no scrub is running:**

- `[s]` Start Scrub - Begin data integrity verification

**When a scrub is in progress:**

- `[x]` Stop Scrub - Cancel the current scrub

**Always available:**

- `[r]` Refresh - Update the status display
- `[q]` Return - Go back to main menu

## Keyboard Controls

| Key | Action |
|-----|--------|
| ++s++ | Start a scrub |
| ++x++ | Stop a scrub |
| ++r++ | Refresh status |
| ++j++ / ++k++ | Scroll up/down |
| ++ctrl+u++ / ++ctrl+d++ | Page up/down |
| ++escape++ / ++q++ | Return to menu |

## Scrub Progress

During a scrub, you'll see:

- Data scanned vs total
- Scan speed (MB/s)
- Percentage complete
- Estimated time remaining
- Any errors found

!!! warning "Stopping a Scrub"
    Stopping a scrub will require starting over from the beginning next time. The scrub will resume from the start, not from where it stopped.

## Resilver Operations

If a drive has been replaced or was temporarily offline, a resilver operation will run automatically. The maintenance view shows resilver progress the same way as scrub progress.

!!! danger "Don't Interrupt Resilvers"
    Unlike scrubs, resilvers are critical for restoring redundancy. Avoid stopping a resilver unless absolutely necessary.

## Example Session

```
Pool Maintenance: NIXROOT

======================================================================
Pool: NIXROOT
======================================================================

POOL STATUS
----------------------------------------------------------------------
  pool: NIXROOT
 state: ONLINE
  scan: scrub repaired 0B in 00:27:41 with 0 errors on Sun Mar  1 00:54:59 2026

POOL HEALTH
----------------------------------------------------------------------
  Health:        ONLINE
  Size:          1.82T
  Allocated:     1.40T
  Free:          371G
  Fragmentation: 8%
  Capacity:      77%


ACTIONS
----------------------------------------------------------------------
  [s] Start Scrub - Begin data integrity verification
  [r] Refresh     - Update the status display
  [q] Return      - Go back to main menu

──────────────────────────────────────────────────────────────────────
Scroll: j/k | 45% | s=scrub x=stop r=refresh q=return
```

---

Made with <3 by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/timlinux/zfs-backup)
