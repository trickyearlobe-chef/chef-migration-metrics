# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

## Branch map (2026-07-13)

- `main` — holds all merged work.
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

## NOW — ships first: Cop Analysis tabs (`plans/cop-analysis-tabs.md`)

Not started; needs a fresh branch `fix/cop-analysis-tabs`. Fixes three Remediation-page
Cop Analysis inconsistencies caused by mixing two grains (git repo = 1:1 cookbook;
server cookbook = many versions × orgs):
- **Double-count** in "All sources" (a name in both sources counted twice → cop 2026 vs
  Blocker card 1945).
- **Stale drill-down** not reset on filter change (server rows persist under Git filter).
- **Hidden pagination** — drill-down drops `resp.pagination` (header N vs 20 shown).

Fix = replace the source dropdown with **Cop Analysis (Server)** + **Cop Analysis (Git)**
tabs, each at its natural grain (auto-resolves the double-count). Chunks:
1. Backend (`handle_cookstyle_cops.go`): server drill-down grouped by name + nested
   `versions[]`; paginate by name; + unit/functional tests.
2. Frontend (`RemediationPage`, `CopAnalysisTab.tsx`): two tabs, reset-on-change,
   surface pagination, server expand UI; + vitest.
3. Spec + nav; deep-link `?source=` fixes.
**Invariant to assert:** within a tab, header count == drill-down total.
Instance of [[cross-view-value-mismatch-bug-class]].

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
