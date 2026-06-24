<!-- SPDX-FileCopyrightText: Tim Sutton / Kartoza -->
<!-- SPDX-License-Identifier: MIT -->

# Changelog

All notable changes to Kartoza ZFS Backup Tool are recorded here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the
project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.6.0] - 2026-06-24

### Added

- **Per-snapshot progress tracking.** While `syncoid` runs, a background
  goroutine polls the destination snapshot list every two seconds. Snapshots
  that have arrived light up Kartoza blue (done), the next still-missing
  snapshot flashes orange (in flight), the rest stay as empty circles
  (pending). On failure the in-flight snapshot turns red so you can see
  exactly where the chain broke. Applied to all four sync sites: local
  backup, force backup, pull-remote, push-remote.
- **Automatic legacy-layout migration.** At the start of every backup, any
  dataset sitting at the old flat path (`NIXBACKUPS/home`) is renamed into
  the hostname namespace (`NIXBACKUPS/<hostname>/home`). If a dataset exists
  at both paths the migration aborts with a clear error so existing
  snapshots are never silently merged or destroyed.
- **`atuin` and other unmounted datasets are now backed up.** The
  `getChildDatasets` / `getRemoteChildDatasets` discovery no longer skips
  datasets with `mountpoint=-`, so application-managed datasets are
  included.
- **Kartoza brand mkdocs theme.** Documentation site rebuilt with the
  Kartoza screencaster theme: Nunito + JetBrains Mono, sticky tabs, hero
  landing page, flat brand-coloured admonitions, glightbox image zoom,
  git-revision-date-localized.
- **New section index pages** (`getting-started/`, `user-guide/`,
  `admin-guide/`, `developer-guide/`, `about/`) so every tab has a landing
  page.
- **`requirements-docs.txt`** pinning the mkdocs build dependencies.

### Changed

- **Docs deployment**. The `docs.yml` workflow now uses the modern GitHub
  Pages model (`upload-pages-artifact` + `deploy-pages@v4`) with the right
  `pages: write` / `id-token: write` permissions and a concurrency group,
  replacing the older `mkdocs gh-deploy --force` push to the `gh-pages`
  branch.
- **`sendDatasetProgress` deep-copies snapshot dots** so the new background
  poller cannot race the UI.
- **In-flight snapshot dot is now animated** via the bubbletea spinner
  glyph, matching the dataset-level "syncing" icon.
- **Hostname-namespaced layout is now the documented default.** README and
  the configuration guide describe the auto-migration and the rationale.

### Fixed

- Backups no longer silently skip `NIXROOT/atuin` (and any other
  `mountpoint=-` datasets).
- Snapshot dot grid no longer renders the entire batch as orange "syncing"
  — only the snapshot actually in flight does.

## [1.5.0] - 2026-05-24

- Comprehensive PDF reports with full pool inventory.

## [1.4.0] - 2026-05-19

- Markdown + PDF backup reports written to
  `~/.local/share/zfs-backup/reports/` after every run, with the dataset
  matrix, timings, sizes and snapshot counts.
- Redesigned in-flight progress UI: global progress bar, per-dataset
  progress bar, and per-dataset snapshot dot matrix for the active dataset.
- Skip non-mounted datasets (mountpoint `-`) during sync — superseded by
  the 1.6.0 change above.

## [1.3.0] - 2026-05-16

- Multi-host backup support with hostname-namespaced datasets on the
  destination.
- Pull remote backup (read from a remote ZFS pool over SSH).
- Push backup to a remote backup server.
- All-dataset backup (no longer limited to `home`).
- Saved remote-host profiles persisted to
  `~/.config/zfs-backup/hosts.json`.
- Smart pool defaults based on the `BACKUP` keyword in the pool name.
- Fix: force-backup pool selection visible after confirm.
- Bug fix: backup hang when syncing a new dataset because the destination
  did not yet exist — destinations are now pre-created with
  `zfs create -p`.

[Unreleased]: https://github.com/timlinux/zfs-backup/compare/v1.6.0...HEAD
[1.6.0]: https://github.com/timlinux/zfs-backup/compare/v1.5.0...v1.6.0
[1.5.0]: https://github.com/timlinux/zfs-backup/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/timlinux/zfs-backup/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/timlinux/zfs-backup/releases/tag/v1.3.0
