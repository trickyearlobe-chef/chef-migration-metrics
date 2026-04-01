# Version Distribution Enhancements — Component Specification

> **TL;DR:** Add two enhancements to the Chef Client Version Distribution and OS Platform Distribution dashboard cards: (1) a toggle to aggregate by full semver or major version only, and (2) a stacked active/stale node breakdown in each bar. Scoped to point-in-time bar charts only — historical trend charts are out of scope.

## Scope

### In Scope

- Chef Client Version Distribution card (`VersionDistributionCard`)
- OS Platform Distribution card (`PlatformDistributionCard`)
- Point-in-time bar chart views only
- Both the unfiltered SQL path and the ownership-filtered in-memory path

### Out of Scope

- Version Distribution Trend card (historical stacked area chart)
- Stale Trend card
- Metric snapshot JSONB payload changes (not needed for point-in-time views)
- Elasticsearch/Kibana dashboard changes

---

## Feature 1: Major/Full Semver Aggregation Toggle

### Behaviour

- A toggle control (segmented button or dropdown) on each card allows the user to switch between:
  - **Full version** (default) — group by the complete version string as today (e.g. `18.5.0`, `17.0.0`, `ubuntu 22.04`)
  - **Major version** — group by major version number only (e.g. `18`, `17`, `ubuntu`)
- The toggle state is local to each card (not persisted across page loads).
- When set to "Major version", the API is called with `?group_by=major` and the backend performs the aggregation.

### Backend Changes

#### API Query Parameter

Both endpoints accept an optional `group_by` query parameter:
- `GET /api/v1/dashboard/version-distribution?group_by=major`
- `GET /api/v1/dashboard/platform-distribution?group_by=major`

Valid values: `full` (default), `major`. Invalid values are ignored (treated as `full`).

#### `internal/datastore/node_snapshot_filter.go`

Modify `CountNodeVersionDistribution` and `CountNodePlatformDistribution` to accept a `groupByMajor bool` parameter (or equivalent), and adjust the SQL expression passed to `countNodeDistribution`:

- **Version distribution, full:** `COALESCE(NULLIF(cn.chef_version, ''), 'unknown')` (unchanged)
- **Version distribution, major:** `COALESCE(NULLIF(SPLIT_PART(cn.chef_version, '.', 1), ''), 'unknown')`
- **Platform distribution, full:** existing `CASE` expression (unchanged)
- **Platform distribution, major:** `COALESCE(NULLIF(cn.platform, ''), 'unknown')` (drops `platform_version`)

#### `internal/webapi/handle_dashboard_version.go`

- Parse `group_by` query parameter.
- Pass to `CountNodeVersionDistribution`.
- For the mid-collection metric snapshot fallback path (`versionDistFromMetricSnapshots`): re-aggregate the `distribution` map keys by extracting major version (`strings.SplitN(ver, ".", 2)[0]`).
- For the ownership-filtered in-memory path: apply the same major-version extraction when building the `counts` map.

#### `internal/webapi/handle_dashboard_platform.go`

- Parse `group_by` query parameter.
- Pass to `CountNodePlatformDistribution`.
- For the ownership-filtered in-memory path: when `group_by=major`, use only `n.Platform` (skip appending `n.PlatformVersion`).

### Frontend Changes

#### `frontend/src/api.ts`

- `fetchVersionDistribution(organisation?, groupBy?)` — add optional `group_by` parameter to URL.
- `fetchPlatformDistribution(organisation?, groupBy?)` — same.

#### `frontend/src/pages/dashboard/StatusCards.tsx`

- Add a `groupBy` state (`"full" | "major"`) to `VersionDistributionCard` and `PlatformDistributionCard`.
- Render a toggle control (two-button segmented control) above the bar chart: **Full** | **Major**.
- Pass `groupBy` to the fetch function and include it in the `useCallback` dependency array.

#### `frontend/src/types.ts`

No changes — response shape is identical regardless of aggregation level.

---

## Feature 2: Active/Stale Node Breakdown in Bar Chart

### Behaviour

- Each bar in the version and platform distribution charts is split into two segments:
  - **Active** (blue/purple, left portion) — nodes where `is_stale = false`
  - **Stale** (amber/muted, right portion) — nodes where `is_stale = true`
- The count label shows the total. A tooltip or legend indicates the active/stale split.
- When all nodes in a version are active, the bar appears as a single solid colour (no visual change from today).

### Backend Changes

#### Response Shape

The `distribution` array entries gain two new fields. Both endpoints (`version-distribution` and `platform-distribution`) return:

```json
{
  "total_nodes": 150,
  "distribution": [
    {
      "version": "18.5.0",
      "count": 100,
      "percent": 66.7,
      "active_count": 92,
      "stale_count": 8
    }
  ]
}
```

Platform distribution uses `"platform"` instead of `"version"` as the label key (unchanged).

#### `internal/datastore/node_snapshot_filter.go`

Replace the `countNodeDistribution` return type from `map[string]int` to a richer structure:

```go
type DistributionEntry struct {
    Total  int
    Active int
    Stale  int
}
```

Update the SQL to use conditional aggregation:

```sql
SELECT {expr} AS {alias},
       COUNT(*) AS cnt,
       COUNT(*) FILTER (WHERE NOT cn.is_stale) AS active_cnt,
       COUNT(*) FILTER (WHERE cn.is_stale) AS stale_cnt
  FROM completed_nodes cn
 {where}
 GROUP BY {alias}
 ORDER BY cnt DESC, {alias} ASC
```

Update `CountNodeVersionDistribution` and `CountNodePlatformDistribution` signatures to return `map[string]DistributionEntry`.

#### `internal/webapi/handle_dashboard_version.go`

- Update `versionDistEntry` to include `ActiveCount int` and `StaleCount int` JSON fields.
- Update `buildDistributionResponse` generic helper (in `handle_dashboard_platform.go`) — the `makeFn` callback receives the `DistributionEntry` instead of a plain `int` count.
- Mid-collection metric snapshot fallback: the existing JSONB payload has aggregate `stale_nodes`/`fresh_nodes` but not per-version breakdown. For this path, return `active_count`/`stale_count` as 0 (or omit) and let the frontend render a single-colour bar. This edge case only occurs during a running collection.
- Ownership-filtered in-memory path: each `NodeSnapshot` has `IsStale` — accumulate per-version active/stale counts trivially.

#### `internal/webapi/handle_dashboard_platform.go`

- Same treatment as version distribution. Update `platformCount` struct with `ActiveCount`/`StaleCount`.
- Update `buildDistributionResponse` helper signature (it's generic, so the change propagates).

### Frontend Changes

#### `frontend/src/types.ts`

```typescript
export interface VersionCount {
  version: string;
  count: number;
  percent: number;
  active_count: number;
  stale_count: number;
}

export interface PlatformCount {
  platform: string;
  count: number;
  percent: number;
  active_count: number;
  stale_count: number;
}
```

#### `frontend/src/pages/dashboard/StatusCards.tsx`

Replace the single `bar-chart-fill` div with two adjacent divs inside `bar-chart-track`:

```tsx
<div className="bar-chart-track">
  <div
    className="bar-chart-fill bg-blue-500"
    style={{ width: `${activePct}%` }}
  />
  {stalePct > 0 && (
    <div
      className="bar-chart-fill bg-amber-300"
      style={{ width: `${stalePct}%` }}
    />
  )}
</div>
```

Add a legend below each card header: ● Active ● Stale.

Ensure the `bar-chart-fill` CSS class does not set `border-radius` on the right side of the first segment or the left side of the second segment when stacked.

#### `frontend/src/api.ts`

No changes — same endpoints, just richer response.

---

## Testing

### Backend

- `node_snapshot_filter_test.go`: Test `countNodeDistribution` with mixed stale/active nodes returns correct breakdown. Test `group_by=major` collapses versions correctly.
- `handle_dashboard_test.go`: Test version-distribution endpoint with `?group_by=major` returns collapsed versions. Test response includes `active_count` and `stale_count` fields.
- `collector_test.go`: No changes needed (metric snapshot payload format unchanged).

### Frontend

- Verify toggle switches between full and major aggregation.
- Verify stacked bars render correctly with mixed active/stale.
- Verify single-colour bar when all nodes are active.
- Verify tooltip/legend shows active/stale counts.

---

## Acceptance Criteria

1. Version Distribution card has a Full/Major toggle that changes the grouping level.
2. Platform Distribution card has a Full/Major toggle that changes the grouping level.
3. Each bar in both cards shows a stacked active (coloured) / stale (amber) breakdown.
4. A legend indicates the meaning of the two bar segments.
5. The total count label remains unchanged.
6. Clicking a bar still navigates to the filtered node list.
7. No changes to trend charts or historical data.
8. Existing tests continue to pass; new tests cover the added functionality.