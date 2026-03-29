# Test Kitchen Driver Abstraction — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Driver Override / Overlay Generation

- [ ] Refactor overlay generation to be driver-aware (extract driver-agnostic overlay builder)
- [ ] Implement built-in driver profile registry (dokken, vcenter, vra, ec2, azurerm, google, vagrant, openstack)
- [ ] Implement `custom` profile with operator-supplied `image_field_name`
- [ ] Generate non-dokken overlay: driver block from `driver_settings` + `driver_secrets`
- [ ] Generate non-dokken overlay: platforms section from platform map with image field mapping
- [ ] Generate non-dokken overlay: per-platform `driver_settings` merged with top-level defaults
- [ ] Generate non-dokken overlay: transport block from platform map entry
- [ ] Skip Docker startup check when driver is non-dokken
- [ ] Add driver credential startup validation (all `driver_secrets` exist and decrypt)
- [ ] Add platform map startup validation (non-empty for non-dokken, entries have `image`)
- [ ] Add `image_field_name` startup validation for `custom` profile
- [ ] Write unit tests for overlay generation per driver profile
- [ ] Write unit tests for startup validation (dokken, non-dokken, custom, missing credentials)

## Credential Injection

- [ ] Resolve `driver_secrets` via credential resolver at test run time
- [ ] Set `CMM_TK_SECRET_<UPPER_KEY>` environment variables in child process
- [ ] Resolve transport `password_credential` per platform map entry
- [ ] Set `CMM_TK_TRANSPORT_<NORMALIZED_PLATFORM>` environment variables in child process
- [ ] Resolve transport `ssh_key_credential` per platform map entry
- [ ] Set `CMM_TK_KEY_<NORMALIZED_PLATFORM>` environment variables in child process
- [ ] Clear all `CMM_TK_*` environment variables after Test Kitchen process exits
- [ ] Zero credential plaintext from memory after overlay write and env var setup
- [ ] Write unit tests for env var naming derivation (hyphens, dots, mixed case)
- [ ] Write unit tests for credential resolution failure paths
- [ ] Write unit tests for memory zeroing and env var cleanup

## Platform Image Mapping

- [ ] Parse platform map from config (`analysis_tools.test_kitchen.platform_map`)
- [ ] Implement platform lookup by `kitchen_name` during overlay generation
- [ ] Log `WARN` for platforms in `.kitchen.yml` not found in platform map
- [ ] Pass through all platforms unchanged when driver is `dokken` and map is empty
- [ ] Write unit tests for platform lookup (found, not found, empty map, dokken passthrough)

## Platform Coverage Analysis

- [ ] Parse `.kitchen.yml` from git repo working directory to extract platform names
- [ ] Query production platforms per cookbook from `cookbook_node_usage` + `node_snapshots`
- [ ] Implement fuzzy matching: split kitchen name on last hyphen → `(os, version)`
- [ ] Implement major version matching: `centos-7` matches `7.9.2009`
- [ ] Implement `platform_family` grouping: `rhel-9` matches `rocky`, `alma`, `centos`
- [ ] Handle unparseable kitchen names (report as `unmatched`)
- [ ] Compute coverage report: tested_and_in_production, tested_not_in_production, in_production_not_tested
- [ ] Compute gap_count, total_production_nodes, covered_node_count, coverage_percentage
- [ ] Upsert results into `cookbook_platform_coverage` table
- [ ] Schedule coverage recomputation after each collection + analysis cycle
- [ ] Write unit tests for platform name parsing and fuzzy matching
- [ ] Write unit tests for coverage computation (all categories, edge cases)

## Database Migration

- [ ] Add `driver` column (TEXT, nullable) to `git_repo_test_kitchen_results`
- [ ] Add `platform_name` column (TEXT, nullable) to `git_repo_test_kitchen_results`
- [ ] Create `cookbook_platform_coverage` table with JSONB `coverage_data`
- [ ] Write migration up and down scripts
- [ ] Write functional tests for new columns and table (build-tagged `//go:build functional`)

## Configuration

- [ ] Add `test_kitchen.driver` config field (default: `dokken`)
- [ ] Add `test_kitchen.timeout_minutes` config field (replaces `test_kitchen_timeout_minutes`)
- [ ] Add `test_kitchen.driver_settings` config field (map)
- [ ] Add `test_kitchen.driver_secrets` config field (map)
- [ ] Add `test_kitchen.image_field_name` config field (string)
- [ ] Add `test_kitchen.platform_map` config field (list of platform map entries)
- [ ] Backward compatibility: accept `test_kitchen_timeout_minutes` with deprecation warning
- [ ] Write unit tests for config parsing of new fields
- [ ] Write unit tests for config validation (missing required fields per driver)

## Web API

- [ ] Add `GET /api/v1/cookbooks/:name/platform-coverage` endpoint
- [ ] Write handler tests for coverage endpoint (found, not found, empty coverage)

## Dashboard

- [ ] Add platform coverage summary to cookbook detail page
- [ ] Highlight coverage gaps (in production but untested)
- [ ] Show node counts per platform in coverage display

## Documentation

- [ ] Add Test Kitchen driver configuration section to top-level `README.md`
- [ ] Document platform map setup for vCenter deployment
- [ ] Document driver migration procedure (vCenter → vRA example)