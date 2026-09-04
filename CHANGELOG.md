<!-- SPDX-FileCopyrightText: Tim Sutton / Kartoza -->
<!-- SPDX-License-Identifier: MIT -->

# Changelog

All notable changes to Kartoza ZFS Backup Tool are recorded here. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the
project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.1.0] - 2026-09-03

### Security

- **Preparing a backup device can no longer wipe the wrong disk.** The old
  flow accepted a free-typed /dev path - its placeholder was literally
  `/dev/sda` - and erased it after a single `y`. The disk is now chosen from
  a vetted picker: anything mounted, in use by an imported ZFS pool, or
  holding the running system is listed but refused, with the reason shown.
  Before wiping, the user must type the disk's own name (not a stock word),
  and `performPrepare` re-vets the disk immediately before touching it, so
  even a future caller that skips the picker cannot wipe a disk the system
  is using.
- **Force Full Backup now demands a typed DESTROY.** It deletes every backup
  snapshot on the destination pool; that cost is now named, pool and all,
  and must be typed - a single `y` no longer suffices.

### Added

- **The main menu was redesigned.** Operations are grouped into named
  sections (Back Up, Restore, Health, Pools, Danger Zone) in workflow order.
  Every entry carries a safety badge - read-only, makes changes, or
  destructive - and on wide terminals a detail pane shows the highlighted
  operation's full contract: what it touches and what it never touches,
  plus the guard on destructive entries. `/` filters the menu as you type.
  Help and Exit moved to the footer keys they already had (`?`, `q`).
- **A pool that stops responding can now be fixed from the app.** When ZFS
  suspends I/O to a pool, the failure screen offers `f` to open a guided
  recovery, and there is a **Fix a Pool That Stopped Responding** menu item.
  It diagnoses the pool, then works through the remedies gentlest first -
  `zpool clear`, then a forced export and re-import - re-checking the pool
  after each and stopping as soon as it is back. Nothing in the ladder
  destroys data, the forceful step asks first, and every command runs under a
  45-second deadline because commands against a wedged pool can otherwise
  block in the kernel forever. When the ladder is exhausted it says what is
  left to check by hand.
- **The version header now shows the commit the binary was built from**, e.g.
  `Version 2.1.0 (fa247f7)`. A version number alone cannot tell you whether a
  rebuild actually took effect. The flake, `package.nix` and the Makefile all
  inject it; a build with no SHA available shows the bare version rather than
  empty brackets.

### Fixed

- **A suspended pool is now explained instead of leaked.**
  `pool I/O is currently suspended` means ZFS lost the pool's devices - an
  external drive unplugged, a cable knocked, a USB enclosure dropping off the
  bus - and no amount of retrying inside zfs-backup will help. The error now
  names the condition and gives the recovery steps (`zpool clear`, forced
  export and re-import, reboot) with the pool name filled in.
- **Multi-line errors are no longer centred line by line.** The result screen
  centred every line of an error independently, which scattered indented
  command lines across the screen. The headline is still centred; the detail
  is left-aligned in a box so commands stay readable and copy-pasteable.

- **ZFS errors now say what ZFS actually said.** `runCommandOutput` captured
  the command's output and then discarded it, so every failure reached the
  user as `zfs failed: exit status 1` - a message naming neither the problem
  nor the pool. The captured text is now part of the error, turning
  "failed to check key status: zfs failed: exit status 1" into
  "...: cannot open 'NIXBACKUPS': dataset does not exist".
- **A pool is no longer mistaken for one whose name contains it.**
  `isPoolImported` substring-matched the whole `zpool list` table, so a longer
  pool name or a column heading counted as a match. The import stage then
  reported success without importing anything and the next stage failed
  against a pool that was not there. Pool names are now matched exactly.
- **Resuming re-checks the pool import and key load.** Both were skipped when
  a previous run had marked them complete, but neither stays true: a drive can
  be unplugged, a pool exported, a machine rebooted. Both are idempotent and
  cheap, so a resumed run re-checks them instead of assuming. Genuinely
  expensive stages are still skipped as before.
- **An unencrypted backup pool no longer fails the key stage.** ZFS reports
  keystatus `-` for a dataset with no encryption, which is not the same as a
  key that is not loaded yet; the two are now distinguished. The three copies
  of this logic were merged into one.

- **Backup no longer fails when a dataset shares the hostname's name.** On
  host `abyss`, a dataset called `abyss` made the legacy flat path
  (`NIXBACKUPS/abyss`) and the hostname namespace container
  (`NIXBACKUPS/abyss`) the same dataset, so the layout migration asked ZFS to
  rename it inside itself and the run died with `New dataset name cannot be a
  descendant of current dataset name`. That collision is now detected, both
  datasets are left untouched, the run continues, and the report explains what
  was skipped and why. Merging or renaming would risk data loss, so neither is
  attempted automatically.
- **A pool can no longer be selected as both source and destination.**
  Replicating a pool onto itself sent every dataset into a namespace beside its
  own source and drove the layout migration into the same impossible rename.
  It now fails immediately with a plain message instead of a ZFS error.

### Added

- **The health check and cleanup now say when no backup scope is set.** With no
  scope chosen every top-level dataset counts as in scope, so the `-Backup`
  snapshots older versions left on them were treated as managed and never
  reported. Upgrading users ran the check, saw a healthy pool, ran the cleanup,
  reclaimed nothing, and had no way to tell why. Both screens now explain the
  situation and point at Backup Scope. Choose a scope first, then check, then
  clean.
- **Orphan cleanup is now a menu option.** Reclaiming the space left behind by
  the pre-2.0 recursive snapshot bug previously required knowing the
  `cleanup-orphans` subcommand and its `--yes` flag. It is now a
  **Clean Up Orphaned Snapshots** item on the main menu, and the
  **Backup Health Check** screen offers `c` to jump straight from the diagnosis
  to the cure with the pool already selected.
- The cleanup screen walks three phases in place rather than in a popup: the
  dry run (what would be destroyed, what each snapshot uniquely holds, and
  anything a safety check held back), the typed `DESTROY` confirmation, and the
  outcome with space usage re-read afterwards.
- **Backup Scope**, **Backup Health Check** and **Clean Up Orphaned Snapshots**
  are now documented on the in-app help screen.

### Changed

- The TUI and the `cleanup-orphans` subcommand now share one plan-and-destroy
  implementation (`buildCleanupPlan`, `renderCleanupPlan`,
  `destroyPlannedSnapshots`). A snapshot the CLI refuses to touch is equally
  untouchable from the menu; neither route can drift into being the softer one.

### Safety

- Unchanged from 2.0.0 and now asserted at the shared layer: snapshots are
  destroyed one at a time, never with a range expression and never recursively;
  ``, held snapshots, snapshots with dependent clones and in-scope
  datasets are never touched.

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

[Unreleased]: https://github.com/timlinux/zfs-backup/compare/v2.1.0...HEAD
[2.1.0]: https://github.com/timlinux/zfs-backup/compare/v2.0.0...v2.1.0
[2.0.0]: https://github.com/timlinux/zfs-backup/compare/v1.6.0...v2.0.0
[1.6.0]: https://github.com/timlinux/zfs-backup/compare/v1.5.0...v1.6.0
[1.5.0]: https://github.com/timlinux/zfs-backup/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/timlinux/zfs-backup/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/timlinux/zfs-backup/releases/tag/v1.3.0
