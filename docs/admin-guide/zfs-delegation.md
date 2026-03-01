# ZFS Delegation

This guide explains how to configure ZFS delegation to run backups without sudo.

## Overview

By default, ZFS operations require root privileges. ZFS delegation allows you to grant specific permissions to non-root users.

!!! warning "Limitations"
    Even with delegation, some operations (like pool import/export) may still require root. For most users, running with `sudo` is simpler.

## Configuring Delegation

### Step 1: Enable Delegation on Pools

```bash
sudo zpool set delegation=on NIXROOT
sudo zpool set delegation=on NIXBACKUPS
```

### Step 2: Grant Permissions on Source Pool

```bash
sudo zfs allow -u $USER \
  snapshot,send,hold,release,mount,destroy \
  NIXROOT
```

### Step 3: Grant Permissions on Backup Pool

```bash
sudo zfs allow -u $USER \
  snapshot,receive,mount,create,destroy,load-key,change-key \
  NIXBACKUPS
```

## Permission Reference

### Required for Backup

| Permission | Purpose |
|------------|---------|
| snapshot | Create snapshots |
| send | Send snapshot streams |
| hold | Hold snapshots during send |
| release | Release snapshot holds |

### Required for Restore

| Permission | Purpose |
|------------|---------|
| receive | Receive snapshot streams |
| create | Create datasets |
| mount | Mount filesystems |

### Required for Encryption

| Permission | Purpose |
|------------|---------|
| load-key | Load encryption key |
| change-key | Change encryption key |

## Checking Current Permissions

View current delegations:

```bash
# Check source pool
zfs allow NIXROOT

# Check backup pool
zfs allow NIXBACKUPS
```

## Removing Delegation

To revoke permissions:

```bash
# Remove all permissions for user
sudo zfs unallow -u $USER NIXROOT
sudo zfs unallow -u $USER NIXBACKUPS
```

## Limitations

Even with full delegation, these operations typically require root:

- `zpool import` - Importing pools
- `zpool export` - Exporting pools
- `udisksctl power-off` - Powering off USB drives

### Workaround: Sudoers Rules

You can create specific sudoers rules for these commands:

```bash
# /etc/sudoers.d/zfs-backup
youruser ALL=(root) NOPASSWD: /sbin/zpool import *
youruser ALL=(root) NOPASSWD: /sbin/zpool export *
youruser ALL=(root) NOPASSWD: /usr/bin/udisksctl power-off *
```

!!! danger "Security Warning"
    Be careful with NOPASSWD rules. They allow the specified commands without authentication.

## Testing Delegation

After configuring, test without sudo:

```bash
# Should work with delegation
zfs list
zfs snapshot NIXROOT/home@test
zfs destroy NIXROOT/home@test

# May still require sudo
zpool import NIXBACKUPS  # Usually needs sudo
```

## Recommended Approach

For most users, we recommend:

1. **Use sudo** for the backup tool itself
2. **Configure delegation** only if you have specific security requirements
3. **Use the TUI** which handles permissions gracefully

---

Made with :heart: by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/kartoza/zfs-backup)
