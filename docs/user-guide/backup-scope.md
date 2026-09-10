<!-- SPDX-FileCopyrightText: Tim Sutton / Kartoza -->
<!-- SPDX-License-Identifier: MIT -->

# Backup Scope and Health

<span class="kz-eyebrow">KARTOZA · ZFS BACKUP</span>

Which datasets does the tool actually touch, how do you change that, and how do
you check nothing is quietly piling up?

## The rule

!!! abstract "Snapshot scope invariant"

    zfs-backup never creates a snapshot on a dataset it is not going to
    replicate **and** subsequently prune.

One list of datasets is worked out at the start of every run, and every phase —
snapshot, replicate, prune, bookmark — uses that same list. Datasets outside it
are never snapshotted, never sent, never pruned. The pool root dataset and
nested child datasets are never snapshotted at all.

Pruned snapshots become **bookmarks** of the same name, so incremental sends
still have a base without holding the snapshot's data.

Only snapshots matching zfs-backup's own naming pattern — for example
`2026-08-14.10h-00-Backup` — are ever pruned or destroyed. sanoid `autosnap_*`
snapshots, your own snapshots, and `@blank` are left strictly alone.

## Choosing which datasets are backed up

By default every top-level dataset of the source pool is backed up. To narrow
it, open **Backup Scope** from the main menu:

| Key | Action |
|-----|--------|
| ↑ / k, ↓ / j | Move the cursor |
| space | Tick or untick the dataset under the cursor |
| a | Tick everything |
| n | Untick everything |
| enter | Save |
| esc | Return to the menu without saving |

Or from the command line:

```bash
sudo zfs-backup scope                              # show the current scope
sudo zfs-backup scope --datasets home              # back up only POOL/home
sudo zfs-backup scope --datasets home,atuin        # back up two datasets
sudo zfs-backup scope --all                        # back up everything again
sudo zfs-backup scope --pool OTHERPOOL --datasets srv
```

The scope is saved per pool in `~/.config/zfs-backup/scope.json`. Ticking every
dataset clears the restriction rather than freezing today's list, so a dataset
you create later is still backed up.

!!! tip "Pair it with your snapshot policy"

    If sanoid already snapshots `POOL/home` on a schedule, scoping zfs-backup to
    `home` keeps the two tools' responsibilities aligned: sanoid owns local
    retention, zfs-backup owns replication to the backup drive.

## Upgrading from 1.x: do this in order

The order matters, and getting it wrong makes the tool look broken.

With no scope chosen, **every** top-level dataset counts as in scope. The
`-Backup` snapshots that older versions strewed across `root`, `nix` and the
rest therefore look managed rather than orphaned, so the health check reports
almost nothing and the cleanup reclaims almost nothing.

So choose the scope first:

1. **Backup Scope** - tick only the datasets you actually want backed up.
   Everything you leave out becomes eligible for cleanup.
2. **Backup Health Check** - now it can see the debris on the datasets you
   excluded.
3. Press ++c++ - review the dry run, then type `DESTROY`.
4. Run a backup. From here on zfs-backup only snapshots what it also
   replicates and prunes.

Both the health check and the cleanup screen say so if no scope is set yet, so
you are not left guessing why a pool full of debris reports as healthy.

## Checking backup health

**Backup Health Check** in the menu, or `zfs-backup doctor` on the command line,
is completely read-only. It reports:

- zfs-backup `-Backup` snapshots sitting on datasets outside the scope, which
  nothing will ever prune;
- `syncoid_*` sync snapshots older than 24 hours, left behind by a failed send;
- datasets whose snapshots consume more than half their quota.

```bash
sudo zfs-backup doctor
sudo zfs-backup doctor --pool NIXROOT
```

It exits `0` when the pool is clean and `1` when it finds something, so it works
in a cron job or a monitoring check.

### quota vs refquota

This is what decides how a snapshot leak shows up:

| Property | Counts snapshots? | Symptom |
|----------|-------------------|---------|
| `quota` | Yes — the dataset plus its snapshots and descendants | The dataset hits its limit and writes fail, even with very little live data |
| `refquota` | No — referenced (live) data only | Pool free space is silently consumed instead |

A root dataset with a `quota` will eventually fail writes to `/` if snapshots
accumulate. That is the case worth watching.

## Cleaning up orphaned snapshots

Versions before 2.0.0 snapshotted the whole pool recursively but only pruned one
dataset, so `-Backup` snapshots accumulated on datasets that were never meant to
be backed up. Cleanup removes them.

The easiest route is **Clean Up Orphaned Snapshots** in the menu, or pressing
++c++ on the health check screen. The screen opens on a dry run: it lists every
snapshot it would destroy, what each one uniquely holds, and anything a safety
check held back. Nothing is destroyed until you press ++d++ and type `DESTROY`.

The same thing is available on the command line for scripting:

```bash
sudo zfs-backup cleanup-orphans                    # dry run - destroys nothing
sudo zfs-backup cleanup-orphans --dataset NIXROOT/root
sudo zfs-backup cleanup-orphans --yes              # then type DESTROY to confirm
```

Dry run is the default. `--yes` asks you to type `DESTROY` before anything is
removed; `--force` skips that prompt for automation.

### What it refuses to touch

The menu screen and the subcommand share one implementation, so the rules below
hold identically whichever route you take. Destroying a snapshot is
irreversible, so cleanup will not act on:

- `@blank` — on NixOS "erase your darlings" installs, `POOL/root@blank` is
  rolled back to on every boot, and destroying it breaks the system;
- snapshots with a **hold**;
- snapshots with dependent **clones**;
- datasets that are in the backup scope, which are pruned normally;
- anything that does not match zfs-backup's own naming patterns.

If it cannot prove a snapshot is safe to destroy, it skips it and says why.

!!! warning "Space frees up late, not gradually"

    A snapshot's `USED` counts only the blocks unique to that one snapshot.
    Blocks held by two or more snapshots are attributed to none of them. That
    means deleting a subset of the orphans can free almost nothing, and usage
    may barely move until the last snapshot pinning a block is gone. Do not
    stop half way and conclude it did not work.

## Browsing the backup pool

The health check and cleanup look at the *source* pool. **Browse Backup
Snapshots** (Health section of the menu) looks at the other side: it walks
every dataset and snapshot on the backup pool and judges each one against the
source, so you can see at a glance which backup history is healthy and which
is debris nothing will ever prune.

Pick the source pool, then the backup pool to browse. The left pane lists the
backup pool's datasets with a red count beside any that carry deletion
candidates; the right pane shows the selected dataset's snapshots, newest
first, each with a verdict:

| Verdict | Meaning |
|---|---|
| `base` | The newest snapshot shared with the source — the incremental base for the next backup. Never a candidate: destroying it would force a full re-send. |
| `synced` | An older snapshot the source also still has (as a snapshot or bookmark). |
| `retained` | The source has pruned its copy; the backup keeps it by design. |
| `ORPHAN` | Orphaned from the source: its dataset is gone, or no longer in the backup scope, or it is a stale syncoid sync-snapshot. Nothing will ever prune it — this is a deletion candidate. |
| `recent` | A syncoid sync-snapshot young enough to belong to a send still in flight. |
| `protected` | Never touched (for example `@blank`). |
| `foreign` | Not created by zfs-backup. Shown for completeness, never touched. |
| `remote` | Lives under another host's namespace — judge it on that machine. |

The highlighted snapshot's verdict is explained in a sentence below the panes.
Press `o` to jump straight to the next candidate, wrapping around the whole
pool. Browsing is completely read-only.

Press `c` to clean up the candidates. Every candidate is re-vetted live
against the same rules as the orphan cleanup — holds, clones and protected
tags are checked again at that moment — then you see a full `zfs destroy -nv`
dry run and must type `DESTROY` to proceed. Snapshots are destroyed one at a
time, never with a range expression, and the datasets themselves with their
current contents are always kept.

!!! note "If the backup scope cannot be read"

    The browser degrades safely: without a readable scope it cannot know
    whether a source dataset is still meant to be replicated, so those
    snapshots are shown as `retained` rather than guessed to be orphans.
    Only certainties — a missing source dataset, stale syncoid debris — are
    offered as candidates.

## When a dataset fails to replicate

If a send fails, zfs-backup destroys the snapshot it created for that dataset in
that run, names the dataset in the run summary, and exits non-zero. A failed run
leaves no residue behind, and automation can tell the difference between "ran"
and "succeeded".

---

Made with :heart: by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/timlinux/zfs-backup)
