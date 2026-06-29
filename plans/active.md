# Active Plan

## Current chunk — CookStyle status precise backfill (finish branch `feature/cookstyle-violations-browser`)

One-time Go re-derivation that replaces the coarse boolean backfills shipped by
this branch's migrations, so first boot shows exact status instead of
approximate-until-next-scan. Completes the status-materialisation feature this
branch introduced (the imprecision is self-healing today, so this is
correctness-completion, not a blocker). Tracked in `todo-tech-debt.md` (CookStyle
§ "Status Materialisation" + "Readiness Integration").

Why coarse today: migration 0041 backfills `cookstyle_status` from `passed`
(`passed→ready` else `blocked`) and 0042 backfills node readiness `status` from
`is_ready` — neither can recover `needs_review`, which needs the stored offences
+ classification, which SQL can't evaluate.

Scope (files):
- `internal/datastore/` — read all server + git result rows (stored `offences`)
  and re-derive `cookstyle_status`; re-derive node readiness `status`.
- `internal/analysis/` — reuse `DeriveCookstyleStatus(offenses, rules, resolver)`
  (`cookstyle_status.go:35`) for result rows; node `status` comes from the
  `ReadinessEvaluator` per-cookbook verdicts under `review_blocks_readiness`.
- migrations `0041`/`0042` (context only — do NOT edit shipped migrations).
- A new boot-time backfill routine OR a Go-migration step (see open Qs).
- Functional test: `internal/datastore/cookstyle_status_functional_test.go`.

Open design Qs (settle first):
- Trigger: idempotent boot-time routine vs a Go-migration step. How to detect
  "needs backfill" without re-running every boot (e.g. a marker row / version
  gate) — must be idempotent and cheap to skip.
- Cost at ~17k result rows + node-readiness rows: batch reads, single pass; the
  resolver/rules load once per target (mirror the complexity scorer's
  `classifierCache`, not the per-item resolver — see tech-debt "Scan-Time
  Classification Override Query Is Per-Item").
- Node readiness re-derivation reuses the org-scoped evaluator cache, not a
  per-node entry point (none exists).

Steps (TDD):
1. Functional test (real DB, `-tags functional`): seed a server result with
   `passed=true` + a review-classified offence; assert backfill re-derives
   `needs_review`, not `ready`. Add a git-result + a node-readiness case.
2. Implement the backfill reusing `DeriveCookstyleStatus` for results.
3. Wire the trigger (idempotent); prove re-run is a no-op.
4. `go test ./...` + the functional suite against the local DB
   (`CMM_TEST_DATABASE_URL`).

Acceptance:
- After backfill, no row carries a coarse status (`needs_review` rows present
  where offences warrant it).
- Idempotent: second run changes nothing.
- Existing scan / reclassification / propagation paths unchanged.
- Functional test fails against the coarse SQL backfill, passes with the Go path.

Dependencies: none. Start in a fresh context — read CLAUDE.md, this plan, and
`specifications/cop-classification.md` only.

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
