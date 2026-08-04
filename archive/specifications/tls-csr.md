# TLS — CSR Generation

> Part of the [TLS and Certificate Management](tls.md) spec. In-app Certificate
> Signing Request generation for operators whose CA issues static certificates:
> generate a keypair + CSR in the product, submit the CSR to the CA, paste the
> signed certificate back. Requires `cert_source: db` ([tls-static.md § 2.7](tls-static.md)).

## 4. CSR Generation

### 4.1 Purpose

Operators using an internal/enterprise CA cannot use ACME but still want
UI-driven certificate management. CSR generation lets the product create the
private key and a Certificate Signing Request without the key ever leaving the
server. The operator submits the CSR to their CA out of band, then pastes the
signed certificate back through the static DB cert path
([tls-static.md § 2.7](tls-static.md)). The private key is never exported.

### 4.2 Endpoint

```
POST /api/v1/admin/config/server/generate-csr
```

Generates a new keypair, stores the private key as **pending** (§ 4.5), builds
the CSR, and returns the CSR PEM (downloadable). The response never contains the
private key.

### 4.3 Inputs

| Field | Required | Description |
|-------|----------|-------------|
| `common_name` | Yes | Subject CN (typically the primary FQDN). |
| `organization` | No | Subject O. |
| `organizational_unit` | No | Subject OU. |
| `country` | No | Subject C (2-letter ISO code). |
| `dns_sans` | No | List of DNS Subject Alternative Names. |
| `ip_sans` | No | List of IP Subject Alternative Names. |
| `key_algorithm` | No | Keypair algorithm (§ 4.4). Default `ecdsa-p256`. |

At least one identifier (CN or a SAN) must be present. The CN, when set, should
also appear in `dns_sans` (modern CAs/clients validate SANs, not CN).

### 4.4 Key Algorithms

| `key_algorithm` | Notes |
|-----------------|-------|
| `ecdsa-p256` | **Default.** Modern, small, fast; widely supported. |
| `ecdsa-p384` | Larger ECDSA curve. |
| `rsa-2048` | Maximum compatibility with older CAs/clients. |
| `rsa-3072` | — |
| `rsa-4096` | — |

### 4.5 Pending-Key Lifecycle

1. **Generate.** A new private key is generated for the chosen algorithm and
   stored at config key `server.tls.private_key.pending`
   (`secret: true`, encrypted at rest — same stack as
   [tls-static.md § 2.7](tls-static.md)). Any prior pending key is overwritten.
2. **CSR returned.** The CSR is built over the pending key with the requested
   subject and SANs and returned as PEM. The active cert/key
   (`server.tls.certificate` / `server.tls.private_key`) are untouched — the
   listener keeps serving the current certificate while signing is in progress.
3. **Sign externally.** The operator submits the CSR to their CA and obtains a
   signed certificate (+ chain).
4. **Match-and-promote.** The operator pastes the signed certificate through the
   static DB cert save path (`PUT /api/v1/admin/config/server`,
   [tls-static.md § 2.6](tls-static.md)). Save-time preflight verifies the
   certificate's public key **matches the pending private key**:
   - **Match:** the pending key is promoted to active — written to
     `server.tls.private_key`, the certificate written to
     `server.tls.certificate`, and `server.tls.private_key.pending` is deleted.
     The listener reloads (§ 2.3) and serves the new certificate.
   - **No match** (cert does not correspond to the pending key, or no pending key
     exists): the API returns `422` and nothing is written; the pending key is
     left intact for a corrected upload.

A pending key persists across restarts (it is in the DB) until promoted or
overwritten by a new CSR generation. Generating a new CSR before promoting the
previous one discards the earlier pending key.

### 4.6 Security

- The private key (active and pending) is `secret: true` and is never returned by
  any API, logged, or included in error messages.
- CSR generation does not weaken fail-open: the active certificate is only
  replaced on a successful match-and-promote, so an in-flight CSR can never brick
  the listener.
