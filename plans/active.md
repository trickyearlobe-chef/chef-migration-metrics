# Active Plan

Current: **CookStyle status vocabulary & consistency** (implementing) — see
`plans/cookstyle-status-consistency.md`. Chunks 1 (SoT derivation foundation),
2 (re-eval propagation + audit), and 3 (API surfacing — materialised
`cookstyle_status` column via migration 0041; scan + propagation + rescore write
it; surfaced in remediation + cookbook/git-repo list responses) landed.
**Next: Chunk 4** (shared CS badge + list adoption, frontend) — run fresh for
clean context. Goal: complete all 8 chunks.

Also open: **Spec/Plan Drift Control** — see `plans/spec-drift-control.md`.
Chunks A (lint) + B/D (rules) landed in `main`. Open:
- **E — drift sweep** (approved; multi-agent spec↔code audit → report). Run next
  session for clean context.
- **C — criteria↔test linkage** (stable IDs on acceptance criteria + a coverage
  script). Prioritise from E's findings.
- Copied-contract backlog: 5 specs still WARN (`diagnostic-bundle`,
  `system-health-{package-layout,frontend,api-endpoint,configuration}`) —
  reference-don't-copy conversion, fold into E.

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun;
  breaks login anchoring + ownership matching — `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
