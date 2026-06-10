# Active — TLS in DB, CSR generation, ACME/Route53

Branch: `feature/tls-db-certs-csr-acme`. Full plan + per-chunk detail:
[plans/tls-db-certs-csr-acme.md](tls-db-certs-csr-acme.md).

Sequence: `0 → 1 → 2 → 3 → 3a → 4 → 5 → 6 → 7 → 8 → 9 → 9a → 10 → 11`.

## Done

Chunks 0–6 — spec split, config schema/validation, CertManager PEM source, DB
cert/key storage + admin API, the `tls reset`/`tls clear-ca` repair CLI (recovery
escape hatch), the DB cert/key UI (**Feature 1 complete**), the CSR generation
backend (`internal/tls/csr.go` + generate-csr endpoint + match-and-promote), and
the CSR UI (`generateCSR()` + AdminServerPage CSR panel — **Feature 2 complete**).
Chunk 7 — ACME core engine: new `internal/acme/` (DB storage layer, account +
order flow via `x/crypto/acme` behind a `Solver` seam, renewal scheduler with
1h→24h backoff + expiry-warning events). 3a unblocks everything below.

## Next chunk

**Chunk 8 — ACME HTTP-01 + mode wiring.** Scope: `cmd/.../main.go` (replace the
`acme` "not implemented" error), `internal/tls/listener.go` / `internal/acme`.
Serve `/.well-known/acme-challenge/` on the redirect listener (challenge >
redirect priority) via an HTTP-01 `Solver`; ToS gate; staging-URL WARN; HSTS.
Fail open to plain HTTP when no cert can be obtained (§ 2.4/3.11) — recoverable
via the Chunk 3a CLI. TDD: http-01 challenge served; mode-selection; ToS-false
refusal. Depends on Chunk 7 (done).

## Notes

- CI supply-chain scanning work shipped in PR #49; remaining follow-ups moved to
  [plans/todo-ci.md](todo-ci.md).
