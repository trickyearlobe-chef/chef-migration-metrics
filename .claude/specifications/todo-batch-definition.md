# Todo — Batch Definition & Git Kitchen Controls (Phase 5)

## DB Migration & Datastore

- [ ] Migration `0015_kitchen_batches.up.sql` / `.down.sql`
- [ ] `internal/datastore/kitchen_batches.go` — types, CRUD, scan helpers
- [ ] `internal/datastore/kitchen_batches_test.go` — validation + scan tests
- [ ] Git repo exclusion columns + datastore methods
- [ ] Git repo exclusion tests

## Batch Resolution

- [ ] `internal/batch/resolver.go` — filter pipeline, glob matching, estimate
- [ ] `internal/batch/resolver_test.go` — per-filter tests, AND combination, max_count cap

## API

- [ ] `internal/webapi/handle_kitchen_batches.go` — CRUD, run, cancel, delete
- [ ] `internal/webapi/handle_kitchen_batches_test.go` — handler unit tests
- [ ] Git repo exclusion endpoints (exclude, clear, list excluded)

## Router & Interface

- [ ] Add batch + exclusion methods to `DataStore` interface in `store.go`
- [ ] Add mock implementations in `store_mock_test.go`
- [ ] Register routes in `registerRoutes()`

## Frontend

- [ ] `frontend/src/types.ts` — `KitchenBatch`, `BatchFilters`, `ResolvedCookbook`, `BatchEstimate`
- [ ] `frontend/src/api.ts` — batch + exclusion API functions
- [ ] Batch management UI (list, create, detail, dry-run preview)
- [ ] Exclusion management UI
- [ ] Frontend tests