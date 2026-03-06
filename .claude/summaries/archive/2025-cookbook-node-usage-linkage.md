# Task Summary: Cookbook-Node Usage Linkage (Step 8)

**Date:** 2025-07-14
**Scope:** `internal/collector/`, `internal/datastore/` — wiring up the cookbook-node usage records that link each node snapshot to the cookbooks it runs

## Context

The collector (`internal/collector/collector.go`) had a fully working collection pipeline (Steps 1–7 and Step 9) but Step 8 — building `cookbook_node_usage` records — was explicitly deferred with a comment: *"deferred to a future iteration — the schema and queries are in place but we need to look up cookbook IDs by (org, name, version) which requires a helper."*

The `cookbook_node_usage` table, the `CookbookNodeUsage` type, and all the repository methods (`BulkInsertCookbookNodeUsage`, `ListCookbookNodeUsageByCookbook`, `CountNodesByCookbook`, etc.) already existed in `internal/datastore/cookbook_node_usage.go`. What was missing was:

1. A way to efficiently resolve cookbook IDs by (org, name, version) without N+1 queries
2. A way to get back node snapshot IDs after bulk insert (the existing `BulkInsertNodeSnapshots` returned only a count)
3. The actual wiring in the collector to build and persist usage records

## What Was Done

### 1. `internal/datastore/cookbooks.go` — New `GetServerCookbookIDMap` method

Added a batch lookup method that returns `map[cookbookName]map[cookbookVersion]cookbookID` for all Chef-server-sourced cookbooks in an organisation via a single `SELECT id, name, version FROM cookbooks WHERE organisation_id = $1 AND source = 'chef_server'` query. This eliminates the N+1 problem that was the stated blocker for Step 8.

### 2. `internal/datastore/node_snapshots.go` — New `BulkInsertNodeSnapshotsReturningIDs` method

Added a variant of `BulkInsertNodeSnapshots` that uses `RETURNING id` and returns `map[nodeName]snapshotID` alongside the inserted count. The existing `BulkInsertNodeSnapshots` method is preserved (unchanged API) — it delegates to a shared `bulkInsertNodeSnapshots(ctx, params, returnIDs bool)` implementation. When `returnIDs` is false, the method uses `ExecContext` (no scan). When true, it uses `QueryRowContext` + `Scan` to capture the generated UUID.

### 3. `internal/collector/collector.go` — Step 8 wired up

**Tracking per-node cookbook versions:** During Step 4 (snapshot param construction), the collector now builds a `nodeCookbookVersions` map (`map[nodeName]map[cookbookName]version`) alongside the existing `activeCookbookNames` set. Nodes with no cookbooks are excluded from this map.

**Switching to ID-returning bulk insert:** The `BulkInsertNodeSnapshots` call was replaced with `BulkInsertNodeSnapshotsReturningIDs`, which returns a `snapshotIDMap` correlating node names to their generated snapshot UUIDs.

**Building usage records (Step 8):** After cookbook upsert (Step 6) and stale marking (Step 7), the collector:
1. Calls `GetServerCookbookIDMap(org.ID)` to get the full cookbook ID lookup map
2. Iterates `nodeCookbookVersions`, resolving each (cookbookName, version) to a cookbook ID and pairing it with the snapshot ID from `snapshotIDMap`
3. Skips entries where the cookbook name or version is not in the ID map (counts as `missingCookbooks`, logged as WARN)
4. Skips entries where the node has no snapshot ID (shouldn't happen, but guarded)
5. Bulk-inserts the resulting `InsertCookbookNodeUsageParams` via `BulkInsertCookbookNodeUsage`

The entire Step 8 is **non-fatal** — if the cookbook ID map query fails or the bulk insert fails, it logs a WARN and the collection run still completes successfully. The node snapshot JSON columns still contain the cookbook data as a fallback.

### 4. Tests added

**Collector tests (12 new):**
- `TestNodeCookbookVersionsTracking` — verifies per-node version map is built correctly
- `TestNodeCookbookVersionsTracking_NoCookbooks` — nodes with nil/empty cookbooks excluded
- `TestNodeCookbookVersionsTracking_MixedNodes` — mix of nodes with and without cookbooks
- `TestBuildUsageParams_AllMatch` — all cookbooks resolve, correct params built
- `TestBuildUsageParams_MissingCookbookName` — unknown cookbook name counted as missing
- `TestBuildUsageParams_MissingCookbookVersion` — known name but unknown version counted as missing
- `TestBuildUsageParams_MissingSnapshotID` — node with no snapshot ID silently skipped
- `TestBuildUsageParams_EmptyInputs` — nil inputs produce empty output
- `TestBuildUsageParams_EmptyNodeCookbooks` — empty cookbook map per node produces no records
- `TestBuildUsageParams_MultipleMissing` — multiple missing cookbooks counted correctly
- `TestBuildUsageParams_SameCookbookDifferentNodes` — same cookbook on two nodes produces two records
- `TestBuildUsageParams_SameCookbookDifferentVersions` — same cookbook at different versions resolves to different IDs

The linkage logic was extracted into a `buildUsageParams()` helper function in the test file, replicating the Step 8 algorithm for isolated testing without a database.

**Datastore tests (3 new):**
- `TestBulkInsertNodeSnapshots_EmptySlice` — nil and empty slice return (0, nil)
- `TestBulkInsertNodeSnapshotsReturningIDs_EmptySlice` — nil and empty slice return (nil, 0, nil)
- `TestBulkInsertCookbookNodeUsage_EmptySlice` — nil and empty slice return (0, nil)

## Final State

- **Build:** `go build ./...` — clean, no errors
- **Vet:** `go vet ./...` — clean
- **Tests:** `go test ./...` — all pass
  - `internal/collector`: 56 tests (was 44, +12 new)
  - `internal/datastore`: 48 tests (was 45, +3 new)
  - All other packages: unchanged, all passing

## Known Gaps

- **Functional DB tests:** The new `GetServerCookbookIDMap` and `BulkInsertNodeSnapshotsReturningIDs` methods need functional tests against a real PostgreSQL instance (build-tagged `//go:build functional`). The unit tests cover the empty-slice fast paths and the linkage logic, but not actual SQL execution.
- **Duplicate node names:** If two nodes have the same name in a single collection run, `BulkInsertNodeSnapshotsReturningIDs` will return the snapshot ID of the last one inserted. The usage records will only link to that last snapshot. In practice, Chef server node names are unique per organisation, so this is not expected to occur.
- **Cookbook version drift:** If the Chef server has a cookbook version that a node reports running but the version was not in the `GetCookbooks` inventory response (rare edge case), the usage record will be skipped with a WARN. It will resolve on the next collection run if the cookbook is present.

## Files Modified

### Production code
- `internal/datastore/cookbooks.go` — added `GetServerCookbookIDMap` / `getServerCookbookIDMap` (batch ID lookup)
- `internal/datastore/node_snapshots.go` — added `BulkInsertNodeSnapshotsReturningIDs`, refactored into shared `bulkInsertNodeSnapshots` with `returnIDs` flag
- `internal/collector/collector.go` — added `nodeCookbookVersions` tracking, switched to `BulkInsertNodeSnapshotsReturningIDs`, implemented Step 8

### Tests
- `internal/collector/collector_test.go` — 12 new tests for version tracking and usage param building
- `internal/datastore/datastore_test.go` — 3 new tests for empty-slice edge cases

### Documentation
- `.claude/specifications/todo/data-collection.md` — marked completed items (multi-org, fault tolerance, periodic job, node persistence, stale flagging, policy persistence, cookbook-node linkage); added new "Cookbook-Node Usage Linkage" section
- `.claude/Structure.md` — updated descriptions for `node_snapshots.go` and `cookbooks.go`
- `.claude/summaries/2025-cookbook-node-usage-linkage.md` — this file