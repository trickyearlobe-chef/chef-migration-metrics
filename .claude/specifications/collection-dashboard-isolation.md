# Collection–Dashboard Isolation

Fixes three related bugs that share a root cause: dashboard reads are not isolated from in-progress collection writes.

## Bug Summary

### Bug 1 — Sawtooth Trend Graphs

**Symptom:** Ownership-filtered version-distribution trend graphs show a sawtooth pattern — counts drop sharply when collection starts, then recover when it completes.

**Root cause:** The ownership-filtered trend path in `handleDashboardVersionDistributionTrend` queries `node_snapshots` by `collection_run_id`. During collection, `BulkUpsertNodeSnapshots` progressively updates each node's `collection_run_id` to the current run. A mid-collection dashboard query sees only the nodes already upserted — a partial count. The unfiltered path reads from `metric_snapshots` (append-only, written once at completion) and is unaffected.

**Affected code:**
- `internal/webapi/handle_dashboard.go` — ownership-filtered branch of `handleDashboardVersionDistributionTrend` (L270+)
- `internal/datastore/node_snapshots.go` — `CountChefVersionsByCollectionRun`, `CountChefVersionsByCollectionRunFiltered`

### Bug 2 — Summary/Detail Count Mismatch

**Symptom:** Dashboard summary numbers (version distribution, readiness) don't match drill-down detail page counts.

**Root cause:** Summary endpoints (`handleDashboardVersionDistribution`) query the live `node_snapshots` table. Detail pages also query `node_snapshots` but may use different query paths (by UUID vs by name). When collection is in progress, the summary and detail queries can see different subsets of nodes depending on timing and which nodes have been upserted with the new `collection_run_id`. This is amplified by the UUID drift problem documented in tech debt item B0.

**Affected code:**
- `internal/webapi/handle_dashboard.go` — all summary endpoints that query `node_snapshots`
- `internal/webapi/handle_nodes.go` — detail queries

### Bug 3 — Context Timeout Stops Ingest

**Symptom:** Collection stops mid-run on large fleets, leaving `node_snapshots` in a partially-updated state.

**Root cause:** The scheduler creates a context with `context.WithCancel(context.Background())` — no timeout. But HTTP request contexts or other deadlines may propagate. When collection is interrupted, some nodes have the new `collection_run_id` and some still have the old one. This amplifies bugs 1 and 2 because the partial state persists until the next successful full collection.

**Affected code:**
- `internal/collector/scheduler.go` — context creation at L120
- `internal/collector/collector.go` — `collectOrganisation` deferred cleanup (L706–723)

## Fix Strategy

The fix addresses all three bugs with a single architectural change: **make dashboard reads independent of in-progress collection state.**

### Approach: Enrich `metric_snapshots` With Per-Node Data

Currently `metric_snapshots` stores only aggregate counts (version distribution totals). The ownership-filtered trend path cannot use it because it needs per-node granularity to apply ownership filtering.

**The fix:** Store per-node version data in the `metric_snapshots` JSONB payload so that ownership filtering can be applied to the snapshot data without touching `node_snapshots`.

### Changes Required

### Collector — Enrich Metric Snapshot Payload

In `recordMetricSnapshots`, add a `nodes` array to the `chef_version_distribution` JSONB payload containing each node's name and version. This enables ownership filtering against snapshot data.

**Current payload structure:**
```
{"distribution": {"18.5.0": 100, "17.0.0": 50}, "total_nodes": 150, "stale_nodes": 5, "fresh_nodes": 145}
```

**New payload structure:**
```
{"distribution": {"18.5.0": 100, "17.0.0": 50}, "total_nodes": 150, "stale_nodes": 5, "fresh_nodes": 145, "nodes": [{"name": "web01", "version": "18.5.0"}, {"name": "db01", "version": "17.0.0"}, ...]}
```

The `nodes` array is only used when ownership filtering is active. The pre-aggregated `distribution` map continues to serve unfiltered queries.

### Dashboard Trend Handler — Use Metric Snapshots for All Paths

Rewrite `handleDashboardVersionDistributionTrend` so both the unfiltered and ownership-filtered paths read from `metric_snapshots`.

**Unfiltered path** (unchanged): Read `distribution` from JSONB, return directly.

**Ownership-filtered path** (changed): Read `nodes` array from JSONB, apply ownership include/exclude filtering in memory, re-aggregate into a distribution map.

This eliminates the `CountChefVersionsByCollectionRun` and `CountChefVersionsByCollectionRunFiltered` calls from the trend handler entirely.

### Dashboard Summary Handler — Guard Against Mid-Collection Reads

For `handleDashboardVersionDistribution` and `handleDashboardVersionDistributionWithOwnerFilter`, add a guard that checks whether a collection is currently running for each org. If so, serve summary data from the most recent completed `metric_snapshots` row instead of querying live `node_snapshots`.

This ensures summary and detail counts are consistent: both reflect the last completed collection, not a partially-updated in-progress state.

### Collection Run Resilience

Improve the collector's handling of interrupted runs so that partial `node_snapshots` updates don't leave the database in an inconsistent state visible to dashboard queries:

- When a collection run is interrupted or fails, the `collection_run_id` on partially-updated nodes is stale but harmless because dashboard queries no longer depend on it for trend data.
- The existing "early completion" pattern (mark run completed after node snapshots, before cookbooks) is fine because `metric_snapshots` are recorded at that same point.

No schema changes are required for this aspect — the fix is entirely in the query path.

## Constraints

- No new database migrations. The change is to the JSONB payload content only.
- The `nodes` array in `metric_snapshots` must be bounded. For organisations with >100K nodes, this could produce large JSONB values. If `len(snapshotParams) > 50000`, omit the `nodes` array and log a warning. The ownership-filtered trend path falls back to "data not available for ownership filtering" for that snapshot.
- `CountChefVersionsByCollectionRun` and `CountChefVersionsByCollectionRunFiltered` remain in the codebase (other callers may exist) but are no longer called from trend handlers.
- Existing `metric_snapshots` rows without the `nodes` field must be handled gracefully — the ownership-filtered path treats them as "no ownership data available" and skips them.
- The readiness trend handler (`handleDashboardReadinessTrend`) has a separate but related issue — it queries live `CountNodeReadiness` rather than using metric snapshots. This spec does NOT cover enriching readiness metric snapshots (that is a follow-up). The readiness trend is less affected because it doesn't use `collection_run_id` for its queries.

## Acceptance Criteria

- Ownership-filtered version-distribution trend returns stable counts with no sawtooth during active collection.
- Unfiltered trend behaviour is unchanged (already reads from `metric_snapshots`).
- Dashboard version-distribution summary numbers match the most recent completed collection when a collection is in progress.
- Mid-collection or interrupted collection runs do not cause partial counts in any dashboard endpoint covered by this fix.
- Existing `metric_snapshots` rows without `nodes` field are handled gracefully (skipped for ownership-filtered trend, used normally for unfiltered trend).
- Large organisations (>50K nodes) degrade gracefully — ownership trend data is unavailable rather than causing OOM or excessive JSONB sizes.
- All existing tests continue to pass.
- New tests cover: ownership-filtered trend from metric snapshots, mid-collection summary guard, large-org fallback, backward-compatible JSONB parsing.

## Out of Scope

- Readiness trend enrichment (follow-up spec).
- UUID-to-natural-key migration (tech debt B0 — separate effort).
- Context timeout hardening for the collector (related but orthogonal — the fix here makes timeout consequences less visible to dashboard users).
- Removing `CountChefVersionsByCollectionRun*` functions (may have other callers).