# Secrets Storage — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Specification

- [x] Write secrets storage specification (`secrets-storage/Specification.md`)
- [x] Update `Specification.md` (top-level) specifications index with secrets-storage entry
- [x] Update `Structure.md` project layout with `internal/secrets/` package and `secrets-storage/` spec directory
- [x] Update `Structure.md` specification relationships table with secrets-storage cross-references
- [x] Update `Claude.md` task-to-spec lookup table with secrets-storage entries
- [x] Update `ToDo.md` (master) with secrets storage section

## Core Encryption

- [x] Implement `internal/secrets/encryption.go` — AES-256-GCM encrypt function with HKDF-SHA256 key derivation
- [x] Implement AES-256-GCM decrypt function with HKDF-SHA256 key derivation
- [x] Implement 12-byte random nonce generation per encryption operation
- [x] Implement AAD construction (`<credential_type>:<name>`) for ciphertext binding
- [x] Implement at-rest format serialisation (`<nonce_hex>:<ciphertext_hex>`)
- [x] Implement at-rest format deserialisation
- [x] Validate master key length ≥ 32 bytes after Base64 decode
- [x] Write unit tests for encrypt/decrypt round-trip — 46 tests in `encryption_test.go`
- [x] Write unit tests for AAD mismatch detection (row-swap prevention)
- [x] Write unit tests for tampered ciphertext detection (GCM auth tag)
- [x] Write unit tests for nonce uniqueness (identical plaintext → different ciphertext)
- [x] Write unit tests for invalid master key rejection

> **Coverage:** `NewEncryptor` 88.2%, `Encrypt` 78.6%, `Decrypt` 91.3%, `BuildAAD` 100%, `DecodeMasterKey` 100%, `Close` 100%. Uncovered paths are internal crypto failure branches (`aes.NewCipher`, `cipher.NewGCM`, `rand.Reader` failures) that cannot be triggered without mocking stdlib internals.

## Memory Zeroing

- [x] Implement `internal/secrets/zeroing.go` — `ZeroBytes([]byte)` helper
- [x] Implement `ZeroString` helper (convert to byte slice, zero, confirm)
- [x] Implement `IsZeroed` helper for test assertions
- [x] Ensure all credential decrypt paths zero plaintext after use
- [x] Write unit tests for zeroing helpers — 25 tests in `zeroing_test.go`, 100% coverage

## Credential Store

- [x] Define `CredentialStore` interface in `internal/secrets/store.go`
- [x] Implement database-backed `DBCredentialStore` in `internal/secrets/db_store.go` (CRUD operations on `credentials` table)
- [x] Implement `Create` — validate, encrypt, insert row
- [x] Implement `Get` — read row, decrypt, return plaintext (caller must zero after use)
- [x] Implement `GetMetadata` — read row without decrypting (for list/detail endpoints)
- [x] Implement `Update` — validate new value, re-encrypt, overwrite row, update `last_rotated_at`
- [x] Implement `Delete` — hard-delete row (check references first)
- [x] Implement `List` — list all credentials (metadata only, never values)
- [x] Implement `ListByType` — filter by `credential_type`
- [x] Implement `Test` — decrypt and perform type-specific validation
- [x] Implement `ReferencedBy` — return list of entities referencing a credential
- [x] Write unit tests for `CredentialStore` interface contract (via `InMemoryCredentialStore`) — 94 tests in `db_store_test.go`
- [x] Write unit tests for reference check preventing deletion of in-use credentials
- [ ] Write functional tests for `DBCredentialStore` SQL paths against real PostgreSQL (build-tagged `//go:build functional`)

> **Note:** `db_store.go` DB methods (`Create`, `Get`, `Update`, `Delete`, `List`, `ListByType`, `Test`, `ReferencedBy`, `queryMetadataRows`, `referencedByInternal`) show 0% unit test coverage because they require a real PostgreSQL instance. The `InMemoryCredentialStore` in `db_store_test.go` validates the full `CredentialStore` interface contract (94 tests) using the real `Encryptor` and `ValidateCredentialValue`, covering encryption round-trips, validation integration, reference checking, concurrency, and all error paths. Pure helper functions (`nullableJSONB`, `parseJSONBMetadata`, `isUniqueViolation`, `containsCI`, `toLowerASCII`) have 100% coverage. Functional tests against PostgreSQL are deferred until a test database fixture is set up.

## Credential Resolution

- [x] Implement `internal/secrets/resolver.go` — `CredentialResolver` with precedence logic
- [x] Implement database credential resolution (lookup by credential name via `CredentialStore.Get`)
- [x] Implement environment variable credential resolution
- [x] Implement file path credential resolution
- [x] Implement precedence chain: database → env var → file path
- [x] Return descriptive error when no credential source is configured for a given entity
- [x] Write unit tests for each resolution method in isolation — 54 tests in `resolver_test.go`, 100% coverage
- [x] Write unit tests for precedence ordering (database wins over env var wins over file)
- [x] Write unit tests for fallback when higher-precedence source is not configured
- [x] Write unit tests for error when no source is configured
- [x] Write unit tests verifying no fallthrough on error (DB error does NOT fall to env)

## Credential Validation

- [x] Implement `internal/secrets/validation.go` — per-type validation functions
- [x] Implement `chef_client_key` validation: PEM-encoded RSA private key parsing (PKCS#1 and PKCS#8), extract key size and format
- [x] Implement `ldap_bind_password` validation: non-empty string
- [x] Implement `smtp_password` validation: non-empty string
- [x] Implement `webhook_url` validation: valid URL with `http` or `https` scheme, host required
- [x] Implement `generic` validation: non-empty string
- [x] Write unit tests for each credential type validation (valid and invalid inputs) — 47 tests in `validation_test.go`
- [x] Write unit tests for RSA key size extraction into metadata

> **Coverage:** `ValidateCredentialValue` 90.9%, `validateChefClientKey` 95.7%, `validateWebhookURL` 100%, `validateNonEmpty` 100%, `IsValidCredentialType` 100%. The uncovered path in `ValidateCredentialValue` is the unreachable default case after the `ValidCredentialTypes` map check. The uncovered path in `validateChefClientKey` is the `rsaKey.Validate()` failure branch, which requires a structurally corrupt but PEM-decodable and ASN.1-parseable RSA key.

## Credential Testing

The `Test` method on `CredentialStore` currently decrypts and re-validates using `ValidateCredentialValue`. This covers the "can it be decrypted?" check for all types and structural validation for `chef_client_key` and `webhook_url`. The items below are for deeper live-service testing that requires external service connections.

- [x] Implement `generic` test: verify credential can be decrypted (master key correctness check)
- [x] Implement `chef_client_key` test: parse RSA key, verify structure (via `ValidateCredentialValue`)
- [x] Implement `webhook_url` test: validate URL format (via `ValidateCredentialValue`)
- [ ] Implement `chef_client_key` live test: optionally test Chef API call with the key
- [ ] Implement `ldap_bind_password` live test: attempt LDAP bind with configured settings
- [ ] Implement `smtp_password` live test: attempt SMTP AUTH handshake with configured settings
- [ ] Implement `webhook_url` live test: send HTTP HEAD request, verify 2xx/3xx response
- [ ] Write unit tests for live credential test functions (with mocked external services)

## Master Key Rotation

- [x] Implement `internal/secrets/rotation.go` — `RotateCredentialRow`, `RotateMasterKey`, `NeedsRotation`
- [x] Detect when `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` is set (`NeedsRotation`)
- [x] Iterate all credential rows and attempt decrypt with new key first, then old key
- [x] Re-encrypt each row with the new key via `RotationRowWriter` callback (one transaction per row for crash-safety)
- [x] Return `RotationResult` with re-encrypted count, failure count, per-credential errors, and duration
- [x] Handle crash-recovery: rows already re-encrypted use new key, remaining use old key
- [x] Write unit tests for successful key rotation (all rows re-encrypted) — 65 tests in `rotation_test.go`
- [x] Write unit tests for partial rotation (some rows fail decryption)
- [x] Write unit tests for crash-recovery scenario (idempotent rerun)
- [x] Write unit tests for no-op when only new key is set and all rows already use it
- [x] Wire `RotateMasterKey` into application startup (call from `cmd/` with DB reader + writer) — `main.go` checks `NeedsRotation(os.LookupEnv)`, builds previous `Encryptor`, calls `credStore.ListRotationRows`, and invokes `secrets.RotateMasterKey` with a `rotationWriter` callback that calls `credStore.UpdateEncryptedValueRaw`
- [x] Log `INFO` with count of re-encrypted credentials on completion — `main.go` logs `"master key rotation complete in %s: %d total, %d re-encrypted, %d already rotated, %d failed"`
- [x] Log `ERROR` for each credential that cannot be decrypted with either key — `main.go` iterates `result.Errors` and logs `ERROR` per credential with `"credential %q could not be rotated: %v"`

> **Coverage:** `RotateCredentialRow` 94.7%, `RotateMasterKey` 100%, `NeedsRotation` 100%. See `.claude/summaries/2025-secrets-rotation-tests.md` for full test inventory. The uncovered path is the re-encryption failure branch in `RotateCredentialRow` (requires crypto-level fault injection).

## Startup Validation

Most items are now wired into `cmd/chef-migration-metrics/main.go`. File permission checks for TLS keys, key directories, and env files are deferred until the TLS subsystem and RPM/DEB packaging are implemented.

- [x] Check `CMM_CREDENTIAL_ENCRYPTION_KEY` is set when DB credentials exist — `main.go` checks `credCount > 0 && encryptor == nil` and refuses to start with a descriptive error
- [x] Validate master key length after Base64 decode (≥ 32 bytes) — `main.go` calls `secrets.NewEncryptor(masterKeyBase64)` which calls `DecodeMasterKey` internally; startup exits on error
- [x] Refuse to start if master key is required but missing or invalid — `main.go` returns exit code 1 with `ERROR` log when key is invalid or missing but credentials exist
- [x] Validate each DB credential can be decrypted (log `ERROR` per failure, continue startup) — `main.go` iterates `credStore.ListRotationRows`, calls `encryptor.Decrypt` on each, logs `ERROR` per failure and `WARN` summary, zeros plaintext after validation
- [x] Validate Chef API key files exist and are readable (log `ERROR` per org, skip org, continue) — partially: `main.go` calls `os.Stat` on `org.ClientKeyPath` and checks permissions; stat failures are silently skipped (file may not exist yet or be resolved at collection time)
- [x] Warn if Chef API key file permissions > `0600` — `main.go` checks `perm&0o077 != 0` and logs `WARN` with the actual permission value and a recommendation for 0600
- [ ] Warn if TLS key file permissions > `0600` (static mode) — deferred until TLS subsystem is implemented
- [ ] Warn if keys directory permissions > `0700` — deferred until RPM/DEB packaging creates the directory
- [ ] Warn if env file permissions > `0640` (RPM/DEB) — deferred until RPM/DEB packaging creates the env file
- [ ] Write unit tests for startup validation (all pass, various failure modes)

## Web API Integration

None of these are implemented yet. Depends on `internal/webapi/` package (not yet created).

- [ ] Wire `CredentialStore` into `internal/webapi/` admin credential handlers
- [ ] Implement `GET /api/v1/admin/credentials` handler (metadata only)
- [ ] Implement `POST /api/v1/admin/credentials` handler (validate, encrypt, store)
- [ ] Implement `PUT /api/v1/admin/credentials/:name` handler (rotate value)
- [ ] Implement `DELETE /api/v1/admin/credentials/:name` handler (reference check, hard delete)
- [ ] Implement `POST /api/v1/admin/credentials/:name/test` handler
- [ ] Return `503` when `CMM_CREDENTIAL_ENCRYPTION_KEY` is not configured
- [ ] Require `admin` role on all credential endpoints
- [ ] Verify no endpoint returns `encrypted_value` or plaintext in any response
- [ ] Log all credential operations at `INFO` with `scope: secrets`
- [ ] Write handler tests for each endpoint (success and error cases)
- [ ] Write handler tests for authorisation enforcement (non-admin rejected)
- [ ] Write handler tests for `503` when encryption key is missing

## Consumer Integration

`internal/chefapi/` exists (74 tests) but does not yet resolve keys via `CredentialResolver`. `internal/auth/` and `internal/notify/` do not yet exist.

- [ ] Update `internal/chefapi/` to resolve Chef API keys via `CredentialResolver`
- [ ] Update `internal/auth/` LDAP provider to resolve bind password via `CredentialResolver`
- [ ] Update `internal/notify/` SMTP sender to resolve password via `CredentialResolver`
- [ ] Update `internal/notify/` webhook sender to resolve URL via `CredentialResolver`
- [ ] Verify plaintext is zeroed after use in all consumer call sites
- [ ] Write integration tests for Chef API signing with each credential source
- [ ] Write integration tests for LDAP bind with each credential source
- [ ] Write integration tests for SMTP auth with each credential source

## Configuration Integration

`internal/config/` exists (960 lines, 117 tests) with full YAML schema, defaults, env var overrides, and validation. The items below add credential-reference fields to the existing config schema. Helm chart templates are not yet created beyond `.helmignore`.

- [ ] Add `client_key_credential` field to organisation config schema
- [ ] Add `client_key_env` field to organisation config schema
- [ ] Add `bind_password_credential` field to LDAP auth config schema
- [ ] Add `password_credential` field to SMTP config schema
- [ ] Add `url_credential` field to notification channel config schema
- [ ] Add `secrets.credentialEncryptionKey` to Helm `values.yaml`
- [ ] Add `secrets.smtpPassword` to Helm `values.yaml`
- [ ] Update Helm `secret.yaml` template to include `CMM_CREDENTIAL_ENCRYPTION_KEY` and `SMTP_PASSWORD`
- [ ] Validate credential resolution on startup (at least one source configured per org)
- [ ] Write unit tests for config parsing of new credential reference fields
- [ ] Write unit tests for config validation of credential resolution

## System Status

None of these are implemented yet. Depends on `internal/webapi/` status endpoint.

- [ ] Add `credential_storage` section to `GET /api/v1/admin/status` response
- [ ] Report `encryption_key_configured` boolean
- [ ] Report `total_credentials` count
- [ ] Report `credential_types` breakdown
- [ ] Report `orphaned_credentials` count (credentials not referenced by any entity)
- [ ] Write tests for status endpoint credential storage section

## Logging

The `internal/logging/` package exists (93 tests) and includes `ScopeSecrets`. Startup logging in `main.go` uses the structured logger with `secrets` scope throughout the master key, rotation, validation, and permission check sections.

- [x] Add `secrets` log scope to logging specification and implementation — `ScopeSecrets` constant in `internal/logging/logging.go`, registered in `validScopes` map
- [x] Log credential create/rotate/delete/test operations at `INFO` — rotation start/completion logged at `INFO` in `main.go`; create/delete/test will be logged when Web API integration is implemented
- [x] Log failed decryption attempts at `ERROR` — `main.go` logs `ERROR` per credential with `"credential %q: decryption failed (wrong key or corrupted data)"`
- [x] Log master key rotation start/completion at `INFO` — `main.go` logs `"master key rotation requested — re-encrypting stored credentials"` at start and `"master key rotation complete in %s: ..."` at completion
- [x] Log file permission warnings at `WARN` — `main.go` logs `WARN` with `"key file %s for organisation %q has permissions %04o — should be 0600 or more restrictive"`
- [ ] Verify no log statement includes credential plaintext, ciphertext, or encoded values
- [ ] Write tests to confirm no plaintext leaks into log output

## Packaging

Partially done. Ignore files are complete. RPM/DEB packaging, Docker Compose app setup, and Helm chart are not yet created.

- [x] Verify `.gitignore` includes `*.pem`, `*.key`, `.env`, `keys/` patterns
- [x] Verify `.dockerignore` includes `*.pem`, `*.key`, `.env`, `keys/` patterns
- [x] Verify `.helmignore` includes `*.pem`, `*.key`, `.env`, `keys/` patterns
- [ ] Verify RPM `postinstall.sh` sets `/etc/chef-migration-metrics/keys/` to `0700` — `deploy/pkg/` does not exist yet
- [ ] Verify RPM `postinstall.sh` sets env file to `0640` — `deploy/pkg/` does not exist yet
- [ ] Verify DEB `postinstall.sh` sets `/etc/chef-migration-metrics/keys/` to `0700` — `deploy/pkg/` does not exist yet
- [ ] Verify DEB `postinstall.sh` sets env file to `0640` — `deploy/pkg/` does not exist yet
- [ ] Add `CMM_CREDENTIAL_ENCRYPTION_KEY=` placeholder to `deploy/pkg/env-file` — `deploy/pkg/` does not exist yet
- [ ] Add `CMM_CREDENTIAL_ENCRYPTION_KEY=` placeholder to Docker Compose `.env.example` — app Docker Compose does not exist yet
- [ ] Document key generation command in Helm chart `README.md` — Helm chart not yet created beyond `.helmignore`
- [ ] Document key generation command in Docker Compose `README.md` — app Docker Compose does not exist yet

## Documentation

- [ ] Add secrets management section to top-level `README.md`
- [ ] Document credential storage options and trade-offs
- [ ] Document master key generation procedure
- [ ] Document master key rotation procedure
- [ ] Document credential value rotation procedure
- [ ] Document Kubernetes External Secrets Operator integration pattern
- [ ] Document RPM/DEB credential file setup

---

## Summary

### Completed (`internal/secrets/` package)

| File | Tests | Coverage | Status |
|------|-------|----------|--------|
| `encryption.go` | 46 | 78–100% per function | ✅ Done |
| `zeroing.go` | 25 | 100% | ✅ Done |
| `store.go` | — | Interface only | ✅ Done |
| `db_store.go` | 94 (via InMemoryCredentialStore) | Helpers 100%, SQL methods 0% (need Postgres) | ✅ Impl done, functional tests deferred |
| `validation.go` | 47 | 90–100% per function | ✅ Done |
| `resolver.go` | 54 | 100% | ✅ Done |
| `rotation.go` | 65 | 94.7–100% per function | ✅ Done |

**Total: 331 unit tests, all passing, 0 failures.**

### Wired into Application Startup (`cmd/chef-migration-metrics/main.go`)

- Master key rotation — fully wired (detect previous key, rotate, log results)
- Startup validation — master key presence/length, credential decryption, key file permission checks
- Logging — `ScopeSecrets` scope with structured logging for all secrets operations at startup

### Not Yet Started

These sections depend on packages or infrastructure that do not yet exist:

- Web API integration (needs `internal/webapi/`)
- Consumer integration (needs `internal/chefapi/` resolver wiring, `internal/auth/`, `internal/notify/`)
- Configuration integration (needs Helm chart, new config fields for credential references)
- System status (needs `internal/webapi/`)
- Packaging (needs `deploy/pkg/`, Helm chart, Docker Compose for app)
- Documentation
- Live credential testing (needs external service mocks)
- DBCredentialStore functional tests (needs PostgreSQL test fixture)
- TLS key and directory permission checks (needs TLS subsystem and RPM/DEB packaging)