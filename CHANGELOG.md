<!-- SPDX-FileCopyrightText: Tim Sutton / Kartoza -->
<!-- SPDX-License-Identifier: MIT -->

# Changelog

All notable changes to Kartoza ZFS Backup Tool are recorded here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the
project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.0.0] - 2026-08-14

### Fixed

- **Snapshot scope now equals replication scope.** Every backup run took a
  recursive snapshot of the whole source pool (`zfs snapshot -r POOL@...`)
  while replication and pruning only ever covered a narrower set. The result
  was that `-Backup` snapshots piled up forever on the pool root, on nested
  descendants, and on every dataset except `POOL/home` — silently consuming
  the quota of datasets the user never asked to back up, until writes to `/`
  started failing with quota-exceeded errors. Snapshots are now taken one
  dataset at a time over a single canonical list that also drives replication
  and pruning, and `zfs snapshot -r` is never used.
- **Pruning covers every dataset, not just `home`.** `pruneOldLocalSnapshots`
  and `pruneBackupSnapshots` were hardcoded to `POOL/home@`, so every other
  dataset accumulated snapshots indefinitely. Pruning now iterates the same
  canonical dataset list as the rest of the run.
- **Backup-side pruning works again.** It targeted `destPool/home@` while
  1.6.0 writes to `destPool/<hostname>/home`, so it had been a no-op since the
  layout migration landed. It now resolves the real destination path.
- **`--no-sync-snap` is passed to syncoid** wherever zfs-backup created its own
  snapshot to replicate from. syncoid's `syncoid_<host>_<timestamp>` snapshots
  are orphaned on the source whenever a send fails, and nothing cleaned them
  up. The pull-from-remote flow deliberately keeps sync snapshots: it never
  snapshots the remote itself, so they are its only guaranteed replication base.
- **Failed datasets are reported, not silently swallowed.** A dataset that
  fails to replicate now has the snapshot created for it this run destroyed
  again, is named in the run summary, and makes the run exit non-zero. The
  backup disk is still exported and powered off safely first.
- **Only zfs-backup's own snapshots are ever pruned.** Pruning previously
  matched every snapshot on `POOL/home`, which included sanoid's `autosnap_*`
  snapshots. It now matches only the `YYYY-MM-DD.HHh-MM-Backup` pattern, and
  `@blank` is protected everywhere — destroying it breaks NixOS "erase your
  darlings" installs.
- **A snapshot is never destroyed before its bookmark is confirmed.** The old
  prune ignored the result of `zfs bookmark` and destroyed regardless, which
  could cost the incremental base.

### Added

- **Backup scope.** Choose which datasets a pool backs up. Anything outside the
  scope is never snapshotted, replicated or pruned. Available as a TUI screen
  ("Backup Scope", tick boxes with space / `a` / `n` / enter) and as
  `zfs-backup scope [--pool POOL] [--datasets a,b] [--all]`. Saved per pool to
  `~/.config/zfs-backup/scope.json`. Unconfigured pools keep backing up every
  top-level dataset, so nobody's coverage shrinks on upgrade.
- **`zfs-backup doctor`** and the TUI "Backup Health Check" screen: a read-only
  report of out-of-scope `-Backup` snapshots, stale `syncoid_*` snapshots, and
  datasets whose snapshots are eating their quota. Exits 1 when issues exist.
- **`zfs-backup cleanup-orphans`** to reclaim the space the old behaviour
  leaked. Dry run by default; `--yes` plus a typed `DESTROY` confirmation to
  act (`--force` for automation). Refuses to touch `@blank`, held snapshots,
  snapshots with dependent clones, datasets in scope, and anything not matching
  zfs-backup's own naming patterns. Destroys one snapshot at a time, never a
  range expression.
- **Test suite.** Unit tests run anywhere via a fake command runner and assert
  the scope invariant directly. ZFS integration tests behind the `integration`
  build tag create a throwaway file-backed pool (`ZFS_BACKUP_INTEGRATION=1`,
  root required).
- **CI.** `go vet`, `gofmt` check on new sources, and `go test ./...` on every
  push and pull request.

### Changed

- **BREAKING:** zfs-backup no longer snapshots datasets it does not replicate.
  If you relied on it as a general-purpose recursive snapshotter for your whole
  pool, it is not one any more, and never safely was — use sanoid for that.
  Existing `-Backup` snapshots on out-of-scope datasets are left alone; run
  `zfs-backup doctor` to see them and `zfs-backup cleanup-orphans` to remove
  them.
- **BREAKING:** a backup run with any failed dataset now exits non-zero.
  Automation that treated exit code 0 as "ran" rather than "succeeded" will
  start reporting these failures.
- Pruning no longer removes sanoid `autosnap_*` snapshots from `POOL/home`.
  Retention of those snapshots is sanoid's job and is governed by its own
  policy.
- The end-of-run report counts snapshots across the datasets actually backed
  up, instead of assuming `POOL/home`.

### Migration

No action is required for the fix itself — the next run simply stops creating
new orphans. To clean up what earlier versions left behind:

```bash
sudo zfs-backup doctor                  # see what is affected
sudo zfs-backup cleanup-orphans         # dry run, destroys nothing
sudo zfs-backup cleanup-orphans --yes   # after reviewing, type DESTROY
```

To narrow what gets backed up (for example to `home` only):

```bash
sudo zfs-backup scope --pool NIXROOT --datasets home
```

Note that space is only reclaimed once *every* snapshot pinning a given block
is gone, so usage may barely move until the last few are destroyed.

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

[Unreleased]: https://github.com/timlinux/zfs-backup/compare/v2.0.0...HEAD
[2.0.0]: https://github.com/timlinux/zfs-backup/compare/v1.6.0...v2.0.0
[1.6.0]: https://github.com/timlinux/zfs-backup/compare/v1.5.0...v1.6.0
[1.5.0]: https://github.com/timlinux/zfs-backup/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/timlinux/zfs-backup/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/timlinux/zfs-backup/releases/tag/v1.3.0
