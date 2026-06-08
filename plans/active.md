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

## Chunk 1 — One-step batch submission  [done]

Done (`KitchenBatchesPage.tsx` + tests): "Create & Run" action (gated on TK
enabled) creates then runs in one handler and lands on the running detail; plain
Save now fetches the detail and lands there (not the list). No preview/estimate
work — blast radius stays bounded by the form's `maxCount` cap and
`previousStatus` filter.

## Chunk 2 — Per-instance results table (full-stack)  [done]

Done: `GET /kitchen/batches/:id/instances` → `handleListBatchInstances` →
`ListBatchInstances` (dispatched in `handleKitchenBatchDetail`; returns the full
`KitchenBatchInstance` rows, `[]` when empty). Frontend `fetchBatchInstances(id)`
+ `KitchenBatchInstance` type; `BatchInstancesTable` in the detail view groups by
cookbook (collapsible, per-status summary on each header) with instance-level
status badges; refreshes on the same 5s tick as progress while active. Backend +
frontend handler/render/expand tests.

## Chunk 3 — Live updates via WebSocket (depends on Chunk 2)  [next]

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
