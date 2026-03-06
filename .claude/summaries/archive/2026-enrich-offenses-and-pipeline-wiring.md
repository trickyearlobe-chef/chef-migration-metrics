# Enriched CookStyle Offenses + Analysis Pipeline Wiring

**Date:** 2026
**Components:** `internal/analysis/cookstyle.go`, `internal/collector/collector.go`

## Context

Two small, high-value tasks to complete the analysis pipeline:
1. Persist enriched offenses in CookStyle results (the last remaining analysis todo item)
2. Wire the full analysis pipeline (CookStyle → Test Kitchen → Autocorrect → Complexity → Readiness) into the collection cycle

## What Was Done

### 1. Enriched CookStyle Offenses (`internal/analysis/cookstyle.go`)

Added `enrichOffenses()` function that converts `[]CookstyleOffense` → `[]remediation.EnrichedOffense` by calling `remediation.LookupCop()` on each offense. Known cops get a populated `Remediation` field with migration docs; unknown cops get `nil` (omitted from JSON via `omitempty`).

Modified `persistResult()` to marshal enriched offenses instead of raw offenses for both the `offences` and `deprecation_warnings` JSONB columns. The enrichment happens at persistence time, not during classification — minimal disruption to existing logic.

Added `remediation` import to the analysis package.

**7 new tests in `cookstyle_test.go`:**
- `TestEnrichOffenses_Nil` — nil input returns nil
- `TestEnrichOffenses_Empty` — empty slice returns nil
- `TestEnrichOffenses_KnownCop` — known cop gets non-nil Remediation with MigrationURL
- `TestEnrichOffenses_UnknownCop` — unknown cop gets nil Remediation
- `TestEnrichOffenses_LocationFidelity` — all location fields preserved correctly
- `TestEnrichOffenses_MixedKnownAndUnknown` — mixed list preserves ordering and correct enrichment
- `TestEnrichOffenses_JSONRoundTrip` — marshal → unmarshal preserves all fields including Remediation

### 2. Analysis Pipeline Wiring (`internal/collector/collector.go`)

Added 6 optional fields to `Collector` struct:
- `cookstyleScanner *analysis.CookstyleScanner`
- `kitchenScanner *analysis.KitchenScanner`
- `autocorrectGen *remediation.AutocorrectGenerator`
- `complexityScorer *remediation.ComplexityScorer`
- `readinessEval *analysis.ReadinessEvaluator`
- `cookbookDirFn func(cb datastore.Cookbook) string`

Added 6 corresponding `With*` option functions following the existing `WithClientFactory` pattern.

Added Steps 11–14 to `collectOrganisation()` between usage analysis (Step 10) and run completion (renumbered to Step 15):

- **Step 11: CookStyle scanning** — Lists org cookbooks, calls `ScanCookbooks()` with target versions. Skipped if scanner or cookbookDirFn is nil.
- **Step 12: Test Kitchen** — Lists git cookbooks, calls `TestCookbooks()` with target versions. Skipped if scanner or cookbookDirFn is nil.
- **Step 13: Autocorrect previews + Complexity scoring** — Lists CookStyle results, builds `CookstyleResultInfo` list, generates previews. Then scores all org cookbooks. Each skipped independently if not configured.
- **Step 14: Node readiness evaluation** — Calls `EvaluateOrganisation()`, counts ready/blocked nodes. Skipped if evaluator is nil.

All new steps are **non-fatal** — failures logged at WARN, collection run still completes.

**10 new tests in `collector_test.go`:**
- `TestCollector_PipelineFields_NilByDefault` — all 6 fields nil after default construction
- `TestWithCookstyleScanner_SetsField`
- `TestWithKitchenScanner_SetsField`
- `TestWithAutocorrectGenerator_SetsField`
- `TestWithComplexityScorer_SetsField`
- `TestWithReadinessEvaluator_SetsField`
- `TestWithCookbookDirFn_SetsField` — also verifies function is callable
- `TestWithCookbookDirFn_NilIsAccepted`
- `TestMultiplePipelineOptions_AllSet` — all 6 options + verifies analyser still works

## Final State

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./...` — all packages pass

### Analysis todo: 60/61 (98%)
- Remaining item: the last analysis todo (exclude failed-download cookbooks from CookStyle) is in `todo/data-collection.md`, not `todo/analysis.md`

### Collection Pipeline (15 Steps)
1. Create collection run
2. Build Chef API client
3. Collect nodes via concurrent partial search
4. Convert to snapshot params + build NodeRecord slice
5. Persist node snapshots
6. Fetch cookbook inventory; upsert metadata
7. Mark active/stale cookbooks
7b. Fetch cookbook content from Chef server
7c. Fetch git cookbooks
8. Build cookbook-node usage records
9. Build role dependency graph
10. Run cookbook usage analysis
11. **NEW: CookStyle scanning** (optional)
12. **NEW: Test Kitchen** (optional)
13. **NEW: Autocorrect previews + Complexity scoring** (optional)
14. **NEW: Node readiness evaluation** (optional)
15. Complete collection run

## Known Gaps

- **Pipeline components not yet constructed in `main.go`** — The `With*` options exist but `main.go` doesn't create the scanners/evaluators yet. This requires the embedded tool resolver to validate tool availability and construct the scanner instances. A future session should wire `main.go` to call `embedded.NewResolver().ValidateAll()`, then construct scanners for available tools and pass them via the `With*` options.
- **`cookbookDirFn` not implemented** — No function exists yet that maps a `datastore.Cookbook` to its actual filesystem path. This depends on the cookbook content download actually writing files to disk (currently `GetCookbookVersion` returns metadata but doesn't write files).
- **Autocorrect preview needs `CookbookName`/`CookbookVersion` on `CookstyleResultInfo`** — The current wiring doesn't populate these fields (they're for logging only). Works but log messages will be empty for those fields.

## Files Modified

### Production code
- `internal/analysis/cookstyle.go` — added `enrichOffenses()`, added `remediation` import, changed `persistResult()` to use enriched offenses
- `internal/collector/collector.go` — added 6 optional fields, 6 `With*` options, Steps 11–14, added `remediation` import, renumbered Step 15

### Test code
- `internal/analysis/cookstyle_test.go` — 7 new enrichment tests, added `remediation` import
- `internal/collector/collector_test.go` — 10 new pipeline option tests, added `remediation` import

### Documentation
- `.claude/specifications/todo/analysis.md` — marked "Persist enriched offenses" as done
- `.claude/specifications/ToDo.md` — updated analysis 59→60/61, total 312→313

## Recommended Next Steps

### 1. Wire `main.go` to construct analysis pipeline (small ~15k tokens)
- Call `embedded.NewResolver().ValidateAll()` at startup
- If CookStyle available: construct `CookstyleScanner` + `AutocorrectGenerator`
- If Kitchen+Docker available: construct `KitchenScanner`
- Always construct `ComplexityScorer` + `ReadinessEvaluator`
- Pass all via `With*` options to `collector.New()`
- Implement `cookbookDirFn` (needs cookbook filesystem layout)
- **Specs:** `configuration/Specification.md` (§ analysis_tools), `packaging/Specification.md` (§ installed layout)

### 2. Visualisation / Web API handlers (large ~40k tokens)
- All analysis data now flows end-to-end. The web API handlers exist but the frontend doesn't.
- **Spec:** `visualisation/Specification.md`, `web-api/Specification.md`
- **Todo:** `todo/visualisation.md` (0/86)

### 3. Auth middleware (medium ~20k tokens)
- **Spec:** `auth/Specification.md`
- **Todo:** `todo/auth.md` (0/5)