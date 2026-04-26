# Fix Node Kitchen Config & Simplify Platform Mapping

## Goal

1. Node kitchen runs use the real driver (not `dummy`) so converge actually works.
2. Hypervisor templates are surfaced in the UI so operators can pick real images.
3. Platform mapping is easier to set up with discovered platforms and template pickers.

## Context

- Node kitchen generates `.kitchen.yml` with `driver: dummy` and relies on an overlay to fix it.
- The overlay returns empty when the ohai platform name (`almalinux-10.1`) doesn't match the platform map entry (`alma10`).
- Result: dummy driver → fake VM → SSH to nothing → `root@'s password:` → crash.
- The backend already has `ListTemplates()` (Proxmox/vCenter) and `GET /api/v1/hypervisor/templates`.
- The backend already has `RebuildDiscoveredPlatforms()` and `GET /api/v1/admin/platform-mapping/status` which returns both discovered platforms AND hypervisor templates.
- The frontend receives `mappingStatus.templates` but never renders it — zero references in TSX.
- Images are created manually with free-text ID fields — no template picker.

## Specs to Read

- `.claude/specifications/test-kitchen-drivers.md` (overlay generation, platform map, credential model)
- `.claude/specifications/test-kitchen-config-ui.md` (admin UI spec)

## Phase 1 — Unblock Node Kitchen (backend only)

### 1a. `GenerateKitchenYML` accepts `TestKitchenConfig`

File: `internal/nodekitchen/config_gen.go`

- Add `TKConfig *config.TestKitchenConfig` to `KitchenGenConfig`.
- When `TKConfig` is non-nil and `TKConfig.Driver` is set, generate the real driver block (name, driver_settings, driver_secrets as ERB env refs) instead of `driver: dummy`.
- Use `analysis.LookupProfile()` for the image field name.
- Find the best matching image for the node's platform: try `MatchPlatform` first, then fall back to matching ohai `platform-version` against image names with fuzzy logic (lowercase substring), then fall back to first image if only one exists.
- Generate the platform block with the matched image ID, per-image driver settings.
- Generate the transport block from the matched image (username, password as ERB, ssh_key as ERB path ref).
- Generate the provisioner block with chef_license_key ERB ref when configured.
- When `TKConfig` is nil or `TKConfig.Driver` is empty, keep current `dummy` behaviour (backward compatible).

### 1b. Runner skips overlay when full config is generated

File: `internal/nodekitchen/runner.go`

- In `Run()`, pass `&r.deps.TKConfig` into `KitchenGenConfig.TKConfig`.
- When `TKConfig.Driver` is set, skip the `GenerateOverlay` call (set overlay to `""`).
- Credential resolution and SSH key file writing remain unchanged.

### 1c. Tests

File: `internal/nodekitchen/config_gen_test.go`

- `TestGenerateKitchenYML_WithTKConfig_RealDriver` — proxmox driver, image, transport.
- `TestGenerateKitchenYML_WithTKConfig_NoMatchFallbackSingleImage` — single image, no platform match, uses it anyway.
- `TestGenerateKitchenYML_WithTKConfig_DriverSecrets` — ERB env var refs in driver block.
- `TestGenerateKitchenYML_WithTKConfig_SSHKeyPath` — `CMM_TK_KEY_PATH_*` in transport.
- `TestGenerateKitchenYML_WithTKConfig_NilConfig` — falls back to dummy.
- `TestGenerateKitchenYML_WithTKConfig_EmptyDriver` — falls back to dummy.

### 1d. Update `KitchenGenConfig` docs

File: `internal/nodekitchen/config_gen.go` — update struct doc comment.

## Phase 2 — Template Picker in Images UI (frontend)

### 2a. Fetch and display hypervisor templates

File: `frontend/src/pages/AdminTestKitchenPage.tsx`

- Add state for `hypervisorTemplates` (from `mappingStatus.templates` which is already fetched).
- In the Images section, add an "Import Template" button that opens a picker/dropdown.
- Picker shows templates from the hypervisor: name, ID, guest OS.
- Clicking a template pre-fills a new image entry with `name` from template name and `id` from template ID.
- If no hypervisor is configured or templates are empty, show a greyed-out hint.

### 2b. Template ID autocomplete on existing images

File: `frontend/src/pages/AdminTestKitchenPage.tsx`

- Replace the free-text "Infrastructure ID" input with a combobox.
- Dropdown options come from `hypervisorTemplates`.
- User can still type a custom value (combobox, not strict select).

## Phase 3 — Simplify Platform Mapping UI (frontend)

### 3a. Show all discovered platforms (not just unmapped)

File: `frontend/src/pages/AdminTestKitchenPage.tsx`

- Replace the amber "Unmapped Platforms" warning box with a full mapping table.
- Columns: Platform Name (from discovered), OS Family, Cookbook Count, Status (mapped/unmapped/skipped), Mapped Image.
- Each unmapped row has an image dropdown (populated from the images list) and "Map" button.
- Each mapped row shows the current mapping with an "Edit"/"Remove" option.
- Skipped rows are greyed out.

### 3b. Quick-map: one click to create mapping + image

- When a user clicks "Map" on an unmapped platform and no suitable image exists, offer to create one from a hypervisor template in a single flow: pick template → creates image entry + platform map entry.

### 3c. Platform name help text

- Show the node ohai format (`almalinux-10.1`) alongside the cookbook format (`alma10`) so operators understand both naming worlds.
- Add a note explaining that glob patterns (`almalinux-*`) can match multiple platforms.

## Acceptance Criteria

### Phase 1
- Node kitchen run for `homekube001.home.arpa` uses `proxmox` driver, template `117`, and reaches the converge step (no more `[Dummy] Create`).
- Existing git kitchen overlay behaviour is unchanged (no regressions).
- All existing tests pass; new tests cover the real-driver path.

### Phase 2
- Admin TK page shows available Proxmox templates in the images section.
- Clicking a template pre-fills a new image entry.
- Infrastructure ID field has autocomplete from hypervisor templates.

### Phase 3
- All discovered platforms are visible in the platform map section.
- Unmapped platforms can be mapped to images in one or two clicks.
- Mapped/skipped/unmapped status is clear at a glance.

## Order of Work

1. Phase 1a + 1c + 1b + 1d (backend, unblocks node kitchen immediately)
2. Phase 2a + 2b (frontend, template picker)
3. Phase 3a + 3b + 3c (frontend, mapping UX)
4. Delete this plan.