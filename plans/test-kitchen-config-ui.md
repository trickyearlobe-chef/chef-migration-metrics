# Plan: Test Kitchen Configuration UI

## Goal

Build an admin UI at `/admin/test-kitchen` to configure Test Kitchen driver settings, so the full demo flow (add config → run scan → view results) works without touching `config.yml`.

## Specs to Read

- `.claude/specifications/test-kitchen-config-ui.md` — this feature
- `.claude/specifications/test-kitchen-drivers.md` — driver profiles, validation rules
- `.claude/specifications/configuration.md` — existing config schema

## Reference Patterns

- `internal/datastore/credentials.go` → datastore CRUD pattern
- `internal/webapi/handle_credentials.go` → admin handler pattern
- `frontend/src/pages/credentials/` → admin page pattern
- `internal/webapi/router.go` → route registration

## Steps

### 1. Migration `0010_runtime_settings`

- Create `runtime_settings` table (key TEXT PK, value JSONB, updated_at, updated_by)
- Up and down scripts
- Test: functional test for table creation

### 2. Datastore CRUD

- `GetRuntimeSetting(key) → (json.RawMessage, updated_at, updated_by, error)`
- `SetRuntimeSetting(key, value, updated_by) → error`
- `DeleteRuntimeSetting(key) → error`
- Tests: get/set/delete, not-found returns nil, JSONB round-trip

### 3. API Handlers

- `GET /api/v1/admin/test-kitchen/config` — read DB, fall back to YAML, return merged config + source
- `PUT /api/v1/admin/test-kitchen/config` — validate, save to DB, return saved config
- `DELETE /api/v1/admin/test-kitchen/config?confirm=true` — delete DB override
- Wire into router via `r.adminOnly`
- Tests: happy path, validation errors, credential reference warnings, fallback to file, admin-only

### 4. Scanner Runtime Config Swap

- Add `KitchenScanner.SetTestKitchenConfig(cfg config.TestKitchenConfig)`
- In collector, before `TestGitRepos`: read DB config, call setter if found, else reset to YAML default
- Tests: setter updates config used by `buildOverlay`

### 5. Frontend Page

- New route `/admin/test-kitchen` in App.tsx
- Add nav item in AppLayout.tsx admin section
- Page component `AdminTestKitchenPage.tsx`:
  - Driver dropdown (dokken, vcenter, vra, ec2, azurerm, google, vagrant, openstack, custom)
  - Enabled toggle, timeout input
  - Key-value editor for driver_settings
  - Key-value editor for driver_secrets (value = credential dropdown)
  - Platform map table (kitchen_name, image, transport, per-platform driver_settings)
  - Save button → PUT, success/error banner
  - Revert to File Config → DELETE with confirm
  - Source indicator
- API functions in api.ts
- Types in types.ts

### 6. Commit and Verify

- Run all Go tests
- Run frontend build
- Clean up plan

## Acceptance Criteria

- Admin can open `/admin/test-kitchen`, select `vcenter`, fill in settings, add platform map entries, and save
- Config is persisted in DB and survives restart
- Next collection run uses the saved config (no restart needed)
- Reverting to file config works
- Validation errors shown inline
- All new code has tests