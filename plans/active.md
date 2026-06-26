# Active Plan

**CookStyle status vocabulary & consistency — COMPLETE.** All chunks (1–8b)
landed; "done lives in code". Chunk 8b shipped the trend-recompute engine
(`internal/analysis/cookstyle_recompute.go`: re-derive status + weighted
complexity from `cookstyle_offence_fingerprints` valid-at-T under the current
resolver, bounded to current membership), the
`/dashboard/cookstyle/recompute-trend` endpoint + frozen-boundary marker
(`recompute_available_from`), the Recomputed-Trend dashboard card, and a
`cookstyle_status` rollup column in the cookbook-remediation export.

Current: **Spec/Plan Drift Control** — see `plans/spec-drift-control.md`.
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
