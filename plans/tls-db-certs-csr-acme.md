# Plan — TLS in DB, CSR generation, ACME/Route53

Branch: `feature/tls-db-certs-csr-acme`.

Three features, sequenced. Each chunk = one session. TDD throughout (tests first).
Spawned agents write files only — caller handles git.

## Goal

1. **Cert + key in the DB** (encrypted `config_store`), configurable via the UI,
   instead of only on disk.
2. **Generate CSRs** in-app for static cert issuance (operator submits CSR to
   their CA, pastes the signed cert back).
3. **Implement ACME** (`mode: acme`) with **Route53 DNS-01**, using AWS IAM creds.

## Decisions (locked)

- **ACME protocol:** `golang.org/x/crypto/acme` (low-level, already in `go.mod` —
  0 new modules). We write account/order/challenge/renewal/storage orchestration
  ourselves. Rationale: supply-chain minimisation — `certmagic` ~doubles the
  module tree, `lego` pulls ~all 150 DNS-provider SDKs into `go.sum` (lockfile
  scanners flag the lot). [[supply-chain-priority-deps]]
- **Route53 client:** `aws-sdk-go-v2` route53 subset (`config`, `credentials`,
  `service/route53`, `smithy-go` — ~6-8 AWS-maintained modules). The only
  accepted new third-party surface; avoids hand-owning SigV4 signing.
- **ACME state storage:** DB (`config_store`, encrypted), not disk. Avoids LE
  rate-limit re-issue on restart; no persistent volume needed; unifies with (1).
- **Static cert source:** coexist — add `server.tls.cert_source: file | db`
  (default `file` for back-comat). File paths keep working (k8s cert-manager
  mounts); `db` is the UI-driven path.
- **AWS creds:** stored as encrypted secrets in `config_store` (`secret: true`,
  never returned via API), with fallback to env vars / IAM instance role.
- **Anti-lockout / recovery:** **Repair CLI only** — `tls reset` (set
  `mode: off` in DB) and `tls clear-ca` (drop `ca_path`). No break-glass env
  lever. Recovery boundary stays **host access** (needs `DATABASE_URL` +
  `CMM_CREDENTIAL_ENCRYPTION_KEY`, both host-side), replacing today's
  "move the cert/key/CA file on host" path which vanishes once TLS material is
  in the DB. Because the CLI (not an env var) is the escape hatch, anti-lockout
  todo item 1 (env override ignored on DB path) is **no longer load-bearing** —
  leave it as a documented limitation. `ca_path` may live in the DB since
  `tls clear-ca` recovers an mTLS lock.

## Architecture notes

- Reuse the existing encryption stack: `internal/secrets` (AES-256-GCM, HKDF,
  per-row nonce, AAD bind) + `internal/configstore` (`Set`/`GetSecret`, `secret`
  flag controls API redaction). Master key `CMM_CREDENTIAL_ENCRYPTION_KEY`.
- Cert (public) stored `secret: false`; private key stored `secret: true`.
- DB source reloads on `configHolder.Reload()` (config change), not file-watch.
  File source keeps the existing poll-watch (`certmanager.go`).
- Fail-open (§2.4) and degraded `/api/v1/server/tls-status` behaviour preserved
  for every mode.

## Key config keys (config_store)

- `server.tls.certificate` — leaf+chain PEM (secret:false)
- `server.tls.private_key` — key PEM (secret:true)
- `server.tls.private_key.pending` — CSR-generated key awaiting signed cert
- `server.tls.acme.account_key` — ACME account key (secret:true)
- `server.tls.acme.cert` / `.key` — issued cert/key (key secret:true)
- `server.tls.acme.route53.access_key_id` / `.secret_access_key` (secret:true)
- `server.tls.acme.route53.region` / `.hosted_zone_id` (non-secret)

---

## Chunk 0 — Spec updates (DONE)

Signed off and complete. `tls.md` split into a thin index + `tls-static.md` /
`tls-acme.md` / `tls-csr.md`; `overview.md` routing + `configuration-schema-server.md`
ACME/static tables and inbound links updated to match. Resolved open items:
spec sign-off granted; `file`+`db` `cert_source` coexist (k8s/cert-manager file
mounts exist); CSR default key algo `ecdsa-p256`.

Original scope (for reference):

- `specifications/tls.md` §2: add `cert_source`, DB-stored cert/key model,
  encrypted-at-rest, key never exposed, reload-on-config-change.
- New section: **CSR generation** — keypair algo choice, subject/SAN inputs,
  pending-key lifecycle, signed-cert match-and-promote.
- §3 ACME: specify `x/crypto/acme`, DB-backed storage, Route53 via
  `aws-sdk-go-v2`, AWS cred resolution order, renewal scheduler + backoff.
  Update §3.4 (provider config) and §3.5 (storage → DB). Extend §2.4/§5.3 so an
  **unobtainable ACME cert also fails open to plain HTTP** (today §5.3 assumes
  `acme` is always healthy). §5.3 recovery rewritten for the repair CLI in
  Chunk 3a.
- tls.md is 401 lines; additions will breach the 500-line cap. **Split** into a
  thin index + `tls-static.md` / `tls-acme.md` / `tls-csr.md` (per CLAUDE.md spec
  split rule). Update `specifications/overview.md`.
- **Acceptance:** specs approved; pre-commit line-length hook passes.

## Chunk 1 — Config schema + validation (DONE)

Done 2026-06-09. `TLSConfig.CertSource` (`cert_source`, default `file`) + env
override `..._TLS_CERT_SOURCE`; `static`+`db` skips cert/key path requirement
(validated at save/preflight, not startup); `cert_source` value validated
(`file`|`db`). dns-01 route53 requires `region`+`hosted_zone_id` in
`dns_provider_config` (region also via `AWS_REGION`/`AWS_DEFAULT_REGION`),
skipped when `AWS_ACCESS_KEY_ID` set (env/role escape). `ACMEConfig.StoragePath`
marked deprecated/parse-only (ACME state is DB-backed per §3.5). Frontend
`config.ts` gained `cert_source` + `dns_provider_config`. AWS creds (access/secret
key) intentionally NOT in YAML — config-store secrets only (later chunks).
Tech debt logged: load-time dns-01 validation can't see config-store region/zone.
10 new config tests; full `internal/config` + AdminServerPage suites green.

Original scope (for reference):

- Scope: `internal/config/config.go` (TLSConfig `CertSource`; ACME storage mode;
  Route53 cred keys), `frontend/src/types/config.ts` (add `cert_source`,
  `dns_provider_config`), validation (`validate*`).
- Rules: `cert_source: db` requires cert+key present in store (checked at
  save/preflight, not startup — startup stays structural per §2.4); dns-01
  requires region + hosted zone or env creds.
- TDD: config validation tests first.
- **Acceptance:** new fields parse/default/validate; existing config unaffected;
  `go test ./internal/config/...` green.

## Chunk 2 — CertManager: in-memory PEM source (foundational) (DONE)

Done 2026-06-09. New `internal/tls/source.go` defines a `pemSource` interface
with two impls: `fileSource` (paths on disk) and `bytesSource` (mutex-guarded
in-memory PEM). `CertManager` now holds a `source` instead of cert/key paths.
`NewCertManager(certPath, keyPath, …)` (file) is unchanged for callers; new
`NewCertManagerFromPEM(certPEM, keyPEM, …)` builds the bytes source. Shared
`newCertManager` constructor; `loadCertificate` now fetches PEM from the source
and a pure `buildCertificate` helper does parse+validate+leaf-parse (used by
both load and reload). New `ReloadFromPEM(cert, key)` validates before swapping
and keeps the previous cert on failure (config-change reload path for
`cert_source: db`); errors on a file source. `CertPath()/KeyPath()` return `""`
for the bytes source. `WatchForChanges` and `checkKeyPermissions` are no-ops for
the bytes source (file-only via type assertion). All existing TLS tests pass
unchanged; new `certmanager_pem_test.go` covers both sources, reload, mismatch,
expiry, file-source guard, and serve-cert parity. `go test -race`, `go vet`,
`gofmt` clean; full repo builds.

Original scope (for reference):

- Scope: `internal/tls/certmanager.go`, `internal/tls/listener.go`.
- Refactor `NewCertManager` to accept a PEM **byte source** (file path OR
  in-memory bytes) behind one interface. File source keeps poll-watch; bytes
  source exposes `ReloadFromPEM(cert, key)`.
- Preserve all current behaviour (mTLS, expiry warn, hot-reload, fail-open).
- TDD: table tests for both sources; reload + mismatch cases.
- **Acceptance:** existing TLS tests pass unchanged; new bytes-source tests green.

## Chunk 3 — DB cert/key storage + admin API (Feature 1 backend) (DONE)

Done 2026-06-09. New `apptls.ValidateStaticPairBytes` (in-memory pair preflight)
and `apptls.CertMetadata`/`CertMetadataFromPEM` (operator-safe subject/SANs/expiry,
key never touched). Config-store keys `server.tls.certificate` (secret:false),
`server.tls.private_key` (secret:true), `…private_key.pending` added — standalone
entries, excluded from AllConfigKeys/assembly (default case ignores them).
`ListenerConfig` gained `CertSource`/`CertPEM`/`KeyPEM`; `NewListener` builds the
CertManager from PEM when `cert_source: db`. Admin `PUT` now branches on
`cert_source`: `file` keeps the path+`ValidateStaticPair` requirement; `db` skips
paths, validates the submitted pair (`422` on mismatch/partial/none-stored),
persists cert (non-secret) + key (secret), and triggers an in-place reload via a
new `webapi.TLSReloadHolder`/`CertReloader` (`WithTLSReload`) — best-effort, falls
back to restart (`RestartRequired: true` always). `GET` augments the response with
`tls_certificate_info` for a db source; the key is never returned. `main.go`
static branch loads DB PEM (`loadDBCertKey`), fails open to plain HTTP when
missing/unreadable (§2.4), and registers the running CertManager into the reload
holder. Tests: tls pair/metadata + listener-db; webapi handler (success, redaction,
mismatch, partial, none-stored, keep-existing, GET metadata, reload trigger);
main `loadDBCertKey` round-trip/missing/nil-store. `go test -race`, `go vet`,
`gofmt` (my files) clean.

Original scope (for reference):

- Scope: `internal/webapi/handle_admin_config_server.go` (+ helper),
  `internal/configstore`, `cmd/.../main.go` static-mode wiring.
- Persist cert (secret:false) + key (secret:true); preflight-validate the pair
  before write (reuse `apptls.ValidateStaticPair` adapted for bytes); reject
  `422` on bad pair.
- Static listener builds CertManager from DB PEM when `cert_source: db`;
  rebuild/reload on `configHolder.Reload()`.
- API never returns the key; returns cert metadata (subject, SANs, expiry).
- TDD: handler tests (save valid/invalid pair, key redaction, reload).
- **Acceptance:** can save cert/key via API, listener serves it, key never
  returned, bad pair rejected. Depends on Chunks 1,2.

## Chunk 3a — TLS recovery CLI + anti-lockout (DONE)

Done 2026-06-10. New `tls` repair subcommands dispatched in `main.go` `run()`
before flag parsing: `tls reset` (set `server.tls.mode: off`) and `tls clear-ca`
(drop `server.tls.ca_path`), in `cmd/.../tlsrepair.go`. Both read-modify-write the
`server.tls` section only (never create a shadowing section where none exists →
`repairNoSection` reported), are idempotent (`repairNoChange` when already in the
desired state), and preserve all other TLS fields. Store is built from env only
(`CMM_DATABASE_URL`/`DATABASE_URL` + `CMM_CREDENTIAL_ENCRYPTION_KEY`) so it works
when the YAML config is broken/absent; no break-glass override (recovery boundary
= host access, per tls.md § 6.3). Added `configstore.DeserializeValue` (inverse of
`SerializeValue`) for targeted single-section decode. Spec § 6.3 was already
rewritten in Chunk 0. Tests: reset/clear-ca (changed/no-change/no-section,
field-preservation) via the in-memory store; `SerializeValue`/`DeserializeValue`
round-trip. `go test -race`, `go vet`, build clean; new files gofmt-clean.

Original scope (for reference):

Must exist the moment DB `cert_source` is usable — it is the replacement escape
hatch for the host-file removal that no longer applies once TLS material is in
the DB. Blocks shipping Chunk 3 / mTLS-via-DB / ACME.

- Scope: new CLI subcommands `tls reset` (set `server.tls.mode: off` in
  `config_store`) and `tls clear-ca` (delete `server.tls.ca_path` /
  `ca` entry), under `cmd/.../`; reuse existing datastore + configstore +
  encryptor (host-side `DATABASE_URL` + `CMM_CREDENTIAL_ENCRYPTION_KEY`).
- Confirm scenarios: bad-cert-in-DB still fail-opens (§2.4); mTLS-via-DB lock
  recovers via `tls clear-ca` + restart; ACME-can't-issue fail-opens (Chunk 8).
- Spec: rewrite `tls.md` §5.3 recovery (host-file → repair CLI); note that
  anti-lockout item 1's env override is a documented limitation, not the hatch.
- TDD: CLI mutates DB to the expected state; idempotent; clear operator output.
- **Acceptance:** an operator locked out (mTLS-via-DB or bad DB cert) recovers
  with a CLI command + restart, no host file access to PEM material required.
  Depends on Chunk 3.

## Chunk 4 — DB cert/key UI (Feature 1 frontend) (DONE)

Done 2026-06-10. `types/config.ts` gained `CertMetadata` (subject/issuer/
dns_names/ip_addresses/not_before/not_after), write-only `TLSConfig.certificate`
+ `private_key` (sent on save, never returned), and read-only
`ServerConfig.tls_certificate_info`; re-exported `CertMetadata` from the api
barrel. `AdminServerPage.tsx` static block now has a **Certificate source**
selector (file | db): file keeps the cert/key path inputs; db shows the
installed-cert metadata panel (subject/issuer/SANs/validity, expired flag),
a paste-PEM certificate textarea, and a **write-only** private-key textarea
(never pre-filled — GET returns no key — and cleared after save since the PUT
response omits it). `handleSave` carries `tls_certificate_info` forward across
the metadata-less PUT response so the panel persists until the next reload. The
existing CA-path/mTLS field is shared by both sources. 8 new vitest cases (source
toggle, file↔db field swap, never-pre-filled key, metadata panel, empty-state
prompt, db save payload, post-save clear); full frontend suite 348 green; tsc +
eslint clean.

Original scope (for reference):

- Scope: `frontend/src/pages/AdminServerPage.tsx`, `frontend/src/api/config.ts`.
- `cert_source` selector; cert textarea (paste PEM); key textarea (write-only,
  never pre-filled); current-cert metadata panel; save + restart flow.
- TDD: component tests (vitest) for source toggle + write-only key.
- **Acceptance:** operator configures DB cert/key end-to-end in the UI.
  Depends on Chunk 3.

## Chunk 5 — CSR generation backend (Feature 2 backend) (DONE)

Done 2026-06-10. New `internal/tls/csr.go`: `GenerateCSR(CSRRequest)` generates a
key (ecdsa-p256 default, ecdsa-p384/rsa-2048/3072/4096) and a PKCS#10 CSR over the
subject (CN/O/OU/C) + DNS/IP SANs; key returned PKCS#8 PEM ("PRIVATE KEY").
Validates identifier-present (CN or a SAN), key algo, IP-SAN parse — operator-safe
errors, never key material. New `POST /api/v1/admin/config/server/generate-csr`
(`handle_admin_config_server_csr.go`, admin-only): generates, stores the key at
`server.tls.private_key.pending` (secret, overwriting any prior), returns CSR PEM
+ echoed algo; key never returned. PUT db branch extended for match-and-promote
(§ 4.5): a cert pasted with no key matches against the pending key via
`ValidateStaticPairBytes` — on match it promotes (writes active cert+key, deletes
pending), `422` on mismatch/no-pending; the active pair is only replaced on a
clean match so fail-open holds. `pendingKeyPEM` helper reads the secret; persist
now marshals the unified `dbCertPEM`/`dbKeyPEM`. Tests: CSR content/algos/identifier/
IP/algo-reject; handler success/redaction/503/422s/method; full round-trip
generate→sign→promote, promote-mismatch, promote-no-pending. `go test -race`, vet,
gofmt, build clean.

Original scope (for reference):

- Scope: new `internal/tls/csr.go`, new handler
  `POST /api/v1/admin/config/server/generate-csr`, configstore lifecycle.
- Input: subject (CN, O, OU, country), SANs (DNS + IP), key algo
  (rsa-2048/3072/4096, ecdsa-p256/p384). Generate key → store as
  `private_key.pending` (secret); build CSR; return CSR PEM (downloadable).
- Signed-cert upload (Chunk 3 path) matches against pending key; on match,
  promote pending → active and activate cert; else `422`.
- TDD: CSR content (subject/SANs/algo), pending→active promotion, mismatch.
- **Acceptance:** generate CSR, sign externally, paste cert, listener serves it.
  Depends on Chunk 3.

## Chunk 6 — CSR UI (Feature 2 frontend) (DONE)

Done 2026-06-10. `api/config.ts` gained `GenerateCSRRequest`/`GenerateCSRResponse`
types + a `generateCSR()` POST to `…/server/generate-csr`. `AdminServerPage.tsx`
shows a CSR panel inside the `cert_source: db` block only: subject fields
(CN/O/OU/C), a reusable `ChipEditor` for DNS + IP SANs, a key-algorithm dropdown
(ecdsa-p256 default + p384/rsa-2048/3072/4096). Generate is disabled until an
identifier (CN or a SAN) is present; on success the returned CSR PEM is shown in a
read-only textarea with a Download CSR button (blob) and guidance to paste the
signed cert into the Certificate (PEM) field above (match-and-promote). Generate
errors surface inline; the private key is never shown (stored pending server-side).
8 new vitest cases (db-only panel, algo default+options, identifier gate, SAN
add/remove, payload, CSR display+guidance, error). Full frontend suite 355 green;
tsc + eslint clean.

Original scope (for reference):

- Scope: `AdminServerPage.tsx`, `api/config.ts`.
- Subject fields, SAN list editor, algo dropdown, Generate → show/download CSR;
  guidance to paste the signed cert into the Feature-1 cert field.
- **Acceptance:** full CSR round-trip in the UI. Depends on Chunks 4,5.

## Chunk 7 — ACME core engine (Feature 3 core) (DONE)

Done 2026-06-10. New `internal/acme/` package owns the ACME orchestration:
- `storage.go` — DB-backed state layer over a `SecretStore` seam (satisfied by
  `*configstore.Store`): account-key (PKCS#8 PEM, secret), issued cert (public)
  + key (secret), stored as JSON-encoded PEM matching the static path. A missing
  half-pair reads as `ErrNotStored` (fail-open). New config-store keys
  `server.tls.acme.account_key` / `.cert` / `.key` added to `assembly.go` as
  standalone entries (excluded from assembly, like the static cert keys).
- `solver.go` — `Solver` interface + `Challenge` seam (HTTP-01/DNS-01 solvers
  land in Chunks 8/9); `LogFunc` callback (no logging-pkg import).
- `client.go` — `acmeClient` interface (subset of `x/crypto/acme.Client`,
  satisfied structurally; `*acme.Client` is the real impl) + `Config` (engine's
  view of `server.tls.acme`) + `realClientFactory`.
- `manager.go` — `NewManager` + `Obtain`: ensure/register account (persist key
  only on success; already-registered tolerated), authorize each domain via the
  solver (compute HTTP01/DNS01 response, present → accept → wait → cleanup),
  finalize with a fresh ECDSA-P256 cert key + CSR over the domains, persist
  cert/key. `agree_to_tos: false` → `ErrTOSNotAccepted` (fail-open, § 3.7/3.11).
- `renew.go` — `Renewer`: `checkOnce` (issue if none, renew when within
  `renew_before_days`, WARN + `certificate_expiry_warning` event within 7 days
  without renewal) + `Run` loop with exponential backoff (1h→24h cap). Pure
  decision fns `renewalDue`/`expiryWarning`/`nextBackoff`.

TDD with fake `acmeClient`/`Solver`/`CertObtainer` + in-memory `SecretStore`
(no network): storage round-trip + half-pair; full HTTP-01/DNS-01 order flow,
new-vs-existing account, ToS refusal, no-matching-challenge, cleanup-on-failure;
backoff/due/warn tables + checkOnce issue/healthy/renew/warn paths.
`go test -race`, `go vet ./...`, gofmt, build clean. Mode wiring + the real
challenge solvers are Chunks 8/9.

Original scope (for reference):

- Scope: new `internal/acme/` — account registration (persist account key in
  DB), order flow via `x/crypto/acme`, cert/key persistence to config_store,
  renewal scheduler (pre-expiry, exponential backoff 1h→24h cap), expiry-warning
  events. DB-backed storage layer.
- TDD: orchestration against a mock ACME directory (Pebble optional); renewal
  timing; storage round-trip. No network in unit tests.
- **Acceptance:** issue + persist + schedule-renew against a test CA, all in DB.
  Depends on Chunk 3 (storage patterns).

## Chunk 8a — Self-signed degraded fail-open (foundation, both modes) (DONE)

Done 2026-06-10. New `internal/tls/selfsigned.go`: `GenerateSelfSigned(hosts)`
builds an ephemeral ECDSA-P256 self-signed cert/key (DNS+IP SANs split from
hosts; defaults to localhost/loopback when none), used as the degraded fail-open
listener. `internal/tls/listener.go`: `ListenerConfig.HSTSEnabled func() bool`
live-gate threaded into `HSTSMiddleware(next, trustedProxy, enabled)` (nil ⇒
always on) so HSTS is suppressed while serving the untrusted self-signed cert and
resumes on in-place promotion. `internal/webapi/handle_server_tls_status.go`:
`TLSStatus.Kind` (`self-signed`|`plain`) + `SetDegradedKind`/`SetHealthy`/
`IsDegraded`; `tls_reload.go` gained `SetOnReload` (fires after a successful
in-place reload). `cmd/.../main.go`: `degradeToSelfSigned` (serves self-signed
HTTPS, registers the CertManager for promotion, falls back to plain HTTP only if
gen/build fails) replaces plain-HTTP fail-open on the static `cert_source:db`
path; `hstsEnabledFn` wired; `SetOnReload(tlsStatus.SetHealthy)` clears degraded
on promotion. Frontend: `TLSStatus.kind` type + kind-aware `TLSDegradedBanner`
copy (untrusted self-signed vs INSECURE plain HTTP). Specs `tls-static.md § 2.4`,
`tls.md § 6.3`, `tls-acme.md § 3.11` rewritten to the self-signed degraded model.
TDD: self-signed gen (loadable/self-signed/SAN-split/defaults/validity/ephemeral),
HSTS suppress+resume, status kind/healthy round-trip, `degradeToSelfSigned`
end-to-end HTTPS serve + no-HSTS + promotion-clears-degraded, banner kinds.
`go test -race ./...`, vet, gofmt, build clean; frontend tsc/eslint clean,
357 vitest green.

Original scope (for reference):

Design change (user-approved): the fail-open path no longer drops to plain HTTP
as its primary behaviour. When no usable CA cert is available it serves an
**ephemeral self-signed cert** so the admin/recovery UI stays **encrypted**
(browser click-through) instead of cleartext. This fires exactly on first-run /
ACME-not-yet-working, where a plaintext login is worst. Plain HTTP is kept only
as a **last resort** if self-signed generation or the TLS bind itself fails.
Applies to **both** static `cert_source:db` and `acme` (8b) fail-open.

- Scope: new `internal/tls/selfsigned.go` (generate ephemeral ECDSA-P256
  self-signed cert+key for the configured domains / a sane fallback name);
  `internal/tls/listener.go` (HSTS **live-gate** so HSTS is suppressed while
  degraded and resumes on in-place promotion to a real cert);
  `internal/webapi/handle_server_tls_status.go` (degraded "kind" =
  self-signed|plain; `SetHealthy`/clear for promotion; `IsDegraded` accessor);
  `cmd/.../main.go` (static `cert_source:db` fail-open → self-signed, plain-HTTP
  last resort; new `degradeToSelfSigned` helper alongside `degradeToPlainHTTP`).
- HSTS: never send `Strict-Transport-Security` over a self-signed cert (it would
  pin and block the click-through on the next visit → self-inflicted lockout).
  Gate via `ListenerConfig.HSTSEnabled func() bool` (nil ⇒ on, preserves tests).
- Spec: update `tls-static.md § 2.4` and `tls.md § 6.3` (degraded model =
  self-signed primary, plain-HTTP last resort, HSTS-off-while-degraded; recovery
  still the Chunk 3a CLI).
- TDD: self-signed cert generation (names/SANs/validity/parse); HSTS suppressed
  while degraded + restored on promote; static db fail-open serves self-signed,
  plain-HTTP only when self-signed unavailable.
- **Acceptance:** a missing/invalid static DB cert serves a self-signed cert over
  HTTPS (degraded, no HSTS), not plain HTTP; plain HTTP only as last resort.
  Depends on Chunks 2,3.

## Chunk 8b — ACME HTTP-01 + mode wiring (DONE)

Done 2026-06-10. New `internal/acme/http01.go`: `HTTP01Solver` (Solver seam,
mutex `token→keyAuth` map, `Handler()` serving `/.well-known/acme-challenge/`,
404 for unknown token / non-challenge path). `internal/tls/listener.go`:
`NewChallengeRedirectServer(listenAddr, redirectPort, httpsPort, challenge)` —
port-80 server that serves the challenge handler first then 301s everything else
to HTTPS (shared `redirectToHTTPS` helper extracted from `redirectHandler`).
`cmd/.../acme.go`: `setupACME` replaces the not-implemented error — staging WARN,
builds `acme.Storage`/`HTTP01Solver`/`Manager`/`Renewer`, **always comes up on
HTTPS** (stored issued cert if present else 8a self-signed degraded), HTTPS
listener runs `HTTPRedirectPort:0` (port 80 owned by the challenge server),
background renewer wrapped in `promotingIssuer` (`ReloadFromPEM`-promotes obtained
cert in place, `SetOnReload` clears degraded + resumes HSTS); `http-01` +
`http_redirect_port:0`, `dns-01` (Chunk 9), and unknown challenge all fail open to
self-signed (never exit); challenge-server bind failure logged, non-fatal.
`serverResult` gained `challengeSrv`+`renewerCancel`; `awaitShutdown` cancels the
renewer and drains the challenge server. TDD (fakes, no network — ToS-false keeps
the renewer offline): solver serve/404/cleanup/multi-token, challenge>redirect
priority + redirect-port logic, `promotingIssuer` promote-clears-degraded /
error-stays-degraded, `setupACME` self-signed-degraded / stored-cert-healthy /
no-redirect-port / dns-01 fail-open, staging detection. `go test -race`, vet,
gofmt, build clean. Specs `§3.3`/`§3.11` already accurate (8a) — no change.

Original scope (for reference):

- Scope: `cmd/.../main.go` (replace the `acme` "not implemented" error),
  `internal/tls/listener.go` (challenge-aware redirect server),
  new `internal/acme/http01.go`.
- New `HTTP01Solver` (Solver seam): mutex map `token→keyAuth`; `Handler()` serves
  `/.well-known/acme-challenge/<token>`. Install it on the **redirect listener**
  (challenge > redirect priority, § 3.3) via a `NewChallengeRedirectServer`
  constructor; the HTTPS `Listener` runs with `HTTPRedirectPort: 0` (port 80
  owned by the challenge/redirect server for the whole process lifetime).
- main.go `acme` case: build `acme.Storage`/`HTTP01Solver`/`Manager`/`Renewer`;
  translate `config.ACMEConfig`→`acme.Config`; staging-URL WARN; `http-01` with
  `http_redirect_port: 0` → ERROR. **Always come up on HTTPS**: serve the stored
  real cert if present, else the 8a **self-signed** cert (degraded); run the
  Renewer in the background, which obtains and `ReloadFromPEM`-**promotes**
  self-signed → real with no restart (clears degraded). ToS-false / order-fails
  stay self-signed-degraded (§ 3.11). Plain-HTTP last resort via 8a.
- Shutdown: extend `serverResult` with the challenge server + a renewer cancel
  func so `awaitShutdown` drains them.
- Spec: `tls-acme.md § 3.11` updated to the 8a self-signed degraded model.
- TDD: http-01 challenge served; mode-selection picks acme; ToS-false → degraded
  (not exit); in-place promote swaps the live cert. Fakes only, no network.
- **Acceptance:** `mode: acme` + http-01 obtains a cert against staging; before
  issuance the UI is reachable over self-signed HTTPS. Depends on Chunks 7,8a.

## Chunk 8c — CLI: deliberate plain-HTTP (behind TLS-terminating proxy) (DONE)

Done 2026-06-10. New `cmd/.../tlsmode.go`: `tls mode <off|static|acme>` added to
the 3a dispatch (`tlsrepair.go` `runTLSCommand`), with `parseTLSModeArgs`
(mode + optional `--trusted-proxy[=true|false]`, any order, bare flag ⇒ true),
`tlsSetMode` (section-preserving, no-section-safe, idempotent — `tlsResetMode` is
now a thin alias), `tlsSetTrustedProxy` (scalar `server.trusted_proxy`; false
when absent ⇒ no orphan key), and `runTLSMode` (combined operator output, gates
the trusted-proxy write on the server.tls section existing). **Required wiring:**
`server.trusted_proxy` was YAML-only and silently dropped on migration — added
`KeyServerTrustedProxy` to `configstore/assembly.go`
(AllConfigKeys + assembleFields + ConfigToSections) so the CLI/UI value is read
at startup and on reload. Same env-only store + idempotency + no break-glass as
3a. Spec: `tls.md § 9.1` (behind-proxy deployment) + `tls-static.md` pointer.
TDD (no network): assembly round-trip/known-key; mode set-each/no-section/alias,
trusted-proxy true⇄false/idempotent/false-when-absent, `parseTLSModeArgs` table.
`go test -race`, vet, gofmt, build clean. UI toggle is Chunk 8d.

Original scope (for reference):

User request: a CLI to **force plain HTTP on purpose** so the app can sit behind
a load balancer / reverse proxy that terminates TLS. Distinct from fail-open
(8a) — that's an error state; this is an intended deployment. `mode: off`
already yields plain HTTP and `tls reset` already sets it, but `tls reset` is
recovery-framed and does not also set `trusted_proxy` (needed so the HSTS /
scheme detection honours `X-Forwarded-Proto` from the proxy).

- Scope: `cmd/.../tlsrepair.go` (or a sibling `tlsmode.go`), extending the
  Chunk 3a repair-CLI dispatch.
- Add `tls mode <off|static|acme>` (sets `server.tls.mode`), generalising
  `tls reset` (which stays as the recovery alias for `mode off`). For the proxy
  case, a `--trusted-proxy[=true|false]` flag on `tls mode off` also sets
  `server.trusted_proxy` so HSTS is emitted for proxied HTTPS requests.
- Same env-only store + section-preserving read-modify-write + idempotency as
  3a; clear operator output. No git, no remotes.
- Spec: note the behind-proxy deployment in `tls-static.md`/`tls.md` (plain HTTP
  + `trusted_proxy` + proxy terminates TLS).
- TDD: `tls mode` sets each mode; `--trusted-proxy` toggles the flag;
  idempotent; preserves other fields.
- **Acceptance:** operator runs one CLI command to put the app in plain-HTTP
  behind-proxy mode (mode off + trusted_proxy) and back. Depends on Chunk 3a.

## Chunk 8d — UI: behind-proxy plain-HTTP toggle (DONE)

User also wants the behind-proxy switch in the admin UI (not just the CLI),
dynamic per project rules (takes effect without restart). Same semantics as 8c:
set `server.tls.mode: off` + `server.trusted_proxy: true`.

- Scope: `frontend/src/pages/AdminServerPage.tsx`, `frontend/src/api/config.ts` /
  `types/config.ts` (surface `trusted_proxy` if not already), backend config save
  path if a field is missing.
- A "Terminate TLS at a proxy (plain HTTP)" toggle that, when enabled, sets
  mode off + trusted_proxy; explains HSTS is then driven by `X-Forwarded-Proto`.
  Guard against accidental lockout (confirm step / clear warning).
- TDD: component test (toggle sets mode off + trusted_proxy in the save payload;
  warning shown).
- **Acceptance:** operator switches to/from behind-proxy plain-HTTP from the UI.
  Depends on Chunk 8c (shared semantics) + existing admin config save.

## Chunk 9 — Route53 DNS-01 solver (Feature 3 DNS)

- Scope: new `internal/acme/route53.go`, `go.mod` (+aws-sdk-go-v2 subset).
- UPSERT/DELETE TXT `_acme-challenge.<domain>`, poll `GetChange` until `INSYNC`.
- Creds: env / IAM role / encrypted config secrets (resolution order); region +
  hosted-zone resolution.
- Note `go.mod` additions for the CI lockfile-scan plan; record any debt.
- TDD: solver against a mocked Route53 API (interface seam, no real AWS).
- **Acceptance:** dns-01 obtains a cert against staging via Route53.
  Depends on Chunks 7,8.

## Chunk 9a — Route53 hostname self-registration (A record)

Opt-in (`register_hostname`, default off) self-publishing of an A record per
`acme.domains` entry so the server's FQDN resolves to the host, reusing the
Chunk 9 Route53 client/UPSERT/`GetChange`. Spec: [tls-acme.md § 3.13](../specifications/tls-acme.md).

- Scope: `internal/acme/route53.go` (or a sibling `hostname.go`), a small IP
  resolver helper (new, stdlib `net` only), `internal/config` (ACMEConfig:
  `RegisterHostname bool`, `HostnameTTL int`, `HostnameInterface string`,
  `HostnameIP string`) + validation, `frontend/src/types/config.ts`.
- IP resolution (first non-empty wins): `hostname_ip` → `hostname_interface`
  (named iface global-unicast IPv4) → auto-detect (default-route interface IPv4,
  no packets sent). Explicit-but-unusable = `ERROR` + skip, no fall-through.
- Names = each `acme.domains`; wildcard domains skipped with `WARN`. A records
  only. TTL `hostname_ttl` (default 60).
- Lifecycle: UPSERT at ACME startup, each renewal cycle (re-asserts a changed
  DHCP IP), and on relevant config change; poll `GetChange` to `INSYNC`. No
  delete on shutdown; disabling stops updates but leaves the record.
- Fail-soft: registration failure logs `ERROR` on `tls` scope + TLS status, but
  never blocks issuance/renewal/fail-open or aborts startup (A-record is
  orthogonal to the TXT-based DNS-01 challenge). No new deps/IAM (reuses
  `route53:ChangeResourceRecordSets`).
- TDD: IP resolver (literal/iface/auto, unusable cases); record-set build per
  domain incl. wildcard skip; UPSERT+poll against the mocked Route53 seam;
  re-assert on IP change. No real AWS/network.
- **Acceptance:** with the toggle on, an A record per domain is UPSERTed to the
  configured IP and reaches `INSYNC`; failures degrade soft. Depends on Chunk 9.

## Chunk 10 — ACME UI + AWS creds (Feature 3 frontend)

- Scope: `AdminServerPage.tsx`, `types/config.ts`, `api/config.ts`, backend cred
  storage.
- Render the missing `dns_provider` + `dns_provider_config` (region, hosted
  zone) fields, AWS cred inputs (secret, write-only), dns-01 conditional block,
  ToS toggle, staging warning, ACME cert-status panel (issued/expiry/last
  renewal). Hostname self-registration (Chunk 9a): `register_hostname` tickbox +
  an "IP source: Auto / Interface / Manual IP" selector over `hostname_interface`
  / `hostname_ip` (+ `hostname_ttl`), shown only for `dns_provider: route53`.
- **Acceptance:** configure + run ACME dns-01 entirely from the UI, including
  toggling hostname self-registration. Depends on Chunks 9, 9a.

## Chunk 11 — Ignore files, packaging, docs, debt

- Scope: `.gitignore` / `.dockerignore` (no acme storage dir now it's DB; any new
  artifacts), packaging units (drop acme volume reqs if any), `plans/todo-*.md`,
  `plans/todo-tech-debt.md`.
- **Acceptance:** ignore files current; todos updated; debt recorded.

---

## Sequence / dependencies

`0 → 1 → 2 → 3 → 3a → 4` (Feature 1 done) `→ 5 → 6` (Feature 2 done)
`→ 7 → 8a → 8b → 9 → 9a → 10` (Feature 3 done) `→ 11`. Chunk 8c (deliberate
plain-HTTP CLI) depends only on 3a and can land any time.

Chunk 9a (Route53 hostname self-registration) depends on Chunk 9's Route53
client/UPSERT/`GetChange` and feeds the Chunk 10 UI; it is independent of cert
issuance (A-record vs the TXT challenge) so it can slip without blocking 8/9.

Chunk 3a (recovery CLI) must merge with/before anything that lets TLS material
live in the DB reach users — it gates Chunk 4, mTLS-via-DB, and ACME. Chunk 7
reuses Chunk 3 storage; 5/6 reuse Chunk 3 cert path. Do not start a chunk until
its dependencies are merged.

## Open items to confirm before Chunk 1 (RESOLVED)

- Spec sign-off (Chunk 0). ✓ granted.
- Deployment: k8s/cert-manager file-mount users exist → keep `file`+`db`
  `cert_source` coexist. ✓
- CSR key-algo default: `ecdsa-p256` (offer rsa-2048/3072/4096, ecdsa-p384). ✓
