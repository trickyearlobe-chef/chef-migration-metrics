# Active — TLS in DB, CSR generation, ACME/Route53

Branch: `feature/tls-db-certs-csr-acme`. Full plan + per-chunk detail:
[plans/tls-db-certs-csr-acme.md](tls-db-certs-csr-acme.md).

Sequence: `0 → 1 → 2 → 3 → 3a → 4 → 5 → 6 → 7 → 8a → 8b → 9 → 9a → 10 → 11`
(8c done; 8d — admin-UI behind-proxy toggle — depends on 8c).

## Done

Chunks 0–6 — spec split, config schema/validation, CertManager PEM source, DB
cert/key storage + admin API, the `tls reset`/`tls clear-ca` repair CLI (recovery
escape hatch), the DB cert/key UI (**Feature 1 complete**), the CSR generation
backend (`internal/tls/csr.go` + generate-csr endpoint + match-and-promote), and
the CSR UI (`generateCSR()` + AdminServerPage CSR panel — **Feature 2 complete**).
Chunk 7 — ACME core engine: new `internal/acme/` (DB storage layer, account +
order flow via `x/crypto/acme` behind a `Solver` seam, renewal scheduler with
1h→24h backoff + expiry-warning events). 3a unblocks everything below.
Chunk 8a — self-signed degraded fail-open foundation: fail-open now serves an
ephemeral self-signed cert over HTTPS (HSTS suppressed) instead of cleartext,
plain HTTP last-resort only; HSTS live-gate; status `kind` + in-place promotion
clears degraded; static `cert_source:db` retrofitted; banner kind-aware. Specs
§2.4/§6.3/§3.11 updated. 8a unblocks 8b's fail-open.
Chunk 8b — ACME HTTP-01 + mode wiring: new `internal/acme/http01.go`
`HTTP01Solver` (Solver seam + `Handler()`); `internal/tls`
`NewChallengeRedirectServer` (challenge > redirect priority, § 3.3) on port 80;
`main.go` `setupACME` replaces the not-implemented error — always comes up on
HTTPS (stored cert else 8a self-signed degraded), background Renewer +
`promotingIssuer` `ReloadFromPEM`-promotes in place clearing degraded; HTTPS
listener runs `HTTPRedirectPort:0`; staging WARN; `http-01`+`http_redirect_port:0`
and `dns-01` (Chunk 9) fail open to self-signed; `serverResult`/`awaitShutdown`
drain challenge server + renewer cancel. TDD, no network. Specs already accurate.
Chunk 8c — deliberate plain-HTTP CLI: new `cmd/.../tlsmode.go` adds
`tls mode <off|static|acme>` to the 3a dispatch (generalises `tls reset`, now a
thin alias); `--trusted-proxy[=true|false]` on `mode off` also sets
`server.trusted_proxy`. Required wiring: `server.trusted_proxy` is now a
config-store section (`KeyServerTrustedProxy` in `assembly.go` —
AllConfigKeys/assembleFields/ConfigToSections) so the CLI/UI value is read at
startup + reload (it was YAML-only and lost on migration before). Same env-only
store + section-preserving idempotency as 3a. Spec `tls.md § 9.1` (+ `tls-static.md`
pointer). TDD, no network.

## Next chunk

**Chunk 8d — UI: behind-proxy plain-HTTP toggle.** Admin-UI switch mirroring 8c:
a "Terminate TLS at a proxy (plain HTTP)" toggle that sets `server.tls.mode: off`
+ `server.trusted_proxy: true` (dynamic, no restart), with a lockout-guard
confirm/warning explaining HSTS is then driven by `X-Forwarded-Proto`. Scope:
`frontend/src/pages/AdminServerPage.tsx`, `api/config.ts`/`types/config.ts`
(surface `trusted_proxy`), and the backend server-config save path (must now also
persist `server.trusted_proxy` — 8c added the config-store key + assembly; check
the admin PUT handler in `handle_admin_config_server.go` writes it). TDD (vitest:
toggle sets mode off + trusted_proxy in the payload; warning shown). Depends on
Chunk 8c (done). See detail plan Chunk 8d.

## Notes

- CI supply-chain scanning work shipped in PR #49; remaining follow-ups moved to
  [plans/todo-ci.md](todo-ci.md).
