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

## Chunk 0 — Spec updates (NEEDS SIGN-OFF)

Specs may not change without owner approval (CLAUDE.md). Get sign-off, then:

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

## Chunk 1 — Config schema + validation

- Scope: `internal/config/config.go` (TLSConfig `CertSource`; ACME storage mode;
  Route53 cred keys), `frontend/src/types/config.ts` (add `cert_source`,
  `dns_provider_config`), validation (`validate*`).
- Rules: `cert_source: db` requires cert+key present in store (checked at
  save/preflight, not startup — startup stays structural per §2.4); dns-01
  requires region + hosted zone or env creds.
- TDD: config validation tests first.
- **Acceptance:** new fields parse/default/validate; existing config unaffected;
  `go test ./internal/config/...` green.

## Chunk 2 — CertManager: in-memory PEM source (foundational)

- Scope: `internal/tls/certmanager.go`, `internal/tls/listener.go`.
- Refactor `NewCertManager` to accept a PEM **byte source** (file path OR
  in-memory bytes) behind one interface. File source keeps poll-watch; bytes
  source exposes `ReloadFromPEM(cert, key)`.
- Preserve all current behaviour (mTLS, expiry warn, hot-reload, fail-open).
- TDD: table tests for both sources; reload + mismatch cases.
- **Acceptance:** existing TLS tests pass unchanged; new bytes-source tests green.

## Chunk 3 — DB cert/key storage + admin API (Feature 1 backend)

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

## Chunk 3a — TLS recovery CLI + anti-lockout (lands WITH Chunk 3)

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

## Chunk 4 — DB cert/key UI (Feature 1 frontend)

- Scope: `frontend/src/pages/AdminServerPage.tsx`, `frontend/src/api/config.ts`.
- `cert_source` selector; cert textarea (paste PEM); key textarea (write-only,
  never pre-filled); current-cert metadata panel; save + restart flow.
- TDD: component tests (vitest) for source toggle + write-only key.
- **Acceptance:** operator configures DB cert/key end-to-end in the UI.
  Depends on Chunk 3.

## Chunk 5 — CSR generation backend (Feature 2 backend)

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

## Chunk 6 — CSR UI (Feature 2 frontend)

- Scope: `AdminServerPage.tsx`, `api/config.ts`.
- Subject fields, SAN list editor, algo dropdown, Generate → show/download CSR;
  guidance to paste the signed cert into the Feature-1 cert field.
- **Acceptance:** full CSR round-trip in the UI. Depends on Chunks 4,5.

## Chunk 7 — ACME core engine (Feature 3 core)

- Scope: new `internal/acme/` — account registration (persist account key in
  DB), order flow via `x/crypto/acme`, cert/key persistence to config_store,
  renewal scheduler (pre-expiry, exponential backoff 1h→24h cap), expiry-warning
  events. DB-backed storage layer.
- TDD: orchestration against a mock ACME directory (Pebble optional); renewal
  timing; storage round-trip. No network in unit tests.
- **Acceptance:** issue + persist + schedule-renew against a test CA, all in DB.
  Depends on Chunk 3 (storage patterns).

## Chunk 8 — ACME HTTP-01 + mode wiring

- Scope: `cmd/.../main.go` (replace the `acme` "not implemented" error),
  `internal/tls/listener.go` / `internal/acme`.
- Serve `/.well-known/acme-challenge/` on the redirect listener (challenge >
  redirect priority); ToS gate; staging-URL WARN; HSTS. **Fail open to plain
  HTTP when no cert can be obtained** (per the §2.4/§5.3 extension) so an ACME
  misconfig never locks out the UI; recoverable via the Chunk 3a CLI.
- TDD: http-01 challenge served; mode-selection; ToS-false refusal.
- **Acceptance:** `mode: acme` + http-01 obtains a cert against staging.
  Depends on Chunk 7.

## Chunk 9 — Route53 DNS-01 solver (Feature 3 DNS)

- Scope: new `internal/acme/route53.go`, `go.mod` (+aws-sdk-go-v2 subset).
- UPSERT/DELETE TXT `_acme-challenge.<domain>`, poll `GetChange` until `INSYNC`.
- Creds: env / IAM role / encrypted config secrets (resolution order); region +
  hosted-zone resolution.
- Note `go.mod` additions for the CI lockfile-scan plan; record any debt.
- TDD: solver against a mocked Route53 API (interface seam, no real AWS).
- **Acceptance:** dns-01 obtains a cert against staging via Route53.
  Depends on Chunks 7,8.

## Chunk 10 — ACME UI + AWS creds (Feature 3 frontend)

- Scope: `AdminServerPage.tsx`, `types/config.ts`, `api/config.ts`, backend cred
  storage.
- Render the missing `dns_provider` + `dns_provider_config` (region, hosted
  zone) fields, AWS cred inputs (secret, write-only), dns-01 conditional block,
  ToS toggle, staging warning, ACME cert-status panel (issued/expiry/last
  renewal).
- **Acceptance:** configure + run ACME dns-01 entirely from the UI.
  Depends on Chunk 9.

## Chunk 11 — Ignore files, packaging, docs, debt

- Scope: `.gitignore` / `.dockerignore` (no acme storage dir now it's DB; any new
  artifacts), packaging units (drop acme volume reqs if any), `plans/todo-*.md`,
  `plans/todo-tech-debt.md`.
- **Acceptance:** ignore files current; todos updated; debt recorded.

---

## Sequence / dependencies

`0 → 1 → 2 → 3 → 3a → 4` (Feature 1 done) `→ 5 → 6` (Feature 2 done)
`→ 7 → 8 → 9 → 10` (Feature 3 done) `→ 11`.

Chunk 3a (recovery CLI) must merge with/before anything that lets TLS material
live in the DB reach users — it gates Chunk 4, mTLS-via-DB, and ACME. Chunk 7
reuses Chunk 3 storage; 5/6 reuse Chunk 3 cert path. Do not start a chunk until
its dependencies are merged.

## Open items to confirm before Chunk 1

- Spec sign-off (Chunk 0).
- Deployment: any k8s/cert-manager file-mount users? (confirms `cert_source`
  coexist is worth keeping — assumed yes).
- CSR key-algo default (assumed ecdsa-p256, offer rsa-2048).
