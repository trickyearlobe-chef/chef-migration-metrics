# Test Kitchen Configuration UI — Specification

## TL;DR

Add an admin page at `/admin/test-kitchen` that lets operators configure the Test Kitchen driver, connection settings, platform map, and credential references through the browser. Settings are stored in a `runtime_settings` database table and merged over the YAML file defaults (DB wins). The kitchen scanner reads config from DB at the start of each collection run — no restart required.

## Problem

All Test Kitchen driver configuration (driver name, driver_settings, driver_secrets, platform_map) lives in `config.yml` and requires SSH access, YAML editing, and an application restart to change. Credentials can already be managed via Admin → Credentials, but the config that references those credentials is file-only. This makes it impossible to demo or deploy vCenter/EC2/vRA without filesystem access.

## Scope

### In Scope

- Database table for runtime settings (generic key-value with JSONB values)
- API endpoints to read and write Test Kitchen configuration
- Frontend admin page with form-based editing
- Config merge: DB settings override YAML file defaults
- Scanner picks up new config on next collection run (no restart)
- Startup validation of DB-stored config (same rules as file config)

### Out of Scope

- UI for other config sections (collection schedule, concurrency, etc.) — future work
- Live reload mid-run — config is read at run start, not mid-converge
- Config versioning or change history — use the audit log pattern from credentials

## Database

### New Table: `runtime_settings`

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `key` | TEXT | No | Setting key (primary key) |
| `value` | JSONB | No | Setting value |
| `updated_at` | TIMESTAMPTZ | No | Last update time |
| `updated_by` | TEXT | No | Username of admin who last changed it |

**Primary key:** `key`

Migration number: `0010_runtime_settings`

### Key Schema

A single key `test_kitchen` stores the entire Test Kitchen configuration as one JSONB document. This keeps the read path simple (one row fetch) and matches the shape of the YAML config.

Stored value structure:

```
{
  "enabled": true,
  "driver": "vcenter",
  "timeout_minutes": 30,
  "driver_settings": {
    "vcenter_host": "vcenter.example.com",
    "vcenter_username": "user@vsphere.local",
    "vcenter_disable_ssl_verify": false,
    "clone_type": "full",
    "datacenter": "Datacenter"
  },
  "driver_secrets": {
    "vcenter_password": "vcenter-password"
  },
  "image_field_name": "",
  "platform_map": [
    {
      "kitchen_name": "ubuntu-22.04",
      "image": "tmpl-ubuntu-2204-base",
      "driver_settings": {
        "cluster": "Cluster-01",
        "resource_pool": "Kitchen",
        "folder": "kitchen-vms"
      },
      "transport": {
        "username": "kitchen",
        "password_credential": "kitchen-vm-password"
      }
    }
  ]
}
```

## API Endpoints

All endpoints are admin-only (`r.adminOnly`).

### GET /api/v1/admin/test-kitchen/config

Returns the effective Test Kitchen configuration (DB merged over YAML). Response includes a `source` field per section so the UI can show what came from the file vs what was overridden.

Response `200 OK`:

```
{
  "config": {
    "enabled": true,
    "driver": "vcenter",
    "timeout_minutes": 30,
    "driver_settings": { ... },
    "driver_secrets": { ... },
    "image_field_name": "",
    "platform_map": [ ... ]
  },
  "source": "database",
  "updated_at": "2025-07-18T10:30:00Z",
  "updated_by": "admin"
}
```

When no DB override exists, `source` is `"file"` and `updated_at`/`updated_by` are omitted.

### PUT /api/v1/admin/test-kitchen/config

Validates and saves the Test Kitchen configuration to the database. Applies the same validation rules as file-based config (see test-kitchen-drivers.md § Startup Validation).

Request body: same shape as the `config` field in the GET response.

Response `200 OK`: returns the saved config (same shape as GET).

Response `422 Unprocessable Entity`: validation errors.

```
{
  "error": "validation_failed",
  "details": [
    "driver_secrets reference 'vcenter-password' does not exist in credential store",
    "platform_map[2] missing required field 'image'"
  ]
}
```

Validation checks:
- If driver is not `dokken`, `platform_map` must be non-empty
- Each platform map entry must have `kitchen_name` and `image`
- No duplicate `kitchen_name` values in platform map
- If driver is `custom`, `image_field_name` must be set
- All `driver_secrets` values must reference existing credentials (warn, don't block)
- All `password_credential` and `ssh_key_credential` values must reference existing credentials (warn, don't block)

### DELETE /api/v1/admin/test-kitchen/config

Removes the DB override, reverting to file-based config. Requires `?confirm=true`.

Response `204 No Content`.

## Config Merge Behaviour

### Precedence

```
DB runtime_settings → YAML file → built-in defaults
```

DB values completely replace the YAML `test_kitchen` block when present — this is a full override, not a field-level merge. The operator edits the complete config in the UI; there is no partial patching.

### Read Path

Add a `GetTestKitchenConfig` method to the datastore that:

1. Queries `runtime_settings` for key `test_kitchen`
2. If found, unmarshals JSONB into `config.TestKitchenConfig` and returns it
3. If not found, returns nil (caller falls back to YAML)

### Scanner Integration

The `KitchenScanner` gets a new `SetTestKitchenConfig(cfg config.TestKitchenConfig)` method. The collector calls `GetTestKitchenConfig` from the datastore at the start of each collection run. If a DB config exists, it calls `SetTestKitchenConfig` before invoking `TestGitRepos`. This is safe because collection runs are serialized by `Collector.mu`.

Flow per collection run:

1. Collector acquires `mu` lock (existing)
2. Collector reads `runtime_settings` for `test_kitchen` key
3. If DB config exists → `kitchenScanner.SetTestKitchenConfig(dbConfig)`
4. If no DB config → `kitchenScanner.SetTestKitchenConfig(yamlConfig)` (reset to file default)
5. Collector calls `kitchenScanner.TestGitRepos(...)` (existing)
6. Collector releases `mu` lock (existing)

## Frontend

### Route

`/admin/test-kitchen` — added to the admin nav alongside Credentials, Users, etc.

### Page Layout

The page has three sections stacked vertically:

**1. Driver Configuration**
- Driver dropdown: `dokken`, `vcenter`, `vra`, `ec2`, `azurerm`, `google`, `vagrant`, `openstack`, `custom`
- Timeout minutes: number input
- Enabled: toggle switch
- Image field name: text input (shown only when driver is `custom`)

**2. Driver Settings & Secrets**
- Two-column key-value editor for `driver_settings` (add/remove rows)
- Key-value editor for `driver_secrets` where the value column is a dropdown populated from the credentials list (`fetchCredentials`)
- When a known driver is selected, pre-populate common setting keys as empty rows (e.g. for `vcenter`: `vcenter_host`, `vcenter_username`, `vcenter_disable_ssl_verify`, `clone_type`, `datacenter`)

**3. Platform Map**
- Table with columns: Kitchen Name, Image, Driver Settings (expandable), Transport Username, Transport Password Credential (dropdown), Transport SSH Key Credential (dropdown)
- Add Row / Remove Row buttons
- Per-row expandable driver settings (key-value editor, collapsed by default)
- Credential dropdowns populated from the credentials list

**Footer**
- Save button — PUT to API, show success/error banner
- Revert to File Config button — DELETE to API with confirmation modal
- Source indicator: "Currently using: database config (saved by admin at ...)" or "Currently using: file config"

### Existing Component Reuse

- `<Modal>` component for confirmation dialogs
- Error/success banners matching credential page pattern
- Form styling using existing `INPUT_CLS` constant and Tailwind classes
- API client functions in `api.ts` following existing `fetchCredentials` / `createCredential` patterns

## Validation

### Client-Side (immediate feedback)

- Kitchen name and image are required on each platform map row
- No duplicate kitchen names
- Image field name required when driver is `custom`
- At least one platform map entry when driver is not `dokken`

### Server-Side (on PUT)

- All client-side checks repeated
- Credential references checked against credential store (warnings, not blockers — credentials can be created after config is saved)
- Full `TestKitchenConfig` validation (same as startup validation)

## Testing

### Backend

- Datastore: `GetRuntimeSetting`, `SetRuntimeSetting`, `DeleteRuntimeSetting` — unit tests for CRUD, not-found, JSONB round-trip
- Handler: `GET/PUT/DELETE /api/v1/admin/test-kitchen/config` — tests for happy path, validation errors, not-found fallback to file, credential reference warnings, 422 on bad input, admin-only access
- Config merge: test that DB config overrides YAML, deletion reverts to YAML
- Scanner integration: test that `SetTestKitchenConfig` updates the config used by `buildOverlay`

### Frontend

- Component renders all three sections
- Driver dropdown change shows/hides relevant fields
- Platform map add/remove row works
- Save calls PUT, shows success banner
- Validation errors display inline

## Implementation Order

1. Migration `0010_runtime_settings` — create table
2. Datastore CRUD — `Get/Set/DeleteRuntimeSetting` methods
3. API handlers — `GET/PUT/DELETE /api/v1/admin/test-kitchen/config`
4. `KitchenScanner.SetTestKitchenConfig` method
5. Collector integration — read DB config at run start
6. Frontend page — driver config, settings/secrets, platform map, save/revert
7. Tests throughout

## Related Specifications

| Specification | Relevance |
|---------------|-----------|
| test-kitchen-drivers.md | Driver profiles, overlay generation, platform map schema, startup validation rules |
| configuration.md | YAML config schema for `analysis_tools.test_kitchen` |
| secrets-storage.md | Credential store — driver_secrets and transport credentials reference these |