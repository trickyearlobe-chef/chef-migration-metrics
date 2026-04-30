# Encrypted Config Store — ToDo

## Phase 1 — Functional/Manual Testing

- [ ] Verify migration runs cleanly up and down (requires functional test DB)
- [ ] Functional tests for credential adapter round-trip through real DB
- [ ] Functional test: insert legacy rows, migrate, verify decrypt
- [ ] Verify existing credential API and UI still work (manual/functional testing)

## Phase 4 — Deprecation (deferred post-release)

- [ ] Migration to drop `credentials` table
- [x] Migration to drop `runtime_settings` table
- [ ] Remove YAML config parsing for non-bootstrap fields
- [ ] Remove env var overrides for non-bootstrap fields
- [x] Remove `runtime_settings` datastore methods
- [ ] Update all specs and documentation