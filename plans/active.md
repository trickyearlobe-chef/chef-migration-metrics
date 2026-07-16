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

**Topology drives this** (see `customer-topology-ingest` memory + spec Node identity):
clustered Automate (2 orgs, ~50k each) CMM pulls + Data Feeds; **standalone DMZ Infra
Servers (~2–3k each) CMM CANNOT pull — ingest-only → orphan nodes are first-class**.

**Code-complete + committed:** fixtures, migration `0052_converge_runs`, normaliser
(with real-shape fixes: `cookbooks` name→{version}, `chef_version` from node tree),
config + admin toggle, store (dedup + partition purge), handler `POST /api/v1/ingest`,
read API `GET /api/v1/nodes/runs/{org}/{node}`, Node Detail **Overview | Runs tabs**,
retention ticker (hourly + startup). **Node-direct/run_converge path PROVEN LIVE.**

Remaining:

1. **"Run events" top-level view (REQUIRED — the DMZ gap; DECIDED design).** A per-node
   Runs tab reaches only pulled nodes, so DMZ ingest-only telemetry (no `node_snapshots`)
   is stored-but-invisible. Decision: a run-centric **dedicated top-level "Run events"
   view over `converge_runs`** (sibling of Nodes/Cookbooks/Git Repos) — list + run detail,
   default failures, own export. **View-level filters sourced from `converge_runs` itself**
   (org = `SELECT DISTINCT organisation`, status, node, chef_version, time) — must NOT use
   the global org filter (organisations-table-driven → excludes DMZ orgs). Reads
   `converge_runs` directly (indexed, retention-bounded); does NOT fabricate node/org
   records or touch pull aggregates. Node Detail Runs tab stays. Chosen over a virtual
   node-list union (perf: DISTINCT over firehose + heavy node-list query) and an ingest
   node registry (extra table to maintain) — see [[event-ingest-build-state]].
   Build: nav entry + route + list page (reuse list-view/filter infra; **NO saved filters
   for MVP** — would need a `saved_filters.view_name` CHECK migration) + run-detail page +
   `datastore` list/count/export methods + web API route + own export.
   **OPEN DESIGN (not yet figured out):** a key extract is **"nodes failing the prospective
   CC19 (target-version) run."** Discriminator = `chef_version` (prod run = current e.g.
   18.x; speculative = target 19.x), so `chef_version=target ∧ status=failure`. But the
   deliverable is a **distinct-NODE rollup** ("these N nodes fail on 19 + each one's latest
   error/failing cookbook·recipe"), not a raw run list — likely a "target-version failures"
   MODE of Run events. Open Qs: latest-run vs ever-failed; per-node columns; relationship to
   the existing static readiness signal (this is the stronger empirical one; feeding it into
   readiness is MVP-out-of-scope); confirm speculative runs ingest with `chef_version=target`.
   Design deliberately before/within the Run events build.
2. **Live-validate the other two shapes** — Automate Data Feed (customer's clustered-org
   transport; fixtures are authored → fidelity risk) and Chef Server proxy (the DMZ
   transport; same `run_converge` shape as the proven direct path). `chef-load` on
   `dev.home.arpa` for a small firehose. Needs an Automate admin token (classifier blocks
   me minting it — user runs it or adds a permission rule).
3. **Lab cleanup** — node `data_collector.rb` + `ssl_verify_mode :verify_none` still set.

Acceptance: a converge failure shows with error+backtrace+failing cookbook·recipe;
partitions rotate; unauth POST accepted; **and a DMZ ingest-only node (no node_snapshots)
is visible in the UI with its runs** (the topology-driven criterion I missed first pass).

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
