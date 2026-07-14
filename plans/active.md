# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

## Branch map (2026-07-14)

- `main` — holds all merged work. Released line: **v2.16.2**.
- **Parked** `fix/node-list-count-split` — P3: split the node-list `COUNT(*) OVER()`
  into a separate count query (WIP, compiles, tests NOT run). **Low urgency,
  deploy-risky** (shared node-list + export read path) → the nodes page is not
  user-slow, this is just the heaviest DB query; test thoroughly before any customer
  ship. Full state + resume steps in `plans/p3-node-list-perf.md`.
- **Parked** `chore/spec-drift-report` — one-time spec↔code drift report;
  deprioritised behind feature delivery. Don't nag to merge (see [[spec-drift-parked]]).
- No other branches in flight. Start new work on a fresh branch.

## NOW — next up

- **Saved Filters** (`plans/saved-filters.md`) — customer-requested feature, next
  is **Chunk 2** (Nodes view UI; the storage + API backend is in).
  **Outranks the remaining tech debt**, incl. the per-target teardown.

## Queued — Saved Filters (`plans/saved-filters.md`, spec `saved-filters.md`)

Name and persist a filter selection on a list view (driving case: a ~20-role
"All Windows OS" cohort on Nodes). Persistence + UI only — the multi-role filter
already works (`NodeSnapshotFilter.Roles`). Private w/ explicit share; filters only
(no sort/page); stale refs kept and warned on apply.

Remaining: Chunk 2 (Nodes UI — save/apply control, stale-reference warning on
apply), then Chunk 3 (extract the control to the other three list views).

## Queued — Spec/Plan Drift Control (`plans/spec-drift-control.md`)

Chunks A/B/D landed. Open: **E** (drift sweep — the parked `chore/spec-drift-report`
branch is its output); **C** (criteria↔test linkage). 5 specs still WARN on
copied-contract (`diagnostic-bundle`, `system-health-*`) — fold into E.

## Queued — structural refactors (own branches, `todo-tech-debt.md`)

- `CookstyleStore` sub-interface split (DataStore at 190 methods).
- Extract pipeline stages from the two remediation god-handlers
  (`handle_cookbook_remediation.go`, `handle_git_repo_remediation.go` — each is
  one ~480-line function). No shared extraction between them; they serve
  different sources.

## Deferred proposal — event ingest (`plans/proposal-event-ingest.md`)

Firehose ingest of Chef `data_collector` converge events. FOR DISCUSSION after the
perf/feature work; not approved. Closes the active-only test blind spot.

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun —
  `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
