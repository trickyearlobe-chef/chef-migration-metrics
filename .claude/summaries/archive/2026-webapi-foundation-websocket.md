# Task Summary: Web API Foundation + WebSocket Real-Time Events

**Date:** 2026  
**Area:** Web API (`internal/webapi/`), Configuration, Specifications  
**Status:** Complete — foundation built, ready for REST handler implementation

---

## What Was Done

### Specification Updates

1. **Web API spec** (`.claude/specifications/web-api/Specification.md`) — Added **WebSocket Real-Time Events** section (~150 lines):
   - `GET /api/v1/ws` endpoint with auth via query param or cookie
   - Connection lifecycle (upgrade, connected event, ping/pong, reconnection with exponential backoff)
   - Event envelope format (`event`, `timestamp`, `data`)
   - 17 event types across 6 categories: connection, collection, analysis, export, log, notification
   - Hub-and-spoke server architecture with slow consumer eviction
   - Configuration schema under `server.websocket` (6 settings)
   - `nhooyr.io/websocket` dependency note

2. **Visualisation spec** (`.claude/specifications/visualisation/Specification.md`) — Added **Real-Time Updates** section:
   - Dashboard behaviour for each event type (auto-refresh, progress indicators, log streaming)
   - Connection status indicator (green/amber/red)
   - Graceful degradation to polling when WebSocket unavailable

3. **Configuration spec** (`.claude/specifications/configuration/Specification.md`) — Added `server.websocket` block:
   - `enabled`, `max_connections`, `send_buffer_size`, `write_timeout_seconds`, `ping_interval_seconds`, `pong_timeout_seconds`
   - Settings table with defaults
   - Added to full example YAML

4. **ToDo** (`.claude/specifications/ToDo.md`) — Updated next steps to reflect completed foundation and define next session scope (wire router + implement REST handlers)

### New Package: `internal/webapi/`

| File | Lines | Purpose |
|------|-------|---------|
| `eventhub.go` | ~260 | Event type constants (17), `Event` envelope with custom `MarshalJSON` (nil Data → `{}`), `NewEvent()` helper, `client` type with buffered send channel, `EventHub` with hub-and-spoke fan-out, `atomic.Int64` client counter, functional options (`WithMaxConnections`, `WithSendBufferSize`), `Run()`/`Stop()`/`Broadcast()`/`Register()`/`Unregister()`/`ClientCount()` |
| `websocket.go` | ~230 | `WebSocketHandler` implementing `http.Handler` — HTTP→WebSocket upgrade via `github.com/coder/websocket`, read pump (discards client messages, processes control frames), write pump (drains send channel, writes JSON text frames), ping/pong keepalive, configurable timeouts, logger callback to avoid importing logging package |
| `response.go` | ~226 | `ParsePagination()` (defaults 1/50, max 500, clamp), `ParseSort()` (allowed-field whitelist), `WriteJSON()`/`WriteError()`/`WriteErrorf()`/`WriteBadRequest()`/`WriteNotFound()`/`WriteUnauthorized()`/`WriteForbidden()`/`WriteInternalError()`/`WritePaginated()`, `PaginationParams`/`PaginationResponse`/`PaginatedResponse`/`ErrorResponse`/`SortParams` types, error code constants |
| `router.go` | ~302 | `Router` struct assembling `http.ServeMux` with all spec endpoints — health (with WebSocket status), version, WebSocket (conditional on config), placeholders for all 40+ API routes grouped by spec section, SPA frontend fallback (catches unmatched `/api/` as 404), `secondsToDuration()` helper, functional options (`WithVersion`, `WithLogger`) |
| `eventhub_test.go` | ~520 | 32 tests covering: Event construction + timestamp range, JSON marshalling (nil data → `{}`, with data roundtrip), hub register+broadcast, multiple clients, unregister, slow consumer eviction, max connections rejection, client count tracking, stop closes all clients, stop idempotent, broadcast after stop, register after stop, default options, non-positive option ignored, event type constant uniqueness, pagination parsing (defaults/custom/clamp/invalid), offset+limit, pagination response (exact/remainder/single/zero), WriteJSON, WriteError, WritePaginated, ParseSort (default/custom/disallowed), secondsToDuration |

### Config Changes (`internal/config/config.go`)

- Added `WebSocketConfig` struct with `*bool Enabled` (nil = default true via `IsEnabled()`) + 5 int fields
- Embedded in `ServerConfig` as `WebSocket WebSocketConfig`
- Added defaults in `setDefaults()`: max_connections=100, send_buffer_size=64, write_timeout=10s, ping_interval=30s, pong_timeout=60s
- Added validation in `validateServer()`: all values ≥ 1, pong_timeout > ping_interval

### Dependency Added

- `github.com/coder/websocket` v1.8.14 — pure Go WebSocket library, no C dependencies. Successor to deprecated `nhooyr.io/websocket` (same author, same API, now maintained by Coder).

---

## Key Design Decisions

1. **Event-notification-only WebSocket** — The WebSocket channel carries lightweight event signals; clients re-fetch REST endpoints for actual data. This keeps the WebSocket layer thin and the REST API as the single source of truth.

2. **Hub-and-spoke with bounded buffers** — Each client has a buffered send channel. If full (slow consumer), the client is evicted rather than blocking the hub. Dropped clients reconnect and re-fetch state.

3. **Atomic client counter** — `ClientCount()` uses `atomic.Int64` updated by the run goroutine, avoiding data races without routing through the event loop.

4. **Logger callback pattern** — The webapi package uses `func(level, msg string)` callbacks rather than importing the logging package, preventing circular dependencies. The caller (main.go) wires in a real logger.

5. **`*bool` for WebSocket enabled** — Allows distinguishing "not set" (nil → default true) from explicit `enabled: false` in YAML.

---

## What's NOT Done (Next Session Scope)

1. **Wire `webapi.Router` into `main.go`** — replace the inline mux, start EventHub goroutine
2. **Implement ~12 real REST endpoint handlers** serving data from existing datastore methods:
   - Organisations list
   - Nodes list + detail + by-version + by-cookbook
   - Cookbooks list + detail
   - Logs list + detail + collection runs
   - All 7 filter option endpoints
   - Admin status
3. **Handler tests** with httptest + mock datastore
4. **Dashboard endpoints** (version distribution, readiness, cookbook compatibility) — may fit in same session or need follow-up
5. **Auth middleware** — not started, can be done in parallel

---

## Files Modified

- `.claude/specifications/web-api/Specification.md` — WebSocket section added
- `.claude/specifications/visualisation/Specification.md` — Real-time updates section added
- `.claude/specifications/configuration/Specification.md` — websocket config block added
- `.claude/specifications/ToDo.md` — next steps updated
- `internal/config/config.go` — WebSocketConfig struct, defaults, validation
- `go.mod` / `go.sum` — nhooyr.io/websocket v1.8.17

## Files Created

- `internal/webapi/eventhub.go`
- `internal/webapi/websocket.go`
- `internal/webapi/response.go`
- `internal/webapi/router.go`
- `internal/webapi/eventhub_test.go`

## Test Results

- 32 new tests in `internal/webapi/` — all passing
- `internal/config/` tests — still passing
- `internal/datastore/` tests — still passing
- `go build ./...` — clean
- `go vet ./internal/webapi/ ./internal/config/` — clean