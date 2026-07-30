# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

## Branch map (2026-07-30)

`main` — released line **v2.18.9**, deployed at the customer. Collection is hourly; a
full 3-org cycle takes ~28 min.

Unmerged branches, none merged without explicit permission:

- `fix/node-readiness-index-row-size` — **code, ready, `make ci` green.** Migration 0054.
  **Merge and release first: this is the only open item with active data loss.**
- `docs/correct-filesystem-shape-claims` — **docs + one test comment, ready, CI green.**
  Corrects the Windows filesystem shape claims and adds the § 1.4.1 census rule.
- `docs/attribute-shape-validation` — **SUPERSEDED, delete without merging.** Its census
  rule was cherry-picked onto the branch above; its other commit records a claim that the
  v2.18.9 revert made false.
- `fix/cookstyle-correctable-flag` — **SUPERSEDED, delete without merging.** Its plan
  file is on the branch above; its other commit edits an `active.md` this one replaces.
- `feature/runtime-observability` — **SUPERSEDED, delete without merging.** Same reason.
- `chore/spec-drift-report` — parked; don't nag to merge (see [[spec-drift-parked]]).

## NOW — merge and release migration 0054

`node_readiness` upserts are being rejected: `blocking_cookbooks` (JSONB) sat in a btree
index `INCLUDE` list, and index tuples are capped at 2704 bytes with no TOAST escape.
Measured over six hours: **22,043 rejected writes, 10,605 nodes with readiness older than
their own snapshot, 89 with none at all.** Logged loudly, invisible in the UI, and it
self-selects for nodes with the most blocking cookbooks — the ones a CC19 assessment
depends on. Worsens as incompatibilities accumulate.

Latent since the original schema (0001, recreated in 0009); nothing recent caused it.
No backfill needed — affected nodes correct themselves on the next collection.

## Queued — collector performance (`plans/collector-performance.md`)

Measured baselines, the invariants learned during the incident, the ruled-out list, and
the open items. Highest value: **role fetch via the search index** (verified viable on
the lab; recovers ~7 min of the 28-minute cycle), then **collector streaming** (the only
fix for peak memory scaling with fleet size).

## Queued — cookstyle correctable flag (`plans/cookstyle-correctable-fix.md`)

Auto-correctable counts read 0 fleet-wide; complexity scores are inflated as a result, so
remediation prioritisation is skewed, not just the display. Root cause verified against
deployed Cookstyle 8.7.6 / RuboCop 1.86.1. Three chunks, plus a re-scan and an explicit
preview reset — without the reset the fix silently no-ops.

## Queued — runtime observability (`plans/runtime-observability.md`)

Heap and goroutine profile capture in the diagnostic bundle, threshold-triggered
auto-capture, heap history. `performance.pprof_enabled` exists but cannot be set by any
means — the plan proposes deleting it rather than wiring it up.

## Queued — Event Ingest follow-ups (`plans/todo-event-ingest.md`)

MVP shipped. Highest-value first:

- **CC19 target-version failing-nodes preset.** Rollup and filters are built and tested;
  `useTargetChefVersion` exists but is unused on `RunEventsPage`, so the target version
  must be picked by hand. This is the wiring.
- **Live-fidelity validation** of Data Feed (fixtures still authored) + Chef Server proxy.
- Lab cleanup; depsolve/attributes-only gap. See `todo-event-ingest.md`.

Note before enabling at the customer: ~11 CCRs/second ≈ 950k events/day. Size the ingest
path against that first.

## Queued — Spec/Plan Drift Control (`plans/spec-drift-control.md`)

Chunks A/B/D landed. Open: **E** (drift sweep — the parked `chore/spec-drift-report`
branch is its output); **C** (criteria↔test linkage). 5 specs still WARN on
copied-contract (`diagnostic-bundle`, `system-health-*`) — fold into E.

## Queued — structural refactors (own branches, `todo-tech-debt.md`)

- `CookstyleStore` sub-interface split (`webapi.DataStore` at **210** methods and growing).
- Extract pipeline stages from the two remediation god-handlers
  (`handle_cookbook_remediation.go` ~499 lines, `handle_git_repo_remediation.go` ~486
  lines — each is one oversized function). No shared extraction; different sources.

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun —
  `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
