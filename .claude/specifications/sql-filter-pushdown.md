# SQL Filter Push-Down — Component Specification

> **Implementation language:** Go. See `../../CLAUDE.md` for language and concurrency rules.

> Component specification for pushing node snapshot filtering from Go application
> memory down into PostgreSQL WHERE clauses, eliminating the load-everything
> bottleneck that prevents the dashboard from scaling to 100k+ nodes.

---

## TL;DR

Replace the current pattern — `ListNodeSnapshotsByOrganisation` loads every node
per organisation into Go memory, then `export.FilterNodes()` discards most of
them — with a single SQL query that applies all filters as a dynamic WHERE
clause, selects only lightweight columns, and returns a total count for
pagination. Follows the existing `ListLogEntries` / `LogEntryFilter` pattern
(argN counter, `nextArg` helper, `WHERE 1=1` base clause). Handlers that
currently loop over organisations and filter in Go will call a single
`ListNodeSnapshotsFiltered` method instead. The `export.FilterNodes` function is
**kept as-is** for the export system, which has different lifecycle requirements.

---

## Problem

### Current Architecture

Every web API handler that needs filtered node data follows the same pattern:

1. Resolve the list of organisations (all, or filtered by query parameter).
2. **For each organisation**, call `ListNodeSnapshotsByOrganisation(ctx, orgID)`
   which executes:
   ```sql
   SELECT ns.* FROM node_snapshots ns
   INNER JOIN collection_runs cr ON cr.id = ns.collection_run_id
   WHERE ns.organisation_id = $1
     AND cr.status = 'completed'
     AND cr.started_at = (SELECT MAX(...))
   ORDER BY ns.node_name
   ```
3. Accumulate all returned `NodeSnapshot` structs into a single `[]NodeSnapshot`.
4. Call `export.FilterNodes(allNodes, filters)` to apply substring/boolean
   filters **in Go**.
5. Slice the filtered result for pagination.

### Why This Does Not Scale

| Concern | Impact at 100k nodes |
|---------|---------------------|
| **Full table scan per org** | Every request deserialises all JSONB columns (`filesystem`, `cookbooks`, `custom_attributes`) — typically 5–50 KB per node — even though list views never display them. |
| **N+1 org loop** | With 20 organisations, the handler issues 20 sequential queries and concatenates results. |
| **In-memory filtering** | Go allocates, scans, and discards the vast majority of rows. GC pressure scales linearly with total node count. |
| **No SQL-level pagination** | `LIMIT`/`OFFSET` is applied after the entire dataset is materialised in Go. Postgres cannot optimise for early termination. |
| **Repeated across handlers** | `handleNodes`, `handleNodesByVersion`, `handleDashboardVersionDistribution`, `handleDashboardPlatformDistribution`, and all `handleFilter*` endpoints independently repeat this pattern. |

---

## Solution: SQL WHERE Clause Push-Down

### Overview

Introduce a `NodeSnapshotFilter` struct and a `ListNodeSnapshotsFiltered`
datastore method that builds a dynamic SQL query with:

- A WHERE clause constructed from the filter fields using parameterised
  placeholders (`$1`, `$2`, …).
- A lightweight column projection that excludes heavy JSONB columns.
- A window-function or CTE-based total count so the caller gets both the
  page of rows and the total matching count in one round-trip.
- `LIMIT` and `OFFSET` applied in SQL.

This follows the established pattern in `ListLogEntries` / `LogEntryFilter`
(see `internal/datastore/log_entries.go` L287–369).

---

## Filter Struct

```go
// NodeSnapshotFilter holds optional filter criteria for listing node snapshots.
// All string filters use case-insensitive substring (ILIKE) matching to
// maintain behavioural parity with export.FilterNodes.
type NodeSnapshotFilter struct {
    // OrganisationIDs restricts results to nodes belonging to these orgs.
    // Empty means all organisations (subject to collection run validation).
    OrganisationIDs []string

    // NodeName filters by case-insensitive substring match on node_name.
    NodeName string

    // Environment filters by case-insensitive substring match on chef_environment.
    Environment string

    // Platform filters by case-insensitive substring match on the combined
    // "platform platform_version" string.
    Platform string

    // ChefVersion filters by case-insensitive substring match on chef_version.
    ChefVersion string

    // PolicyName filters by case-insensitive substring match on policy_name.
    PolicyName string

    // PolicyGroup filters by case-insensitive substring match on policy_group.
    PolicyGroup string

    // Role filters by case-insensitive substring match against any element
    // in the roles JSONB array.
    Role string

    // Stale filters by exact boolean match on is_stale.
    // nil means no filter (return both stale and fresh nodes).
    Stale *bool

    // Limit caps the number of returned rows. 0 means no limit.
    Limit int

    // Offset is the number of rows to skip (for pagination).
    Offset int
}
```

**File location:** `internal/datastore/node_snapshots.go` (alongside the
existing `NodeSnapshot` type).

---

## Method Signature

```go
// ListNodeSnapshotsFiltered retrieves node snapshots matching the given
// filter, ordered by node_name ascending. It returns:
//   - the page of matching snapshots (lightweight projection, no heavy JSONB),
//   - the total count of all matching rows (for pagination metadata),
//   - any error encountered.
//
// Only nodes from completed collection runs are returned.
func (db *DB) ListNodeSnapshotsFiltered(
    ctx context.Context,
    f NodeSnapshotFilter,
) ([]NodeSnapshot, int, error)
```

The three-return-value signature (rows, total, error) enables the handler to
emit `PaginatedResponse` without a separate count query.

---

## SQL Construction

### Base Query Template

```sql
WITH completed_nodes AS (
    SELECT ns.id, ns.collection_run_id, ns.organisation_id, ns.node_name,
           ns.chef_environment, ns.chef_version,
           ns.platform, ns.platform_version, ns.platform_family,
           ns.run_list, ns.roles,
           ns.policy_name, ns.policy_group,
           ns.ohai_time, ns.is_stale, ns.collected_at, ns.created_at
      FROM node_snapshots ns
     INNER JOIN collection_runs cr ON cr.id = ns.collection_run_id
     WHERE cr.status = 'completed'
       AND cr.started_at = (
             SELECT MAX(cr2.started_at)
               FROM collection_runs cr2
              WHERE cr2.organisation_id = ns.organisation_id
                AND cr2.status = 'completed'
           )
)
SELECT *, COUNT(*) OVER () AS total_count
  FROM completed_nodes
 WHERE 1=1
   -- dynamic filters appended here --
 ORDER BY node_name
 LIMIT $N OFFSET $M
```

### Dynamic WHERE Clause Builder

Follow the `ListLogEntries` pattern exactly:

```go
args := []interface{}{}
argN := 0

nextArg := func() string {
    argN++
    return fmt.Sprintf("$%d", argN)
}
```

### Filter Field → SQL Clause Mapping

| Filter Field | SQL Clause | Notes |
|-------------|-----------|-------|
| `OrganisationIDs` | `organisation_id = ANY($N)` | Uses `pq.Array(f.OrganisationIDs)`. Skipped when slice is empty. |
| `NodeName` | `LOWER(node_name) LIKE '%' \|\| LOWER($N) \|\| '%'` | Case-insensitive substring. |
| `Environment` | `LOWER(chef_environment) LIKE '%' \|\| LOWER($N) \|\| '%'` | Case-insensitive substring. |
| `Platform` | `LOWER(CONCAT(platform, ' ', COALESCE(platform_version, ''))) LIKE '%' \|\| LOWER($N) \|\| '%'` | Matches existing `export.FilterNodes` behaviour: `platform + " " + platform_version`. |
| `ChefVersion` | `LOWER(chef_version) LIKE '%' \|\| LOWER($N) \|\| '%'` | Case-insensitive substring. |
| `PolicyName` | `LOWER(policy_name) LIKE '%' \|\| LOWER($N) \|\| '%'` | Case-insensitive substring. |
| `PolicyGroup` | `LOWER(policy_group) LIKE '%' \|\| LOWER($N) \|\| '%'` | Case-insensitive substring. |
| `Role` | `EXISTS (SELECT 1 FROM jsonb_array_elements_text(roles) r WHERE LOWER(r) LIKE '%' \|\| LOWER($N) \|\| '%')` | Substring match against any element in the JSONB array. Matches `nodeHasRole` behaviour. |
| `Stale` | `is_stale = $N` | Exact boolean match. Skipped when `nil`. |
| `Limit` | `LIMIT $N` | Appended after ORDER BY. 0 means omitted (no limit). |
| `Offset` | `OFFSET $N` | Appended after LIMIT. 0 means omitted. |

All string filters are skipped when the field is the zero value (empty string).

### Total Count Strategy

Use `COUNT(*) OVER ()` as a window function in the outer SELECT. This returns
the total matching count on every row without a separate query. The scan
function reads the extra column from each row (it is the same value on every
row; only the first value is used). When zero rows are returned, the total is 0.

**Alternative considered:** A separate `SELECT COUNT(*)` with the same WHERE
clause. Rejected because it doubles query construction and Postgres work.

---

## Lightweight Projection

### Columns Included in List Queries

For list/filter endpoints, only the columns needed by the frontend table view
are selected. This avoids deserialising and transferring heavy JSONB payloads.

| Column | Type | Reason for inclusion |
|--------|------|---------------------|
| `id` | UUID | Primary key, used for detail links and readiness lookups |
| `collection_run_id` | UUID | Foreign key context |
| `organisation_id` | UUID | Organisation scoping and name lookup |
| `node_name` | TEXT | Display, sorting, ownership matching |
| `chef_environment` | TEXT | Display, filter |
| `chef_version` | TEXT | Display, filter, version distribution |
| `platform` | TEXT | Display, filter |
| `platform_version` | TEXT | Display, combined platform string |
| `platform_family` | TEXT | Display |
| `run_list` | JSONB | Run list display in node row (small array) |
| `roles` | JSONB | Role display and role filter (small array) |
| `policy_name` | TEXT | Display, filter |
| `policy_group` | TEXT | Display, filter |
| `ohai_time` | DOUBLE PRECISION | Last check-in display |
| `is_stale` | BOOLEAN | Display, filter |
| `collected_at` | TIMESTAMPTZ | Display |
| `created_at` | TIMESTAMPTZ | Display |

### Columns Excluded from List Queries

| Column | Type | Typical Size | Reason for exclusion |
|--------|------|-------------|---------------------|
| `filesystem` | JSONB | 5–30 KB | Only needed on node detail view |
| `cookbooks` | JSONB | 2–20 KB | Only needed on node detail and by-cookbook queries |
| `custom_attributes` | JSONB | 0.5–10 KB | Only needed for ownership auto-derivation |

The existing `scanNodeSnapshot` function expects all columns. A new
`scanNodeSnapshotLightweight` function will be added that scans the reduced
column set, setting the excluded JSONB fields to `nil`.

The full-column `GetNodeSnapshot` and `GetNodeSnapshotByName` methods remain
unchanged — they are used by the node detail endpoint and export system where
all columns are needed.

---

## Collection Run Validation

### Current-State Semantics

The `node_snapshots` table has entity semantics — `UNIQUE (organisation_id,
node_name)` — so there is exactly one row per node. Each row references the
`collection_run_id` from the most recent completed collection that observed
that node.

The existing `ListNodeSnapshotsByOrganisation` validates this by joining to
`collection_runs` and filtering on the MAX `started_at` for the organisation.
The new filtered query must preserve this guarantee.

### CTE Approach

The CTE `completed_nodes` (shown in the base query above) applies the
collection run validation once, then the outer query applies user-specified
filters. This cleanly separates infrastructure concerns (which rows are valid)
from user concerns (which rows match the filter).

The correlated subquery `WHERE cr.started_at = (SELECT MAX(cr2.started_at) ...
WHERE cr2.organisation_id = ns.organisation_id)` handles the multi-org case
correctly: it selects the latest completed run **per organisation**, not
globally.

### Multi-Organisation Support

When `OrganisationIDs` is empty (no org filter), the query spans all
organisations. The correlated subquery correctly picks the latest completed run
per org. When `OrganisationIDs` is populated, the `organisation_id = ANY($N)`
clause in the outer WHERE restricts the CTE output to matching orgs.

---

## Handlers to Update

### `handleNodes` — Primary Beneficiary

**Current:** Loops over organisations, calls `ListNodeSnapshotsByOrganisation`
per org, concatenates, calls `filterNodes` (which delegates to
`export.FilterNodes`), slices for pagination.

**New:** Build a `NodeSnapshotFilter` from query parameters. Call
`ListNodeSnapshotsFiltered(ctx, filter)`. Use the returned total count for
`PaginatedResponse`. No in-memory filtering or pagination.

The ownership filter (`ownedKeys` map) remains an in-memory post-filter for
now, as ownership is resolved from a separate table. A future optimisation can
push ownership into a SQL JOIN.

### `handleNodesByVersion` — Push `chef_version` Exact Match to SQL

**Current:** Loads all nodes per org, iterates to find exact `chef_version`
match.

**New:** Set `NodeSnapshotFilter.ChefVersion` to the path parameter value. Since
the filter uses ILIKE substring matching, and this handler needs **exact**
match, either:

- Add a `ChefVersionExact` field to `NodeSnapshotFilter` that generates
  `chef_version = $N` instead of an ILIKE clause, **or**
- Use the substring filter (which is a superset) and do a final exact-match
  pass in Go on the (now much smaller) result set.

**Recommended:** Add `ChefVersionExact string` to the filter struct. The SQL
clause is `chef_version = $N` (no ILIKE). This avoids false positives (e.g.
filter for `17.0.0` matching `17.0.0.1`).

### `handleNodesByCookbook` — Keep In-Memory

**Current:** Loads all nodes, inspects the `cookbooks` JSONB column keys.

**New:** No change. This handler requires scanning JSONB keys within the
`cookbooks` column to determine which cookbooks a node uses. Pushing this
into SQL would require a GIN index on `cookbooks` and
`jsonb_exists(cookbooks, $name)`, which is feasible but out of scope for this
change. The `cookbooks` column must still be loaded for this handler, so the
lightweight projection does not apply. Continue using
`ListNodeSnapshotsByOrganisation` here.

### `handleDashboardVersionDistribution` — SQL Aggregate

**Current:** Loads all nodes across all orgs, counts `chef_version` in Go.

**New:** Add a dedicated aggregate method:

```go
func (db *DB) CountNodeVersionDistribution(
    ctx context.Context,
    orgIDs []string,
) (map[string]int, int, error)
```

SQL:

```sql
SELECT COALESCE(NULLIF(chef_version, ''), 'unknown') AS version,
       COUNT(*) AS cnt
  FROM node_snapshots ns
 INNER JOIN collection_runs cr ON cr.id = ns.collection_run_id
 WHERE cr.status = 'completed'
   AND cr.started_at = (
         SELECT MAX(cr2.started_at)
           FROM collection_runs cr2
          WHERE cr2.organisation_id = ns.organisation_id
            AND cr2.status = 'completed'
       )
   -- optional: AND ns.organisation_id = ANY($1)
 GROUP BY version
```

Returns `(versionCounts map[string]int, totalNodes int, error)`.

### `handleDashboardPlatformDistribution` — SQL Aggregate

**Current:** Loads all nodes, builds `platform + " " + platform_version` string
in Go, counts occurrences.

**New:** Add a dedicated aggregate method:

```go
func (db *DB) CountNodePlatformDistribution(
    ctx context.Context,
    orgIDs []string,
) (map[string]int, int, error)
```

SQL:

```sql
SELECT CONCAT(
           COALESCE(NULLIF(platform, ''), 'unknown'),
           CASE WHEN platform_version IS NOT NULL AND platform_version != ''
                THEN ' ' || platform_version
                ELSE ''
           END
       ) AS platform_label,
       COUNT(*) AS cnt
  FROM node_snapshots ns
 INNER JOIN collection_runs cr ON cr.id = ns.collection_run_id
 WHERE cr.status = 'completed'
   AND cr.started_at = (
         SELECT MAX(cr2.started_at)
           FROM collection_runs cr2
          WHERE cr2.organisation_id = ns.organisation_id
            AND cr2.status = 'completed'
       )
   -- optional: AND ns.organisation_id = ANY($1)
 GROUP BY platform_label
```

### Filter Endpoints — SQL DISTINCT Queries

The `handleFilter*` endpoints (`handleFilterEnvironments`,
`handleFilterPlatforms`, `handleFilterPolicyNames`, `handleFilterPolicyGroups`,
`handleFilterRoles`) currently load all nodes and extract unique values in Go.

**New:** Add dedicated DISTINCT query methods:

```go
func (db *DB) ListDistinctChefEnvironments(ctx context.Context, orgIDs []string) ([]string, error)
func (db *DB) ListDistinctPlatforms(ctx context.Context, orgIDs []string) ([]string, error)
func (db *DB) ListDistinctPolicyNames(ctx context.Context, orgIDs []string) ([]string, error)
func (db *DB) ListDistinctPolicyGroups(ctx context.Context, orgIDs []string) ([]string, error)
func (db *DB) ListDistinctRoles(ctx context.Context, orgIDs []string) ([]string, error)
```

Example SQL for environments:

```sql
SELECT DISTINCT chef_environment
  FROM node_snapshots ns
 INNER JOIN collection_runs cr ON cr.id = ns.collection_run_id
 WHERE cr.status = 'completed'
   AND cr.started_at = (
         SELECT MAX(cr2.started_at)
           FROM collection_runs cr2
          WHERE cr2.organisation_id = ns.organisation_id
            AND cr2.status = 'completed'
       )
   AND chef_environment IS NOT NULL
   AND chef_environment != ''
   -- optional: AND ns.organisation_id = ANY($1)
 ORDER BY chef_environment
```

For roles (JSONB array):

```sql
SELECT DISTINCT r.value
  FROM node_snapshots ns
 INNER JOIN collection_runs cr ON cr.id = ns.collection_run_id,
       jsonb_array_elements_text(ns.roles) AS r(value)
 WHERE cr.status = 'completed'
   AND cr.started_at = (
         SELECT MAX(cr2.started_at)
           FROM collection_runs cr2
          WHERE cr2.organisation_id = ns.organisation_id
            AND cr2.status = 'completed'
       )
   -- optional: AND ns.organisation_id = ANY($1)
 ORDER BY r.value
```

### `export.FilterNodes` — No Change

The `export.FilterNodes` function in `internal/export/filter.go` is **not
modified**. It is used by the export system (`collectBlockedNodes`,
`collectReadyNodes`) which operates on full `NodeSnapshot` slices loaded for
export jobs. The export system has different lifecycle requirements (batch
processing, full JSONB access, no pagination) and does not benefit from the
same optimisation. Keeping it unchanged also means the existing comprehensive
test suite in `export_test.go` continues to validate filter semantics.

---

## Database Migration

### Migration Number

Next available migration: `0002_sql_filter_pushdown.up.sql` /
`0002_sql_filter_pushdown.down.sql`.

> **Note:** Verify the actual next available number at implementation time by
> checking the `migrations/` directory. The consolidated `0001_initial_schema`
> may not be the only migration present.

### Up Migration

```sql
-- =============================================================================
-- Migration: SQL Filter Push-Down Indexes
-- =============================================================================
-- Adds indexes to support the dynamic WHERE clause filters used by
-- ListNodeSnapshotsFiltered and the aggregate/DISTINCT query methods.

-- GIN index on the roles JSONB column to support the EXISTS subquery
-- used by the role filter:
--   EXISTS (SELECT 1 FROM jsonb_array_elements_text(roles) r
--           WHERE LOWER(r) LIKE '%' || LOWER($N) || '%')
CREATE INDEX IF NOT EXISTS idx_node_snapshots_roles_gin
    ON node_snapshots USING GIN (roles);

-- Expression index on the combined platform string to support the
-- platform substring filter:
--   LOWER(CONCAT(platform, ' ', COALESCE(platform_version, '')))
CREATE INDEX IF NOT EXISTS idx_node_snapshots_platform_combined
    ON node_snapshots (LOWER(CONCAT(platform, ' ', COALESCE(platform_version, ''))));

-- Expression index on LOWER(node_name) for case-insensitive substring search.
-- The BTREE index supports LIKE with a constant prefix, but our pattern
-- is '%...%' so this primarily helps if Postgres chooses a seq scan with
-- filter. The existing idx_node_snapshots_node_name covers exact/prefix
-- lookups; this covers the LOWER() expression.
CREATE INDEX IF NOT EXISTS idx_node_snapshots_node_name_lower
    ON node_snapshots (LOWER(node_name));
```

### Down Migration

```sql
DROP INDEX IF EXISTS idx_node_snapshots_roles_gin;
DROP INDEX IF EXISTS idx_node_snapshots_platform_combined;
DROP INDEX IF EXISTS idx_node_snapshots_node_name_lower;
```

### Index Design Rationale

| Index | Type | Purpose |
|-------|------|---------|
| `idx_node_snapshots_roles_gin` | GIN | Required for `jsonb_array_elements_text` containment queries on the `roles` column. Without this, Postgres must seq-scan and decompose every row's roles array. |
| `idx_node_snapshots_platform_combined` | BTREE (expression) | Avoids recomputing `LOWER(CONCAT(...))` on every row. For `%substring%` patterns Postgres may still seq-scan, but the expression is pre-computed. |
| `idx_node_snapshots_node_name_lower` | BTREE (expression) | Pre-computes `LOWER(node_name)` to avoid per-row function evaluation. |

Existing indexes from the initial schema that already support the new queries:

- `idx_node_snapshots_organisation_id` — supports `organisation_id = ANY(...)`.
- `idx_node_snapshots_chef_version` — supports `chef_version = $N` exact match.
- `idx_node_snapshots_chef_environment` — supports environment lookups.
- `idx_node_snapshots_policy_name` — supports policy name lookups.
- `idx_node_snapshots_policy_group` — supports policy group lookups.
- `idx_node_snapshots_is_stale` — supports `is_stale = $N` exact match.
- `idx_node_snapshots_collection_run_id` — supports the JOIN to `collection_runs`.

---

## Datastore Interface Updates

The project uses a `Store` interface (or mock struct) in the web API tests. The
following methods must be added to the interface and the mock:

```go
ListNodeSnapshotsFiltered(ctx context.Context, f NodeSnapshotFilter) ([]NodeSnapshot, int, error)
CountNodeVersionDistribution(ctx context.Context, orgIDs []string) (map[string]int, int, error)
CountNodePlatformDistribution(ctx context.Context, orgIDs []string) (map[string]int, int, error)
ListDistinctChefEnvironments(ctx context.Context, orgIDs []string) ([]string, error)
ListDistinctPlatforms(ctx context.Context, orgIDs []string) ([]string, error)
ListDistinctPolicyNames(ctx context.Context, orgIDs []string) ([]string, error)
ListDistinctPolicyGroups(ctx context.Context, orgIDs []string) ([]string, error)
ListDistinctRoles(ctx context.Context, orgIDs []string) ([]string, error)
```

The existing `ListNodeSnapshotsByOrganisation` method is **not removed** — it
is still used by `handleNodesByCookbook` and the export system.

---

## Testing

### Unit Tests — WHERE Clause Builder

Test the dynamic SQL construction in isolation. These tests do not require a
database connection.

**Test cases:**

| Test | Input | Expected WHERE fragment |
|------|-------|------------------------|
| Empty filter | `NodeSnapshotFilter{}` | No additional WHERE clauses beyond `1=1` |
| Single org | `OrganisationIDs: ["org-1"]` | `AND organisation_id = ANY($1)` |
| Multiple orgs | `OrganisationIDs: ["org-1", "org-2"]` | `AND organisation_id = ANY($1)` |
| Node name | `NodeName: "web"` | `AND LOWER(node_name) LIKE '%' \|\| LOWER($1) \|\| '%'` |
| Environment | `Environment: "prod"` | `AND LOWER(chef_environment) LIKE '%' \|\| LOWER($1) \|\| '%'` |
| Platform | `Platform: "ubuntu"` | `AND LOWER(CONCAT(...)) LIKE '%' \|\| LOWER($1) \|\| '%'` |
| Chef version | `ChefVersion: "17"` | `AND LOWER(chef_version) LIKE '%' \|\| LOWER($1) \|\| '%'` |
| Policy name | `PolicyName: "base"` | `AND LOWER(policy_name) LIKE '%' \|\| LOWER($1) \|\| '%'` |
| Policy group | `PolicyGroup: "prod"` | `AND LOWER(policy_group) LIKE '%' \|\| LOWER($1) \|\| '%'` |
| Role | `Role: "web"` | `AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(roles) ...)` |
| Stale true | `Stale: boolPtr(true)` | `AND is_stale = $1` |
| Stale false | `Stale: boolPtr(false)` | `AND is_stale = $1` |
| Stale nil | `Stale: nil` | No `is_stale` clause |
| Combined filters | Multiple fields set | All corresponding clauses present with correct `$N` numbering |
| Pagination | `Limit: 25, Offset: 50` | `LIMIT $N OFFSET $M` with correct arg positions |

To enable unit testing without a live database, extract the WHERE-clause
building logic into an unexported helper function:

```go
func buildNodeSnapshotFilterQuery(f NodeSnapshotFilter) (string, []interface{})
```

This function returns the complete SQL string and the args slice. Unit tests
call this function and assert on the returned SQL string and argument count.

### Integration Tests — Real Postgres

**Build tag:** `//go:build functional`

These tests run against a real PostgreSQL instance (e.g. via Docker in CI).

**Test cases:**

1. **Filter semantics match `export.FilterNodes`**: Insert a known set of nodes.
   For each filter combination, verify that `ListNodeSnapshotsFiltered` returns
   the same set of node IDs as loading all nodes and running
   `export.FilterNodes`.

2. **Case insensitivity**: Insert nodes with mixed-case environment names.
   Filter with different casing. Verify matches.

3. **Substring matching**: Insert `"production"`, filter with `"prod"`. Verify
   match. Filter with `"staging"`. Verify no match.

4. **Role filter**: Insert nodes with roles `["base", "webserver"]`. Filter
   with `"web"`. Verify match. Filter with `"database"`. Verify no match.

5. **Stale filter**: Insert stale and fresh nodes. Filter with `Stale: true`,
   verify only stale returned. Filter with `Stale: false`, verify only fresh.
   Filter with `Stale: nil`, verify all returned.

6. **Platform combined filter**: Insert node with `platform="ubuntu"`,
   `platform_version="22.04"`. Filter with `"ubuntu 22"`. Verify match.

7. **Pagination**: Insert 50 nodes. Request `Limit: 10, Offset: 20`. Verify
   10 rows returned, total count is 50, correct rows by sort order.

8. **Collection run validation**: Insert nodes referencing a non-completed
   collection run. Verify they are excluded from results.

9. **Multi-org**: Insert nodes across 3 orgs. Filter with 2 org IDs. Verify
   only those orgs' nodes are returned.

10. **Lightweight projection**: Verify that returned `NodeSnapshot` structs
    have nil `Filesystem`, `Cookbooks`, and `CustomAttributes` fields.

11. **Aggregate queries**: Verify `CountNodeVersionDistribution` and
    `CountNodePlatformDistribution` return correct counts.

12. **DISTINCT queries**: Verify each `ListDistinct*` method returns the
    expected unique values.

---

## Implementation Order

1. **Migration** — Add the new indexes.
2. **Filter struct** — Define `NodeSnapshotFilter` in `internal/datastore/node_snapshots.go`.
3. **Query builder unit tests** — Write tests for `buildNodeSnapshotFilterQuery`.
4. **Query builder** — Implement `buildNodeSnapshotFilterQuery` and
   `ListNodeSnapshotsFiltered`.
5. **Lightweight scanner** — Implement `scanNodeSnapshotLightweight`.
6. **Aggregate methods** — `CountNodeVersionDistribution`,
   `CountNodePlatformDistribution`.
7. **DISTINCT methods** — `ListDistinct*` methods.
8. **Interface and mock updates** — Add new methods to the `Store` interface
   and the test mock.
9. **Handler updates** — Update `handleNodes`, `handleNodesByVersion`,
   `handleDashboardVersionDistribution`, `handleDashboardPlatformDistribution`,
   and `handleFilter*` handlers.
10. **Integration tests** — Write `//go:build functional` tests.
11. **Verify parity** — Cross-check filtered results against `export.FilterNodes`
    on a representative dataset.

---

## Performance Expectations

| Metric | Before (100k nodes, 10 orgs) | After |
|--------|------------------------------|-------|
| Data transferred from Postgres | ~500 MB (all JSONB) | ~5 MB (lightweight columns, one page) |
| Go heap allocation | ~500 MB peak | ~100 KB (one page of structs) |
| Query count per request | 10 (one per org) | 1 |
| Latency (p95) | 2–5 s | < 100 ms |
| Postgres work | 10 full table scans | 1 index-assisted scan with LIMIT |

---

## Related Specifications

- [Datastore Specification](./datastore.md) — table schema, indexes, constraints
- [Web API Specification](./web-api.md) — endpoint contracts, pagination, filters
- [Visualisation Specification](./visualisation.md) — frontend filter UI
- [Configuration Specification](./configuration.md) — per-page defaults