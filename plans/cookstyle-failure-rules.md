# Plan — CookStyle Configurable Failure Rules

Spec: `specifications/cookstyle-failure-rules.md`
Branch: `feature/cookstyle-failure-rules` (create from `specification/cookstyle-failure-rules`)

## Chunk 1 — Core evaluation engine + config (backend, no UI)

Scope: `internal/analysis/`, `internal/config/`
Dependencies: none

Steps:
1. Add `CookstyleFailurePreset` and `CookstyleFailureRules` to `AnalysisToolsConfig`
2. Add defaults (`setDefaults`): empty rules + preset "default"
3. Add validation: preset values, severity strings, key non-empty
4. Create `internal/analysis/failure_rules.go`:
   - `CookstyleFailureRules` type (map + resolved sorted prefixes)
   - `DefaultFailureRules()`, `StrictFailureRules()`, `RelaxedFailureRules()`
   - `EffectiveRules(preset string, explicit map) CookstyleFailureRules`
   - `EvaluatePassFail(offenses []CookstyleOffense, rules CookstyleFailureRules) bool`
   - Longest-prefix-match logic
5. Write tests (`failure_rules_test.go`): presets produce expected results, default matches current `isErrorOrFatal` behaviour, longest-prefix, catch-all, empty rules
6. Wire into scanner: add `WithCookstyleFailureRulesFn(func() CookstyleFailureRules)` option
7. Replace `isErrorOrFatal` calls at `:560` and `:701` with `EvaluatePassFail`
8. Update `cookstyle_test.go` — existing tests pass with default rules

Acceptance:
- `go test ./internal/analysis/... ./internal/config/...` passes
- Default rules produce identical verdicts to old hardcoded logic (verified by test)
- Config loads and validates with new fields

## Chunk 2 — Re-score applier (backend)

Scope: `internal/webapi/`, `internal/datastore/`
Dependencies: Chunk 1

Steps:
1. Add datastore method: `ListCookstyleResultsForRescore()` — returns id + offences + error_message + current passed for both tables
2. Add datastore method: `BatchUpdateCookstylePassed(updates)` for both tables
3. Create `internal/webapi/cookstyle_rescore.go`:
   - `rescoreCookstyleResults(db, rules, logger) (total, changed int)`
   - Deserialise offences, evaluate, collect updates, batch-write
4. Wire as config applier for `analysis_tools` section (subsystem granularity)
5. After re-score, call `RecomputeGitRepoStatus` for affected repos
6. Tests: mock DB, verify re-score changes verdicts when rules differ

Acceptance:
- Changing config triggers re-score
- Verdicts update in DB without full rescan
- Response includes `verdicts_changed` count

## Chunk 3 — Admin API endpoint

Scope: `internal/webapi/`
Dependencies: Chunk 2

Steps:
1. Extend existing `PUT /admin/config/analysis_tools` to accept the new fields
2. Return `verdicts_changed` in response body after re-score
3. `GET /admin/config/analysis_tools` returns current preset + effective rules
4. Tests: round-trip preset → rules → re-score → response

Acceptance:
- API accepts preset or explicit rules
- Response shows effective rules and verdicts changed
- Invalid preset/severities return 422

## Chunk 4 — Frontend checkbox grid

Scope: `frontend/src/`
Dependencies: Chunk 3

Steps:
1. Add types: `CookstyleFailureRules`, preset names
2. Add/extend API client for analysis_tools config (GET/PUT with new fields)
3. Create `CookstyleFailureRulesGrid` component:
   - Preset dropdown (Strict/Default/Relaxed/Custom)
   - 5×5 checkbox grid (namespaces × severities)
   - Preset selection populates grid; manual change → "Custom"
4. Integrate into `AdminAnalysisToolsPage`
5. Toast on save: "Failure rules updated. N cookbook verdicts changed."
6. Tests: preset populates checkboxes, manual change → Custom, save payload correct

Acceptance:
- Grid renders with current rules from API
- Preset changes reflect in grid
- Save sends correct payload, shows toast with verdict count
