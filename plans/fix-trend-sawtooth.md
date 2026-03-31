# Fix: Trend Chart Sawtooth Aggregation

## Goal

Eliminate the sawtooth pattern on dashboard trend charts when viewing all orgs. Per-org metric snapshots are plotted as individual X-axis points instead of being summed per collection cycle.

## Root Cause

Backend returns one data point per (org, snapshot_time). Frontend sorts by time and plots each as a separate point — so the chart bounces between different-sized orgs.

## Approach

Backend aggregation in the trend handlers. Per-org snapshots already have accurate counts. When no `organisation` filter is set, group snapshots whose `snapshot_at` falls in the same hour and sum distributions/counts across orgs. Return one merged point per time bucket. Single-org view unchanged.

## Affected Handlers

- `handleDashboardVersionDistributionTrend` — sum `distribution` maps, sum `total_nodes`
- `handleDashboardStaleTrend` — sum `stale_nodes`, `fresh_nodes`, `total_nodes`
- `handleDashboardReadinessTrend` — sum `ready`, `blocked`, `total_nodes`

## Specs to Read

- `.claude/specifications/visualisation.md` — Historical Trending section

## Steps

1. Add a shared helper `groupSnapshotsByHour` that buckets `[]MetricSnapshot` by `snapshot_at` truncated to the hour.
2. Add per-type merge functions: merge version distributions (sum map values), merge stale counts, merge readiness counts.
3. Write tests for the merge/grouping logic.
4. Update `handleDashboardVersionDistributionTrend` non-ownership path: fetch all orgs' snapshots, group by hour, merge, return one point per bucket.
5. Update `handleDashboardStaleTrend` likewise.
6. Update `handleDashboardReadinessTrend` likewise.
7. Run all backend tests.
8. Verify frontend needs no changes (it already sorts by timestamp).
9. Update todo-visualisation.md.

## Acceptance Criteria

- All-orgs view: one data point per collection cycle with summed counts.
- Single-org view: unchanged behaviour.
- Ownership-filtered paths: unaffected.
- All existing trend tests pass; new tests cover multi-org merge.
- No frontend changes required.