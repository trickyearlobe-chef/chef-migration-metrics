# Active Plan — Batch Progress UI

Goal: make the kitchen batch detail view show live, per-instance results so an
operator can watch a bulk scan and see exactly which cookbook/instance passed,
failed, or timed out — without digging through the queue or per-repo views.

Source todo: `plans/todo-bulk-kitchen-scanning.md` § Batch run UI items (poll
progress / real-time bar / per-cookbook results table / cancel).

Current state (verified 2026-06-08):
- Backend is largely DONE: `GET /kitchen/batches/:id/progress` (aggregate
  counts via `CountBatchInstancesByStatus`), `batch_progress`/`batch_complete`
  WebSocket events, `/cancel`, restart recovery. `kitchen_batch_instances`
  holds per-instance status; `ListBatchInstances(batchID)` exists in the
  datastore but has NO HTTP endpoint.
- Frontend `BatchDetailView` (`KitchenBatchesPage.tsx`) already polls progress
  every 5s, renders `BatchProgressBar`, and has a Cancel button. It shows the
  PLANNED cookbooks (`estimate.cookbooks`), not per-instance results, and does
  NOT subscribe to the WebSocket events.

Start each chunk in a fresh thread; read only this plan + the named files.
TDD: write/extend tests before code. Note: `KitchenBatchesPage.tsx` is ~1160
lines (already flagged in tech-debt) — prefer adding small child components.

## Chunk 1 — Per-instance results table (full-stack)  [next]

Scope: `internal/webapi/handle_kitchen_batches.go` + `router.go` + `store.go`
(if needed), `internal/datastore` (method exists), `frontend/src/api/kitchen.ts`,
`frontend/src/types/kitchen.ts`, `frontend/src/pages/KitchenBatchesPage.tsx`.
- Backend: add `GET /api/v1/kitchen/batches/:id/instances` → `ListBatchInstances`,
  returning per-instance {git_repo_name, suite, platform, instance_name, status,
  error_message, started_at, completed_at}. Add route + handler test.
- Frontend: `fetchBatchInstances(id)` + `KitchenBatchInstance` type; render an
  expandable table in `BatchDetailView` grouped by cookbook, with instance-level
  status badges (reuse `StatusBadge`). Fetch on open and on each progress tick.
- TDD: backend handler test (instances returned for a batch); frontend test
  (rows render + group expand).
Acceptance: batch detail lists every instance with its status, grouped/expandable
by cookbook; refreshes as the batch runs.

## Chunk 2 — Live updates via WebSocket (depends on Chunk 1)

Scope: `frontend/src/pages/KitchenBatchesPage.tsx` (+ `useWebSocket`).
- Subscribe to `batch_progress` and `batch_complete` in `BatchDetailView`;
  on event for this batch id, refresh progress + instances immediately.
- Keep the 5s poll as a fallback only while `running`/`preparing`; stop it on
  terminal status.
- TDD: frontend test that a `batch_progress` event updates the bar without the
  poll firing.
Acceptance: progress + results update within ~1s of a status change, no 5s lag.

## Chunk 3 — Cancel UX polish (independent of Chunk 2)

Scope: `frontend/src/pages/KitchenBatchesPage.tsx`.
- Confirm dialog before cancel; optimistic UI (button → "Cancelling…", disabled)
  then refetch; surface errors.
- TDD: frontend test for the optimistic transition + refetch.
Acceptance: clicking Cancel updates the UI immediately and the batch ends in
`cancelled`.

## Notes

- Per-instance data source is `kitchen_batch_instances` (authoritative for batch
  runs), not the queue or `git_kitchen_results` (the `/results` endpoint was
  removed in the git-kitchen rebuild).
- Out of scope: live per-line log streaming (separate SSE tech-debt item);
  extracting `KitchenBatchesPage.tsx` into smaller files (tracked in tech-debt).
