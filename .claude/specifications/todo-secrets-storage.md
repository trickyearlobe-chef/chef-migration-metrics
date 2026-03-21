# Secrets Storage — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Credential Store

- [ ] Write functional tests for `DBCredentialStore` SQL paths against real PostgreSQL (build-tagged `//go:build functional`)

## Credential Testing

- [ ] Implement `chef_client_key` live test: optionally test Chef API call with the key
- [ ] Implement `ldap_bind_password` live test: attempt LDAP bind with configured settings
- [ ] Implement `smtp_password` live test: attempt SMTP AUTH handshake with configured settings
- [ ] Implement `webhook_url` live test: send HTTP HEAD request, verify 2xx/3xx response
- [ ] Write unit tests for live credential test functions (with mocked external services)

## Startup Validation

- [ ] Warn if TLS key file permissions > `0600` (static mode) — deferred until TLS subsystem is implemented
- [ ] Warn if keys directory permissions > `0700` — deferred until RPM/DEB packaging creates the directory
- [ ] Warn if env file permissions > `0640` (RPM/DEB) — deferred until RPM/DEB packaging creates the env file
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

- [ ] Verify no log statement includes credential plaintext, ciphertext, or encoded values
- [ ] Write tests to confirm no plaintext leaks into log output

## Packaging

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