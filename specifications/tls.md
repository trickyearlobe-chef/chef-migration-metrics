# TLS and Certificate Management - Component Specification

> TLS termination and certificate lifecycle management for Chef Migration Metrics.

## TL;DR

Three listening modes: **plain HTTP** (`mode: off`), **static TLS** (`mode: static`, operator-provided cert/key), and **ACME automatic** (`mode: acme`, Let's Encrypt / ZeroSSL with HTTP-01 or DNS-01 challenges). Supports certificate reload on SIGHUP, filesystem watching, HTTP-to-HTTPS redirect listener, and HSTS. DNS-01 provider: Route 53. Backward compatible with deprecated boolean `tls.enabled`.
## 1. Listening Modes

### 1.1 Mode Selection

| `server.tls.mode` | Behaviour |
|--------------------|-----------|
| `off` (default) | Plain HTTP on `server.port`. No encryption. |
| `static` | HTTPS on `server.port` using certificate and key files from disk. |
| `acme` | HTTPS on `server.port` using certificates obtained automatically via ACME. |

### 1.2 HTTP-to-HTTPS Redirect

When TLS is active (`mode: static` or `mode: acme`), an optional secondary listener can serve HTTP-to-HTTPS redirects:

| Setting | Default | Description |
|---------|---------|-------------|
| `server.tls.http_redirect_port` | `0` (disabled) | When set to a valid port (e.g. `80`), the application starts a secondary HTTP listener that responds to all requests with a `301 Moved Permanently` redirect to the HTTPS equivalent URL. |

The redirect listener serves **only** redirects — no API responses, no static assets, no health checks. This prevents accidental exposure of sensitive data over plain HTTP.

`http_redirect_port` must differ from `server.port` (the HTTPS listen port). If they are equal, both listeners would attempt to bind the same port and one would fail at startup. This is rejected by validation — at startup and at save time (see § 2.6).

**Exception:** The ACME HTTP-01 challenge path (`/.well-known/acme-challenge/`) is served on the redirect listener when `mode: acme` and the HTTP-01 solver is in use (see section 3.4).

### 1.3 Port Defaults

The `server.port` default remains `8080` regardless of TLS mode. Operators who enable TLS and want the standard HTTPS port should explicitly set `server.port: 443`. This avoids surprising behaviour changes when toggling TLS on and off.

## 2. Static Certificate Mode

### 2.1 Configuration

```yaml
server:
  port: 443
  tls:
    mode: static
    cert_path: /etc/chef-migration-metrics/tls/server.crt
    key_path: /etc/chef-migration-metrics/tls/server.key
    ca_path: ""                    # Optional: CA bundle for client certificate validation
    min_version: "1.2"             # Minimum TLS version (default: "1.2")
    http_redirect_port: 80         # Optional: redirect HTTP to HTTPS
```

| Setting | Required | Default | Description |
|---------|----------|---------|-------------|
| `cert_path` | Yes (when `mode: static`) | — | Path to the PEM-encoded TLS certificate file. May include intermediate certificates (full chain). |
| `key_path` | Yes (when `mode: static`) | — | Path to the PEM-encoded private key file. Must be readable by the application process. |
| `ca_path` | No | `""` | Path to a PEM-encoded CA bundle. When set, the server enables mutual TLS (mTLS) and validates client certificates against this CA. |
| `min_version` | No | `"1.2"` | Minimum accepted TLS protocol version. Valid values: `"1.2"`, `"1.3"`. TLS 1.0 and 1.1 are not supported. |

### 2.2 Certificate Chain

The `cert_path` file should contain the full certificate chain in PEM format, ordered from leaf to root:

1. Server certificate
2. Intermediate CA certificate(s)
3. Root CA certificate (optional — clients typically have this in their trust store)

### 2.3 Certificate Reload

The application must support **automatic certificate reload** without restart:

- On receiving `SIGHUP`, the application re-reads `cert_path` and `key_path` from disk and begins serving the new certificate for subsequent TLS handshakes. Existing connections are not interrupted.
- Alternatively, the application may use filesystem watching (e.g. `fsnotify`) to detect changes to the certificate files and reload automatically. This is particularly useful in Kubernetes where cert-manager updates the Secret (and therefore the mounted files) in place.
- If the new certificate files are invalid (unparseable, mismatched key), the reload must fail gracefully: the application continues serving with the previous valid certificate and logs an `ERROR`-level message describing the failure.

### 2.4 Startup Behaviour (Fail-Open)

When `mode: static`, the application builds the TLS listener at startup using the
same load path as save-time preflight (§ 2.6): `cert_path`/`key_path` present and
readable, PEM parses, the key matches the certificate, `ca_path` (when set) is a
valid PEM bundle, and `min_version` is `"1.2"` or `"1.3"`.

If the listener **cannot** be built for any of these reasons, the application
MUST NOT exit. Instead it:

- Logs an `ERROR` on the `tls` scope describing the failure (never including
  private key material).
- Records a **degraded** state (`{degraded: true, reason}`) exposed on the status
  endpoint (§ 5.3).
- Starts a **plain HTTP** listener on the configured
  `server.listen_address:server.port` so the admin UI stays reachable to fix the
  problem.

This fail-open behaviour guarantees a bad certificate can never lock an operator
out of the UI. Save-time preflight (§ 2.6) makes this path rare — it normally
only triggers when certificate files change on disk underneath an already-running
deployment, or when `server.tls` was written before preflight existed.

An **expired** certificate that otherwise loads is not a failure: the listener
starts in static (HTTPS) mode and logs a `WARN` (operators may be mid-renewal).

There is no runtime auto-recovery — the plain listener is already bound to the
port. The degraded state clears on the next restart with a working certificate
(see § 5.3).

### 2.5 Environment Variable Overrides

| Environment Variable | Overrides |
|----------------------|-----------|
| `CHEF_MIGRATION_METRICS_SERVER_TLS_MODE` | `server.tls.mode` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_CERT_PATH` | `server.tls.cert_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_KEY_PATH` | `server.tls.key_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_CA_PATH` | `server.tls.ca_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_MIN_VERSION` | `server.tls.min_version` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_HTTP_REDIRECT_PORT` | `server.tls.http_redirect_port` |

### 2.6 Save-Time Preflight Validation

When the static-mode configuration is changed through the admin API
(`PUT /api/v1/admin/config/server`), the certificate is validated **before the
change is persisted**, using the same load path as startup (cert/key readable,
PEM parses, key matches certificate, and `ca_path` — when set — is a valid PEM
bundle). On failure the API returns `422` and does not write to the config
store, so an unusable certificate can never be committed and brick the listener
on the next restart. The redirect-vs-listen-port collision (§ 1.2) is checked the
same way. See [configuration-validation.md § Save-time preflight](configuration-validation.md).

## 3. ACME Automatic Certificate Management

### 3.1 Overview

When `server.tls.mode` is `acme`, the application uses the ACME protocol ([RFC 8555](https://tools.ietf.org/html/rfc8555)) to automatically obtain and renew TLS certificates from a CA such as [Let's Encrypt](https://letsencrypt.org/). Designed for internet-facing deployments reachable on port 80, or internal deployments using DNS-01 challenges. Use [`github.com/caddyserver/certmagic`](https://github.com/caddyserver/certmagic) (recommended) or [`golang.org/x/crypto/acme/autocert`](https://pkg.go.dev/golang.org/x/crypto/acme/autocert).

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
      dns_provider_config: {}    # Provider-specific key/value pairs
      storage_path: /var/lib/chef-migration-metrics/acme
      renew_before_days: 30
      agree_to_tos: false        # Must be true to accept CA's Terms of Service
    min_version: "1.2"
    http_redirect_port: 80
```

### 3.3 Challenge Types

**HTTP-01** — The CA sends an HTTP request to `http://<domain>/.well-known/acme-challenge/<token>`. Requires the application (or its redirect listener) to be reachable on port 80 from the internet. When `http_redirect_port` is set, the challenge handler is installed on the redirect listener (challenge path takes priority over redirect). When `http_redirect_port` is not set and `challenge: http-01`, log `ERROR` at startup advising the operator to set `http_redirect_port: 80`.

**DNS-01** — The CA verifies a TXT record at `_acme-challenge.<domain>`. The application creates and cleans up the DNS record via a provider API. Requires API credentials for a supported DNS provider. This is the only challenge type that works for internal/private domains or wildcard certificates.

### 3.4 Supported DNS Providers

| Provider | `dns_provider` value | Required config |
|----------|---------------------|-----------------|
| Amazon Route 53 | `route53` | `aws_region` (uses IAM role or env vars `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`) |

> **Security:** DNS provider credentials must never appear in configuration files in plain text. Use environment variables or provider-native credential mechanisms (e.g. AWS instance roles).

### 3.5 Certificate Storage

ACME account keys, certificates, private keys, and metadata are persisted to `acme.storage_path` (subdirectories: `accounts/`, `certificates/`). Must survive restarts to avoid re-registering accounts, hitting rate limits, or losing keys. File permissions: `0700` (directory) and `0600` (files) — set on startup if too permissive, logging `WARN` if it cannot.

### 3.6 Certificate Renewal

- Automatically renew before expiry, controlled by `renew_before_days` (default: 30).
- Exponential backoff on failure: 1 hour initial, 24 hour cap.
- Log `INFO` on success (with new expiry), `ERROR` on failure (with error and current expiry).
- When within 7 days of expiry without successful renewal, log `WARN` and send `certificate_expiry_warning` event if notifications are configured.

### 3.7 Terms of Service

`agree_to_tos` must be explicitly `true`. If `false`, refuse to start in ACME mode and log `ERROR` including the CA's ToS URL.

### 3.8 Staging vs Production

Use `ca_url: https://acme-staging-v02.api.letsencrypt.org/directory` for testing (untrusted certs, higher rate limits). Use `https://acme-v02.api.letsencrypt.org/directory` for production. Log `WARN` at startup when using a staging URL.

### 3.9 Environment Variable Overrides

| Environment Variable | Overrides |
|----------------------|-----------|
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_EMAIL` | `server.tls.acme.email` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CA_URL` | `server.tls.acme.ca_url` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CHALLENGE` | `server.tls.acme.challenge` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_DNS_PROVIDER` | `server.tls.acme.dns_provider` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_STORAGE_PATH` | `server.tls.acme.storage_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_AGREE_TO_TOS` | `server.tls.acme.agree_to_tos` |

### 3.10 Startup Validation

Fail fast if: `domains` empty; `email` empty; `agree_to_tos` not `true`; `storage_path` missing or not writable; `challenge: dns-01` with empty `dns_provider` or missing config keys; `challenge: http-01` with `http_redirect_port: 0`; `renew_before_days` outside 1–89; `ca_url` not a valid URL.

## 4. TLS Configuration Details

### 4.1 Cipher Suites

The application relies on Go's `crypto/tls` default cipher suite selection — secure, well-maintained, and intentionally not exposed to avoid misconfiguration. TLS 1.3 suites are fixed by the protocol. TLS 1.2 defaults prefer ECDHE with AEAD ciphers (AES-GCM, ChaCha20-Poly1305).

### 4.2 HSTS Header

When TLS is active, include on all HTTPS responses:

```
Strict-Transport-Security: max-age=63072000; includeSubDomains
```

The `max-age` of 2 years follows current best practice. Never sent on HTTP redirect responses.

## 5. Interaction with Other Components

### 5.1 Health Checks

The health check endpoint (`/api/v1/admin/status`) is served on the main listener. Kubernetes probes must use HTTPS or skip TLS verification (`scheme: HTTPS`). The `healthcheck` CLI subcommand must support `--insecure` for HTTPS without verification.

### 5.2 Logging

TLS-related events use the `tls` log scope. Key events:

- **INFO:** mode selected, certificate loaded/reloaded, ACME certificate obtained/renewed, redirect listener started.
- **WARN:** certificate expiring soon (within 7 days), staging CA URL detected.
- **ERROR:** certificate reload failed (continues with previous cert), ACME renewal failed (includes current expiry), ToS not accepted, static TLS failed at startup → fell back to plain HTTP (§ 2.4).

### 5.3 Degraded TLS Status and Recovery

When startup falls open to plain HTTP (§ 2.4), the degraded state is published on
a public, DB-independent endpoint so the UI can warn on every page — including
before login:

```
GET /api/v1/server/tls-status   →   200 OK
{ "degraded": true, "reason": "TLS listener setup failed: <cause>" }
```

When TLS is healthy (or `mode` is `off`/`acme`), the endpoint returns
`{ "degraded": false }`. The endpoint requires no authentication and never
queries the database, so the banner renders even when other subsystems are down.
The `reason` never contains private key material.

The frontend shows a prominent global banner whenever `degraded` is true:
**"TLS failed — running INSECURE. Fix the certificate and restart: <reason>"**.
The Server & TLS admin page surfaces the same state inline.

**Operator recovery:**

1. The banner confirms the server is serving plain HTTP and gives the reason.
2. Correct the certificate/key files (or the `cert_path`/`key_path`/`ca_path`
   values) under `server.tls`. Save-time preflight (§ 2.6) rejects an unusable
   pair before it is persisted.
3. Restart the service. On restart with a valid pair, static HTTPS resumes and
   the degraded state clears. There is no in-place recovery — the fallback plain
   listener holds the port until the process restarts.

## 6. Backward Compatibility

The previous schema used a boolean `server.tls.enabled` field:

```yaml
server:
  tls:
    enabled: true
    cert_path: /path/to/cert.pem
    key_path: /path/to/key.pem
```

- If `tls.enabled: true` and `tls.mode` is not set → treat as `mode: static`, log `WARN` about deprecation.
- If `tls.enabled: false` (or absent) and `tls.mode` is not set → default `mode: off`.
- If both are present → `tls.mode` wins, `tls.enabled` is ignored with a `WARN`.

## 7. Full Configuration Reference

### 7.1 Plain HTTP

```yaml
server:
  port: 8080
  tls:
    mode: off
```

### 7.2 Static Certificates

```yaml
server:
  port: 443
  tls:
    mode: static
    cert_path: /etc/chef-migration-metrics/tls/server.crt
    key_path: /etc/chef-migration-metrics/tls/server.key
    min_version: "1.2"
    http_redirect_port: 80
```

### 7.3 ACME with HTTP-01 (Let's Encrypt)

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
      storage_path: /var/lib/chef-migration-metrics/acme
      renew_before_days: 30
      agree_to_tos: true
    min_version: "1.2"
    http_redirect_port: 80
```

### 7.4 ACME with DNS-01 (Route 53)

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
        aws_region: us-east-1
        # Credentials from AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY env vars or instance role
      storage_path: /var/lib/chef-migration-metrics/acme
      renew_before_days: 30
      agree_to_tos: true
    min_version: "1.2"
```

## 8. Security Considerations

### 8.1 Private Key Protection

- Private key files (static and ACME-generated) must have permissions no more permissive than `0600`. Log `WARN` if more permissive.
- ACME storage directory permissions must be `0700`.
- Private keys must never be logged, included in error messages, or exposed via any API endpoint.

### 8.2 Rate Limits

Let's Encrypt rate limits:

| Limit | Value |
|-------|-------|
| Certificates per Registered Domain | 50 per week |
| Duplicate Certificate | 5 per week |
| Failed Validation | 5 per account, per hostname, per hour |

The application must not request certificates unnecessarily. Persistent storage (section 3.6) is critical to avoid hitting rate limits on restart.