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
- **In flight** `feature/event-ingest-mvp` — Event Ingest MVP (see NOW).
- No other branches in flight. Start new work on a fresh branch.

## NOW — Event Ingest MVP (`feature/event-ingest-mvp`)

Spec: `specifications/event-ingest.md`. Transport is empirically validated
(`plans/proposal-event-ingest.md`, and the `automate-datafeed-behavior` finding):
CMM is a passive `POST /api/v1/ingest` sink accepting three shapes (node direct /
Chef Server proxy `run_converge`, and Automate Data Feed per-node state), normalised
into a partitioned `converge_runs` table, surfaced on a new Node Detail **Runs** tab.
**No auth** (tech debt). Key = (organisation, node_name); dedup on run_id.

Landed (in git): **fixtures**, **migration** `0052_converge_runs`, **normaliser**
(`internal/ingest`). Landed (working tree, pending commit): **config** (`IngestConfig`
+ store registration + admin `PUT /api/v1/admin/config/ingest` toggle), **store**
(`BulkUpsertConvergeRuns` dedup + `PurgeConvergeRunPartitions`, functional tests),
**handler** `POST /api/v1/ingest` (gunzip, NDJSON, size/record caps, one-txn-per-body,
200-and-drop, no auth). `converge_runs` decoupled: no FKs, `organisation` = delivered
org name; PK `(run_id, end_time)`.

Remaining:

1. **Read API** — GET runs for a node (org+name), paginated (`web-api.md`). Needs a
   `datastore` read method (+ interface/mock) and a `NodeDetail`-scoped route.
2. **Frontend Runs tab** — `NodeDetailPage` new tab + panel + `api/nodes.ts`; renders
   runs incl. collapsible failure detail. Reuse existing panel pattern.
3. **Retention scheduler** — `PurgeConvergeRunPartitions` exists + tested but is NOT
   wired to a periodic job yet; hook it into the app scheduler (cutoff = now −
   `retention_days`). Low urgency at 2-day pilot scale.
4. **E2E on lab** — enable `ingest.enabled`, point Automate Data Feed at the running app;
   drive a success + a `ruby_block`-raise failure; confirm rows (via SQL until the Runs
   tab lands) then the tab. `chef-load` for volume/firehose.

Acceptance: fixtures normalise correctly; a gzip NDJSON batch yields the right deduped
rows and a malformed body persists nothing; a converge failure shows on the node's
Runs tab with error class/message + failing cookbook·recipe + backtrace; partitions
older than the window drop cleanly; unauthenticated POST is accepted (tech-debt noted).

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

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun —
  `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
