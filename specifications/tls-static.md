# TLS — Static Certificate Mode

> Part of the [TLS and Certificate Management](tls.md) spec. Covers
> `mode: static`: operator-provided cert/key from **file** or **DB**, mTLS,
> reload, fail-open startup, and save-time preflight. CSR generation lives in
> [tls-csr.md](tls-csr.md); ACME in [tls-acme.md](tls-acme.md).

## 2. Static Certificate Mode

### 2.1 Configuration

```yaml
server:
  port: 443
  tls:
    mode: static
    cert_source: file              # file (default) | db
    cert_path: /etc/chef-migration-metrics/tls/server.crt   # cert_source: file
    key_path: /etc/chef-migration-metrics/tls/server.key    # cert_source: file
    ca_path: ""                    # Optional: CA bundle for client cert validation
    min_version: "1.2"             # Minimum TLS version (default: "1.2")
    http_redirect_port: 80         # Optional: redirect HTTP to HTTPS
```

| Setting | Required | Default | Description |
|---------|----------|---------|-------------|
| `cert_source` | No | `file` | Where the cert/key come from: `file` (paths on disk, below) or `db` (encrypted in the config store, see § 2.7). |
| `cert_path` | Yes when `mode: static` and `cert_source: file` | — | Path to the PEM-encoded TLS certificate file. May include intermediate certificates (full chain). |
| `key_path` | Yes when `mode: static` and `cert_source: file` | — | Path to the PEM-encoded private key file. Must be readable by the application process. |
| `ca_path` | No | `""` | Path to a PEM-encoded CA bundle. When set, the server enables mutual TLS (mTLS) and validates client certificates against this CA. |
| `min_version` | No | `"1.2"` | Minimum accepted TLS protocol version. Valid values: `"1.2"`, `"1.3"`. TLS 1.0 and 1.1 are not supported. |

`cert_source` defaults to `file` for backward compatibility, so existing
file-mount deployments (k8s cert-manager, host paths) are unaffected. `db` is the
UI-driven path (§ 2.7).

### 2.2 Certificate Chain

The certificate (file or DB) should contain the full certificate chain in PEM
format, ordered from leaf to root:

1. Server certificate
2. Intermediate CA certificate(s)
3. Root CA certificate (optional — clients typically have this in their trust store)

### 2.3 Certificate Reload

The application must support **automatic certificate reload** without restart.
The mechanism depends on `cert_source`:

**`cert_source: file`** — file-watch / signal driven:

- On receiving `SIGHUP`, the application re-reads `cert_path` and `key_path` from disk and begins serving the new certificate for subsequent TLS handshakes. Existing connections are not interrupted.
- Alternatively, the application may use filesystem watching (e.g. `fsnotify`) to detect changes to the certificate files and reload automatically. This is particularly useful in Kubernetes where cert-manager updates the Secret (and therefore the mounted files) in place.

**`cert_source: db`** — config-change driven:

- The DB source reloads when the configuration changes (`configHolder.Reload()`),
  not via file-watch. Saving a new cert/key through the admin API (§ 2.6, § 2.7)
  triggers a reload, and the listener begins serving the new certificate for
  subsequent handshakes. Existing connections are not interrupted. `SIGHUP` is a
  no-op for the DB source.

For both sources, if the new certificate material is invalid (unparseable,
mismatched key), the reload must fail gracefully: the application continues
serving with the previous valid certificate and logs an `ERROR`-level message
describing the failure.

### 2.4 Startup Behaviour (Fail-Open)

When `mode: static`, the application builds the TLS listener at startup using the
same load path as save-time preflight (§ 2.6): the cert/key are present and
loadable (from file or DB per `cert_source`), the PEM parses, the key matches the
certificate, `ca_path` (when set) is a valid PEM bundle, and `min_version` is
`"1.2"` or `"1.3"`.

If the listener **cannot** be built for any of these reasons, the application
MUST NOT exit. Instead it:

- Logs an `ERROR` on the `tls` scope describing the failure (never including
  private key material).
- Records a **degraded** state (`{degraded: true, reason}`) exposed on the status
  endpoint ([tls.md § 6.3](tls.md#63-degraded-tls-status-and-recovery)).
- Starts a **plain HTTP** listener on the configured
  `server.listen_address:server.port` so the admin UI stays reachable to fix the
  problem.

This fail-open behaviour guarantees a bad certificate can never lock an operator
out of the UI. Save-time preflight (§ 2.6) makes this path rare — it normally
only triggers when certificate files change on disk underneath an already-running
deployment, when DB material is altered out-of-band, or when `server.tls` was
written before preflight existed.

**Startup validation is structural-only.** Configuration validation checks that
`cert_path`/`key_path` are set when `mode: static` and `cert_source: file`, but
never that the files exist, are readable, or parse — that is the listener's
concern (above). For `cert_source: db`, startup never requires the cert/key to be
present in the store (that is a save/preflight check, § 2.7); a missing DB cert
falls open to plain HTTP exactly like a missing file. A missing or moved
cert/key/CA therefore never aborts startup. Save-time preflight (§ 2.6) still
loads the certificate before persisting, so an unusable certificate can never be
committed through the admin API.

An **expired** certificate that otherwise loads is not a failure: the listener
starts in static (HTTPS) mode and logs a `WARN` (operators may be mid-renewal).

There is no runtime auto-recovery — the plain listener is already bound to the
port. The degraded state clears on the next restart with a working certificate,
or — for the DB source — when a valid pair is saved and the listener reloads
(§ 2.3). Recovery for an mTLS lockout or a bad DB cert is the repair CLI; see
[tls.md § 6.3](tls.md#63-degraded-tls-status-and-recovery).

### 2.5 Environment Variable Overrides

| Environment Variable | Overrides |
|----------------------|-----------|
| `CHEF_MIGRATION_METRICS_SERVER_TLS_MODE` | `server.tls.mode` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_CERT_SOURCE` | `server.tls.cert_source` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_CERT_PATH` | `server.tls.cert_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_KEY_PATH` | `server.tls.key_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_CA_PATH` | `server.tls.ca_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_MIN_VERSION` | `server.tls.min_version` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_HTTP_REDIRECT_PORT` | `server.tls.http_redirect_port` |

Env overrides set scalar config fields. They do **not** inject cert/key PEM
material for the DB source — DB cert/key are managed through the admin API/UI and
the repair CLI only. This is a documented limitation, not a recovery lever; the
recovery boundary is host access to `DATABASE_URL` +
`CMM_CREDENTIAL_ENCRYPTION_KEY` (see [tls.md § 6.3](tls.md#63-degraded-tls-status-and-recovery)).

### 2.6 Save-Time Preflight Validation

When the static-mode configuration is changed through the admin API
(`PUT /api/v1/admin/config/server`), the certificate is validated **before the
change is persisted**, using the same load path as startup (cert/key loadable,
PEM parses, key matches certificate, and `ca_path` — when set — is a valid PEM
bundle). For `cert_source: db`, the cert+key submitted in the request are
validated as a pair before being written to the config store (§ 2.7). On failure
the API returns `422` and does not write, so an unusable certificate can never be
committed and brick the listener on the next restart/reload. The
redirect-vs-listen-port collision (§ 1.2 in the index) is checked the same way.
See [configuration-validation.md § Save-time preflight](configuration-validation.md).

### 2.7 Certificate Source: File vs DB

`cert_source: db` stores the certificate and private key in the encrypted config
store instead of on disk, so an operator can configure TLS entirely through the
UI with no host filesystem access.

**Storage model** (config store keys):

| Key | `secret` | Contents |
|-----|----------|----------|
| `server.tls.certificate` | `false` | Leaf + chain PEM (public; safe to return). |
| `server.tls.private_key` | `true` | Private key PEM (never returned by any API). |
| `server.tls.private_key.pending` | `true` | CSR-generated key awaiting its signed cert (see [tls-csr.md](tls-csr.md)). |

**Encrypted at rest.** Secret-flagged values reuse the existing encryption stack
(`internal/secrets`: AES-256-GCM, HKDF-derived per-key material, per-row nonce,
AAD binding) under the master key `CMM_CREDENTIAL_ENCRYPTION_KEY`, identical to
all other config-store secrets. The certificate (public) is stored non-secret;
the private key is stored secret.

**Key never exposed.** The private key is `secret: true` and is never returned
through any API. The admin API returns only certificate **metadata** — subject,
SANs, and expiry — so the UI can show what is installed without the key leaving
the server. The DB cert/key are written only through the admin save path (§ 2.6)
or CSR promotion (tls-csr.md), and cleared only via the repair CLI.

**Preflight + activation.** On save, the cert+key pair is validated together
(§ 2.6); a mismatched or unparseable pair returns `422` and nothing is written.
On success the pair is persisted and the listener reloads on the config change
(§ 2.3) to serve the new certificate.

**mTLS.** `ca_path` may still be set with `cert_source: db`; it continues to
reference a CA bundle for client-cert validation. An mTLS lockout is recovered
with the repair CLI (`tls clear-ca`), not by editing host files — see
[tls.md § 6.3](tls.md#63-degraded-tls-status-and-recovery).

### 2.8 Configuration Reference

**Static, file source:**

```yaml
server:
  port: 443
  tls:
    mode: static
    cert_source: file
    cert_path: /etc/chef-migration-metrics/tls/server.crt
    key_path: /etc/chef-migration-metrics/tls/server.key
    min_version: "1.2"
    http_redirect_port: 80
```

**Static, DB source** (cert/key live encrypted in the config store; set via the
admin UI, not in this file):

```yaml
server:
  port: 443
  tls:
    mode: static
    cert_source: db
    min_version: "1.2"
    http_redirect_port: 80
```
