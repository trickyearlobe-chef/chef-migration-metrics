# Encrypted Config Store — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done | [!] Blocked

---

## Phase 1: Schema and Store

### Migration

- [x] Create `0011_config_store.up.sql` — `config_store` table (key TEXT PK, encrypted_value BYTEA, nonce BYTEA, secret BOOLEAN, updated_at TIMESTAMPTZ, updated_by TEXT)
- [x] Create `0011_config_store.down.sql` — drop `config_store` table
- [ ] Verify migration runs cleanly up and down (requires functional test DB)

### Datastore CRUD

- [x] `ConfigEntry` struct in `internal/datastore/config_store.go`
- [x] `GetConfigEntry(ctx, key)` — single row by PK
- [x] `SetConfigEntry(ctx, entry)` — upsert via INSERT ON CONFLICT
- [x] `DeleteConfigEntry(ctx, key)` — hard delete, idempotent
- [x] `ListConfigEntries(ctx)` — all rows ordered by key
- [x] `ListConfigEntriesByPrefix(ctx, prefix)` — WHERE key LIKE prefix%
- [x] `CountConfigEntries(ctx)` — row count
- [x] `ConfigStoreIsEmpty(ctx)` — convenience bool
- [x] Functional tests for all CRUD methods (`//go:build functional`)

### Encryption Layer

- [x] New package `internal/configstore/`
- [x] `Store` struct wrapping `*datastore.DB` + `*secrets.Encryptor`
- [x] `Get(ctx, key)` — decrypt, return `json.RawMessage`
- [x] `GetSecret(ctx, key)` — decrypt secret-flagged entries only
- [x] `Set(ctx, key, value, secret, updatedBy)` — encrypt JSON, generate nonce, upsert
- [x] `Delete(ctx, key)` — passthrough
- [x] `List(ctx)` — metadata only (no decrypted values)
- [x] `ListByPrefix(ctx, prefix)` — metadata only
- [x] `GetAll(ctx)` — decrypt all non-secret entries for config assembly
- [x] `IsEmpty(ctx)` — convenience
- [x] Raw AES-256-GCM with separate nonce column (not hex-encoded string format)
- [x] AAD bound to key name: `[]byte(key)`
- [x] Unit tests for encrypt/decrypt round-trip, AAD mismatch, secret flag filtering

### Credential Store Adapter

- [x] `CredentialStoreAdapter` in `internal/configstore/credential_adapter.go`
- [x] Implements `secrets.CredentialStore` interface
- [x] Maps credential name `foo` → config store key `credentials/foo`
- [x] All entries stored with `secret=true`
- [x] `Create` — validate, encrypt, set
- [x] `Get` — decrypt, deserialise to `secrets.Credential`
- [x] `GetMetadata` — metadata without plaintext
- [x] `Update` — validate, re-encrypt, set
- [x] `Delete` — delete with reference check
- [x] `List` — list by prefix `credentials/`
- [x] `ListByType` — list by prefix + filter by type
- [x] `Test` — get + validate
- [x] `ReferencedBy` — query orgs, auth, notifications for references
- [x] Unit tests for interface compliance
- [ ] Functional tests for round-trip through real DB

### Legacy Data Migration

- [x] `MigrateFromLegacy()` in `internal/configstore/migrate.go`
- [x] Re-encrypt `credentials` rows under new AAD scheme as `credentials/<name>`
- [x] Migrate `runtime_settings` rows as non-secret entries
- [x] Idempotent — skip if `config_store` already has entries
- [x] Log migration counts
- [x] Unit tests with mock DB
- [ ] Functional test: insert legacy rows, migrate, verify decrypt

### YAML Auto-Migration

- [x] `MigrateFromYAML()` in `internal/configstore/yaml_migrate.go`
- [x] `IsFullYAML()` detection (presence of `organisations` key)
- [x] Serialise each config section to JSON and encrypt into DB via `ConfigToSections()`
- [x] Rename `config.yml` to `config.yml.migrated` (preserves existing backup)
- [x] Write bootstrap `config.yml` with only `database_url`, `listen_address`, `listen_port` (0600 permissions)
- [x] Skip if `config_store` already has entries (log warning)
- [x] Unit tests: migration, idempotency, file rename, bootstrap content, round-trip assembly, permissions, error propagation

### Config Assembly

- [x] `AssembleConfig()` in `internal/configstore/assembly.go`
- [x] Decrypt all non-secret config entries
- [x] Unmarshal JSON into `config.Config` fields per key naming convention (via YAML decoder)
- [x] Apply defaults and validate (same rules as YAML path)
- [x] Unit tests: assemble from known JSON, validation errors, missing optional sections

### Config Reload

- [x] `ConfigHolder` in `internal/configstore/reloader.go`
- [x] `Get()` — read lock, return `*config.Config` pointer
- [x] `Reload(ctx)` — write lock, re-assemble from DB, swap pointer
- [x] `Set(cfg)` — write lock, replace (initial load)
- [x] Unit tests: concurrent read/write safety, reload replaces config (with -race)

### Startup Integration

- [x] Update `loadConfig()` to detect bootstrap vs full YAML via `IsFullYAML()`
- [x] Add `setupConfigStore()` phase after migrations and secrets setup
- [x] Run `MigrateFromLegacy()` for credential + runtime_settings migration
- [x] Run `MigrateFromYAML()` when full YAML detected
- [x] `AssembleConfig()` — load from DB if populated, else YAML (backward compat)
- [x] Carry over bootstrap values (database_url, listen_address, listen_port) from YAML
- [x] Create `ConfigHolder` with assembled config and store
- [x] Wire `CredentialStoreAdapter` in place of `DBCredentialStore` with `dbRefChecker`
- [x] `CMM_CREDENTIAL_ENCRYPTION_KEY` mandatory from first startup (fatal if missing)
- [x] Preserve legacy `DBCredentialStore` for key rotation and validation before migration
- [x] All 18 test packages pass with `-race`, zero existing test breakage
- [ ] Verify existing credential API and UI still work (requires manual/functional testing)

## Phase 2: API Endpoints

- [x] `GET /api/v1/admin/config/organisations` — list organisations
- [x] `PUT /api/v1/admin/config/organisations` — replace organisations
- [x] `GET /api/v1/admin/config/collection` — collection settings
- [x] `PUT /api/v1/admin/config/collection` — update collection
- [x] `GET /api/v1/admin/config/target-versions` — target versions
- [x] `PUT /api/v1/admin/config/target-versions` — update target versions
- [x] `GET /api/v1/admin/config/git-urls` — git base URLs
- [x] `PUT /api/v1/admin/config/git-urls` — update git URLs
- [x] `GET /api/v1/admin/config/concurrency` — concurrency settings
- [x] `PUT /api/v1/admin/config/concurrency` — update concurrency
- [x] `GET /api/v1/admin/config/analysis-tools` — analysis tools config
- [x] `PUT /api/v1/admin/config/analysis-tools` — update analysis tools
- [x] `GET /api/v1/admin/config/test-kitchen` — test kitchen config
- [x] `PUT /api/v1/admin/config/test-kitchen` — update test kitchen
- [x] `DELETE /api/v1/admin/config/test-kitchen` — reset to defaults
- [x] `GET /api/v1/admin/config/server` — server settings
- [x] `PUT /api/v1/admin/config/server` — update server settings
- [x] `GET /api/v1/admin/config/auth` — auth config
- [x] `PUT /api/v1/admin/config/auth` — update auth
- [x] `GET /api/v1/admin/config/notifications` — notifications
- [x] `PUT /api/v1/admin/config/notifications` — update notifications
- [x] `GET /api/v1/admin/config/logging` — logging settings
- [x] `PUT /api/v1/admin/config/logging` — update logging
- [x] Validation per section (inline in each PUT handler)
- [x] `restart_required` boolean in PUT responses (`server` and `auth` return true; all others false)
- [x] Secret field redaction in GET responses (N/A — config sections store credential names/references, not plaintext secrets; actual secrets live under `credentials/` keys with `secret=true`)
- [x] Adapt credential API endpoints to `config_store` backend (already wired in `main.go` via `CredentialStoreAdapter`; handlers use `r.credentialStore` which is the adapter)
- [x] Handler tests for each endpoint (success, validation errors, method-not-allowed)

## Phase 3: Admin UI Pages

- [ ] Settings navigation group in sidebar
- [ ] Organisations page (table layout, credential dropdown, add/remove rows)
- [ ] Collection page (schedule, thresholds)
- [ ] Target Versions page (tag-style input, semver validation)
- [ ] Git URLs page
- [ ] Concurrency page (number inputs with min 1)
- [ ] Analysis Tools page
- [ ] Server & TLS page (mode-dependent fields, restart-required banner)
- [ ] Authentication page
- [ ] Notifications page
- [ ] Logging page
- [ ] Exports page
- [ ] Setup wizard for empty `config_store`
- [ ] Component tests for each page

## Phase 4: Deprecation

- [ ] Migration to drop `credentials` table (after validated release)
- [ ] Migration to drop `runtime_settings` table (after validated release)
- [ ] Remove YAML config parsing for non-bootstrap fields
- [ ] Remove env var overrides for non-bootstrap fields
- [ ] Remove `runtime_settings` datastore methods
- [ ] Update all specs and documentation