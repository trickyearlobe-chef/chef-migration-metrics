# TODO — TLS refinements (backlog)

General TLS feature refinements (non-lockout). Anti-lockout gaps and the
443-lifeboat idea live in [todo-tls-antilockout.md](todo-tls-antilockout.md).

## 1. Cert-chain display + reorder-on-save — DONE

Raised 2026-06-10. Both parts landed:

- **Display the chain** (W1-A, branch `feature/tls-chain-display`):
  `ChainMetadataFromPEM` surfaces per-cert CN/SANs/issuer/validity + structural
  `role` (leaf/intermediate/root) in `tls_certificate_info` (an array) for
  static-DB and ACME; UI renders a per-cert chain panel.
- **Reorder on save** (W1-B, branch `feature/tls-chain-reorder`):
  `ReorderChainPEM` sorts an operator-supplied `cert_source: db` bundle into
  leaf → intermediate(s) → root before preflight/storage (issuer→subject linking,
  non-self-signed leaf first). Incomplete/non-linking bundles are stored with a
  non-fatal `warnings` entry in the save response, never rejected. CSR-promoted
  and `cert_source: file` bundles are left as-is (per `tls-static.md` § 2.2).

Follow-up (small, not blocking): the reorder warning is currently transient
(PUT-response `warnings` only). Persisting/surfacing it on the GET status panel
(re-derive from the stored chain) + FE display of the warning is unbuilt.
