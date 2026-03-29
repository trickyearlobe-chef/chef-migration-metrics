# Plan: Test Kitchen Driver Abstraction

## Goal

Replace the hardcoded Docker/dokken Test Kitchen driver assumption with a pluggable driver architecture. Five capabilities: driver override, credential injection, platform image mapping, platform coverage analysis, and config-only driver migration.

## Specs to Read

- `.claude/specifications/test-kitchen-drivers.md` — primary spec (all 5 capabilities)
- `.claude/specifications/todo-test-kitchen-drivers.md` — 95 tasks across 9 categories
- `.claude/specifications/analysis.md` §2 — overlay generation, startup validation, coverage step
- `.claude/specifications/configuration.md` § Analysis Tools — `test_kitchen.*` settings
- `.claude/specifications/datastore.md` §7, §23 — modified table + new table
- `.claude/specifications/project-conventions.md` — naming, migrations, error handling

## Current State

- `internal/config/config.go`: `TestKitchenConfig` has `DriverOverride`, `DriverConfig`, `PlatformOverrides`, `ExtraYAML` — old model, needs replacing with spec's `driver`, `driver_settings`, `driver_secrets`, `image_field_name`, `platform_map`
- `internal/analysis/kitchen.go`: `buildOverlay` generates overlays using old config fields; `testOne` does converge/verify/destroy with no credential injection
- `internal/datastore/git_repo_test_kitchen_results.go`: already has `driver_used` and `platform_tested` columns (Go struct fields exist, migration pending verification)
- `migrations/`: 6 migrations exist (0001–0006); no coverage table yet
- `internal/secrets/`: full resolver + store + zeroing infrastructure already built

## Implementation Order

### Phase 1: Configuration (config struct + parsing + validation)

1. Replace `TestKitchenConfig` fields with spec schema: `Driver`, `TimeoutMinutes`, `DriverSettings`, `DriverSecrets`, `ImageFieldName`, `PlatformMap`
2. Add `PlatformMapEntry` struct: `KitchenName`, `Image`, `DriverSettings`, `Transport`
3. Add `PlatformMapTransport` struct: `Username`, `PasswordCredential`, `SSHKeyCredential`
4. Backward compat: if `test_kitchen_timeout_minutes` is set and `test_kitchen.timeout_minutes` is not, copy with deprecation warning
5. Update `validateAnalysisTools` — driver profile validation, platform map validation, `image_field_name` required for `custom`
6. Update `setDefaults` — `driver` defaults to `dokken`, `timeout_minutes` to 30
7. Tests for config parsing, validation, backward compat

### Phase 2: Driver Profile Registry

1. New file `internal/analysis/driver_profiles.go` — `DriverProfile` struct with `Name`, `ImageFieldName`, `TypicalSecrets`
2. Built-in profiles: dokken, vcenter, vra, ec2, azurerm, google, vagrant, openstack
3. `custom` profile resolves `ImageFieldName` from config
4. `LookupProfile(driverName string, imageFieldName string) DriverProfile` function
5. Tests for each profile and custom fallback

### Phase 3: Overlay Generation Refactor

1. Refactor `buildOverlay` to use new config fields and driver profiles
2. dokken path: provisioner-only overlay when no platform map (unchanged behaviour)
3. Non-dokken path: driver block from `driver_settings` + `driver_secrets` ERB refs, platforms section from platform map with image field mapping, per-platform `driver_settings` merge, transport block
4. Skip platforms not in map with WARN log
5. Update `effectiveDriver` to use `tkConfig.Driver` instead of `tkConfig.DriverOverride`
6. Remove old `DriverOverride`, `DriverConfig`, `PlatformOverrides`, `ExtraYAML` fields
7. Tests: overlay per driver profile, dokken backward compat, missing platform WARN

### Phase 4: Credential Injection

1. New file `internal/analysis/kitchen_credentials.go`
2. `resolveDriverSecrets(ctx, resolver, driverSecrets) (envVars map[string]string, cleanup func(), err error)` — resolves each `driver_secrets` entry via `CredentialResolver`, builds `CMM_TK_SECRET_<UPPER_KEY>` env vars
3. `resolveTransportSecrets(ctx, resolver, platformMap) (envVars map[string]string, cleanup func(), err error)` — resolves `password_credential` → `CMM_TK_TRANSPORT_<NORMALIZED_PLATFORM>`, `ssh_key_credential` → `CMM_TK_KEY_<NORMALIZED_PLATFORM>`
4. Env var naming: uppercase, hyphens/dots → underscores
5. Inject env vars into child process environment in `testOne` (extend `sanitiseKitchenEnv`)
6. Clear all `CMM_TK_*` env vars and zero plaintext after TK process exits
7. Tests: env var naming derivation, resolution failure paths, memory zeroing, cleanup

### Phase 5: Startup Validation

1. Update startup validation in `testOne` or scanner init:
   - dokken: existing Docker check (unchanged)
   - non-dokken: all `driver_secrets` exist and decrypt → ERROR if not
   - non-dokken: `platform_map` non-empty → WARN if empty
   - non-dokken: each entry has `image` → WARN per entry without
   - `custom`: `image_field_name` configured → ERROR if not
   - transport secrets: each referenced credential exists → WARN per entry
2. Skip Docker check when driver is non-dokken
3. Tests for each validation path

### Phase 6: Database Migration

1. Migration `0007_kitchen_driver_columns.up.sql`: `ALTER TABLE git_repo_test_kitchen_results ADD COLUMN driver TEXT, ADD COLUMN platform_name TEXT` (nullable, no default — backward compat)
2. Migration `0007_kitchen_driver_columns.down.sql`: `ALTER TABLE git_repo_test_kitchen_results DROP COLUMN driver, DROP COLUMN platform_name`
3. Migration `0008_cookbook_platform_coverage.up.sql`: create `cookbook_platform_coverage` table per datastore spec §23
4. Migration `0008_cookbook_platform_coverage.down.sql`: drop table
5. Note: Go structs already have `DriverUsed`/`PlatformTested` — verify column names match migration (`driver`/`platform_name` in DB vs `driver_used`/`platform_tested` in Go). Reconcile naming.

### Phase 7: Platform Image Mapping

1. Parse `platform_map` from config during overlay generation
2. Lookup by `kitchen_name` — if found, include platform with mapped image and settings; if not, WARN and skip
3. dokken passthrough when map is empty
4. Tests: found, not found, empty map, dokken passthrough

### Phase 8: Platform Coverage Analysis

1. New file `internal/analysis/coverage.go`
2. Parse `.kitchen.yml` platforms from git repo working dir
3. Query production platforms per cookbook from node snapshots + cookbook usage
4. Fuzzy matching: split on last hyphen → `(os, version)`, major version match, `platform_family` grouping
5. Compute: `tested_and_in_production`, `tested_not_in_production`, `in_production_not_tested`, `gap_count`, `coverage_percentage`
6. New datastore file `internal/datastore/cookbook_platform_coverage.go` — CRUD for coverage table
7. Upsert results after each analysis cycle
8. Tests: platform name parsing, fuzzy matching, coverage computation edge cases

### Phase 9: Web API + Dashboard

1. `GET /api/v1/cookbooks/:name/platform-coverage` endpoint in `internal/webapi/`
2. Handler tests: found, not found, empty coverage
3. Frontend: coverage summary on cookbook detail page, gap highlighting

## Acceptance Criteria

- `driver: dokken` with no platform map produces identical behaviour to current code
- `driver: vcenter` with platform map generates correct `.kitchen.local.yml` with ERB credential refs
- Switching `driver: vcenter` → `driver: vra` is config-only (no code change)
- All `CMM_TK_SECRET_*` and `CMM_TK_TRANSPORT_*` env vars cleared after TK exits
- Credential plaintext zeroed from memory after overlay write
- Platform coverage correctly fuzzy-matches `centos-7` → `7.9.2009` and `rhel-9` → rocky/alma/centos
- Existing tests continue passing throughout
- Each phase results in its own commit(s)