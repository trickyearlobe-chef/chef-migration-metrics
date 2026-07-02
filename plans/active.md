# Active Plan

## In flight — Cop list full universe + durability (`feature/cookstyle-violations-browser`)

- **Cops-list universe fix** (uncommitted): `GET /cookstyle/cops` now lists all
  known cops (curated + mappings + custom + scanned), with a `triggered_only`
  toggle. Cop Analysis stays triggered-only.
- **Durability** (`plans/cop-classification-durability.md`): #2 live `--show-cops`
  inventory + drift report → #3 seed the static tables into the DB (include #3
  while the branch is unshipped). (#1 department-default classification done.)

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
