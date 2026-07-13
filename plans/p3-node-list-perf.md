# P3 — Node-list heavy path: split the COUNT(*) OVER() (node_snapshots)

## STATUS — PARKED (branch `fix/node-list-count-split`), as of 2026-07-13

**Purpose.** Split the node-list query's `COUNT(*) OVER()` into a separate
`COUNT(*)` so the rows query can use an index (`node_name`) + `LIMIT` instead of
materialising and sorting all ~119k `node_snapshots` rows per request.

**Urgency: LOW. Do NOT rush to the customer.** The nodes page is NOT slow to
users — this query is just the single biggest DB load. Fixing it is worthwhile
for DB pressure, not UX. It touches a core read path shared by the node list AND
export, so it is **deploy-risky and must be thoroughly tested** (identical rows,
identical EXACT pagination total, every filter/sort/page case) before shipping.
Cop Analysis tabs ship first.

**State: implemented, COMPILES, tests NOT yet run.** On this branch:
- `buildNodeSnapshotFilterQuery` — dropped `COUNT(*) OVER()` from the rows query.
- `buildNodeSnapshotCountQuery` — new single-source count-query builder;
  `CountNodeSnapshotsFiltered` refactored to use it (removed a pre-existing
  duplicate inline query — this method already existed).
- `scanFilteredNodeSnapshots` — signature now `([]NodeSnapshot, error)` (no
  trailing total scan); both callers updated (`ListNodeSnapshotsFiltered` now
  fetches the total via `CountNodeSnapshotsFiltered`; `ListNodeSnapshotsForExport`
  updated to the 2-value return).
- Builder tests updated: rows query asserts NO `COUNT(*) OVER()`; added
  count-query builder tests.

**To resume:** `git checkout fix/node-list-count-split`, re-read this doc, then:
1. `go test ./internal/datastore/ ./internal/webapi/` (builder + handler).
2. Functional: `CMM_TEST_DATABASE_URL=... go test -tags functional ./internal/datastore/`
   — assert rows + EXACT total unchanged for all filter/sort/pagination cases.
3. Add the functional EXPLAIN test (default-sort rows query uses
   `idx_node_snapshots_node_name`, no full sort/temp spill) — pattern as P1.
4. Confirm frontend pagination unaffected (exact total preserved).
5. Representative-scale check before any customer deploy.

---


## Problem (proven, customer EXPLAIN)

`ListNodeSnapshotsFiltered` (`node_snapshot_filter.go`) serves the node list with
a single query that appends `COUNT(*) OVER () AS total_count`
(`buildNodeSnapshotFilterQuery`, ~line 207). The window forces PostgreSQL to
materialise every matching row before `LIMIT 50`, then top-N sort those rows.
Customer scale (~119k rows): light path ~440 ms floor, heavy path ~600 ms with a
large temp spill (width≈1796, temp read≈47k). `idx_node_snapshots_node_name`
exists but the window defeats index-scan-then-LIMIT.

## Constraint

**Exact total is mandatory.** The frontend computes `total_pages` and renders
"showing X of N" (`Pagination.tsx`); an estimated count would break pagination.
So the count must stay exact — we just stop computing it via the row-materialising
window.

## Design

Split the single query into two, reusing the shared `buildNodeSnapshotFilterParts`
(cte, join, where, args) so filters can never drift between them:

1. **Count query** — `SELECT COUNT(*) FROM current_nodes cn <join> <where>`. No
   ORDER BY, no LIMIT, no heavy projection → lean aggregate scan; exact count.
2. **Rows query** — the existing SELECT minus `COUNT(*) OVER()`, keeping
   ORDER BY + LIMIT/OFFSET. Without the window, the default `ORDER BY node_name`
   is servable by `idx_node_snapshots_node_name` (index scan + LIMIT 50, no full
   sort, no temp spill).

`ListNodeSnapshotsFiltered` runs both; `scanFilteredNodeSnapshots` stops reading
`total_count` off the rows and takes it from the count query.

## Behaviour

Identical results, order, and exact pagination — purely internal. One nuance:
count and rows are now two statements, so under concurrent writes they can differ
by a row. Acceptable for a periodic-collection dashboard (note it, don't guard).

Non-default sorts (chef_environment, platform, ohai_time, migration_state) still
sort, but over narrower rows without the window; add supporting indexes later if
a non-default sort proves hot. Out of scope here.

## TDD

- **Builder tests** (`node_snapshot_filter_test.go`): update the ones asserting
  `COUNT(*) OVER()` in the rows query (it moves out); add a count-query builder
  test (SELECT COUNT(*), same WHERE, no ORDER BY/LIMIT); assert the rows query has
  no window and keeps ORDER BY + LIMIT.
- **Functional**: seed nodes across orgs/filters; assert (a) rows + pagination
  unchanged vs current behaviour, (b) total exact == full-set count, (c) EXPLAIN
  the default-sort rows query uses `idx_node_snapshots_node_name` (no Seq-Scan+Sort
  spill) at realistic selectivity+ANALYZE (as in the P1 index test).
- Run functional suite (CMM_TEST_DATABASE_URL) after each change.

## Acceptance

- Same rows + exact total for all existing filter/sort/pagination cases.
- Default-sort rows query is index-served (no full sort / temp spill).
- Builder + functional + non-functional suites green.
