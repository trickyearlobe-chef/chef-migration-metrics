# WebSocket Log Streaming — Component Specification

> Real-time log entry streaming via the existing WebSocket infrastructure.
> Extends the [Logging specification](logging.md) and [Web API specification](web-api.md).

---

## TL;DR

The existing EventHub and WebSocket handler already broadcast events to connected clients, but `EventLogEntry` (`"log_entry"`) events are **defined but never broadcast**. This specification adds:

1. **Client-side subscriptions** — clients send `subscribe`/`unsubscribe` messages to opt in to `log_entry` events with optional server-side severity filtering.
2. **Broadcast hook on DBWriter** — an optional callback fires after each log entry is persisted, broadcasting it through the EventHub to subscribed clients only.
3. **Frontend `useWebSocket` hook** — connects to `/api/v1/ws`, manages subscriptions, reconnection, and exposes incoming events to React components.
4. **LogsTab integration** — live log entries are prepended to the table in real time when the user is viewing the Logs tab.

No batching is needed — peak log volume is ~3–5 entries/second during active collection, zero between cycles.

---

## Architecture Overview

```
┌──────────────┐     WriteEntry()     ┌──────────────┐
│   Logger     │ ──────────────────▶  │  DBWriter    │
│ (all scopes) │                      │              │
└──────────────┘                      │  1. Persist  │
                                      │     to DB    │
                                      │              │
                                      │  2. Call     │
                                      │  OnBroadcast │
                                      └──────┬───────┘
                                             │
                                             ▼
                                      ┌──────────────┐
                                      │  EventHub    │
                                      │              │
                                      │  Broadcast() │
                                      │  with sub    │
                                      │  filtering   │
                                      └──────┬───────┘
                                             │
                              ┌──────────────┼──────────────┐
                              ▼              ▼              ▼
                        ┌──────────┐  ┌──────────┐  ┌──────────┐
                        │ Client A │  │ Client B │  │ Client C │
                        │ sub:     │  │ sub:     │  │ no sub   │
                        │ min=WARN │  │ min=INFO │  │ (skip)   │
                        └──────────┘  └──────────┘  └──────────┘
```

---

## Backend Changes

### 1. Client Subscriptions in EventHub (`eventhub.go`)

#### Subscription Model

Each client gains a `subscriptions` map tracking which event types it has subscribed to, along with per-subscription filters:

```go
type subscription struct {
    MinSeverity string // for log_entry: "DEBUG", "INFO", "WARN", "ERROR"; empty = no filter
}

type client struct {
    send          chan Event
    subscriptions map[string]subscription // keyed by event type constant
    mu            sync.RWMutex
}
```

#### Subscription-Gated Broadcasting

The `Run()` loop's broadcast case checks subscriptions before queuing:

- If `evt.Type` is `EventLogEntry`, only deliver to clients that have `subscriptions["log_entry"]`.
- For `log_entry` events, apply the `MinSeverity` filter: compare the entry's severity against the subscription's minimum. Skip if below.
- **All other event types** continue to broadcast unconditionally to all clients (existing behaviour preserved).

#### New Methods on `client`

```go
func (c *client) Subscribe(eventType string, sub subscription)
func (c *client) Unsubscribe(eventType string)
func (c *client) IsSubscribed(eventType string) bool
func (c *client) GetSubscription(eventType string) (subscription, bool)
```

All methods are protected by `c.mu`.

#### New Method on EventHub

```go
// BroadcastFiltered sends an event only to clients whose subscriptions
// match. For event types that are not subscription-gated, it falls through
// to unconditional delivery.
func (h *EventHub) BroadcastFiltered(evt Event) {
    // Internally calls Broadcast with subscription check
}
```

In practice, the existing `Broadcast()` method is updated to check subscriptions for `EventLogEntry` events. This avoids changing every call site.

#### Subscription-Gated Event Types

Only `EventLogEntry` is subscription-gated initially. A package-level set controls this:

```go
var subscriptionGatedEvents = map[string]bool{
    EventLogEntry: true,
}
```

Events not in this set are broadcast unconditionally (backwards compatible).

### 2. WebSocket Message Parsing (`websocket.go`)

#### Client-to-Server Messages

The `readPump` currently discards all incoming messages. It is updated to parse JSON messages with the following schema:

```json
{
  "action": "subscribe",
  "event": "log_entry",
  "filters": {
    "min_severity": "WARN"
  }
}
```

```json
{
  "action": "unsubscribe",
  "event": "log_entry"
}
```

#### Message Schema

```go
type wsClientMessage struct {
    Action  string           `json:"action"`  // "subscribe" or "unsubscribe"
    Event   string           `json:"event"`   // event type constant
    Filters *wsClientFilters `json:"filters"` // optional, action-specific
}

type wsClientFilters struct {
    MinSeverity string `json:"min_severity"` // for log_entry subscriptions
}
```

#### Validation

- `action` must be `"subscribe"` or `"unsubscribe"`.
- `event` must be a recognised event type constant that is subscription-gated.
- `min_severity` (if present) must be one of `"DEBUG"`, `"INFO"`, `"WARN"`, `"ERROR"`.
- Invalid messages are logged at DEBUG level and silently ignored (no error sent back to client).

#### Read Pump Changes

```go
func (h *WebSocketHandler) readPump(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, c *client, remoteAddr string) {
    // ... existing close/pong handling ...
    
    // Parse message
    var msg wsClientMessage
    if err := json.Unmarshal(data, &msg); err != nil {
        h.logf("DEBUG", "ignoring unparseable message from %s: %v", remoteAddr, err)
        continue
    }
    
    switch msg.Action {
    case "subscribe":
        // validate and add subscription
        c.Subscribe(msg.Event, subscription{MinSeverity: msg.Filters.MinSeverity})
        h.logf("DEBUG", "client %s subscribed to %s (min_severity=%s)", remoteAddr, msg.Event, msg.Filters.MinSeverity)
    case "unsubscribe":
        c.Unsubscribe(msg.Event)
        h.logf("DEBUG", "client %s unsubscribed from %s", remoteAddr, msg.Event)
    }
}
```

The `readPump` signature changes to accept the `*client` so it can manage subscriptions. The `client` is passed from `ServeHTTP`.

### 3. Broadcast Hook on DBWriter (`db_writer.go`)

#### New Option

```go
// WithOnBroadcast sets a callback invoked after each successful database
// insert. The callback receives the original Entry. It is intended for
// broadcasting the entry to WebSocket clients via the EventHub.
//
// The callback is invoked synchronously in WriteEntry. It must not block.
func WithOnBroadcast(fn func(entry Entry)) DBWriterOption
```

#### WriteEntry Changes

After a successful `InsertLogEntry`, call `onBroadcast(entry)` if set:

```go
func (dw *DBWriter) WriteEntry(entry Entry) error {
    // ... existing insert logic ...
    
    _, err := dw.inserter.InsertLogEntry(dw.ctx, p)
    if err != nil {
        // ... existing error handling ...
        return nil
    }
    
    // Broadcast to WebSocket clients (if anyone is listening)
    dw.mu.RLock()
    onBcast := dw.onBroadcast
    dw.mu.RUnlock()
    if onBcast != nil {
        onBcast(entry)
    }
    
    return nil
}
```

The broadcast only fires on **successful** inserts — failed entries are not broadcast.

### 4. Wiring in `main.go`

```go
// In the DB writer setup section (around line 170):
dbWriter := logging.NewDBWriter(dbAdapter,
    logging.WithContext(context.Background()),
    logging.WithOnError(func(entry logging.Entry, dbErr error) {
        log.Printf("WARN: failed to persist log entry to database: %v", dbErr)
    }),
    logging.WithOnBroadcast(func(entry logging.Entry) {
        hub.Broadcast(webapi.NewEvent(webapi.EventLogEntry, map[string]any{
            "severity":             entry.Severity.String(),
            "scope":                string(entry.Scope),
            "message":              entry.Message,
            "timestamp":            entry.Timestamp.Format(time.RFC3339Nano),
            "organisation":         entry.Organisation,
            "cookbook_name":         entry.CookbookName,
            "cookbook_version":      entry.CookbookVersion,
            "commit_sha":           entry.CommitSHA,
            "chef_client_version":  entry.ChefClientVersion,
            "collection_run_id":    entry.CollectionRunID,
            "export_job_id":        entry.ExportJobID,
            "tls_domain":           entry.TLSDomain,
            // process_output intentionally omitted — too large for WebSocket
            // frames. Clients fetch full detail via REST on click.
        }))
    }),
)
```

**Important:** `process_output` is **not** broadcast over WebSocket. It can contain kilobytes of CookStyle/Test Kitchen output. Clients that need it fetch via `GET /api/v1/logs/:id`.

The EventHub must be created **before** the DB writer so the broadcast closure can capture it. Move EventHub creation above the DB writer wiring.

### 5. Log Entry Event Payload

The `log_entry` WebSocket event has this shape:

```json
{
  "event": "log_entry",
  "timestamp": "2025-01-20T12:00:00.000Z",
  "data": {
    "severity": "WARN",
    "scope": "cookstyle_scan",
    "message": "CookStyle found 3 offenses in apache2 0.2.0",
    "timestamp": "2025-01-20T12:00:00.000Z",
    "organisation": "production",
    "cookbook_name": "apache2",
    "cookbook_version": "0.2.0",
    "collection_run_id": "abc-123"
  }
}
```

Empty string fields are included (not omitted) for simplicity. The frontend can ignore them.

### 6. Severity Comparison

For subscription filtering, severity is compared numerically:

| Severity | Value |
|----------|-------|
| DEBUG    | 0     |
| INFO     | 1     |
| WARN     | 2     |
| ERROR    | 3     |

A subscription with `min_severity: "WARN"` receives only `WARN` and `ERROR` entries.

The `severityValue()` helper function maps severity strings to integers, defaulting to 0 (DEBUG) for unknown values.

---

## Frontend Changes

### 1. `useWebSocket` Hook (`frontend/src/hooks/useWebSocket.ts`)

A reusable hook that manages a single WebSocket connection to `/api/v1/ws`.

#### API

```typescript
interface UseWebSocketOptions {
  /** Auto-connect on mount. Default: true */
  autoConnect?: boolean;
  /** Reconnect delay in ms. Default: 3000 */
  reconnectDelay?: number;
  /** Max reconnect attempts. Default: 10 */
  maxReconnectAttempts?: number;
}

interface UseWebSocketReturn {
  /** Current connection state */
  status: "connecting" | "connected" | "disconnected" | "error";
  /** Subscribe to an event type with optional filters */
  subscribe: (event: string, filters?: Record<string, string>) => void;
  /** Unsubscribe from an event type */
  unsubscribe: (event: string) => void;
  /** Register a callback for incoming events of a given type */
  onEvent: (event: string, callback: (data: unknown) => void) => () => void;
  /** Manually disconnect */
  disconnect: () => void;
  /** Manually reconnect */
  reconnect: () => void;
}
```

#### Behaviour

- Opens a WebSocket to `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/api/v1/ws`.
- On `open`: sets status to `"connected"`, replays any pending subscriptions.
- On `close`: sets status to `"disconnected"`, schedules reconnection with exponential backoff (capped at `reconnectDelay * 2^attempt`, max 30 seconds).
- On `message`: parses the JSON `Event` envelope, dispatches to registered callbacks by event type.
- On component unmount: sends unsubscribe messages for all active subscriptions, then closes the connection.
- `subscribe()` sends `{"action":"subscribe","event":"...","filters":{...}}` and records the subscription locally so it can be replayed on reconnect.
- `unsubscribe()` sends `{"action":"unsubscribe","event":"..."}` and removes the local record.

#### Reconnection

- Uses exponential backoff starting at `reconnectDelay` (default 3s).
- Resets attempt counter on successful connection.
- Stops attempting after `maxReconnectAttempts` (default 10) — sets status to `"error"`.
- On reconnect, replays all active subscriptions.

### 2. WebSocket Event Types (`frontend/src/types.ts`)

```typescript
/** WebSocket event envelope from the server */
export interface WSEvent<T = unknown> {
  event: string;
  timestamp: string;
  data: T;
}

/** Payload shape for log_entry WebSocket events */
export interface WSLogEntryData {
  severity: string;
  scope: string;
  message: string;
  timestamp: string;
  organisation?: string;
  cookbook_name?: string;
  cookbook_version?: string;
  commit_sha?: string;
  chef_client_version?: string;
  collection_run_id?: string;
  export_job_id?: string;
  tls_domain?: string;
}
```

### 3. LogsTab Integration (`frontend/src/pages/LogsPage.tsx`)

#### Behaviour

- On mount, if the WebSocket is available and the user is on the Logs tab:
  - Subscribe to `log_entry` with `min_severity` matching the current filter.
  - Register a callback that prepends incoming entries to the displayed list.
- When `minSeverity` filter changes: unsubscribe, then re-subscribe with the new value.
- When the user navigates away from the Logs tab or the component unmounts: unsubscribe.
- **Live entries are prepended to the top of the current page only** — they do not affect pagination counts or other pages. A subtle "N new entries" indicator appears if the user has scrolled down.
- When the user changes page (pagination), live entries for the previous page are discarded.

#### Visual Indicators

- A small "live" dot/badge next to the tab title when streaming is active.
- New entries briefly flash with a highlight animation (e.g. a green left border that fades).
- Connection status shown subtly (e.g. "● Connected" / "○ Reconnecting..." in the filter bar area).

---

## Concurrency and Safety

- `client.subscriptions` is protected by `client.mu` (read-write mutex).
- `DBWriter.onBroadcast` is protected by the existing `DBWriter.mu`.
- `EventHub.Broadcast()` remains non-blocking — if the internal buffer is full, the event is silently dropped.
- Per-client send channels remain bounded — slow consumers are still evicted.
- The `onBroadcast` callback in `WriteEntry` is synchronous but must not block. `hub.Broadcast()` is non-blocking by design (select with default).

---

## Performance Considerations

- **Zero cost when no one is watching**: If no clients have subscribed to `log_entry`, the broadcast still fires but the hub's fan-out loop skips delivery (subscription check per client).
- **Server-side severity filtering**: Clients receive only entries at or above their `min_severity`. A client watching at `ERROR` level during a cookstyle scan receives ~0 entries/second instead of ~5/second.
- **No process_output over WebSocket**: The largest field in log entries (potentially KB of CookStyle/Test Kitchen output) is never broadcast. Clients fetch it on demand via REST.
- **Individual broadcasts (no batching)**: At ~3-5 entries/second peak, individual WebSocket frames are trivial. Batching would add complexity and latency for no measurable benefit.

---

## Testing Strategy

### Backend Unit Tests

| Test | File | Description |
|------|------|-------------|
| `TestClient_Subscribe` | `eventhub_test.go` | Subscribe adds entry to subscriptions map |
| `TestClient_Unsubscribe` | `eventhub_test.go` | Unsubscribe removes entry |
| `TestClient_SubscribeConcurrent` | `eventhub_test.go` | Concurrent subscribe/unsubscribe is safe |
| `TestEventHub_BroadcastFilteredLogEntry` | `eventhub_test.go` | log_entry only delivered to subscribed clients |
| `TestEventHub_BroadcastFilteredSeverity` | `eventhub_test.go` | Severity filter applied correctly |
| `TestEventHub_BroadcastNonGatedEvent` | `eventhub_test.go` | Non-gated events still broadcast to all |
| `TestDBWriter_OnBroadcast` | `db_writer_test.go` | Callback fires after successful insert |
| `TestDBWriter_OnBroadcastNotCalledOnError` | `db_writer_test.go` | Callback does not fire on insert failure |
| `TestReadPump_SubscribeMessage` | `websocket_test.go` | Valid subscribe message updates client subscriptions |
| `TestReadPump_UnsubscribeMessage` | `websocket_test.go` | Unsubscribe message removes subscription |
| `TestReadPump_InvalidMessage` | `websocket_test.go` | Malformed messages are silently ignored |

### Frontend Tests (future)

- `useWebSocket` hook: connection lifecycle, subscribe/unsubscribe message format, reconnection with replay.
- `LogsTab` integration: live entry prepend, filter change triggers re-subscribe, unmount triggers unsubscribe.

---

## Files Modified

### Backend
| File | Change |
|------|--------|
| `internal/webapi/eventhub.go` | Add `subscription` type, update `client` struct with subscriptions, update broadcast loop for subscription-gated filtering |
| `internal/webapi/eventhub_test.go` | Add subscription and filtered broadcast tests |
| `internal/webapi/websocket.go` | Parse subscribe/unsubscribe messages in `readPump`, pass `*client` to `readPump` |
| `internal/webapi/websocket_test.go` | New file — test message parsing |
| `internal/logging/db_writer.go` | Add `WithOnBroadcast` option, call callback after successful insert |
| `internal/logging/db_writer_test.go` | New file — test broadcast callback behaviour |
| `cmd/chef-migration-metrics/main.go` | Move EventHub creation before DB writer, wire `WithOnBroadcast` to `hub.Broadcast` |

### Frontend
| File | Change |
|------|--------|
| `frontend/src/hooks/useWebSocket.ts` | New file — WebSocket connection hook |
| `frontend/src/types.ts` | Add `WSEvent` and `WSLogEntryData` interfaces |
| `frontend/src/pages/LogsPage.tsx` | Integrate `useWebSocket` for live streaming in LogsTab |

---

## Related Specifications

- [Logging](logging.md) — log entry structure, scopes, severity levels
- [Web API](web-api.md) — REST endpoints, WebSocket endpoint
- [Visualisation](visualisation.md) — log viewer UI
- [Configuration](configuration.md) — WebSocket config (`max_connections`, `send_buffer_size`, timeouts)