# Active Plan — Batch UX (submission + progress)

Goal: make bulk kitchen batches (1) quick to launch — form to running in one
action, blast radius bounded by the existing max-count cap and previous-status
filter — and (2) observable while they run, with live per-instance results.

Source todo: `plans/todo-bulk-kitchen-scanning.md` § Batch run UI.

Current flow to run a batch (verified 2026-06-08, `KitchenBatchesPage.tsx`):
New Batch → fill form → Save (creates a *draft*, returns to the **list**) →
find & open the batch → "Run Batch". Four actions + navigation. `createKitchenBatch`
returns a bare `KitchenBatch` (no estimate); the estimate only appears in the
detail view via `getKitchenBatch`. The form shows no live estimate, so the
operator commits before seeing how many VMs the batch will start.

Start each chunk in a fresh thread; read only this plan + the named files.
TDD: write/extend tests before code. `KitchenBatchesPage.tsx` is ~1160 lines
(flagged in tech-debt) — prefer small child components over growing it.

## Chunk 1 — One-step batch submission  [next]

Frontend only: `KitchenBatchesPage.tsx` (+ `api/kitchen.ts` if a helper helps).
No preview/estimate work — blast radius is already bounded by the form's
`maxCount` cap and `previousStatus` filter (`passed`/`failed`/`untested` =
never-ran), so the operator doesn't need a VM count to launch safely.
- **"Create & Run"** primary action on the form: `createKitchenBatch` →
  `runKitchenBatch` in one handler, then show the running detail. One path from
  form to running.
- **Save lands on the detail** (not the list): `handleCreate` →
  `getKitchenBatch(id)` → `setSelectedBatch(detail)`, so a plain Save still puts
  the runnable batch in front of the operator. Removes the hunt-and-open step.
- TDD: frontend tests for Create & Run → running, and Save → detail.
Acceptance: an operator goes form → running in one click; plain Save lands on
the runnable detail rather than the list.

## Chunk 2 — Per-instance results table (full-stack)

Scope: `handle_kitchen_batches.go` + `router.go`, `api/kitchen.ts`,
`types/kitchen.ts`, `KitchenBatchesPage.tsx`.
- Backend: `GET /kitchen/batches/:id/instances` → `ListBatchInstances` (method
  exists), returning per-instance {git_repo_name, suite, platform, instance_name,
  status, error_message, started_at, completed_at}. Route + handler test.
- Frontend: `fetchBatchInstances(id)` + type; expandable table in the detail view
  grouped by cookbook with instance-level status badges; refresh on each tick.
- TDD: backend handler test; frontend rows-render/expand test.
Acceptance: detail lists every instance with status, grouped/expandable by
cookbook, refreshing as the batch runs.

## Chunk 3 — Live updates via WebSocket (depends on Chunk 2)

Scope: `KitchenBatchesPage.tsx` (+ `useWebSocket`).
- Subscribe to `batch_progress`/`batch_complete` (backend already broadcasts);
  refresh progress + instances on event; keep the 5s poll only while active.
- TDD: a `batch_progress` event updates the bar without the poll firing.
Acceptance: progress + results update within ~1s of a status change.

## Chunk 4 — Cancel UX polish (independent of Chunk 3)

Scope: `KitchenBatchesPage.tsx`.
- Confirm dialog; optimistic UI ("Cancelling…", disabled) then refetch.
- TDD: optimistic transition + refetch.
Acceptance: Cancel updates the UI immediately; batch ends `cancelled`.

## Notes

- Per-instance data source is `kitchen_batch_instances` (authoritative), not the
  queue or `git_kitchen_results` (the `/results` endpoint was removed).
- Out of scope: live per-line log streaming (SSE tech-debt item); splitting
  `KitchenBatchesPage.tsx` into smaller files (tech-debt).
