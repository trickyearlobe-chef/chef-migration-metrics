# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

## Branch map (2026-07-30)

`main` — deployed release at the customer is **v2.18.9**. Collection is hourly; a full
3-org cycle takes ~28 min.

**`main` carries unreleased work: migration 0054** (see NOW below). Cut the next release
once the current batch of work is in — it stops ongoing readiness data loss, so do not
leave it unreleased indefinitely.

**No open branches.**

## NOW — release migration 0054 (merged, unreleased)

`node_readiness` upserts are being rejected: `blocking_cookbooks` (JSONB) sat in a btree
index `INCLUDE` list, and index tuples are capped at 2704 bytes with no TOAST escape.
Measured over six hours: **22,043 rejected writes, 10,605 nodes with readiness older than
their own snapshot, 89 with none at all.** Logged loudly, invisible in the UI, and it
self-selects for nodes with the most blocking cookbooks — the ones a CC19 assessment
depends on. Worsens as incompatibilities accumulate.

Latent since the original schema (0001, recreated in 0009); nothing recent caused it.
No backfill needed — affected nodes correct themselves on the next collection.

Merged to `main`, not yet released.

Release preconditions — the bump target runs no tests:

1. `make ci` and `make vuln-go` must pass first (`bump-patch-push` does not depend on `ci`).
2. **The push is a human step.** CLAUDE.md forbids interacting with remotes; an assistant
   may bump and tag locally (`make bump-patch`) but must not push without explicit,
   per-action authorisation.
3. **Deploy needs a quiet window.** Migrations run at startup (`main.go:2877`), and 0054
   does a plain `DROP`/`CREATE INDEX` inside a transaction, taking an `ACCESS EXCLUSIVE`
   lock on `node_readiness` that blocks reads and writes until the rebuild finishes.
   Confirmed acceptable by the operator; schedule accordingly.

## Queued — collector performance (`plans/collector-performance.md`)

Measured baselines, the invariants learned during the incident, the ruled-out list, and
the open items. Scope decision (2026-07-30): everything **except collector streaming**,
which is parked — it touches the shared collection path with silent-corruption failure
modes and conflicts with runtime-observability Chunk 4. Remaining, in order: log-retention
decoupling, collection history, the early `completed_at` stamp, the Node 20 action bump.

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

## Queued — structural refactors (own branches, `todo-tech-debt.md`)

- `CookstyleStore` sub-interface split (`webapi.DataStore` at **210** methods and growing).
- Extract pipeline stages from the two remediation god-handlers
  (`handle_cookbook_remediation.go` ~499 lines, `handle_git_repo_remediation.go` ~486
  lines — each is one oversized function). No shared extraction; different sources.

## Parked — Spec/Plan Drift Control (`plans/spec-drift-control.md`)

Not on the work list. Chunks A/B/D landed; C and E remain, and any drift report must be
regenerated from scratch. Don't propose or nag to pick this up
(see [[spec-drift-parked]]).

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun —
  `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
