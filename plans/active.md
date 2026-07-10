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

Delivery order: (1) roles fix ✅ DONE, (2) data-layer query diagnostics ✅ DONE
(v2.16.1 admin EXPLAIN runner), (3) node_snapshots big problem with real diags.

- **P2 roles — DONE.** `role_summary` materialisation shipped; confirmed sub-second at
  customer scale via the admin EXPLAIN runner. Root cause eliminated. (Residual =
  `COUNT(*) OVER()` full-count + external-merge sort — same class as P3, folded into the
  P1/P3 fix directions, not a roles-specific task.)
- **P1 coverage full-scan hotspot — NEXT.** Proven: `cookbooks ? $1` unindexed Seq
  Scan over ~119k `node_snapshots` (712 ms/query, 119,073 rows removed by filter),
  driven by the per-org coverage loop (`collector.go:1476`). Candidate fix: GIN index
  on `node_snapshots.cookbooks` (jsonb_path_ops) → `?` index scan. Evaluate insert
  write-cost tradeoff; TDD.
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
