# Filter Push-Down + UX Overhaul

## Goal

Fix broken pagination when readiness filter is active (client-side post-filter reduces visible rows, skips nodes on unseen pages). Then modernise the filter system per `filter-ux-overhaul.md`.

## Specs

- `.claude/specifications/filter-ux-overhaul.md` — full spec
- `.claude/specifications/web-api.md` §Filtering, §Filter Option Endpoints

## Steps

### Part A — Backend

1. ~~**Readiness filter SQL push-down**~~ ✅ Done
   - Added `ReadinessFilter` and `TargetChefVersion` to `NodeSnapshotFilter`
   - LEFT JOIN with `node_readiness` in `buildNodeSnapshotFilterParts`
   - Removed client-side `matchesReadinessFilter` and `displayNodes` post-filter
   - 8 new tests

2. ~~**Multi-value filter support**~~ ✅ Done
   - Added `Environments`, `Platforms`, `ChefVersions`, `PolicyNames`, `PolicyGroups` slice fields
   - `ANY($N)` SQL for multi-value, LIKE for single value
   - Comma-separated query param parsing in `nodeSnapshotFilterFromRequest`
   - 12 new tests (8 SQL builder, 4 request parsing)

3. ~~**Cookbooks SQL push-down**~~ ✅ Done
   - Created `CookbookFilter` struct and `ListCookbooksFiltered` in `cookbook_filter.go`
   - CTE with LEFT JOIN to cookstyle results, CASE for compatibility
   - Replaced entire in-memory pipeline in `handleCookbooks`
   - 12 new SQL builder tests, 7 handler tests updated

4. ~~**`?q=` on filter endpoints**~~ ✅ Done
   - Added `DistinctValueOpts` (SearchPrefix, Limit) to `ListDistinctNodeValues` and `ListDistinctNodeRoles`
   - All filter handlers parse `?q=` with Limit=50 when set
   - Backward compatible

### Part B — Frontend

5. ~~**`GlobalFilterContext`**~~ ✅ Done
   - Created `context/GlobalFilterContext.tsx` with URL param persistence
   - Refactored `useTargetChefVersion` to thin wrapper around context
   - All 6 pages using the hook get global state for free

6. ~~**New filter components**~~ ✅ Done
   - Created `FilterMultiCheckbox` — checkbox dropdown + removable chips
   - Created `FilterTypeAhead` — debounced `?q=` search + chips
   - Both follow existing Tailwind patterns

7. ~~**App integration**~~ ✅ Done
   - `GlobalFilterProvider` wrapping all protected routes in `App.tsx`
   - Global filter bar (target version + staleness) in `AppLayout` top bar
   - Per-page target version selectors still work via `useTargetChefVersion` wrapper

### Remaining (not yet started)

8. **Page filter migrations** — Replace per-page `FilterCombobox`/`FilterSelect` with `FilterMultiCheckbox` and `FilterTypeAhead` on NodesPage, CookbooksPage, GitReposPage. Remove per-page target version selectors (now in global bar). Wire staleStatus from global context into API calls.

## Acceptance Criteria

- ✅ Readiness filter returns correct pagination totals (no client-side post-filtering)
- ✅ Multi-select filters produce OR-within-dimension, AND-across-dimension results
- ✅ Cookbook list endpoint does not load all cookbooks into memory
- ✅ `?q=` on roles returns prefix-matched subset, ≤50 results
- ✅ Global filters persist across page navigation via URL params
- ✅ All existing Go and frontend tests pass; new tests for each step
- [ ] Pages use new multi-select filter components (step 8)