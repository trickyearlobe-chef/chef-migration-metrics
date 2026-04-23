# Todo — Git Kitchens / Per-Instance Results (Phase 6)

## DB Migration & Datastore

- [x] Migration `0016_git_kitchen_results.up.sql` / `.down.sql`
- [x] `internal/datastore/git_kitchen_results.go` — types, CRUD, scan helpers
- [x] `internal/datastore/git_kitchen_results_test.go` — upsert, list, count tests

## Batch Execution Engine

- [x] `internal/batch/executor.go` — fan-out orchestrator with bounded concurrency
- [x] `internal/batch/executor_test.go` — mock runner, concurrency, cancellation
- [x] `internal/batch/kitchen_runner.go` — per-instance runner, overlay, backup/restore
- [x] `internal/batch/kitchen_runner_test.go` — chef version override, local.yml conflict

## Resolver Enhancements (deferred from Phase 5)

- [x] Platform filter resolution (cross-ref kitchen analysis data)
- [x] Previous status filter resolution (cross-ref git_kitchen_results)
- [x] Populate ResolvedCookbook.Platforms/Suites from analysis data
- [x] Resolver tests with platform/status filters

## API

- [x] Wire batch execution into `POST /api/v1/kitchen/batches/:id/run`
- [x] `GET /api/v1/kitchen/batches/:id/results` — per-instance results
- [x] `GET /api/v1/kitchen/batches/:id/progress` — status counts
- [x] `GET /api/v1/git-kitchen-results` — cross-batch result query
- [x] Add new DataStore methods to interface + mock
- [ ] Handler tests for new endpoints

## Frontend

- [ ] Batch detail results tab (per-instance table)
- [ ] Batch progress display (poll + stats)
- [ ] Cross-batch results page with filters
- [ ] Dashboard: per-instance breakdown replacing single pass/fail
- [ ] Dashboard: platform/suite matrix view
- [ ] Frontend tests for new components