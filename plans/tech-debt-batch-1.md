# Plan: Tech Debt Batch 1

## Goal

Close ~9 low-risk, high-value tech debt items in a single branch.

## Specs to Read

- `todo-tech-debt.md` (already read)
- `project-conventions.md` (for Go/frontend patterns)

## Items (ordered by dependency/risk)

1. **B4b** — Remove dead `CountChefVersionsByCollectionRun` functions
2. **B6** — Deduplicate `nodeResp` struct in `handle_nodes.go`
3. **B9** — Log swallowed error in `WriteJSON`
4. **B13** — Remove useless compile-time check in `handle_cookbooks.go`
5. **B7** — Extract generic `PaginateSlice[T]` helper
6. **B17** — API response type for coverage endpoint (fix float64 leak)
7. **B11** — Reconcile `operator` role
8. **F2** — Add React Error Boundary
9. **B16** — Fix credential zeroing with `[]byte` pipeline

## Steps

For each item:
1. Read affected files
2. Write/update tests
3. Implement the fix
4. Run tests (`go test ./...` or `npm test`)
5. Commit with `fix(<scope>): <summary>`

## Acceptance Criteria

- All 9 items completed and committed
- `go build ./...` clean
- `go test ./...` all pass
- Frontend builds clean (`npm run build`)
- Tech debt list updated (items checked off / removed)