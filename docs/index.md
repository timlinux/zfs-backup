---
hide:
  - navigation
  - toc
---
<!-- SPDX-FileCopyrightText: Tim Sutton / Kartoza -->
<!-- SPDX-License-Identifier: MIT -->

<div class="kz-hero" markdown>

<span class="kz-eyebrow">KARTOZA · ZFS BACKUP</span>

# Snapshot, send, sleep well.

A terminal UI for ZFS backups that does the careful, encrypted, incremental
work for you — and shows you exactly what's happening, dataset by dataset and
snapshot by snapshot, while it runs.

<div class="kz-cta" markdown>
[:material-rocket-launch: Get started](getting-started/index.md){ .kz-cta__primary }
[:material-book-open-page-variant: User guide](user-guide/index.md){ .kz-cta__secondary }
[:simple-github: GitHub](https://github.com/timlinux/zfs-backup){ .kz-cta__secondary }
</div>

</div>

![Kartoza ZFS Backup Tool](assets/images/kartoza-zfs-backup.png){ .kz-figure }

## Why it exists

ZFS already has the right primitives for backups — atomic snapshots,
incremental send/receive, bookmarks, native encryption. What's missing for
most people is a calm, observable way to drive them. Cron + a wall of stderr
is the usual answer; the usual outcome is that nobody notices when the chain
breaks.

This tool wraps those primitives in a terminal interface that walks you
through the whole flow: import the encrypted destination, snapshot the source,
send only what changed, prune the chain, eject the disk. Every step is named,
timed, and visualised — including a live snapshot-by-snapshot dot grid so you
can *see* the backup catch up rather than guess.

## What you can do with it

<div class="grid cards" markdown>

-   :material-package-variant:{ .lg .middle } __Incremental backups__

    ---

    Snapshot the entire source pool and let `syncoid` send only the changes
    since the previous run. The destination receives at block level — fast,
    bandwidth-efficient, and faithful.

    [:octicons-arrow-right-24: Backup operations](user-guide/backup-operations.md)

-   :material-server-network:{ .lg .middle } __Multi-host on one drive__

    ---

    Every dataset lives under `NIXBACKUPS/<hostname>/<dataset>` so multiple
    machines can share one encrypted disk without colliding. Legacy flat
    layouts are auto-migrated on first run.

    [:octicons-arrow-right-24: Configuration](admin-guide/configuration.md)

-   :material-cloud-sync:{ .lg .middle } __Pull and push over SSH__

    ---

    Pull snapshots from a remote machine to a local drive, or push local
    snapshots to a remote backup server — same UI, same progress reporting,
    same hostname namespacing.

    [:octicons-arrow-right-24: Backup operations](user-guide/backup-operations.md)

-   :material-download:{ .lg .middle } __Browse and restore__

    ---

    Open any snapshot in a dual-panel file explorer with vim/yazi
    keybindings, walk the tree, and restore individual files or directories
    to any location.

    [:octicons-arrow-right-24: Restore files](user-guide/restore-files.md)

-   :material-shield-lock:{ .lg .middle } __Encrypted by default__

    ---

    The destination pool is created with ZFS native AES-256-GCM encryption.
    The key is prompted for on every import; data at rest is never in clear.

    [:octicons-arrow-right-24: Configuration](admin-guide/configuration.md)

-   :material-file-document-multiple:{ .lg .middle } __Full PDF reports__

    ---

    After every run, a markdown + PDF report lands in
    `~/.local/share/zfs-backup/reports/` with timings, sizes, snapshot dot
    grids, and the full pool inventory.

    [:octicons-arrow-right-24: Pool information](user-guide/pool-info.md)

</div>

## Install

<div class="grid cards" markdown>

-   :material-snowflake:{ .lg .middle } __Nix / NixOS__

    ---

    `nix run github:timlinux/zfs-backup` — straight from the flake, no install
    step.

    [:octicons-arrow-right-24: Installation](admin-guide/installation.md)

-   :material-linux:{ .lg .middle } __Linux binary__

    ---

    Single static binary on the [releases page](https://github.com/timlinux/zfs-backup/releases) — drop into `/usr/local/bin/` and run with `sudo`.

    [:octicons-arrow-right-24: Installation](admin-guide/installation.md)

-   :material-source-branch:{ .lg .middle } __From source__

    ---

    `go build` inside the `nix develop` shell. Everything reproducibly
    pinned via the flake.

    [:octicons-arrow-right-24: Building](developer-guide/building.md)

</div>

## Project status

[![Docs](https://github.com/timlinux/zfs-backup/actions/workflows/docs.yml/badge.svg)](https://github.com/timlinux/zfs-backup/actions/workflows/docs.yml)
[![Release](https://github.com/timlinux/zfs-backup/actions/workflows/release.yml/badge.svg)](https://github.com/timlinux/zfs-backup/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/timlinux/zfs-backup)](https://github.com/timlinux/zfs-backup/releases)
[![License](https://img.shields.io/github/license/timlinux/zfs-backup)](https://github.com/timlinux/zfs-backup/blob/main/LICENSE)

<div class="kz-footer-credits" markdown>
Made with 💗 by [Kartoza](https://kartoza.com) &middot;
[Sponsor on GitHub](https://github.com/sponsors/kartoza) &middot;
[Repository](https://github.com/timlinux/zfs-backup)
</div>
