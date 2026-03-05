# Analysis — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Cookbook Usage Analysis

- [ ] Determine which cookbooks are in active use from collected `automatic.cookbooks` attribute
- [ ] Determine which versions of each cookbook are in use
- [ ] Determine which roles reference each cookbook
- [ ] Determine which Policyfile policy names and policy groups reference each cookbook
- [ ] Determine which nodes are running each cookbook and version
- [ ] Count nodes running each cookbook and version
- [ ] Count platform versions and platform families running each cookbook and version
- [ ] Persist usage analysis results to datastore (including policy name and policy group references)

## Cookbook Compatibility Testing

- [ ] Implement embedded tool resolution — look for `cookstyle` and `kitchen` in `analysis_tools.embedded_bin_dir` first, fall back to `PATH`
- [ ] Implement startup validation for `kitchen` (check embedded dir, then PATH; disable Test Kitchen if not found)
- [ ] Implement startup validation for `cookstyle` (check embedded dir, then PATH; disable CookStyle if not found)
- [ ] Implement startup validation for `docker` (`docker info`; warn and disable Test Kitchen if not found)
- [ ] Implement Test Kitchen integration for cookbooks sourced from git
- [ ] Support testing against multiple configured target Chef Client versions
- [ ] Test only the HEAD commit of the default branch (`main` or `master`)
- [ ] Skip test run if HEAD commit SHA is unchanged since last test run for a given cookbook + target Chef Client version
- [ ] Record HEAD commit SHA alongside each test result
- [ ] Record convergence pass/fail per cookbook + target Chef Client version + HEAD commit SHA
- [ ] Record test suite pass/fail per cookbook + target Chef Client version + HEAD commit SHA
- [ ] Dispatch Test Kitchen runs in parallel using goroutines (one per cookbook + target Chef Client version), bounded by the `concurrency.test_kitchen_run` worker pool setting
- [ ] Capture stdout/stderr from each Test Kitchen process and return alongside pass/fail result
- [ ] Honour `analysis_tools.test_kitchen_timeout_minutes` for Test Kitchen process timeout
- [ ] Implement CookStyle linting for cookbooks sourced from Chef server (no test suite)
- [ ] Implement CookStyle version profiles — enable only cops relevant to the specific target Chef Client version being tested
- [ ] Maintain CookStyle cop-to-version mapping as embedded application data
- [ ] Fall back to full `ChefDeprecations` and `ChefCorrectness` namespaces when target version cannot be mapped
- [ ] Run CookStyle scans in parallel using goroutines, bounded by the `concurrency.cookstyle_scan` worker pool setting
- [ ] Capture stdout/stderr from each CookStyle process and return alongside results
- [ ] Honour `analysis_tools.cookstyle_timeout_minutes` for CookStyle process timeout
- [ ] Skip CookStyle scan for cookbook versions already scanned in the datastore (immutability optimisation)
- [ ] Record CookStyle results and deprecation warnings keyed by organisation + cookbook name + version + target Chef version
- [ ] Implement manual rescan option for CookStyle consistent with cookbook download rescan
- [ ] Persist all test results to datastore

## Remediation Guidance

- [ ] Implement auto-correct preview generation:
  - [ ] Create temporary copy of cookbook directory
  - [ ] Run `cookstyle --auto-correct --format json` on the copy
  - [ ] Generate unified diff between original and auto-corrected files
  - [ ] Compute statistics (total offenses, correctable, remaining, files modified)
  - [ ] Persist auto-correct preview to `autocorrect_previews` table
  - [ ] Clean up temporary copy
  - [ ] Only generate for cookbooks with CookStyle offenses
  - [ ] Cache results for immutable Chef server cookbook versions
- [ ] Implement migration documentation link enrichment:
  - [ ] Build and maintain cop-to-documentation mapping table (`cop_name → { description, migration_url, introduced_in, removed_in, replacement_pattern }`)
  - [ ] Ship mapping as embedded data in the application binary
  - [ ] Enrich every `ChefDeprecations/*` and `ChefCorrectness/*` offense with its mapping entry
  - [ ] Persist enriched offenses in CookStyle results
- [ ] Implement cookbook complexity scoring:
  - [ ] Compute weighted score per cookbook per target Chef Client version (error: 5, deprecation: 3, correctness: 3, non-auto-correctable: 4, modernize: 1, TK converge fail: 20, TK test fail: 10)
  - [ ] Classify score into labels: `none` (0), `low` (1-10), `medium` (11-30), `high` (31-60), `critical` (61+)
  - [ ] Compute blast radius: affected node count, role count (using dependency graph), policy count
  - [ ] Persist complexity records to `cookbook_complexity` table
  - [ ] Recompute after every CookStyle scan and Test Kitchen run cycle

## Node Upgrade Readiness

- [ ] Implement readiness calculation per node per target Chef Client version
- [ ] Evaluate nodes in parallel using goroutines, bounded by the `concurrency.readiness_evaluation` worker pool setting
- [ ] Check all cookbooks in expanded run-list are compatible with target version
- [ ] Check available disk space meets threshold for Habitat bundle installation
- [ ] Record blocking reasons per node (incompatible cookbooks with complexity scores, insufficient disk space)
- [ ] Handle stale nodes: treat disk space data as unknown, set `stale_data` flag on readiness result
- [ ] Include complexity score and label in each blocking cookbook entry
- [ ] Persist readiness results to datastore