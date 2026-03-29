# Test Kitchen Driver Abstraction — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Review Bugs (must fix before merge)

- [x] `DriverSettings` type is `map[string]string` — cannot represent nested YAML structures (spec shows nested `vm_customization`). Change to `map[string]any` in both `TestKitchenConfig` and `PlatformMapEntry`. Update overlay generation and tests.
- [x] `majorVersionMatch` false-positive: only compares before first `.`, so `22.04` matches `22.10`. Fix to compare as many version components as the kitchen version specifies. Add test for `majorVersionMatch("22.04", "22.10")` → `false`.

## Review Fixes (fix or track in tech debt)

- [x] `InjectCredentialEnvVars` returns `baseEnv` unfiltered when creds are nil/empty — stale `CMM_TK_*` vars leak to child process. Move stripping logic before the early return.
- [x] Unknown driver warns "proceeding as custom" but skips `image_field_name` validation. Condition should be `driver == "custom" || !knownDrivers[driver]`.
- [x] Silent `json.Unmarshal` error discard in `cookbook_platform_coverage.go` scanner — corrupt JSONB data swallowed. Log or return error.
- [x] `ValidateDriverCredentials` discards resolved plaintext without zeroing — decrypted bytes leak to GC. Zero immediately after validation check.
- [x] Map iteration in `buildOverlay` produces non-deterministic YAML — sort keys before writing `DriverSettings`, `DriverSecrets`, and per-platform settings.
- [x] `ParseKitchenYMLPlatforms` doesn't strip inline YAML comments — `- name: ubuntu-22.04  # LTS` parses as `"ubuntu-22.04  # LTS"`.
- [x] `normalizeEnvVarSuffix` only replaces `-` and `.` — platform names with `/`, spaces, or other special chars produce invalid env var names. Strip all non-alphanumeric/non-underscore chars.
- [x] Redundant `CREATE INDEX` on `cookbook_name` in migration 0008 — unique constraint already creates an index. Remove the explicit index.
- [x] No duplicate `kitchen_name` validation in platform map config.
- [x] `MethodNotAllowed` handler test only checks `!= 200` instead of asserting `405`.

## Review Test Gaps

- [x] Add test: `majorVersionMatch("22.04", "22.10")` → false (exposes version bug)
- [x] Add test: `InjectCredentialEnvVars` with stale `CMM_TK_*` and nil creds (exposes stripping bug)
- [x] Add test: per-platform `DriverSettings` in overlay (code path at kitchen.go L669-671 untested)
- [x] Add test: `DriverSecrets` without `DriverSettings` (secrets-only driver block)
- [x] Add test: unknown driver + missing `image_field_name`
- [x] Add test: `LookupProfile` ignoring `imageFieldNameOverride` for built-in drivers
- [x] Add test: `ComputeCoverage([], nonEmptyProduction)` — empty kitchen with production data

---

## Driver Override / Overlay Generation

- [x] Refactor overlay generation to be driver-aware (extract driver-agnostic overlay builder)
- [x] Implement built-in driver profile registry (dokken, vcenter, vra, ec2, azurerm, google, vagrant, openstack)
- [x] Implement `custom` profile with operator-supplied `image_field_name`
- [x] Generate non-dokken overlay: driver block from `driver_settings` + `driver_secrets`
- [x] Generate non-dokken overlay: platforms section from platform map with image field mapping
- [x] Generate non-dokken overlay: per-platform `driver_settings` merged with top-level defaults
- [x] Generate non-dokken overlay: transport block from platform map entry
- [x] Skip Docker startup check when driver is non-dokken
- [x] Add driver credential startup validation (all `driver_secrets` exist and decrypt)
- [x] Add platform map startup validation (non-empty for non-dokken, entries have `image`)
- [x] Add `image_field_name` startup validation for `custom` profile
- [x] Write unit tests for overlay generation per driver profile
- [x] Write unit tests for startup validation (dokken, non-dokken, custom, missing credentials)

## Credential Injection

- [x] Resolve `driver_secrets` via credential resolver at test run time
- [x] Set `CMM_TK_SECRET_<UPPER_KEY>` environment variables in child process
- [x] Resolve transport `password_credential` per platform map entry
- [x] Set `CMM_TK_TRANSPORT_<NORMALIZED_PLATFORM>` environment variables in child process
- [x] Resolve transport `ssh_key_credential` per platform map entry
- [x] Set `CMM_TK_KEY_<NORMALIZED_PLATFORM>` environment variables in child process
- [x] Clear all `CMM_TK_*` environment variables after Test Kitchen process exits
- [x] Zero credential plaintext from memory after overlay write and env var setup
- [x] Write unit tests for env var naming derivation (hyphens, dots, mixed case)
- [x] Write unit tests for credential resolution failure paths
- [x] Write unit tests for memory zeroing and env var cleanup

## Platform Image Mapping

- [x] Parse platform map from config (`analysis_tools.test_kitchen.platform_map`)
- [x] Implement platform lookup by `kitchen_name` during overlay generation
- [x] Log `WARN` for platforms in `.kitchen.yml` not found in platform map
- [x] Pass through all platforms unchanged when driver is `dokken` and map is empty
- [x] Write unit tests for platform lookup (found, not found, empty map, dokken passthrough)

## Platform Coverage Analysis

- [x] Parse `.kitchen.yml` from git repo working directory to extract platform names
- [x] Query production platforms per cookbook from `cookbook_node_usage` + `node_snapshots`
- [x] Implement fuzzy matching: split kitchen name on last hyphen → `(os, version)`
- [x] Implement major version matching: `centos-7` matches `7.9.2009`
- [x] Implement `platform_family` grouping: `rhel-9` matches `rocky`, `alma`, `centos`
- [x] Handle unparseable kitchen names (report as `unmatched`)
- [x] Compute coverage report: tested_and_in_production, tested_not_in_production, in_production_not_tested
- [x] Compute gap_count, total_production_nodes, covered_node_count, coverage_percentage
- [x] Upsert results into `cookbook_platform_coverage` table
- [x] Schedule coverage recomputation after each collection + analysis cycle
- [x] Write unit tests for platform name parsing and fuzzy matching
- [x] Write unit tests for coverage computation (all categories, edge cases)

## Database Migration

- [x] Add `driver` column (TEXT, nullable) to `git_repo_test_kitchen_results`
- [x] Add `platform_name` column (TEXT, nullable) to `git_repo_test_kitchen_results`
- [x] Create `cookbook_platform_coverage` table with JSONB `coverage_data`
- [x] Write migration up and down scripts
- [ ] Write functional tests for new columns and table (build-tagged `//go:build functional`)

## Configuration

- [x] Add `test_kitchen.driver` config field (default: `dokken`)
- [x] Add `test_kitchen.timeout_minutes` config field (replaces `test_kitchen_timeout_minutes`)
- [x] Add `test_kitchen.driver_settings` config field (map)
- [x] Add `test_kitchen.driver_secrets` config field (map)
- [x] Add `test_kitchen.image_field_name` config field (string)
- [x] Add `test_kitchen.platform_map` config field (list of platform map entries)
- [x] Backward compatibility: accept `test_kitchen_timeout_minutes` with deprecation warning
- [x] Write unit tests for config parsing of new fields
- [x] Write unit tests for config validation (missing required fields per driver)

## Web API

- [x] Add `GET /api/v1/cookbooks/:name/platform-coverage` endpoint
- [x] Write handler tests for coverage endpoint (found, not found, empty coverage)

## Dashboard

- [ ] Add platform coverage summary to cookbook detail page
- [ ] Highlight coverage gaps (in production but untested)
- [ ] Show node counts per platform in coverage display

## Documentation

- [ ] Add Test Kitchen driver configuration section to top-level `README.md`
- [ ] Document platform map setup for vCenter deployment
- [ ] Document driver migration procedure (vCenter → vRA example)