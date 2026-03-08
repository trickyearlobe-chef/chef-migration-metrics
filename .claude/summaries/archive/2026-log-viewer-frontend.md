# Log Viewer Frontend + EventHub Bug Fix

**Date:** 2026-03-07
**Component:** Frontend — Log Viewer page; WebAPI — EventHub race condition fix

## Context

The backend log API endpoints (Group 3 in HANDLER_PROGRESS.md) were fully implemented — `GET /api/v1/logs`, `GET /api/v1/logs/:id`, and `GET /api/v1/logs/collection-runs` — but there was no frontend page to consume them. The visualisation todo had 10 log viewer items all unstarted (0/10). This was the most impactful next step because it provides operational visibility into what the tool is doing, and the backend was already complete.

## What Was Done

### 1. TypeScript types (`frontend/src/types.ts`)

Added `LogEntry`, `LogListResponse`, `CollectionRun`, `CollectionRunWithOrg`, and `CollectionRunListResponse` types matching the backend JSON response shapes from `handle_logs.go` and `datastore/collection_runs.go`.

### 2. API client functions (`frontend/src/api.ts`)

Added three new API functions:
- `fetchLogs(filters?)` — `GET /api/v1/logs` with `LogFilterQuery` params (scope, severity, min_severity, organisation, cookbook_name, collection_run_id, since, until, pagination)
- `fetchLogDetail(id)` — `GET /api/v1/logs/:id`
- `fetchCollectionRuns(filters?)` — `GET /api/v1/logs/collection-runs` with `CollectionRunFilterQuery` params (organisation, status, pagination)

### 3. Log Viewer page (`frontend/src/pages/LogsPage.tsx` — 506 lines)

Two-tab page with full filtering and pagination:

**Log Entries tab:**
- Paginated table (50 per page) with timestamp, severity badge (colour-coded: gray=DEBUG, blue=INFO, amber=WARN, red=ERROR), scope pill, message (truncated), and organisation columns
- Min severity filter dropdown (DEBUG/INFO/WARN/ERROR, defaults to INFO)
- Scope filter dropdown (12 backend scopes: collection_run, git_operation, test_kitchen_run, cookstyle_scan, notification_dispatch, export_job, tls, readiness_evaluation, startup, secrets, remediation, webapi)
- Organisation filter via existing `OrgSelector` context
- Click-to-expand rows that fetch full log detail via `fetchLogDetail(id)`
- Expanded detail panel shows: full message (whitespace-preserved), metadata grid (organisation, cookbook name+version, collection run ID, commit SHA, chef client version, notification channel, export job ID, TLS domain), process output in terminal-styled `<pre>` block (dark bg, green text, scrollable, max-height 256px), and entry ID/timestamp footer

**Collection Runs tab:**
- Paginated table (25 per page) with organisation name, status badge (colour-coded: blue=running, green=completed, red=failed, amber=interrupted), started timestamp, computed duration, nodes collected, total nodes, and error message columns
- Status filter dropdown (running/completed/failed/interrupted)
- Failed runs highlighted with red background tint, running with blue tint

### 4. Routing and navigation

- Added `<Route path="/logs" element={<LogsPage />} />` to `App.tsx`
- Added "Logs" nav item to `AppLayout.tsx` sidebar with document icon (Heroicons outline `DocumentTextIcon` path)

### 5. Todo updates

Marked 9 of 10 log viewer items as done in `todo/visualisation.md`. The only remaining item is "Implement log retention purge based on configured retention period" which is a backend/config concern, not a frontend task.

Updated `ToDo.md` progress: visualisation from 46/86 (53%) to 55/86 (63%), total from 424/679 (62%) to 433/679 (63%).

### 6. EventHub `Register` race condition fix (`internal/webapi/eventhub.go`)

Fixed a pre-existing bug where `TestEventHub_RegisterAfterStop` hung indefinitely, causing `go test ./internal/webapi/...` to time out after 60s.

**Root cause:** The `register` channel is buffered (capacity 16). In `Register()`, a `select` between `h.register <- c` and `<-h.done` picks randomly when both are ready (Go spec). Because the buffer has space, the write can succeed even after `Run()` has exited and will never drain the channel. The client's `send` channel is never closed, so any caller blocking on `<-c.send` hangs forever.

**Fix:** After the `h.register <- c` case succeeds, immediately re-check `h.done` with a non-blocking select. If `done` is closed (meaning `Run()` has exited or is exiting), close `c.send` so callers don't block. The added code is 5 lines inside the existing `select` case.

### 7. Summary housekeeping

Archived `2026-dockerfile-gem-pinning.md` and `2026-docker-compose-stack.md` to keep summaries ≤ 8.

## Final State

### Build & Vet

- `go build ./...` — clean
- `go vet ./...` — clean
- **All 14 Go test packages pass** — full suite completes in ~40s. The `webapi` package (previously timing out at 60s due to `TestEventHub_RegisterAfterStop` hang) now completes in ~3s.
- Frontend TypeScript diagnostics show only "Cannot find module 'react'" errors due to missing `node_modules` in CI-less environment — same as all other existing pages

### Files Created

- `frontend/src/pages/LogsPage.tsx` — log viewer page (506 lines)

### Files Modified

- `frontend/src/types.ts` — added LogEntry, CollectionRun, CollectionRunWithOrg, and response types
- `frontend/src/api.ts` — added fetchLogs, fetchLogDetail, fetchCollectionRuns functions and filter query interfaces
- `frontend/src/App.tsx` — added LogsPage import and /logs route
- `frontend/src/components/AppLayout.tsx` — added Logs nav item to sidebar
- `internal/webapi/eventhub.go` — fixed Register race condition (5-line addition in Register method)
- `.claude/specifications/todo/visualisation.md` — marked 9 log viewer items done
- `.claude/specifications/ToDo.md` — updated progress counts

## Known Gaps

- **Log retention purge** — the one remaining log viewer todo item. This is a backend/config task (scheduled purge of old log entries based on `logging.retention_days`), not a frontend concern.
- **Date/time range filter UI** — the backend supports `since`/`until` RFC3339 params and the API client accepts them, but the frontend does not yet render date picker inputs. Users can filter by scope and severity which covers the primary use case.
- **Cookbook name filter UI** — the backend and API client support `cookbook_name` filtering, but the frontend does not yet render a cookbook name input. The scope filter effectively narrows to cookbook-related logs.
- **Collection run drill-through** — clicking a collection run does not yet navigate to log entries filtered by that run's ID. The `collection_run_id` filter param is wired in the API client and ready to use.
## Recommended Next Steps (Priority Order)

### 1. Historical Trending Charts (`todo/visualisation.md` — 5 items, 0 done)
**Why:** The dashboard currently shows point-in-time snapshots. Trending over time is the key value proposition for a migration tracking tool — operators need to see whether they're making progress. The backend already has `GET /api/v1/dashboard/version-distribution/trend` and `GET /api/v1/dashboard/readiness/trend` endpoints wired up.
**Scope:** Create a `TrendChart` component (simple SVG line chart or integrate a lightweight charting lib like recharts), add trend visualisations to `DashboardPage.tsx` for version distribution and readiness over time. ~300-500 lines of frontend code.
**Specs:** `visualisation/Specification.md` (§ Historical Trending)

### 2. Data Exports (`todo/visualisation.md` — 8 items, 0 done)
**Why:** Operators need to export node lists and remediation reports for offline analysis, change management tickets, and stakeholder reporting. Currently no way to get data out of the tool except the UI.
**Scope:** Backend export pipeline (`internal/export/`), export API handlers, frontend download buttons. Significant backend work needed.
**Specs:** `web-api/Specification.md` (§ Exports), `visualisation/Specification.md` (§ Data Exports)

### 3. Configuration TLS Enhancements (`todo/configuration.md` — 25 remaining items)
**Why:** 69% complete; several items are ACME/Let's Encrypt auto-cert and advanced TLS features that would make production deployment smoother.
**Specs:** `tls/Specification.md`, `configuration/Specification.md`