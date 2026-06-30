# Active Plan

## Current chunk — CookStyle full-ruleset scanning + addon cops (`plans/cookstyle-full-ruleset.md`)

Approved 2026-06-30. **On `feature/cookstyle-violations-browser`.** A (drop
`--only` → full ruleset, shared arg/sidecar helper) and B (Noise prefix defaults
for cosmetic departments) are done. Remaining: **C — functional merge-blocker**
verifying the `Lint/DeprecatedClassMethods` blocker now fires end-to-end; D (load
operator addon RuboCop cop files from disk); E (widen autocorrect preview to the
full ruleset). No new UI.

## Queued next — Spec/Plan Drift Control (`plans/spec-drift-control.md`)

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
