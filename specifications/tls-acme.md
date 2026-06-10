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
      register_hostname: false   # Publish an A record for each domain (§ 3.13)
      hostname_ttl: 60           # A-record TTL in seconds (§ 3.13)
      hostname_interface: ""     # Use this interface's IPv4 (§ 3.13)
      hostname_ip: ""            # Use this literal IPv4 (highest precedence, § 3.13)
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
the CA to validate. The TXT record is removed after validation. The same
`route53:ChangeResourceRecordSets` permission also covers optional hostname
self-registration (§ 3.13) — no additional IAM is required.

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
- It serves the admin UI over HTTPS using an **ephemeral self-signed certificate**
  (HSTS suppressed), so the configuration can be fixed over an encrypted channel;
  it falls back to **plain HTTP** only as a last resort if the self-signed
  listener itself cannot be brought up.

The server therefore **always comes up on HTTPS**: the stored issued cert if one
exists in the DB (§ 3.5), otherwise the self-signed degraded cert. The renewal
scheduler keeps running (§ 3.6) and, once issuance succeeds, swaps the real cert
in **without a restart** (clearing the degraded state and resuming HSTS). If a
valid cert already exists in the DB it is served immediately while renewal is
retried with backoff; only a total absence of a usable cert is degraded. Recovery
from a stuck ACME configuration is the repair CLI (`tls reset`) — see
[tls.md § 6.3](tls.md#63-degraded-tls-status-and-recovery).

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
      register_hostname: true      # Also publish an A record per domain (§ 3.13)
      renew_before_days: 30
      agree_to_tos: true
    min_version: "1.2"
```

### 3.13 Hostname Self-Registration (Route 53 A record)

When `dns_provider: route53` is configured, the application can also publish and
maintain a DNS **A record** for the server itself, so the FQDN clients use
resolves to the host without the operator hand-editing DNS. It reuses the same
Route 53 credentials, hosted zone, and `route53:ChangeResourceRecordSets`
permission as the DNS-01 solver (§ 3.4) — no additional configuration or IAM.

This is **opt-in** and **off by default** (`register_hostname: false`), so
deployments whose FQDN is managed manually (external DNS, a load balancer, or
another automation system) are unaffected.

**Names.** One A record is UPSERTed for **each name in `acme.domains`**, so the
published names always match the issued certificate's SANs. A wildcard domain
(`*.example.com`) is skipped with a `WARN` — an A record cannot be published for
a wildcard name.

**IP address resolution** (first non-empty wins):

1. `hostname_ip` — a literal IPv4 address, published verbatim.
2. `hostname_interface` — the global-unicast IPv4 of the named interface
   (e.g. `eth0`).
3. *Auto-detect* (default) — the IPv4 of the interface that carries the host's
   **default route**, i.e. the address the OS would source off-link traffic
   from. Determined without sending packets.

When `hostname_ip` or `hostname_interface` is set but unusable (not a valid
IPv4 / no global-unicast IPv4 on that interface), registration is **skipped with
an `ERROR`** — there is no silent fall-through to auto-detect, because the
operator was explicit. Only the auto-detect path chooses an address on its own.

**TTL.** `hostname_ttl` (default `60` seconds). Low by design: the host IP is
often DHCP-assigned and may change.

**Lifecycle.**

- The A record(s) are UPSERTed at ACME startup, on each renewal cycle, and when
  relevant configuration changes; the solver polls `GetChange` to `INSYNC`.
- Re-asserting on every renewal cycle means a changed DHCP lease is corrected
  automatically (the next cycle UPSERTs the new IP).
- The record is **not deleted on shutdown** — the server is expected to restart.
  Turning `register_hostname` off stops further updates but **leaves the existing
  record** for the operator to remove (or repoint) manually.

**Fail-soft, orthogonal to issuance.** DNS-01 validates via a TXT record, so
A-record self-registration is independent of certificate issuance. A
registration failure (no IPv4 detectable, an explicit IP/interface unusable, or
a Route 53 error) logs an `ERROR` on the `tls` scope and is surfaced in TLS
status, but **never** blocks issuance, renewal, or the fail-open path (§ 3.11),
and never aborts startup.

> **Security.** Self-registration publishes the host's IP in the configured
> hosted zone. For internet-facing zones this exposes the address publicly (as
> any A record does); internal deployments using a private hosted zone keep it
> internal. Credentials remain encrypted config-store secrets (§ 3.4).
