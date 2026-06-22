# Secrets Storage — ToDo

## Bugs

- [ ] **Operational status page mislabels config-synced orgs' credential source as "file".** `handle_admin_status.go:185-188` derives the source solely from the DB column `ClientKeyCredentialName` (empty → "file", non-empty → "database"). But the startup org sync at `main.go:877-882` never copies `org.ClientKeyCredential` (config) into `UpsertOrganisationParams.ClientKeyCredentialName`, so for **all config-synced orgs** that column is always NULL — the field is effectively hardcoded to "file" regardless of whether the YAML uses `client_key_credential:` (DB/encrypted store) or `client_key_path:` (real file). Only API-created orgs (`handle_organisations.go`) report "database" correctly. **Display-only bug** — runtime resolution is unaffected (collector.go:1582-1592 and main.go:1375-1383 fall back to live config). **Fix options:** (a) populate `ClientKeyCredentialName` in the sync params from `org.ClientKeyCredential` (smaller; only fixes the cred-name case); or (b) have the status handler consult live config with the same precedence the collector/ClientFactory use — DB cred name → config cred name → config file path — to report file/database truthfully (more accurate, distinguishes real file vs store). (Found 2026-06-19.)

## Credential Store

- [ ] Write functional tests for `DBCredentialStore` SQL paths against real PostgreSQL (build-tagged `//go:build functional`)

## Credential Testing

- [ ] Implement `chef_client_key` live test: optionally test Chef API call with the key
- [ ] Write unit tests for live credential test functions (with mocked external services)

## Startup Validation

- [ ] Warn if keys directory permissions > `0700`
- [ ] Warn if env file permissions > `0640` (RPM/DEB)
- [ ] Write unit tests for startup validation (all pass, various failure modes)

> TLS key-file permission warning (>0600) is **done**: `config.go` startup
> validation (`server.tls.key_path`) and `tls/certmanager.go checkKeyPermissions`.
> The two items above are about the credential key material (keys dir / env
> file), not TLS, and are still open.

## Consumer Integration

- [ ] Update `internal/chefapi/` to resolve Chef API keys via `CredentialResolver`
- [ ] Verify plaintext is zeroed after use in all consumer call sites
- [ ] Write integration tests for Chef API signing with each credential source

## Configuration Integration

- [ ] Add `client_key_env` field to organisation config schema

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
- [ ] Document RPM/DEB credential file setup