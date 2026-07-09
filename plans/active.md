# Active Plan

## In flight — CookStyle Reliability / Trustworthy Reds (`plans/cookstyle-reliability.md`)

Pivot after the durability work: make CookStyle a reliable migration indicator
(reds = "we know", Review = "operator decides", Noise = "provably harmless"),
strip the non-existent per-target dimension, back out chunk 3 (done). Durability
#1 (dept defaults, reframed as Review-worklist) + #2 (drift) stay; #3 (DB seed)
abandoned. Spec revision gated on user approval (Phase 1). See the plan for the
phased breakdown + acceptance criteria.

## Queued — Spec/Plan Drift Control (`plans/spec-drift-control.md`)

Chunks A (lint) + B/D (rules) landed in `main`. Open:
- **E — drift sweep** (approved; multi-agent spec↔code audit → report).
- **C — criteria↔test linkage** (stable IDs on acceptance criteria + coverage
  script). Prioritise from E's findings.
- Copied-contract backlog: 5 specs still WARN (`diagnostic-bundle`,
  `system-health-{package-layout,frontend,api-endpoint,configuration}`) —
  reference-don't-copy conversion, fold into E.

## Queued — List-view / node_snapshots perf (`plans/list-view-perf.md`)

Delivery order: (1) roles fix, (2) data-layer query diagnostics, (3) node_snapshots
big problem with real diags.

- **P2 roles — diagnosed + design APPROVED** (`plans/roles-perf-design.md`,
  2026-07-07). Root cause: query-time derived aggregation over all ~37k roles
  (`node_count` containment + `role_compat` expansion). Fix: materialise
  `role_summary` (structural cols at collection; compat/tk via existing
  `cookstyle_propagation`). Read path now reads/rolls up `role_summary` (no
  recursive CTE, no seeded path). The O(N²) `array_position` lived only in the
  seeded path and is gone with it; `work_mem` tuning is moot now the recursive
  expansion is off the request path. NEXT: measure list p50 at customer scale to
  confirm the fix, then close P2 and start P1 coverage full-scan.
- **P1 coverage full-scan hotspot — proven** (5.6M unindexed `node_snapshots`
  scans); fix after roles + tooling.
- **P3** — 6 s `node_snapshots` full-row fetches; caller unknown (tooling from step
  2 will identify it).

## Deferred proposal — event ingest (`plans/proposal-event-ingest.md`)

Firehose ingest of Chef `data_collector` converge events. FOR DISCUSSION, after the
perf work. Value: closes the active-only test blind spot (special-job runlist
cookbooks are invisible → untested). Key undecided fork: automatic discovery
(firehose) vs declared special-job runlists (no firehose). Not approved.

## Queued — post-merge structural refactors (own branches, `todo-tech-debt.md`)

- `CookstyleStore` sub-interface split (DataStore at 190 methods).
- Split the 978-line `handle_cookstyle_cops.go` god-handler per REST resource.

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun;
  breaks login anchoring + ownership matching — `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
