# Data Layer Revamp

## Context

The application has duplicated derived calculations (complexity scores, readiness, blast radius, TK status) computed independently in API handlers, frontend sort comparators, dashboard aggregation, and export formatters. When logic drifts between copies, filters disagree with dashboards. See tech debt: "Duplicated Derived Calculations".

Additionally, cross-org aggregations (roles/cookbooks presented with `organisations []string`) may produce incorrect counts if array lengths are mistaken for entity counts. See tech debt: "Cross-org aggregations may produce incorrect counts".

## Observed Bugs (from customer demo)

These drive the need for this revamp:

1. Node readiness shows TK passes but no TK runs have actually passed (different definition of "TK passed" in readiness evaluator vs dashboard)
2. Trend graphs don't react to staleness filters
3. Filtering nodes by non-existent policy shows no filtering effect
4. Filtering nodes by matching roles gives incorrect results
5. Filtering nodes by chef version gives incorrect matches
6. Chef version filter has debounce/validation bug — `12` works, `12.` clears, back to `12` shows no matches
7. Role filter: partial text entry doesn't filter, only autocomplete selection works (typing `sample` doesn't match but selecting from dropdown does)
8. Export buttons don't work / exports should be "filtered view of current page" not separate buttons
9. Cookbook view: selecting "fresh" still shows inactive cookbooks — need staleness to mean "not referenced by any fresh node"
10. Consistency errors in calculations obvious to humans but not caught by tests
11. Cookstyle "passed" semantics confusing — means no FATAL offenses, not zero offenses
12. Derived status computed differently at multiple call sites (5+ places call ComputeTKStatus with different input prep)
13. `handleGitRepos` loads ALL cookstyle + ALL kitchen results into memory then filters in Go
14. Blast radius / complexity scores calculated independently in API handlers, frontend sorts, dashboard aggregation, exports
15. Collection-run gating missing — trends include partial collection data
16. Metric snapshots don't partition cleanly by collection run

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

### Phase 2: Write-Time Materialisation

Push derived calculations into the DB at collection time. API surfaces read pre-computed values instead of re-deriving.

- Add materialised columns/tables for each metric defined in Phase 1
- Compute at end of collection run (single writer)
- Remove duplicate calculation logic from handlers/frontend
- Enables correct server-side sort/filter/pagination

### Phase 3: Server-Side Pagination

With materialised values in DB, implement proper server-side pagination with correct sort order.

### Phases 4–8: TBD

Further improvements (index optimisation, query plan audit, caching strategy) to be planned after Phase 3.

## Principles

- Each phase builds on the previous — no skipping
- Backup exists as safety net for schema changes
- TDD for all new logic
- No silent divergence from specs
