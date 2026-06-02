# Configuration - Component Specification

> **TL;DR** — Single YAML config file with environment variable overrides for secrets. Key sections: `server` (bind address, port, TLS mode — off/static/acme), `database` (PostgreSQL URL), `collection` (Chef server orgs, schedule, stale thresholds), `target_versions` (Chef Client versions to test against), `git` (cookbook repo URLs), `concurrency` (worker pool sizes per task type), `analysis_tools` (embedded CookStyle/TK bin dir, timeouts), `auth` (local/SAML), `exports` (output dir, retention, async threshold), `elasticsearch` (NDJSON export toggle, output dir), `logging` (level, retention). All sensitive values must be set via env vars, never inlined. See `todo/configuration.md` for implementation status.

## Overview

This document specifies the configuration surface area for the Chef Migration Metrics application. Configuration controls all aspects of the application including Chef server connectivity, collection scheduling, analysis targets, datastore connectivity, logging behaviour, and authentication providers.

## Live Reload Requirement

**All configuration changes made via the admin API (config store) MUST take effect immediately without requiring an application restart.** This applies to all config-store-backed settings including:

- Backup schedule (cron expression, enabled/disabled)
- Target Chef version
- Collection schedule
- Git base URLs
- Kitchen settings
- Any other admin-configurable value

Components that consume config-store values must either:
1. Read the current value from the config store on each use (pull), OR
2. Subscribe to config-change notifications and update their state (push)

Approach (1) is preferred for simplicity unless the component has long-lived state (e.g. a cron scheduler that needs to reschedule its next tick).

Requiring an application restart to pick up config changes is a **bug**.

## Configuration File

Configuration is stored in a YAML file. The path to the configuration file may be specified via a command-line flag or the `CHEF_MIGRATION_METRICS_CONFIG` environment variable. If neither is specified, the application looks for `config.yml` in the current working directory.

## Secrets and Credentials

Sensitive values (private key paths, passwords, tokens) must never be stored in source control. All sensitive configuration values must support being overridden by environment variables. The configuration file should reference key files by path, not inline their contents.

### Credential Storage Options

The application supports three ways to supply credentials, listed in order of resolution precedence:

| Method | When to use | Example |
|--------|-------------|---------|
| **Database** | Multi-org deployments, containerised environments, management via Web UI | `client_key_credential: myorg-production-key` references a row in the `credentials` table |
| **Environment variable** | Container orchestrators (Kubernetes Secrets, ECS task definitions), CI/CD | `CMM_CREDENTIAL_ENCRYPTION_KEY` |
| **File path** | Traditional on-premises installs, simple single-org setups | `client_key_path: /etc/chef-migration-metrics/keys/myorg.pem` |

When multiple sources are configured for the same credential, database takes precedence over environment variable, which takes precedence over file path. This allows operators to migrate incrementally from file-based to database-stored credentials without changing the config file.

### Credential Encryption Key

Credentials stored in the database are encrypted at the application layer using AES-256-GCM. The master encryption key must be provided externally — it is never stored in the database alongside the encrypted values. See the [Datastore Specification](../datastore/Specification.md) for the full encryption model.

```yaml
credential_encryption_key_env: CMM_CREDENTIAL_ENCRYPTION_KEY
```

| Setting | Required | Default | Description |
|---------|----------|---------|-------------|
| `credential_encryption_key_env` | When DB credentials are used | `CMM_CREDENTIAL_ENCRYPTION_KEY` | Name of the environment variable containing the master encryption key. The key must be at least 32 bytes (256 bits), Base64-encoded. |

The key itself must **never** appear in the YAML config file. Only the name of the environment variable that holds it is configured.

| Environment Variable | Description |
|----------------------|-------------|
| `CMM_CREDENTIAL_ENCRYPTION_KEY` | Base64-encoded AES-256 master key for encrypting/decrypting database-stored credentials. Required if any `*_credential` references exist in the config or if credentials have been created via the Web API. |
| `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` | Base64-encoded previous master key, required during key rotation. Set this alongside the new key when rotating. After successful restart and re-encryption, remove it. |

---

## Configuration Schema

### Chef Server Organisations

A list of one or more Chef Infra Server organisations to collect data from. Each organisation is independently configured.

```yaml
organisations:
  # Option A: File-based key (traditional)
  - name: myorg-production
    chef_server_url: https://chef.example.com
    org_name: myorg-production
    client_name: chef-migration-metrics
    client_key_path: /etc/chef-migration-metrics/keys/myorg-production.pem

  # Option B: Database-stored key (recommended for multi-org / container deployments)
  - name: myorg-staging
    chef_server_url: https://chef.example.com
    org_name: myorg-staging
    client_name: chef-migration-metrics
    client_key_credential: myorg-staging-key
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | A unique friendly name for this organisation used in logs and the UI |
| `chef_server_url` | Yes | The base URL of the Chef Infra Server |
| `org_name` | Yes | The Chef organisation name as registered on the server |
| `client_name` | Yes | The name of the API client to authenticate as |
| `client_key_path` | Conditional | Absolute path to the RSA private key file for the API client. Required unless `client_key_credential` is set. |
| `client_key_credential` | Conditional | Name of a credential in the `credentials` database table containing the RSA private key. Takes precedence over `client_key_path` if both are set. The credential must have `credential_type: chef_client_key`. |

> **Note:** At least one of `client_key_path` or `client_key_credential` must be specified per organisation. Organisations created via the Web API always use `client_key_credential` (the key is uploaded through the API and stored encrypted in the database).

---

### Target Chef Client Version

The single Chef Client version to test cookbook compatibility against. Changing this value invalidates all cookstyle and kitchen results and resets materialised status columns to 'untested'.

```yaml
target_chef_version: "18.5.0"
```

**Note:** The code currently stores this as a list (`target_chef_versions: [...]`) and picks the highest. This is tech debt — it should be a single scalar value. See `plans/todo-tech-debt.md`.

---

### Git Base URLs

A list of base URLs used to resolve cookbook git repositories. When fetching a cookbook from git, the application will attempt each base URL in order until a valid repository is found.

```yaml
git_base_urls:
  - https://github.com/myorg
  - https://gitlab.example.com/chef-cookbooks
```

---

### Collection Schedule

Controls how frequently the background node collection job runs.

The staleness model has three tiers: **Healthy** → **Missing** (amber) → **Gone** (red).

```yaml
collection:
  schedule: "0 * * * *"                   # cron expression — default: every hour
  stale_node_warning_hours: 72            # hours before a node is flagged Missing (amber)
  stale_node_critical_days: 7            # days before a node is flagged Gone (red)
  stale_node_threshold_days: 7            # legacy — stale_node_critical_days defaults to this value
  stale_cookbook_threshold_days: 365      # cookbooks not updated in this many days are flagged as stale
  skip_server_cookbook_download: false    # skip downloading cookbooks from Chef Server (scan git repos only)
  delete_server_cookbooks_after_scan: false  # delete downloaded server cookbook files after scanning
```

| Setting | Default | Description |
|---------|---------|-------------|
| `schedule` | `0 * * * *` | Cron expression controlling collection frequency |
| `stale_node_warning_hours` | `72` | Nodes whose `ohai_time` is older than this many hours are flagged **Missing** (amber). These nodes have missed at least one Chef run but may still recover. |
| `stale_node_critical_days` | `7` | Nodes whose `ohai_time` is older than this many days are flagged **Gone** (red). Defaults to `stale_node_threshold_days` for backward compatibility. |
| `stale_node_threshold_days` | `7` | Legacy single-threshold field. Still respected; `stale_node_critical_days` defaults to this value if not set explicitly. |
| `stale_cookbook_threshold_days` | `365` | Cookbooks whose most recent version was first observed longer than this many days ago are flagged as stale in the dashboard. This helps teams identify unmaintained cookbooks that may need attention beyond compatibility fixes. |
| `skip_server_cookbook_download` | `false` | When `true`, skips downloading cookbooks from the Chef Server. Only git-sourced cookbooks are scanned. Useful when Chef Server cookbook data is unreliable or irrelevant. |
| `delete_server_cookbooks_after_scan` | `false` | Controls whether downloaded Chef Server cookbook files are deleted after the scan pipeline runs. Enable this to minimise disk usage. The default of `false` retains files on disk so they can be inspected for troubleshooting. |

---

### Concurrency

Controls the size of the worker pool for each independently parallelised task. Each task type has a distinct resource profile (network I/O, CPU, disk) and must be tunable independently.

```yaml
concurrency:
  organisation_collection: 5   # Number of Chef server organisations to collect from in parallel
  node_page_fetching: 10       # Number of concurrent pagination requests within a single organisation
  git_pull: 10                 # Number of cookbook git repositories to pull in parallel
  cookbook_download: 4          # Number of concurrent cookbook downloads from the Chef Server API
  cookstyle_scan: 8            # Number of concurrent CookStyle scans
  test_kitchen_run: 4          # Number of concurrent Test Kitchen runs (CPU/disk intensive — keep lower)
  readiness_evaluation: 20     # Number of nodes to evaluate for upgrade readiness in parallel
```

| Setting | Default | Notes |
|---------|---------|-------|
| `organisation_collection` | `5` | Bounded by Chef server capacity and network. One goroutine per org. |
| `node_page_fetching` | `10` | Concurrent pagination requests within one org. Bounded by Chef server rate limits. |
| `git_pull` | `10` | Network-bound. Can be set higher on fast networks. |
| `cookbook_download` | `4` | Network-bound (Chef Server API). Each download fetches a manifest then individual files. Can be increased for large fleets with many pending cookbooks. |
| `cookstyle_scan` | `8` | CPU-bound but lightweight. Can typically match available CPU cores. |
| `test_kitchen_run` | `4` | CPU and disk intensive — set conservatively to avoid resource exhaustion. |
| `readiness_evaluation` | `20` | Pure computation against in-memory/datastore data — can be set high. |

---

### Analysis Tools

Controls the location and behaviour of the embedded CookStyle and Test Kitchen tools used for cookbook compatibility testing.

All packaging formats (RPM, DEB) ship with a self-contained Ruby environment under `/opt/chef-migration-metrics/embedded/` that includes CookStyle, Test Kitchen, the `kitchen-dokken` driver, and their gem dependencies. This eliminates external dependencies on Chef Workstation or system Ruby. See the [Packaging Specification](../packaging/Specification.md) for the embedded environment build and layout.

```yaml
analysis_tools:
  embedded_bin_dir: /opt/chef-migration-metrics/embedded/bin
  cookstyle_timeout_minutes: 10
  test_kitchen:
    enabled: true
    timeout_minutes: 30
    driver: dokken
    driver_settings: {}
    driver_secrets: {}
    image_field_name: ""
    images: []
    platform_map: []
```

| Setting | Default | Description |
|---------|---------|-------------|
| `embedded_bin_dir` | `/opt/chef-migration-metrics/embedded/bin` | Directory containing the embedded `cookstyle`, `kitchen`, and `ruby` binaries. At startup, the application looks for these tools here first. If the directory does not exist or the binaries are not found, the application falls back to `PATH` lookup. This fallback supports development environments and source builds where the embedded tree may not be present. |
| `cookstyle_timeout_minutes` | `10` | Maximum wall-clock time for a single CookStyle scan before the process is killed and the result recorded as failed. |
| `test_kitchen.enabled` | `true` | Master toggle for Test Kitchen testing. When set to `false`, Test Kitchen is disabled regardless of whether the `kitchen` and `docker` binaries are available. When `true` (the default), Test Kitchen is enabled automatically if both binaries are detected at startup. Set this to `false` to turn off Test Kitchen without removing Docker or Kitchen from the system. |
| `test_kitchen.timeout_minutes` | `30` | Maximum wall-clock time for a single Test Kitchen converge or verify step. Replaces `test_kitchen_timeout_minutes`. |
| `test_kitchen.driver` | `dokken` | Test Kitchen driver profile. Built-in profiles: `dokken`, `vcenter`, `vra`, `ec2`, `azurerm`, `google`, `vagrant`, `openstack`, `custom`. See [Test Kitchen Driver Abstraction](test-kitchen-drivers.md). |
| `test_kitchen.driver_settings` | `{}` | Driver connection settings as key-value pairs (plaintext). Keys are driver-specific (e.g. `vcenter_host`, `region`). |
| `test_kitchen.driver_secrets` | `{}` | Driver secret settings. Keys are driver setting names, values are credential names from the `credentials` table. |
| `test_kitchen.image_field_name` | set by profile | Driver-specific field name for the image identifier in the platform map. Required only for the `custom` profile. |
| `test_kitchen.images` | `[]` | Image registry list. Each entry defines a named image with its driver-specific identifier and optional per-image settings. See [Test Kitchen Driver Abstraction](test-kitchen-drivers.md) § ImageEntry Fields. |
| `test_kitchen.platform_map` | `[]` | Platform image mapping list. See [Test Kitchen Driver Abstraction](test-kitchen-drivers.md) § Platform Image Mapping. |

**Image Entry Fields** — each entry in `analysis_tools.test_kitchen.images[]`:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | Unique label used as the reference value in `platform_map[].image`. |
| `id` | string | required (non-dokken) | Driver-specific image identifier (template name, AMI ID, etc.). |
| `install_method` | string | `"download"` | How Chef is installed on instances using this image. `"download"` installs from the network; `"baked_in"` means Chef is pre-installed in the image. |
| `chef_client_path` | string | `""` | Path to the chef-client binary when `install_method` is `"baked_in"` (e.g. `/opt/chef/bin/chef-client`). Required for `baked_in`; ignored for `download`. |
| `driver_settings` | map | `{}` | Per-image driver setting overrides, merged on top of top-level `driver_settings`. |
| `transport` | object | nil | Transport credentials: `username`, `password_credential`, `ssh_key_credential`. |
| `chef_download_urls` | map | `{}` | Map of `version → URL`. When set for the target version, the overlay uses `download_url` instead of `product_version`. |

> **Path resolution order:** For `cookstyle` and `kitchen`, the application resolves binaries in this order:
> 1. `<embedded_bin_dir>/cookstyle` (or `kitchen`)
> 2. Standard `PATH` lookup
>
> This means a standard RPM/DEB/container installation uses the embedded tools automatically, while a developer running from source with `cookstyle` and `kitchen` installed via Chef Workstation or `gem install` will use their system copies.

For driver-specific configuration examples (vCenter, vRA, EC2), see [Test Kitchen Driver Abstraction](test-kitchen-drivers.md) § Configuration Schema.

> **Docker requirement:** The three analysis tools have independent Docker requirements. **CookStyle** never needs Docker — it runs as a host process performing static analysis. **Test Kitchen** only needs Docker when `test_kitchen.driver` is `dokken` (the default); non-dokken drivers (vcenter, ec2, vra, etc.) provision real VMs via their own APIs and have no Docker dependency. **ChefSpec** always needs Docker regardless of the Test Kitchen driver — it runs cookbook code inside disposable containers to isolate side effects from Ruby execution (see [ChefSpec Specification](chefspec.md) § 2. Why Docker). If Docker is unavailable: dokken-based Test Kitchen and ChefSpec are disabled, but CookStyle scanning and non-dokken Test Kitchen testing still function.

> **Disabling Test Kitchen:** To disable Test Kitchen without uninstalling Docker or Kitchen, set `analysis_tools.test_kitchen.enabled: false`. This is useful in environments where Docker is present for other purposes but Test Kitchen runs are not wanted (e.g. resource-constrained hosts, CI pipelines that only need CookStyle results, or during initial evaluation). When disabled, the startup log emits an informational message confirming the override.

---

### Upgrade Readiness

```yaml
readiness:
  install_path_linux: /apps/hab     # filesystem path where the Chef Client bundle is installed on Linux
  install_path_windows: 'd:\apps\hab'  # filesystem path on Windows
  install_size_mb: 2048             # disk space required for the install bundle (MB)
  min_remaining_free_percent: 20    # minimum % of the filesystem that must remain free after install
```

| Setting | Default | Description |
|---------|---------|-------------|
| `install_path_linux` | `/apps/hab` | Installation target path on Linux nodes. Used as the input to the longest-prefix-match filesystem lookup. |
| `install_path_windows` | `d:\apps\hab` | Installation target path on Windows nodes. Matched against the drive letter of each filesystem entry. |
| `install_size_mb` | `2048` | Disk space in MB required for the Chef Client bundle install. Both thresholds (absolute and percentage) must pass for a node to be considered ready. |
| `min_remaining_free_percent` | `20` | After reserving `install_size_mb`, at least this percentage of the filesystem's total capacity must remain free. Prevents installs that would leave a nearly-full volume. |

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

Controls the export of data to Elasticsearch for analysis with Kibana. The application writes NDJSON (newline-delimited JSON) files to a directory, which a Logstash pipeline reads and indexes into Elasticsearch. See the [Elasticsearch Export Specification](../elasticsearch/Specification.md) for the full document type reference, pipeline design, and ELK testing stack.

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

Controls the HTTP listener for the Web API and dashboard frontend. The server supports three TLS modes: plain HTTP (`off`), externally-managed certificates (`static`), and automatic certificate management via ACME (`acme`). See the [TLS and Certificate Management specification](../tls/Specification.md) for full details on certificate lifecycle, ACME challenge types, renewal, and security considerations.

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

Certificates are automatically reloaded on `SIGHUP` or when file changes are detected via filesystem watching. See [TLS specification § 2.3](../tls/Specification.md#23-certificate-reload).

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

See [TLS specification § 3](../tls/Specification.md#3-acme-automatic-certificate-management) for full details on challenge types, DNS provider configuration, certificate storage, renewal, multi-replica coordination, and rate limits.

#### Backward Compatibility

The previous `server.tls.enabled` boolean is deprecated but still recognised for backward compatibility:

- If `tls.enabled: true` is present and `tls.mode` is not set, the application treats this as `mode: static` and logs a deprecation warning.
- If `tls.enabled: false` (or absent) and `tls.mode` is not set, the application defaults to `mode: off`.
- If both `tls.enabled` and `tls.mode` are present, `tls.mode` takes precedence and `tls.enabled` is ignored (with a warning).

> **Note on HTTPS:** The [authentication specification](../auth/Specification.md) requires all login flows to be over HTTPS. In production, enable native TLS (static or ACME mode) or place the application behind a TLS-terminating reverse proxy.

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

The frontend communicates with the backend exclusively through the `/api/v1` endpoints documented in the [Web API specification](../web-api/Specification.md). All routes not matching `/api/` serve the SPA's `index.html` to support client-side routing.

---

### Logging

```yaml
logging:
  level: INFO                # One of: DEBUG, INFO, WARN, ERROR
  retention_days: 90         # Number of days to retain log entries before purging
```

---

### Ownership

Controls ownership tracking features. When disabled (default), all ownership UI elements are hidden and ownership tables are not populated. See the [Ownership Specification](../ownership/Specification.md) for the full feature design.

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
| `ownership.auto_rules` | `[]` | List of auto-derivation rules. See [Ownership Specification](../ownership/Specification.md) § 2.2 for rule types and field definitions. |

---

### Authentication

See the [Authentication and Authorisation specification](../auth/Specification.md) for full details. Authentication providers are configured under the `auth` key.

```yaml
auth:
  providers:
    - type: local

    - type: saml
      idp_metadata_url: https://idp.example.com/saml/metadata
      sp_entity_id: chef-migration-metrics
```

---

## Environment Variable Overrides

Any configuration value may be overridden by an environment variable using the following naming convention:

```
CHEF_MIGRATION_METRICS_<SECTION>_<KEY>
```

The following environment variables are explicitly supported for sensitive values:

| Environment Variable | Description |
|----------------------|-------------|
| `CHEF_MIGRATION_METRICS_CONFIG` | Path to the configuration file |
| `CMM_CREDENTIAL_ENCRYPTION_KEY` | Base64-encoded AES-256 master key for encrypting/decrypting database-stored credentials. Required when any `*_credential` config references or Web API-created credentials exist. |
| `CMM_CREDENTIAL_ENCRYPTION_KEY_PREVIOUS` | Base64-encoded previous master key, required only during key rotation. Remove after successful rotation. |
| `DATABASE_URL` | Full datastore connection URL, overrides `datastore.url` |
| `CHEF_MIGRATION_METRICS_SERVER_PORT` | Override `server.port` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_MODE` | Override `server.tls.mode` (`off`, `static`, `acme`) |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_CERT_PATH` | Override `server.tls.cert_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_KEY_PATH` | Override `server.tls.key_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_CA_PATH` | Override `server.tls.ca_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_MIN_VERSION` | Override `server.tls.min_version` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_HTTP_REDIRECT_PORT` | Override `server.tls.http_redirect_port` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_EMAIL` | Override `server.tls.acme.email` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CA_URL` | Override `server.tls.acme.ca_url` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_CHALLENGE` | Override `server.tls.acme.challenge` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_DNS_PROVIDER` | Override `server.tls.acme.dns_provider` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_STORAGE_PATH` | Override `server.tls.acme.storage_path` |
| `CHEF_MIGRATION_METRICS_SERVER_TLS_ACME_AGREE_TO_TOS` | Override `server.tls.acme.agree_to_tos` |
| `CHEF_MIGRATION_METRICS_ANALYSIS_TOOLS_EMBEDDED_BIN_DIR` | Override `analysis_tools.embedded_bin_dir` — path to directory containing embedded `cookstyle`, `kitchen`, and `ruby` binaries |
| `CHEF_MIGRATION_METRICS_ELASTICSEARCH_ENABLED` | Override `elasticsearch.enabled` — set to `true` to enable Elasticsearch NDJSON export |
| `CHEF_MIGRATION_METRICS_ELASTICSEARCH_OUTPUT_DIRECTORY` | Override `elasticsearch.output_directory` — path where NDJSON files are written |
| `CMM_OWNERSHIP_ENABLED` | Override `ownership.enabled` |
| `CMM_OWNERSHIP_AUDIT_LOG_RETENTION_DAYS` | Override `ownership.audit_log.retention_days` |

---

## Validation

On startup, the application must validate the configuration and fail fast with a descriptive error message if:

- Any required field is missing
- A referenced key file does not exist or is not readable
- A `*_credential` reference names a credential that does not exist in the `credentials` table (log `ERROR` with the credential name and the config field that references it)
- A `*_credential` reference exists but `CMM_CREDENTIAL_ENCRYPTION_KEY` is not set or is invalid (fatal — cannot decrypt database credentials)
- `CMM_CREDENTIAL_ENCRYPTION_KEY` is set but is not valid Base64 or decodes to fewer than 32 bytes (fatal)
- A credential in the `credentials` table cannot be decrypted with the current or previous master key (log `ERROR` per credential; fatal if the credential is required by an active organisation or provider)
- An organisation has neither `client_key_path` nor `client_key_credential` configured (no credential source available)
- The datastore is not reachable
- An unknown configuration key is present (to catch typos)
- A cron expression is invalid
- A target Chef Client version string is not a valid semver
- The `server.port` is not a valid port number (1–65535)
- `server.tls.mode` is not one of `off`, `static`, or `acme`
- `server.tls.min_version` is not one of `"1.2"` or `"1.3"` (when mode is `static` or `acme`)
- `server.tls.http_redirect_port` is set but is not a valid port number (1–65535)
- **Static mode validation:**
  - `server.tls.mode` is `static` but `cert_path` or `key_path` is missing or empty
  - The certificate file at `cert_path` does not exist or is not readable
  - The key file at `key_path` does not exist or is not readable
  - The certificate and key do not form a valid pair
  - The certificate is expired at startup time (log `WARN` — do not prevent startup, as the operator may be in the process of renewing)
  - `ca_path` is set but the file does not exist or is not a valid PEM bundle
- **ACME mode validation:**
  - `server.tls.mode` is `acme` but `acme.domains` is empty
  - `server.tls.mode` is `acme` but `acme.email` is empty
  - `server.tls.mode` is `acme` but `acme.agree_to_tos` is not `true`
  - `acme.storage_path` does not exist or is not writable
  - `acme.challenge` is not one of `http-01`, `tls-alpn-01`, or `dns-01`
  - `acme.challenge` is `dns-01` but `acme.dns_provider` is empty
  - `acme.challenge` is `dns-01` but required `dns_provider_config` keys for the selected provider are missing
  - `acme.challenge` is `http-01` but `http_redirect_port` is `0` (fatal — the HTTP-01 challenge cannot be served)
  - `acme.renew_before_days` is less than 1 or greater than 89
  - `acme.ca_url` is not a valid URL
  - `acme.trusted_roots` is set but the file does not exist or is not a valid PEM bundle
- **Backward compatibility:** Both `server.tls.enabled` and `server.tls.mode` are present (log `WARN` — `mode` takes precedence)
- The exports output directory does not exist or is not writable
- `stale_node_threshold_days` or `stale_cookbook_threshold_days` is less than 1
- `analysis_tools.cookstyle_timeout_minutes` is less than 1
- `analysis_tools.test_kitchen_timeout_minutes` is less than 1
- `analysis_tools.embedded_bin_dir` is set to a non-empty value but the directory does not exist (log `WARN` — not fatal, as the application falls back to `PATH` lookup)
- `elasticsearch.output_directory` does not exist or is not writable when `elasticsearch.enabled` is `true`
- `elasticsearch.retention_hours` is less than 1
- `ownership.auto_rules[].name` must be unique across all rules
- `ownership.auto_rules[].owner` must reference an existing owner when auto-derivation runs (validated at rule evaluation time, not startup — owners may be created after config is written)
- `ownership.auto_rules[].type` must be one of: `node_attribute`, `node_name_pattern`, `policy_match`, `cookbook_name_pattern`, `git_repo_url_pattern`, `role_match`
- `ownership.auto_rules[].pattern` must be a valid Go regex when required by the rule type
- `ownership.auto_rules[].attribute_path` is required when type is `node_attribute`
- `ownership.auto_rules[].match_value` is required when type is `node_attribute`
- `ownership.auto_rules[].policy_name` is required when type is `policy_match`
- `ownership.audit_log.retention_days` must be a non-negative integer

---

> **Note:** See [Web API specification § WebSocket Real-Time Events](../web-api/Specification.md#websocket-real-time-events) for the event types, envelope format, and client reconnection behaviour.

---

## Full Example


```yaml
# -- Credential encryption (required when using database-stored credentials) --
# The actual key value must be set via environment variable, not inlined here.
credential_encryption_key_env: CMM_CREDENTIAL_ENCRYPTION_KEY

organisations:
  # File-based key
  - name: myorg-production
    chef_server_url: https://chef.example.com
    org_name: myorg-production
    client_name: chef-migration-metrics
    client_key_path: /etc/chef-migration-metrics/keys/myorg-production.pem

  # Database-stored key (created via Web API)
  # - name: myorg-staging
  #   chef_server_url: https://chef.example.com
  #   org_name: myorg-staging
  #   client_name: chef-migration-metrics
  #   client_key_credential: myorg-staging-key

target_chef_versions:
  - "18.5.0"
  - "19.0.0"

git_base_urls:
  - https://github.com/myorg
  - https://gitlab.example.com/chef-cookbooks

collection:
  schedule: "0 * * * *"
  stale_node_threshold_days: 7
  stale_cookbook_threshold_days: 365

# Database migrations are applied automatically on startup.
# No configuration is required — migration files are embedded in the binary.

concurrency:
  organisation_collection: 5
  node_page_fetching: 10
  git_pull: 10
  cookbook_download: 4
  cookstyle_scan: 8
  test_kitchen_run: 4
  readiness_evaluation: 20

readiness:
  install_path_linux: /apps/hab
  install_path_windows: 'd:\apps\hab'
  install_size_mb: 2048
  min_remaining_free_percent: 20

datastore:
  url: postgres://localhost:5432/chef_migration_metrics

server:
  listen_address: "0.0.0.0"
  port: 8080
  tls:
    mode: "off"
  websocket:
    enabled: true
    max_connections: 100
    send_buffer_size: 64
    write_timeout_seconds: 10
    ping_interval_seconds: 30
    pong_timeout_seconds: 60
    # --- Static certificate example (uncomment and set mode: static) ---
    # cert_path: /etc/chef-migration-metrics/tls/server.crt
    # key_path: /etc/chef-migration-metrics/tls/server.key
    # ca_path: ""
    # min_version: "1.2"
    # http_redirect_port: 80
    # --- ACME example (uncomment and set mode: acme) ---
    # acme:
    #   domains:
    #     - chef-metrics.example.com
    #   email: admin@example.com
    #   ca_url: https://acme-v02.api.letsencrypt.org/directory
    #   challenge: http-01
    #   storage_path: /var/lib/chef-migration-metrics/acme
    #   renew_before_days: 30
    #   agree_to_tos: true
  graceful_shutdown_seconds: 30

frontend:
  base_path: "/"

logging:
  level: INFO
  retention_days: 90

ownership:
  enabled: false
  audit_log:
    retention_days: 365
  auto_rules: []

auth:
  providers:
    - type: local

notifications:
  enabled: false
  channels: []
  readiness_milestones:
    - 50
    - 75
    - 90
    - 100
  stale_node_alert_count: 50

exports:
  max_rows: 100000
  async_threshold: 10000
  output_directory: /var/lib/chef-migration-metrics/exports
  retention_hours: 24

analysis_tools:
  embedded_bin_dir: /opt/chef-migration-metrics/embedded/bin
  cookstyle_timeout_minutes: 10
  test_kitchen:
    enabled: true
    timeout_minutes: 30
    driver: dokken
    images:
      - name: alma9
        id: tmpl-alma9-kitchen
        install_method: download
      - name: ubuntu2204-baked
        id: tmpl-ubuntu2204-baked
        install_method: baked_in
        chef_client_path: /opt/chef/bin/chef-client

elasticsearch:
  enabled: false
  output_directory: /var/lib/chef-migration-metrics/elasticsearch
  retention_hours: 48
```