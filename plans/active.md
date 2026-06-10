# Active — TLS in DB, CSR generation, ACME/Route53

Branch: `feature/tls-db-certs-csr-acme`. Full plan + per-chunk detail:
[plans/tls-db-certs-csr-acme.md](tls-db-certs-csr-acme.md).

Sequence: `0 → 1 → 2 → 3 → 3a → 4 → 5 → 6 → 7 → 8a → 8b → 9 → 9a → 10 → 11`
(8c — deliberate plain-HTTP CLI — depends only on 3a, lands any time).

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

## Next chunk

**Chunk 8b — ACME HTTP-01 + mode wiring.** Replace the `acme` "not implemented"
error in `main.go`. New `internal/acme/http01.go` `HTTP01Solver` (Solver seam +
`Handler()` for `/.well-known/acme-challenge/<token>`), installed on the redirect
listener (challenge > redirect priority, § 3.3) via a `NewChallengeRedirectServer`
constructor; HTTPS `Listener` runs with `HTTPRedirectPort: 0` (port 80 owned by
the challenge/redirect server). Build `acme.Storage`/`Manager`/`Renewer`; always
come up on HTTPS — stored real cert if present, else the **8a self-signed** cert;
the Renewer obtains + `ReloadFromPEM`-promotes self-signed → real in place
(clears degraded via `SetOnReload`). Staging WARN; `http-01` + `http_redirect_port:0`
→ ERROR. Shutdown wiring for the challenge server + renewer cancel. TDD (fakes,
no network). Depends on Chunks 7, 8a (done).

Then **8c** (`tls mode off --trusted-proxy` CLI, deliberate behind-proxy plain
HTTP, depends on 3a) and **8d** (matching admin-UI toggle). See detail plan.

## Notes

- CI supply-chain scanning work shipped in PR #49; remaining follow-ups moved to
  [plans/todo-ci.md](todo-ci.md).
