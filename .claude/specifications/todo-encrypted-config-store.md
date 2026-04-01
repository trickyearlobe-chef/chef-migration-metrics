# Encrypted Config Store — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Phase 1: Schema and Store

### Migration

- [ ] Create `0011_config_store.up.sql` — `config_store` table (key TEXT PK, encrypted_value BYTEA, nonce BYTEA, secret BOOLEAN, updated_at TIMESTAMPTZ, updated_by TEXT)
- [ ] Create `0011_config_store.down.sql` — drop `config_store` table
- [ ] Verify migration runs cleanly up and down

### Datastore CRUD

- [ ] `ConfigEntry` struct in `internal/datastore/config_store.go`
- [ ] `GetConfigEntry(ctx, key)` — single row by PK
- [ ] `SetConfigEntry(ctx, entry)` — upsert via INSERT ON CONFLICT
- [ ] `DeleteConfigEntry(ctx, key)` — hard delete, idempotent
- [ ] `ListConfigEntries(ctx)` — all rows ordered by key
- [ ] `ListConfigEntriesByPrefix(ctx, prefix)` — WHERE key LIKE prefix%
- [ ] `CountConfigEntries(ctx)` — row count
- [ ] `ConfigStoreIsEmpty(ctx)` — convenience bool
- [ ] Functional tests for all CRUD methods (`//go:build functional`)

### Encryption Layer

- [ ] New package `internal/configstore/`
- [ ] `Store` struct wrapping `*datastore.DB` + `*secrets.Encryptor`
- [ ] `Get(ctx, key)` — decrypt, return `json.RawMessage`
- [ ] `GetSecret(ctx, key)` — decrypt secret-flagged entries only
- [ ] `Set(ctx, key, value, secret, updatedBy)` — encrypt JSON, generate nonce, upsert
- [ ] `Delete(ctx, key)` — passthrough
- [ ] `List(ctx)` — metadata only (no decrypted values)
- [ ] `ListByPrefix(ctx, prefix)` — metadata only
- [ ] `GetAll(ctx)` — decrypt all non-secret entries for config assembly
- [ ] `IsEmpty(ctx)` — convenience
- [ ] Raw AES-256-GCM with separate nonce column (not hex-encoded string format)
- [ ] AAD bound to key name: `[]byte(key)`
- [ ] Unit tests for encrypt/decrypt round-trip, AAD mismatch, secret flag filtering

### Credential Store Adapter

- [ ] `CredentialStoreAdapter` in `internal/configstore/credential_adapter.go`
- [ ] Implements `secrets.CredentialStore` interface
- [ ] Maps credential name `foo` → config store key `credentials/foo`
- [ ] All entries stored with `secret=true`
- [ ] `Create` — validate, encrypt, set
- [ ] `Get` — decrypt, deserialise to `secrets.Credential`
- [ ] `GetMetadata` — metadata without plaintext
- [ ] `Update` — validate, re-encrypt, set
- [ ] `Delete` — delete with reference check
- [ ] `List` — list by prefix `credentials/`
- [ ] `ListByType` — list by prefix + filter by type
- [ ] `Test` — get + validate
- [ ] `ReferencedBy` — query orgs, auth, notifications for references
- [ ] Unit tests for interface compliance
- [ ] Functional tests for round-trip through real DB

### Legacy Data Migration

- [ ] `MigrateFromLegacy()` in `internal/configstore/migrate.go`
- [ ] Re-encrypt `credentials` rows under new AAD scheme as `credentials/<name>`
- [ ] Migrate `runtime_settings` rows as non-secret entries
- [ ] Idempotent — skip if `config_store` already has entries
- [ ] Log migration counts
- [ ] Unit tests with mock DB
- [ ] Functional test: insert legacy rows, migrate, verify decrypt

### YAML Auto-Migration

- [ ] `MigrateFromYAML()` in `internal/configstore/yaml_migrate.go`
- [ ] Detect full YAML (presence of `organisations` key)
- [ ] Serialise each config section to JSON and encrypt into DB
- [ ] Rename `config.yml` to `config.yml.migrated`
- [ ] Write bootstrap `config.yml` with only `database_url`, `listen_address`, `listen_port`
- [ ] Skip if `config_store` already has entries (log warning)
- [ ] Unit tests: migration, idempotency, file rename, bootstrap content

### Config Assembly

- [ ] `AssembleConfig()` in `internal/configstore/assembly.go`
- [ ] Decrypt all non-secret config entries
- [ ] Unmarshal JSON into `config.Config` fields per key naming convention
- [ ] Apply defaults and validate (same rules as YAML path)
- [ ] Unit tests: assemble from known JSON, validation errors, missing optional sections

### Config Reload

- [ ] `ConfigHolder` in `internal/configstore/reloader.go`
- [ ] `Get()` — read lock, return `*config.Config` pointer
- [ ] `Reload(ctx)` — write lock, re-assemble from DB, swap pointer
- [ ] `Set(cfg)` — write lock, replace (initial load)
- [ ] Unit tests: concurrent read/write safety, reload replaces config

### Startup Integration

- [ ] Update `loadConfig()` to detect bootstrap vs full YAML
- [ ] Add `migrateConfigStore()` phase after migrations and secrets setup
- [ ] Add `assembleConfig()` — load from DB if populated, else YAML (backward compat)
- [ ] Create `ConfigHolder`, pass to collector and HTTP handlers
- [ ] Wire `CredentialStoreAdapter` in place of `DBCredentialStore`
- [ ] Verify existing credential API and UI still work
- [ ] `CMM_CREDENTIAL_ENCRYPTION_KEY` mandatory from first startup

## Phase 2: API Endpoints

- [ ] `GET /api/v1/admin/config/organisations` — list organisations
- [ ] `PUT /api/v1/admin/config/organisations` — replace organisations
- [ ] `GET /api/v1/admin/config/collection` — collection settings
- [ ] `PUT /api/v1/admin/config/collection` — update collection
- [ ] `GET /api/v1/admin/config/target-versions` — target versions
- [ ] `PUT /api/v1/admin/config/target-versions` — update target versions
- [ ] `GET /api/v1/admin/config/git-urls` — git base URLs
- [ ] `PUT /api/v1/admin/config/git-urls` — update git URLs
- [ ] `GET /api/v1/admin/config/concurrency` — concurrency settings
- [ ] `PUT /api/v1/admin/config/concurrency` — update concurrency
- [ ] `GET /api/v1/admin/config/analysis-tools` — analysis tools config
- [ ] `PUT /api/v1/admin/config/analysis-tools` — update analysis tools
- [ ] `GET /api/v1/admin/config/test-kitchen` — test kitchen config
- [ ] `PUT /api/v1/admin/config/test-kitchen` — update test kitchen
- [ ] `DELETE /api/v1/admin/config/test-kitchen` — reset to defaults
- [ ] `GET /api/v1/admin/config/server` — server settings
- [ ] `PUT /api/v1/admin/config/server` — update server settings
- [ ] `GET /api/v1/admin/config/auth` — auth config
- [ ] `PUT /api/v1/admin/config/auth` — update auth
- [ ] `GET /api/v1/admin/config/notifications` — notifications
- [ ] `PUT /api/v1/admin/config/notifications` — update notifications
- [ ] `GET /api/v1/admin/config/logging` — logging settings
- [ ] `PUT /api/v1/admin/config/logging` — update logging
- [ ] Validation per section (reuse existing `Validate` methods)
- [ ] `restart_required` boolean in PUT responses
- [ ] Secret field redaction in GET responses
- [ ] Adapt credential API endpoints to `config_store` backend
- [ ] Handler tests for each endpoint (success, validation errors, auth)

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