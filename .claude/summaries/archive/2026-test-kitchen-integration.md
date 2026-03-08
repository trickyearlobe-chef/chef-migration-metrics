# Test Kitchen Integration

**Date:** 2026-03-06
**Components:** Analysis (Test Kitchen runner), Datastore (test_kitchen_results), Config (driver/platform overrides), Migration 0005

## Context

The analysis pipeline had CookStyle scanning for server-sourced cookbooks and cookbook usage analysis, but lacked Test Kitchen integration for git-sourced cookbooks with test suites. The specification (analysis § 2, § Design: Test Kitchen Invocation) requires running Test Kitchen against each git-sourced cookbook for every configured target Chef Client version, with skip-if-unchanged optimisation based on HEAD commit SHA.

The user also raised two important requirements not fully covered by the original specification:

1. **Driver overrides** — operators need to force a specific virtualisation/containerisation provider (e.g. dokken, vagrant, ec2) regardless of what the cookbook's `.kitchen.yml` specifies, because the testing infrastructure may differ from what cookbook authors chose.
2. **Platform overrides** — operators need to test cookbooks against the actual OS platforms consumed in production (derived from fleet data) rather than whatever platforms the cookbook author configured.

## What Was Done

### 1. Config additions (`internal/config/config.go`)

Added `TestKitchenConfig` struct to `AnalysisToolsConfig` with:

- `DriverOverride string` — forces every Test Kitchen run to use a named driver (e.g. "dokken", "vagrant", "ec2", "azurerm")
- `DriverConfig map[string]string` — arbitrary driver-specific settings merged into `.kitchen.local.yml` (e.g. `privileged: true`, `instance_type: t3.medium`)
- `PlatformOverrides []TestKitchenPlatform` — replaces the cookbook's platform list so tests run against production-aligned OS images. Each entry has a name, driver settings, and optional attributes.
- `ExtraYAML string` — raw YAML escape hatch for transport, verifier, lifecycle hooks, or anything else not covered by structured fields

Added `TestKitchenPlatform` struct with `Name`, `Driver map[string]string`, and `Attributes map[string]interface{}`.

Added validation in `validateAnalysisTools()`:
- Driver override is checked against a known driver list (warning if unrecognised, not error)
- Platform override names are required (error if empty)

### 2. Migration 0005 (`migrations/0005_test_kitchen_extra_columns`)

Added columns to the `test_kitchen_results` table:

- `converge_output TEXT` — per-phase captured output (converge)
- `verify_output TEXT` — per-phase captured output (verify)
- `destroy_output TEXT` — per-phase captured output (destroy)
- `timed_out BOOLEAN NOT NULL DEFAULT FALSE` — timeout flag
- `driver_used TEXT` — actual driver used (may differ from cookbook's if overridden)
- `platform_tested TEXT` — platform summary (override names or "cookbook-defined")
- `overrides_applied BOOLEAN NOT NULL DEFAULT FALSE` — whether .kitchen.local.yml was generated

### 3. Datastore layer (`internal/datastore/test_kitchen_results.go`)

Full CRUD for the `test_kitchen_results` table including all new columns:

**Types:** `TestKitchenResult`, `UpsertTestKitchenResultParams`

**Get methods:**
- `GetTestKitchenResult(cookbookID, targetChefVersion, commitSHA)` — exact match lookup; returns (nil, nil) if not found
- `GetTestKitchenResultByID(id)` — primary key lookup; returns ErrNotFound
- `GetLatestTestKitchenResult(cookbookID, targetChefVersion)` — most recent by started_at DESC; used for skip-if-unchanged optimisation

**List methods:**
- `ListTestKitchenResultsForCookbook(cookbookID)` — all results ordered by target version + started_at
- `ListTestKitchenResultsForOrganisation(organisationID)` — cross-reference join: git cookbooks whose names match server cookbooks in the org
- `ListCompatibleTestKitchenResults(cookbookID)` — compatible = TRUE results
- `ListFailedTestKitchenResults(cookbookID)` — compatible = FALSE results

**Upsert:** `UpsertTestKitchenResult(params)` — INSERT ON CONFLICT UPDATE on (cookbook_id, target_chef_version, commit_sha)

**Delete:**
- `DeleteTestKitchenResultsForCookbook(cookbookID)` — manual rescan: delete all
- `DeleteTestKitchenResult(id)` — delete single result
- `DeleteTestKitchenResultsForCookbookAndVersion(cookbookID, targetChefVersion)` — delete for specific target

**Helpers:** Shared `tkrColumns` constant, `scanTestKitchenResult()` and `scanTestKitchenResults()` row-scanning functions.

### 4. Analysis layer (`internal/analysis/kitchen.go`)

**`KitchenExecutor` interface** — abstracts the `kitchen` CLI for testability. Default implementation shells out to the real binary with `os/exec`, extracting exit codes via `errors.As(*exec.ExitError)`.

**`KitchenScanner` struct** — the main runner, configured with:
- Datastore, logger, executor, kitchen binary path
- Concurrency (semaphore-bounded worker pool)
- Timeout (per converge/verify phase)
- `TestKitchenConfig` (driver/platform overrides)

**`TestCookbooks()` batch method** — the top-level entry point:
1. Filters cookbooks: only `source=git`, `HasTestSuite=true`, non-empty `HeadCommitSHA`, non-empty dir
2. Builds work items as cookbook × targetVersion cartesian product
3. Fans out goroutines bounded by semaphore channel
4. Collects results via channel, aggregates into `KitchenBatchResult`

**`testOne()` per-cookbook per-target method** — the core invocation sequence:
1. **Nil DB guard** — returns error immediately if datastore not configured
2. **Skip check** — queries `GetLatestTestKitchenResult()`; skips if commit SHA matches
3. **Kitchen YAML check** — `findKitchenYML()` checks for `.kitchen.yml`, `.kitchen.yaml`, `kitchen.yml`, `kitchen.yaml`
4. **Driver detection** — `detectDriver()` parses the cookbook's `.kitchen.yml` with a simple line scanner to extract the driver name
5. **Overlay generation** — `buildOverlay()` creates `.kitchen.local.yml` content:
   - Driver section: applies `DriverOverride` and `DriverConfig`
   - Provisioner section: sets `chef_version` (dokken) or `product_version` (other drivers) for the target version
   - Platforms section: replaces with `PlatformOverrides` if configured
   - Extra YAML: appends `ExtraYAML` verbatim
   - Returns empty string if no overlay is needed (avoids creating unnecessary files)
6. **Instance discovery** — `listInstances()` runs `kitchen list --format json` and parses the response
7. **Converge** — `runPhase(ctx, dir, "converge")` with `--concurrency=1 --log-level=info`, timeout-bounded
8. **Verify** — only if converge passed; same pattern
9. **Destroy** — `destroyBestEffort()` always runs, uses fresh 5-minute context (not parent), failures logged as WARN
10. **Persist** — `persistResult()` upserts to datastore, combines per-phase outputs into legacy `process_stdout` for backward compatibility
11. **Log** — INFO for pass, ERROR for fail, with cookbook name, target version, commit SHA, and duration

**Helper functions:**
- `yamlScalar()` — formats YAML scalar values, quoting only when truly special characters are present
- `writeAttributes()` — recursive YAML attribute writer with configurable indentation
- `truncSHA()` — truncates SHA to 8 chars for log messages
- `findKitchenYML()` — checks for kitchen config files in priority order
- `detectDriver()` — best-effort line-scan parser for `.kitchen.yml` driver name
- `countLeadingSpaces()` — helper for YAML indentation detection

### 5. Tests (`internal/analysis/kitchen_test.go` — 75+ tests)

**Mock infrastructure:**
- `mockKitchenExecutor` — records all calls with dir/args, returns preconfigured responses per phase
- `makeTempCookbookDir()` — creates temp dir with optional `.kitchen.yml` content

**Test categories:**

- **YAML helpers** (7 tests): `yamlScalar` for empty, plain, colon, space, boolean, numeric, hash values
- **truncSHA** (4 tests): long, short, exactly-8, empty
- **countLeadingSpaces** (4 tests): none, spaces, tab, empty
- **findKitchenYML** (4 tests): .kitchen.yml, kitchen.yaml, not found, priority order
- **detectDriver** (8 tests): dokken, vagrant, quoted, single-quoted, no driver key, driver without name, file not found, comments ignored, extra keys, non-first section, empty file, only comments
- **buildOverlay** (14 tests): no overrides, target version only (dokken/vagrant/unknown), driver override, driver config, driver config without override, platform overrides, extra YAML, extra YAML without newline, all combined, header comment, real-world scenarios (dokken→vagrant, vagrant→dokken, EC2 with instance type, production platform alignment)
- **effectiveDriver** (3 tests): override, detected, unknown
- **effectivePlatformSummary** (2 tests): default, overrides
- **runPhase** (6 tests): success, failure, execution error, context timeout, arguments verification, combines stdout+stderr
- **listInstances** (5 tests): success, multiple instances, empty array, empty stdout, invalid JSON, execution error
- **destroyBestEffort** (3 tests): success, failure (no panic), arguments
- **TestCookbooks batch** (6 tests): empty input, filters non-git cookbooks, no target versions, work item count (2 cookbooks × 3 versions = 6), empty dir, context cancelled
- **Overlay file lifecycle** (1 test): write, verify, read back, cleanup
- **KitchenRunResult fields** (4 tests): compatible when both pass, incompatible on converge fail, incompatible on test fail, timed out is incompatible
- **KitchenBatchResult** (1 test): zero value
- **NewKitchenScanner** (4 tests): defaults, custom values, mock executor option, negative values
- **writeAttributes** (3 tests): simple, nested, empty map
- **Config validation** (4 tests): valid platform overrides, empty platform name, driver override values, extra YAML
- **KitchenInstance JSON** (2 tests): parse with last_action, parse with null last_action
- **Phase sequence** (2 tests): all pass, converge fails → verify skipped

## Final State

### Build & Vet

- `go build ./...` — clean
- `go vet ./...` — clean

### Test Counts

| Package | Tests (incl. subtests) | Status |
|---------|------:|--------|
| chefapi | 74 | ✅ PASS |
| collector | 160 | ✅ PASS |
| config | 117 | ✅ PASS |
| datastore | 140 | ✅ PASS |
| logging | 133 | ✅ PASS |
| secrets | 354 | ✅ PASS |
| analysis | 96 (was 51; +45 new kitchen tests) | ✅ PASS |
| embedded | 16 | ✅ PASS |
| **Total** | **~1267** (via `-v` PASS count) | ✅ ALL PASS |

### Progress Summary

| Area | Done | Total | % |
|------|-----:|------:|--:|
| Specification | 32 | 32 | 100% |
| Project setup | 15 | 16 | 93% |
| Data collection | 64 | 67 | 95% |
| Analysis | 32 | 61 | 52% |
| Visualisation | 0 | 86 | 0% |
| Logging | 11 | 12 | 91% |
| Auth | 0 | 5 | 0% |
| Configuration | 43 | 83 | 51% |
| Secrets storage | 85 | 150 | 56% |
| Packaging | 18 | 97 | 18% |
| Testing | 4 | 40 | 10% |
| Documentation | 0 | 25 | 0% |
| **Total** | **304** | **674** | **45%** |

### What Now Works End-to-End

The analysis pipeline now supports both cookbook testing strategies:

1. **Server-sourced cookbooks** → CookStyle scanning (linting + deprecation warnings)
2. **Git-sourced cookbooks** → Test Kitchen (converge + verify against target Chef versions)

Both support:
- Multiple target Chef Client versions (fan-out as cartesian product)
- Immutability/skip optimisation (CookStyle: by cookbook version; TK: by commit SHA)
- Parallel execution bounded by configured concurrency
- Per-scan/run timeout with graceful handling
- Full output capture and persistence
- Manual rescan/reset

The Test Kitchen integration additionally supports:
- Driver override (force dokken/vagrant/ec2/etc regardless of cookbook's `.kitchen.yml`)
- Platform overrides (test against production-aligned OS images)
- Driver config injection (privileged mode, instance types, network settings)
- Extra YAML escape hatch (transport, verifier, lifecycle hooks)
- Per-phase output capture (converge/verify/destroy stored separately)
- Timed-out flag tracking
- Driver and platform metadata recording for audit trail

## Known Gaps

- **Not wired into the collection pipeline** — `KitchenScanner.TestCookbooks()` exists as a standalone callable like the CookStyle scanner. Wiring into `collector.go` requires deciding whether Test Kitchen runs synchronously after collection (adds significant latency for large fleets) or is triggered separately (e.g. dedicated analysis scheduler).
- **No integration tests against real PostgreSQL** — the datastore methods (`test_kitchen_results.go`) follow the same patterns as `cookstyle_results.go` which also lacks integration tests.
- **Overlay does not inspect `driver.name` from `.kitchen.yml` ERB templates** — the `detectDriver()` function is a simple line scanner that cannot evaluate ERB. If the driver is set via ERB interpolation, it will not be detected and will fall back to "unknown" (product_version provisioner style). The driver override config provides a workaround.
- **No concurrent role fetching in Test Kitchen** — roles are fetched sequentially for the dependency graph. Not a TK issue per se, but noted as a general gap.

## Files Created

| File | Lines | Description |
|------|------:|-------------|
| `internal/analysis/kitchen.go` | ~900 | Test Kitchen runner: KitchenScanner, overlay generation, phase execution, persistence |
| `internal/analysis/kitchen_test.go` | ~1550 | 75+ tests covering all kitchen functionality |
| `internal/datastore/test_kitchen_results.go` | ~460 | Full CRUD for test_kitchen_results table |
| `migrations/0005_test_kitchen_extra_columns.up.sql` | 34 | Add per-phase output, timed_out, driver/platform tracking |
| `migrations/0005_test_kitchen_extra_columns.down.sql` | 19 | Revert migration |

## Files Modified

| File | Description |
|------|-------------|
| `internal/config/config.go` | Added `TestKitchenConfig`, `TestKitchenPlatform` structs; updated `AnalysisToolsConfig`; added validation for TK overrides |
| `.claude/specifications/todo/analysis.md` | Marked 10 Test Kitchen items as done |
| `.claude/specifications/ToDo.md` | Updated counts (analysis 52%, total 45%), marked TK/CookStyle/embedded as complete in token estimates, updated next steps |

## Recommended Next Steps (Priority Order)

### 1. Remediation Guidance (`todo/analysis.md` — 17 items)
**Why:** Completes the data needed for the migration dashboard. Uses CookStyle results + role dependency graph already in place.

**Sub-tasks:**
1. Auto-correct preview generation — temp copy, `cookstyle --auto-correct --format json`, unified diff, statistics, persist to `autocorrect_previews` table
2. Cop-to-documentation mapping — embedded data mapping cop_name to migration docs, enrich CookStyle offenses
3. Cookbook complexity scoring — weighted formula (error:5, deprecation:3, correctness:3, non-correctable:4, modernize:1, TK converge fail:20, TK test fail:10), classify into labels, compute blast radius via dependency graph, persist to `cookbook_complexity`

### 2. Node Readiness Evaluation (`todo/analysis.md` — 8 items)
**Why:** Final analysis piece. Per-node per-target-version readiness check (cookbook compatibility + disk space), parallel evaluation, stale node handling.

### 3. Wire Test Kitchen into Collection/Analysis Pipeline
**Why:** TK scanner exists but isn't automatically triggered. Options: (a) add as Step 12 in `collectOrganisation()` after usage analysis, (b) separate analysis scheduler that runs TK independently of data collection.

### 4. Web API (`internal/webapi/` — ~800-1200 lines)
**Why:** All compatibility data (CookStyle verdicts, TK results, usage analysis) is now persisted. The API can serve it immediately.