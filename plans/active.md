# Active — SAML config UX

Branch: `fix/saml-config-ux` (off `refactor/config-live-reload` — depends on that
branch's Chunk B applier machinery in `config_apply.go` + `storeAdminConfigSection`).
Spec: `specifications/auth.md` (SP Endpoints / SP Metadata Export UI / SAML provider
reload — all updated for this work). Driver: an operator hit `405 method_not_allowed`
guessing the ACS callback URL (nothing surfaced it), and a `sp_entity_id` change
needed a restart (provider built once at boot).

## Chunk SAML-1 — surface SP endpoint URLs (admin endpoint + UI)

The admin SAML page surfaces only the metadata URL; the ACS (callback) URL is not
discoverable, so hand-configured IdPs (Google/Okta) get guessed → 405. Surface the
backend-computed ACS/SLO/metadata URLs + SP entity ID with copy actions.

### Scope
- `internal/auth/samlsp/provider.go` — accessors `ACSURL()/SLOURL()/MetadataURL()/EntityID()`
  reading the configured `sp`/`cfg` values (the source of truth the metadata advertises).
- `internal/webapi/handle_saml_admin.go` — admin-only `GET /api/v1/admin/saml/endpoints`
  returning the four values as JSON; 501/not-initialised when no provider (mirror the
  metadata endpoint's gating).
- `internal/webapi/router.go` — register the route (only when `samlHandler != nil`).
- `frontend/src/api/` + `AdminAuthPage.tsx` — fetch + render ACS/SLO/metadata/entity
  read-only with copy buttons (extend the existing metadata-URL copy surface).

### Steps (TDD)
1. Go: provider accessor tests (configured → returns advertised URL); admin handler
   test (JSON shape; not-initialised path). Red→green.
2. Implement accessors + handler + route.
3. Frontend: test the SAML section renders the ACS URL + copy; implement.
4. `go test ./internal/webapi/... ./internal/auth/...`; `npm test` for the page.

### Acceptance
- ACS (callback), SLO, metadata URLs + entity ID surfaced with copy; values are
  backend-computed and match the SP metadata. New tests red→green; suites green.

## Chunk SAML-2 — live-reload the SAML provider (auth subsystem applier) ✅ DONE

Done in two commits: (1) `session_expiry`/`lockout_attempts` live reads
(`WithSessionLifetimeFunc`/`WithLockoutAttemptsFunc`); (2) SAML provider rebuild —
`SAMLHandler` guards provider+endpoints under one mutex with `SetProvider`/`prov()`,
request handlers 501 on nil provider, `WithSAMLReconciler` + `samlApplier` (subsystem),
auth PUT registers `appliedApplier` + the SAML applier, `main.go` `buildSAMLProvider`
refactor always creates the handler and wires the reconciler (rebuild from holder +
swap, incl. enable/disable). Auth section now applied/false (no SAML) or subsystem/false
(SAML wired). Full suite + `-race` + 387 frontend tests green.

### (original plan below)

`setupSAML` builds the provider once at boot; `auth` PUT has no applier → pessimistic
process/true → `sp_entity_id` etc. need a restart. Rebuild the provider in place on
save (subsystem), mirroring the backup reconciler pattern.

### Scope
- `internal/auth/samlsp/provider.go` — a rebuild path (fresh provider from a new
  `Config`, re-fetch IdP metadata). Keep `metadataMu` discipline.
- `internal/webapi/handle_saml.go` — guard `SAMLHandler.provider` for concurrent
  swap (it's read on every login/ACS/SLO request); add a locked setter/getter.
- `internal/webapi/router.go` — `WithSAMLReconciler(func() error)` option (parameterless,
  reads reloaded holder — webapi stays decoupled from samlsp construction).
- `internal/webapi/handle_admin_config_auth.go` — register the auth-section applier
  (subsystem) when the reconciler is wired; process default otherwise.
- `cmd/chef-migration-metrics/main.go` — refactor `setupSAML` to a reusable
  `buildSAMLProvider(cfg)`; wire `WithSAMLReconciler` rebuilding from
  `app.configHolder.Get()` and swapping into `app.samlHandler`. Handle enable
  (was nil at boot) / disable (config removes SAML) transitions.

### Steps (TDD)
1. samlsp rebuild test (URLs/entity swap after rebuild). Handler concurrent-swap
   `-race` test. webapi: auth PUT with reconciler → subsystem/false; reconciler
   error → 500 (previous provider keeps serving).
2. Implement, wire, run full suites incl. `-race`.

### Acceptance
- Auth section reports subsystem/false when wired; `sp_entity_id` change live (no
  restart); race-clean. Update `todo-configuration.md` Bucket 1 `auth.*`.
