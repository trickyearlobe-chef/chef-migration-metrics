# Configuration — Configuration Schema (Collection & Analysis)

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

All packaging formats (RPM, DEB) ship with a self-contained Ruby environment under `/opt/chef-migration-metrics/embedded/` that includes CookStyle, Test Kitchen, the `kitchen-dokken` driver, and their gem dependencies. This eliminates external dependencies on Chef Workstation or system Ruby. See the [Packaging Specification](packaging.md) for the embedded environment build and layout.

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

> **Docker requirement:** The two analysis tools have independent Docker requirements. **CookStyle** never needs Docker — it runs as a host process performing static analysis. **Test Kitchen** only needs Docker when `test_kitchen.driver` is `dokken` (the default); non-dokken drivers (vcenter, ec2, vra, etc.) provision real VMs via their own APIs and have no Docker dependency. If Docker is unavailable: dokken-based Test Kitchen is disabled, but CookStyle scanning and non-dokken Test Kitchen testing still function.

> **Disabling Test Kitchen:** To disable Test Kitchen without uninstalling Docker or Kitchen, set `analysis_tools.test_kitchen.enabled: false`. This is useful in environments where Docker is present for other purposes but Test Kitchen runs are not wanted (e.g. resource-constrained hosts, CI pipelines that only need CookStyle results, or during initial evaluation). When disabled, the startup log emits an informational message confirming the override.
