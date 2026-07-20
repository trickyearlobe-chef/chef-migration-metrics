# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

## Branch map (2026-07-20)

- `main` — holds all merged work. Released line: **v2.18.4**.
- **Event Ingest MVP is MERGED** (`25551f4` + Data Feed fixes `f6161e6`/`3b11111`) and
  released. `feature/event-ingest-mvp` is gone. Post-MVP follow-ups → `todo-event-ingest.md`.
- **Parked** `fix/node-list-count-split` — P3: split the node-list `COUNT(*) OVER()`
  into a separate count query (WIP, compiles, tests NOT run). **Low urgency,
  deploy-risky** (shared node-list + export read path) → the nodes page is not
  user-slow, this is just the heaviest DB query; test thoroughly before any customer
  ship. Full state + resume steps in `plans/p3-node-list-perf.md`.
- **Parked** `chore/spec-drift-report` — one-time spec↔code drift report;
  deprioritised behind feature delivery. Don't nag to merge (see [[spec-drift-parked]]).
- No feature branch in flight. Pull the next chunk from Queued/backlog onto a fresh branch.

## NOW — (nothing active)

No chunk is in flight. Pick the next deliberately from Queued below or a `todo-*.md`
backlog, start it on a fresh branch, and record the chunk (scope/steps/acceptance) here.

## Queued — Event Ingest follow-ups (`plans/todo-event-ingest.md`)

MVP shipped; these are post-MVP. Highest-value first:

- **CC19 target-version failing-nodes preset.** The generic distinct-node rollup and
  `chef_version ∧ status=failure` filters are built and tested; Run events defaults
  status to `failure`. Missing: the auto-wired "prospective target-version" preset —
  `useTargetChefVersion` exists but is unused on `RunEventsPage`, so the target version
  must be picked by hand. Design is largely decided (generic rollup); this is the wiring.
- **Live-fidelity validation** of Data Feed (fixtures still authored) + Chef Server proxy.
- Lab cleanup; depsolve/attributes-only gap. See `todo-event-ingest.md` for detail.

## Queued — Spec/Plan Drift Control (`plans/spec-drift-control.md`)

Chunks A/B/D landed. Open: **E** (drift sweep — the parked `chore/spec-drift-report`
branch is its output); **C** (criteria↔test linkage). 5 specs still WARN on
copied-contract (`diagnostic-bundle`, `system-health-*`) — fold into E.

## Queued — structural refactors (own branches, `todo-tech-debt.md`)

- `CookstyleStore` sub-interface split (`webapi.DataStore` at **210** methods and growing).
- Extract pipeline stages from the two remediation god-handlers
  (`handle_cookbook_remediation.go` ~499 lines, `handle_git_repo_remediation.go` ~486
  lines — each is one oversized function). No shared extraction between them; they serve
  different sources.

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun —
  `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
