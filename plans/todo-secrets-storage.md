# Secrets Storage — ToDo

The feature is IMPLEMENTED and in use: config-store-backed credential store
(`internal/configstore/credential_adapter.go`), `CredentialResolver`
(`internal/secrets/resolver.go`, DB→env→file precedence), encryption + zeroing,
admin-status reporting, packaging perms (nfpm), and spec docs
(`journeys/secrets-storage*.md`). Remaining = tests, doc-surfacing, startup
hardening, and one display-only bug. Sourcing model: only minimal values come from
env/`config.yaml`; client keys live in the DB via the UI (see
[[credential-sourcing-model]]) — env-var sourcing for client keys is out of scope.

## Setting up git access

- [ ] **Nothing in CMM manages SSH: `git clone` inherits whatever identity and `known_hosts`
  the service account has.** Requirement and the decisions behind it:
  `journeys/service-git-access.md`, suite `internal/webapi/service_git_access_journey_test.go`,
  which recomputes what is outstanding. Do not restate the requirement here.

## Bugs

- [ ] **Status page mislabels config-synced orgs' credential source as "file"**
  (display-only, low priority). `handle_admin_status.go:185-188` derives source solely
  from DB column `ClientKeyCredentialName` (empty→"file"); startup sync
  `main.go:896-901` never copies `org.ClientKeyCredential` (config) into the upsert
  params, so orgs using YAML `client_key_credential:` always report "file". Runtime
  resolution is unaffected. Fix: populate `ClientKeyCredentialName` in the sync params,
  or have the status handler consult live config with the resolver's precedence.

## Enhancements

- [ ] **Database connection strings have no credential type of their own.** A DSN is saved
  as "Generic", so the credentials page can neither say what format it wants nor check the
  one it was given — the same problem `chef_client_key` solved for PEM files by being its
  own type. Distinct `postgres_url` and `mssql_url` types would let the page carry the
  format and validate on save, which is where somebody typing a connection string is
  actually standing.

  **Deliberately not done yet.** The format example now sits on the ownership import's
  connection step instead (`OwnershipMappedImport.tsx`), because that is the page the
  person setting up an import is looking at, and they are importing owners rather than
  administering credentials. Typed secrets are the better answer when the credentials page
  is next worked on; until then the example is where it gets read.

## Tests (implementation exists; coverage is the gap)

- [ ] Functional PostgreSQL tests (`//go:build functional`) for the credential store —
  `CredentialStoreAdapter` (configstore-backed; there is no `DBCredentialStore` type).
  Adapter is unit-tested only (`credential_adapter_test.go`).
- [ ] `chef_client_key` live test — optionally make a real Chef API call with the key.
  Currently offline PEM validation only (`credential_adapter.go:359`,
  `secrets/validation.go:100`).
- [ ] Unit tests for the live credential-test function (mocked external services).
- [ ] Integration tests for Chef API signing per credential source (DB / file). Signing
  is tested with a directly-supplied key only (`chefapi/client_test.go:346-473`).
- [ ] Log-capture test asserting no credential plaintext/ciphertext reaches log output.
  Leak-prevention is currently proven only indirectly (`encryption_test.go`,
  `credential_store_test.go`); no test captures log output.

## Startup validation

- [ ] Warn if keys directory permissions > `0700`.
- [ ] Warn if env file permissions > `0640`.
- [ ] Unit tests for the two warnings above (only the TLS key_path 0600 check is tested
  today — `config_test.go:1066-1099`).

## Documentation / deploy

- [ ] Surface a secrets-management section in the top-level `README.md`. Today it covers
  commit-prevention only and links out to the spec (`README.md:507-532`); the runtime
  model + procedures live in `journeys/secrets-storage-*.md` but aren't surfaced.
- [ ] `deploy/docker-compose`: add `CMM_CREDENTIAL_ENCRYPTION_KEY=` placeholder to
  `.env.example` and a key-generation command to `README.md` (the RPM/DEB `env-file`
  already has the placeholder — `deploy/pkg/env-file:20`).

## Decision (no code unless we change our mind)

- `internal/chefapi/` takes injected private-key bytes; resolution via `CredentialResolver`
  happens at call sites (`collector.go:1612-1648`, `main.go:1613-1630`), not inside
  chefapi. This is the current design and acceptable — recorded so it isn't re-raised as
  "chefapi doesn't use the resolver."
