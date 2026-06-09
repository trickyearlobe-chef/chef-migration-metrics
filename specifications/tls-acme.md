# TLS — ACME Automatic Certificate Management

> Part of the [TLS and Certificate Management](tls.md) spec. Covers `mode: acme`:
> automatic issuance/renewal via the ACME protocol, HTTP-01 and Route 53 DNS-01
> challenges, and DB-backed (encrypted) state storage.

## 3. ACME Automatic Certificate Management

### 3.1 Overview

When `server.tls.mode` is `acme`, the application uses the ACME protocol
([RFC 8555](https://tools.ietf.org/html/rfc8555)) to automatically obtain and
renew TLS certificates from a CA such as
[Let's Encrypt](https://letsencrypt.org/). Designed for internet-facing
deployments reachable on port 80, or internal deployments using DNS-01
challenges.

**Implementation:** the low-level [`golang.org/x/crypto/acme`](https://pkg.go.dev/golang.org/x/crypto/acme)
client (already in `go.mod` — zero new modules). The product owns the
account / order / challenge / renewal / storage orchestration directly. Higher
level libraries (`certmagic`, `lego`, `autocert`) are deliberately **not** used:
they substantially enlarge the dependency/lockfile surface (`lego` pulls in the
full set of DNS-provider SDKs), which conflicts with the supply-chain
minimisation policy.

### 3.2 Configuration

```yaml
server:
  port: 443
  tls:
    mode: acme
    acme:
      domains: [chef-metrics.example.com]
      email: admin@example.com
      ca_url: https://acme-v02.api.letsencrypt.org/directory
      trusted_roots: ""          # Optional: PEM file of additional CA roots
      challenge: http-01         # http-01 | dns-01
      dns_provider: ""           # route53 (required when challenge: dns-01)
      dns_provider_config: {}    # Provider-specific key/value pairs (§ 3.4)
      renew_before_days: 30
      agree_to_tos: false        # Must be true to accept CA's Terms of Service
    min_version: "1.2"
    http_redirect_port: 80
```

ACME state (account key, issued cert/key) is stored in the **encrypted config
store**, not on disk — there is no `storage_path` (§ 3.5).

### 3.3 Challenge Types

**HTTP-01** — The CA sends an HTTP request to
`http://<domain>/.well-known/acme-challenge/<token>`. Requires the application (or
its redirect listener) to be reachable on port 80 from the internet. When
`http_redirect_port` is set, the challenge handler is installed on the redirect
listener (challenge path takes priority over redirect). When `http_redirect_port`
is not set and `challenge: http-01`, log `ERROR` at startup advising the operator
to set `http_redirect_port: 80`.

**DNS-01** — The CA verifies a TXT record at `_acme-challenge.<domain>`. The
application creates and cleans up the DNS record via a provider API. Requires API
credentials for a supported DNS provider. This is the only challenge type that
works for internal/private domains or wildcard certificates.

### 3.4 Supported DNS Providers

| Provider | `dns_provider` value | Required `dns_provider_config` |
|----------|---------------------|--------------------------------|
| Amazon Route 53 | `route53` | `region`, `hosted_zone_id` |

**Route 53 client.** The DNS-01 solver uses the `aws-sdk-go-v2` Route 53 subset
(`config`, `credentials`, `service/route53`, `smithy-go`). The solver UPSERTs the
TXT record, then polls `GetChange` until the change set is `INSYNC` before telling
the CA to validate. The TXT record is removed after validation.

**AWS credential resolution order:**

1. Encrypted config-store secrets `server.tls.acme.route53.access_key_id` /
   `server.tls.acme.route53.secret_access_key` (`secret: true`), when set.
2. Environment variables `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`.
3. IAM instance role (the AWS default credential chain).

`region` and `hosted_zone_id` come from `dns_provider_config` (or
`server.tls.acme.route53.region` / `.hosted_zone_id`).

> **Security:** DNS provider credentials must never appear in plain text in
> configuration files. Store them as encrypted config-store secrets (above) or
> supply them via environment variables / an AWS instance role. Config-store
> secrets are `secret: true` and never returned by any API.

### 3.5 Certificate Storage (DB)

ACME account key, issued certificate, private key, and metadata are persisted to
the **encrypted config store**, not the filesystem. This survives restarts
(avoiding re-registration, rate limits, and lost keys) without a persistent
volume, and unifies storage with the static DB cert path
([tls-static.md § 2.7](tls-static.md)).

| Key | `secret` | Contents |
|-----|----------|----------|
| `server.tls.acme.account_key` | `true` | ACME account private key. |
| `server.tls.acme.cert` | `false` | Issued leaf + chain PEM. |
| `server.tls.acme.key` | `true` | Issued certificate private key. |
| `server.tls.acme.route53.access_key_id` | `true` | Route 53 access key (DNS-01). |
| `server.tls.acme.route53.secret_access_key` | `true` | Route 53 secret key (DNS-01). |
| `server.tls.acme.route53.region` | `false` | Route 53 region. |
| `server.tls.acme.route53.hosted_zone_id` | `false` | Route 53 hosted zone ID. |

Secret values use the same encryption stack as all other config-store secrets
(AES-256-GCM, HKDF, per-row nonce, AAD; master key
`CMM_CREDENTIAL_ENCRYPTION_KEY`). Private keys are never returned by any API.

### 3.6 Certificate Renewal

- Automatically renew before expiry, controlled by `renew_before_days` (default: 30).
- Exponential backoff on failure: 1 hour initial, 24 hour cap.
- Log `INFO` on success (with new expiry), `ERROR` on failure (with error and current expiry).
- When within 7 days of expiry without successful renewal, log `WARN` and send `certificate_expiry_warning` event if notifications are configured.

### 3.7 Terms of Service

`agree_to_tos` must be explicitly `true`. If `false`, the application does not
attempt ACME issuance and logs `ERROR` including the CA's ToS URL; per § 3.11 it
then falls open to plain HTTP rather than aborting startup.

### 3.8 Staging vs Production

Use `ca_url: https://acme-staging-v02.api.letsencrypt.org/directory` for testing
(untrusted certs, higher rate limits). Use
`https://acme-v02.api.letsencrypt.org/directory` for production. Log `WARN` at
startup when using a staging URL.

### 3.9 Environment Variable Overrides

| Environment Variable | Overrides |
|----------------------|-----------|
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_EMAIL` | `server.tls.acme.email` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CA_URL` | `server.tls.acme.ca_url` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CHALLENGE` | `server.tls.acme.challenge` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_DNS_PROVIDER` | `server.tls.acme.dns_provider` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_AGREE_TO_TOS` | `server.tls.acme.agree_to_tos` |

There is no `storage_path` override — ACME state lives in the DB (§ 3.5).

### 3.10 Startup Validation

Structural validation fails fast if: `domains` empty; `email` empty;
`challenge: dns-01` with empty `dns_provider` or missing required config keys
(`region`, `hosted_zone_id` for `route53`, unless supplied via env/role);
`challenge: http-01` with `http_redirect_port: 0`; `renew_before_days` outside
1–89; `ca_url` not a valid URL.

`agree_to_tos: false` and runtime issuance failures are **not** fatal — they are
handled by fail-open (§ 3.11), so a misconfigured ACME deployment never locks the
operator out of the UI.

### 3.11 Fail-Open When No Certificate Can Be Obtained

ACME mode must never lock an operator out. When a usable certificate cannot be
obtained or loaded — ToS not accepted, the order/challenge fails, DNS-01 creds
are missing/invalid, the CA is unreachable, or no previously issued cert is yet
in the DB — the application MUST NOT exit. As with static fail-open
([tls-static.md § 2.4](tls-static.md)):

- It logs an `ERROR` on the `tls` scope (never including key material).
- It records the **degraded** state on the status endpoint
  ([tls.md § 6.3](tls.md#63-degraded-tls-status-and-recovery)).
- It serves the admin UI over **plain HTTP** so the configuration can be fixed.

If a valid cert already exists in the DB (§ 3.5), the listener serves it while
renewal is retried with backoff (§ 3.6); only a total absence of a usable cert
falls open. Recovery from a stuck ACME configuration is the repair CLI
(`tls reset`) — see [tls.md § 6.3](tls.md#63-degraded-tls-status-and-recovery).

### 3.12 Configuration Reference

**ACME with HTTP-01 (Let's Encrypt):**

```yaml
server:
  port: 443
  tls:
    mode: acme
    acme:
      domains: [chef-metrics.example.com]
      email: admin@example.com
      ca_url: https://acme-v02.api.letsencrypt.org/directory
      challenge: http-01
      renew_before_days: 30
      agree_to_tos: true
    min_version: "1.2"
    http_redirect_port: 80
```

**ACME with DNS-01 (Route 53):**

```yaml
server:
  port: 443
  tls:
    mode: acme
    acme:
      domains: [chef-metrics.internal.example.com]
      email: admin@example.com
      ca_url: https://acme-v02.api.letsencrypt.org/directory
      challenge: dns-01
      dns_provider: route53
      dns_provider_config:
        region: us-east-1
        hosted_zone_id: Z0123456789ABCDEFGHIJ
        # Credentials from encrypted config-store secrets, AWS_ACCESS_KEY_ID/
        # AWS_SECRET_ACCESS_KEY env vars, or an IAM instance role (§ 3.4).
      renew_before_days: 30
      agree_to_tos: true
    min_version: "1.2"
```
