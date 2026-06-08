# Active Plan — Batch UX (submission + progress)

Goal: make bulk kitchen batches (1) quick and safe to launch — fewer steps,
with the VM estimate visible before committing — and (2) observable while they
run, with live per-instance results.

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

Scope: `frontend/src/pages/KitchenBatchesPage.tsx`, `frontend/src/api/kitchen.ts`,
`frontend/src/types/kitchen.ts`; backend `internal/webapi/handle_kitchen_batches.go`
only if adding a preview endpoint.
- Land on the batch **detail** after Save (not the list): `handleCreate` →
  `getKitchenBatch(id)` → `setSelectedBatch(detail)`, so the estimate + "Run
  Batch" are immediately in front of the operator. Removes the hunt-and-open step.
- Add a **live estimate in the create form**: a `POST /kitchen/batches/preview`
  (filters → `BatchEstimate`, no persistence; reuse `resolveBatch`) called
  debounced as filters change, showing estimated VMs/cookbooks before Save.
- Add a **"Create & Run"** primary action: creates, shows a confirm with the
  estimated VM count, then runs — one path from form to running.
- TDD: backend preview-endpoint test; frontend tests for land-on-detail, live
  estimate, and Create & Run confirm→run.
Acceptance: an operator can go form → running in one confirmed action, with the
VM estimate visible beforehand; plain Save still lands on the runnable detail.

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
