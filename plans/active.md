# Active Plan — Sweep status on admin TK page

Goal: surface orphan-sweep outcomes on the admin Test Kitchen config page so an
operator can see the result of the *background* sweep (not just a manually
triggered one) — "last sweep: N destroyed of N scanned, <time> ago".

Source todo: `plans/todo-bulk-kitchen-scanning.md` § Frontend (last open item)
and § Orphan Sweep (the "expose on admin TK config page" bullet).

Context (verified 2026-06-08):
- Backend is built. `POST /api/v1/kitchen/orphan-sweep?dry_run=…`
  (`handleOrphanSweep`) and the `StartSweepTicker` background goroutine both
  broadcast an `orphan_sweep_complete` WebSocket event whose `Data` is the
  `SweepResult` (scanned/destroyed/skipped_too_young/skipped_unparsed/errors/
  dry_run/details) with a top-level event `Timestamp`.
- Frontend `OrphanSweepSection` (`AdminTestKitchenPage.tsx` ~:1318) has a manual
  "Run Sweep" button + dry-run toggle and renders the returned `SweepResult`.
  It does **not** subscribe to `orphan_sweep_complete`, so background-ticker
  sweeps never appear, and `result` is null on page load.

Predecessor: Batch UX plan (Chunks 1–4) complete — see git history and the
checked-off Frontend items in the bulk-kitchen-scanning todo.

## Chunk 1 — Live sweep status in OrphanSweepSection  [next]

Scope: `frontend/src/pages/AdminTestKitchenPage.tsx`,
`frontend/src/pages/AdminTestKitchenPage.test.tsx`. Frontend only — the backend
already emits the event.
- Subscribe `OrphanSweepSection` to `orphan_sweep_complete` via
  `useWebSocket().onEvent`; store the event's `SweepResult` + its timestamp as
  the displayed "last sweep" (replaces/augments the manual-only `result`).
- Render a "Last sweep: N destroyed of N scanned · <relative time> ago" line
  (with the dry-run badge) above the existing detail table; updates whether the
  sweep was manual or from the ticker. Reuse the existing relative-time helper
  if one exists; otherwise add a small `… ago` formatter.
- Manual "Run Sweep" still works; its own broadcast (or direct response) feeds
  the same display — avoid double-rendering.
- TDD: (a) an `orphan_sweep_complete` event with no prior manual run populates
  the last-sweep line; (b) the relative-time label renders; (c) manual run still
  shows its result. Mock `useWebSocket` per the `KitchenBatchesPage.test.tsx`
  hoisted-listener pattern.
Acceptance: a background sweep's outcome shows on the admin TK page without a
manual click; the line shows counts + how long ago.

## Notes

- Out of scope: persistence across a full page reload (showing a sweep that
  completed before the page opened) needs a backend `GET` last-sweep endpoint
  storing the most recent `SweepResult` — none exists today. If wanted, that's a
  follow-up backend chunk; record it in the todo rather than scope-creeping here.
- Remaining Orphan Sweep backend bullets (folder filter, inter-instance sweep,
  age/prefix scoping) stay in the todo; independent of this UI chunk.
