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

- **Saved Filters Chunk 3** (`plans/saved-filters.md`) — extract `SavedFilterBar`
  to the Roles, Cookbooks and Git Repos views. The backend already serves all
  four. **Outranks the remaining tech debt**, incl. the per-target teardown.

## Queued — Saved Filters (`plans/saved-filters.md`, spec `saved-filters.md`)

Remaining: Chunk 3 only (generalise the control to the other three list views).

Two things Chunk 3 must carry over from the Nodes view:

- **Applying goes through the view's own filter-setting path, not the URL.** The
  list views hold filter state in `useState` and read URL params only as inbound
  seeding on mount. Do not assume URL-as-state — the spec used to, wrongly.
- **The stale-reference check is per-view.** It needs that view's entity
  catalogue; on Nodes it is roles (`missingRoles`/`staleRoleWarning` in
  `pages/nodeSavedFilters.ts`). An unloadable catalogue must report *nothing*,
  not "everything vanished".

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
