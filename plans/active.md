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
3a unblocks everything below.

## Next chunk

**Chunk 7 — ACME core engine (Feature 3 core).** Scope: new `internal/acme/` —
account registration (persist account key in DB), order flow via
`golang.org/x/crypto/acme`, cert/key persistence to `config_store`, renewal
scheduler (pre-expiry, exponential backoff 1h→24h cap), expiry-warning events,
DB-backed storage layer. TDD: orchestration against a mock ACME directory (Pebble
optional), renewal timing, storage round-trip — no network in unit tests. Reuses
Chunk 3 storage patterns. Depends on Chunk 3 (done).

## Notes

- CI supply-chain scanning work shipped in PR #49; remaining follow-ups moved to
  [plans/todo-ci.md](todo-ci.md).
