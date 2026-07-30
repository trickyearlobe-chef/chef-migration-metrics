# Active Plan

Single source of truth for what is in flight. **Read this first at session start.**

## Branch map (2026-07-20)

- `main` — holds all merged work. Released line: **v2.18.4**.
- **Event Ingest MVP is MERGED** (`25551f4` + Data Feed fixes `f6161e6`/`3b11111`) and
  released. `feature/event-ingest-mvp` is gone. Post-MVP follow-ups → `todo-event-ingest.md`.
- **Parked** `fix/node-list-count-split` — P3: split the node-list `COUNT(*) OVER()`
  into a separate count query (WIP, compiles, tests NOT run). **Low urgency,
  deploy-risky** (shared node-list + export read path) → the nodes page is not
  user-slow, this is just the heaviest DB query; test thoroughly before any customer
  ship. Full state + resume steps in `plans/p3-node-list-perf.md`.
- **Parked** `chore/spec-drift-report` — one-time spec↔code drift report;
  deprioritised behind feature delivery. Don't nag to merge (see [[spec-drift-parked]]).
- No feature branch in flight. Pull the next chunk from Queued/backlog onto a fresh branch.

## NOW — collector run-duration + memory (branch `fix/collector-role-fetch-and-timeouts`)

Production incident 2026-07-29/30: host OOM, ~22GB Go heap on a 32GB box, ~134k nodes
across 3 orgs. Diagnosis below is measured, not inferred.

**Root causes (measured)**

- Role dependency graph fetches roles **one at a time, sequentially**
  (`collector.go:1430`). 31,958 roles × ~55ms round-trip = **29m16s** for one org.
  This dominates run duration; CPU sits at ~0.06/core because it is all network wait.
- Chef API client has **no HTTP timeout** — `http.DefaultClient` (`client.go:143`).
  One stalled connection wedges a run forever; `IsRunning()` stays true and every
  scheduled tick is then skipped, so collection silently stops until restart.
- Node collection materialises the whole fleet as `map[string]interface{}`
  (`client.go:359`) at **~193KB/node measured** (62,046 nodes → 12GB peak). Three orgs
  concurrently ≈ 26GB, which is the observed 22GB.
- Collection cron was `*/10` against a 60–90min run → ~100% duty cycle, so the
  collection peak was the steady state. Operator is moving this to hourly.

**Ruled out by evidence** (do not re-investigate): GIN index on `node_snapshots.cookbooks`
(69MB, zero dead tuples, autovacuum current); `perf.Recorder` (bounded ring buffers,
`perf/stats.go:37`); logging `DBWriter` (synchronous, no buffer); event ingest
(`converge_runs` empty). No leak — goroutines flat at 26–30.

**Chunk A (this branch) — DONE, `make ci` green, uncommitted**

Concurrent role fetch (`concurrency.role_fetching`, default 10), Chef HTTP client
timeouts, and the `filesystem` → `by_pair` search narrowing. Awaiting validation of the
next real collection run.

**Chunk B — item 5: stream node pages + per-batch commits (NOT started)**

Scope: `collector.go:822-987`, `node_snapshots.go:259`.

Makes peak O(page) instead of O(fleet) — the only change that stops peak memory scaling
with fleet size. Blockers to design around, all of which currently assume the complete
node set is in hand:

- `deduplicateSnapshotParams` (pagination-boundary duplicates)
- cookbook aggregation: `allCookbookNames` / `activeCookbookNames` / `activeCookbookVersions`
- `nodeRecords` for usage analysis
- `snapshotParams` is read downstream at `collector.go:1007/1045/1557`, so it stays
  pinned for the org's entire pipeline (including role fetching). Needs projecting down
  to the few fields those consumers actually use.

Riskiest item on the list — shared collection path, silent-corruption failure modes.
Deserves its own branch and a lab run before shipping.

**Queued next (own chunks)**

- **Role fetch via search index** — `/search/role` paginated at 1000 would turn 31,958
  requests into ~32. Better than the Chunk A fan-out; do once the fan-out is proven.
- **Decouple log retention from collection runs** — purge only fires at the tail of a
  successful run (`collector.go:679`); `log_entries` had 240k dead tuples and autovacuum
  a day stale. Needs a ticker + partitioning so expiry is a `DROP PARTITION`.
- **Keep collection history** — `PurgeOldCollectionRuns` (`collector.go:689`) keeps only
  the latest terminal run per org, so there is no duration trend to diagnose regressions
  with. Retain ~30 days.
- `collection_runs.completed_at` is stamped **early** (Step 4b, `collector.go:1022`), so
  the duration column excludes Steps 5–14. Document or add a true end-to-end duration.

Note: the dev org stays in scheduled collection — it is the CC19 deployment-cookbook
proving ground.

## Queued — Event Ingest follow-ups (`plans/todo-event-ingest.md`)

MVP shipped; these are post-MVP. Highest-value first:

- **CC19 target-version failing-nodes preset.** The generic distinct-node rollup and
  `chef_version ∧ status=failure` filters are built and tested; Run events defaults
  status to `failure`. Missing: the auto-wired "prospective target-version" preset —
  `useTargetChefVersion` exists but is unused on `RunEventsPage`, so the target version
  must be picked by hand. Design is largely decided (generic rollup); this is the wiring.
- **Live-fidelity validation** of Data Feed (fixtures still authored) + Chef Server proxy.
- Lab cleanup; depsolve/attributes-only gap. See `todo-event-ingest.md` for detail.

## Queued — Spec/Plan Drift Control (`plans/spec-drift-control.md`)

Chunks A/B/D landed. Open: **E** (drift sweep — the parked `chore/spec-drift-report`
branch is its output); **C** (criteria↔test linkage). 5 specs still WARN on
copied-contract (`diagnostic-bundle`, `system-health-*`) — fold into E.

## Queued — structural refactors (own branches, `todo-tech-debt.md`)

- `CookstyleStore` sub-interface split (`webapi.DataStore` at **210** methods and growing).
- Extract pipeline stages from the two remediation god-handlers
  (`handle_cookbook_remediation.go` ~499 lines, `handle_git_repo_remediation.go` ~486
  lines — each is one oversized function). No shared extraction between them; they serve
  different sources.

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun —
  `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
