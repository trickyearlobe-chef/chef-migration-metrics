# Secrets Storage — ToDo

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
- [ ] Warn if keys directory permissions > `0700`
- [ ] Warn if env file permissions > `0640` (RPM/DEB)
- [ ] Write unit tests for startup validation (all pass, various failure modes)

## Consumer Integration

- [ ] Update `internal/chefapi/` to resolve Chef API keys via `CredentialResolver`
- [ ] Update `internal/auth/` LDAP provider to resolve bind password via `CredentialResolver`
- [ ] *(deferred — `internal/notify/` not yet implemented)* Update SMTP sender to resolve password via `CredentialResolver`
- [ ] *(deferred — `internal/notify/` not yet implemented)* Update webhook sender to resolve URL via `CredentialResolver`
- [ ] Verify plaintext is zeroed after use in all consumer call sites
- [ ] Write integration tests for Chef API signing with each credential source
- [ ] Write integration tests for LDAP bind with each credential source
- [ ] Write integration tests for SMTP auth with each credential source

## Configuration Integration

- [ ] Add `client_key_env` field to organisation config schema
- [ ] Add `password_credential` field to SMTP config schema
- [ ] *(deferred — `internal/notify/` not yet implemented)* Add `url_credential` field to notification channel config schema

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

- [ ] Verify RPM `postinstall.sh` sets `/etc/chef-migration-metrics/keys/` to `0700`
- [ ] Verify RPM `postinstall.sh` sets env file to `0640`
- [ ] Verify DEB `postinstall.sh` sets `/etc/chef-migration-metrics/keys/` to `0700`
- [ ] Verify DEB `postinstall.sh` sets env file to `0640`
- [ ] Add `CMM_CREDENTIAL_ENCRYPTION_KEY=` placeholder to `deploy/pkg/env-file`
- [ ] Add `CMM_CREDENTIAL_ENCRYPTION_KEY=` placeholder to `deploy/docker-compose/.env.example`
- [ ] Document key generation command in `deploy/docker-compose/README.md`

## Documentation

- [ ] Add secrets management section to top-level `README.md`
- [ ] Document credential storage options and trade-offs
- [ ] Document master key generation procedure
- [ ] Document master key rotation procedure
- [ ] Document credential value rotation procedure
- [ ] Document Kubernetes External Secrets Operator integration pattern
- [ ] Document RPM/DEB credential file setup