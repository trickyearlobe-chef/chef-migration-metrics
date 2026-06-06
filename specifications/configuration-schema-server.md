# Configuration — Configuration Schema (Readiness, Storage, Server & Auth)

### Upgrade Readiness

```yaml
readiness:
  install_path_linux: /hab          # filesystem path where the Chef Client bundle is installed on Linux
  install_path_windows: 'C:\hab'    # filesystem path on Windows
  install_size_mb_linux: 3072       # disk space required for the Linux install bundle (MB)
  install_size_mb_windows: 6144     # disk space required for the Windows install bundle (MB)
  min_remaining_free_percent: 20    # minimum % of the filesystem that must remain free after install
```

> ⚠️ **Warning: non-default install paths carry significant risk.** While symlinking the install directory to another filesystem location is supported, it does not guarantee that path-dependent behaviour within Chef and its cookbooks will continue to work correctly. Many cookbooks hardcode assumptions about the default install location. On **Windows**, relocating the install directory also changes the configuration directory, which causes failures with `knife bootstrap`. Only override `install_path_linux` or `install_path_windows` after fully understanding these risks and testing the non-standard path in a representative environment.

| Setting | Default | Description |
|---------|---------|-------------|
| `install_path_linux` | `/hab` | Installation target path on Linux nodes. Used as the input to the longest-prefix-match filesystem lookup. Override only after understanding the risks of a non-standard install location. |
| `install_path_windows` | `C:\hab` | Installation target path on Windows nodes. Matched against the drive letter of each filesystem entry. Override only after understanding the risks of a non-standard install location. |
| `install_size_mb_linux` | `3072` | Disk space in MB required for the Chef Client bundle install on Linux. |
| `install_size_mb_windows` | `6144` | Disk space in MB required for the Chef Client bundle install on Windows. |
| `min_remaining_free_percent` | `20` | After reserving the platform install size, at least this percentage of the filesystem's total capacity must remain free. Both the absolute and percentage conditions must pass. |

The `install_path_linux` and `install_path_windows` fields must be accompanied by a prominent warning in the UI wherever they are displayed or edited. See the warning text in this spec — it must cover: cookbooks hardcoding default paths, and the Windows-specific knife bootstrap config directory issue.

---

### Exports

Controls the behaviour of data export operations.

```yaml
exports:
  max_rows: 100000               # maximum number of rows in a single export
  async_threshold: 10000         # exports larger than this are processed asynchronously
  output_directory: /var/lib/chef-migration-metrics/exports
  retention_hours: 24            # how long completed export files are retained
```

| Setting | Default | Description |
|---------|---------|-------------|
| `max_rows` | `100000` | Maximum number of rows in a single export. Prevents runaway exports. |
| `async_threshold` | `10000` | Exports estimated to contain more than this many rows are processed asynchronously. The API returns a job ID and the frontend polls for completion. |
| `output_directory` | `/var/lib/chef-migration-metrics/exports` | Directory where export files are written before download. Must be writable by the application. |
| `retention_hours` | `24` | Completed export files are deleted after this many hours. |

---

### Elasticsearch Export

Controls the export of data to Elasticsearch for analysis with Kibana. The application writes NDJSON (newline-delimited JSON) files to a directory, which a Logstash pipeline reads and indexes into Elasticsearch. See the Elasticsearch Export Specification for the full document type reference, pipeline design, and ELK testing stack.

```yaml
elasticsearch:
  enabled: false                                           # whether the Elasticsearch export is active
  output_directory: /var/lib/chef-migration-metrics/elasticsearch  # directory for NDJSON files
  retention_hours: 48                                      # how long NDJSON files are retained
```

| Setting | Default | Description |
|---------|---------|-------------|
| `enabled` | `false` | Whether the Elasticsearch export is active. When disabled, no NDJSON files are written and the export post-processing step is skipped entirely. |
| `output_directory` | `/var/lib/chef-migration-metrics/elasticsearch` | Directory where NDJSON files are written for Logstash to pick up. Must be writable by the application and readable by Logstash. When using the ELK testing stack (`deploy/elk/`), this path should correspond to the shared volume mount. |
| `retention_hours` | `48` | How long NDJSON files are retained before the application deletes them. Should be long enough for Logstash to process them. The default of 48 hours provides ample buffer for intermittent Logstash downtime. |

> **Decoupled architecture:** The application has no direct dependency on Elasticsearch or Logstash. It only writes NDJSON files to disk. Logstash is responsible for reading and indexing them. This means the Elasticsearch export can be enabled even when no ELK stack is running — the files accumulate in the output directory and are cleaned up after `retention_hours`.

> **Shared volume:** When running the application and ELK stack in separate Docker Compose environments, the `output_directory` must be accessible to both. See `deploy/elk/README.md` for instructions on configuring a shared host directory or Docker volume.

---

### Datastore

```yaml
datastore:
  url: postgres://localhost:5432/chef_migration_metrics
```

Credentials for the datastore should be supplied via the `DATABASE_URL` environment variable in preference to the configuration file.

---

### Database Migrations

Database schema changes must be managed through migrations. Migrations ensure the schema evolves safely and reproducibly across all environments (development, CI, production).

- Migrations are versioned SQL files stored in a `migrations/` directory in the repository.
- Each migration file is named with a numeric prefix to establish ordering, followed by a descriptive name:
  ```
  migrations/
  ├── 0001_create_nodes.sql
  ├── 0002_create_cookbooks.sql
  ├── 0003_create_test_results.sql
  └── ...
  ```
- Migrations are applied automatically on application startup before any other database operations. The application must refuse to start if any pending migration fails.
- Applied migrations are recorded in a `schema_migrations` table in the database so that already-applied migrations are never re-run.
- Migrations must be **additive and backward compatible** wherever possible. Destructive changes (dropping columns or tables) must be performed in a separate migration after the application code no longer references the removed schema.
- Down migrations (rollback) are not required but may be provided as a paired `_down.sql` file for development convenience.
- A Go migration library must be used (e.g. `golang-migrate/migrate`) rather than implementing migration tracking from scratch.

---

### Web Server

Controls the HTTP listener for the Web API and dashboard frontend. The server supports three TLS modes: plain HTTP (`off`), externally-managed certificates (`static`), and automatic certificate management via ACME (`acme`). See the [TLS and Certificate Management specification](tls.md) for full details on certificate lifecycle, ACME challenge types, renewal, and security considerations.

```yaml
server:
  listen_address: "0.0.0.0"       # Interface to bind to (default: all interfaces)
  port: 8080                       # Listen port (set to 443 when TLS is active)
  tls:
    mode: "off"                    # "off" | "static" | "acme"

    # --- Static certificate settings (mode: static) ---
    cert_path: ""                  # Path to PEM-encoded certificate (full chain)
    key_path: ""                   # Path to PEM-encoded private key
    ca_path: ""                    # Optional: CA bundle for mutual TLS (mTLS)
    min_version: "1.2"             # Minimum TLS version: "1.2" or "1.3"
    http_redirect_port: 0          # Optional: start HTTP listener to redirect to HTTPS

    # --- ACME settings (mode: acme) ---
    acme:
      domains: []                  # List of domain names for the certificate
      email: ""                    # Contact email for the ACME account
      ca_url: "https://acme-v02.api.letsencrypt.org/directory"
      challenge: "http-01"         # "http-01" | "tls-alpn-01" | "dns-01"
      dns_provider: ""             # Required when challenge is dns-01
      dns_provider_config: {}      # Provider-specific key/value pairs
      storage_path: "/var/lib/chef-migration-metrics/acme"
      renew_before_days: 30        # Begin renewal this many days before expiry
      agree_to_tos: false          # Must be true to accept the CA's Terms of Service
      trusted_roots: ""            # Optional: PEM file of additional CA roots to trust

  # --- WebSocket settings ---
  websocket:
    enabled: true                  # Enable/disable the WebSocket endpoint (default: true)
    max_connections: 100           # Maximum concurrent WebSocket connections (default: 100)
    send_buffer_size: 64           # Per-client outbound event buffer size (default: 64)
    write_timeout_seconds: 10      # Timeout for writing a single frame (default: 10)
    ping_interval_seconds: 30      # Server-initiated ping interval (default: 30)
    pong_timeout_seconds: 60       # Time to wait for pong before closing (default: 60)

  graceful_shutdown_seconds: 30    # Time to wait for in-flight requests on shutdown
```

#### General Settings

| Setting | Default | Notes |
|---------|---------|-------|
| `listen_address` | `0.0.0.0` | Set to `127.0.0.1` to restrict to localhost only. |
| `port` | `8080` | Any available port. Operators should set to `443` when TLS is active. The default does not change when TLS is enabled. |
| `tls.mode` | `off` | `off` — plain HTTP, no encryption. `static` — HTTPS using certificate/key files from disk. `acme` — HTTPS using certificates obtained automatically via the ACME protocol. |
| `tls.min_version` | `"1.2"` | Minimum TLS protocol version. Valid values: `"1.2"`, `"1.3"`. Applies to both `static` and `acme` modes. TLS 1.0 and 1.1 are not supported. |
| `tls.http_redirect_port` | `0` (disabled) | When set to a valid port (e.g. `80`), starts a secondary HTTP listener that responds with `301` redirects to HTTPS. In `acme` mode with `http-01` challenge, this listener also serves ACME challenge responses. |
| `graceful_shutdown_seconds` | `30` | On `SIGTERM`/`SIGINT`, the server waits this long for in-flight requests to complete before forcing shutdown. |

#### WebSocket Settings

| Setting | Default | Notes |
|---------|---------|-------|
| `websocket.enabled` | `true` | Set to `false` to disable the WebSocket endpoint entirely. The REST API and dashboard continue to function normally; the frontend falls back to periodic polling. |
| `websocket.max_connections` | `100` | Maximum number of simultaneous WebSocket connections. New connections are rejected with `503 Service Unavailable` when the limit is reached. Set higher for deployments with many concurrent dashboard users. |
| `websocket.send_buffer_size` | `64` | Size of each client's outbound event channel. If a client's buffer fills up (slow consumer), the server closes that connection. The client is expected to reconnect automatically. |
| `websocket.write_timeout_seconds` | `10` | Maximum time to write a single WebSocket frame before closing the connection. |
| `websocket.ping_interval_seconds` | `30` | How often the server sends WebSocket ping frames to detect dead connections. |
| `websocket.pong_timeout_seconds` | `60` | How long the server waits for a pong response before closing the connection. Must be greater than `ping_interval_seconds`. |

#### Static Certificate Settings (mode: static)

| Setting | Required | Default | Notes |
|---------|----------|---------|-------|
| `tls.cert_path` | Yes | — | Path to PEM-encoded TLS certificate file. May include intermediate certificates (full chain). Must be readable by the application process. |
| `tls.key_path` | Yes | — | Path to PEM-encoded private key file. Must be readable by the application process. Never commit to source control. |
| `tls.ca_path` | No | `""` | Path to a PEM-encoded CA bundle. When set, enables mutual TLS (mTLS) — the server requires and validates client certificates against this CA. |

Certificates are automatically reloaded on `SIGHUP` or when file changes are detected via filesystem watching. See [TLS specification § 2.3](tls.md#23-certificate-reload).

#### ACME Settings (mode: acme)

| Setting | Required | Default | Notes |
|---------|----------|---------|-------|
| `tls.acme.domains` | Yes | `[]` | Domain names for the certificate. Must be resolvable and (for HTTP-01/TLS-ALPN-01) reachable from the internet. |
| `tls.acme.email` | Yes | `""` | Contact email registered with the ACME CA. Used for expiry notifications from the CA. |
| `tls.acme.ca_url` | No | Let's Encrypt production | ACME directory URL. Use `https://acme-staging-v02.api.letsencrypt.org/directory` for testing. |
| `tls.acme.challenge` | No | `http-01` | Challenge type: `http-01`, `tls-alpn-01`, or `dns-01`. |
| `tls.acme.dns_provider` | When `dns-01` | `""` | DNS provider for DNS-01 challenges: `route53`, `cloudflare`, `gcloud`, `azure`, `rfc2136`. |
| `tls.acme.dns_provider_config` | When `dns-01` | `{}` | Provider-specific configuration. Credentials should use `_env`-suffixed keys referencing environment variables. |
| `tls.acme.storage_path` | No | `/var/lib/chef-migration-metrics/acme` | Persistent directory for ACME account keys, certificates, and metadata. Must survive restarts. |
| `tls.acme.renew_before_days` | No | `30` | Begin certificate renewal this many days before expiry. Must be between 1 and 89. |
| `tls.acme.agree_to_tos` | Yes | `false` | Must be explicitly set to `true`. The application refuses to start in ACME mode until the operator accepts the CA's Terms of Service. |
| `tls.acme.trusted_roots` | No | `""` | Path to a PEM file of additional CA roots to trust when communicating with the ACME CA (useful for private ACME servers). |

See [TLS specification § 3](tls.md#3-acme-automatic-certificate-management) for full details on challenge types, DNS provider configuration, certificate storage, renewal, multi-replica coordination, and rate limits.

#### Backward Compatibility

The previous `server.tls.enabled` boolean is deprecated but still recognised for backward compatibility:

- If `tls.enabled: true` is present and `tls.mode` is not set, the application treats this as `mode: static` and logs a deprecation warning.
- If `tls.enabled: false` (or absent) and `tls.mode` is not set, the application defaults to `mode: off`.
- If both `tls.enabled` and `tls.mode` are present, `tls.mode` takes precedence and `tls.enabled` is ignored (with a warning).

> **Note on HTTPS:** The [authentication specification](auth.md) requires all login flows to be over HTTPS. In production, enable native TLS (static or ACME mode) or place the application behind a TLS-terminating reverse proxy.

---

### Frontend

The web dashboard is a single-page application (SPA) built with **React** and bundled into the Go binary as embedded static assets. No separate frontend server is required.

```yaml
frontend:
  base_path: "/"               # URL base path for the dashboard (useful behind a reverse proxy)
```

| Setting | Default | Notes |
|---------|---------|-------|
| `base_path` | `/` | Set to e.g. `/chef-metrics/` if the application is served under a sub-path behind a reverse proxy. Must include trailing slash. |

The frontend communicates with the backend exclusively through the `/api/v1` endpoints documented in the [Web API specification](web-api.md). All routes not matching `/api/` serve the SPA's `index.html` to support client-side routing.

---

### Logging

```yaml
logging:
  level: INFO                # One of: DEBUG, INFO, WARN, ERROR
  retention_days: 90         # Number of days to retain log entries before purging
```

---

### Ownership

Controls ownership tracking features. When disabled (default), all ownership UI elements are hidden and ownership tables are not populated. See the [Ownership Specification](ownership.md) for the full feature design.

```yaml
ownership:
  enabled: false  # Default: false. Enable ownership tracking features.

  audit_log:
    retention_days: 365  # Days to retain audit log entries. 0 = retain indefinitely.

  auto_rules: []
  # Auto-derivation rules are defined here. Example:
  # auto_rules:
  #   - name: aws-nodes-to-cloud-team
  #     owner: cloud-team
  #     type: node_attribute
  #     attribute_path: automatic.cloud.provider
  #     match_value: "aws"
  #   - name: web-prod-nodes
  #     owner: web-platform
  #     type: node_name_pattern
  #     pattern: "^web-prod-.*"
  #   - name: payment-policy
  #     owner: payments-team
  #     type: policy_match
  #     policy_name: "payment-app"
  #   - name: acme-cookbooks
  #     owner: acme-platform
  #     type: cookbook_name_pattern
  #     pattern: "^acme-.*"
  #   - name: web-team-repos
  #     owner: web-platform
  #     type: git_repo_url_pattern
  #     pattern: "gitlab\\.example\\.com/team-web/.*"
```

| Setting | Default | Description |
|---------|---------|-------------|
| `ownership.enabled` | `false` | Enable ownership tracking. When disabled, tables still exist but are not populated and UI elements are hidden. |
| `ownership.audit_log.retention_days` | `365` | Days to retain ownership audit log entries. Set to `0` to disable purging. |
| `ownership.auto_rules` | `[]` | List of auto-derivation rules. See [Ownership Specification](ownership.md) § 2.2 for rule types and field definitions. |

---

### Authentication

See the [Authentication and Authorisation specification](auth.md) for full details. Authentication providers are configured under the `auth` key.

```yaml
auth:
  providers:
    - type: local

    - type: saml
      idp_metadata_url: https://idp.example.com/saml/metadata
      sp_entity_id: chef-migration-metrics
```
