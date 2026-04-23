# Plan: Phase 6 — Git Kitchens (Per-Instance Results)

## Goal

Replace the single-row-per-cookbook TK result model with per-instance results `(cookbook, target_version, platform, suite)`, wire batch execution, handle `.kitchen.local.yml` conflicts, and update the dashboard.

## Specs to Read

- `.claude/specifications/kitchen-refactor.md` §Git Kitchens (L252-318), §Dashboard Impact (L361-382), §Migration Path (L382-396)
- `.claude/specifications/todo-batch-definition.md` (deferred items feeding into this phase)

## Steps

### Step 1: DB Migration 0016 — `git_kitchen_results` table

- New table alongside existing `git_repo_test_kitchen_results` (old table retained per migration path).
- UUID PK (justified: 5-column natural key is unwieldy for FK references from `vm_tracking`, batch progress queries).
- Unique constraint: `(git_repo_name, git_repo_url, target_chef_version, platform_name, suite_name)`.
- FKs: `batch_id` → `kitchen_batches(id)` nullable, `vm_tracking_id` → `vm_tracking(id)` nullable.
- Indexes: `batch_id`, `git_repo_name`, `(converge_passed, tests_passed)` for dashboard queries.
- Down migration drops the table.

### Step 2: Datastore CRUD — `internal/datastore/git_kitchen_results.go`

- Types: `GitKitchenResult`, `UpsertGitKitchenResultParams`, `ListGitKitchenResultsParams` (filter by batch_id, repo name, platform, status).
- Methods: `UpsertGitKitchenResult`, `GetGitKitchenResult`, `ListGitKitchenResults`, `ListGitKitchenResultsByBatch`, `ListGitKitchenResultsByRepo`, `CountGitKitchenResultsByStatus`, `DeleteGitKitchenResultsByBatch`.
- Tests: scan helpers, upsert idempotency, list filtering.

### Step 3: Batch execution engine — `internal/batch/executor.go`

- `Executor` struct with deps: `Resolver`, `KitchenRunner` interface, `DataStore`, `Logger`, concurrency config.
- `KitchenRunner` interface: `RunInstance(ctx, RunInstanceRequest) RunInstanceResult` — runs a single (cookbook, platform, suite) instance.
- `Execute(ctx, batchID)`: resolve batch → transition to running → fan out with bounded worker pool → write per-instance results → transition to completed/cancelled.
- Concurrency bounded by `max_concurrent_vms` from batch definition.
- Context cancellation support (for cancel endpoint).
- Dry-run path: resolve → write estimate → transition to completed (already exists in API, just needs wiring).
- Tests: mock runner, verify concurrency limiting, cancellation, status transitions, result persistence.

### Step 4: Kitchen instance runner — `internal/batch/kitchen_runner.go`

- Adapter that bridges `KitchenRunner` interface to the existing `analysis.KitchenScanner.testOne()` logic.
- `RunInstanceRequest`: repo name, repo URL, commit SHA, target version, platform, suite, working dir.
- `RunInstanceResult`: converge_passed, tests_passed, timed_out, outputs, duration, error.
- `.kitchen.local.yml` conflict handling (spec §287-297): backup → generate overlay → run → restore.
- Chef version override in overlay (spec §297-318): `product_name` = `chef` or `chef_ice` based on major version, `product_version` from target.
- Credential resolution reuses existing `ResolveKitchenCredentials`.
- Tests: overlay generation with chef version override, `.kitchen.local.yml` backup/restore, mock executor.

### Step 5: Wire batch execution into API

- Update `POST /api/v1/kitchen/batches/:id/run` handler to launch `batch.Executor.Execute` in a goroutine.
- Update `POST /api/v1/kitchen/batches/:id/cancel` to cancel the executor's context.
- New endpoints:
  - `GET /api/v1/kitchen/batches/:id/results` — per-instance results for a batch.
  - `GET /api/v1/kitchen/batches/:id/progress` — counts by status (pending/running/passed/failed).
  - `GET /api/v1/git-kitchen-results?repo=&platform=&status=` — cross-batch result query.
- Add new DataStore methods to interface + mock.
- Tests: handler unit tests for new endpoints.

### Step 6: Resolver enhancements (deferred Phase 5 items)

- Platform filter: cross-ref `kitchen_analysis_results` to check if cookbook has matching platforms. Wire `KitchenAnalysisProvider` into resolver.
- Previous status filter: cross-ref `git_kitchen_results` (new table) for last result status. Wire `TestKitchenResultProvider` into resolver.
- Populate `ResolvedCookbook.Platforms` and `Suites` from kitchen analysis data for VM estimates.
- Tests: resolver with platform/status filters active.

### Step 7: Frontend — batch execution UI

- Batch detail: add "Results" tab with per-instance table (cookbook × platform × suite matrix).
- Progress bar / stats while batch is running (poll `/progress` endpoint).
- Result status badges: passed (green), failed (red), timed out (yellow), running (blue).
- Filter/sort on results table.
- Cross-batch results page: `GET /api/v1/git-kitchen-results` with filters.

### Step 8: Frontend — dashboard updates

- Git Kitchen Results section: expandable per-instance breakdown replacing single pass/fail.
- Platform/suite matrix view (rows = cookbooks, columns = platforms).
- Batch grouping with aggregate stats.
- Filter by platform, status, batch, cookbook pattern.

## Acceptance Criteria

- Migration 0016 applies and rolls back cleanly.
- Per-instance results stored with `(cookbook, version, platform, suite)` granularity.
- Batch execution respects `max_concurrent_vms`, `max_count`, cancellation.
- `.kitchen.local.yml` backup/restore works correctly.
- Chef version override generates correct provisioner config for chef vs chef_ice.
- Platform and previous-status filters work in batch resolver.
- Dashboard shows per-instance breakdown, batch progress, platform matrix.
- Old `git_repo_test_kitchen_results` table retained, untouched.
- All new Go tests pass, existing tests unbroken.

## Order of Work

Steps 1-2 (schema + datastore) are foundational. Step 3-4 (executor + runner) depend on 1-2. Step 5 (API) depends on 3. Step 6 (resolver) is independent of 3-5. Steps 7-8 (frontend) depend on 5.

Parallelisable: Steps 3+6 can proceed in parallel after Step 2 is done.