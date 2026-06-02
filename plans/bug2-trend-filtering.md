# Bug 2 — Trend Staleness/Platform Filtering

## Goal

Trend graphs show data broken down by staleness tier. Fresh-node readiness includes blocking-reason breakdown (cookstyle, TK, disk). Pure aggregates — no per-node JSONB blobs.

## Specs to Read

- `specifications/enriched-metric-snapshots.md` (primary)
- `specifications/staleness-tiers.md` (staleness computation)
- `specifications/semantic-contracts.md` (canonical definitions)

## Steps

1. Write tests for `buildNodeMetricsPayload` (TDD)
2. Create payload builder — computes all aggregates from node params + readiness results
3. Remove `maxNodesInMetricSnapshot` constant (no longer needed)
4. Consolidate `recordMetricSnapshots` + `recordReadinessSnapshots` into `recordNodeMetricsSnapshot` (called after readiness)
5. Write `node_metrics` snapshot type alongside legacy types (transition period)
6. Write tests for trend handlers reading `node_metrics` format
7. Update readiness trend handler — read from `node_metrics`, fall back to legacy
8. Update version-distribution trend handler — same pattern
9. Update stale trend handler — read `by_staleness` from `node_metrics`
10. Add `?stale=` param to readiness/version trend endpoints (filter which tier's data to return)
11. Frontend: pass staleness filter to trend API calls
12. Integration tests

## Deferred

- Platform filtering within fresh (needs per-platform-per-tier cross-tab)
- Re-aggregation on threshold change
- Ownership on historical trends

## Acceptance Criteria

- `node_metrics` snapshot written each collection with ~2-5 KB payload
- Trend endpoints accept `?stale=` param (default: `fresh`)
- Readiness trend shows `blocked_by` breakdown (cookstyle, TK, disk)
- Legacy snapshots handled gracefully (`filter_limited: true`)
- All existing tests pass
- `foodcritic` and `chefspec` placeholders present (zero values)
