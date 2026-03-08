# Analysis — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Cookbook Usage Analysis

- [x] Determine which cookbooks are in active use from collected `automatic.cookbooks` attribute — `internal/analysis/usage.go` Phase 3 compares aggregated cookbook versions against the full server inventory; `buildActiveSet()` identifies versions with at least one node
- [x] Determine which versions of each cookbook are in use — Phase 2 `aggregateTuples()` aggregates by `cookbookVersionKey{Name, Version}`; each unique version gets its own `aggregatedUsage` entry
- [x] Determine which roles reference each cookbook — Phase 1 `extractNodeTuples()` carries each node's `Roles` slice; Phase 2 accumulates into `aggregatedUsage.Roles` set per cookbook version
- [x] Determine which Policyfile policy names and policy groups reference each cookbook — Phase 1 carries `PolicyName`/`PolicyGroup`; Phase 2 accumulates into `PolicyNames`/`PolicyGroups` sets; tested in `TestEndToEnd_FullAnalysisPipeline` with mixed classic and Policyfile nodes
- [x] Determine which nodes are running each cookbook and version — Phase 2 `aggregatedUsage.NodeNames` set tracks distinct node names per cookbook version
- [x] Count nodes running each cookbook and version — `aggregatedUsage.NodeCount` incremented per distinct node; persisted as `node_count` in `cookbook_usage_detail` table
- [x] Count platform versions and platform families running each cookbook and version — `aggregatedUsage.PlatformCounts` (keyed by `platform/platform_version`) and `PlatformFamilyCounts` (keyed by `platform_family`); persisted as JSONB in `cookbook_usage_detail`
- [x] Persist usage analysis results to datastore (including policy name and policy group references) — `RunUsageAnalysis()` writes header to `cookbook_usage_analysis` and detail rows to `cookbook_usage_detail` in a single transaction; migration `0003_cookbook_usage_analysis.up.sql` creates both tables

## Cookbook Compatibility Testing

- [x] Implement embedded tool resolution — look for `cookstyle` and `kitchen` in `analysis_tools.embedded_bin_dir` first, fall back to `PATH` — `internal/embedded/embedded.go` Resolver.ResolvePath() checks embedded dir then exec.LookPath; CommandExecutor interface for testability; 16 tests in `embedded_test.go`
- [x] Implement startup validation for `kitchen` (check embedded dir, then PATH; disable Test Kitchen if not found) — Resolver.ValidateKitchen() runs `kitchen version`, returns ToolInfo with Available/Version/Error
- [x] Implement startup validation for `cookstyle` (check embedded dir, then PATH; disable CookStyle if not found) — Resolver.ValidateCookstyle() runs `cookstyle --version`, returns ToolInfo
- [x] Implement startup validation for `docker` (`docker info`; warn and disable Test Kitchen if not found) — Resolver.ValidateDocker() runs `docker info --format json`, parses ServerVersion from JSON output
- [x] Implement Test Kitchen integration for cookbooks sourced from git — `internal/analysis/kitchen.go` KitchenScanner.TestCookbooks() batch runner with testOne() per cookbook×target version; KitchenExecutor interface for testability; driver/platform override support via `config.TestKitchenConfig`; .kitchen.local.yml overlay generation; per-phase execution (converge→verify→destroy); 75+ tests in `kitchen_test.go`
- [x] Support testing against multiple configured target Chef Client versions — TestCookbooks() fans out work items as cookbook×targetVersion cartesian product; provisioner overlay auto-detects dokken vs standard driver and sets chef_version or product_version accordingly
- [x] Test only the HEAD commit of the default branch (`main` or `master`) — testOne() uses cb.HeadCommitSHA (recorded by git fetching in data collection) as the commit_sha for each test result
- [x] Skip test run if HEAD commit SHA is unchanged since last test run for a given cookbook + target Chef Client version — testOne() Step 1 calls db.GetLatestTestKitchenResult() and compares commit_sha; skips with Skipped=true and SkipReason if unchanged
- [x] Record HEAD commit SHA alongside each test result — CommitSHA field on KitchenRunResult and UpsertTestKitchenResultParams; persisted to commit_sha column in test_kitchen_results table
- [x] Record convergence pass/fail per cookbook + target Chef Client version + HEAD commit SHA — converge_passed column populated from phase exit code; unique constraint (cookbook_id, target_chef_version, commit_sha)
- [x] Record test suite pass/fail per cookbook + target Chef Client version + HEAD commit SHA — tests_passed column; verify phase only runs if converge passed; compatible = converge_passed AND tests_passed
- [x] Dispatch Test Kitchen runs in parallel using goroutines (one per cookbook + target Chef Client version), bounded by the `concurrency.test_kitchen_run` worker pool setting — TestCookbooks() uses semaphore channel sized by s.concurrency; sync.WaitGroup coordinates goroutines
- [x] Capture stdout/stderr from each Test Kitchen process and return alongside pass/fail result — per-phase output captured in ConvergeOutput/VerifyOutput/DestroyOutput fields; combined into ProcessStdout for backward compatibility; migration 0005 adds per-phase columns
- [x] Honour `analysis_tools.test_kitchen_timeout_minutes` for Test Kitchen process timeout — converge and verify phases wrapped in context.WithTimeout(ctx, s.timeout); TimedOut flag set on deadline exceeded; destroy uses independent 5-minute timeout via fresh context
- [x] Implement CookStyle linting for cookbooks sourced from Chef server (no test suite) — `internal/analysis/cookstyle.go` CookstyleScanner.ScanCookbooks() batch runner, scanOne() per cookbook×target version; CookstyleExecutor interface for testability; 34 tests in `cookstyle_test.go`
- [x] Implement CookStyle version profiles — enable only cops relevant to the specific target Chef Client version being tested — when target version is specified, `--only ChefDeprecations,ChefCorrectness` restricts to migration-critical namespaces; CookStyle handles version-relevance internally within those namespaces; no static cop map needed since CookStyle JSON output already carries cop_name, severity, message, corrected flag, and location per offense
- [x] Maintain CookStyle cop-to-version mapping as embedded application data — NOT NEEDED: CookStyle JSON output already provides all offense metadata dynamically; no static mapping to maintain; cop-to-documentation mapping (separate concern) implemented in `internal/remediation/copmapping.go` with 60+ embedded entries
- [x] Fall back to full `ChefDeprecations` and `ChefCorrectness` namespaces when target version cannot be mapped — buildCookstyleArgs() uses `--only ChefDeprecations,ChefCorrectness` for any target version; without a target version the full default rule set runs
- [x] Run CookStyle scans in parallel using goroutines, bounded by the `concurrency.cookstyle_scan` worker pool setting — ScanCookbooks() fans out work items via semaphore channel bounded by s.concurrency
- [x] Capture stdout/stderr from each CookStyle process and return alongside results — RawStdout/RawStderr fields on CookstyleScanResult, persisted to process_stdout/process_stderr columns
- [x] Honour `analysis_tools.cookstyle_timeout_minutes` for CookStyle process timeout — scanOne() wraps execution in context.WithTimeout(ctx, s.timeout); timeout error detected via context.DeadlineExceeded
- [x] Skip CookStyle scan for cookbook versions already scanned in the datastore (immutability optimisation) — scanOne() calls db.GetCookstyleResult() first; skips with Skipped=true if existing result found
- [x] Record CookStyle results and deprecation warnings keyed by organisation + cookbook name + version + target Chef version — persisted via db.UpsertCookstyleResult() to cookstyle_results table with unique constraint on (cookbook_id, target_chef_version)
- [x] Implement manual rescan option for CookStyle consistent with cookbook download rescan — ResetResults(cookbookID) and ResetAllResults(organisationID) delete existing results so next scan cycle re-evaluates
- [x] Persist all test results to datastore — `internal/datastore/cookstyle_results.go` provides Get, List, Upsert (INSERT ON CONFLICT UPDATE), Delete operations for cookstyle_results table

## Remediation Guidance

- [x] Implement auto-correct preview generation:
  - [x] Create temporary copy of cookbook directory — `internal/remediation/autocorrect.go` copyDirectory() creates a temp copy via filepath.WalkDir, copyFile helper
  - [x] Run `cookstyle --auto-correct --format json` on the copy — AutocorrectGenerator.generateOne() runs via AutocorrectExecutor interface; buildAutocorrectArgs() constructs CLI args with --auto-correct --format json and optional --only for target versions
  - [x] Generate unified diff between original and auto-corrected files — generateUnifiedDiffs() compares file maps, computeEdits() uses LCS-based O(mn) diff, groupHunks() produces unified diff hunks with 3-line context
  - [x] Compute statistics (total offenses, correctable, remaining, files modified) — generateOne() parses post-autocorrect JSON for remaining offenses, computes correctable = total - remaining, counts files from diff
  - [x] Persist auto-correct preview to `autocorrect_previews` table — persistPreview() calls db.UpsertAutocorrectPreview(); full CRUD in `internal/datastore/autocorrect_previews.go`
  - [x] Clean up temporary copy — defer os.RemoveAll(tmpDir) in generateOne()
  - [x] Only generate for cookbooks with CookStyle offenses — generateOne() skips with SkipReason="zero offenses" when OffenseCount == 0
  - [x] Cache results for immutable Chef server cookbook versions — generateOne() checks db.GetAutocorrectPreview() and skips if existing preview found
- [x] Implement migration documentation link enrichment:
  - [x] Build and maintain cop-to-documentation mapping table (`cop_name → { description, migration_url, introduced_in, removed_in, replacement_pattern }`) — `internal/remediation/copmapping.go` embeddedCopMappings slice with 60+ ChefDeprecations/* and ChefCorrectness/* cops
  - [x] Ship mapping as embedded data in the application binary — compiled Go data in embeddedCopMappings; copMappingIndex built at init() time; LookupCop(), AllCopMappings(), CopMappingCount() exported
  - [x] Enrich every `ChefDeprecations/*` and `ChefCorrectness/*` offense with its mapping entry — EnrichedOffense type with optional *CopMapping Remediation field; LookupCop() provides per-offense enrichment
  - [x] Persist enriched offenses in CookStyle results — `enrichOffenses()` in `internal/analysis/cookstyle.go` converts `[]CookstyleOffense` to `[]remediation.EnrichedOffense` via `remediation.LookupCop()` before marshalling to JSONB in `persistResult()`; both `offences` and `deprecation_warnings` columns now contain enriched data with optional `remediation` field; 7 tests in `cookstyle_test.go`
- [x] Implement cookbook complexity scoring:
  - [x] Compute weighted score per cookbook per target Chef Client version (error: 5, deprecation: 3, correctness: 3, non-auto-correctable: 4, modernize: 1, TK converge fail: 20, TK test fail: 10) — `internal/remediation/complexity.go` ComputeComplexityScore() pure function with named weight constants
  - [x] Classify score into labels: `none` (0), `low` (1-10), `medium` (11-30), `high` (31-60), `critical` (61+) — ScoreToLabel() with LabelNone/Low/Medium/High/Critical constants
  - [x] Compute blast radius: affected node count, role count (using dependency graph), policy count — ComplexityScorer.loadBlastRadii() combines cookbook_usage_detail (nodes, policies) with CountRolesPerCookbook (roles)
  - [x] Persist complexity records to `cookbook_complexity` table — persistComplexity() calls db.UpsertCookbookComplexity(); full CRUD in `internal/datastore/cookbook_complexity.go`
  - [x] Recompute after every CookStyle scan and Test Kitchen run cycle — ComplexityScorer.ScoreCookbooks() batch method reads latest CookStyle, autocorrect preview, and TK results per cookbook×target version

## Node Upgrade Readiness

- [x] Implement readiness calculation per node per target Chef Client version — `internal/analysis/readiness.go` ReadinessEvaluator.evaluateOne() computes per-node per-target readiness combining cookbook compatibility and disk space checks; EvaluateOrganisation() batch method loads snapshots, pre-loads cookbook ID map, fans out work items
- [x] Evaluate nodes in parallel using goroutines, bounded by the `concurrency.readiness_evaluation` worker pool setting — EvaluateOrganisation() uses semaphore channel sized by e.concurrency; sync.WaitGroup coordinates goroutines; context cancellation breaks the dispatch loop
- [x] Check all cookbooks in expanded run-list are compatible with target version — evaluateCookbooks() parses automatic.cookbooks JSONB, calls checkCookbookCompatibility() per cookbook which checks TK results first (converge_passed AND tests_passed), then CookStyle (passed=true → compatible_cookstyle_only), then untested; TK takes precedence over CookStyle
- [x] Check available disk space meets threshold for Habitat bundle installation — evaluateDiskSpace() parses automatic.filesystem JSONB, determines install path (/hab on Linux, C:\hab on Windows), finds longest-prefix mount match via findBestMountLinux/findBestMountWindows, extracts kb_available (handles both string and numeric values via toInt64), converts to MB, compares against configurable min_free_disk_mb threshold
- [x] Record blocking reasons per node (incompatible cookbooks with complexity scores, insufficient disk space) — BlockingCookbook struct with Name, Version, Reason (incompatible/untested), Source (test_kitchen/cookstyle/none), ComplexityScore, ComplexityLabel; persisted as JSONB array in blocking_cookbooks column; disk space insufficiency tracked via sufficient_disk_space boolean (nil=unknown)
- [x] Handle stale nodes: treat disk space data as unknown, set `stale_data` flag on readiness result — evaluateOne() checks snapshot.IsStale; stale nodes get SufficientDiskSpace=nil and AvailableDiskMB=nil; StaleData flag propagated to persistence; unknown disk space blocks readiness (errs on the side of caution)
- [x] Include complexity score and label in each blocking cookbook entry — evaluateCookbooks() enriches each BlockingCookbook by calling db.GetCookbookComplexity() with the cookbook ID and target version; ComplexityScore and ComplexityLabel populated from cookbook_complexity table
- [x] Persist readiness results to datastore — persistResult() marshals BlockingCookbooks to JSON, calls db.UpsertNodeReadiness() with full UpsertNodeReadinessParams; `internal/datastore/node_readiness.go` provides full CRUD: Get (by snapshot+target, by ID), List (by snapshot, by org, by org+target, ready, blocked, stale), Count, Upsert (INSERT ON CONFLICT UPDATE on node_snapshot_id+target_chef_version), Delete (by snapshot, by org, by org+target, by ID); 82 tests in readiness_test.go