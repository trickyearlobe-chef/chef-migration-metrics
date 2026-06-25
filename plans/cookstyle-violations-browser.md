# Plan — Cop Classification & Analysis

Spec: `specifications/cop-classification.md`
Branch: `feature/cookstyle-violations-browser` (continuing)

## Status

- ✅ Chunk 1 — API endpoint (`GET /api/v1/cookstyle/violations`) — committed
- ⚠️  Chunk 2 — Flat-list frontend tab — committed but **will be replaced** by Cop Analysis view
- ✅ Chunk 3 — Data model & classification resolution — committed
- ✅ Chunk 4 — Cop aggregation API — committed
- ✅ Chunk 5 — Cop Analysis frontend tab — committed
- ✅ Chunk 6 — Custom cops management UI — committed
- ✅ Chunk 7 — Updated cookbook/git repo detail views — committed
- ✅ Chunk 8 — Custom cop scanning in analysis pipeline — committed

## Revision — superseded

The accuracy/usability/consistency follow-up (status vocabulary, single source of
truth, readiness, history) moved to its own cross-surface plan:
**`plans/cookstyle-status-consistency.md`**. See there.

## Chunk 3 — Data model & classification resolution (backend)

Scope: `internal/datastore/`, `internal/remediation/`, `migrations/`
Dependencies: none

Steps:
1. Add migration: `cop_classifications` table (cop_name, target_chef_version, classification, reason, created_by, timestamps)
2. Add migration: `custom_cop_definitions` table (cop_name, description, pattern_type, pattern, file_glob, target_chef_version_min, removed_in, classification, enabled, timestamps)
3. Add datastore CRUD methods for both tables
4. Add classification resolution logic:
   - Priority: operator override → RemovedIn auto-seed → curated defaults → unclassified
   - `ResolveClassification(copName, targetVersion) → (classification, source)`
5. Add curated defaults as compiled Go data (known behaviour-change cops)
6. Update pass/fail determination to use classification (Blocker → fail, Unclassified → severity fallback)
7. Tests

Acceptance:
- Classification resolution returns correct priority order
- Pass/fail uses classification when available, falls back to severity rules
- CRUD operations work for both tables

## Chunk 4 — Cop aggregation API (backend)

Scope: `internal/webapi/`
Dependencies: Chunk 3

Steps:
1. `GET /api/v1/cookstyle/cops` — per-cop aggregation with classification, affected cookbooks count, auto-fix %, unblocks
2. `GET /api/v1/cookstyle/cops/:cop_name/cookbooks` — drill-down to affected cookbooks
3. `PUT /api/v1/cookstyle/cops/:cop_name/classification` — set/update classification
4. CRUD endpoints for `/api/v1/cookstyle/custom-cops`
5. Tests

Acceptance:
- Cop list returns aggregated data with resolved classifications
- Unblocks calculation correct (cookbooks that would pass without this cop)
- Classification changes trigger re-evaluation of affected results

## Chunk 5 — Cop Analysis frontend tab

Scope: `frontend/src/`
Dependencies: Chunk 4

Steps:
1. Replace the flat-list CookstyleViolationsTab with CopAnalysisTab
2. Summary cards: Blocker/Review/Noise/Unclassified cop counts + cookbook counts
3. Cop table with classification badges, RemovedIn, cookbooks affected, offences, auto-fix %, unblocks
4. Classification filter (default: show Blockers)
5. Inline reclassification (click badge → dropdown + reason)
6. Drill-down: click cop → show affected cookbooks
7. Tests

Acceptance:
- Cop table shows one row per cop with resolved classification
- Reclassification updates immediately
- Drill-down links to existing remediation pages
- Filter by classification level

## Chunk 6 — Custom cops management UI

Scope: `frontend/src/pages/Admin*`
Dependencies: Chunk 4

Steps:
1. Admin → CookStyle → Custom Cops page
2. CRUD form: cop name, description, pattern (regex/literal), file glob, target version, classification
3. Enable/disable toggle
4. Test pattern against sample code (optional nice-to-have)
5. Tests

Acceptance:
- Can create, edit, delete custom cops
- Custom cops appear in the Cop Analysis view like regular cops

## Chunk 7 — Updated cookbook/git repo detail views

Scope: `frontend/src/pages/CookbookRemediationPage.tsx`, `GitRepoRemediationPage.tsx`
Dependencies: Chunk 4

Steps:
1. Add classification badge next to each offense group's cop name
2. Show `RemovedIn` inline where available
3. Add summary stat: "N blockers / N review / N noise"
4. Filter offense groups by classification level
5. Tests

Acceptance:
- Classification badges visible on existing remediation pages
- RemovedIn displayed for cops that have it
- Filter hides noise offences by default

## Chunk 8 — Custom cop scanning in analysis pipeline

Scope: `internal/analysis/`
Dependencies: Chunk 3

Steps:
1. Load enabled custom cop definitions at scan time
2. For each cookbook source file matching file_glob, run pattern matching
3. Produce offences in standard format with `cop_name` = custom cop name
4. Store in same offences JSONB as cookstyle results
5. Tests

Acceptance:
- Custom cops produce offences during analysis
- Offences appear in remediation pages and cop analysis view

