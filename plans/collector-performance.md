# Collector Performance and Correctness

Measured baselines and open work from the 2026-07-29/30 production incident and the
days following. Figures are measured at customer scale unless marked otherwise.

## Measured baselines (2026-07-30, v2.18.9, hourly schedule)

- **Full 3-org cycle: ~28 min.** Was 60–90 min before the role fan-out.
- **Role fetching: ~7m16s per cycle**, ~26% of the run. 73,910 roles across 3 orgs:
  org-a 35,465 in 3m35s; org-dev 31,958 in 3m02s; org-b (Windows) 6,487 in 39s.
  Consistently ~170 roles/s at 10 workers ≈ 59ms per round trip. Zero fetch failures.
- **Fan-out speedup: 9.6×** on the same org and role count (org-dev: 29m16s → 3m02s).
  Near-linear against 10 workers, so headroom remains.
- **Node collection: ~193KB of Go heap per node**; ~12GB peak for a single 62k-node org.
  Peak still scales with fleet size.
- Fleet: ~134k nodes — ~75k RHEL, ~55k Windows, ~250 AIX, ~830 with no platform.

## Invariants learned the hard way

- **Ohai attribute shapes vary by platform and Chef version.** Windows delivers
  `filesystem.by_mountpoint` keyed by drive letter and **no `by_pair`**; the fixture in
  `TestHandleNodeDisks_HappyPath_Windows` has encoded this since the original work.
  Narrowing the node search to `filesystem.by_pair` (v2.18.6) silently emptied the
  attribute for 55,488 of 55,489 Windows nodes; reverted in v2.18.9 and pinned by
  `TestNodeSearchAttributes_FilesystemIsNotNarrowed`.
- **Validate attribute shapes against `node_snapshots`, not sample nodes** — see the
  census requirement in `specifications/data-collection.md` § 1.4.1. One node, or a lab
  on current Chef, is not evidence about a heterogeneous estate.
- **Collection interval must exceed run duration.** Overlapping ticks are skipped
  (`scheduler.go:226`), so a short cron makes the collection peak the steady state.
  Nodes converge every 2h, so hourly or slower loses nothing.
- **No unbounded column (JSONB, unbounded TEXT, arrays) in a btree index key or
  `INCLUDE` list** — index tuples are not TOASTed and are capped at 2704 bytes.

## Ruled out by evidence — do not re-investigate

GIN index on `node_snapshots.cookbooks` (69MB, zero dead tuples, autovacuum current);
`perf.Recorder` (bounded ring buffers, `perf/stats.go:37`); logging `DBWriter`
(synchronous, no buffer); event ingest (`converge_runs` empty). There was no memory
leak — goroutines stayed flat at 26–30.

## Open — highest value first

### Role fetch via the search index

**Verified viable on the lab (2026-07-30).** Partial search on the `role` index with
`attributes: ["name","run_list","env_run_lists"]` returns nested maps and arrays intact,
which is what `BuildRoleDependencies` consumes. Verified with a temporary role carrying
populated `env_run_lists`; probe role deleted afterwards.

Replaces 73,910 individual `GET /roles/<name>` calls with ~74 paginated requests.
Recovers ~7 min per cycle and cuts Chef server load by three orders of magnitude.

Unverified at scale, handle defensively: whether the role index enforces a lower `rows`
cap than nodes, and whether pagination-boundary duplication occurs as it does for nodes
(cf. `deduplicateSnapshotParams`).

Stopgap if this is deferred: raise `concurrency.role_fetching` from 10 (live-editable,
no restart). 20 workers ≈ 3m40s, 30 ≈ 2m25s. Trades Chef server load for time — watch
for `failed to fetch role` warnings.

### Collector streaming + per-batch commits

Scope: `collector.go:822-987`, `node_snapshots.go:259`. Makes peak memory O(page) rather
than O(fleet) — the only change that stops peak scaling with fleet size. Blockers, all
of which assume the complete node set is in hand:

- `deduplicateSnapshotParams` (pagination-boundary duplicates)
- cookbook aggregation: `allCookbookNames` / `activeCookbookNames` / `activeCookbookVersions`
- `nodeRecords` for usage analysis
- `snapshotParams` is read at `collector.go:1007/1045/1557`, so it stays pinned for the
  org's whole pipeline; needs projecting down to the fields those consumers use

Riskiest item here — shared collection path, silent-corruption failure modes. Own branch,
lab run before shipping.

### Audit remaining indexes for unbounded INCLUDE columns

Migration 0054 fixed `idx_node_readiness_target_name_eval`. Grep the other migrations for
`INCLUDE` lists containing JSONB or unbounded TEXT — the same failure mode is silent
until a row grows past 2704 bytes.

### Decouple log retention from collection runs

The purge only fires at the tail of a successful run (`collector.go:679`). `log_entries`
was seen with 240k dead tuples and autovacuum a day stale. Needs a ticker plus
partitioning so expiry is a `DROP PARTITION` rather than a `DELETE`.

### Retain collection history

`PurgeOldCollectionRuns` (`collector.go:689`) keeps only the latest terminal run per org,
so there is no duration trend to diagnose regressions against. Retain ~30 days.

### Document or fix the early completed_at stamp

`collection_runs.completed_at` is stamped at Step 4b (`collector.go:1022`), so its
duration covers only the node-snapshot phase and excludes Steps 5–16. Per-org totals are
now logged (v2.18.7), but the column still misleads anyone querying it directly.

### Release workflow: Node 20 deprecation

`softprops/action-gh-release` targets Node 20 and is being forced onto Node 24 by GitHub.
Bump the pinned action before support is withdrawn.
