# Filter Push-Down + UX Overhaul

## Goal

Fix broken pagination when readiness filter is active (client-side post-filter reduces visible rows, skips nodes on unseen pages). Then modernise the filter system per `filter-ux-overhaul.md`.

## Specs

- `.claude/specifications/filter-ux-overhaul.md` — full spec
- `.claude/specifications/web-api.md` §Filtering, §Filter Option Endpoints

## Steps

### Part A — Backend

1. **Readiness filter SQL push-down** — Add `ReadinessFilter` and `TargetChefVersion` fields to `NodeSnapshotFilter`. In `buildNodeSnapshotFilterParts`, JOIN `node_readiness` when either is set. Map filter values (`ready`, `blocked`, `cookbooks_blocked`, `disk_blocked`, `disk_unknown`) to SQL conditions on `is_ready`, `all_cookbooks_compatible`, `sufficient_disk_space`. Remove client-side `matchesReadinessFilter` and `displayNodes` post-filter in `NodesPage.tsx`.
   - Files: `node_snapshot_filter.go`, `node_snapshot_filter_test.go`, `handle_nodes.go`, `NodesPage.tsx`

2. **Multi-value filter support** — Change `Environment`, `Platform`, `ChefVersion`, `PolicyName`, `PolicyGroup` on `NodeSnapshotFilter` from `string` to `[]string`. Generate `ANY($N)` for multi-value, keep `LIKE` for single value. Parse comma-separated query params in `nodeSnapshotFilterFromRequest`.
   - Files: `node_snapshot_filter.go`, `node_snapshot_filter_test.go`, `handle_nodes.go`

3. **Cookbooks SQL push-down** — Create `CookbookFilter` struct and `ListCookbooksFiltered` query joining `server_cookbooks` with `server_cookbook_cookstyle_results`. Handle name, active, compatibility, download_status, org, sort, pagination in SQL. Replace in-memory pipeline in `handleCookbooks`.
   - Files: `cookbook_filter.go` (new), `cookbook_filter_test.go` (new), `handle_cookbooks.go`, `store.go`

4. **`?q=` prefix search on filter endpoints** — Add optional `q` param to `ListDistinctNodeValues` and `ListDistinctNodeRoles`. Apply `LOWER(col) LIKE LOWER($N) || '%'` and `LIMIT 50` when present. Wire in `handleFilterRoles` and others.
   - Files: `node_snapshot_filter.go`, `handle_filters.go`

### Part B — Frontend

5. **`GlobalFilterContext`** — React context for `targetChefVersion` + `staleStatus`, persisted in URL params. Provided at app root. Retire `useTargetChefVersion` hook (or reduce to thin wrapper).
   - Files: `context/GlobalFilterContext.tsx` (new), `App.tsx`, `useTargetChefVersion.ts`

6. **New filter components** — `FilterMultiCheckbox` (checkbox dropdown + chips), `FilterTypeAhead` (debounced `?q=` search). Add multi-select mode to `FilterCombobox`.
   - Files: `components/FilterMultiCheckbox.tsx` (new), `components/FilterTypeAhead.tsx` (new), `components/FilterInputs.tsx`

7. **Page migrations** — Update `NodesPage`, `CookbooksPage`, `GitReposPage` to use `GlobalFilterContext`, new filter components, and multi-value query params. Remove per-page target version state.
   - Files: `NodesPage.tsx`, `CookbooksPage.tsx`, `GitReposPage.tsx`

## Acceptance Criteria

- Readiness filter returns correct pagination totals (no client-side post-filtering)
- Multi-select filters produce OR-within-dimension, AND-across-dimension results
- Cookbook list endpoint does not load all cookbooks into memory
- `?q=` on roles returns prefix-matched subset, ≤50 results
- Global filters persist across page navigation via URL params
- All existing Go and frontend tests pass; new tests for each step