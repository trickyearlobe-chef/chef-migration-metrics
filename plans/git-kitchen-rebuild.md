# Plan: Git Kitchen Rebuild

## Goal

Remove both broken git kitchen execution paths and rebuild with a clean model per `git-kitchen.md` spec.

## Spec

`.claude/specifications/git-kitchen.md`

## Approach

Two-phase: **demolish** (remove old code, get to clean compile), then **build** (executor → datastore → planner → API → frontend). Each phase has atomic commits.

## Phase 1 — Demolish

### 1a. DB migration: drop old table, reshape results table

- Migration `0017`: drop `git_repo_test_kitchen_results`, drop `batch_id`/`vm_tracking_id` FKs from `git_kitchen_results`, consolidate output columns, add `instance_name` and `passed` columns, remove `converge_passed`/`tests_passed`/`converge_output`/`verify_output`/`destroy_output`/`template_used`
- Update datastore: remove old table model + queries, update `GitKitchenResult` struct

### 1b. Remove old backend execution code

- Delete `internal/batch/kitchen_runner.go` + test
- Delete `internal/batch/executor.go` + test
- Delete `internal/batch/resolver.go` + test (only used by batch executor)
- Delete `internal/webapi/handle_git_kitchen_run.go`
- Delete `internal/webapi/handle_git_kitchen_results.go`
- Remove `handleGitRepoRescanTestKitchen()` from `handle_git_repos.go`
- Remove `handleAdminRerunAllTestKitchen()` from `handle_admin_rescan_all.go`
- Remove routes from `router.go`
- Remove `KitchenScanner` test methods from `analysis/kitchen.go` (keep analyser, helper utils, `KitchenExecutor` interface)
- Remove `kitchenScanner.TestGitRepos()` call from `collector/collector.go`
- Remove `GitKitchenRunner` interface, factory, wiring from webapi server struct

### 1c. Remove old frontend code

- Delete `GitKitchenResultsPage.tsx`
- Remove TKResultCard, old buttons, old handlers from `GitRepoDetailPage.tsx`
- Remove "Rerun All Test Kitchen" from `AdminActionsPage.tsx`
- Remove API functions and types
- Remove route from React router

### 1d. Verify clean compile + tests pass

## Phase 2 — Build

### 2a. New datastore layer

- New `GitKitchenResult` struct matching spec table
- `UpsertGitKitchenResult()`, `ListGitKitchenResults()`, `GetGitKitchenResult()`

### 2b. Executor

- New `internal/gitkitchen/executor.go`
- Single function: `RunInstance(ctx, repo_dir, instance_name, chef_version, tk_config) → Result`
- Overlay generation, credential injection, `kitchen test <exact-name>`, output capture
- Tests

### 2c. Planner

- New `internal/gitkitchen/planner.go`
- Reads analyser data, expands suite×platform, checks platform mapping
- Returns instances tagged mapped/unmapped
- Tests

### 2d. Scheduler

- New `internal/gitkitchen/scheduler.go`
- Bounded concurrency worker pool, context-cancellable
- Calls executor per instance, persists results
- Tests

### 2e. API endpoints

- `GET /api/v1/kitchen/instances` + `/:repo`
- `POST /api/v1/kitchen/run`
- `GET /api/v1/kitchen/results` + `/:id`
- Wire into router

### 2f. Frontend

- New instance list on GitRepoDetailPage with run buttons
- New GitKitchenResultsPage reading from single table
- Update AdminActionsPage with "Run All Mapped"

## Acceptance Criteria

- `go build ./...` passes after each commit
- `go test ./...` passes after each commit
- `npm run build` passes after frontend changes
- Old tables dropped, new table created
- Single manual kitchen run works end-to-end from UI
- Output fully captured in single field, scrollable in UI
