# Plan: Encrypted Config Store — Phase 1

## Goal

Replace the YAML config file with an encrypted database-backed config store. All settings are AES-256-GCM encrypted in a single `config_store` table. The YAML file becomes a bootstrap file containing only `database_url`, `listen_address`, `listen_port`. Existing `credentials` and `runtime_settings` tables are consolidated into `config_store`.

## Specs to Read

- `.claude/specifications/encrypted-config-store.md` — primary spec (read in full)
- `.claude/specifications/configuration.md` — current YAML schema (key naming, struct mapping)
- `.claude/specifications/secrets-storage.md` — encryption model, credential store interface
- `.claude/specifications/project-conventions.md` — migration naming, Go patterns, test conventions

## Existing Code to Understand

- `internal/config/config.go` — Config struct and all nested types, `Load()`, `Parse()`, `Validate()`
- `internal/secrets/encryption.go` — `Encryptor`, `Encrypt()`, `Decrypt()`, `BuildAAD()`
- `internal/secrets/db_store.go` — `DBCredentialStore` CRUD (SQL patterns to adapt)
- `internal/secrets/store.go` — `CredentialStore` interface, types
- `internal/secrets/resolver.go` — `CredentialResolver` (must keep working, change backing store)
- `internal/datastore/runtime_settings.go` — `RuntimeSetting` CRUD (being replaced)
- `cmd/chef-migration-metrics/main.go` — startup sequence (`run()`, `loadConfig()`, `setupSecrets()`)
- `migrations/0001_initial_schema.up.sql` — `credentials` table schema (UUID PK, `encrypted_value` TEXT, `metadata` JSONB)

## Key Design Constraints

- `config_store` uses TEXT primary key (`key` column), not UUID
- `encrypted_value` is BYTEA (not TEXT like current `credentials` table) — spec says BYTEA, but existing encryption returns hex-encoded string, so keep TEXT for compatibility
- `nonce` is separate BYTEA column (12 bytes) — **NOTE:** current encryption packs nonce into the encrypted_value as `<nonce_hex>:<ciphertext_hex>`. The spec says separate columns. Follow the spec — this means the config store encrypt/decrypt uses separate nonce storage, not the `Encryptor.Encrypt()` string format. Write new encrypt/decrypt helpers that work with raw bytes.
- `secret` BOOLEAN controls API read behaviour
- AAD binds to the key name (not credential_type:name like current scheme)
- Credential migration must re-encrypt under new AAD scheme

## Ordered Steps

### Step 1: Migration `0011_config_store`

**Files:** `migrations/0011_config_store.up.sql`, `migrations/0011_config_store.down.sql`

- Create `config_store` table per spec schema (key TEXT PK, encrypted_value BYTEA, nonce BYTEA, secret BOOLEAN, updated_at TIMESTAMPTZ, updated_by TEXT)
- **Do NOT migrate data in SQL** — credential re-encryption requires the app-layer encryption key, which SQL doesn't have. The migration only creates the table.
- Do NOT drop `credentials` or `runtime_settings` yet — data migration happens in Go at startup, tables are dropped in a later migration after verification.

**Tests:** Migration up/down runs cleanly (verified by existing migration test infra).

### Step 2: Config Store Datastore CRUD

**Files:** `internal/datastore/config_store.go`, `internal/datastore/config_store_test.go`

Types:
- `ConfigEntry` struct: `Key`, `EncryptedValue []byte`, `Nonce []byte`, `Secret bool`, `UpdatedAt`, `UpdatedBy`

Methods on `*DB`:
- `GetConfigEntry(ctx, key) (*ConfigEntry, error)` — single row by PK
- `SetConfigEntry(ctx, entry *ConfigEntry) error` — upsert (INSERT ON CONFLICT UPDATE)
- `DeleteConfigEntry(ctx, key) error` — hard delete, idempotent
- `ListConfigEntries(ctx) ([]ConfigEntry, error)` — all rows ordered by key
- `ListConfigEntriesByPrefix(ctx, prefix) ([]ConfigEntry, error)` — `WHERE key LIKE prefix || '%'`
- `CountConfigEntries(ctx) (int, error)` — count for startup checks
- `ConfigStoreIsEmpty(ctx) (bool, error)` — convenience

These are raw CRUD — no encryption logic. Encryption is handled one layer up.

**Tests:** Functional tests (`//go:build functional`) following `runtime_settings_test.go` patterns — round-trip, upsert, delete idempotent, list, list-by-prefix, count.

### Step 3: Config Store Encryption Layer

**Files:** `internal/configstore/configstore.go`, `internal/configstore/configstore_test.go`

New package `internal/configstore/` — the high-level encrypt/decrypt layer above raw datastore CRUD.

Type `Store` struct wrapping `*datastore.DB` + `*secrets.Encryptor`.

Methods:
- `Get(ctx, key) (json.RawMessage, error)` — decrypt and return JSON value; returns `ErrNotFound` for missing keys
- `GetSecret(ctx, key) (json.RawMessage, error)` — same but only if `secret=true`
- `Set(ctx, key string, value json.RawMessage, secret bool, updatedBy string) error` — encrypt JSON value, generate nonce, upsert
- `Delete(ctx, key string) error`
- `List(ctx) ([]EntryMetadata, error)` — key, secret flag, updated_at, updated_by (no values)
- `ListByPrefix(ctx, prefix) ([]EntryMetadata, error)`
- `GetAll(ctx) (map[string]json.RawMessage, error)` — decrypt all non-secret entries (for config assembly)
- `IsEmpty(ctx) (bool, error)`

Encryption: Use raw AES-256-GCM from `crypto/cipher` directly (not `Encryptor.Encrypt()` which returns hex string format). Store nonce as raw bytes in the nonce column. AAD = `[]byte(key)` (the config store key, not credential_type:name).

**Tests:** Unit tests with a real `Encryptor` and mock DB interface. Round-trip encrypt/decrypt, AAD mismatch detection, secret flag filtering.

### Step 4: Credential Store Adapter

**Files:** `internal/configstore/credential_adapter.go`, `internal/configstore/credential_adapter_test.go`

Implement `secrets.CredentialStore` interface backed by `configstore.Store`. Maps:
- Credential name `foo` → config store key `credentials/foo`
- Always `secret=true`
- `Create` → `Set` with key `credentials/<name>`, validate first
- `Get` → `GetSecret` with key `credentials/<name>`, deserialise to `Credential`
- `GetMetadata` → read entry metadata without decrypting value (or decrypt and discard plaintext)
- `Update` → `Set` (overwrite)
- `Delete` → `Delete`
- `List` → `ListByPrefix("credentials/")`
- `ListByType` → `ListByPrefix("credentials/")` + filter
- `Test` → `Get` + validate
- `ReferencedBy` → query organisations, auth, etc. (same logic as `DBCredentialStore.referencedByInternal`)

The stored JSON value for credentials: `{"credential_type":"chef_client_key","value":"<base64-plaintext>","metadata":{...},"last_rotated_at":"...","created_by":"...","created_at":"..."}`

**Tests:** Unit tests verifying interface compliance. Functional tests for round-trip through real DB.

### Step 5: Data Migration (Go startup)

**Files:** `internal/configstore/migrate.go`, `internal/configstore/migrate_test.go`

Function `MigrateFromLegacy(ctx, db *datastore.DB, store *Store, encryptor *secrets.Encryptor) error`:
1. Check if `config_store` has any entries — if yes, skip (idempotent)
2. Read all rows from `credentials` table, re-encrypt each under new AAD scheme, insert as `credentials/<name>` with `secret=true`
3. Read all rows from `runtime_settings` table, encrypt each value, insert with original key (e.g. `test_kitchen`) with `secret=false`
4. Log count of migrated credentials and settings

This runs after migrations, before config assembly. Does NOT drop old tables — that's a future migration after the release is validated.

**Tests:** Unit tests with mock DB. Functional test: insert test rows into `credentials` and `runtime_settings`, run migration, verify they appear in `config_store` with correct keys and can be decrypted.

### Step 6: YAML Auto-Migration

**Files:** `internal/configstore/yaml_migrate.go`, `internal/configstore/yaml_migrate_test.go`

Function `MigrateFromYAML(ctx, store *Store, cfg *config.Config, yamlPath string) error`:
1. Check if `config_store` is empty — if not, log warning and return (DB config takes precedence)
2. Serialise each config section to JSON and `Set()` into the store with appropriate key names per spec key naming table
3. Rename `config.yml` to `config.yml.migrated`
4. Write bootstrap `config.yml` with only `database_url`, `listen_address`, `listen_port`
5. Log `INFO: Configuration migrated to database`

Section-to-key mapping per spec: `organisations`, `target_chef_versions`, `git_base_urls`, `collection`, `concurrency`, `analysis_tools`, `readiness`, `server.tls`, `server.websocket`, `server.graceful_shutdown_seconds`, `frontend`, `logging`, `auth`, `notifications`, `smtp`, `exports`, `elasticsearch`, `ownership`

**Tests:** Unit tests: full YAML migration, idempotency, file rename, bootstrap file content. Use temp dirs for file operations.

### Step 7: Config Assembly from DB

**Files:** `internal/configstore/assembly.go`, `internal/configstore/assembly_test.go`

Function `AssembleConfig(ctx, store *Store) (*config.Config, error)`:
1. `GetAll()` to get all decrypted non-secret values
2. For each known key, unmarshal JSON into the corresponding `config.Config` field
3. Call `cfg.Validate()` (same validation as YAML path)
4. Return assembled config

This replaces the YAML `Parse()` path for DB-backed config. Bootstrap values (database_url, listen_address, listen_port) come from the bootstrap file, everything else from the DB.

**Tests:** Unit tests: assemble from known JSON values, validation errors propagated, missing optional sections use defaults.

### Step 8: Config Reload (Pointer Swap)

**Files:** `internal/configstore/reloader.go`, `internal/configstore/reloader_test.go`

Type `ConfigHolder` struct:
- `mu sync.RWMutex`
- `cfg *config.Config`
- `store *Store`

Methods:
- `Get() *config.Config` — read lock, return pointer
- `Reload(ctx) error` — write lock, re-assemble from DB, swap pointer
- `Set(cfg *config.Config)` — write lock, replace (for initial load)

The collector calls `Reload()` at the start of each collection run. HTTP handlers call `Get()`.

**Tests:** Unit tests: concurrent read/write safety, reload replaces config, Get returns latest.

### Step 9: Startup Sequence Integration

**Files:** Modify `cmd/chef-migration-metrics/main.go`

Update `run()` phases:
1. `loadConfig()` — now loads bootstrap config only (detect full vs bootstrap YAML)
2. After `setupDatabase()` and `runMigrations()`:
   a. `setupSecrets()` — create encryptor (same as today)
   b. New: `migrateConfigStore()` — run `MigrateFromLegacy()` then `MigrateFromYAML()` if needed
   c. New: `assembleConfig()` — if `config_store` has entries, assemble from DB; else use YAML config (backward compat during transition)
3. Create `ConfigHolder`, set initial config
4. Pass `ConfigHolder` to collector and HTTP handlers instead of raw `*config.Config`

Update `setupSecrets()`:
- Create `configstore.Store`
- Create `CredentialStoreAdapter` to replace `DBCredentialStore`
- Create `CredentialResolver` with the adapter

**Tests:** Integration test: full startup with bootstrap YAML + populated config_store.

### Step 10: Todo File

**Files:** `.claude/specifications/todo-encrypted-config-store.md`

Create todo tracking all items from this plan plus Phase 2/3/4 items from the spec.

## Acceptance Criteria

- `config_store` table exists after migration 0011
- Config store CRUD: encrypt, decrypt, get, set, delete, list all work with AES-256-GCM
- Credential adapter implements `secrets.CredentialStore` interface — existing credential API and UI work unchanged
- Legacy data migration: `credentials` and `runtime_settings` rows appear in `config_store` with correct keys
- YAML auto-migration: full config.yml → DB, file renamed, bootstrap written
- Config assembly: all config sections load correctly from DB
- Config reload: pointer swap under mutex, concurrent-safe
- Startup sequence: app boots with either full YAML (auto-migrates) or bootstrap YAML + DB config
- `CMM_CREDENTIAL_ENCRYPTION_KEY` is mandatory from first startup
- All tests pass (unit + functional where DB available)
- No existing tests broken

## Out of Scope (Phase 2+)

- API endpoints (GET/PUT per section) — Phase 2
- Admin UI settings pages — Phase 3
- Setup wizard — Phase 3
- Dropping `credentials` and `runtime_settings` tables — future migration after validated release
- Removing YAML parsing for non-bootstrap fields — Phase 4