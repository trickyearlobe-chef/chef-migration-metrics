# Tech Debt Batch 3 — Low Priority Cleanup

## Goal

Resolve 6 of 8 🟢 Low tech debt items, reducing backlog from 24 → 18.

## Items

| Item | What | Files |
|------|------|-------|
| B12 | Remove deprecated `filterNodes` function | `handle_nodes.go` |
| F9 | Add `GitRepoFilterQuery` type | `frontend/src/api.ts`, `GitReposPage.tsx` |
| F12 | Centralise `perPage` constants | `frontend/src/constants.ts`, 10 page files |
| B14 | Add timeout bounds to background contexts | `handle_admin_rescan_all.go`, `handle_exports.go` |
| B15 | Make DB pool settings configurable | `internal/datastore/datastore.go`, `internal/config/config.go` |
| P7 | Split `handle_dashboard.go` | `internal/webapi/handle_dashboard.go` → multiple files |

## Specs to Read

- `todo-tech-debt.md` (already read)
- `project-conventions.md` (for naming/structure rules)
- `config.md` (for B15 config pattern)

## Order

1. **B12** — Remove deprecated `filterNodes`. Verify no callers, delete, run tests.
2. **F9** — Add `GitRepoFilterQuery` type in `api.ts`, use in `GitReposPage.tsx`.
3. **F12** — Add `constants.ts` with `DEFAULT_PAGE_SIZE`/`SMALL_PAGE_SIZE`, update all page files.
4. **B14** — Add `context.WithTimeout` to background goroutines in admin rescan and exports.
5. **B15** — Add pool settings to `DatastoreConfig`, wire into `datastore.go` `Open()`.
6. **P7** — Split `handle_dashboard.go` into focused files by endpoint group.

Each item gets its own commit after tests pass.

## Acceptance Criteria

- All 6 items resolved, removed from `todo-tech-debt.md`
- All Go tests pass (`go test ./...`)
- Frontend builds clean (`npm run build`)
- Go builds clean (`go build ./...`)
- No new lint warnings