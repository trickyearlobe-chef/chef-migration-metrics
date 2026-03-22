# Dashboard Live Update — Component Specification

> Real-time dashboard refresh via the existing WebSocket infrastructure.
> Extends the [WebSocket Log Streaming specification](websocket-log-streaming.md)
> and the [Visualisation specification](visualisation.md).

---

## TL;DR

The dashboard currently loads data once on mount and never refreshes unless the
user changes the organisation filter or navigates away and back. This spec adds
**event-driven refresh** using the `useWebSocket` hook already built for log
streaming. When the backend broadcasts lifecycle events (collection complete,
cookbook status changed, readiness updated, etc.), each dashboard card that
depends on the affected data re-fetches via its existing REST endpoint.

No new API endpoints are needed. No new data is pushed over WebSocket — the
events are lightweight signals that tell the frontend "this data domain changed,
refetch if you're displaying it."

---

## Architecture Overview

```
┌──────────────────┐     hub.Broadcast()      ┌──────────────┐
│  Collector /     │ ──────────────────────▶   │  EventHub    │
│  Analysis /      │   e.g. collection_complete│              │
│  Rescan handlers │                           │  Fan-out to  │
│                  │                           │  subscribers │
└──────────────────┘                           └──────┬───────┘
                                                      │
                                          ┌───────────┼───────────┐
                                          ▼           ▼           ▼
                                    ┌──────────┐ ┌──────────┐ ┌──────────┐
                                    │Dashboard │ │Dashboard │ │ Logs     │
                                    │ Tab A    │ │ Tab B    │ │ Page     │
                                    │ (status) │ │ (trends) │ │ (logs)   │
                                    │          │ │          │ │          │
                                    │ refetch  │ │ refetch  │ │ prepend  │
                                    │ affected │ │ affected │ │ live     │
                                    │ cards    │ │ cards    │ │ entries  │
                                    └──────────┘ └──────────┘ └──────────┘
```

---

## Current State

### Dashboard data flow today

- `DashboardPage` renders 6 "Current Status" cards and 4 "Trends" cards.
- Each card is a self-contained component with its own `load()` callback
  (a `useCallback` wrapping `fetch*()` from `api.ts`).
- Cards fetch once on mount via `useEffect(() => { load(); }, [load])` and
  re-fetch when the `organisation` prop changes.
- **No polling, no WebSocket, no manual refresh button.**

### Events broadcast today

Of the 17 event type constants defined in `eventhub.go`, only 8 are actually
broadcast in production code:

| Event | Broadcast from |
|-------|---------------|
| `connected` | `websocket.go` (initial handshake) |
| `log_entry` | `main.go` (DBWriter broadcast callback) |
| `rescan_started` | `handle_admin_rescan_all.go`, `handle_cookbook_rescan.go` |
| `cookbook_status_changed` | `handle_cookbook_reset_git.go` |
| `git_repo_status_changed` | `handle_git_repos.go` (rescan + reset) |
| `export_started` | `handle_exports.go` |
| `export_complete` | `handle_exports.go` |
| `export_failed` | `handle_exports.go` |

**Never broadcast (but constants defined):**

| Event | Why it matters |
|-------|---------------|
| `collection_started` | Dashboard can't know a run began |
| `collection_progress` | Dashboard can't show intermediate progress |
| `collection_complete` | Dashboard can't know when node data is fresh |
| `collection_failed` | Dashboard can't show collection errors |
| `readiness_updated` | Dashboard can't know readiness changed |
| `complexity_updated` | Dashboard can't know complexity changed |
| `rescan_complete` | Rescans broadcast start but never finish |
| `notification_sent` | Not dashboard-relevant but incomplete |
| `notification_failed` | Not dashboard-relevant but incomplete |

---

## Backend Changes

### 1. Broadcast callback for the Collector (`internal/collector/`)

The `Collector` struct has no reference to the `EventHub` and must not import
the `webapi` package (dependency direction). Follow the same pattern used by
`logging.DBWriter.WithOnBroadcast` — inject a callback function.

#### New Option

```go
// WithBroadcaster sets a callback invoked at key lifecycle points during
// collection runs. The callback receives an event type string and a data
// payload. It is intended for broadcasting progress to WebSocket clients.
//
// The callback must not block. EventHub.Broadcast() is non-blocking by design.
func WithBroadcaster(fn func(eventType string, data any)) Option {
    return func(c *Collector) { c.broadcaster = fn }
}
```

Add a `broadcaster func(string, any)` field to the `Collector` struct, and a
helper method:

```go
func (c *Collector) broadcast(eventType string, data any) {
    if c.broadcaster != nil {
        c.broadcaster(eventType, data)
    }
}
```

#### Broadcast insertion points in `collector.go`

| Location | Event Type | Data Payload |
|----------|-----------|--------------|
| `Run()` — after logging "starting collection run" (after L560) | `collection_started` | `{ "organisation_count": N }` |
| `Run()` — per-org result loop, after each org completes (L625–L641) | `collection_progress` | `{ "organisation": name, "status": "completed"\|"failed", "completed": M, "total": N }` |
| `Run()` — after logging "collection run complete" (after L649) | `collection_complete` | `{ "organisations_succeeded": N, "organisations_failed": M, "duration_seconds": D }` |
| `Run()` — when org listing fails or no orgs found (L553) | `collection_failed` | `{ "error": "message" }` |
| `collectOrganisation()` — after node snapshots persisted (after L903) | `collection_progress` | `{ "organisation": name, "phase": "nodes_collected", "node_count": N }` |
| `collectOrganisation()` — after server cookbook pipeline completes (after L1103) | `cookbook_status_changed` | `{ "organisation": name, "cookbook_count": N }` |
| `collectOrganisation()` — after git repo pipeline completes (after L1207) | `git_repo_status_changed` | `{ "organisation": name, "repo_count": N }` |
| `collectOrganisation()` — after complexity scoring (after L1125 and L1313) | `complexity_updated` | `{ "organisation": name }` |
| `collectOrganisation()` — after readiness evaluation (after L1427) | `readiness_updated` | `{ "organisation": name, "ready_count": N, "blocked_count": M }` |

#### Wiring in `main.go`

```go
collOpts = append(collOpts, collector.WithBroadcaster(
    func(eventType string, data any) {
        hub.Broadcast(webapi.NewEvent(eventType, data))
    },
))
```

This goes in the collector setup section (around L652), after the hub has been
created and before `collector.New()` is called.

### 2. Add `rescan_complete` broadcasts

The rescan handlers already broadcast `rescan_started` but never
`rescan_complete`. Add broadcasts after the rescan goroutine finishes:

| File | Location | Event |
|------|----------|-------|
| `handle_admin_rescan_all.go` | After the background rescan goroutine completes | `rescan_complete` with `{ "cookbook_count": N }` |
| `handle_cookbook_rescan.go` | After the single cookbook rescan completes | `rescan_complete` with `{ "cookbook_name": name, "cookbook_version": ver }` |

These handlers already have access to `r.hub` via the `Router` struct.

### 3. No subscription gating needed

Unlike `log_entry`, dashboard events are **not** subscription-gated. They are
broadcast unconditionally to all connected clients (existing behaviour for all
non-`log_entry` events). This is correct because:

- Dashboard events are infrequent (a few per collection cycle, not per-second).
- The payloads are tiny (a few hundred bytes).
- Any connected client benefits from knowing data changed.

---

## Frontend Changes

### 1. Approach: Refresh key prop via `DashboardPage`

Each card's `load()` is a private `useCallback` that re-runs when its
dependencies change. The simplest way to trigger a re-fetch is to add a
`refreshKey` prop (a counter) to each card's dependencies:

```typescript
function VersionDistributionCard({
  organisation,
  refreshKey,
}: {
  organisation?: string;
  refreshKey: number;
}) {
  // ...
  const load = useCallback(() => {
    // existing fetch logic
  }, [organisation, refreshKey]);  // <-- refreshKey added
  // ...
}
```

When `refreshKey` increments, `useCallback` returns a new reference, which
triggers the `useEffect` that calls `load()`.

### 2. Event-to-card mapping

`DashboardPage` manages refresh keys per data domain, not per card. Multiple
cards may share the same refresh key if they depend on the same data domain.

| Refresh Key | WebSocket Events That Increment It | Cards That Use It |
|-------------|-----------------------------------|-------------------|
| `nodeRefreshKey` | `collection_complete` | `VersionDistributionCard`, `PlatformDistributionCard`, `VersionDistributionTrendCard`, `StaleTrendCard` |
| `cookbookRefreshKey` | `cookbook_status_changed`, `rescan_complete` | `CookbookCompatibilityCard`, `GitRepoCompatibilityCard` |
| `readinessRefreshKey` | `readiness_updated`, `collection_complete` | `ReadinessCard`, `ReadinessTrendCard` |
| `gitRepoRefreshKey` | `git_repo_status_changed`, `rescan_complete` | `GitRepoCompatibilityCard`, `TestKitchenCompatibilityCard` |
| `complexityRefreshKey` | `complexity_updated`, `cookbook_status_changed`, `rescan_complete` | `ComplexityTrendCard` |

Some events increment multiple keys. For example, `collection_complete`
increments `nodeRefreshKey` and `readinessRefreshKey` because a completed
collection means both node data and readiness data may have changed.

### 3. DashboardPage changes

```typescript
export function DashboardPage() {
  const { selectedOrg: org } = useOrg();
  const [activeTab, setActiveTab] = useSearchParamTab();

  // WebSocket for live dashboard refresh.
  const { onEvent } = useWebSocket();

  // Refresh keys — incremented when relevant events arrive.
  const [nodeRefreshKey, setNodeRefreshKey] = useState(0);
  const [cookbookRefreshKey, setCookbookRefreshKey] = useState(0);
  const [readinessRefreshKey, setReadinessRefreshKey] = useState(0);
  const [gitRepoRefreshKey, setGitRepoRefreshKey] = useState(0);
  const [complexityRefreshKey, setComplexityRefreshKey] = useState(0);

  useEffect(() => {
    const cleanups = [
      onEvent("collection_complete", () => {
        setNodeRefreshKey((k) => k + 1);
        setReadinessRefreshKey((k) => k + 1);
      }),
      onEvent("cookbook_status_changed", () => {
        setCookbookRefreshKey((k) => k + 1);
        setComplexityRefreshKey((k) => k + 1);
      }),
      onEvent("readiness_updated", () => {
        setReadinessRefreshKey((k) => k + 1);
      }),
      onEvent("git_repo_status_changed", () => {
        setGitRepoRefreshKey((k) => k + 1);
      }),
      onEvent("rescan_complete", () => {
        setCookbookRefreshKey((k) => k + 1);
        setGitRepoRefreshKey((k) => k + 1);
        setComplexityRefreshKey((k) => k + 1);
      }),
      onEvent("complexity_updated", () => {
        setComplexityRefreshKey((k) => k + 1);
      }),
    ];
    return () => cleanups.forEach((fn) => fn());
  }, [onEvent]);

  // ... render cards with refreshKey props
}
```

### 4. Debouncing

During a collection run, events arrive in bursts (one `cookbook_status_changed`
per org, one `readiness_updated` per org, etc.). Without debouncing, a 5-org
collection would trigger 5 rapid refetches per card.

Add a small debounce (500ms) to each refresh key increment:

```typescript
const debouncedIncrement = useRef<Record<string, ReturnType<typeof setTimeout>>>({});

function incrementWithDebounce(
  setter: React.Dispatch<React.SetStateAction<number>>,
  key: string,
) {
  clearTimeout(debouncedIncrement.current[key]);
  debouncedIncrement.current[key] = setTimeout(() => {
    setter((k) => k + 1);
  }, 500);
}
```

This collapses rapid-fire events into a single refetch per 500ms window.

### 5. Visual indicators

- **No "live" dot needed** — unlike the logs page which streams individual
  entries, the dashboard just silently refetches. The user sees the numbers
  update without needing to know why.
- **Optional**: A subtle "Last updated: HH:MM:SS" timestamp in the page header
  that updates whenever any card refetches. Low priority.

### 6. No subscription needed for dashboard events

Dashboard events (`collection_complete`, `cookbook_status_changed`, etc.) are
**not** subscription-gated — they broadcast to all connected clients
unconditionally. The frontend does not need to send `subscribe` messages for
these events; it only needs to register `onEvent` callbacks.

This differs from `log_entry` events which require an explicit subscription
(because they are high-volume and carry per-entry data).

---

## Concurrency and Safety

- The `broadcaster` callback on the `Collector` is called from within the
  collection goroutine. `hub.Broadcast()` is non-blocking and goroutine-safe.
- Multiple collection events may fire in rapid succession (one per org). The
  hub's broadcast buffer (capacity 256) can absorb this easily — a collection
  run with 10 orgs generates ~30-40 events total.
- Frontend debouncing prevents rapid-fire REST requests from overloading the
  backend during bursts.

---

## Performance Considerations

- **Event payloads are tiny** — each broadcast is a few hundred bytes of JSON.
  No data aggregation happens in the event; the client refetches via REST.
- **REST endpoints are fast** — dashboard queries use pre-aggregated SQL
  (counts, group-bys). Refetching 6 cards adds ~6 lightweight DB queries.
- **Debouncing prevents thundering herd** — 500ms debounce means a 10-org
  collection triggers at most 1-2 refetches per card instead of 10.
- **No extra load when dashboard is closed** — events still broadcast but
  no `onEvent` callbacks are registered to trigger fetches.

---

## Testing Strategy

### Backend Unit Tests

| Test | File | Description |
|------|------|-------------|
| `TestCollector_BroadcasterCalledOnStart` | `collector_test.go` | `collection_started` broadcast fires at run start |
| `TestCollector_BroadcasterCalledOnComplete` | `collector_test.go` | `collection_complete` broadcast fires with org counts |
| `TestCollector_BroadcasterCalledOnFail` | `collector_test.go` | `collection_failed` broadcast fires on error |
| `TestCollector_BroadcasterCalledPerOrg` | `collector_test.go` | `collection_progress` fires per org |
| `TestCollector_NilBroadcaster` | `collector_test.go` | No panic when broadcaster is nil |
| `TestRescanAll_BroadcastsComplete` | `handle_admin_rescan_all_test.go` | `rescan_complete` fires after rescan |
| `TestRescanCookbook_BroadcastsComplete` | `handle_cookbook_rescan_test.go` | `rescan_complete` fires after single rescan |

### Frontend Tests (future)

- `DashboardPage` integration: verify `onEvent` callbacks are registered for
  the correct event types.
- Verify debouncing: rapid-fire events produce only one refetch per window.
- Verify refresh key propagation: incrementing a key triggers `load()` in the
  affected cards.

---

## Files Modified

### Backend

| File | Change |
|------|--------|
| `internal/collector/collector.go` | Add `broadcaster` field, `WithBroadcaster` option, `broadcast()` helper, and broadcast calls at lifecycle points |
| `internal/collector/collector_test.go` | Test broadcaster callback invocation |
| `internal/webapi/handle_admin_rescan_all.go` | Add `rescan_complete` broadcast after background rescan finishes |
| `internal/webapi/handle_cookbook_rescan.go` | Add `rescan_complete` broadcast after single cookbook rescan finishes |
| `cmd/chef-migration-metrics/main.go` | Wire `WithBroadcaster` to `hub.Broadcast` in collector setup |

### Frontend

| File | Change |
|------|--------|
| `frontend/src/pages/DashboardPage.tsx` | Add `useWebSocket`, refresh key state, `onEvent` listeners with debouncing, pass `refreshKey` props to all cards |

---

## Sequencing

This specification depends on:

- [x] WebSocket infrastructure (`eventhub.go`, `websocket.go`) — already exists
- [x] `useWebSocket` hook (`hooks/useWebSocket.ts`) — already built
- [x] Subscription-gated broadcasting for `log_entry` — already working

This specification is independent of any future work on the logs page or other
WebSocket consumers.

---

## Related Specifications

- [WebSocket Log Streaming](websocket-log-streaming.md) — `useWebSocket` hook, subscription model
- [Logging](logging.md) — log entry structure, scopes
- [Visualisation](visualisation.md) — dashboard card definitions
- [Data Collection](data-collection.md) — collector pipeline lifecycle
- [Configuration](configuration.md) — WebSocket config