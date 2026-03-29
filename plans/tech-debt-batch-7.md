# Plan: Tech Debt Batch 7 — B4a, B5, F6

## Goal

Resolve 3 of 4 remaining tech debt items. Defer B0 (UUIDs → natural keys) — XL scope, needs own spec + phased migration.

## Items

| ID | Summary | Size |
|----|---------|------|
| **B4a** | Readiness trend → metric snapshots | M |
| **B5** | Datastore tests | L |
| **F6** | Split DependencyGraphPage + DashboardPage | M |

## Specs to Read

- `todo-tech-debt.md` — item definitions
- `datastore.md` — `metric_snapshots` table, `readiness_summary` JSONB schema
- `collection-dashboard-isolation.md` — context on sawtooth bug + snapshot strategy
- `project-conventions.md` — naming, test patterns

## Steps

### 1. Branch

Create `fix/tech-debt-batch-7` from `main`.

### 2. B4a — Readiness trend metric snapshots

Key insight: readiness evaluation runs at Step 14 (after CookStyle + TK), but `recordMetricSnapshots` runs at Step 4c (after node collection). Readiness data is not yet available at 4c.

- Add `recordReadinessSnapshot` to `collector.go`, called after Step 14 completes.
- Build `readiness_summary` JSONB payload: `{ total_nodes, ready, blocked, nodes: [{name, is_ready}] }` per target version. Mirror the `nodes` / `nodes_omitted` pattern from `chef_version_distribution`.
- Insert with `snapshot_type: "readiness_summary"` and `target_chef_version` set.
- Rewrite `handleDashboardReadinessTrend` to read from `ListMetricSnapshotsByOrganisationAndVersion` instead of calling `CountNodeReadiness`. Unmarshal JSONB, aggregate across orgs, return trend points with `completed_at` timestamps.
- Write tests first: collector payload builder test, handler tests with mock snapshots.
- Commit.

### 3. B5 — Datastore tests

22 source files lack tests. All SQL queries need live Postgres (`CMM_TEST_DATABASE_URL`). Pure-function / validation tests can run without Postgres.

- Audit each untested file for testable pure logic (validation, parameter building, scan helpers).
- Write unit tests for validation paths (`InsertMetricSnapshot` param validation, `UpsertNodeReadiness` param validation, etc.) that don't need Postgres.
- Add integration test stubs gated on `CMM_TEST_DATABASE_URL` for CRUD operations in `functional_test.go` or per-file test files.
- Target: cover validation logic in all files, integration stubs for the most critical (metric_snapshots, node_readiness, node_snapshots, organisations, owners).
- Commit.

### 4. F6 — Split large page files

**DependencyGraphPage.tsx** (1,646 lines, 7 components):
- Create `frontend/src/pages/dependency-graph/` directory.
- Extract: `GraphView`, `ForceGraph`, `SelectedNodePanel`, `TableView`, `SharedCookbooksCard`, `TableRow` into separate files.
- Keep `DependencyGraphPage` as the shell in `index.tsx`.

**DashboardPage.tsx** (1,061 lines, 11 card components):
- Create `frontend/src/pages/dashboard/` directory.
- Extract each card: `VersionDistributionCard`, `PlatformDistributionCard`, `ReadinessCard`, `CookbookCompatibilityCard`, `GitRepoCompatibilityCard`, `TestKitchenCompatibilityCard`, `VersionDistributionTrendCard`, `ReadinessTrendCard`, `ComplexityTrendCard`, `StaleTrendCard`.
- Keep `DashboardPage` shell in `index.tsx`.

- Update imports in consuming files (router, etc.).
- Run `npm run build` + `npm test` to verify.
- Commit.

### 5. Update tech debt list

- Check off B4a, B5, F6.
- Remove completed items' detail text (keep checked line only).
- B0 remains as sole open item.
- Commit.

### 6. Clean up

- Delete this plan.
- Commit.

## Acceptance Criteria

- `go test ./...` — all pass
- `go build ./...` — clean
- `golangci-lint run ./...` — 0 issues
- `npm run build` — clean
- `npm test` — all pass
- Readiness trend endpoint returns snapshot-based data with timestamps
- No regressions in existing handler tests
- DependencyGraphPage.tsx and DashboardPage.tsx each < 200 lines after split