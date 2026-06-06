# Web API — WebSocket Real-Time Events

## WebSocket Real-Time Events

### Overview

The API provides a WebSocket endpoint for pushing real-time event notifications to connected dashboard clients. This eliminates the need for polling on views that benefit from live updates (dashboard summaries, log viewer, export progress).

The WebSocket channel carries **lightweight event notifications only** — it does not duplicate query logic. When a client receives an event, it re-fetches the relevant REST endpoint to get the updated data. This keeps the WebSocket layer thin and the REST API as the single source of truth.

### Endpoint

#### `GET /api/v1/ws`

Upgrades the HTTP connection to a WebSocket connection.

**Authentication:** The session token must be provided as a query parameter (`?token=<session-token>`) or via the `session` cookie. The token is validated during the HTTP upgrade handshake. If the token is missing, expired, or invalid, the server rejects the upgrade with `401 Unauthorized`.

**Protocol:** The server uses JSON text frames. Binary frames are not used.

**Subprotocol:** None required. Clients should not request a subprotocol.

### Connection Lifecycle

1. The client opens a WebSocket connection to `/api/v1/ws?token=<session-token>`.
2. The server validates the session and upgrades the connection.
3. The server sends a `connected` event to confirm the session is active.
4. The server pushes events as they occur. The client does not send application-level messages (the channel is server-to-client only).
5. The client should send WebSocket ping frames periodically (recommended: every 30 seconds) to keep the connection alive. The server responds with pong frames automatically.
6. The server sends a WebSocket close frame if the session expires or the server is shutting down.
7. The client should implement automatic reconnection with exponential backoff (recommended: 1s, 2s, 4s, 8s, max 30s).

### Event Envelope

All events use a consistent JSON envelope:

```json
{
  "event": "collection_complete",
  "timestamp": "2024-06-15T14:30:00Z",
  "data": { ... }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `event` | string | Event type identifier (see table below) |
| `timestamp` | ISO-8601 string | When the event occurred |
| `data` | object | Event-specific payload (may be empty `{}` for simple signals) |

### Event Types

#### Connection Events

| Event | Trigger | Data |
|-------|---------|------|
| `connected` | WebSocket connection established | `{ "session_expires_at": "..." }` |

#### Collection Events

| Event | Trigger | Data |
|-------|---------|------|
| `collection_started` | A collection run begins for an organisation | `{ "organisation": "myorg", "run_id": "..." }` |
| `collection_progress` | Periodic progress update during collection | `{ "organisation": "myorg", "run_id": "...", "nodes_collected": 150, "total_nodes": 2500 }` |
| `collection_complete` | A collection run completes successfully | `{ "organisation": "myorg", "run_id": "...", "nodes_collected": 2500 }` |
| `collection_failed` | A collection run fails | `{ "organisation": "myorg", "run_id": "...", "error": "..." }` |

#### Analysis Events

| Event | Trigger | Data |
|-------|---------|------|
| `cookbook_status_changed` | A cookbook's compatibility status changed | `{ "cookbook_name": "nginx", "cookbook_version": "5.1.0", "target_chef_version": "19.0.0", "previous_status": "untested", "new_status": "compatible" }` |
| `readiness_updated` | Readiness counts changed after analysis | `{ "organisation": "myorg", "target_chef_version": "19.0.0" }` |
| `complexity_updated` | Complexity scores recalculated for cookbooks | `{ "cookbook_name": "nginx", "cookbook_version": "5.1.0" }` |
| `rescan_started` | A manual cookbook rescan was triggered | `{ "cookbook_name": "nginx", "cookbook_version": "5.1.0", "job_id": "..." }` |
| `rescan_complete` | A manual cookbook rescan finished | `{ "cookbook_name": "nginx", "cookbook_version": "5.1.0", "job_id": "..." }` |
| `git_repo_status_changed` | Git repo rescan/reset requested | `{ "git_repo_name": "nginx", "action": "rescan", "repos_affected": 1 }` |

#### Export Events

| Event | Trigger | Data |
|-------|---------|------|
| `export_started` | An async export job was created | `{ "job_id": "...", "export_type": "ready_nodes", "format": "csv" }` |
| `export_complete` | An export job finished and is ready for download | `{ "job_id": "...", "export_type": "ready_nodes", "format": "csv", "row_count": 1800, "download_url": "/api/v1/exports/.../download" }` |
| `export_failed` | An export job failed | `{ "job_id": "...", "error": "..." }` |

#### Log Events

| Event | Trigger | Data |
|-------|---------|------|
| `log_entry` | A new log entry was persisted (scoped to collection, analysis, or export) | `{ "id": "...", "severity": "ERROR", "scope": "collection", "message": "...", "organisation": "myorg" }` |

#### Notification Events

| Event | Trigger | Data |
|-------|---------|------|
| `notification_sent` | A notification was dispatched to a channel | `{ "id": "...", "channel_name": "slack-ops", "event_type": "cookbook_status_changed", "status": "sent" }` |
| `notification_failed` | A notification delivery failed | `{ "id": "...", "channel_name": "slack-ops", "event_type": "cookbook_status_changed", "status": "failed", "error": "..." }` |
| `ownership_assigned` | An ownership assignment was created | `{ "owner_name": "web-platform", "entity_type": "cookbook", "entity_key": "acme-web", "assignment_source": "manual" }` |
| `ownership_removed` | An ownership assignment was removed | `{ "owner_name": "web-platform", "entity_type": "cookbook", "entity_key": "acme-web" }` |
| `ownership_reassigned` | Assignments were bulk-reassigned between owners | `{ "from_owner": "old-team", "to_owner": "new-team", "reassigned": 47 }` |

### Server-Side Architecture

The WebSocket layer uses a **hub-and-spoke** pattern:

- A single `EventHub` goroutine manages all connected clients.
- When an application event occurs (collection completes, export finishes, etc.), the originating component calls `EventHub.Broadcast(event)`.
- The hub fans out the event to all connected client send channels.
- Each client connection has a dedicated write goroutine that drains its send channel and writes JSON frames to the WebSocket.
- If a client's send channel is full (slow consumer), the server closes the connection rather than blocking the hub. The client is expected to reconnect.

**Concurrency guarantees:**

- `Broadcast()` is safe to call from any goroutine.
- Client registration and deregistration are serialised through the hub's event loop.
- The hub does not block on slow clients — bounded send channels with drop-on-full semantics.

### Client Reconnection

Clients must handle disconnections gracefully:

- On WebSocket close or error, wait and reconnect with exponential backoff.
- On reconnection, re-fetch all visible REST endpoints to catch any events missed during the disconnection window.
- The server does not maintain event history or replay missed events. The REST API is the source of truth.

### Configuration

WebSocket behaviour is configured under the `server` key:

```yaml
server:
  websocket:
    enabled: true                  # Enable/disable WebSocket endpoint (default: true)
    max_connections: 100           # Maximum concurrent WebSocket connections (default: 100)
    send_buffer_size: 64           # Per-client send channel buffer size (default: 64)
    write_timeout_seconds: 10      # Timeout for writing a single frame (default: 10)
    ping_interval_seconds: 30      # Server-initiated ping interval (default: 30)
    pong_timeout_seconds: 60       # Time to wait for pong before closing (default: 60)
```

| Setting | Default | Description |
|---------|---------|-------------|
| `enabled` | `true` | Set to `false` to disable the WebSocket endpoint entirely. The REST API continues to function normally. |
| `max_connections` | `100` | Maximum number of simultaneous WebSocket connections. New connections are rejected with `503 Service Unavailable` when the limit is reached. |
| `send_buffer_size` | `64` | Size of each client's outbound event buffer. If the buffer is full, the client is disconnected (slow consumer protection). |
| `write_timeout_seconds` | `10` | Maximum time to write a single WebSocket frame before closing the connection. |
| `ping_interval_seconds` | `30` | How often the server sends ping frames to detect dead connections. |
| `pong_timeout_seconds` | `60` | How long the server waits for a pong response before closing the connection. Must be greater than `ping_interval_seconds`. |

### Dependency

The WebSocket implementation uses the `github.com/coder/websocket` package (pure Go, no C dependencies, actively maintained). This is the successor to the now-deprecated `nhooyr.io/websocket` — same author, same API, maintained by Coder. This is the only additional dependency required.

---
