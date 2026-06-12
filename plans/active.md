# Active — decouple disk verdict from target Chef version

Branch: `fix/disk-readiness-decouple-target`. Spec: `analysis-node-readiness.md`
(§ "Version invariance and cross-view consistency" already states the contract).

## Problem (confirmed against the dev DB)

The disk verdict (`sufficient_disk_space`) is **version-invariant** — it needs only
the node's filesystem data + platform install size — yet it is computed *only* as a
side-effect of per-target readiness evaluation, which is gated on
`len(TargetChefVersions) > 0` (`collector.go:1464`) and stored per
`(node, target_chef_version)` in `node_readiness`. So with **no target version
configured** (e.g. after an app reinit), readiness eval never runs, `node_readiness`
is empty for every node, and disk status vanishes everywhere:
- List view: `node.readiness` empty → `DiskBadge "unknown"`.
- Detail view: `ReadinessSection` returns null (no rows) → the whole disk card
  disappears, leaving only the "View Filesystem Details" link.
Same root cause makes a freshly-collected, not-yet-evaluated node show unknown.

## Fix (approved 2026-06-12): store the verdict per node, at collection time

Compute the disk verdict once when the node snapshot is written and store it on
`node_snapshots` (NULL = indeterminate). Display + filter read this node-level value,
independent of target version / readiness rows. Chosen over derive-on-read so the
`disk_blocked`/`disk_unknown` filter chips keep working in SQL with pagination.

### Chunk 1 — extract a pure `EvaluateDisk` function [refactor, no schema change]
- New `analysis.EvaluateDisk(filesystem, platform, DiskConfig) DiskVerdict` (pure;
  reuses `parseFilesystemAttribute`/`findBestMount`/`toInt64`). `DiskConfig` =
  install path/size per platform + `MinRemainingFreePercent`. `DiskVerdict` =
  `{Sufficient *bool, AvailableMB *int, RequiredMB int, TotalMB *int}` — `Sufficient`/
  `AvailableMB` nil when filesystem data is missing/unparseable.
- Refactor `ReadinessEvaluator.evaluateOne` disk block to build a `DiskConfig` (from
  `configFn`/baked) and delegate to `EvaluateDisk` (stale still → nil, applied by the
  caller). Behaviour-preserving — existing `readiness_test.go` disk cases guard it.
- TDD: `EvaluateDisk` unit tests (sufficient; insufficient by absolute; insufficient
  by min-free-%; unknown/no-fs; windows path; required always set). All green + `-race`.

### Chunk 2 — persist verdict on node_snapshots [migration + datastore + collector] [DONE]
- Migration 0037: `node_snapshots.sufficient_disk_space BOOLEAN`, `available_disk_mb INT`,
  `required_disk_mb INT` (all NULL-able; NULL sufficient = indeterminate). + down.
- datastore: add the 3 fields to `NodeSnapshot` + `InsertNodeSnapshotParams`; wire the
  single + bulk upsert (INSERT / ON CONFLICT / RETURNING) and both `scanNodeSnapshot`s.
- collector: at snapshot build (`collector.go:865`) compute via `EvaluateDisk` from live
  cfg (install path/size + min-free-%) and a stale check; populate the params. Disk is
  now produced for every collected node regardless of target version.
- TDD: datastore round-trip (set/NULL); collector populates verdict + NULL when stale/no-fs.

Chunk 3 split by layer: backend (API + filter) first, then frontend.

### Chunk 3a — backend: node-level disk on API + filter [DONE]
- webapi `nodeResp` now carries node-level `disk_status` + `disk_detail` +
  `sufficient_disk_space`/`available_disk_mb`/`required_disk_mb` from the snapshot,
  via shared `diskStatusFor`/`diskDetailFor` (extracted in `check_status.go`;
  `deriveDiskStatus`/`diskDetail` delegate). Detail handler already returns the raw
  `node` snapshot, which now includes the disk fields.
- filter: `disk_blocked` → `cn.sufficient_disk_space = false`, `disk_unknown` →
  `cn.sufficient_disk_space IS NULL` (was the `node_readiness` EXISTS). Disk columns
  threaded through the `current_nodes` CTE + light/heavy projection + filtered scan.
- TDD: handler test (disk_status sufficient/insufficient/unknown with NO readiness
  rows); filter SQL-string tests updated to the column form; functional round-trip +
  disk filter (light + heavy scan) verified against a real Postgres. All `-race`.

### Chunk 3b — frontend: consume node-level disk [next]
- list `DiskBadge` reads `node.disk_status`/`node.disk_detail` (drop the
  `readiness[0]` fallback); detail `DiskSpacePanel` reads the node-level fields from
  `data.node` and renders even with zero readiness rows (move it out of
  `ReadinessSection`). Update `types/nodes.ts`.
- `node_readiness` disk columns become vestigial → follow-up tech-debt to drop them.
- TDD: list badge + detail card with no readiness rows.

## Spec
- `analysis-node-readiness.md` §version-invariance already states the contract; Chunk 2
  moves *where* the verdict is stored (node-level) — flag whether a spec note is wanted
  before editing (do not modify specs without asking).
