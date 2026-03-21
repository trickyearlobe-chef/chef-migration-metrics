# TLS and Certificate Management - Component Specification

> Component specification for TLS termination and certificate lifecycle management in Chef Migration Metrics.
> See the [top-level specification](../Specification.md) for project overview and scope.

---

## TL;DR

Three listening modes: **plain HTTP** (`mode: off`), **static TLS** (`mode: static`, operator-provided cert/key with optional mTLS), and **ACME automatic** (`mode: acme`, Let's Encrypt / ZeroSSL with HTTP-01, TLS-ALPN-01, or DNS-01 challenges). Supports certificate reload on SIGHUP, filesystem watching, HTTP-to-HTTPS redirect listener, HSTS, OCSP stapling, and multi-replica coordination via file locking. DNS-01 providers: Route 53, Cloudflare, Google Cloud DNS, Azure DNS, RFC 2136. Backward compatible with deprecated boolean `tls.enabled`. See `configuration/Specification.md` for the full YAML schema.

---

## Overview

Chef Migration Metrics supports three modes for serving HTTP traffic:

1. **Plain HTTP** — no encryption, suitable for development, localhost, or when TLS is terminated by an upstream reverse proxy or load balancer.
2. **TLS with externally-managed certificates** — the operator provides certificate and key files generated outside the application (e.g. from an internal CA, purchased certificates, or certificates issued by cert-manager in Kubernetes).
3. **TLS with ACME automatic certificate management** — the application obtains and renews certificates automatically using the ACME protocol (e.g. Let's Encrypt, ZeroSSL, or any RFC 8555-compliant CA).

All three modes are mutually exclusive and selected via the `server.tls.mode` configuration setting. The application must serve on a single listener — it does not simultaneously serve both HTTP and HTTPS on different ports (though an optional HTTP-to-HTTPS redirect listener is supported when TLS is active).

---

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

**Exception:** The ACME HTTP-01 challenge path (`/.well-known/acme-challenge/`) is served on the redirect listener when `mode: acme` and the HTTP-01 solver is in use (see section 3.4).

### 1.3 Port Defaults

The `server.port` default remains `8080` regardless of TLS mode. Operators who enable TLS and want the standard HTTPS port should explicitly set `server.port: 443`. This avoids surprising behaviour changes when toggling TLS on and off.

---

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

### 2.4 Startup Validation

On startup, the application must fail fast if:

- `mode` is `static` but `cert_path` or `key_path` is missing or empty.
- The certificate file does not exist or is not readable.
- The key file does not exist or is not readable.
- The certificate and key do not form a valid pair (the public key in the certificate does not match the private key).
- The certificate is expired at startup time (log `WARN` but do not prevent startup — operators may be in the process of renewing).
- `ca_path` is set but the file does not exist or is not a valid PEM bundle.
- `min_version` is not one of `"1.2"` or `"1.3"`.

### 2.5 Environment Variable Overrides

| Environment Variable | Overrides |
|----------------------|-----------|
| `CHEF_MIGRATION_METRICS_SERVER_TLS_MODE` | `server.tls.mode` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_CERT_PATH` | `server.tls.cert_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_KEY_PATH` | `server.tls.key_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_CA_PATH` | `server.tls.ca_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_MIN_VERSION` | `server.tls.min_version` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_HTTP_REDIRECT_PORT` | `server.tls.http_redirect_port` |

---

## 3. ACME Automatic Certificate Management

### 3.1 Overview

When `server.tls.mode` is `acme`, the application uses the ACME protocol ([RFC 8555](https://tools.ietf.org/html/rfc8555)) to automatically obtain and renew TLS certificates from a Certificate Authority such as [Let's Encrypt](https://letsencrypt.org/).

This mode is designed for internet-facing deployments where the application is directly reachable on ports 80 and/or 443, or where DNS-based challenges can be used for internal deployments.

### 3.2 Implementation

The ACME client must be implemented using the [`golang.org/x/crypto/acme/autocert`](https://pkg.go.dev/golang.org/x/crypto/acme/autocert) package or a more feature-rich library such as [`github.com/caddyserver/certmagic`](https://github.com/caddyserver/certmagic). CertMagic is recommended because it supports:

- Multiple ACME challenge types (HTTP-01, TLS-ALPN-01, DNS-01)
- Pluggable DNS providers for DNS-01 challenges
- Automatic renewal well before expiry
- On-demand certificate issuance
- Persistent storage backends
- OCSP stapling
- Clean integration with Go's `crypto/tls` and `net/http`

The choice of library is an implementation decision, but the specification below describes behaviour in terms of CertMagic's capabilities.

### 3.3 Configuration

```yaml
server:
  port: 443
  tls:
    mode: acme
    acme:
      # --- Required ---
      domains:
        - chef-metrics.example.com
      email: admin@example.com

      # --- CA ---
      ca_url: https://acme-v02.api.letsencrypt.org/directory     # Production
      # ca_url: https://acme-staging-v02.api.letsencrypt.org/directory  # Staging
      trusted_roots: ""            # Optional: PEM file of additional CA roots to trust

      # --- Challenge type ---
      challenge: http-01           # http-01 | tls-alpn-01 | dns-01

      # --- DNS-01 challenge (required when challenge: dns-01) ---
      dns_provider: ""             # e.g. route53, cloudflare, gcloud, azure
      dns_provider_config: {}      # Provider-specific key/value pairs

      # --- Storage ---
      storage_path: /var/lib/chef-migration-metrics/acme
        # Directory for ACME account keys, certificates, and metadata.
        # Must be persistent and writable. In Kubernetes, back this with a PVC.

      # --- Renewal ---
      renew_before_days: 30        # Begin renewal this many days before expiry

      # --- Rate limiting / safety ---
      agree_to_tos: false          # Must be set to true to accept the CA's Terms of Service

    min_version: "1.2"
    http_redirect_port: 80         # Serves redirects AND HTTP-01 challenges
```

### 3.4 Challenge Types

The ACME protocol defines several challenge types for domain validation. The application must support all three:

#### HTTP-01

The CA sends an HTTP request to `http://<domain>/.well-known/acme-challenge/<token>`. The application must serve the correct response.

- **Requirement:** The application (or its redirect listener) must be reachable on port 80 from the internet.
- When `http_redirect_port` is set, the HTTP-01 challenge handler is automatically installed on the redirect listener alongside the redirect handler. The challenge path takes priority over the redirect.
- When `http_redirect_port` is not set and `challenge: http-01`, the application must log an `ERROR` at startup advising the operator to set `http_redirect_port: 80` or use a different challenge type.

#### TLS-ALPN-01

The CA connects to port 443 and performs a TLS handshake using the `acme-tls/1` ALPN protocol. The application presents a self-signed validation certificate.

- **Requirement:** The application must be reachable on port 443 from the internet.
- This challenge runs on the main HTTPS listener — no additional port is needed.
- This is the recommended challenge type when port 80 is not available.

#### DNS-01

The CA verifies a TXT record at `_acme-challenge.<domain>`. The application creates and cleans up the DNS record via a provider API.

- **Requirement:** API credentials for a supported DNS provider.
- This is the only challenge type that works for internal/private domains not reachable from the internet.
- This is the only challenge type that supports wildcard certificates.

### 3.5 Supported DNS Providers

DNS-01 challenge support must include at least the following providers. Additional providers may be added over time via the pluggable DNS solver interface.

| Provider | `dns_provider` value | Required `dns_provider_config` keys |
|----------|---------------------|--------------------------------------|
| Amazon Route 53 | `route53` | `aws_access_key_id`, `aws_secret_access_key`, `aws_region` (or use instance role / environment) |
| Cloudflare | `cloudflare` | `api_token` (scoped to Zone:DNS:Edit) |
| Google Cloud DNS | `gcloud` | `gcp_project`, `gcp_service_account_json_env` (env var name containing the JSON key) |
| Azure DNS | `azure` | `azure_subscription_id`, `azure_resource_group`, `azure_tenant_id`, `azure_client_id`, `azure_client_secret_env` |
| RFC 2136 (Dynamic DNS) | `rfc2136` | `nameserver`, `tsig_key_name`, `tsig_key_secret_env`, `tsig_algorithm` |

> **Security:** DNS provider credentials must never appear in configuration files in plain text. Use `_env` suffixed keys to reference environment variables, or use provider-native credential mechanisms (e.g. AWS instance roles, GCP workload identity).

### 3.6 Certificate Storage

ACME account keys, issued certificates, private keys, and metadata must be persisted to `acme.storage_path`. This directory must survive application restarts to avoid:

- Re-registering ACME accounts on every startup.
- Hitting rate limits by requesting new certificates on every startup.
- Losing the private key for an issued certificate.

| Path within `storage_path` | Contents |
|----------------------------|----------|
| `accounts/` | ACME account registration data and private keys |
| `certificates/` | Issued certificates, private keys, and metadata |
| `locks/` | File-based locks to coordinate renewal across replicas (see section 3.8) |

File permissions on `storage_path` must be restricted to `0700` (directory) and `0600` (files). The application must set these permissions on startup if they are too permissive, and log a `WARN` if it cannot.

### 3.7 Certificate Renewal

- The application must automatically renew certificates before they expire, controlled by `renew_before_days` (default: 30 days).
- Renewal attempts must use exponential backoff on failure, starting at 1 hour and capping at 24 hours.
- A successful renewal must be logged at `INFO` level with the new certificate's expiry date.
- A failed renewal must be logged at `ERROR` level with the error detail and the current certificate's expiry date.
- When a certificate is within 7 days of expiry and renewal has not succeeded, the application must log a `WARN` on every renewal attempt and (if notifications are configured) send a notification on the `certificate_expiry_warning` event.

### 3.8 Multi-Replica Coordination

When multiple application replicas share the same `storage_path` (e.g. via a shared PVC in Kubernetes), only one replica should perform ACME operations at a time. The ACME library (e.g. CertMagic) provides file-based locking for this purpose. If the chosen library does not provide this, the application must implement its own file lock at `storage_path/locks/`.

Additionally, if the application uses a database advisory lock for other single-leader operations (see the data-collection specification), the ACME renewal can optionally participate in the same leader election. However, file-based locking in the storage directory is sufficient and keeps the ACME subsystem independent of the database.

### 3.9 Terms of Service

The `agree_to_tos` setting must be explicitly set to `true` by the operator. If it is `false` (the default), the application must refuse to start in ACME mode and log an `ERROR` explaining that the operator must review and accept the CA's Terms of Service.

The error message must include the URL to the CA's Terms of Service (obtained from the ACME directory endpoint) so the operator can review them.

### 3.10 Staging vs Production

Operators should test ACME configuration against the CA's staging environment before using production. Let's Encrypt staging and production directories:

| Environment | `ca_url` |
|-------------|----------|
| Staging | `https://acme-staging-v02.api.letsencrypt.org/directory` |
| Production | `https://acme-v02.api.letsencrypt.org/directory` |

Staging certificates are not trusted by browsers but have much higher rate limits. The application must log a `WARN` at startup when using a staging CA URL to remind the operator that certificates will not be trusted by clients.

### 3.11 Environment Variable Overrides

| Environment Variable | Overrides |
|----------------------|-----------|
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_EMAIL` | `server.tls.acme.email` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CA_URL` | `server.tls.acme.ca_url` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CHALLENGE` | `server.tls.acme.challenge` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_DNS_PROVIDER` | `server.tls.acme.dns_provider` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_STORAGE_PATH` | `server.tls.acme.storage_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_AGREE_TO_TOS` | `server.tls.acme.agree_to_tos` |

DNS provider credential environment variables are provider-specific and documented in the `dns_provider_config` table above.

### 3.12 Startup Validation

On startup, the application must fail fast if:

- `mode` is `acme` but `acme.domains` is empty.
- `mode` is `acme` but `acme.email` is empty.
- `mode` is `acme` but `acme.agree_to_tos` is not `true`.
- `acme.storage_path` does not exist or is not writable.
- `acme.challenge` is `dns-01` but `acme.dns_provider` is empty.
- `acme.challenge` is `dns-01` but required `dns_provider_config` keys for the selected provider are missing.
- `acme.challenge` is `http-01` but `http_redirect_port` is `0` (log `ERROR` with guidance — this is a fatal misconfiguration because the HTTP-01 challenge cannot be served).
- `acme.renew_before_days` is less than 1 or greater than 89 (Let's Encrypt certificates are valid for 90 days).
- `acme.ca_url` is not a valid URL.

---

## 4. TLS Configuration Details

### 4.1 Cipher Suites

The application relies on Go's `crypto/tls` default cipher suite selection, which automatically negotiates the strongest mutually-supported suite. Go's defaults are secure and well-maintained — manual cipher suite configuration is intentionally not exposed to avoid misconfiguration.

For TLS 1.3, the cipher suites are fixed by the protocol and cannot be configured.

For TLS 1.2, Go's defaults prefer ECDHE key exchange with AEAD ciphers (AES-GCM, ChaCha20-Poly1305).

### 4.2 OCSP Stapling

When using ACME mode, the ACME library (CertMagic) automatically handles OCSP stapling — fetching and caching OCSP responses and stapling them to TLS handshakes. This improves client connection performance and privacy.

In static mode, Go's standard library handles OCSP stapling automatically if the certificate includes an OCSP responder URL and the responder is reachable.

### 4.3 HSTS Header

When TLS is active (any mode other than `off`), the application should include a `Strict-Transport-Security` header on all HTTPS responses:

```
Strict-Transport-Security: max-age=63072000; includeSubDomains
```

This instructs browsers to always use HTTPS for this domain. The `max-age` of 2 years follows current best practice. The header is only sent on HTTPS responses — never on HTTP redirect responses.

### 4.4 Mutual TLS (mTLS)

Mutual TLS is optionally supported in `static` mode via the `ca_path` setting. When `ca_path` is configured:

- The server requests a client certificate during the TLS handshake (`tls.RequireAndVerifyClientCert`).
- The client certificate must be signed by a CA in the `ca_path` bundle.
- If the client does not present a valid certificate, the TLS handshake fails.

mTLS is not supported in ACME mode because ACME-issued certificates are for server authentication only. If mTLS is needed with ACME, the operator should use a reverse proxy that handles client certificate validation.

---

## 5. Interaction with Other Components

### 5.1 Authentication

The [authentication specification](../auth/Specification.md) requires that all login flows use HTTPS. When `server.tls.mode` is `off`, the application must:

- Log a `WARN` at startup: "TLS is disabled — authentication traffic will not be encrypted. Use a TLS-terminating reverse proxy in production."
- Still allow login flows to proceed (the application does not enforce HTTPS at the application layer — this is the operator's responsibility).

When TLS is active, the HTTP redirect listener (if enabled) must **not** serve any API or authentication endpoints — only redirects and ACME challenges.

### 5.2 Health Checks

The health check endpoint (`/api/v1/admin/status`) is served on the main listener. When TLS is active:

- Kubernetes liveness/readiness probes must be configured to use HTTPS, or the probe must skip TLS verification (`scheme: HTTPS` in the probe definition).
- The Helm chart's probe configuration must be updated to support both HTTP and HTTPS based on the TLS settings.
- The `healthcheck` CLI subcommand (used by the Docker `HEALTHCHECK` instruction) must support connecting over HTTPS with optional TLS verification skip via a `--insecure` flag.

### 5.3 Helm Chart

When deploying with the Helm chart:

- Static mode: TLS certificate and key can be provided via a Kubernetes Secret and mounted into the pod. The chart should support `server.tls.cert_secret_name` and `server.tls.cert_secret_keys` values for this purpose.
- ACME mode: The `storage_path` must be backed by a PVC for persistence. The chart should include an additional PVC for ACME storage when `server.tls.mode` is `acme`.
- In Kubernetes, it is common to terminate TLS at the Ingress controller or service mesh level and leave `server.tls.mode: off`. The application's native TLS support is most useful for non-Kubernetes deployments, Docker Compose, or when end-to-end encryption to the pod is required.

### 5.4 Docker Compose

The Docker Compose example should include a commented-out TLS configuration showing both static and ACME modes. For ACME, the compose file should map port 80 (for HTTP-01 challenges) and port 443 (for HTTPS), and mount a named volume for `storage_path`.

### 5.5 Logging

TLS-related events must be logged with the `tls` scope:

| Event | Severity | Message |
|-------|----------|---------|
| TLS mode selected | `INFO` | "TLS mode: static (cert: /path/to/cert.pem)" or "TLS mode: acme (domains: [example.com])" or "TLS mode: off" |
| Certificate loaded | `INFO` | "TLS certificate loaded: subject=CN=example.com, issuer=..., expires=2025-01-01T00:00:00Z" |
| Certificate reloaded (static) | `INFO` | "TLS certificate reloaded from disk" |
| Certificate reload failed | `ERROR` | "TLS certificate reload failed: <reason>. Continuing with previous certificate." |
| ACME certificate obtained | `INFO` | "ACME certificate obtained for [example.com], expires 2025-04-01T00:00:00Z" |
| ACME certificate renewed | `INFO` | "ACME certificate renewed for [example.com], new expiry 2025-07-01T00:00:00Z" |
| ACME renewal failed | `ERROR` | "ACME certificate renewal failed for [example.com]: <error>. Current certificate expires 2025-04-01T00:00:00Z" |
| Certificate expiring soon | `WARN` | "TLS certificate for [example.com] expires in 5 days. Renewal has not succeeded." |
| ACME staging CA detected | `WARN` | "ACME CA URL is a staging endpoint — certificates will not be trusted by clients" |
| ACME ToS not accepted | `ERROR` | "ACME Terms of Service not accepted. Set server.tls.acme.agree_to_tos: true. ToS URL: <url>" |
| HTTP redirect listener started | `INFO` | "HTTP-to-HTTPS redirect listener started on :80" |
| mTLS enabled | `INFO` | "Mutual TLS enabled: client certificates validated against <ca_path>" |

### 5.6 Notifications

When notifications are enabled, the following TLS-related notification event is available:

| Event | Description |
|-------|-------------|
| `certificate_expiry_warning` | Sent when a certificate is within 7 days of expiry and automatic renewal has not succeeded. Includes domain name(s), current expiry timestamp, and last renewal error. |

Operators can subscribe to this event on any configured notification channel (webhook, email).

---

## 6. Backward Compatibility

The previous configuration schema used a boolean `server.tls.enabled` field:

```yaml
# Old format (deprecated)
server:
  tls:
    enabled: true
    cert_path: /path/to/cert.pem
    key_path: /path/to/key.pem
```

For backward compatibility:

- If `server.tls.enabled` is present and `true`, and `server.tls.mode` is not set, the application must treat this as `mode: static` and log a `WARN`: "server.tls.enabled is deprecated — use server.tls.mode: static instead."
- If `server.tls.enabled` is `false` (or absent) and `server.tls.mode` is not set, the application defaults to `mode: off`.
- If both `server.tls.enabled` and `server.tls.mode` are present, `server.tls.mode` takes precedence and `server.tls.enabled` is ignored (with a `WARN` log).

---

## 7. Full Configuration Reference

### 7.1 Plain HTTP

```yaml
server:
  listen_address: "0.0.0.0"
  port: 8080
  tls:
    mode: off
  graceful_shutdown_seconds: 30
```

### 7.2 Static Certificates

```yaml
server:
  listen_address: "0.0.0.0"
  port: 443
  tls:
    mode: static
    cert_path: /etc/chef-migration-metrics/tls/server.crt
    key_path: /etc/chef-migration-metrics/tls/server.key
    min_version: "1.2"
    http_redirect_port: 80
  graceful_shutdown_seconds: 30
```

### 7.3 Static Certificates with mTLS

```yaml
server:
  listen_address: "0.0.0.0"
  port: 443
  tls:
    mode: static
    cert_path: /etc/chef-migration-metrics/tls/server.crt
    key_path: /etc/chef-migration-metrics/tls/server.key
    ca_path: /etc/chef-migration-metrics/tls/client-ca.crt
    min_version: "1.3"
    http_redirect_port: 80
  graceful_shutdown_seconds: 30
```

### 7.4 ACME with HTTP-01 Challenge (Let's Encrypt)

```yaml
server:
  listen_address: "0.0.0.0"
  port: 443
  tls:
    mode: acme
    acme:
      domains:
        - chef-metrics.example.com
      email: admin@example.com
      ca_url: https://acme-v02.api.letsencrypt.org/directory
      challenge: http-01
      storage_path: /var/lib/chef-migration-metrics/acme
      renew_before_days: 30
      agree_to_tos: true
    min_version: "1.2"
    http_redirect_port: 80
  graceful_shutdown_seconds: 30
```

### 7.5 ACME with TLS-ALPN-01 Challenge

```yaml
server:
  listen_address: "0.0.0.0"
  port: 443
  tls:
    mode: acme
    acme:
      domains:
        - chef-metrics.example.com
      email: admin@example.com
      ca_url: https://acme-v02.api.letsencrypt.org/directory
      challenge: tls-alpn-01
      storage_path: /var/lib/chef-migration-metrics/acme
      renew_before_days: 30
      agree_to_tos: true
    min_version: "1.2"
  graceful_shutdown_seconds: 30
```

### 7.6 ACME with DNS-01 Challenge (Cloudflare)

```yaml
server:
  listen_address: "0.0.0.0"
  port: 443
  tls:
    mode: acme
    acme:
      domains:
        - chef-metrics.example.com
        - "*.chef-metrics.example.com"   # Wildcard — requires DNS-01
      email: admin@example.com
      ca_url: https://acme-v02.api.letsencrypt.org/directory
      challenge: dns-01
      dns_provider: cloudflare
      dns_provider_config:
        api_token_env: CLOUDFLARE_API_TOKEN
      storage_path: /var/lib/chef-migration-metrics/acme
      renew_before_days: 30
      agree_to_tos: true
    min_version: "1.2"
  graceful_shutdown_seconds: 30
```

### 7.7 ACME with DNS-01 Challenge (Route 53)

```yaml
server:
  listen_address: "0.0.0.0"
  port: 443
  tls:
    mode: acme
    acme:
      domains:
        - chef-metrics.internal.example.com
      email: admin@example.com
      ca_url: https://acme-v02.api.letsencrypt.org/directory
      challenge: dns-01
      dns_provider: route53
      dns_provider_config:
        aws_region: us-east-1
        # aws_access_key_id and aws_secret_access_key read from standard
        # AWS environment variables or instance role — no config needed
      storage_path: /var/lib/chef-migration-metrics/acme
      renew_before_days: 30
      agree_to_tos: true
    min_version: "1.2"
  graceful_shutdown_seconds: 30
```

---

## 8. Security Considerations

### 8.1 Private Key Protection

- Private key files (both static and ACME-generated) must have filesystem permissions no more permissive than `0600`.
- The application must log a `WARN` at startup if key file permissions are more permissive than `0600`.
- ACME storage directory permissions must be `0700`.
- Private keys must never be logged, included in error messages, or exposed via any API endpoint.

### 8.2 Certificate Transparency

Let's Encrypt and most public CAs submit certificates to Certificate Transparency (CT) logs. Operators should be aware that domain names in requested certificates become publicly visible. For internal domains where this is undesirable, use a private ACME CA or static certificates from an internal CA.

### 8.3 Rate Limits

Let's Encrypt enforces rate limits on certificate issuance:

| Limit | Value |
|-------|-------|
| Certificates per Registered Domain | 50 per week |
| Duplicate Certificate | 5 per week |
| Failed Validation | 5 failures per account, per hostname, per hour |

The application must not request certificates unnecessarily. Persistent storage of ACME data (section 3.6) is critical to avoid hitting rate limits on restart.

### 8.4 Credential Isolation

DNS provider credentials for ACME DNS-01 challenges should be scoped to the minimum required permissions:

- **Cloudflare:** Use an API token scoped to `Zone:DNS:Edit` for the specific zone only.
- **Route 53:** Use an IAM policy restricted to `route53:GetChange`, `route53:ChangeResourceRecordSets`, and `route53:ListHostedZonesByName` for the specific hosted zone.
- **Google Cloud DNS:** Use a service account with the `dns.admin` role scoped to the specific DNS zone.
- **Azure DNS:** Use a service principal with the `DNS Zone Contributor` role scoped to the specific DNS zone resource.

---

## Related Specifications

- [Configuration Specification](../configuration/Specification.md) — overall configuration schema and validation rules
- [Web API Specification](../web-api/Specification.md) — HTTP endpoints served over the TLS listener
- [Authentication and Authorisation](../auth/Specification.md) — requires HTTPS for login flows
- [Packaging Specification](../packaging/Specification.md) — Dockerfile, Helm chart, and Docker Compose TLS integration
- [Logging Specification](../logging/Specification.md) — log scopes and severity levels for TLS events