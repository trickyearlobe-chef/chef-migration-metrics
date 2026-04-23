# Plan: Phase 5 — Batch Definition & Git Kitchen Controls

## Goal

Add batch definition model, cookbook filters/exclusions, dry-run preview, and batch history so bulk Git Kitchen runs can be controlled safely before Phase 6 executes them.

## Specs to Read

- `.claude/specifications/kitchen-refactor.md` §Batch Definition (L185-252)
- `.claude/specifications/project-conventions.md`

## Steps

### 1. DB Migration 0015 — `kitchen_batches` table + exclusion columns

- `kitchen_batches`: UUID PK, `name` TEXT NOT NULL, `filters` JSONB NOT NULL DEFAULT '{}', `max_count` INTEGER, `max_concurrent_vms` INTEGER, `dry_run` BOOLEAN DEFAULT false, `status` TEXT NOT NULL DEFAULT 'draft', `created_by` TEXT, `created_at`, `started_at`, `completed_at`, indexes on status
- Add columns to `git_repos`: `kitchen_excluded` BOOLEAN DEFAULT false, `kitchen_exclude_reason` TEXT, `kitchen_excluded_by` TEXT, `kitchen_excluded_at` TIMESTAMPTZ
- Down migration drops table and columns
- Write tests: datastore validation tests

### 2. Datastore CRUD — `internal/datastore/kitchen_batches.go`

Types:
- `KitchenBatch` struct (mirrors table)
- `BatchFilters` struct (Go representation of JSONB: `CookbookNames []string`, `Platforms []string`, `ExcludeCookbooks []string`, `HasTestSuite *bool`, `PreviousStatus string`, `TargetChefVersions []string`, `IncludeExcluded bool`)
- `CreateKitchenBatchParams`, `UpdateKitchenBatchParams`

Methods:
- `CreateKitchenBatch` — INSERT RETURNING
- `GetKitchenBatch` — by UUID
- `ListKitchenBatches` — ordered by created_at DESC
- `UpdateKitchenBatch` — partial update (name, filters, max_count, max_concurrent_vms, dry_run)
- `UpdateKitchenBatchStatus` — status + started_at/completed_at transitions
- `DeleteKitchenBatch` — only if status is draft or completed
- `SetGitRepoKitchenExclusion` — update exclusion columns on git_repos
- `ClearGitRepoKitchenExclusion` — reset exclusion columns
- `ListExcludedGitRepos` — WHERE kitchen_excluded = true

Tests: `kitchen_batches_test.go` — validation, scan helpers, filter serialisation

### 3. Batch Resolution — `internal/batch/resolver.go`

New package `internal/batch/`.

`Resolver` takes `BatchFilters` + datastore and returns resolved cookbook list:
- `ResolveBatch(ctx, filters) -> []ResolvedCookbook`
- `ResolvedCookbook`: Name, GitRepoURL, Platforms []string, Suites []string, EstimatedVMs int
- Filter pipeline: fetch all git repos → apply name/glob filter → apply has_test_suite → apply exclusions (unless include_excluded) → apply platform filter (cross-ref kitchen analysis) → apply previous_status filter (cross-ref TK results) → apply max_count cap
- `EstimateBatchSize(resolved) -> BatchEstimate` (total cookbooks, total VMs, per-platform breakdown)

Tests: `resolver_test.go` — mock datastore, test each filter stage independently, test AND combination, test max_count cap, test glob matching

### 4. API Handlers — `internal/webapi/handle_kitchen_batches.go`

Endpoints per spec:
- `POST /api/v1/kitchen/batches` — create batch, validate filters, return 201
- `GET /api/v1/kitchen/batches` — list all batches
- `GET /api/v1/kitchen/batches/:id` — get batch detail; if status != draft, include resolved cookbook list
- `POST /api/v1/kitchen/batches/:id/run` — set status to `previewing` (dry_run) or `running`, return resolved list; actual execution is Phase 6
- `POST /api/v1/kitchen/batches/:id/cancel` — set status to `cancelled` if running
- `DELETE /api/v1/kitchen/batches/:id` — delete if draft/completed/cancelled
- `PUT /api/v1/kitchen/batches/:id` — update batch definition (only if draft)

Exclusion endpoints (on existing git-repos routes or new):
- `POST /api/v1/git-repos/:name/exclude` — body: `{reason, excluded_by}`
- `DELETE /api/v1/git-repos/:name/exclude` — clear exclusion
- `GET /api/v1/git-repos/excluded` — list excluded repos

Tests: `handle_kitchen_batches_test.go`

### 5. Router Wiring & DataStore Interface

- Add batch methods to `DataStore` interface in `store.go`
- Add mock implementations in `store_mock_test.go`
- Register routes in `registerRoutes()`
- Add `WithBatchResolver` router option if needed, or instantiate resolver inline

### 6. Frontend Types & API — `frontend/src/types.ts`, `frontend/src/api.ts`

Types: `KitchenBatch`, `BatchFilters`, `ResolvedCookbook`, `BatchEstimate`
API functions: `createKitchenBatch`, `listKitchenBatches`, `getKitchenBatch`, `runKitchenBatch`, `cancelKitchenBatch`, `deleteKitchenBatch`, `updateKitchenBatch`, `excludeGitRepo`, `clearGitRepoExclusion`, `listExcludedGitRepos`

### 7. Frontend UI — Batch Management Page

New page or section in `AdminTestKitchenPage.tsx`:
- Batch list table (name, status, cookbook count, VM estimate, created, actions)
- Create batch form (name, filter fields, max_count, max_concurrent_vms, dry_run toggle)
- Batch detail view: resolved cookbook list with estimated VM count
- Dry-run preview: table of cookbooks × platforms × suites
- Exclusion management: table of excluded cookbooks with reason, add/remove

### 8. Update Todos

- Update `todo-node-kitchens.md` remaining items if any resolved
- Create `todo-batch-definition.md` tracking Phase 5

## Acceptance Criteria

- User can create a batch with name and filters
- Filters resolve against real git_repos + kitchen_analysis data
- Dry-run shows resolved cookbook list with VM estimate without executing
- Cookbooks can be persistently excluded with reason
- Batch history tracks status transitions (draft → previewing/running → completed/cancelled)
- All datastore methods have unit tests
- All API handlers have unit tests
- Frontend can create, preview, and manage batches