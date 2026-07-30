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

### Collector streaming + per-batch commits

Scope: `collector.go:822-987`, `node_snapshots.go:259`. Makes peak memory O(page) rather
than O(fleet) — the only change that stops peak scaling with fleet size. Blockers, all
of which assume the complete node set is in hand:

- `deduplicateSnapshotParams` (pagination-boundary duplicates)
- cookbook aggregation: `allCookbookNames` / `activeCookbookNames` / `activeCookbookVersions`
- `nodeRecords` for usage analysis
- `snapshotParams` stays pinned for the org's whole pipeline — it is read repeatedly
  after the bulk insert (around `collector.go:1005`, `1012`, `1032-1034`, `1071`, `1593`,
  `1796`); needs projecting down to the fields those consumers actually use

Riskiest item here — shared collection path, silent-corruption failure modes. Own branch,
lab run before shipping.

### Retain collection history

There is no duration trend to diagnose regressions against, because the **write model is
an upsert**: `collection_runs` holds at most one row per organisation.
`PurgeOldCollectionRuns` (`collection_runs.go:428`) is a no-op retained for backward
compatibility — it is not why history is missing.

So this is a schema and write-path change (a row per run, or a history table, plus a real
purge), not a retention-policy tweak. Size it accordingly.

### Document or fix the early completed_at stamp

`collection_runs.completed_at` is stamped at Step 4b (`collector.go:1047-1053`), so its
duration covers only the node-snapshot phase and excludes the remaining steps through
Step 16 (`collector.go:1613`). Per-org totals are
now logged (v2.18.7), but the column still misleads anyone querying it directly.

### Conflicts with runtime-observability Chunk 4

Collector streaming and runtime-observability Chunk 4 (collection progress state) both
rewrite the same region of `collector.go` around `snapshotParams` lifetime. Decide which
lands first; they cannot proceed in parallel.

### Release workflow: Node 20 deprecation

`softprops/action-gh-release` targets Node 20 and is being forced onto Node 24 by GitHub.
Bump the pinned action before support is withdrawn.
