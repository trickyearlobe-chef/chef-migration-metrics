# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

## Branch map (2026-07-13)

- `main` — holds all merged work.
- **Pending merge** `fix/cop-analysis-tabs` — Cop Analysis tabs feature complete
  (all 3 chunks: backend server drill-down grouped by name; frontend Server/Git
  tabs with reset + surfaced pagination; spec + legacy deep-link migration). Go +
  frontend tests green, lint clean. Awaiting merge approval; manual UI check still
  advised (open each tab, confirm header count == what you page through).
- **Parked** `fix/node-list-count-split` — P3: split the node-list `COUNT(*) OVER()`
  into a separate count query (WIP, compiles, tests NOT run). **Low urgency,
  deploy-risky** (shared node-list + export read path) → the nodes page is not
  user-slow, this is just the heaviest DB query; test thoroughly before any customer
  ship. Full state + resume steps in `plans/p3-node-list-perf.md`.
- **Parked** `chore/spec-drift-report` — one-time spec↔code drift report;
  deprioritised behind feature delivery. Don't nag to merge (see [[spec-drift-parked]]).
- No other branches in flight. Start new work on a fresh branch.

Recently merged to `main` (this session): P1 GIN index on `node_snapshots.cookbooks`
(`plans/p1-cookbooks-gin-index.md`); P2 roles `role_summary` fix closure
(`plans/list-view-perf.md`).

## NOW — next up (pick one after the Cop Analysis merge)

- **CookStyle Reliability / Trustworthy Reds** (queued below) — feature work.
- **God-handler split** — `handle_cookstyle_cops.go` (1044 lines) per REST resource;
  now unblocked (its Cop Analysis backend churn just landed). See tech-debt todo.

## Queued — CookStyle Reliability / Trustworthy Reds (`plans/cookstyle-reliability.md`)

Make CookStyle a reliable indicator (reds = "we know", Review = "operator decides",
Noise = "provably harmless"); strip the non-existent per-target dimension. Durability
#1 (dept defaults → Review-worklist) + #2 (drift) stay; #3 (DB seed) abandoned. Spec
revision gated on user approval (Phase 1).

## Queued — Spec/Plan Drift Control (`plans/spec-drift-control.md`)

Chunks A/B/D landed. Open: **E** (drift sweep — the parked `chore/spec-drift-report`
branch is its output); **C** (criteria↔test linkage). 5 specs still WARN on
copied-contract (`diagnostic-bundle`, `system-health-*`) — fold into E.

## Queued — post-merge structural refactors (own branches, `todo-tech-debt.md`)

- `CookstyleStore` sub-interface split (DataStore at 190 methods).
- Split the 978-line `handle_cookstyle_cops.go` god-handler per REST resource.
  (Note: overlaps the Cop Analysis backend work — sequence after it.)

## Deferred proposal — event ingest (`plans/proposal-event-ingest.md`)

Firehose ingest of Chef `data_collector` converge events. FOR DISCUSSION after the
perf/feature work; not approved. Closes the active-only test blind spot.

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun —
  `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
