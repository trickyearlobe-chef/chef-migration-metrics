# Wire Usage Analysis into Collection Cycle

**Date:** 2026-03-06
**Component:** Collector → Analysis integration

## Context

The `internal/analysis/` package was fully implemented (three-phase cookbook usage analysis with persistence) but was never invoked from the collection pipeline. It existed as a standalone callable with no trigger. The most recent task summary (`2026-role-dependency-and-usage-analysis.md`) identified this as the #1 recommended next step.

The goal was to wire `Analyser.RunUsageAnalysis()` into `collectOrganisation()` so that cookbook usage analysis runs automatically at the end of every collection cycle, using the already-collected in-memory data rather than re-querying the database.

## What Was Done

### 1. `internal/collector/collector.go` — Analyser integration

**New import:** `internal/analysis`

**New field on `Collector` struct:** `analyser *analysis.Analyser` — created during `New()` using `analysis.New(db, logger, concurrency)`. The concurrency value is derived from `cfg.Concurrency.NodePageFetching` (both are per-node parallel operations, so sharing the same bound is appropriate).

**Step 4 enhancement — NodeRecord building:** During the existing search row iteration loop (where `snapshotParams`, `activeCookbookNames`, and `nodeCookbookVersions` are built), a `nodeRecords []analysis.NodeRecord` slice is now populated in parallel using `analysis.NodeRecordFromCollectedData()`. This uses the in-memory `NodeData` fields directly — no database re-read required. Each node's name, platform, platform version, platform family, roles, policy name, policy group, and cookbook versions are captured.

**New Step 10 — Cookbook usage analysis:** Inserted between the role dependency graph (Step 9) and collection run completion (renumbered to Step 11):

1. Builds `[]analysis.CookbookInventoryEntry` from the `serverCookbooks` map already in scope (iterates cookbook names and their version entries)
2. Calls `c.analyser.RunUsageAnalysis(ctx, org.ID, run.ID, nodeRecords, inventoryEntries)`
3. On success: logs total/active/unused/detail counts and duration at INFO level
4. On failure: logs the error at WARN level — **non-fatal**, the collection run still completes successfully

**Step renumbering:** The "Complete the collection run" step was renumbered from Step 10 to Step 11 to accommodate the new analysis step.

### 2. `internal/collector/collector_test.go` — New tests (12 new top-level test functions)

**New helper:** `mockSearchRowWithRolesAndPolicy()` — builds a `SearchResultRow` with configurable roles, policy_name, policy_group, platform_version, and platform_family fields (the existing `mockSearchRow` only supported name, env, chef_version, platform, and cookbooks).

**New helper:** `buildInventoryEntries()` — replicates the Step 10 inventory entry building logic for isolated unit testing without a database.

**NodeRecord building tests (4 tests):**
- `TestNodeRecordBuilding_BasicFields` — classic node with roles, cookbooks, all platform fields; verifies all fields populated correctly
- `TestNodeRecordBuilding_PolicyfileNode` — Policyfile node with policy_name/policy_group, no roles; verifies policy fields set and roles nil
- `TestNodeRecordBuilding_NoCookbooks` — node with nil cookbooks; verifies CookbookVersions is nil
- `TestNodeRecordBuilding_MultipleNodes` — 3 nodes (classic, classic, Policyfile) built into records; verifies correct count and per-node field fidelity

**CookbookInventoryEntry building tests (5 tests):**
- `TestBuildInventoryEntries_Basic` — 2 cookbooks with 3 total versions; verifies all name+version pairs present
- `TestBuildInventoryEntries_Empty` — empty input produces empty output
- `TestBuildInventoryEntries_SingleVersion` — single cookbook with single version; verifies exact fields
- `TestBuildInventoryEntries_ManyVersions` — 1 cookbook with 5 versions; verifies all present
- `TestBuildInventoryEntries_ManyCookbooks` — 5 cookbooks with 1 version each; verifies all names present

**Collector construction test (1 test):**
- `TestCollector_AnalyserCreated` — verifies the `analyser` field is non-nil after `New()`

**New imports added:** `sort` (for role verification), `analysis` package.

### 3. `.claude/Structure.md` — Updated

- Collector description updated from "10-step" to "11-step" collection pipeline
- `collector_test.go` description updated to reflect 160 tests (including subtests) and new test categories (node record building, inventory entries)

## Final State

### Build & Vet

- `go build ./...` — clean, no errors
- `go vet ./...` — clean

### Test Counts

| Package | Tests (incl. subtests) | Status |
|---------|------:|--------|
| chefapi | 74 | ✅ PASS |
| collector | 160 (was 150; +10 new top-level tests expanding to 10 with subtests) | ✅ PASS |
| config | 117 | ✅ PASS |
| datastore | 140 | ✅ PASS |
| logging | 133 | ✅ PASS |
| secrets | 354 | ✅ PASS |
| analysis | 51 | ✅ PASS |
| **Total** | **1029** | ✅ ALL PASS |

### Collection Pipeline (11 Steps)

1. Create collection run row
2. Build Chef API client for the organisation
3. Collect all nodes via concurrent partial search
4. Convert search results to snapshot params; build NodeRecord slice for analysis
5. Persist node snapshots in bulk (returning IDs)
6. Fetch cookbook inventory from Chef server; upsert metadata
7. Mark active/stale cookbooks
8. Build and persist cookbook-node usage records
9. Build and persist role dependency graph
10. **NEW: Run cookbook usage analysis** (three-phase: extraction → aggregation → active/unused flagging → persist)
11. Complete the collection run

### What Changed End-to-End

Previously, the analysis package existed but was never called. Now, every successful collection run automatically triggers cookbook usage analysis for that organisation, persisting aggregated results (active/unused counts, per-cookbook-version node counts, platform breakdowns, role and policy references) to the `cookbook_usage_analysis` and `cookbook_usage_detail` tables — all using in-memory data from the same collection cycle with zero additional database reads.

## Known Gaps

- **No dedicated concurrency config for analysis.** The analyser reuses `cfg.Concurrency.NodePageFetching`. If analysis extraction needs independent tuning, a `concurrency.usage_analysis` config field could be added.
- **Analysis runs synchronously within the collection cycle.** For very large organisations (tens of thousands of nodes), this adds latency to the collection run. If this becomes a bottleneck, analysis could be dispatched asynchronously after run completion.
- **No integration test against real PostgreSQL.** The wiring is tested at the unit level (NodeRecord building, inventory entry building, analyser creation). A functional test that exercises the full `collectOrganisation()` → `RunUsageAnalysis()` path against a real database would increase confidence.

## Files Modified

### Production code
- `internal/collector/collector.go` — added `analysis` import, `analyser` field, NodeRecord building in Step 4, new Step 10 (usage analysis), renumbered Step 11

### Test code
- `internal/collector/collector_test.go` — added `sort` and `analysis` imports, `mockSearchRowWithRolesAndPolicy()` helper, `buildInventoryEntries()` helper, 12 new test functions

### Documentation
- `.claude/Structure.md` — updated collector descriptions (11-step pipeline, 160 tests)
- `.claude/summaries/2026-wire-usage-analysis-into-collection.md` — this file

## Recommended Next Steps (Priority Order)

### 1. Cookbook Content Fetching (`todo/data-collection.md` — 12+ items)
**Why:** Required before CookStyle or Test Kitchen can run. Currently the pipeline fetches cookbook *inventory* (names + versions) but not *content*. This is the critical path blocker for the entire compatibility testing pipeline.

**Sub-tasks (in order):**

1. **Add `collection.data_dir` config field** (default: `/var/lib/chef-migration-metrics`). This is the base directory for all downloaded cookbook content. The config schema already has `exports.output_directory` and `elasticsearch.output_directory` but nothing for cookbook storage. Add to `CollectionConfig`, `setDefaults()`, and `validateCollection()` (must exist and be writable).

2. **Implement filesystem namespacing for downloaded cookbooks.** The spec requires content keyed by `organisation + cookbook name + version` because the same name+version can differ between orgs. The DB already enforces this via `uq_cookbooks_server (organisation_id, name, version)`. The on-disk layout should be:
   - Chef server: `<data_dir>/cookbooks/server/<org_id>/<cookbook_name>/<version>/`
   - Git repos: `<data_dir>/cookbooks/git/<sanitised_repo_url>/`
   - Create a path-building helper function that computes the target directory from source type + org + name + version (or git_repo_url).

3. **Add `download_status`/`download_error` columns** to the `cookbooks` table via a new migration (`0004_cookbook_download_status`). Statuses: `pending`, `ok`, `failed`. Default: `pending`.

4. **Implement Chef API cookbook download** — `GET /organizations/ORG/cookbooks/NAME/VERSION` in the chefapi package. The response contains file manifests with S3/bookshelf URLs; each file must be downloaded and written to the namespaced directory.

5. **Implement immutability skip** — skip download if `download_status = 'ok'` and content directory exists. Retry if `failed` or `pending`.

6. **Implement failure handling** — corrupted downloads, 404, 403, network errors. Non-fatal; logged at WARN; status set to `failed` with error detail.

7. **Wire into collection cycle** — add as a new step after cookbook inventory upsert, before usage analysis.

**Specs:** `data-collection/Specification.md` (§ 2.1–2.5), `analysis/Specification.md` (§ CookStyle Invocation step 2 — confirms directory layout expectation), `configuration/Specification.md` (for config field patterns), `packaging/Specification.md` (§ 2.4 — confirms `/var/lib/chef-migration-metrics/` as the working directory)

### 2. Web API Layer (`internal/webapi/`)
**Why:** Unblocks the frontend dashboard. Need a basic router, read-only endpoints for nodes/cookbooks/usage stats/collection runs/role dependency graph.
**Scope:** New package, ~800–1200 lines. Start with health, version, and read-only list/get endpoints.
**Specs:** `web-api/Specification.md`

### 3. CookStyle Integration (`todo/analysis.md` — Compatibility Testing section)
**Why:** Produces the compatibility data that powers the migration dashboard. Depends on #1 (cookbook content fetching).
**Specs:** `analysis/Specification.md` (§ Compatibility Testing)

### 4. Checkpoint/Resume for Collection Runs (`todo/data-collection.md`)
**Why:** Only remaining item in the Node Collection section. Allows failed collection runs to continue without re-collecting already-persisted data.