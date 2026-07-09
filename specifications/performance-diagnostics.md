# Performance Diagnostics

In-app performance instrumentation that works within airgapped/VDI environments where copy-paste is unavailable and pprof output cannot be transferred.

## Problem

General UI/application sluggishness reported. No data exists to identify bottlenecks. The production environment is airgapped, accessed via VDI without copy-paste. Traditional profiling (pprof download, EXPLAIN ANALYZE copy) is impractical. All diagnostics must be readable from the browser.

## Approach

Four layers of instrumentation, all readable via API endpoints and the admin UI:

1. **Request timing middleware** — per-endpoint latency percentiles
2. **PostgreSQL stats dashboard** — query the database's own performance statistics
3. **pprof endpoints** — standard Go profiling (behind admin auth, config-gated)
4. **EXPLAIN runner** — capture query plans (scan type, index usage, buffers, timings) for the hot queries Layer 2 surfaces

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

## Layer 2: PostgreSQL Stats Dashboard

### Rationale

PostgreSQL already collects detailed performance statistics internally. Querying these views gives more actionable data than Go-side DB call timing because you see actual query execution times, scan types, index usage, and table health — not just "the app waited N ms" (which conflates pool wait, network, and execution).

Two data sources:

- **Built-in views** (`pg_stat_user_tables`, `pg_stat_user_indexes`) — always available, no setup, give table/index-level health.
- **`pg_stat_statements` extension** — ships with PostgreSQL as a contrib module (included in the Docker `postgres` image). Gives per-query execution time, call counts, and rows. Requires a one-time `CREATE EXTENSION` (via migration) and a `shared_preload_libraries` config change (via Docker Compose or PostgreSQL config).

### Migration: Enable `pg_stat_statements`

Migration `0006_pg_stat_statements.up.sql`:

```
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
```

Migration `0006_pg_stat_statements.down.sql`:

```
DROP EXTENSION IF EXISTS pg_stat_statements;
```

The extension requires `shared_preload_libraries = 'pg_stat_statements'` in `postgresql.conf`. This is set via:
- **Docker Compose:** add `-c shared_preload_libraries=pg_stat_statements` to the postgres command
- **RPM/DEB (standalone PostgreSQL):** add the same to `postgresql.conf`

If the `shared_preload_libraries` config is missing, `CREATE EXTENSION` will fail. The migration must handle this gracefully — wrap in a `DO` block that catches the error and logs a notice. The endpoint will degrade: table/index stats are still available, but query-level stats return an empty array with a `pg_stat_statements_available: false` flag.

### API Endpoint

`GET /api/v1/admin/performance/db` — admin-only.

Response shape:

```
{
  "pg_stat_statements_available": true,
  "top_queries": [
    {
      "query": "SELECT id, node_name, chef_version ... FROM node_snapshots WHERE ...",
      "calls": 1842,
      "total_time_ms": 45230.5,
      "mean_time_ms": 24.6,
      "min_time_ms": 0.8,
      "max_time_ms": 1530.2,
      "rows": 92100,
      "shared_blks_hit": 184200,
      "shared_blks_read": 420
    }
  ],
  "table_stats": [
    {
      "table_name": "node_snapshots",
      "seq_scan": 12,
      "seq_tup_read": 480000,
      "idx_scan": 9842,
      "idx_tup_fetch": 58200,
      "n_live_tup": 60000,
      "n_dead_tup": 150,
      "last_vacuum": "2025-06-15T10:30:00Z",
      "last_analyze": "2025-06-15T10:30:00Z"
    }
  ],
  "index_stats": [
    {
      "table_name": "node_snapshots",
      "index_name": "idx_node_snapshots_org_node",
      "idx_scan": 9842,
      "idx_tup_read": 58200,
      "idx_tup_fetch": 58200,
      "size_bytes": 2457600
    }
  ],
  "active_queries": [
    {
      "pid": 1234,
      "state": "active",
      "query": "SELECT ...",
      "duration_ms": 450.2,
      "wait_event_type": "IO",
      "wait_event": "DataFileRead"
    }
  ]
}
```

### SQL Queries

**Top queries** (from `pg_stat_statements`, when available):

```
SELECT query, calls, total_exec_time AS total_time_ms,
       mean_exec_time AS mean_time_ms, min_exec_time AS min_time_ms,
       max_exec_time AS max_time_ms, rows,
       shared_blks_hit, shared_blks_read
FROM pg_stat_statements
WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
ORDER BY total_exec_time DESC
LIMIT 20
```

**Table stats** (built-in, always available):

```
SELECT relname AS table_name,
       seq_scan, seq_tup_read, idx_scan, idx_tup_fetch,
       n_live_tup, n_dead_tup, last_vacuum, last_analyze
FROM pg_stat_user_tables
ORDER BY seq_tup_read DESC
```

**Index stats** (built-in, always available):

```
SELECT t.relname AS table_name, i.relname AS index_name,
       idx_scan, idx_tup_read, idx_tup_fetch,
       pg_relation_size(i.relid) AS size_bytes
FROM pg_stat_user_indexes sui
JOIN pg_class t ON sui.relid = t.oid
JOIN pg_class i ON sui.indexrelid = i.oid
ORDER BY idx_scan DESC
```

**Active queries** (built-in, always available):

```
SELECT pid, state, query,
       EXTRACT(EPOCH FROM (now() - query_start)) * 1000 AS duration_ms,
       wait_event_type, wait_event
FROM pg_stat_activity
WHERE datname = current_database()
  AND state != 'idle'
  AND pid != pg_backend_pid()
ORDER BY query_start ASC
```

### Interpreting the Data (for the frontend)

The admin page should display simple diagnostic hints next to the raw numbers:

- **Table with high `seq_scan` and low `idx_scan`** → "This table is being scanned sequentially — may need an index"
- **Table with high `n_dead_tup`** → "Many dead tuples — VACUUM may be needed"
- **Index with zero `idx_scan`** → "This index is never used — candidate for removal"
- **Query with high `max_time_ms`** → "This query has high worst-case latency"
- **Query with high `shared_blks_read` relative to `shared_blks_hit`** → "Low cache hit ratio — data frequently read from disk"

These hints are static rules rendered in the frontend, not computed server-side.

### Stats Reset

`DELETE /api/v1/admin/performance/db` — admin-only. Calls `SELECT pg_stat_statements_reset()` (if extension available) and `SELECT pg_stat_reset()`. Returns `204 No Content`.

**Warning:** `pg_stat_reset()` clears all cumulative table/index stats. The reset button should have a confirmation dialog in the frontend.

## Layer 3: pprof Endpoints

### Behaviour

Expose standard `net/http/pprof` handlers behind admin auth and a configuration toggle. Even though pprof output can't easily be transferred out of VDI, the browser-rendered HTML pages (`/debug/pprof/`, `/debug/pprof/goroutine?debug=1`) are readable and useful for goroutine leak detection and allocation profiling.

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

## Layer 4: EXPLAIN Runner

### Rationale

Layer 2 shows *which* query is hot (calls, total time, cache-hit ratio) and *which*
tables are being sequentially scanned — but not the query's execution plan. The plan
(seq-scan vs index, join strategy, `BUFFERS` shared-hit-vs-read, per-node actual timings)
is what turns "this query is slow" into "it full-scans `node_snapshots` because no index
covers the predicate". This layer runs `EXPLAIN` from the browser and returns the plan text.

The customer environment is airgapped (VDI or file transfer only) and its mail filter
**blocks inbound SQL** (a pasted-SQL email was rejected). So the queries to explain cannot be
sent in ad-hoc — they must already ship in the app. Hence a **canned catalog** of named
explains is the primary path; a free-text box is a secondary convenience for careful VDI paste.

### Safety invariants

- The `EXPLAIN` runs inside a **read-only transaction** (`BEGIN … READ ONLY`), then `ROLLBACK`.
  This is the write backstop even for `EXPLAIN (ANALYZE …)`, which *executes* the statement:
  a `SELECT` runs but cannot write, and an accidental `ANALYZE` of a mutating statement errors.
- A per-run `SET LOCAL statement_timeout` caps cost (the target queries are the slow ones); it
  is transaction-local and auto-resets on rollback.
- Exactly one statement per run (the driver is one-statement-per-exec); never concatenate with `;`.
- `EXPLAIN (… FORMAT TEXT)` returns the plan as multiple rows — the full body must be read, not
  just the first row.

### Canned catalog

Each catalog entry reuses the **live production query builder** for that read path, so the
explained SQL is exactly what the app runs and cannot drift from production across migrations.
Parameters are resolved from **live values** at run time (all organisations; a sample
role/node/cookbook; the active default target version) so **no customer identifiers are
hardcoded** in source. An entry whose live values are unavailable (e.g. empty DB) reports
"unavailable" rather than failing the run.

Initial entries target the roles fix and the `node_snapshots` investigation:

- `roles_list` — the materialised roles-list read path.
- `node_list_heavy` / `node_list_light` — the node-snapshots list with and without the heavy
  JSONB projection (the heavy full-row fetch is a known slow path).
- `cookbook_coverage_containment` — the `cookbooks ? <name>` JSONB key-existence scan (a known
  sequential-scan hotspot).
- `node_single_full_row` — single node-snapshot fetch by org + name.
- `distinct_node_roles` — the distinct-roles unnest over `node_snapshots.roles`.

### Free-text runs

A free-text box accepts a **single** explainable data statement — `SELECT`, `WITH`, `INSERT`,
`UPDATE`, `DELETE`, `MERGE`, `TABLE`, or `VALUES` (leading-keyword check; multi-statement input and
utility statements like `COPY`/`VACUUM`/DDL are rejected because PostgreSQL cannot `EXPLAIN` them).
Writes are permitted because plan-only `EXPLAIN` never executes them; the read-only transaction is
the backstop. If `ANALYZE` is requested for a write, the read-only transaction rejects it and the
runner falls back to a plan-only `EXPLAIN`, returning a note that `ANALYZE` was skipped — so a hot
write query (e.g. a queue-claim `UPDATE … FOR UPDATE SKIP LOCKED`) can still be planned.

### Run-twice ("warm")

An optional second run reports a second plan. The second run demonstrates **buffer-cache**
warmth (`BUFFERS` shows shared-hit rising vs shared-read on the first run), **not** plan-cache
effects — the label must not overpromise "cold vs warm" in the OS-cache sense.

### API Endpoints

`GET /api/v1/admin/performance/explain/catalog` — admin-only. Returns the catalog entries
(`key`, `label`, `description`, whether the entry supports `ANALYZE`) for the UI dropdown.

`POST /api/v1/admin/performance/explain` — admin-only. Body selects **either** a `catalog_key`
**or** free-text `sql`, plus `analyze` and optional `run_twice`. Response shape:

```
{
  "label": "Roles list",
  "param_summary": "orgs=3, target=19.3.15",
  "analyze": true,
  "statement_timeout_ms": 15000,
  "captured_at": "2026-07-09T13:20:00Z",
  "app_version": "2.15.1",
  "run1": { "plan_text": "…", "duration_ms": 1234.5, "truncated": false },
  "run2": { "plan_text": "…", "duration_ms": 118.2, "truncated": false }
}
```

Catalog SQL is **not** echoed to the client (a resolved plan/param summary may embed live
identifiers); free-text SQL is echoed back since the caller supplied it. These endpoints are
registered for admins independently of `performance.enabled`/the request recorder — the EXPLAIN
capability does not depend on request instrumentation.

### Egress

Plans are readable in the browser and downloadable as a single self-contained `.txt` (label,
param summary, timestamp, app version, statement timeout, both plan runs) — small enough to
screenshot over VDI or carry out by file transfer.

### Frontend

An "EXPLAIN" tab on the system-stats page: catalog dropdown + run buttons, a free-text box,
`ANALYZE` and run-twice toggles, both plans rendered monospace, and a "Download .txt" button.
The Top Queries table (Layer 2) gains a per-row "Explain" action that **pre-fills** the free-text
box with that query — because `pg_stat_statements` stores queries normalised (`$1`, no bound
values), such a query cannot be one-click `ANALYZE`d; the canned catalog is the one-click
real-timings path.

## Package Layout

### `internal/perf/` — new package

- `stats.go` — `Recorder` circular buffer, percentile calculation, thread-safe recording
- `middleware.go` — `RequestTimingMiddleware` HTTP middleware
- `stats_test.go`, `middleware_test.go`

### Changes to Existing Packages

- `internal/webapi/router.go` — wrap `protect`/`adminOnly` with timing middleware, register perf endpoints and optional pprof routes
- `internal/webapi/handle_performance.go` — handler implementations for request stats and DB stats endpoints
- `internal/webapi/handle_performance_test.go` — tests
- `internal/config/` — add `Performance` config section
- `internal/datastore/` — add methods to query `pg_stat_statements`, `pg_stat_user_tables`, `pg_stat_user_indexes`, `pg_stat_activity`
- `internal/datastore/query_explain.go` — Layer 4 EXPLAIN runner + canned catalog (reuses the live query builders; read-only transaction + statement_timeout)
- `internal/webapi/handle_explain.go` — Layer 4 handlers (`GET …/performance/explain/catalog`, `POST …/performance/explain`)
- `migrations/` — `0006_pg_stat_statements` up/down pair
- `cmd/chef-migration-metrics/main.go` — pass perf recorder to router, log pprof warning
- Frontend — `AdminExplainPage` + an `explain` tab on the system-stats page; per-row "Explain" action on the Top Queries table

## Configuration

```
performance:
  enabled: true              # master switch for all perf instrumentation
  pprof_enabled: false       # pprof endpoints (off by default)
  window_seconds: 300        # rolling window for request stats (default 5 min)
```

When `performance.enabled` is false, no middleware is installed and perf endpoints return 404.

## Frontend — Admin Performance Page

### Route

`/admin/performance` — visible only to admin users.

### Layout

Four sections:

1. **API Endpoints** — table sorted by p95: method, path, count, p50, p95, p99, max, errors. Colour-code rows: green (<100ms p95), yellow (100–500ms), red (>500ms).
2. **Top Queries** — table sorted by total_time: query (truncated), calls, mean, max, rows, cache hit ratio. Show "pg_stat_statements not available" message if extension missing. Colour-code by max_time: green (<100ms), yellow (100–1000ms), red (>1000ms).
3. **Table Health** — table: name, seq scans, idx scans, live tuples, dead tuples, last vacuum. Inline hints for problem indicators. Colour-code: red if seq_scan > 100 and idx_scan = 0.
4. **Index Usage** — table: table, index, scans, size. Highlight unused indexes (idx_scan = 0) in yellow. Highlight never-scanned large indexes in red.
5. **Actions** — "Reset API Stats" and "Reset DB Stats" buttons (DB reset has confirmation dialog).

Active queries section shows currently running queries and their duration (auto-refresh).

Auto-refresh every 10 seconds via polling (not WebSocket — diagnostic page, simplicity wins).

### Navigation

Add "Performance" link to the admin navigation menu, after "System Stats".

## Acceptance Criteria

- `GET /api/v1/admin/performance` returns per-endpoint latency stats.
- `GET /api/v1/admin/performance/db` returns PostgreSQL table, index, and query stats.
- When `pg_stat_statements` is unavailable, DB endpoint still returns table/index stats with `pg_stat_statements_available: false`.
- Stats reset via DELETE endpoints.
- pprof endpoints available when `pprof_enabled: true`, hidden otherwise.
- All stats viewable from the browser without copy-paste.
- Frontend admin page renders all sections with colour-coded rows and diagnostic hints.
- No measurable latency increase from the request timing middleware (<1µs overhead).
- Rolling window correctly expires old samples.
- Migration enables `pg_stat_statements` gracefully (no failure if `shared_preload_libraries` not set).
- `POST /api/v1/admin/performance/explain` returns a plan for a canned catalog entry or a guarded free-text `SELECT`; `GET …/explain/catalog` lists the entries.
- The EXPLAIN runner never mutates data (read-only transaction) and cannot hang (statement_timeout), verified for the `ANALYZE` path.
- Canned explains reuse the live query builders (no drift) and resolve parameters from live values (no hardcoded customer identifiers).
- Plans are viewable in the browser and downloadable as a self-contained `.txt`.

## Out of Scope

- Distributed tracing (single-binary app, not needed).
- Persistent storage of performance data (in-memory request stats reset on restart; PG stats are cumulative and persist across app restarts).
- Automatic alerting on slow endpoints (use the admin page to investigate manually).
- Frontend bundle size profiling (use browser DevTools directly in VDI).
- Go-side DB call wrapping (PostgreSQL's own stats are more actionable and require zero datastore method changes).