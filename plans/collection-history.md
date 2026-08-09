# Retain Collection History

**Parked (2026-07-30) — do not propose picking this up.** Complex and risky for a small,
non-user-facing gain. The diagnostic question that motivated it ("is the cycle getting
slower?") is answerable today from logs: per-org and total durations are already logged
(`collector.go:677`, `:685`) into `log_entries`, which now retains 90 days. Revisit only
when `semantic-contracts.md` § 10 becomes the goal — misleading trend dips from partial
runs — because that needs the `run_id` this plan creates and log parsing cannot supply it.

Verified against `main` @ `7d2277e`. Revised after an adversarial review; the first draft
would have destroyed the history it exists to create (see Fatal flaw below).

## This implements an existing contract

`journeys/semantic-contracts.md` § 10 "Collection Run Gating" already specifies:
each run gets a unique `run_id`; metric snapshots reference it; only COMPLETE runs feed
trend queries. This is unimplemented spec, not new design. Treat § 10 as the acceptance
target.

## Problem

`collection_runs` has `PRIMARY KEY (organisation_name)` — one row per organisation, ever.
`createCollectionRun` (`collection_runs.go:68-84`) upserts on that key and clears the
previous run's results. No duration trend can exist.

`PurgeOldCollectionRuns` (`collection_runs.go:428-430`) is `return 0, nil`. It is **not**
why history is missing.

## Fatal flaw in the naive change

The four mutators match on organisation alone, with **no status predicate**:

- `UpdateCollectionRunProgress` — `WHERE organisation_name = $1` (`collection_runs.go:120`)
- `CompleteCollectionRun` — `:157`
- `FailCollectionRun` — `:189`
- `InterruptCollectionRun` — `:220`

The instant a second row exists per org, each rewrites **every historical row for that org**.
`InterruptCollectionRun` is the worst: it runs on every startup sweep
(`cmd/chef-migration-metrics/main.go:729`) and on shutdown (`collector.go:806`), so the first
restart after deployment would stamp all history `interrupted`.

It fails **silently**: these use `QueryRowContext` (`:126, :163, :195, :226`) and
`sql.Row.Scan` takes the first row and discards the rest without error. A mass-overwrite
returns success.

**This is the single most important requirement in this plan: every mutator targets a run
id and must fail loudly if it would affect more than one row.**

## Corrections to assumptions (do not reintroduce)

- **The PK is not a concurrency guard.** The upsert means two concurrent callers both
  succeed and both get `status='running'`, clobbering each other. The only guard is
  `Collector.IsRunning()` (`collector.go:231-235`) — a single **process-wide** bool, not
  per-org — checked at `scheduler.go:225`.
- **`ResumeCollectionRun` (`collection_runs.go:289`) has no callers.** Dead code. The real
  path, `ResumeInterruptedRuns` (`collector.go:271-403`), *abandons* the interrupted run to
  `failed` (`:377-383`) and creates a new one (`:395`). Nothing "continues the same row".
- **`checkpoint_start` is vestigial.** Never written — `collector.go:1015-1019` omits the
  field, so it is always 0; read only for a log message (`:377-378`). Note this is a spec
  divergence: `journeys/data-collection.md:266-267` mandates page-level checkpointing
  that does not exist. Do not derive constraints from that spec section.
- **`GetLatestCollectionRun` already orders correctly** — `ORDER BY started_at DESC LIMIT 1`
  (`collection_runs.go:347-348`). So do `ListCollectionRuns` (`:384-394`) and the filtered
  variants (`:545-552`). The read path needs a tiebreaker, not a rewrite.

## Blocking decision — the early completion

`collector.go:1049` sets `status='completed'` at Step 4b, deliberately: the comment at
`:1042-1048` explains the UI shows nodes only from the latest *completed* run, so completing
early makes fresh node data visible while Steps 5-16 continue. Consequences:

- For most of a run's wall-clock there is **no `running` row**, so a partial unique index on
  `(organisation_name) WHERE status='running'` guards only the node-snapshot phase.
- `anyOrgCollectionRunning` (`handle_dashboard_version.go:203-212`) is already wrong for the
  same reason.
- Duration recorded per run excludes most of the run — task 4.

Two readings, one status column. Options:

1. **Split the concerns (recommended).** `status` tracks the true run lifecycle and only
   reaches `completed` at the end; add a separate marker (e.g. `nodes_completed_at`) and
   point the UI's "fresh nodes" queries at that. Fixes task 4, makes the unique index real,
   and preserves the UX the early completion was for.
2. **Keep early completion.** Accept that the DB cannot enforce one run per org, and that
   duration means "node phase only" — then say so in the column name.

**Resolve before Chunk A.** The schema differs between them.

## Open question — logging linkage

`logging.WithCollectionRunID` (`logging.go:310-312`) is passed `run.OrganisationName` at
~90 sites in `collector.go`, plus `node_metrics_snapshot.go:273,285` and
`analysis/usage.go:181,233`. The JSON tag is `collection_run_id` (`logging.go:246`) and the
console prints `run_id=` (`logging.go:432`). The name lies in three places.

- **Rename (recommended):** `WithCollectionRunOrg`. Mechanical, honest, no schema change.
- **Run-scoped:** thread a real run id through ~90 call sites and add a column. No UI asks
  for per-run log filtering — `LogsPage.tsx` filters by org. Backlog it.

---

## Chunk A — schema, write path, and retention

**Depends on:** the early-completion decision. **Blocks:** B, C.
Retention is folded in deliberately: shipping insert-per-run while
`PurgeOldCollectionRuns` is still a stub gives an unbounded table — the same "expiry that
never runs" shape fixed for `log_entries` in 0055.

Scope: `migrations/0056_*`, `internal/datastore/collection_runs.go`,
`internal/collector/collector.go`, `cmd/chef-migration-metrics/main.go`.

Steps:

1. Surrogate run id as PK; `organisation_name` demoted to an indexed column, FK retained.
   Add `json:"id"` to the `CollectionRun` struct (`collection_runs.go:17-28`) — it currently
   emits no id at all.
2. Every mutator takes a run id and is `WHERE id = $1`. **No org-keyed wrappers.** Callers
   without an id (`main.go:723-729`, `collector.go:322-379`) iterate rows already returned by
   `GetRunningCollectionRuns` / `GetInterruptedCollectionRuns`, which carry identity.
3. Replace `QueryRowContext` in the mutators with an affected-row check; more than one row
   affected is an error, not a silent success.
4. `createCollectionRun` inserts instead of upserting.
5. Partial unique index on `(organisation_name) WHERE status='running'` — only meaningful
   under option 1 of the blocking decision.
6. Implement `PurgeOldCollectionRuns` for real (keep N runs per org or N days, configurable),
   driven by a **ticker**, not the tail of a collection run. Never purge a `running` row.
7. Delete dead code rather than re-targeting it: `ResumeCollectionRun` (`:289`),
   `GetLatestCompletedCollectionRun` (`:356`), and the unused `ListCollectionRuns` interface
   method (`store.go:46`) — all have zero production callers.
8. Decide the multi-`interrupted` case: the CHECK constraint allows any number of
   `interrupted` rows per org. `AbandonCollectionRun`'s `WHERE organisation_name=$1 AND
   status='interrupted'` (`:273`) would update all of them, and
   `GetInterruptedCollectionRuns` (`:246`) would return duplicates per org into the loop at
   `collector.go:314`, double-counting `Abandoned`/`Resumed`.
9. `0056.down.sql` must restore `PRIMARY KEY (organisation_name)`, which is **lossy** — it
   destroys all but one row per org. Record that in the migration.

Acceptance:

- Two sequential runs for one org leave two rows with distinct ids and intact results.
- A mutator matching more than one row returns an error and changes nothing.
- A startup sweep with history present leaves historical rows untouched.
- Retention bounds the table, runs with collection stopped, and never purges a running row.
- Existing rows survive the migration.

### Rollback trap

Migrations auto-apply at startup (`main.go:471,474`). Once the unique constraint on
`organisation_name` is dropped, the **old binary's** `ON CONFLICT (organisation_name)`
(`collection_runs.go:71`) fails with Postgres 42P10 and **all collection stops**. Rolling
the binary back requires rolling the migration back, which is lossy. Call this out in the
release notes.

## Chunk B — honest logging linkage

**Depends on:** A, and the logging decision.

Scope: `internal/logging/`, `internal/collector/collector.go`,
`internal/collector/node_metrics_snapshot.go`, `internal/analysis/usage.go`.

Steps: per the resolved option. Note `internal/analysis/` is a call site and is easy to miss.

Acceptance: the field name matches what it holds, in the helper, the JSON tag and the
console format.

## Chunk C — read path, API, consumers

**Depends on:** A.

Scope: `internal/datastore/collection_runs.go`, `internal/webapi/store.go`,
`internal/webapi/store_mock_test.go`, `internal/webapi/handle_logs.go`,
`internal/webapi/handle_organisations.go`, `internal/webapi/handle_admin_status.go`,
`internal/webapi/handle_dashboard_version.go`, `internal/webapi/handle_admin_diagnostic.go`,
`frontend/src/types/logs.ts`.

Steps:

1. Add a deterministic tiebreaker `, id DESC` to every `ORDER BY started_at DESC`:
   `:328, :347, :367, :391, :402, :537`. `started_at` is `now()` — transaction start — so
   ties are possible and ordering is otherwise non-deterministic.
2. **Bound the diagnostic bundle.** `handle_admin_diagnostic.go:201` calls
   `ListCollectionRunsFiltered` with an empty filter — no limit — under a 5-second timeout
   (`:199`). Fine at 3 rows; with history it dumps the table and starts timing out.
3. Fix the frontend type: `frontend/src/types/logs.ts:32-33` declares `id: string` and
   `organisation_id: string`; the Go struct emits neither (it emits `organisation_name`, and
   no id until step A.1). Already stale today.
4. Update the `DataStore` interface (`store.go:42,46,51,55`) and its mock
   (`store_mock_test.go:306,313,320,327`). The interface's `organisationID` parameter name
   is already wrong — it receives a name.
5. Fix `anyOrgCollectionRunning` (`handle_dashboard_version.go:203-212`) per the
   early-completion decision.

Acceptance: runs list returns many rows, newest first, deterministically; the diagnostic
bundle stays bounded; no consumer reads a duration that excludes most of the run.

## Chunk D — specs

**Depends on:** A–C.

Update: `journeys/semantic-contracts.md` § 10 (mark the contract implemented),
`journeys/data-collection.md:236-288` (lifecycle, crash recovery, and the
checkpoint/resume divergence), `journeys/diagnostic-bundle.md:41` ("current run status
per org" becomes false).

---

## Risks

- **Silent history destruction** via an unscoped mutator. The failure returns success and
  the data is gone. Mitigated only by A.2 and A.3.
- **Collection stops entirely** on a binary rollback without a migration rollback (42P10).
- **Unbounded growth** if the reaper slips — hence folding it into A.
- Silent trend corruption from a duration that excludes most of the run, if the
  early-completion decision is deferred.

## Not in scope

Partitioning `collection_runs` (wrong tool at this row count — ~26k rows/year for 3 orgs
hourly); implementing the page-level checkpointing that `data-collection.md` describes;
the `collection_run_org` denormalisation across other tables (in `todo-tech-debt.md`);
metric-snapshot `run_id` referencing, which is the rest of semantic-contracts § 10 and
deserves its own plan once run ids exist.

## Verified non-issues

Checked so they are not re-investigated: **backup/restore** is a whole-DB `pg_dump` with no
table enumeration; **exports** never read `collection_runs`; **no FKs point at it** (0009
replaced them with unenforced text columns); **no rollup reads it** — trends read
`metric_snapshots`; the **paginated API path** is capped by `ParsePagination`
(`response.go:51-53`).
