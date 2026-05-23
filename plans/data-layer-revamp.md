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
| 3 | Filtering nodes by non-existent policy shows no filtering effect | Phase 4 | Pending |
| 4 | Filtering nodes by matching roles gives incorrect results | Phase 4 | Pending |
| 5 | Filtering nodes by chef version gives incorrect matches | Phase 4 | Pending |
| 6 | Chef version filter debounce/validation bug | Phase 4 | Pending |
| 7 | Role filter: partial text entry doesn't filter, only autocomplete selection works | Phase 4 | Pending |
| 8 | Export buttons don't respect current filters | Phase 6 | Partial (backend ready) |
| 9 | Cookbook "fresh" still shows inactive cookbooks | Phase 5 | Pending |
| 10 | Consistency errors in calculations obvious to humans | Phase 1 | ✅ Fixed |
| 11 | Cookstyle "passed" semantics confusing | Phase 1 | ✅ Fixed |
| 12 | Derived status computed differently at multiple call sites | Phase 1+2 | ✅ Fixed |
| 13 | `handleGitRepos` loads ALL results into memory | Phase 3 | ✅ Fixed |
| 14 | Blast radius / complexity scores calculated independently everywhere | Phase 7 | Pending |
| 15 | Collection-run gating missing — trends include partial data | Phase 2 | ✅ Fixed |
| 16 | Metric snapshots don't partition cleanly by collection run | Phase 2 | ✅ Fixed (implicit) |

## Phases

### Phase 0: Backup/Restore ✓ DONE

Safety net before schema changes. Backup create/restore, cron scheduler, maintenance mode, schema version display.

### Phase 1: Semantic Contracts (IN PROGRESS)

Define exactly what each metric means and how it's derived. Audit every calculation path to ensure consistency. Produce a single source of truth for each derived value.

- [x] Catalogue all derived metrics (readiness, complexity, blast radius, TK status, staleness, cookstyle/kitchen status)
- [x] For each: document inputs, formula, where it's calculated, where it's consumed
- [x] Identify discrepancies between calculation sites
- [x] Validate cross-org counting logic (roles/cookbooks with org arrays)
- [x] Output: specification per metric with canonical definition → `.claude/specifications/semantic-contracts.md`
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

Eliminate in-memory fetch-all-then-paginate patterns. Spec: `.claude/specifications/server-side-pagination.md`.

- [x] 3a. Add materialised status columns to `git_repos` + backfill migration + recomputation function
- [x] 3b. Create `GitRepoFilter` SQL builder; migrate `handleGitRepos` to SQL pagination
- [x] 3c. Materialise TK status for cookbooks; remove TK in-memory fallback from `handleCookbooks`
- [x] 3d. Materialise TK status for roles; simplify aggregation query
- [x] 3e. Add sort stability (tie-breaker on unique key) to all list endpoints
- [x] 3f. Wire recompute triggers into all upsert/exclusion/target-change paths

### Phase 4: Node Filter Correctness (Bugs 3–7)

Fix backend filter logic where node queries produce incorrect or missing results. Spec: `.claude/specifications/filter-ux-overhaul.md` (backend section).

- [ ] 4a. Bug 3: Policy name filter — verify SQL WHERE handles non-existent values (returns empty, not unfiltered)
- [ ] 4b. Bug 4: Role filter — fix node-to-role JOIN producing incorrect results (likely OR vs AND or missing expansion_data parse)
- [ ] 4c. Bug 5: Chef version filter — fix ILIKE/prefix matching giving false positives (e.g. `12` matching `12.x` and `112.x`)
- [ ] 4d. Bug 6: Chef version debounce — frontend validation rejects intermediate input (`12.`) then can't recover
- [ ] 4e. Bug 7: Role filter partial text — backend `?q=` prefix search works but frontend only sends on autocomplete select, not on freeform text

### Phase 5: Staleness & Freshness Filters (Bugs 2, 9)

Staleness-aware filtering across all views. Spec: `.claude/specifications/staleness-tiers.md`.

- [ ] 5a. Bug 2: Trend graphs — pass staleness filter param to trend/dashboard aggregation endpoints
- [ ] 5b. Bug 9: Cookbook freshness — "fresh" filter should mean "referenced by at least one fresh node", not just "active"

### Phase 6: Export Filter Parity (Bug 8)

Export respects current page filters. Frontend passes same filter params to export endpoint.

- [ ] 6a. Frontend: wire current filter state into export download URL params
- [ ] 6b. Verify backend export endpoints accept and apply same filters as list endpoints

### Phase 7: Remaining Derived Metric Consolidation (Bug 14)

Push remaining derived calculations (blast radius, complexity scores) to write-time materialisation. Same pattern as TK/cookstyle.

- [ ] 7a. Identify all call sites computing blast radius / complexity
- [ ] 7b. Materialise scores at collection time; API reads stored values
- [ ] 7c. Remove independent re-derivation from frontend sort comparators and export formatters

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
