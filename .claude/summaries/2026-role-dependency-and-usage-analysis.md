# Role Dependency Graph & Cookbook Usage Analysis

**Date:** 2026-03-06
**Components:** Data Collection (role graph), Analysis (cookbook usage)

## Context

Two related features were implemented in a single session:

1. **Role Dependency Graph** — parsing Chef role run_lists to extract cookbook and role dependencies, building a directed graph, and persisting it to the `role_dependencies` table on every collection run.
2. **Cookbook Usage Analysis** — a new `internal/analysis/` package implementing three-phase usage analysis (parallel extraction, aggregation, active/unused flagging) with persistence to new `cookbook_usage_analysis` and `cookbook_usage_detail` tables.

Both features build on existing collected data (node snapshots, cookbook inventory, role details) and require no new external dependencies.

## What Was Done

### Task 1: Role Dependency Graph (4 todo items completed)

**New files:**
- `internal/collector/runlist.go` — Run-list entry parser and graph builder
  - `ParseRunListEntry()` — regex-based parser for `recipe[cookbook::recipe@version]` and `role[name]` entries; strips recipe names and version pins to extract the cookbook name
  - `ParseRunList()` — batch parser that silently skips invalid entries
  - `BuildRoleDependencies()` — processes all roles' default `run_list` and `env_run_lists`, deduplicates dependencies within each role, produces `[]InsertRoleDependencyParams`
- `internal/collector/runlist_test.go` — 32 tests covering all entry formats, edge cases (self-referencing roles, version pins, multiple recipes from same cookbook, env_run_lists deduplication), and large role sets
- `internal/datastore/role_dependencies.go` — Full CRUD + aggregation layer
  - `InsertRoleDependency()` — single upsert with ON CONFLICT
  - `BulkUpsertRoleDependencies()` — batch upsert in a transaction
  - `ReplaceRoleDependenciesForOrg()` — atomic delete-then-insert for full graph refresh (preferred path)
  - `DeleteRoleDependenciesByOrg()`, `DeleteRoleDependenciesByRole()`
  - `ListRoleDependenciesByOrg()`, `ListRoleDependenciesByRole()`, `ListRoleDependenciesByType()`
  - `ListRolesDependingOnCookbook()`, `ListRolesDependingOnRole()` — reverse lookups
  - `CountDependenciesByRole()`, `CountRolesPerCookbook()` — aggregation queries

**Modified files:**
- `internal/collector/collector.go` — Added Step 9 (role dependency graph) between cookbook-node usage (Step 8) and collection run completion (renumbered to Step 10). Fetches all roles via `GetRoles()`, fetches each role's detail via `GetRole()`, builds the graph with `BuildRoleDependencies()`, persists with `ReplaceRoleDependenciesForOrg()`. Non-fatal — failures are logged as WARN and don't fail the collection run.

### Task 2: Cookbook Usage Analysis (8 todo items + 1 Policyfile item completed)

**New files:**
- `internal/analysis/usage.go` — Three-phase analysis engine
  - `Analyser` struct with configurable concurrency
  - `RunUsageAnalysis()` — orchestrates all three phases and persists results in a single transaction
  - **Phase 1** (`extractTuples`) — parallel per-node extraction using goroutines bounded by a semaphore; produces `(cookbook, version, node, platform, roles, policy)` tuples
  - **Phase 2** (`aggregateTuples`) — synchronous aggregation into per-cookbook-version maps: node counts, node name sets, role sets, policy name/group sets, platform counts, platform family counts
  - **Phase 3** (`buildInventorySet`, `buildActiveSet`) — compares aggregated versions against full server inventory to flag active vs unused
  - `buildDetailParams()` — constructs persistence params for all cookbook versions (active and unused), sorted deterministically by name+version
  - `NodeRecordFromSnapshot()` — converts a `datastore.NodeSnapshot` (with JSONB decoding) into an analysis `NodeRecord`; supports both Chef `automatic.cookbooks` format (`{"name":{"version":"x"}}`) and simplified format (`{"name":"x"}`)
  - `NodeRecordFromCollectedData()` — builds a `NodeRecord` directly from in-memory collection data (avoids re-reading from DB)
  - Helper functions: `marshalSortedStringSet()`, `marshalStringIntMap()`
- `internal/analysis/usage_test.go` — 44 tests covering:
  - Phase 1: single/multiple/Policyfile node extraction, empty inputs, parallel extraction with various concurrency levels
  - Phase 2: single tuple, multiple nodes same cookbook, duplicate node deduplication, different versions, Policyfile references, empty platform/policy fields, nil roles
  - Phase 3: inventory set building, active set building, active/unused counting
  - Detail building: active, unused, mixed, ghost cookbooks (not in inventory), deterministic ordering
  - JSON marshaling helpers: sorted string sets, string-int maps
  - NodeRecordFromSnapshot: both cookbook formats, empty/invalid JSON, missing version keys, mixed formats
  - End-to-end: full 5-node fleet with mixed classic/Policyfile nodes, 8 inventory entries, verifying all aggregated counts, platform breakdowns, role sets, policy references

**New migration:**
- `migrations/0003_cookbook_usage_analysis.up.sql` — Creates `cookbook_usage_analysis` (header) and `cookbook_usage_detail` (per-cookbook-version stats) tables with indexes
- `migrations/0003_cookbook_usage_analysis.down.sql` — Drop both tables

**New datastore layer:**
- `internal/datastore/cookbook_usage_analysis.go` — Full persistence layer
  - `InsertCookbookUsageAnalysis()` / `InsertCookbookUsageAnalysisTx()` — header row insert
  - `InsertCookbookUsageDetail()` / `InsertCookbookUsageDetailTx()` — single detail row insert
  - `BulkInsertCookbookUsageDetails()` / `BulkInsertCookbookUsageDetailsTx()` — batch insert in transaction
  - `GetCookbookUsageAnalysis()`, `GetLatestCookbookUsageAnalysis()`, `ListCookbookUsageAnalyses()`, `GetCookbookUsageAnalysisByCollectionRun()` — header queries
  - `ListCookbookUsageDetails()`, `ListCookbookUsageDetailsByCookbook()`, `ListActiveCookbookUsageDetails()`, `ListUnusedCookbookUsageDetails()` — detail queries
  - `DeleteCookbookUsageAnalysis()`, `DeleteCookbookUsageAnalysesByOrg()` — cleanup
  - Validation helpers, `nullableJSON()` for JSONB columns, row scanning helpers

## Final State

### Test Counts

Including subtests (table-driven tests expand to multiple cases):

| Package | Tests (incl. subtests) | Status |
|---------|------:|--------|
| chefapi | 74 | ✅ PASS |
| collector | 150 (was 79 top-level; +32 new in runlist_test.go, rest are subtests) | ✅ PASS |
| config | 117 | ✅ PASS |
| datastore | 140 | ✅ PASS |
| logging | 133 | ✅ PASS |
| secrets | 354 | ✅ PASS |
| analysis | 51 (new) | ✅ PASS |
| **Total** | **1019** | ✅ ALL PASS |

### Progress Summary

| Area | Done | Total | % |
|------|-----:|------:|--:|
| Specification | 32 | 32 | 100% |
| Project setup | 15 | 16 | 93% |
| Data collection | 43 | 67 | 64% |
| Analysis | 8 | 61 | 13% |
| Visualisation | 0 | 86 | 0% |
| Logging | 11 | 12 | 91% |
| Auth | 0 | 5 | 0% |
| Configuration | 43 | 83 | 51% |
| Secrets storage | 85 | 150 | 56% |
| Packaging | 18 | 97 | 18% |
| Testing | 4 | 40 | 10% |
| Documentation | 0 | 25 | 0% |
| **Total** | **259** | **674** | **38%** |

### What Now Works End-to-End

The collection pipeline (Step 1–10) now:
1. Creates a collection run
2. Builds a Chef API client
3. Collects all nodes via concurrent partial search
4. Converts to node snapshots with stale detection
5. Persists node snapshots (bulk insert with ID return)
6. Fetches cookbook inventory from Chef server
7. Upserts cookbook metadata, marks active/stale
8. Builds cookbook-node usage linkage records
9. **NEW: Builds and persists the role dependency graph**
10. Completes the collection run

The analysis package can then run cookbook usage analysis on the collected data, persisting aggregated results with full platform/role/policy breakdowns.

## Known Gaps

- The `RunUsageAnalysis()` method is not yet wired into the collector's collection cycle or scheduler — it exists as a standalone callable. Wiring it in requires deciding whether analysis runs synchronously after collection or is triggered separately.
- The role dependency graph step fetches each role detail sequentially (one `GetRole()` per role). For organisations with hundreds of roles, this could be slow. A concurrent fetch bounded by a semaphore would be an improvement.
- No integration tests against a real PostgreSQL database for the new datastore methods (`role_dependencies.go`, `cookbook_usage_analysis.go`). The existing pattern uses unit tests with mocked interfaces.

## Files Modified

### Production code
- `internal/collector/runlist.go` (new) — 155 lines
- `internal/collector/collector.go` (modified) — added Step 9 role graph building (39 new lines)
- `internal/datastore/role_dependencies.go` (new) — 502 lines
- `internal/datastore/cookbook_usage_analysis.go` (new) — 642 lines
- `internal/analysis/usage.go` (new) — 561 lines
- `migrations/0003_cookbook_usage_analysis.up.sql` (new) — 62 lines
- `migrations/0003_cookbook_usage_analysis.down.sql` (new) — 8 lines

### Test code
- `internal/collector/runlist_test.go` (new) — 650 lines, 32 tests
- `internal/analysis/usage_test.go` (new) — 1520 lines, 44 tests

### Documentation
- `.claude/specifications/todo/data-collection.md` — 5 items marked done (4 role graph + 1 Policyfile)
- `.claude/specifications/todo/analysis.md` — 8 items marked done (cookbook usage analysis)
- `.claude/specifications/ToDo.md` — progress table updated (259/674 = 38%)
- `.claude/Structure.md` — added new files to project layout

## Recommended Next Steps (Priority Order)

### 1. Wire Usage Analysis into Collection Cycle
**Why:** The analysis package exists but isn't automatically triggered. Add a call to `analyser.RunUsageAnalysis()` at the end of `collectOrganisation()` (after Step 9, before Step 10), building `NodeRecord` slices from the already-collected data rather than re-querying the database.
**Scope:** ~50 lines in `collector.go`, plus building the `CookbookInventoryEntry` list from the `serverCookbooks` map already in scope.
**Specs:** None needed — the plumbing is straightforward.

### 2. Cookbook Content Fetching (`todo/data-collection.md` — 12+ items)
**Why:** Required before CookStyle or Test Kitchen can run. Currently the pipeline fetches cookbook *inventory* (names + versions) but not *content*.
**Scope:** Chef API download method, immutability skip, `download_status`/`download_error` columns (new migration), failure handling, retry logic.
**Specs:** `data-collection/Specification.md` (§ Cookbook Fetching, § Download Failure Handling)

### 3. Web API Layer (`internal/webapi/`)
**Why:** Unblocks the frontend dashboard. Need a basic router, read-only endpoints for nodes/cookbooks/usage stats/collection runs/role dependency graph.
**Scope:** New package, ~800–1200 lines. Start with health, version, and read-only list/get endpoints.
**Specs:** `web-api/Specification.md`

### 4. CookStyle Integration (`todo/analysis.md` — Compatibility Testing section)
**Why:** Produces the compatibility data that powers the migration dashboard. Depends on #2 (cookbook content fetching).
**Specs:** `analysis/Specification.md` (§ Compatibility Testing)