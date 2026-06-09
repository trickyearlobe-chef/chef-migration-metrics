# Active — TLS in DB, CSR generation, ACME/Route53

Branch: `feature/tls-db-certs-csr-acme`. Full plan + per-chunk detail:
[plans/tls-db-certs-csr-acme.md](tls-db-certs-csr-acme.md).

Sequence: `0 → 1 → 2 → 3 → 3a → 4 → 5 → 6 → 7 → 8 → 9 → 9a → 10 → 11`.

## Done

Chunks 0, 1, 2, 3, 3a — spec split, config schema/validation, CertManager PEM
source, DB cert/key storage + admin API, and the `tls reset`/`tls clear-ca`
repair CLI (recovery escape hatch). 3a unblocks everything below.

## Next chunk

**Chunk 4 — DB cert/key UI (Feature 1 frontend).** Scope:
`frontend/src/pages/AdminServerPage.tsx`, `frontend/src/api/config.ts`.
`cert_source` selector, cert textarea (paste PEM), write-only key textarea,
current-cert metadata panel, save + restart flow. TDD: vitest component tests
for the source toggle + write-only key. Depends on Chunk 3 (done).

## Notes

- CI supply-chain scanning work shipped in PR #49; remaining follow-ups moved to
  [plans/todo-ci.md](todo-ci.md).
