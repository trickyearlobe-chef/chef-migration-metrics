# Active Plan

Current: **CookStyle status vocabulary & consistency** (implementing) — see
`plans/cookstyle-status-consistency.md`. Chunks 1–7 landed: SoT derivation, re-
eval propagation + audit, API surfacing (materialised `cookstyle_status`,
migration 0041), the 4-state CS badge + list adoption, the remediation detail
redesign, readiness integration + `review_blocks_readiness` toggle (3-state node
`status` + `review_cookbooks`, migration 0042), and the admin Cop Classifications
management section + Failure→Fallback Rules reframe. Chunk 8a landed: the
append-only `cookstyle_offence_fingerprints` storage foundation (migration 0043,
change-deduped writer, scan-path hook, contract + functional tests). **Next:
Chunk 8b** (trend-recompute engine over fingerprint-valid-at-T + current
membership; trends/exports Ready/Needs review/Blocked vocabulary) — **start a
fresh thread**. Note: membership-at-T history does not exist
(`node_fact_snapshots` is a Future Extension), so recompute is bounded to current
membership — make that boundary explicit in any mixed-range chart.

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
