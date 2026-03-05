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

- [ ] Implement `internal/secrets/encryption.go` — AES-256-GCM encrypt function with HKDF-SHA256 key derivation
- [ ] Implement AES-256-GCM decrypt function with HKDF-SHA256 key derivation
- [ ] Implement 12-byte random nonce generation per encryption operation
- [ ] Implement AAD construction (`<credential_type>:<name>`) for ciphertext binding
- [ ] Implement at-rest format serialisation (`<nonce_hex>:<ciphertext_hex>`)
- [ ] Implement at-rest format deserialisation
- [ ] Validate master key length ≥ 32 bytes after Base64 decode
- [ ] Write unit tests for encrypt/decrypt round-trip
- [ ] Write unit tests for AAD mismatch detection (row-swap prevention)
- [ ] Write unit tests for tampered ciphertext detection (GCM auth tag)
- [ ] Write unit tests for nonce uniqueness (identical plaintext → different ciphertext)
- [ ] Write unit tests for invalid master key rejection

## Memory Zeroing

- [ ] Implement `internal/secrets/zeroing.go` — `zeroBytes([]byte)` helper
- [ ] Implement `zeroString` helper (convert to byte slice, zero, confirm)
- [ ] Ensure all credential decrypt paths zero plaintext after use
- [ ] Write unit tests for zeroing helpers

## Credential Store

- [ ] Define `CredentialStore` interface in `internal/secrets/store.go`
- [ ] Implement database-backed `CredentialStore` (CRUD operations on `credentials` table)
- [ ] Implement `Create` — validate, encrypt, insert row
- [ ] Implement `Get` — read row, decrypt, return plaintext (caller must zero after use)
- [ ] Implement `GetMetadata` — read row without decrypting (for list/detail endpoints)
- [ ] Implement `Update` — validate new value, re-encrypt, overwrite row, update `last_rotated_at`
- [ ] Implement `Delete` — hard-delete row (check references first)
- [ ] Implement `List` — list all credentials (metadata only, never values)
- [ ] Implement `ListByType` — filter by `credential_type`
- [ ] Implement `Test` — decrypt and perform type-specific validation
- [ ] Implement `ReferencedBy` — return list of entities referencing a credential
- [ ] Write unit tests for all `CredentialStore` methods (using test database)
- [ ] Write unit tests for reference check preventing deletion of in-use credentials

## Credential Resolution

- [ ] Implement `internal/secrets/resolver.go` — `CredentialResolver` with precedence logic
- [ ] Implement database credential resolution (lookup by `client_key_credential_id` FK)
- [ ] Implement environment variable credential resolution
- [ ] Implement file path credential resolution
- [ ] Implement precedence chain: database → env var → file path
- [ ] Return descriptive error when no credential source is configured for a given entity
- [ ] Write unit tests for each resolution method in isolation
- [ ] Write unit tests for precedence ordering (database wins over env var wins over file)
- [ ] Write unit tests for fallback when higher-precedence source is not configured
- [ ] Write unit tests for error when no source is configured

## Credential Validation

- [ ] Implement `internal/secrets/validation.go` — per-type validation functions
- [ ] Implement `chef_client_key` validation: PEM-encoded RSA private key parsing, extract key size
- [ ] Implement `ldap_bind_password` validation: non-empty string
- [ ] Implement `smtp_password` validation: non-empty string
- [ ] Implement `webhook_url` validation: valid URL with `http` or `https` scheme
- [ ] Implement `generic` validation: non-empty string
- [ ] Write unit tests for each credential type validation (valid and invalid inputs)
- [ ] Write unit tests for RSA key size extraction into metadata

## Credential Testing

- [ ] Implement `chef_client_key` test: parse RSA key, verify signature; optionally test Chef API call
- [ ] Implement `ldap_bind_password` test: attempt LDAP bind with configured settings
- [ ] Implement `smtp_password` test: attempt SMTP AUTH handshake with configured settings
- [ ] Implement `webhook_url` test: send HTTP HEAD request, verify 2xx/3xx response
- [ ] Implement `generic` test: verify credential can be decrypted (master key correctness check)
- [ ] Write unit tests for credential test functions (with mocked external services)

## Master Key Rotation

- [ ] Implement `internal/secrets/rotation.go` — master key rotation on startup
- [ ] Detect when both `CMM_CREDENTIAL_ENCRYPTION_KEY` and `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` are set
- [ ] Iterate all `credentials` rows and attempt decrypt with new key first, then old key
- [ ] Re-encrypt each row with the new key in its own transaction (atomic per row)
- [ ] Update `encrypted_value` and `updated_at` for re-encrypted rows
- [ ] Log `INFO` with count of re-encrypted credentials on completion
- [ ] Log `ERROR` for each credential that cannot be decrypted with either key
- [ ] Mark credentials that fail decryption as unusable (continue startup)
- [ ] Handle crash-recovery: rows already re-encrypted use new key, remaining use old key
- [ ] Write unit tests for successful key rotation (all rows re-encrypted)
- [ ] Write unit tests for partial rotation (some rows fail decryption)
- [ ] Write unit tests for crash-recovery scenario
- [ ] Write unit tests for no-op when only new key is set and all rows already use it

## Startup Validation

- [ ] Check `CMM_CREDENTIAL_ENCRYPTION_KEY` is set when DB credentials exist
- [ ] Validate master key length after Base64 decode (≥ 32 bytes)
- [ ] Refuse to start if master key is required but missing or invalid
- [ ] Validate each DB credential can be decrypted (log `ERROR` per failure, continue startup)
- [ ] Validate Chef API key files exist and are readable (log `ERROR` per org, skip org, continue)
- [ ] Warn if Chef API key file permissions > `0600`
- [ ] Warn if TLS key file permissions > `0600` (static mode)
- [ ] Warn if keys directory permissions > `0700`
- [ ] Warn if env file permissions > `0640` (RPM/DEB)
- [ ] Write unit tests for startup validation (all pass, various failure modes)

## Web API Integration

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

- [ ] Update `internal/chefapi/` to resolve Chef API keys via `CredentialResolver`
- [ ] Update `internal/auth/` LDAP provider to resolve bind password via `CredentialResolver`
- [ ] Update `internal/notify/` SMTP sender to resolve password via `CredentialResolver`
- [ ] Update `internal/notify/` webhook sender to resolve URL via `CredentialResolver`
- [ ] Verify plaintext is zeroed after use in all consumer call sites
- [ ] Write integration tests for Chef API signing with each credential source
- [ ] Write integration tests for LDAP bind with each credential source
- [ ] Write integration tests for SMTP auth with each credential source

## Configuration Integration

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

- [ ] Add `credential_storage` section to `GET /api/v1/admin/status` response
- [ ] Report `encryption_key_configured` boolean
- [ ] Report `total_credentials` count
- [ ] Report `credential_types` breakdown
- [ ] Report `orphaned_credentials` count (credentials not referenced by any entity)
- [ ] Write tests for status endpoint credential storage section

## Logging

- [ ] Add `secrets` log scope to logging specification and implementation
- [ ] Log credential create/rotate/delete/test operations at `INFO`
- [ ] Log failed decryption attempts at `ERROR`
- [ ] Log master key rotation start/completion at `INFO`
- [ ] Log file permission warnings at `WARN`
- [ ] Verify no log statement includes credential plaintext, ciphertext, or encoded values
- [ ] Write tests to confirm no plaintext leaks into log output

## Packaging

- [ ] Verify `.gitignore` includes `*.pem`, `*.key`, `.env`, `keys/` patterns
- [ ] Verify `.dockerignore` includes `*.pem`, `*.key`, `.env`, `keys/` patterns
- [ ] Verify `.helmignore` includes `*.pem`, `*.key`, `.env`, `keys/` patterns
- [ ] Verify RPM `postinstall.sh` sets `/etc/chef-migration-metrics/keys/` to `0700`
- [ ] Verify RPM `postinstall.sh` sets env file to `0640`
- [ ] Verify DEB `postinstall.sh` sets `/etc/chef-migration-metrics/keys/` to `0700`
- [ ] Verify DEB `postinstall.sh` sets env file to `0640`
- [ ] Add `CMM_CREDENTIAL_ENCRYPTION_KEY=` placeholder to `deploy/pkg/env-file`
- [ ] Add `CMM_CREDENTIAL_ENCRYPTION_KEY=` placeholder to Docker Compose `.env.example`
- [ ] Document key generation command in Helm chart `README.md`
- [ ] Document key generation command in Docker Compose `README.md`

## Documentation

- [ ] Add secrets management section to top-level `README.md`
- [ ] Document credential storage options and trade-offs
- [ ] Document master key generation procedure
- [ ] Document master key rotation procedure
- [ ] Document credential value rotation procedure
- [ ] Document Kubernetes External Secrets Operator integration pattern
- [ ] Document RPM/DEB credential file setup