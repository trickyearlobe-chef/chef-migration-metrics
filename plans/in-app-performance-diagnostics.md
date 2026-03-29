# Plan: In-App Performance Diagnostics

## Goal

Add in-app performance instrumentation (request timing, PostgreSQL stats dashboard, pprof) readable from the browser to diagnose sluggishness in an airgapped/VDI environment where copy-paste is unavailable.

## Specs to Read

- `.claude/specifications/performance-diagnostics.md` (primary)
- `.claude/specifications/system-health.md` (existing health endpoint pattern)
- `.claude/specifications/project-conventions.md` (Go/naming conventions)

## Steps

### 1. Add `internal/perf/` package — stats engine

- `stats.go` — thread-safe circular buffer `Recorder` with percentile calculation
- `stats_test.go` — tests for recording, percentiles, rolling window expiry, concurrency safety
- Pure data structure, no HTTP or DB dependency

### 2. Add request timing middleware

- `middleware.go` — `RequestTimingMiddleware` wrapping handlers, recording to `Recorder`
- `middleware_test.go` — tests with `httptest` for latency recording, path normalisation, error counting
- Uses the stats engine from step 1

### 3. Add PostgreSQL stats datastore methods

- `internal/datastore/pg_stats.go` — methods to query `pg_stat_statements`, `pg_stat_user_tables`, `pg_stat_user_indexes`, `pg_stat_activity`
- `internal/datastore/pg_stats_test.go` — tests (unit tests with expected SQL, no live DB needed)
- Graceful degradation: detect if `pg_stat_statements` extension is available before querying it

### 4. Add migration for `pg_stat_statements`

- `migrations/0006_pg_stat_statements.up.sql` — `CREATE EXTENSION IF NOT EXISTS pg_stat_statements` wrapped in a DO block that catches errors gracefully
- `migrations/0006_pg_stat_statements.down.sql` — `DROP EXTENSION IF EXISTS pg_stat_statements`
- Update Docker Compose and Helm to add `shared_preload_libraries=pg_stat_statements` to PostgreSQL config

### 5. Add configuration

- `internal/config/` — add `Performance` section (`enabled`, `pprof_enabled`, `window_seconds`)
- Update config tests

### 6. Wire into router

- `internal/webapi/router.go` — wrap `protect`/`adminOnly` with timing middleware
- Register `GET /api/v1/admin/performance` and `GET /api/v1/admin/performance/db`
- Register `DELETE` reset endpoints
- Conditionally register pprof routes when `pprof_enabled: true`
- `handle_performance.go` — handler implementations
- `handle_performance_test.go` — tests

### 7. Wire pprof and perf config in main.go

- Pass perf recorder to router
- Log warning when pprof enabled

### 8. Frontend admin page

- `frontend/src/pages/AdminPerformancePage.tsx` — five sections: API endpoints, top queries, table health, index usage, active queries
- Colour-coded rows and diagnostic hints
- Reset buttons with confirmation dialog for DB stats
- Auto-refresh every 10 seconds
- Add route and admin nav link

### 9. Update tech debt / todos

- Mark performance profiling item as addressed
- Note any remaining items (e.g. instrumenting additional PG stats views)

## Acceptance Criteria

- [ ] `GET /api/v1/admin/performance` returns per-endpoint p50/p95/p99/max latency
- [ ] `GET /api/v1/admin/performance/db` returns PG table, index, query, and active query stats
- [ ] Graceful degradation when `pg_stat_statements` extension unavailable
- [ ] Stats reset via DELETE endpoints (DB reset calls `pg_stat_statements_reset()` + `pg_stat_reset()`)
- [ ] pprof endpoints available when configured, hidden otherwise
- [ ] All stats viewable from the browser without copy-paste
- [ ] Frontend admin page renders all sections with colour coding and hints
- [ ] Middleware overhead <1µs per request
- [ ] Rolling window correctly expires old samples
- [ ] Migration handles missing `shared_preload_libraries` gracefully
- [ ] All existing tests pass