# Data Layer Revamp

## Context

The application has duplicated derived calculations (complexity scores, readiness, blast radius, TK status) computed independently in API handlers, frontend sort comparators, dashboard aggregation, and export formatters. When logic drifts between copies, filters disagree with dashboards. See tech debt: "Duplicated Derived Calculations".

Additionally, cross-org aggregations (roles/cookbooks presented with `organisations []string`) may produce incorrect counts if array lengths are mistaken for entity counts. See tech debt: "Cross-org aggregations may produce incorrect counts".

## Phases

### Phase 0: Backup/Restore ✓ DONE

Safety net before schema changes. Backup create/restore, cron scheduler, maintenance mode, schema version display.

### Phase 1: Semantic Contracts (NEXT)

Define exactly what each metric means and how it's derived. Audit every calculation path to ensure consistency. Produce a single source of truth for each derived value.

- Catalogue all derived metrics (readiness, complexity, blast radius, TK status, etc.)
- For each: document inputs, formula, where it's calculated, where it's consumed
- Identify discrepancies between calculation sites
- Validate cross-org counting logic (roles/cookbooks with org arrays)
- Output: specification per metric with canonical definition

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
