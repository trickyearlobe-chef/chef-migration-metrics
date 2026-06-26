# Plan — CookStyle Status Vocabulary, Consistency & Single Source of Truth

Branch: `feature/cookstyle-violations-browser` (continuing)
Supersedes chunks 9–11 of `cookstyle-violations-browser.md`.
Status: **implementing.** Decisions below are agreed. Chunks 1–4 released and
landed (SoT derivation; re-eval propagation + audit; API surfacing with a
materialised `cookstyle_status` column; 4-state CS badge + list adoption).
Chunk 5 is next. Remaining chunks 5–8 queued.

## Decisions (agreed)

- **Cop level** (property of a cop): Blocker / Review / Noise / Unclassified.
- **CookStyle rollup** (per cookbook / repo / node): 🟢 Ready / 🟠 Needs review /
  🔴 Blocked / ⚪ Untested. Replaces CS compatible/incompatible/untested +
  passed/failed wording everywhere.
- **CS and TK stay separate** (`dual-compatibility-signals.md`): the rollup is the
  CookStyle signal only; TK keeps passed/failed/partial/untested. Never merge.
- **Readiness**: operator toggle `readiness.review_blocks_readiness` (default
  false, dynamic). Off → Review = ready. On → Review = Needs review (not ready).
- `passed` boolean retained = `status != Blocked` (back-compat for default-off
  readiness + existing consumers).
- **Single source of truth**: ONE derivation `(offences + resolved classification)
  → status + weighted complexity`. Materialise on scan AND on every criteria
  change. All read paths consume the materialised value; badges stop independently
  live-resolving.
- **History**: retroactive recompute of *past* trend points is impossible (raw
  inputs never retained — see findings). Adopt **retroactive-forward**: a
  change-deduped per-scan offence-fingerprint history makes trends recomputable
  for data captured *after* this ships; past points stay frozen.

## Root problem (verified @ 50bde9a, live DB)

`kubernetes-cluster` (target 19.3.15): one cop
`Chef/Deprecations/ResourceWithoutUnifiedTrue`, Review, 5 occ — yet lists
"failing", card "complexity 55/high", detail blocker-filter empty. Pass/fail
(`cookstyle.go:591,738`) and complexity (`complexity.go:160,594`) are
classification-blind; only badges/resolver are correct.

## Single-source-of-truth findings (LSP-verified)

Three independent derivations of "does this pass":
1. Materialised `passed` + `complexity_score` — written at scan time from severity
   rules (`EvaluatePassFail`), re-written by rescore (`cookstyle_rescore.go:101`),
   also severity-only.
2. Live-resolved classification badges/summaries — `Resolve` at read time in
   `handle_git_repo_remediation.go:270`, `handle_cookbook_remediation.go:353`,
   `handle_cookstyle_cops.go:221,287`.
3. A third `EvaluatePassFailWithClassification` only in the Cop Analysis
   "would pass without" calc (`handle_cookstyle_cops.go:499`).

Additional gaps:
- **Reclassification propagates nowhere.** `RescoreCookstyleResults` is invoked
  only from `handle_admin_config_analysis.go:160` (saving failure rules).
  `putCookstyleCopClassification` just upserts a row — no re-eval of passed,
  complexity, compatibility, or readiness.
- **Complexity is never recomputed** even by rescore (only `passed` + git compat).
- **Snapshots store aggregates, not inputs.** `metric_snapshots.Data` is a rolled-
  up blob (`node_metrics_snapshot.go:203,227` stores `{Ready: count, ...}`);
  `cookstyle_results` is current-state (overwritten on rescan). No per-time-point
  offence history exists anywhere → past trends are unrecoverable.

## Derivation / invalidation dependency graph

Each derived value has one derivation and a defined invalidation closure:

| Change event | Stale | Recompute scope | Rescan? |
|---|---|---|---|
| Cookbook/git **code** change | that result's offences | result status+complexity → repo compat → readiness of nodes using it | yes |
| **Cop reclassified** (override) | resolution of that cop | status+complexity of every result containing that cop (affected targets) → compat → dependent nodes | no — re-resolve |
| **Custom-cop** def add/edit | which offences exist | rescan matching files → then as code-change | yes |
| **Failure rules** change | fallback for *unclassified* only | results with unclassified offences → downstream | no |
| **Node run-list** change | that node's cookbook set | **that one node's readiness only** | no |
| Node disk attrs change | that node's disk verdict | that one node's readiness | no |
| **Readiness config / toggle** | readiness derivation (global) | **all** nodes' readiness | no |
| Target version added | per-target resolution | status/complexity/readiness for new target | no |

Principles: `status = f(offences, classification)`; `complexity = f(offences,
classification)`; `compat = f(cs status, tk status)`; `readiness = f(node
membership, per-cookbook compat, disk, readiness config)`. A change invalidates
exactly its downstream closure — nothing global except readiness config and
curated-default/app-upgrade (≈ bulk reclassify).

## Storage model (real scale)

Scanned units = 3,480 active server cookbooks + 2,154 git repos = 5,634; × 3
targets ≈ **16,900 cookstyle results** (`R`). (109,120 total cookbook versions are
irrelevant — only actively-used ones scan.)

Per-scan append-only **fingerprint** (cop_name + count + severity + correctable,
~250 B/result), `bytes/yr ≈ R × f × s`:

| Mode | f | Annual |
|---|---|---|
| Append only on offence change (dedupe) | ~12 | **~50 MB/yr** |
| Weekly scheduled, no dedupe | 52 | ~220 MB/yr |
| Daily, no dedupe | 365 | ~1.5 GB/yr |
| Full offences not fingerprint (×~2.8) | — | up to ~4 GB/yr worst case |

Baseline current set ≈ 4–12 MB. **Storage is not a blocker** with change-dedupe.
SoT current-recompute adds **zero** storage (overwrites columns). The expensive
per-snapshot path is rejected.

## Cross-surface impact map

- Lists: `CookbooksPage`, `GitReposPage`, `CookstyleResultRow`, `StatusBadge`/`CookStyleBadge`
- Detail: `GitRepoRemediationPage`, `CookbookRemediationPage`, `RemediationPage`, `CopAnalysisTab`
- Readiness: `internal/analysis/readiness.go`, readiness card + trend, `/nodes?readiness=`, `AdminReadinessPage`, readiness export
- Trends/exports: `TrendCards` (complexity + readiness), `internal/export/*`
- Config: `AdminCookstylePage` (Failure Rules wording + missing classification mgmt)

## Chunks

### Chunk 1 — SoT derivation foundation (backend)

Scope: `internal/analysis/cookstyle.go`, `cop_classification.go`, `internal/remediation/complexity.go`
Dependencies: none

1. `DeriveCookstyleStatus(offenses, resolver) → Ready|NeedsReview|Blocked` (Untested = caller when no result). Blocked = any Blocker OR any Unclassified that severity-fails; NeedsReview = no blockers + ≥1 Review; else Ready.
2. Classification-aware complexity weighting (Blocker high, Review low, Noise ~0, Unclassified = existing fallback); remove deprecation+manual-fix double-count.
3. Wire into scan: replace `EvaluatePassFail`; set `passed = status != Blocked`; materialise status + complexity.
4. Tests: status truth table; kubernetes-cluster case; unclassified severity-fail → Blocked.

Acceptance: Review-only repo → Needs review, complexity low, passed=true; status/complexity/badges agree.

### Chunk 5 — Detail view redesign (frontend)

Scope: `GitRepoRemediationPage`, `CookbookRemediationPage`
Dependencies: Chunk 3

Verdict headline; three-state badge; collapsible Blocker/Review/Noise/Unclassified sections with counts (Blockers expanded); real empty-state for filters. Tests.

### Chunk 6 — Readiness integration + toggle

Scope: `internal/analysis/readiness.go`, config schema, `web-api-nodes`, dashboard `StatusCards`/`TrendCards`, `AdminReadinessPage`, readiness export
Dependencies: Chunks 1–2

Readiness consumes rollup status; add `readiness.review_blocks_readiness` (default false); node readiness surfaces Ready/Needs review/Blocked across card + trend + `/nodes` filter + admin toggle + export. Tests incl. toggle on/off.

Acceptance: default-off preserves today's ready set; toggle-on moves review-only nodes to Needs review.

Implementation anchors (verified @ b92cf38 — start here, don't re-explore):
- **Per-cookbook verdict (the one switch point):** `readiness.go:checkCookbookCompatibility` (~825) reads `csResult.Passed` / `gitCSResult.Passed` (~849, ~867). Switch to the materialised `CookstyleStatus` (StatusReady/StatusNeedsReview/StatusBlocked from `cookstyle_status.go`). `ready`→compatible, `blocked`→incompatible, `needs_review`→ new "needs review" verdict gated by the toggle.
- **Node rollup:** `readiness.go:evaluateOne` (~559) sets `IsReady`. Add a node-level review concept; persist via the new `review_cookbooks` field (decided — see note below) on `node_readiness`. Default-off: needs_review counts as ready (passed=status!=blocked already preserves this). Toggle-on: review-only node → not ready, status "needs review".
- **Node cookstyle_status vocabulary:** `node_readiness.cookstyle_status` + `check_status.go` currently use `passed/failed/unknown`. Spec `analysis-node-readiness.md` wants three-state node `status`. Decide: either widen this vocab to ready/needs_review/blocked or add a separate node `status` field — keep `check_status.go` consumers working.
- **Config toggle:** add `ReviewBlocksReadiness bool` to `config.go:ReadinessConfig` (~514, default false in setDefaults). Read live via the `ReadinessEvaluator.configFn` pattern (mirror `diskConfig()` ~986). Admin PUT: `handle_admin_config_readiness.go` (~30-66). Frontend: `AdminReadinessPage.tsx`.
- **Buckets to add needs_review:** dashboard `handle_dashboard_readiness.go` (~119) + `CountNodeReadiness` query `node_readiness.go` (~297); trend payload (~280); node list `nodeReadinessSummaryEntry` `handle_nodes.go` (~267) — filter `?readiness_filter=` already supports `needs_review` (`common.ts` `ReadinessFilterValue` already has it).
- Readiness DS test pattern: `readiness_test.go` `fakeReadinessDS` (~28); add toggle-on/off derivation cases.

### Chunk 7 — Admin: classification mgmt + reframe failure rules (frontend)

Scope: `AdminCookstylePage`, new classification-mgmt component
Dependencies: Chunk 3

Searchable list of all cops (resolved class + source, target-version selector, per-cop override, curated defaults visible); reframe Failure Rules as "fallback (unclassified only)". Tests.

### Chunk 8 — Per-scan fingerprint history + retroactive trends (backend + trends)

Scope: new datastore table + writer, snapshot/trend recompute, `TrendCards`, exports
Dependencies: Chunks 1–2, 6

1. Append-only `cookstyle_offence_fingerprints` (per result per scan: cop_name, count, severity, correctable, scanned_at); **dedupe** — only append when fingerprint differs from last.
2. Trend recompute joins membership-at-T to fingerprint-valid-at-T; rebuilds trends under current criteria going forward.
3. Trends + exports use Ready/Needs review/Blocked vocabulary.
4. Tests incl. dedupe + retroactive recompute over a reclassification.

Acceptance: post-ship trend points recompute correctly after a reclassification; past points unchanged; storage grows ~per-change only.

## Spec edits (applied on this branch, uncommitted)

All landed; vocabulary consistent across the set; all files < 500 lines:
- `cop-classification.md` — canonical "CookStyle Rollup Status" (Ready/Needs review/Blocked/**Untested**), CS/TK separation, SoT derivation, Re-evaluation & Propagation + History, 3-section Cop Management Page (Classifications / Custom Cops / Fallback rules).
- `analysis-cookstyle.md` — status + complexity classification-derived (SoT); `status` + derived `passed` persisted.
- `analysis-node-readiness.md` — CookStyle verdict consumes rollup; `review_blocks_readiness` toggle; three-state node `status` + `review_cookbooks`; re-eval note.
- `dual-compatibility-signals.md` — CS badge 4-state; `cs_*` variants renamed; TK unchanged; CS/TK-separate reaffirmed.
- `cookstyle-failure-rules.md` — reframed as Unclassified-cop fallback.
- `web-api-{remediation,server-cookbooks,git-repos}.md` — `cookstyle_status` field.
- `web-api-{nodes,dashboard}.md` — 3-state node `status`, `needs_review` filter, dashboard vocabulary + trend-recompute note.
- `configuration-schema-server.md`, `configuration-full-example.md` — `readiness.review_blocks_readiness`.
- `data-export.md`, `web-api-exports.md`, `visualisation.md` — rollup vocabulary + forward-only trend recompute.
- `enriched-metric-snapshots.md` — change-deduped per-scan offence fingerprint history.

### Executor reconciliation note (RESOLVED)

Node-level review field name: use **`review_cookbooks`** everywhere (it is what
`analysis-node-readiness.md` pins, and the only one already present in code —
`frontend/.../cookstyle-violations.ts`). Drop `review_reasons` / `needs_review_count`
(neither exists in code). Dashboard's per-state count is just a `needs_review`
bucket alongside `ready`/`blocked`; node list reuses the existing
`?readiness_filter=needs_review` (already in `common.ts` `ReadinessFilterValue`).
