# Bug brief: `zfs-backup` recursively snapshots the whole pool but only prunes one dataset

**Target project:** `github:timlinux/zfs-backup`
**Observed on:** `zfs-backup` 1.6.0, host `abyss` (NixOS), pool `NIXROOT`
**Reported:** 2026-08-14
**Severity:** High — silently consumes the quota of datasets the user never asked to back up, eventually causing `zfs: out of space` / quota-exceeded write failures on `/`.

---

## 1. Symptom

On `abyss`, the root dataset hit its quota. Almost all of the space was snapshots, not
data:

```
NAME               USED  USEDSNAP  USEDDS  USEDCHILD  REFER  QUOTA  REFQUOTA  AVAIL
NIXROOT/root      20.2G     19.2G   1.04G         0B  1.04G    30G      none  9.80G
```

**95% of `NIXROOT/root`'s usage is snapshots.** The live filesystem is only 1.04G.

The user's intent — and their entire declarative snapshot policy — is that **only
`NIXROOT/home` is snapshotted**. Their sanoid unit is scoped to exactly one dataset and
literally cannot touch anything else:

```
ExecStartPre=… zfs allow sanoid snapshot,mount,destroy NIXROOT/home
```

So sanoid is not the source. `zfs-backup` is.

## 2. Evidence

### 2.1 Snapshots exist on every dataset in the pool

```
snapshot count per dataset
    144 NIXROOT/home        <- sanoid autosnap_* (expected, correct)
     40 NIXROOT/root        <- NOT expected
     40 NIXROOT/overflow    <- NOT expected
     32 NIXROOT/atuin       <- NOT expected
     21 NIXROOT             <- NOT expected (pool root dataset)
     20 NIXROOT/nix         <- NOT expected
```

### 2.2 The 40 snapshots on `NIXROOT/root` break down into three groups

| Group | Count | Example |
|---|---|---|
| `zfs-backup` snapshots | 21 | `NIXROOT/root@2026-05-17.22h-55-Backup` |
| syncoid sync-snapshots | 18 | `NIXROOT/root@syncoid_abyss_2026-05-19:00:49:53-GMT01:00` |
| disko install snapshot | 1 | `NIXROOT/root@blank` (2026-04-21, unrelated, keep) |

They span 2026-05-17 to 2026-08-13 — i.e. **every `zfs-backup` run since May has been
leaving two orphans per run on each non-`home` dataset.**

Note the near 1:1 pairing: 21 `-Backup` snapshots, 18 `syncoid_*` snapshots. The three
missing `syncoid_*` correspond to the most recent runs. Each backup run creates a
`-Backup` snapshot **and** syncoid creates its own sync-snapshot, and on non-`home`
datasets neither is ever cleaned up.

### 2.3 Bookmarks exist for `home` only

`zfs-backup`'s documented design is to convert pruned snapshots into bookmarks so future
incremental sends still have a base. That conversion happens **only** for `NIXROOT/home`:

```
NIXROOT/home#2026-05-17.22h-55-Backup
NIXROOT/home#2026-05-19.00h-24-Backup
…
NIXROOT/home#autosnap_2026-07-22_22:00:00_hourly
```

There is not a single bookmark on `root`, `nix`, `overflow`, `atuin`, or `NIXROOT`.
So the prune/bookmark path is scoped to the configured source dataset, while the
**snapshot** path is not.

### 2.4 The tool's own help text states the recursion

From `strings` on the 1.6.0 binary:

```
   Using -r to snapshot all datasets under the source pool.
```

## 3. Root cause (verify against source)

The two halves of the tool disagree about scope:

| Phase | Scope | Result |
|---|---|---|
| Snapshot creation | **Whole pool**, via `zfs snapshot -r POOL@<stamp>-Backup` | Snapshots created on every dataset |
| Replication (syncoid) | Configured source dataset only (`POOL/home`) | Only `home` is sent |
| Prune → bookmark | Configured source dataset only | Only `home` is cleaned |

Consequences:

1. **`-Backup` snapshots on non-source datasets are never pruned** — nothing in the
   cleanup path enumerates them, because cleanup iterates the configured source list.
2. **syncoid leaves its sync-snapshots behind on failure.** syncoid normally deletes its
   `syncoid_<host>_<timestamp>` snapshot after a successful send. If syncoid is invoked
   recursively (or per-dataset over the whole pool) and the send fails — no matching
   target dataset on the backup pool, dataset not selected for replication, etc. — the
   sync-snapshot is orphaned. This matches the 18 orphans on `root`.

**Action for the agent:** confirm the above in the source. Specifically locate:

- where the snapshot is taken and whether `-r` / recursive is unconditional;
- how the prune/bookmark routine builds its dataset list;
- how syncoid is invoked, and whether `--no-sync-snap` is passed.

Do not assume my line-level reading is right — the analysis above is derived from
on-disk state and the binary's help strings, not from reading the source.

### 3.1 Why this is worse than it looks

Per-snapshot `USED` values look tiny (typically 5–180 MB):

```
NIXROOT/root@2026-08-13.23h-47-Backup     178M  4.06G
NIXROOT/root@syncoid_abyss_2026-05-29…    129M  5.73G
```

Those sum to roughly 1 GB, yet `USEDSNAP` is **19.2G**. That is because a snapshot's
`USED` counts only blocks *unique to that one snapshot*; blocks retained by two or more
snapshots are attributed to none of them individually.

**Practical implication for the cleanup design:** deleting a subset of the orphans frees
almost nothing. Space is only reclaimed once *every* snapshot pinning a given block is
gone. The cleanup must therefore be all-or-nothing per dataset, and any "delete the
oldest N" strategy will appear to do nothing and mislead the user.

## 4. Prevention — required changes

### 4.1 Snapshot scope must equal replication scope

The invariant to enforce:

> `zfs-backup` must never create a snapshot on a dataset it is not going to replicate
> **and** subsequently prune.

Concretely:

- **Drop the unconditional `-r`.** Take snapshots per-dataset over the explicit,
  configured dataset list, not recursively over the pool.
- If recursive behaviour is genuinely wanted for some users, it must be opt-in
  (`--recursive` / config key), and it must expand the *entire* pipeline — snapshot,
  replicate, prune, bookmark — over the same expanded list. Never one phase only.
- Derive one canonical `datasets_to_process` list at startup and pass it to every phase.
  A single source of truth makes this class of bug structurally impossible.

### 4.2 Pass `--no-sync-snap` to syncoid

Since `zfs-backup` already creates its own named `-Backup` snapshot to serve as the
replication base, syncoid does not need to create another. Passing `--no-sync-snap` tells
syncoid to use existing snapshots instead of making (and trying to clean up) its own,
which removes the orphan-on-failure mode entirely.

If sync-snaps are kept for some reason, ensure failures are trapped and the sync-snap is
destroyed in a cleanup handler.

### 4.3 Fail loudly instead of silently orphaning

If a send fails for a dataset, the run currently leaves debris and carries on quietly.
It should:

- report the failed dataset in the run summary and exit non-zero;
- destroy any snapshot it created for that failed dataset in the same run, so a failed
  run leaves no residue.

### 4.4 Add a `doctor` / `--check` subcommand

Users have no way to notice this until a quota blows. Add a read-only check that reports:

- snapshots matching `zfs-backup`'s own naming pattern (`*-Backup`) on datasets **not**
  in the configured list;
- `syncoid_*` snapshots older than the last successful run;
- per-dataset `usedbysnapshots` vs `quota`, flagging anything over ~50%.

This doubles as the detection tool for §5.

### 4.5 Documentation

Make the snapshot scope explicit and prominent in the README — users reasonably assume a
tool told to back up `POOL/home` will only touch `POOL/home`. Document the interaction
with `quota` vs `refquota` (see §6).

### 4.6 Tests

Regression tests, ideally against a file-backed test pool created in the test harness:

1. Pool with `tank/home`, `tank/root`, `tank/nix`; configure backup of `tank/home` only.
   Run a backup. **Assert `tank/root` and `tank/nix` have zero snapshots.**
2. Run backup twice. Assert no `syncoid_*` snapshots survive on any dataset.
3. Simulate a failing send for one dataset. Assert the run exits non-zero and leaves no
   snapshot behind for that dataset.
4. With `--recursive` (if implemented), assert every dataset gets snapshotted **and**
   pruned/bookmarked — counts stay bounded across many runs.
5. Assert `doctor` detects a hand-planted orphan `tank/root@2026-01-01.00h-00-Backup`.

## 5. Cleanup for already-affected systems

### 5.1 Detection

A system is affected if snapshots matching the `-Backup` or `syncoid_*` patterns exist on
datasets outside the configured backup set:

```bash
# list every dataset carrying zfs-backup-created snapshots
sudo zfs list -H -t snapshot -o name -r POOL \
  | grep -E '@([0-9]{4}-[0-9]{2}-[0-9]{2}\.[0-9]{2}h-[0-9]{2}-Backup|syncoid_)' \
  | sed 's/@.*//' | sort | uniq -c | sort -rn
```

Any dataset in that output that is not a configured backup source is affected.

### 5.2 Pre-flight checks — MANDATORY before destroying anything

Destroying snapshots is **irreversible**. The cleanup routine must verify all of the
following and refuse to proceed if any check fails:

1. **Is the dataset actually replicated anywhere?** If a dataset *is* being sent to a
   backup pool, its most recent snapshot is the incremental base. Destroying it forces a
   full re-send. Determine the latest snapshot/bookmark common to source and target and
   never destroy at or after it.
2. **Holds.** `zfs holds <snap>` — a held snapshot cannot be destroyed and signals
   something else depends on it.
3. **Clones and dependents.** `zfs destroy -nv` reports these; a snapshot with a clone
   must not be destroyed.
4. **Never destroy `@blank`.** On NixOS "erase your darlings" setups, `POOL/root@blank`
   is rolled back to on every boot. Destroying it **breaks the system**. It is also
   harmless to keep — on `abyss` it is 168K. Exclude it by name, always.
5. **Preserve anything the user deliberately created.** Only ever match the tool's own
   naming patterns. Never do a blanket "destroy all snapshots on this dataset".

### 5.3 Dry run first

`zfs destroy` supports `-n` (dry run) and `-v` (verbose). The cleanup must default to dry
run and require an explicit `--yes` / `--confirm` to actually destroy:

```bash
# dry run — shows what would go and how much would be reclaimed
sudo zfs destroy -nv POOL/root@2026-05-17.22h-55-Backup
```

### 5.4 Cleanup procedure

For each affected, non-configured dataset:

```bash
DS=POOL/root

# 1. Enumerate only zfs-backup's own snapshots, explicitly excluding @blank
sudo zfs list -H -t snapshot -o name -r "$DS" \
  | grep "^${DS}@" \
  | grep -E '@([0-9]{4}-[0-9]{2}-[0-9]{2}\.[0-9]{2}h-[0-9]{2}-Backup|syncoid_)' \
  | grep -v '@blank$' \
  > /tmp/zfs-cleanup-list.txt

# 2. REVIEW THE LIST BY HAND. This is the last reversible moment.
cat /tmp/zfs-cleanup-list.txt

# 3. Dry run every entry
while read -r s; do sudo zfs destroy -nv "$s"; done < /tmp/zfs-cleanup-list.txt

# 4. Only after review — destroy one at a time (NOT a range expression)
while read -r s; do sudo zfs destroy -v "$s"; done < /tmp/zfs-cleanup-list.txt
```

Deliberately **not** using range destroy (`zfs destroy pool/ds@a%b`): ranges are easy to
get wrong and will happily take out snapshots that did not match the pattern, including
`@blank`.

Expect space to be reclaimed only after the *last* snapshot pinning shared blocks is
destroyed — usage may barely move until the final few (see §3.1). Verify with:

```bash
sudo zfs list -o name,used,usedbysnapshots,usedbydataset,refer,quota,avail -r POOL
```

On `abyss` this should take `NIXROOT/root` from `USED 20.2G / USEDSNAP 19.2G` to roughly
`USED 1.04G / USEDSNAP ~0`.

### 5.5 Ship it as a subcommand

Package the above as `zfs-backup cleanup-orphans [--dataset X] [--dry-run|--yes]`, sharing
the detection logic with `doctor` (§4.4). Dry run must be the default. Print a clear
summary — datasets affected, snapshot counts, estimated reclaim — and require typed
confirmation before destroying.

## 6. Related note: `quota` vs `refquota`

Worth documenting in the README, because it determines how visible this bug is.

- **`quota`** limits the dataset **plus its snapshots and descendants**. Orphaned
  snapshots count against it — which is why `NIXROOT/root` hit its 30G limit with only
  1.04G of live data.
- **`refquota`** limits only the referenced (live) data, ignoring snapshots.

Systems using `quota` on their root dataset will hit hard write failures on `/` from this
bug. Systems using `refquota` will instead silently eat pool free space. Both are bad, but
the `quota` case is the one that takes the machine down, so it should be called out.

## 7. Project conventions to follow

Per the user's standards:

- Open a GitHub issue first: user story, mermaid context diagram, success criteria, job
  size, epic, tags, assigned to the logged-in user.
- Conventional Commits. This is a bug fix with a behaviour change to snapshot scope —
  judge whether that is `fix:` (patch) or a breaking change for anyone relying on the
  recursive behaviour; if the latter, major bump and document the migration.
- Update `CHANGELOG.md` (Keep a Changelog) with a release section for the version bump.
- Update `SPECIFICATION.md` to state the snapshot-scope invariant from §4.1.
- Tests per §4.6 — this bug is exactly the kind that a scope assertion would have caught.
- PR referencing the issue with `fixes` syntax.

---

**Made with 💗 by [Kartoza](https://kartoza.com)**
