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

## Queued — post-merge structural refactors (own branches, `todo-tech-debt.md`)

- `CookstyleStore` sub-interface split (DataStore at 190 methods).
- Split the 978-line `handle_cookstyle_cops.go` god-handler per REST resource.

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun;
  breaks login anchoring + ownership matching — `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
