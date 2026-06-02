# Server-Side Pagination — Component Specification

## TL;DR

Move all list endpoints to consistent SQL-level pagination with correct sort, filter, and total-count semantics. Eliminate in-memory fetch-all-then-paginate patterns that break under scale and produce incorrect counts when filters are applied post-fetch.

## Problem

### Current State

| Endpoint | Pagination | Filtering | Sorting | Issue |
|----------|-----------|-----------|---------|-------|
| Nodes | SQL LIMIT/OFFSET | SQL WHERE | SQL ORDER BY | ✓ Correct (reference pattern) |
| Cookbooks | SQL LIMIT/OFFSET | SQL WHERE (mostly) | SQL ORDER BY | TK/ownership filter disables SQL pagination → fetch-all |
| Roles | SQL LIMIT/OFFSET | SQL WHERE (mostly) | SQL ORDER BY | TK filter/sort disables SQL pagination → fetch-all |
| Git Repos | In-memory slice | In-memory loops | In-memory sort | Loads ALL repos + ALL cookstyle + ALL kitchen into RAM |

### Bugs Addressed

- **Bug 13**: `handleGitRepos` loads ALL cookstyle + ALL kitchen results into memory then filters in Go. At scale this causes OOM or multi-second latency.
- **Bug 8** (partial): Export should respect current filters — requires filter contract to be consistent between list and export endpoints.

### Architectural Debt

- Roles/Cookbooks TK filter/sort falls back to in-memory pagination because TK status is not materialised in the DB row. Phase 2 already materialised TK status for nodes; extending this to cookbooks/roles eliminates the fallback.
- Git repos have zero SQL pushdown — the entire approach is "load everything, filter in Go, paginate the slice".

## Goals

1. Git repos endpoint uses SQL-level filter/sort/paginate (eliminate fetch-all)
2. Cookbooks TK filter/sort uses SQL (eliminate in-memory fallback)
3. Roles TK filter/sort uses SQL (eliminate in-memory fallback)
4. All list endpoints share a consistent pagination contract
5. Total count reflects post-filter row count (not pre-filter)

## Non-Goals

- Frontend filter UX changes (covered by filter-ux-overhaul spec)
- Cursor-based pagination (LIMIT/OFFSET is sufficient for our scale <100k rows)
- Export pagination (exports stream full filtered set; no page limits)
- Dashboard aggregation queries (separate endpoints, no pagination)

## Pagination Contract

All list endpoints return:

```json
{
  "data": [...],
  "pagination": {
    "page": 1,
    "per_page": 25,
    "total_items": 1042,
    "total_pages": 42
  }
}
```

Query parameters:
- `page` (int, default 1) — 1-indexed page number
- `per_page` (int, default 25, max 100) — items per page
- `sort` (string) — column name from whitelist
- `order` (string, "asc"|"desc", default "asc")

`total_items` MUST reflect the count after all WHERE filters are applied (using `COUNT(*) OVER()` window function, already used by nodes).

## Implementation Strategy

### Git Repos: SQL Filter Builder

Create `internal/datastore/git_repo_filter.go` with a `GitRepoFilter` struct mirroring the node pattern:

```
GitRepoFilter {
  Name              string   // ILIKE substring
  Compatibility     string   // filter on materialised compatibility_status column
  CloneStatus       string   // exact match
  HasTestSuite      *bool
  TKStatus          string   // filter on materialised tk_status column
  Sort              string   // whitelist: name, compatibility, tk_status, clone_status, last_fetched_at
  SortOrder         string
  Limit             int
  Offset            int
}
```

**Note:** No `TargetChefVersion` field — the application uses a single active target. Status columns are materialised scalars on `git_repos`, recomputed when results are written. Target version change resets all statuses to 'untested'.

### Status Materialisation (scalar columns)

The application uses a single active target Chef version. Cookstyle and kitchen results are invalidated when the target changes. This means scalar status columns directly on entity tables are the correct shape — no per-target keying needed.

Add to `git_repos`:
- `compatibility_status TEXT NOT NULL DEFAULT 'untested'` — derived from cookstyle results
- `tk_status TEXT NOT NULL DEFAULT 'untested'` — derived from active kitchen results
- `tk_passed INTEGER NOT NULL DEFAULT 0`
- `tk_total INTEGER NOT NULL DEFAULT 0`

These are updated when:
- Cookstyle results are upserted or deleted (compatibility)
- Kitchen results are upserted or deleted (TK)
- Kitchen exclusions change (TK)
- Git repos are reset or deleted (both → reset to 'untested')
- Target Chef version changes (all results invalidated → columns reset)

### Cookbooks: Materialise TK Status

Add `tk_status TEXT NOT NULL DEFAULT 'untested'` to `server_cookbook_versions`. Populated when kitchen results are written. Eliminates the TK in-memory fallback in `handleCookbooks`.

### Roles: Materialise TK Status

Roles are cross-org — the same role name can have different TK outcomes per org. The existing `role_snapshots` are already per-org. Add `tk_status TEXT NOT NULL DEFAULT 'untested'` to the role storage. The roles list endpoint aggregates across selected orgs using worst-status logic (same as current in-memory behaviour, just pushed to SQL).

### Materialisation Sync Points

A single recomputation function per entity handles all triggers:
- Kitchen result upsert/delete
- Kitchen exclusion create/delete
- Cookstyle result upsert/delete
- Git repo reset/delete
- Target version change (bulk invalidation)

Prefer transactional recomputation scoped to the affected entity.

### Recomputation Architecture

- Derivation functions are **pure and tested** (e.g. `tkstatus.ComputeTKStatus`).
- A single **recompute service** per entity type calls the pure functions. Multiple events trigger it, but the computation path is singular.
- Direct entity recomputation is **synchronous** (e.g. TK result written → repo status recomputed in same tx).
- Downstream cascades are **async** (e.g. repo status changes → mark affected nodes dirty → background worker batch-recomputes).
- Target version change triggers **bulk invalidation** (reset all to 'untested') in one transaction. Stale workers are guarded by target-generation check.

### Filter Struct Pattern

Each entity has its own filter struct (not one mega-struct). Common fields may be embedded:

- `NodeSnapshotFilter` (existing, reference implementation)
- `GitRepoFilter` (new, Phase 3)
- `CookbookFilter` (existing, to be simplified)
- `RoleFilter` (existing, to be simplified)

Each translates directly to SQL WHERE clauses. No intermediate query DSL.

### Sort Column Whitelists

Each endpoint defines its valid sort columns. Invalid sort values fall through to the default (name). Sort columns must be indexed or be part of a composite index that covers the common filter+sort pattern.

| Endpoint | Sortable Columns | Tie-breaker |
|----------|-----------------|-------------|
| Nodes | node_name, chef_environment, chef_version, platform, ohai_time | (organisation_name, node_name) |
| Cookbooks | name, version, compatibility, active, download_status, tk_status | (name, version, organisation_name) |
| Roles | name, node_count, incompatible_cookbook_count, tk_status | (role_name) |
| Git Repos | name, compatibility, tk_status, clone_status, last_fetched_at | (name) |

## Migration Requirements

- Add `compatibility_status`, `tk_status`, `tk_passed`, `tk_total` columns to `git_repos`
- Add `tk_status` column to `server_cookbook_versions`
- Add `tk_status` column to role storage
- Backfill migration must populate existing rows using the same canonical status computation functions used at write-time (avoid semantic drift between backfill and live)
- Migration must be idempotent and safe with backup/restore
- Add indexes on new columns used in WHERE/ORDER BY

## Empty-Page Total Count

`COUNT(*) OVER()` returns no rows when OFFSET exceeds the result set. Handle this with:
- Clamp requested page to valid range after a separate COUNT query, OR
- Run a fallback `SELECT COUNT(*)` when zero rows are returned from the windowed query

Use the same approach as the existing node endpoint (check current behaviour).

## Testing

- Unit tests for each SQL filter builder (no DB required — test query string generation)
- Integration tests with test DB confirming correct pagination counts
- Regression: verify total_items changes correctly when filters narrow results
- Regression: verify sort order is stable (tiebreaker on unique key per endpoint)
- Regression: verify empty-page case returns correct total (not zero)
- Regression: verify backfill produces identical status to current live derivation
- Performance: verify git repos endpoint no longer loads all cookstyle/kitchen into memory

## Rollout

Phase 3 can be split into sub-steps:
1. Create `git_repo_statuses` table + backfill migration + recomputation function
2. Create `GitRepoFilter` SQL builder + migrate `handleGitRepos` to SQL pagination
3. Materialise TK status for cookbooks + remove in-memory fallback
4. Create `role_tk_statuses` table + materialise + remove in-memory fallback from `handleRoles`
5. Add sort stability (tie-breaker on unique key) to all list endpoints
6. Verify export filter contract matches list filter contract (Bug 8 parity)

Note: Cookbook ownership filtering is also in-memory but is deferred (separate from TK concern — ownership spec owns that).
