# Fix: Stale filter + Role detail bugs

## Problem

Global filter "stale" sends `stale=stale` to `/api/v1/nodes`. The backend switch in `handle_nodes.go:parseNodeFilter` only handles `true`, `false`, `fresh`, `warning`, `critical` — `"stale"` falls through silently, returning all nodes unfiltered.

## Approach

Add `"stale"` as a recognized tier value in both the handler switch and the datastore SQL builder. "Stale" means warning + critical combined (ohai_time <= warning threshold).

## Specs to read

- `.claude/specifications/project-conventions.md` (for test patterns)

## Steps

1. Add `"stale"` case to switch in `handle_nodes.go:parseNodeFilter` (~line 532)
2. Add `"stale"` case to SQL builder in `node_snapshot_filter.go` (~line 596)
3. Write/update tests for both layers
4. Run tests, commit

## Bug 2: Role detail page — "Failed to get role detail"

### Root cause
`role_detail.go:275` references `server_cookbook_complexities` (plural) but the table is `server_cookbook_complexity` (singular). The JOIN always fails with a postgres error.

### Fix
Change `server_cookbook_complexities` → `server_cookbook_complexity` on line 275.

## Bug 3: Git repos list + dashboard TK summary show all as "untested"

### Root cause
Two endpoints never query `git_kitchen_results`:
1. `handleGitRepos` — `gitRepoResp` has no `tk_status` field
2. `handleDashboardTestKitchenCompatibility` — iterates repos and marks all as `untested`, never queries results to tally passed/failed

### Fix
- Add a datastore method to aggregate latest kitchen result per (repo, instance) → per-repo summary: total mapped, passed count, failed count
- Wire into `handleGitRepos`: add `tk_status` (passed/failed/untested) + `tk_passed`/`tk_total` fields
- Wire into `handleDashboardTestKitchenCompatibility`: tally passed/failed/untested repos using instance-level results
- Repo status logic: all pass → "passed", any fail → "failed", no results → "untested"
- Excluded/skipped instances don't count toward totals
- Also: rename "excluded" → "skipped" label in GitKitchenSection

## Steps

1. **Stale filter**: Add `"stale"` case to `handle_nodes.go` switch + `node_snapshot_filter.go` SQL
2. **Role detail**: Fix `server_cookbook_complexities` → `server_cookbook_complexity` in `role_detail.go`
3. **TK results aggregation**: Add datastore method for per-repo kitchen summary (total mapped, passed, failed)
4. **Git repos list**: Add `tk_status`, `tk_passed`, `tk_total` to `handleGitRepos` response; update frontend
5. **Dashboard TK card**: Wire kitchen results into `handleDashboardTestKitchenCompatibility`
6. **Excluded → skipped**: Rename label in `GitKitchenSection.tsx`
7. **Trends downsample**: Add daily-downsampled query, update 4 trend handlers
8. Tests + commit for each logical unit

## Bug 4: Dashboard trends graph shows only 1 day

### Root cause
Trend endpoints call `ListMetricSnapshotsByOrganisation(..., limit=10)`. With snapshots recorded every minute, all 10 results are from the last ~10 minutes — same day. Graph shows a single data point.

### Fix
Add a downsampled query that returns one snapshot per day (using `DISTINCT ON (date_trunc('day', snapshot_at))`) and use it in all trend endpoints. This affects:
- `handleDashboardStaleTrend` (stale/fresh)
- `handleDashboardComplexityTrend` (complexity)
- `handleDashboardVersionDistTrend` (version distribution)
- `handleDashboardReadinessTrend` (readiness)

## Acceptance criteria

- `stale=stale` query param returns only nodes with ohai_time <= warning threshold
- `stale=fresh` continues to work (regression check)
- Role detail page loads without error
- Git repos list shows correct TK status per repo
- Kitchen section shows "skipped" instead of "excluded"
- Dashboard trend graphs show data across multiple days
- Existing tests still pass
