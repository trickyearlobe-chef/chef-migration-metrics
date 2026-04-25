# Trend Bugs + api/types Split + Battery Bars

## Goal

Fix two trend card bugs (complexity, readiness), split `api.ts`/`types.ts` by domain, implement version distribution battery bars.

## Specs to Read

- `.claude/specifications/version-battery-bars.md` (read)
- `.claude/specifications/todo-tech-debt.md` (read)
- `.claude/specifications/project-conventions.md` (read)

## Steps

### Phase 1: Complexity Trend Bug

1. Add `buildComplexitySnapshotPayload` in `collector.go` — aggregates `ServerCookbookComplexity` rows per target version into JSON payload matching the fields the frontend expects (total_cookbooks, total_score, average_score, low/medium/high/critical counts).
2. Add `recordComplexitySnapshots` in `collector.go` — calls `ListServerCookbookComplexitiesByOrganisation`, groups by target version, writes `complexity_summary` snapshots via `InsertMetricSnapshot`. Call after complexity scoring completes (~L1128).
3. Write tests for `buildComplexitySnapshotPayload`.
4. Add `complexityTrendPoint` to `trend_aggregation.go` with `completed_at` field; add `mergeComplexityTrendSnapshots`.
5. Write tests for merge function.
6. Rewrite `handleDashboardComplexityTrend` in `handle_dashboard_trends.go` to read from `metric_snapshots` (type `complexity_summary`) with fallback to live query. Follow same pattern as readiness trend handler.
7. Write handler test.
8. Update `ComplexityTrendPoint` in `types.ts` — add `completed_at: string` and `collection_run_org: string`.
9. Rewrite `ComplexityTrendCard` in `TrendCards.tsx` to use real `completed_at` timestamps (same pattern as version distribution and stale cards).

### Phase 2: Readiness Trend Timestamp Bug

10. Update `ReadinessTrendPoint` in `types.ts` — add `completed_at: string` and `collection_run_org: string`.
11. Rewrite `ReadinessTrendCard` in `TrendCards.tsx` to use real `completed_at` timestamps, remove fake `Date.now()` synthesis and stale comment on L113.

### Phase 3: Split `types.ts` by Domain

12. Create `frontend/src/types/` directory with domain files: `common.ts`, `dashboard.ts`, `nodes.ts`, `cookbooks.ts`, `git-repos.ts`, `kitchen.ts`, `admin.ts`, `auth.ts`, `ownership.ts`, `exports.ts`, `remediation.ts`, `dependencies.ts`, `logs.ts`, `websocket.ts`, `credentials.ts`, `config.ts`.
13. Move interfaces into domain files. Keep `types.ts` as barrel re-export for backward compatibility.
14. Verify frontend builds cleanly.

### Phase 4: Split `api.ts` by Domain

15. Create `frontend/src/api/` directory with domain files matching `types/` domains, plus `client.ts` for `apiFetch`, `ApiError`, `buildUrl`, `BASE`.
16. Move functions into domain files. Keep `api.ts` as barrel re-export.
17. Verify frontend builds cleanly.

### Phase 5: Battery Bars

18. Write grouping logic + unit tests (`groupByMajorVersion` in a utility file).
19. Write colour assignment logic + unit tests (relative major version → base colour, shade generation for minors).
20. Build `BatteryBarChart` component with tests (rendering, accordion, keyboard, ARIA).
21. Add `.battery-bar-*` CSS classes to `index.css`.
22. Integrate into `VersionDistributionCard` replacing current per-version bars.
23. Write integration test for `VersionDistributionCard` with battery bars.

### Phase 6: Cleanup

24. Update `todo-tech-debt.md` — remove resolved items (complexity trend bug, readiness trend bug, api.ts split, types.ts split).
25. Run full test suite, verify no regressions.
26. Delete this plan.

## Acceptance Criteria

- Complexity trend card shows multi-point trend line from `metric_snapshots` with real timestamps.
- Readiness trend card uses real `completed_at` timestamps from backend.
- `api.ts` and `types.ts` each under 100 lines (barrel re-exports only); domain files each under 500 lines.
- Battery bar chart renders grouped major versions with expand/collapse, keyboard nav, ARIA.
- All existing tests pass; new tests for all new code.
- Tech debt list updated.