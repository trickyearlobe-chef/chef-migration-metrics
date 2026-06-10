# TODO — TLS refinements (backlog)

General TLS feature refinements (non-lockout). Anti-lockout gaps and the
443-lifeboat idea live in [todo-tls-antilockout.md](todo-tls-antilockout.md).

## 1. Cert-chain display + reorder-on-save

Raised 2026-06-10. Two parts:

- **Display the chain** wherever a cert is shown (static DB/file, CSR-promoted,
  ACME-issued): per-cert CN, subject alt names, not-before / not-after, issuer —
  so an operator can see the full leaf → intermediate → root chain and its expiry
  at a glance.
- **Reorder on save (static upload only):** when an operator pastes/uploads a PEM
  bundle, sort the certs into the correct **leaf → intermediate(s) → CA** order
  before storing, rather than rejecting or trusting input order. Order by matching
  each cert's issuer to the next cert's subject (leaf = the cert whose subject is
  not an issuer of any other in the bundle).

Open questions for the spec:
- Where the metadata is parsed/surfaced (extend `internal/tls/metadata.go`?) and
  the API/UI shape (status panel already shows `tls_certificate_info`).
- Reorder scope: leaf+intermediates only, or also detect/separate a self-signed
  root? Reject vs. warn on a broken/incomplete chain (missing intermediate)?
- Does reorder apply to CSR-promoted and DB-stored bundles too, or static-upload
  paste only?

Touches `tls-static.md` (and the status surface in `tls.md`). New TLS follow-on,
own spec; independent of the 443-lifeboat item.
