# Performance Diagnostics

In-app performance instrumentation that works within airgapped/VDI environments where copy-paste is unavailable and pprof output cannot be transferred.

## Problem

General UI/application sluggishness reported. No data exists to identify bottlenecks. The production environment is airgapped, accessed via VDI without copy-paste. Traditional profiling (pprof download, EXPLAIN ANALYZE copy) is impractical. All diagnostics must be readable from the browser.

## Approach

Three layers of instrumentation, all readable via API endpoints and the admin UI:

1. **Request timing middleware** — per-endpoint latency percentiles
2. **Database query timing** — slow query identification
3. **pprof endpoints** — standard Go profiling (behind admin auth, config-gated)

## Layer 1: Request Timing Middleware

### Behaviour

HTTP middleware wraps every API request and records elapsed time. Stats are stored in-memory with a rolling window (configurable, default 5 minutes).

### Data Collected Per Endpoint

- Request count
- p50, p95, p99 latency (milliseconds)
- Max latency
- Error count (status >= 500)

### API Endpoint

`GET /api/v1/admin/performance` — admin-only.

Returns per-endpoint stats sorted by p95 descending (slowest first).

Response shape:

```
{
  "window_seconds": 300,
  "endpoints": [
    {
      "method": "GET",
      "path": "/api/v1/nodes",
      "count": 142,
      "error_count": 0,
      "p50_ms": 45.2,
      "p95_ms": 312.8,
      "p99_ms": 1024.0,
      "max_ms": 1530.0
    }
  ]
}
```

### Implementation Constraints

- Use a fixed-size circular buffer per endpoint (max 1000 samples per endpoint, max 200 tracked endpoints).
- Thread-safe — concurrent requests must not race.
- Zero allocation in the hot path where possible.
- The middleware MUST wrap all `protect` and `adminOnly` routes. Health and version endpoints are excluded.
- Normalise URL paths to collapse IDs: `/api/v1/nodes/abc-123` → `/api/v1/nodes/{id}`. Use the registered route pattern from the mux, not the raw request URL.

### Stats Reset

`DELETE /api/v1/admin/performance` — admin-only. Clears all recorded stats. Returns `204 No Content`.

## Layer 2: Database Query Timing

### Behaviour

A wrapper around `database/sql` query execution that records elapsed time per query "label". Each datastore method assigns a label (e.g. `ListNodeSnapshotsFiltered`, `CountNodeVersionDistribution`).

### Data Collected Per Query Label

- Execution count
- p50, p95, p99 latency (milliseconds)
- Max latency
- Slow query count (exceeds configurable threshold, default 500ms)

### API Endpoint

`GET /api/v1/admin/performance/db` — admin-only.

Returns per-query stats sorted by p95 descending.

Response shape:

```
{
  "window_seconds": 300,
  "slow_threshold_ms": 500,
  "queries": [
    {
      "label": "ListNodeSnapshotsFiltered",
      "count": 87,
      "slow_count": 3,
      "p50_ms": 12.1,
      "p95_ms": 89.4,
      "p99_ms": 520.0,
      "max_ms": 1200.0
    }
  ]
}
```

### Implementation Constraints

- Stats use the same rolling-window circular buffer approach as request timing.
- Max 500 tracked query labels, max 1000 samples per label.
- The timing wrapper is a function `db.Timed(ctx, label string, fn func() error) error` that records elapsed time around `fn`.
- Existing datastore methods call `db.Timed` around their SQL execution. This is a mechanical change across all methods.
- To avoid boiling the ocean, start with the 10 most-called datastore methods (identified from handler usage): `ListNodeSnapshotsFiltered`, `CountNodeVersionDistribution`, `ListMetricSnapshotsByOrganisation`, `GetLatestCollectionRun`, `ListCollectionRuns`, `CountNodeReadiness`, `ListServerCookbooksByOrganisation`, `ListNodeSnapshotsByOrganisation`, `GetNodeSnapshotByName`, `ListAssignmentsByOwner`.
- Additional methods can be instrumented incrementally.

### Stats Reset

`DELETE /api/v1/admin/performance/db` — admin-only. Clears DB stats. Returns `204 No Content`.

## Layer 3: pprof Endpoints

### Behaviour

Expose standard `net/http/pprof` handlers behind admin auth and a configuration toggle. Even though pprof output can't be copied out of VDI, the browser-rendered HTML pages (`/debug/pprof/`, `/debug/pprof/goroutine?debug=1`) are readable and useful for goroutine leak detection and allocation profiling.

### Configuration

```
performance:
  pprof_enabled: false  # default off
```

### Routes (admin-only, only registered when enabled)

- `/debug/pprof/` — index
- `/debug/pprof/cmdline`
- `/debug/pprof/profile`
- `/debug/pprof/symbol`
- `/debug/pprof/trace`
- `/debug/pprof/goroutine`
- `/debug/pprof/heap`
- `/debug/pprof/allocs`
- `/debug/pprof/block`
- `/debug/pprof/mutex`
- `/debug/pprof/threadcreate`

### Implementation Constraints

- pprof handlers are from `net/http/pprof`. Wrap each with the admin auth middleware.
- When `pprof_enabled` is false, the routes are not registered at all (not just 403).
- Log a warning at startup when pprof is enabled: "pprof endpoints enabled — do not use in production without auth".

## Package Layout

### `internal/perf/` — new package

- `stats.go` — `EndpointStats` circular buffer, percentile calculation, thread-safe recording
- `middleware.go` — `RequestTimingMiddleware` HTTP middleware
- `dbstats.go` — `QueryStats` circular buffer for DB query timing
- `stats_test.go`, `middleware_test.go`, `dbstats_test.go`

### Changes to Existing Packages

- `internal/webapi/router.go` — wrap `protect`/`adminOnly` with timing middleware, register perf endpoints and optional pprof routes
- `internal/datastore/` — add `Timed` method, wrap initial 10 methods
- `internal/config/` — add `Performance` config section
- `cmd/chef-migration-metrics/main.go` — pass perf stats to router

## Configuration

```
performance:
  enabled: true              # master switch for all perf instrumentation
  pprof_enabled: false       # pprof endpoints (off by default)
  window_seconds: 300        # rolling window for stats (default 5 min)
  slow_query_threshold_ms: 500  # DB slow query threshold
```

When `performance.enabled` is false, no middleware is installed and perf endpoints return 404.

## Frontend — Admin Performance Page

### Route

`/admin/performance` — visible only to admin users.

### Layout

Three sections:

1. **API Endpoints** — table of endpoints sorted by p95, with count, p50, p95, p99, max, errors. Colour-code rows: green (<100ms p95), yellow (100-500ms), red (>500ms).
2. **Database Queries** — same layout for query labels. Colour-code by slow_threshold_ms.
3. **Actions** — "Reset API Stats" and "Reset DB Stats" buttons calling the DELETE endpoints.

Auto-refresh every 10 seconds via polling (not WebSocket — this is a diagnostic page, simplicity wins).

### Navigation

Add "Performance" link to the admin navigation menu, after "System Stats".

## Acceptance Criteria

- `GET /api/v1/admin/performance` returns per-endpoint latency stats.
- `GET /api/v1/admin/performance/db` returns per-query latency stats.
- Stats reset via DELETE endpoints.
- pprof endpoints available when `pprof_enabled: true`, hidden otherwise.
- All stats viewable from the browser without copy-paste.
- Frontend admin page renders both tables with colour-coded rows.
- No measurable latency increase from the middleware on normal requests (<1µs overhead).
- Rolling window correctly expires old samples.

## Out of Scope

- Distributed tracing (single-binary app, not needed).
- Persistent storage of performance data (in-memory only, resets on restart).
- Automatic alerting on slow endpoints (use the admin page to investigate manually).
- Frontend bundle size profiling (use browser DevTools directly in VDI).