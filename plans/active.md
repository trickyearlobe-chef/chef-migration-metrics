# Active Plan

Current: **CookStyle status vocabulary & consistency** (implementing) — see
`plans/cookstyle-status-consistency.md`. Chunks 1 (SoT derivation foundation)
and 2 (re-eval propagation + audit: classification-aware rescore,
`CookstylePropagator` full scoped recompute, `cookstyle_audit_log`) landed.
**Next: Chunk 3** (API surfacing — `cookstyle_status` + weighted complexity in
remediation + list responses) — run fresh for clean context.

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
