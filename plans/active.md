# Active — TLS in DB, CSR generation, ACME/Route53

Branch: `feature/tls-db-certs-csr-acme`. Full plan + per-chunk detail:
[plans/tls-db-certs-csr-acme.md](tls-db-certs-csr-acme.md).

Sequence: `0 → 1 → 2 → 3 → 3a → 4 → 5 → 6 → 7 → 8 → 9 → 9a → 10 → 11`.

## Done

Chunks 0, 1, 2, 3, 3a, 4, 5 — spec split, config schema/validation, CertManager
PEM source, DB cert/key storage + admin API, the `tls reset`/`tls clear-ca`
repair CLI (recovery escape hatch), the DB cert/key UI (Feature 1 complete), and
the CSR generation backend (`internal/tls/csr.go` + generate-csr endpoint +
match-and-promote — Feature 2 backend). 3a unblocks everything below.

## Next chunk

**Chunk 6 — CSR UI (Feature 2 frontend).** Scope: `AdminServerPage.tsx`,
`api/config.ts`. Subject fields, SAN list editor, algo dropdown, Generate →
show/download the returned CSR PEM; guidance to paste the signed cert back into
the Feature-1 cert field (the PUT db path auto-promotes the pending key). TDD:
component tests. Depends on Chunks 4, 5 (both done).

## Notes

- CI supply-chain scanning work shipped in PR #49; remaining follow-ups moved to
  [plans/todo-ci.md](todo-ci.md).
