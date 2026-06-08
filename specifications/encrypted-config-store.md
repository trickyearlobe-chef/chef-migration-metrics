# Encrypted Config Store — Specification

## TL;DR

Replace the YAML config file with an encrypted database-backed config store. All settings (config and secrets alike) are AES-256-GCM encrypted in a single `config_store` table. The YAML file becomes a bootstrap file containing only what's needed to reach the database and decrypt. On first start with a YAML file present, the app auto-migrates all settings into the DB and renames the file to `config.yml.migrated`. Admin UI pages provide form-based editing for all config sections — no SSH or YAML editing required after initial setup.

## Problem

Configuration lives in a YAML file on disk. Changing anything requires SSH access, YAML editing, and an application restart. Secrets are split across three mechanisms (file path, env var, DB credential store). The Test Kitchen config UI already proved the DB-override pattern works, but every other config section is still file-only.

## Design Decisions

### Encrypt everything

No distinction between "config" and "secrets" at the storage layer. Every value is encrypted with the same AES-256-GCM scheme used by the existing credential store. Benefits:

- One code path — no branching on sensitivity level
- No classification burden — no need to decide whether a vCenter username is a secret
- DB backups are useless without the master key
- Infrastructure topology (Chef server URLs, org names) is protected

### Bootstrap file

A minimal YAML file provides only what's needed before the DB is available:

```yaml
database_url: postgres://user:pass@localhost:5432/chef_migration_metrics
listen_address: "127.0.0.1"
listen_port: 8080
```

`CMM_CREDENTIAL_ENCRYPTION_KEY` (from the environment) and `database_url` are the only values not stored in the DB. `listen_address`/`listen_port` are **also** stored in the DB as the `server.listen` section — the DB copy is the source of truth for UI editing, while the bootstrap-file copy is retained solely as the **bind-failure fallback**: if the DB-sourced address/port cannot be bound at startup, the server falls back to the bootstrap value (then the hardwired `0.0.0.0:8080` default) and flags degraded mode rather than failing to start. Everything else comes from `config_store` after the DB connection is established.

Environment variable overrides for bootstrap values:

| Variable | Overrides |
|----------|-----------|
| `DATABASE_URL` | `database_url` |
| `CMM_LISTEN_ADDRESS` | `listen_address` |
| `CMM_LISTEN_PORT` | `listen_port` |

`CMM_CREDENTIAL_ENCRYPTION_KEY` remains a mandatory environment variable. It is never stored anywhere persistent.

### Auto-migration from YAML

On startup, if a full `config.yml` exists (detected by the presence of an `organisations` key), the app:

1. Reads and validates the YAML file
2. Connects to the database
3. Checks whether `config_store` already has entries
4. If empty — encrypts and inserts every config section
5. Renames `config.yml` to `config.yml.migrated`
6. Writes a minimal bootstrap `config.yml` with only `database_url`, `listen_address`, `listen_port`
7. Logs `INFO`: "Configuration migrated to database. Original saved as config.yml.migrated."

If `config_store` already has entries and a full YAML is present, the app logs `WARN` ("YAML config file contains settings beyond bootstrap values — these are ignored; config is managed in the database") and continues using DB config. It does not overwrite DB values.

### Credential store consolidation

The existing `credentials` table is merged into `config_store`. Credentials become config entries with a `secret: true` flag that controls API read behaviour (value is never returned). The `credentials` table is dropped after migration.

Migration from `credentials` to `config_store`:

- Each credential row becomes a config entry with key `credentials/<name>`
- The `credential_type` and `metadata` are preserved in the stored JSON
- The `secret` flag is set to `true`
- Existing `encrypted_value` content is re-encrypted under the new key format (or kept as-is if the encryption scheme is identical)

### Test Kitchen Credential Injection

The Test Kitchen driver spec (test-kitchen-drivers.md § Credential Model) defines how credentials are injected into `.kitchen.local.yml` overlays via environment variables and ERB. This flow is unchanged by the config store consolidation — only the backing table changes:

1. `driver_secrets` in the TK config references credential names (e.g. `vcenter_password: vcenter-password`)
2. At run time, the credential resolver looks up `credentials/vcenter-password` in `config_store` (previously `credentials` table)
3. Decrypts the value
4. Sets `CMM_TK_SECRET_VCENTER_PASSWORD=<decrypted>` in the child process environment
5. The `.kitchen.local.yml` overlay references it via ERB: `<%= ENV['CMM_TK_SECRET_VCENTER_PASSWORD'] %>`
6. Transport secrets follow the same pattern: `CMM_TK_TRANSPORT_<NORMALIZED_PLATFORM>`

The credential resolver interface remains the same — callers pass a credential name and get plaintext back. The implementation changes from querying `credentials` to querying `config_store WHERE key = 'credentials/' || name AND secret = true`.

## Database

### Table: `config_store`

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `key` | TEXT | No | Dot-notation config key (primary key) |
| `encrypted_value` | BYTEA | No | AES-256-GCM encrypted JSON value |
| `nonce` | BYTEA | No | 12-byte GCM nonce (unique per write) |
| `secret` | BOOLEAN | No | When true, API never returns the decrypted value |
| `updated_at` | TIMESTAMPTZ | No | Last modification time |
| `updated_by` | TEXT | No | Username of admin who last changed it |

**Primary key:** `key`

**Encryption:** Identical to the existing credential store — HKDF-derived key from `CMM_CREDENTIAL_ENCRYPTION_KEY`, per-row nonce, AAD binding to the key name.

### Key Naming Convention

Config keys use dot notation matching the YAML structure:

| Key | Example Value | Secret |
|-----|---------------|--------|
| `organisations` | `[{"name":"myorg", ...}]` | false |
| `target_chef_versions` | `["18.5.0"]` | false |
| `git_base_urls` | `["https://github.com/my-org"]` | false |
| `collection` | `{"schedule":"0 * * * *", ...}` | false |
| `concurrency` | `{"organisation_collection":5, ...}` | false |
| `analysis_tools` | `{"embedded_bin_dir":"...", ...}` | false |
| `readiness` | `{"min_free_disk_mb":2048}` | false |
| `server.listen` | `{"listen_address":"0.0.0.0","port":8080}` | false |
| `server.tls` | `{"mode":"off", ...}` | false |
| `server.websocket` | `{"enabled":true, ...}` | false |
| `server.graceful_shutdown_seconds` | `30` | false |
| `frontend` | `{"base_path":"/"}` | false |
| `logging` | `{"level":"INFO", ...}` | false |
| `auth` | `{"providers":[...]}` | false |
| `notifications` | `{"enabled":true, ...}` | false |
| `smtp` | `{"host":"...", ...}` | false |
| `exports` | `{"max_rows":100000, ...}` | false |
| `elasticsearch` | `{"enabled":false, ...}` | false |
| `ownership` | `{"enabled":true, ...}` | false |
| `credentials/<name>` | `{"credential_type":"generic", "value":"..."}` | true |

Each top-level YAML section maps to one row. Values are JSON-encoded before encryption. This keeps the row count small (~20 rows for a typical deployment) and reads simple (one row per config section).

### Migration

Migration `0011_config_store`:

**Up:**
- Create `config_store` table
- Migrate rows from `credentials` table into `config_store` with `key = 'credentials/' || name` and `secret = true`
- Migrate rows from `runtime_settings` table into `config_store` with `key = 'test_kitchen'` (preserving the existing Test Kitchen config UI data)
- Drop `credentials` table
- Drop `runtime_settings` table

**Down:**
- Recreate `credentials` table
- Recreate `runtime_settings` table
- Migrate rows back from `config_store`
- Drop `config_store` table

## Startup Sequence

1. Read bootstrap config (`database_url`, `listen_address`, `listen_port`) from YAML + env vars
2. Read `CMM_CREDENTIAL_ENCRYPTION_KEY` from environment — fatal if missing
3. Connect to database, run migrations
4. Check for full YAML config and auto-migrate if needed (see above)
5. Load all `config_store` rows, decrypt values
6. Assemble the in-memory `Config` struct from decrypted values
7. Validate the assembled config (same validation rules as today)
8. Start the HTTP server and background collector

If `config_store` is empty and no full YAML exists, the app starts in "setup mode" — it serves only the admin UI with a setup wizard. The collector does not run until at least one organisation is configured.

## API Endpoints

All endpoints are admin-only.

### Config Sections API

Each config section gets a dedicated GET/PUT endpoint pair for clean UI binding:

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/admin/config/organisations` | List organisations |
| `PUT` | `/api/v1/admin/config/organisations` | Replace organisations list |
| `GET` | `/api/v1/admin/config/collection` | Collection schedule and thresholds |
| `PUT` | `/api/v1/admin/config/collection` | Update collection settings |
| `GET` | `/api/v1/admin/config/target-versions` | Target Chef Client versions |
| `PUT` | `/api/v1/admin/config/target-versions` | Update target versions |
| `GET` | `/api/v1/admin/config/git-urls` | Git base URLs |
| `PUT` | `/api/v1/admin/config/git-urls` | Update git URLs |
| `GET` | `/api/v1/admin/config/concurrency` | Worker pool sizes |
| `PUT` | `/api/v1/admin/config/concurrency` | Update concurrency settings |
| `GET` | `/api/v1/admin/config/analysis-tools` | Analysis tool paths and timeouts |
| `PUT` | `/api/v1/admin/config/analysis-tools` | Update analysis tools config |
| `GET` | `/api/v1/admin/config/test-kitchen` | Test Kitchen driver config |
| `PUT` | `/api/v1/admin/config/test-kitchen` | Update Test Kitchen config |
| `DELETE` | `/api/v1/admin/config/test-kitchen` | Reset TK config to defaults |
| `GET` | `/api/v1/admin/config/server` | TLS, WebSocket, shutdown settings |
| `PUT` | `/api/v1/admin/config/server` | Update server settings |
| `GET` | `/api/v1/admin/config/auth` | Auth providers |
| `PUT` | `/api/v1/admin/config/auth` | Update auth config |
| `GET` | `/api/v1/admin/config/notifications` | Notification channels and triggers |
| `PUT` | `/api/v1/admin/config/notifications` | Update notifications |
| `GET` | `/api/v1/admin/config/logging` | Log level and retention |
| `PUT` | `/api/v1/admin/config/logging` | Update logging settings |

Response for each GET returns the decrypted JSON value for that section. Fields marked `secret` within a section (e.g. passwords embedded in an org config) are redacted in the response.

PUT endpoints validate the input using the same rules as startup validation, encrypt, and store. Changes take effect on the next collection run without restart (config is re-read from DB at the start of each run). Server-level changes (TLS, listen address) require a restart — the API response indicates this.

### Credentials API

Credentials are a special case — they are `secret: true` config entries with a write-only contract.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/admin/credentials` | List credentials (metadata only, no values) |
| `POST` | `/api/v1/admin/credentials` | Create a credential |
| `PUT` | `/api/v1/admin/credentials/:name` | Rotate a credential's value |
| `DELETE` | `/api/v1/admin/credentials/:name` | Delete a credential |
| `POST` | `/api/v1/admin/credentials/:name/test` | Test a credential |

These map to `config_store` rows with key `credentials/<name>` and `secret = true`. The existing credential API contract is preserved — the UI and tests do not change.

### Restart-Required Indicator

PUT responses include a `restart_required` boolean. It is `true` when the change affects:

- `server.tls` (any TLS setting)
- `listen_address` or `listen_port` (bootstrap values — these are also in the DB for UI display but the running process uses the bootstrap values)
- `auth` providers (session infrastructure may need reinitialisation)

The UI shows a banner: "Some changes require an application restart to take effect."

## Frontend Admin Pages

### Navigation

The admin section in the sidebar gains a **Settings** group:

```
Admin
├── Dashboard
├── Users
├── Credentials        (existing — unchanged)
├── Test Kitchen        (existing — unchanged)
└── Settings
    ├── Organisations
    ├── Collection
    ├── Target Versions
    ├── Git URLs
    ├── Concurrency
    ├── Analysis Tools
    ├── Server & TLS
    ├── Authentication
    ├── Notifications
    ├── Logging
    └── Exports
```

### Page Pattern

Each settings page follows the same pattern:

1. Load current values via GET
2. Render a form with appropriate input types
3. Validate client-side on change
4. Save via PUT, show success/error banner
5. Show "Restart required" banner when applicable

### Organisations Page

The most complex form. Table layout with one row per organisation:

| Field | Input Type | Notes |
|-------|-----------|-------|
| Name | Text | Read-only identifier |
| Chef Server URL | Text | URL validation |
| Org Name | Text | |
| Client Name | Text | |
| Credential | Dropdown | Populated from credentials list |
| SSL Verify | Toggle | Default true |

Add/remove organisation rows. Credential dropdown references credentials managed on the existing Credentials page.

Chef API keys (PEM files) are uploaded through the Credentials page as credential type `chef_client_key`. The organisation form references them by name — no file path configuration needed.

### Collection Page

| Field | Input Type | Notes |
|-------|-----------|-------|
| Schedule | Text | Cron expression with syntax hint |
| Stale Node Threshold (days) | Number | |
| Stale Cookbook Threshold (days) | Number | |

### Target Versions Page

Tag-style input — type a version, press Enter to add. Click × to remove. Validates semver format.

### Concurrency Page

| Field | Input Type | Notes |
|-------|-----------|-------|
| Organisation Collection | Number | Min 1 |
| Node Page Fetching | Number | Min 1 |
| Git Pull | Number | Min 1 |
| Cookbook Download | Number | Min 1 |
| CookStyle Scan | Number | Min 1 |
| Test Kitchen Run | Number | Min 1 |
| Readiness Evaluation | Number | Min 1 |

### Server & TLS Page

| Field | Input Type | Notes |
|-------|-----------|-------|
| Listen Address | Text | Interface to bind (e.g. `0.0.0.0`, `127.0.0.1`). Restart required. |
| Port | Number | Listen port (1–65535). Save-time preflight test-binds when the address/port changes; a value that cannot be bound is rejected. Restart required. |
| TLS Mode | Dropdown | off / static / acme |
| Certificate (static) | Textarea | PEM content, shown when mode=static |
| Key (static) | Textarea | PEM content, shown when mode=static, write-only |
| ACME Domains | Tag input | Shown when mode=acme |
| ACME Email | Text | Shown when mode=acme |
| ACME Challenge | Dropdown | http-01 / tls-alpn-01 / dns-01 |
| WebSocket Enabled | Toggle | |
| Graceful Shutdown (seconds) | Number | |

Changes show "Restart required" banner.

## Config Reload Behaviour

| Config Section | Reload Without Restart |
|----------------|----------------------|
| `organisations` | Yes — next collection run |
| `target_chef_versions` | Yes — next collection run |
| `git_base_urls` | Yes — next collection run |
| `collection` | Yes — schedule change takes effect after current tick |
| `concurrency` | Yes — next collection run |
| `analysis_tools` | Yes — next collection run |
| `test_kitchen` (under analysis_tools) | Yes — next collection run |
| `readiness` | Yes — next collection run |
| `notifications` | Yes — next notification event |
| `logging.level` | Yes — immediate |
| `server.tls` | No — restart required |
| `server.websocket` | No — restart required |
| `server.graceful_shutdown_seconds` | No — restart required |
| `auth` | No — restart required |
| `frontend` | No — restart required |
| `smtp` | Yes — next notification send |
| `exports` | Yes — next export request |
| `elasticsearch` | Yes — next collection run |

The collector re-reads config from `config_store` at the start of each collection run. The main `Config` struct is replaced atomically (pointer swap behind a `sync.RWMutex`). HTTP handlers read through the mutex. This is the same pattern the Test Kitchen config UI already uses, extended to all sections.

## Migration Path

### Phase 1: Schema and Store (this spec)

- `config_store` table and migration
- Datastore CRUD methods (encrypt, decrypt, get, set, delete, list)
- Auto-migration from YAML on startup
- Credential store consolidation
- Bootstrap file support
- Config reload via pointer swap

### Phase 2: API Endpoints

- Section-specific GET/PUT handlers
- Validation per section
- Restart-required indicator
- Credential API adapted to use `config_store`

### Phase 3: Admin UI Pages

- Organisations page (most complex — do first to prove the pattern)
- Collection, Target Versions, Git URLs, Concurrency (simple forms)
- Analysis Tools, Server & TLS, Auth, Notifications, Logging, Exports
- Setup wizard for empty `config_store`

### Phase 4: Deprecation

- Remove YAML config parsing for non-bootstrap fields
- Remove env var overrides for non-bootstrap fields
- Remove `runtime_settings` table references
- Update all specs and documentation

## Testing

### Backend

- Datastore: encrypt/decrypt round-trip, get/set/delete, list by prefix, secret flag behaviour
- Auto-migration: full YAML → DB migration, file rename, idempotency (second run is no-op)
- Credential consolidation: credentials table rows appear as `credentials/<name>` in `config_store`
- Config assembly: all sections loaded and validated from `config_store`
- API handlers: GET/PUT for each section, validation errors, restart-required flag, secret redaction
- Config reload: pointer swap under mutex, concurrent read safety

### Frontend

- Each settings page renders, loads data, saves, shows validation errors
- Organisations page: add/remove orgs, credential dropdown populated
- Restart-required banner appears for server/TLS/auth changes
- Setup wizard flow when `config_store` is empty

## Security

- All values encrypted at rest — no plaintext in the database
- Master key (`CMM_CREDENTIAL_ENCRYPTION_KEY`) is mandatory from first startup
- Secret-flagged entries are never returned via API (write-only, same as current credentials)
- Config API is admin-only (existing `r.adminOnly` middleware)
- Audit trail via `updated_at` / `updated_by` on every row
- YAML file is renamed after migration to prevent stale config confusion
- Bootstrap file contains only the DB connection string — no secrets beyond what's in the env

## Related Specifications

| Specification | Impact |
|---------------|--------|
| `configuration.md` | Superseded — YAML schema becomes bootstrap-only |
| `secrets-storage.md` | Credential store section replaced by `config_store` |
| `test-kitchen-config-ui.md` | `runtime_settings` table replaced by `config_store` |
| `web-api.md` | New admin config endpoints added |
| `packaging.md` | Bootstrap file replaces full config in RPM/DEB |
| `datastore.md` | New table, credential table dropped |