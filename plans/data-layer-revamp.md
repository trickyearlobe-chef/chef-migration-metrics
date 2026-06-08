# Data Layer Revamp

## Context

The application has duplicated derived calculations (complexity scores, readiness, blast radius, TK status) computed independently in API handlers, frontend sort comparators, dashboard aggregation, and export formatters. When logic drifts between copies, filters disagree with dashboards. See tech debt: "Duplicated Derived Calculations".

Additionally, cross-org aggregations (roles/cookbooks presented with `organisations []string`) may produce incorrect counts if array lengths are mistaken for entity counts. See tech debt: "Cross-org aggregations may produce incorrect counts".

## Observed Bugs (from customer demo)

These drive the need for this revamp:

| # | Bug | Phase | Status |
|---|-----|-------|--------|
| 1 | Node readiness shows TK passes but no TK runs have actually passed | Phase 1 | ✅ Fixed |
| 2 | Trend graphs don't react to staleness filters | Phase 5 | Pending |
| 3 | Filtering nodes by non-existent role shows no filtering effect | Phase 4 | ✅ Fixed (same as Bug 4) |
| 4 | Filtering nodes by matching roles gives incorrect results | Phase 4 | ✅ Fixed |
| 5 | Filtering nodes by chef version gives incorrect matches | Phase 4 | ✅ Fixed (debounce) |
| 6 | Chef version filter debounce/validation bug | Phase 4 | ✅ Fixed |
| 7 | Role filter: partial text entry doesn't filter, only autocomplete selection works | Phase 4 | ✅ Fixed (same as Bug 4) |
| 8 | Export buttons don't respect current filters | Phase 6 | ✅ Fixed (NodesPage already passes filters to ExportButton) |
| 9 | Cookbook "fresh" still shows inactive cookbooks | Phase 5 | Pending |
| 10 | Consistency errors in calculations obvious to humans | Phase 1 | ✅ Fixed |
| 11 | Cookstyle "passed" semantics confusing | Phase 1 | ✅ Fixed |
| 12 | Derived status computed differently at multiple call sites | Phase 1+2 | ✅ Fixed |
| 13 | `handleGitRepos` loads ALL results into memory | Phase 3 | ✅ Fixed |
| 14 | Blast radius / complexity scores calculated independently everywhere | Phase 7 | ✅ Already resolved — scores materialised at write-time by ComplexityScorer; all handlers read stored values |
| 15 | Collection-run gating missing — trends include partial data | Phase 2 | ✅ Fixed |
| 16 | Metric snapshots don't partition cleanly by collection run | Phase 2 | ✅ Fixed (implicit) |
| 17 | Disk space differs between node list ("Unknown") and detail ("Sufficient") | Phase 4 | ✅ Fixed (`fix/disk-status-version-agnostic`) |

### Bug 17 root cause & prevention

The disk verdict (`sufficient_disk_space`) is **version-invariant** — computed from platform install size + node free space in `evaluateOne`, independent of `target_chef_version` — but stored in every per-target `node_readiness` row and **looked up by target version** in the list view. The detail view renders whichever target rows exist (target-agnostic); the list view selected the row for the globally-selected target version (`highestSemver`, `GlobalFilterContext.tsx`). When that target had no row for a node, the list's `LEFT JOIN` produced `NULL`, rendered as "Disk Unknown", while detail still showed the actual `19.1.164` row as "Sufficient".

**Why the revamp missed it:** the source-of-truth work unified the *derivation* (`deriveDiskStatus`) across views but not *which record represents a node*. Consistency needs the record-**selection** step shared too, not just the derivation function. A `LEFT JOIN` that maps "no row for this target" to the same `NULL` as "evaluated but indeterminate" silently merges two distinct states.

**Fix (read path):** disk filter uses a version-agnostic correlated `EXISTS`/`NOT EXISTS` (`node_snapshot_filter.go`); list badge falls back to any readiness row (`NodesPage.tsx`). **Residual (see tech debt "Duplicated Derived Calculations"):** stop storing the version-invariant disk verdict per target version on the write path.

## Phases

### Phase 0: Backup/Restore ✓ DONE

Safety net before schema changes. Backup create/restore, cron scheduler, maintenance mode, schema version display.

### Phase 1: Semantic Contracts (IN PROGRESS)

Define exactly what each metric means and how it's derived. Audit every calculation path to ensure consistency. Produce a single source of truth for each derived value.

- [x] Catalogue all derived metrics (readiness, complexity, blast radius, TK status, staleness, cookstyle/kitchen status)
- [x] For each: document inputs, formula, where it's calculated, where it's consumed
- [x] Identify discrepancies between calculation sites
- [x] Validate cross-org counting logic (roles/cookbooks with org arrays)
- [x] Output: specification per metric with canonical definition → `specifications/semantic-contracts.md`
- [x] Write conformance tests validating webapi re-derivation matches analysis write-time values
- [x] Add tests proving TK status contract (all callers use canonical function)
- [x] Fix inlined TK status derivation in handle_kitchen_batches.go
- [x] Document known kitchen status divergences with regression tests

### Phase 2: Write-Time Materialisation ✓ DONE

Push derived calculations into the DB at collection time. API surfaces read pre-computed values instead of re-deriving.

- [x] Serve persisted cookstyle_status/kitchen_status from deriveCheckStatus
- [x] Remove redundant override logic in handle_nodes.go (now handled inside deriveCheckStatus)
- [x] Fix GetLatestTestKitchenStatus nil/timed_out handling to match SQL semantics
- [x] Verify frontend only displays API-provided scores (no client-side recomputation)
- [x] Collection run gating: assessed — implicit gating sufficient (see note)
- [x] Remove harmful collection_runs status gate from node queries (caused empty node list on failed runs)

**Note on collection run gating:** Snapshots are only written after their respective steps succeed. The existing architecture implicitly gates by success. The `completed_nodes` CTE was removed entirely — node snapshots use upsert semantics and are valid once written. Orphan cleanup has its own safety guard.

### Phase 3: Server-Side Pagination ✓ DONE

Eliminate in-memory fetch-all-then-paginate patterns. Spec: `specifications/server-side-pagination.md`.

- [x] 3a. Add materialised status columns to `git_repos` + backfill migration + recomputation function
- [x] 3b. Create `GitRepoFilter` SQL builder; migrate `handleGitRepos` to SQL pagination
- [x] 3c. Materialise TK status for cookbooks; remove TK in-memory fallback from `handleCookbooks`
- [x] 3d. Materialise TK status for roles; simplify aggregation query
- [x] 3e. Add sort stability (tie-breaker on unique key) to all list endpoints
- [x] 3f. Wire recompute triggers into all upsert/exclusion/target-change paths

### Phase 4: Node Filter Correctness (Bugs 3–7)

Fix backend and frontend filter logic for node queries.

- [ ] 4a. Bug 4+7: Role filter — add multi-value `Roles []string` to `NodeSnapshotFilter` with exact-match `= ANY(...)` against JSONB array elements. Handler splits comma-separated `?role=` into `Roles` (same pattern as environments/platforms). Fixes substring false positives and multi-select.
- [ ] 4b. Bug 5: Chef version filter — change single-value path from prefix LIKE to exact match (users pick from dropdown, not freeform). Keep prefix only when explicitly requested (future).
- [ ] 4c. Bug 3: Policy name filter — investigate frontend; backend logic appears correct (LIKE '%x%' returns empty for non-existent values). Likely frontend not sending param or clearing it on autocomplete miss.
- [ ] 4d. Bug 6: Chef version debounce — frontend fix: don't clear filter input when intermediate value fails regex validation (e.g. `12.` is incomplete, not invalid).

### Phase 5: Staleness & Freshness Filters (Bugs 2, 9)

Staleness-aware filtering across all views. Spec: `specifications/staleness-tiers.md`.

These are feature work, not quick fixes:

- [ ] 5a. Bug 2: Trend graphs — trends are pre-aggregated in metric snapshots; filtering by current staleness doesn't apply to historical data. Options: (a) re-aggregate at query time filtering by staleness tier at each snapshot timestamp, or (b) store separate stale/fresh trend lines at collection time.
- [ ] 5b. Bug 9: Cookbook freshness — define "fresh cookbook" as "referenced by at least one node with `is_stale = false`". Requires JOIN through `node_snapshots.cookbooks` JSONB. Add `used_by_fresh_nodes` filter param to cookbook endpoint.

### Phase 6: Export Filter Parity (Bug 8)

Export respects current page filters. Frontend passes same filter params to export endpoint.

- [ ] 6a. Frontend: wire current filter state into export download URL params
- [ ] 6b. Verify backend export endpoints accept and apply same filters as list endpoints

### Phase 7: Remaining Derived Metric Consolidation (Bug 14) — RESOLVED

Investigation confirms complexity scores and blast radius are already materialised at write-time by `ComplexityScorer` (persisted to `server_cookbook_complexity` / `git_repo_complexity` tables). All API handlers, exports, and frontend read the stored values. The only read-time derivation is `priority_score = complexity × max(affected_nodes, 1)` in one handler — this is a trivial formula over two stored columns and doesn't warrant a DB column.

No code changes needed.

### Phase 8: Performance & Indexing

Query plan audit, index optimisation, and caching for 120k+ node scale.

- [ ] 8a. EXPLAIN ANALYZE on all list endpoints at scale
- [ ] 8b. Add composite indexes for common filter+sort patterns
- [ ] 8c. Evaluate connection pooling / prepared statement caching

## Principles

- Each phase builds on the previous — no skipping
- Backup exists as safety net for schema changes
- TDD for all new logic
- No silent divergence from specs
