# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

## Branch map (2026-07-30)

`main` — **v2.18.10 released**. Collection is hourly.

**No open branches.**

## NOW — v2.18.11 waiting on the rescan

`main` carries the git repo verdict reset, merged and **unreleased**. Deliberately held:
a rescan is in progress at the customer and a deploy would interrupt it. Cut the tag once
it completes.

The fix only takes effect at the moment `rescan-all-cookstyle` is triggered, so it cannot
help the run in flight — it is for the next one. The list self-corrects as each repo is
scanned (saving a result re-materialises its columns), so the current run still ends with
a correct list.

Release preconditions — the bump target runs no tests:

1. `make ci` and `make vuln-go` must pass first (`bump-patch-push` does not depend on `ci`).
2. **The push is a human step.** CLAUDE.md forbids interacting with remotes; an assistant
   may bump and tag locally but must not push without explicit, per-action authorisation.
3. Local `make ci` needs `TRIVY_SKIP_DB_UPDATE=true` — the 1.2GB Trivy DB pull from
   `mirror.gcr.io` stalls mid-transfer (registry handshake is fine; the blob does not
   move). GitHub CI pulls it fresh, so the gap closes on push.

## Queued — Node 20 deprecation

`softprops/action-gh-release` targets Node 20 and is being forced onto Node 24 by GitHub.
Bump the pinned action before support is withdrawn. Supply-chain check per CLAUDE.md: pin
exact version/SHA. Last survivor of the collector-performance batch.

## Queued — spec drift sweep

Started 2026-07-30 after repeated instances of prose being believed over code. Five specs
cleaned by deleting pasted contracts rather than correcting them. Still drifted:

- `data-collection.md` — mandates page-level checkpointing that does not exist.
  `checkpoint_start` is never written. **Actively misleading**; it produced a false
  constraint in the collection-history plan.
- `enriched-metric-snapshots.md` — describes the fingerprint `correctable` field as a
  re-derivation input; nothing reads it.
- `diagnostic-bundle.md`, `web-api-admin.md` — smaller gaps.

Principle: delete pasted shapes, keep only intent and invariants. No file paths, symbol
paths or line numbers — those rot too.

## Queued — runtime observability (`plans/runtime-observability.md`)

Heap and goroutine profile capture in the diagnostic bundle, threshold-triggered
auto-capture, heap history. `performance.pprof_enabled` exists but cannot be set by any
means — the plan proposes deleting it rather than wiring it up.

## Queued — Event Ingest follow-ups (`plans/todo-event-ingest.md`)

MVP shipped. Highest-value first:

- **CC19 target-version failing-nodes preset.** Rollup and filters are built and tested;
  `useTargetChefVersion` exists but is unused on `RunEventsPage`. This is the wiring.
- **Live-fidelity validation** of Data Feed (fixtures still authored) + Chef Server proxy.
- Lab cleanup; depsolve/attributes-only gap.

Note before enabling at the customer: ~11 CCRs/second ≈ 950k events/day. Size the ingest
path against that first.

## Queued — structural refactors (own branches, `todo-tech-debt.md`)

- `CookstyleStore` sub-interface split (`webapi.DataStore` at 210+ methods and growing).
- Extract pipeline stages from the two remediation god-handlers. No shared extraction;
  different sources.

## Queued — supply chain

Dependabot reports 10 vulnerabilities on the default branch (3 high, 5 moderate, 2 low).
Not contradicted by our clean Trivy run: that scans `frontend/package-lock.json` at
MEDIUM+ with suppressions, while Dependabot covers all manifests. Needs triage.

## Parked — do not propose picking these up

- **Collector streaming** — shared collection path, silent-corruption failure modes,
  conflicts with runtime-observability Chunk 4.
- **Collection history** (`plans/collection-history.md`) and the early `completed_at`
  stamp that rides with it — complex and risky for a small gain; the duration question is
  answerable from logs today.
- **Spec/Plan Drift Control** (`plans/spec-drift-control.md`) — see [[spec-drift-parked]].
- **SAML config follow-ups** — empty `username_attr` warning; local-user username
  collision returning an opaque 500.
