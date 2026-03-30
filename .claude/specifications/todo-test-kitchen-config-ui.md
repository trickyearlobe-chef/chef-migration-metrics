# Test Kitchen Configuration UI — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Database

- [x] Migration `0010_runtime_settings` — create table (key TEXT PK, value JSONB, updated_at, updated_by)
- [x] Migration down script
- [x] Datastore CRUD — `Get/Set/DeleteRuntimeSetting` methods
- [x] Datastore functional tests

## API Handlers

- [x] `GET /api/v1/admin/test-kitchen/config` — returns DB override or file fallback
- [x] `PUT /api/v1/admin/test-kitchen/config` — validates and saves config
- [x] `DELETE /api/v1/admin/test-kitchen/config?confirm=true` — reverts to file config
- [x] Route registration in router.go (`r.adminOnly`)
- [x] Validation: platform_map required for non-dokken, image required per entry, no duplicate kitchen_name, image_field_name for custom
- [x] Handler tests (happy path, validation errors, fallback to file, method not allowed)

## Scanner Integration

- [x] `KitchenScanner.SetTestKitchenConfig` setter method
- [x] Test: setter updates config used by `buildOverlay`
- [x] Collector reads `runtime_settings` before each Test Kitchen batch
- [x] Collector falls back to YAML config when no DB override exists

## Frontend

- [x] Types in `types.ts` — `TestKitchenConfig`, `PlatformMapEntry`, `PlatformMapTransport`, response types
- [x] API functions in `api.ts` — `fetchTestKitchenConfig`, `saveTestKitchenConfig`, `deleteTestKitchenConfig`
- [x] `AdminTestKitchenPage.tsx` — driver config, settings/secrets editors, platform map table
- [x] Route in `App.tsx` with `RequireAdmin` wrapper
- [x] Nav item in `AppLayout.tsx` admin section
- [x] TypeScript compiles clean
- [x] Frontend builds clean

## Remaining

- [ ] End-to-end manual test: save config → trigger collection → verify overlay generated correctly
- [ ] Credential reference warnings on PUT (currently a no-op placeholder)
- [ ] Per-platform driver_settings key-value editor (currently JSON textarea — acceptable for v1)