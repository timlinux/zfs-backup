<!-- SPDX-FileCopyrightText: Tim Sutton / Kartoza -->
<!-- SPDX-License-Identifier: MIT -->

# Getting Started

<span class="kz-eyebrow">KARTOZA · ZFS BACKUP</span>

The shortest path from "I have a fresh USB drive" to "I have an encrypted,
incremental, multi-host backup pool that I trust."

## The five-minute tour

1. **Install the binary.** Either drop the release binary into
   `/usr/local/bin/zfs-backup` or run straight from the flake with
   `nix run github:timlinux/zfs-backup`. See [Installation](../admin-guide/installation.md).
2. **Plug in a fresh USB drive** and run `sudo zfs-backup`. Choose
   **Prepare Backup Device** from the menu and follow the prompts —
   the tool will create an encrypted `NIXBACKUPS` pool with
   AES-256-GCM.
3. **Run a backup.** Choose **Backup ZFS**. The tool snapshots the
   source pool, sends only the changes, prunes old snapshots to
   bookmarks, exports the pool and powers off the drive when it's
   done.
4. **Watch it work.** The per-snapshot dot matrix lights up as each
   snapshot lands on the destination. You see real progress, not a
   spinner.
5. **Read the report.** A markdown + PDF report lands in
   `~/.local/share/zfs-backup/reports/` after every run with every
   timing, every size, and the full pool inventory.

## Next stops

<div class="grid cards" markdown>

-   :material-book-open-page-variant:{ .lg .middle } __User Guide__

    ---

    Full walkthroughs for every menu — backup, restore, pool info,
    maintenance.

    [:octicons-arrow-right-24: User Guide](../user-guide/index.md)

-   :material-cog-outline:{ .lg .middle } __Administrator Guide__

    ---

    Encryption, retention policies, ZFS delegation, packaging.

    [:octicons-arrow-right-24: Administrator Guide](../admin-guide/index.md)

-   :material-code-tags:{ .lg .middle } __Developer Guide__

    ---

    Architecture, build instructions, contributing.

    [:octicons-arrow-right-24: Developer Guide](../developer-guide/index.md)

</div>
