# Active — TLS in DB, CSR generation, ACME/Route53

Branch: `feature/tls-db-certs-csr-acme`. Full plan + per-chunk detail:
[plans/tls-db-certs-csr-acme.md](tls-db-certs-csr-acme.md).

Sequence: `0 → 1 → 2 → 3 → 3a → 4 → 5 → 6 → 7 → 8 → 9 → 9a → 10 → 11`.

## Done

Chunks 0, 1, 2, 3, 3a, 4 — spec split, config schema/validation, CertManager PEM
source, DB cert/key storage + admin API, the `tls reset`/`tls clear-ca` repair
CLI (recovery escape hatch), and the DB cert/key UI (Feature 1 frontend
complete). 3a unblocks everything below.

## Next chunk

**Chunk 5 — CSR generation backend (Feature 2 backend).** Scope: new
`internal/tls/csr.go`, `POST /api/v1/admin/config/server/generate-csr`,
configstore lifecycle. Generate key → store as `private_key.pending` (secret);
build CSR → return PEM. Signed-cert upload (Chunk 3 path) matches against the
pending key; on match promote pending → active, else `422`. TDD: CSR
content/promotion/mismatch. Depends on Chunk 3 (done).

## Notes

- CI supply-chain scanning work shipped in PR #49; remaining follow-ups moved to
  [plans/todo-ci.md](todo-ci.md).
