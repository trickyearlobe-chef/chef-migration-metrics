# Plan — Tech Debt Batch 6 (Final Cleanup)

## Goal

Resolve all remaining tech debt items except B0 (UUIDs→natural keys), which
is XL and requires its own specification and multi-session migration plan.

## Items

| ID | Summary | Size | Approach |
|----|---------|------|----------|
| P8 | Re-enable errcheck linter | S | 0 violations remain — just remove the disable line from `.golangci.yml` and confirm lint passes |
| P1 | Create CHANGELOG.md | M | Generate from `git tag` + `git log` history; group by version, conventional-commit type |
| F4 | Extract shared filter input components | M | Move `FilterInput`, `FilterSelect`, `FilterCombobox` from `NodesPage.tsx` to `frontend/src/components/FilterInputs.tsx`; update all pages that inline the same className pattern |
| F7 | Add frontend tests | M | Install Vitest + Testing Library; add unit tests for `useSort`, `useTargetChefVersion`, `FilterInput`, `FilterSelect`, `FilterCombobox` |
| F6 | Split large monolithic page files | M | Split `DependencyGraphPage.tsx` (7 components, ~1650 lines) and `DashboardPage.tsx` (11 card components, ~1060 lines) into sub-files |
| B4a | Readiness trend snapshots | L | Record `readiness_summary` metric snapshot in `recordMetricSnapshots`; switch `handleDashboardReadinessTrend` to read from snapshots instead of live `CountNodeReadiness` |
| B5 | Datastore tests | L | Add functional tests (build tag `functional`) for the 15 uncovered files following the `functional_test.go` pattern |

## Specs to read

- `todo-tech-debt.md` (current state)
- `project-conventions.md` (naming, file layout)
- `collection-dashboard-isolation.md` (B4a context)
- `datastore.md` (B5 context)

## Ordered steps

1. **P8** — Remove errcheck disable from `.golangci.yml`, run `golangci-lint run`, confirm 0 errcheck issues, fix any staticcheck/govet issues found. Commit.
2. **P1** — Generate `CHANGELOG.md` from git tags. Commit.
3. **F4** — Extract `FilterInput`, `FilterSelect`, `FilterCombobox` to shared component file. Update all consuming pages. Run `npm run build`. Commit.
4. **F7** — Install Vitest + @testing-library/react. Add tests for hooks and filter components. Wire `npm test` to vitest. Commit.
5. **F6** — Split `DependencyGraphPage.tsx` into `pages/dependency-graph/` directory. Split `DashboardPage.tsx` into `pages/dashboard/` directory. Update router imports. Run `npm run build`. Commit.
6. **B4a** — Add `readiness_summary` snapshot recording in collector. Switch trend handler to read snapshots. Add/update tests. Run `go test ./...`. Commit.
7. **B5** — Add functional tests for uncovered datastore files. Run with `go test -tags=functional ./internal/datastore/` (or confirm they compile without the tag). Commit.
8. Update `todo-tech-debt.md` — check off all resolved items, leave only B0. Commit.
9. Remove this plan. Commit.

## Acceptance criteria

- `golangci-lint run` clean (errcheck re-enabled)
- `go test ./...` all green
- `go build ./...` clean
- `npm run build` clean
- `npm test` runs real tests and passes
- `CHANGELOG.md` exists with entries for all 46 tags
- Tech debt list reduced to B0 only (+ B5 if functional tests can't be verified without DB)

## Deferred

- **B0** (UUIDs→natural keys) — requires its own specification and phased migration plan. Out of scope for this batch.