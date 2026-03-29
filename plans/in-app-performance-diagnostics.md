# Plan: In-App Performance Diagnostics

## Goal

Add in-app performance instrumentation (request timing, DB query timing, pprof) readable from the browser to diagnose sluggishness in an airgapped/VDI environment where copy-paste is unavailable.

## Specs to Read

- `.claude/specifications/performance-diagnostics.md` (primary)
- `.claude/specifications/system-health.md` (existing health endpoint pattern)
- `.claude/specifications/project-conventions.md` (Go/naming conventions)

## Steps

### 1. Add `internal/perf/` package — stats engine

- `stats.go` — thread-safe circular buffer with percentile calculation
- `stats_test.go` — tests for recording, percentiles, rolling window expiry, concurrency safety
- Pure data structure, no HTTP or DB dependency

### 2. Add request timing middleware

- `middleware.go` — `RequestTimingMiddleware` wrapping handlers, recording to `EndpointStats`
- `middleware_test.go` — tests with `httptest` for latency recording, path normalisation, error counting
- Uses the stats engine from step 1

### 3. Add DB query timing

- `dbstats.go` — `QueryStats` using same circular buffer approach
- `dbstats_test.go` — tests for label-based recording, slow query counting
- `internal/datastore/` — add `Timed` method, instrument initial 10 methods

### 4. Add configuration

- `internal/config/` — add `Performance` section (`enabled`, `pprof_enabled`, `window_seconds`, `slow_query_threshold_ms`)
- Update config tests

### 5. Wire into router

- `internal/webapi/router.go` — wrap `protect`/`adminOnly` with timing middleware
- Register `GET /api/v1/admin/performance` and `GET /api/v1/admin/performance/db`
- Register `DELETE` reset endpoints
- Conditionally register pprof routes when `pprof_enabled: true`
- `handle_performance.go` — handler implementations
- `handle_performance_test.go` — tests

### 6. Wire pprof in main.go

- Pass perf config to router
- Log warning when pprof enabled

### 7. Frontend admin page

- `frontend/src/pages/AdminPerformancePage.tsx` — API endpoints table, DB queries table, reset buttons, auto-refresh, colour-coded rows
- Add route and admin nav link

### 8. Update tech debt / todos

- Mark performance profiling item as addressed
- Note any remaining items (e.g. instrumenting additional datastore methods)

## Acceptance Criteria

- [ ] `GET /api/v1/admin/performance` returns per-endpoint p50/p95/p99/max latency
- [ ] `GET /api/v1/admin/performance/db` returns per-query p50/p95/p99/max latency
- [ ] Stats reset via DELETE endpoints
- [ ] pprof endpoints available when configured, hidden otherwise
- [ ] All stats viewable from the browser without copy-paste
- [ ] Frontend admin page renders both tables with colour coding
- [ ] Middleware overhead <1µs per request
- [ ] Rolling window correctly expires old samples
- [ ] All existing tests pass