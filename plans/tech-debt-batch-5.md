# Tech Debt Batch 5

## Goal

Resolve 8 tech debt items across backend, frontend, and project hygiene.

## Items

| ID | Area | Summary |
|----|------|---------|
| B4 | Backend | Extract `resolveOwnershipKeys` helper — deduplicate ~25-line pattern across ~10 handlers |
| B2 | Backend | Push cookbook-by-node filtering into SQL — fix false-positive substring matching |
| F1 | Frontend | Extract shared `useSort` hook + `SortableColumnHeader` component from 8 pages |
| F3 | Frontend | Unify sort indicator visuals (bundled with F1) |
| F5 | Frontend | Extract `useTargetChefVersion` hook from 4+ pages |
| P2 | Project | Decide: populate or remove empty `internal/models/` |
| P3 | Project | Descope `internal/notify/` — update specs to mark as future work |

## Specs to Read

- `todo-tech-debt.md` (already read)
- `project-conventions.md` (already read)

## Ordered Steps

### B4 — Extract ownership filter helper

1. Add `resolveOwnershipFilter` method to `Router` in `handle_ownership.go`
   - Signature: `(r *Router) resolveOwnershipFilter(ctx context.Context, of ownerFilter, entityType string) (keys map[string]bool, err error)`
   - Consolidates the resolve + error-return pattern
2. Add `filterByOwnership[T any]` generic helper for the in-memory include/exclude loop
3. Replace all ~10 call sites across `handle_cookbooks.go`, `handle_dashboard_compatibility.go`, `handle_dashboard_platform.go`, `handle_dashboard_readiness.go`, `handle_dashboard_version.go`, `handle_dependency_graph.go`, `handle_nodes.go`, `handle_git_repos.go`
4. Write tests for the new helpers
5. Run `go test ./internal/webapi/...` — all must pass
6. Commit

### B2 — Push cookbook-by-node into SQL

1. Fix `nodeUsesCookbook` — currently uses `strings.Contains` which false-positives (`"apt"` matches `"apt-repo"`). Parse the JSONB and check for exact top-level key match.
2. Update existing tests and add a test for the false-positive case.
3. Run `go test ./internal/webapi/...` — all must pass.
4. Commit.

### F1 + F3 — Shared sort hook and component

1. Create `frontend/src/hooks/useSort.ts` — generic `useSort<T extends string>` hook returning `{ sortField, sortOrder, handleSort, sortIndicator }` with smart defaults (numeric columns descend, text columns ascend)
2. Create `frontend/src/components/SortableColumnHeader.tsx` — unified component using SVG chevrons (OwnersPage style) for consistent visuals (resolves F3)
3. Update all 8 pages: `NodesPage`, `CookbooksPage`, `GitReposPage`, `AdminSystemStatsPage`, `DependencyGraphPage`, `OwnersPage`, `NodeDiskDetailPage`, `CookbookCommittersPage`
4. Remove local `handleSort`, `sortIndicator`, `SortHeader`, `SortableHeader`, `SortableColHeader`, `SortIndicator` from each page
5. `npm run build` must succeed, spot-check in browser not required
6. Commit

### F5 — Extract useTargetChefVersion hook

1. Create `frontend/src/hooks/useTargetChefVersion.ts` — encapsulates `fetchFilterTargetChefVersions` + `highestSemver` pick + loading state
2. Update `NodesPage`, `CookbooksPage`, `GitReposPage`, `RemediationPage`, `CookbookRemediationPage`, `GitRepoRemediationPage`
3. `npm run build` must succeed
4. Commit

### P2 — Remove empty internal/models/

1. Remove the empty `internal/models/` directory
2. Update `project-conventions.md` to note types live in their domain packages (current reality)
3. Commit

### P3 — Descope internal/notify/

1. Update specs that reference `internal/notify/` to mark notifications as future/planned
2. Files to update: `secrets-storage.md`, `todo-visualisation.md`, `todo-secrets-storage.md`
3. Commit

### Wrap-up

1. Update `todo-tech-debt.md` — check off B4, B2, F1, F3, F5, P2, P3
2. Commit
3. Run full test suite (`go test ./...` + `npm run build`)
4. Delete this plan

## Acceptance Criteria

- All existing tests pass
- `go build ./...` and `npm run build` clean
- No new linter warnings
- Tech debt list reduced from 13 to 6 items