# Todo — Batch Definition & Git Kitchen Controls (Phase 5)

## DB Migration & Datastore

- [x] Migration `0015_kitchen_batches.up.sql` / `.down.sql`
- [x] `internal/datastore/kitchen_batches.go` — types, CRUD, scan helpers
- [x] `internal/datastore/kitchen_batches_test.go` — validation + scan tests
- [x] Git repo exclusion columns + datastore methods
- [x] Git repo exclusion tests

## Batch Resolution

- [x] `internal/batch/resolver.go` — filter pipeline, glob matching, estimate
- [x] `internal/batch/resolver_test.go` — per-filter tests, AND combination, max_count cap

## API

- [x] `internal/webapi/handle_kitchen_batches.go` — CRUD, run, cancel, delete
- [x] `internal/webapi/handle_kitchen_batches_test.go` — handler unit tests
- [x] Git repo exclusion endpoints (exclude, clear, list excluded)

## Router & Interface

- [x] Add batch + exclusion methods to `DataStore` interface in `store.go`
- [x] Add mock implementations in `store_mock_test.go`
- [x] Register routes in `registerRoutes()`

## Frontend

- [x] `frontend/src/types.ts` — `KitchenBatch`, `BatchFilters`, `ResolvedCookbook`, `BatchEstimate`
- [x] `frontend/src/api.ts` — batch + exclusion API functions
- [x] Batch management UI (list, create, detail, dry-run preview)
- [x] Exclusion management UI
- [x] Frontend tests for KitchenBatchesPage

## Remaining

- [x] Platform filter resolution (cross-ref kitchen analysis data in resolver — done in Phase 6)
- [x] Previous status filter resolution (cross-ref TK results in resolver — done in Phase 6)
- [x] Batch execution engine (done in Phase 6)