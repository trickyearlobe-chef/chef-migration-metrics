# Remediation Guidance — Session Summary

**Date:** 2026  
**Component:** `internal/remediation/`, `internal/datastore/`, `internal/logging/`  
**Specification:** `analysis/Specification.md` § 4 (Remediation Guidance)  
**Todo file:** `todo/analysis.md` § Remediation Guidance

---

## Context

Remediation guidance is the feature that transforms the tool from a reporting dashboard into a migration management platform. It generates actionable data to help practitioners **fix** incompatible cookbooks, not just find them. This was the next major analysis feature after CookStyle and Test Kitchen integration were completed.

The feature has three sub-components:
1. **Auto-correct preview generation** — run `cookstyle --auto-correct` on a temp copy, produce unified diffs
2. **Migration documentation link enrichment** — map each CookStyle cop to its migration docs
3. **Cookbook complexity scoring** — weighted score + blast radius for prioritisation

---

## What Was Done

### 1. New Package: `internal/remediation/`

Created the entire `internal/remediation/` package with three production files and three test files:

#### `copmapping.go` — Cop-to-Documentation Mapping
- `CopMapping` struct with `cop_name`, `description`, `migration_url`, `introduced_in`, `removed_in`, `replacement_pattern`
- `EnrichedOffense` type for attaching remediation docs to CookStyle offenses
- `OffenseLocation` type mirroring CookStyle JSON location fields
- **60+ embedded cop mappings** covering the most common `ChefDeprecations/*` and `ChefCorrectness/*` cops
- `LookupCop()` — O(1) index lookup by cop name (built at `init()` time)
- `AllCopMappings()` — returns a defensive copy of the full mapping table
- `CopMappingCount()` — returns the number of entries

#### `autocorrect.go` — Auto-Correct Preview Generator
- `AutocorrectExecutor` interface for testability (same pattern as `CookstyleExecutor`)
- `AutocorrectGenerator` with `GeneratePreviews()` batch method
- `generateOne()` implements the full pipeline per cookbook:
  1. Skip if zero offenses
  2. Check for existing cached preview (immutability optimisation)
  3. Resolve cookbook directory
  4. `copyDirectory()` — creates temp copy via `filepath.WalkDir`
  5. Read original file contents
  6. Run `cookstyle --auto-correct --format json` via executor
  7. Parse JSON output for remaining offense count
  8. Read modified files, generate unified diffs
  9. Compute statistics: total, correctable, remaining, files modified
  10. Persist to `autocorrect_previews` table
  11. Clean up temp dir (deferred)
- `buildAutocorrectArgs()` — constructs CLI args with `--auto-correct --format json` and optional `--only` for target versions
- Pure diff functions:
  - `computeEdits()` — LCS-based O(mn) edit script computation
  - `groupHunks()` — converts flat edit sequence to unified diff hunks with configurable context lines
  - `generateUnifiedDiffs()` — compares file maps, produces combined diff output
  - `splitLines()`, `stripNewline()`, `sortStringSlice()` helper functions
- `copyDirectory()`, `copyFile()`, `readAllFiles()` filesystem helpers
- `ResetPreviews()`, `ResetAllPreviews()` for manual rescan

#### `complexity.go` — Cookbook Complexity Scoring
- Named weight constants matching the specification exactly:
  - `WeightErrorFatal = 5`, `WeightDeprecation = 3`, `WeightCorrectness = 3`
  - `WeightNonAutoCorrectable = 4`, `WeightModernize = 1`
  - `WeightTKConvergeFail = 20`, `WeightTKTestFail = 10`
- `ScoreToLabel()` — maps numeric score to `none`/`low`/`medium`/`high`/`critical`
- `ComputeComplexityScore()` — **pure function** with no side effects, safe for testing
- `ComplexityScorer` with `ScoreCookbooks()` batch method:
  1. Pre-loads blast radius data for the organisation
  2. For each cookbook × target version:
     - Loads CookStyle result, classifies offenses from JSONB
     - Loads auto-correct preview for manual fix counts
     - Loads Test Kitchen result
     - Looks up blast radius
     - Computes weighted score and label
     - Persists to `cookbook_complexity` table
- `classifyOffenses()` — parses JSONB offenses from `cookstyle_results`, counts by namespace
- `loadBlastRadii()` — combines `cookbook_usage_detail` (nodes, policies) with `CountRolesPerCookbook` (roles)
- `countJSONBStringArray()` — helper for parsing JSONB arrays
- Namespace helpers: `isDeprecation()`, `isCorrectness()`, `isModernize()`, `isErrorOrFatal()`
- `ResetScores()`, `ResetAllScores()` for manual recomputation

### 2. New Datastore Files

#### `internal/datastore/autocorrect_previews.go`
- `AutocorrectPreview` struct and `UpsertAutocorrectPreviewParams`
- Full CRUD: `GetAutocorrectPreview` (by cookstyle_result_id), `GetAutocorrectPreviewByID`, `ListAutocorrectPreviewsForCookbook`, `ListAutocorrectPreviewsForOrganisation`, `UpsertAutocorrectPreview`, `UpsertAutocorrectPreviewTx`, `DeleteAutocorrectPreviewsForCookbook`, `DeleteAutocorrectPreviewsForOrganisation`, `DeleteAutocorrectPreview`, `DeleteAutocorrectPreviewForCookstyleResult`
- Shared column list and row scanning helpers

#### `internal/datastore/cookbook_complexity.go`
- `CookbookComplexity` struct and `UpsertCookbookComplexityParams`
- Full CRUD: `GetCookbookComplexity` (by cookbook+target), `GetCookbookComplexityByID`, `ListCookbookComplexitiesForCookbook`, `ListCookbookComplexitiesForOrganisation`, `ListCookbookComplexitiesByLabel`, `ListCookbookComplexitiesByTargetVersion`, `UpsertCookbookComplexity`, `UpsertCookbookComplexityTx`, `DeleteCookbookComplexitiesForCookbook`, `DeleteCookbookComplexitiesForOrganisation`, `DeleteCookbookComplexity`

### 3. Logging Scope

Added `ScopeRemediation Scope = "remediation"` to `internal/logging/logging.go` with entry in `validScopes` map.

### 4. Updated Documentation
- `todo/analysis.md` — marked 19 remediation sub-items as done (1 remaining)
- `ToDo.md` — updated analysis row from 32/61 (52%) to 51/61 (83%)
- `Structure.md` — added all new files with descriptions

---

## Final State

### Test Counts
- **`internal/remediation/`**: ~93 tests, all passing
  - `autocorrect_test.go`: ~50 tests (args, split/strip, sort, edits, diffs, hunks, filesystem copy, JSON parsing)
  - `copmapping_test.go`: ~20 tests (lookup, AllCopMappings, count, pointer stability, namespace validation, URLs)
  - `complexity_test.go`: ~23 tests (ScoreToLabel, ComputeComplexityScore all factors, combined, labels, blast radius, namespace helpers, JSONB parsing)
- **Full project**: All packages pass `go test ./...`
- **Build**: `go build ./...` succeeds cleanly

### Coverage
- Pure scoring logic (`ComputeComplexityScore`, `ScoreToLabel`) fully covered including boundary values
- Cop mapping index fully covered (known, unknown, empty, partial, case-sensitive, pointer stability)
- Diff algorithm covered (identical, insert, delete, replace, empty→non-empty, non-empty→empty)
- Filesystem helpers covered (copy, read, nonexistent paths)
- Datastore files compile and integrate correctly but are not unit-tested in isolation (they require a PostgreSQL database; the existing `datastore_test.go` pattern uses live DB)

---

## Known Gaps

1. **Persist enriched offenses in CookStyle results** — The `EnrichedOffense` type and `LookupCop()` function are ready, but the CookStyle scanner in `internal/analysis/cookstyle.go` has not been modified to call `LookupCop()` and persist enriched offenses. This requires modifying `scanOne()` to enrich each offense before marshalling to JSON, and potentially updating the JSONB schema in `cookstyle_results.offences`. This is the **one remaining remediation todo item**.

2. **Integration wiring** — The `AutocorrectGenerator` and `ComplexityScorer` are not yet wired into the main analysis pipeline (the scheduler in `internal/collector/scheduler.go` or a new orchestrator). They need to be called after CookStyle scan and Test Kitchen run cycles complete.

3. **Datastore integration tests** — The new datastore files (`autocorrect_previews.go`, `cookbook_complexity.go`) follow the exact same patterns as the existing tested files but don't have their own unit tests since the project's datastore tests require a live PostgreSQL instance.

---

## Files Modified

### Production Code (new)
- `internal/remediation/autocorrect.go`
- `internal/remediation/copmapping.go`
- `internal/remediation/complexity.go`
- `internal/datastore/autocorrect_previews.go`
- `internal/datastore/cookbook_complexity.go`

### Production Code (modified)
- `internal/logging/logging.go` — added `ScopeRemediation`

### Tests (new)
- `internal/remediation/autocorrect_test.go`
- `internal/remediation/copmapping_test.go`
- `internal/remediation/complexity_test.go`

### Documentation (modified)
- `.claude/specifications/todo/analysis.md` — marked remediation items done
- `.claude/specifications/ToDo.md` — updated analysis count to 51/61 (83%)
- `.claude/Structure.md` — added new file descriptions

---

## Recommended Next Steps

### 1. Persist enriched offenses in CookStyle results (small)
- **Spec:** `analysis/Specification.md` § 4.2
- **Todo:** `todo/analysis.md` — the one remaining remediation item
- **Scope:** Modify `internal/analysis/cookstyle.go` `scanOne()` to call `remediation.LookupCop()` on each offense and include the `remediation` field in the JSONB. May require a new migration if the JSONB shape needs a schema change, but since it's JSONB it's flexible.
- **Tokens:** ~5k input, ~3k output

### 2. Node readiness evaluation (medium)
- **Spec:** `analysis/Specification.md` § 5
- **Todo:** `todo/analysis.md` § Node Upgrade Readiness (8 items, all unchecked)
- **Scope:** Per-node per-target-version readiness check: cookbook compatibility + disk space. Parallel evaluation via bounded worker pool. Persist to `node_readiness` table (already exists in schema).
- **Tokens:** ~20k input, ~10k output
- **Note:** This completes the entire analysis pipeline.

### 3. Web API (large)
- **Spec:** Read `web-api/Specification.md` (not yet created per the specification index)
- **Todo:** `todo/visualisation.md`
- **Scope:** `internal/webapi/` — router, middleware, REST endpoints for all dashboard data including remediation data. Real compatibility + remediation data is now available to serve.
- **Tokens:** ~40k input, ~30k output

### 4. Wire remediation into the analysis pipeline (small)
- **Scope:** After CookStyle scan completes, call `AutocorrectGenerator.GeneratePreviews()`. After both CookStyle and Test Kitchen complete, call `ComplexityScorer.ScoreCookbooks()`. This may be done as part of a scheduler/orchestrator enhancement.
- **Tokens:** ~10k input, ~5k output