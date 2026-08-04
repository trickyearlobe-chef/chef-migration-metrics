# Configuration — Configuration Schema (Collection & Analysis)

### Chef Server Organisations

A list of one or more Chef Infra Server organisations to collect data from. Each organisation is independently configured.

```yaml
organisations:
  # Option A: File-based key (traditional)
  - name: myorg-production
    chef_server_url: https://chef.example.com/organizations/myorg
    client_name: chef-migration-metrics
    client_key_path: /etc/chef-migration-metrics/keys/myorg-production.pem

  # Option B: Database-stored key (recommended for multi-org / container deployments)
  - name: myorg-staging
    chef_server_url: https://chef.example.com/organizations/myorg-staging
    client_name: chef-migration-metrics
    client_key_credential: myorg-staging-key
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | A unique friendly name for this organisation, used in logs and the UI and as the primary key that owns the collected data. Need not match the Chef org name. |
| `chef_server_url` | Yes | The **full** organisation URL, including the `/organizations/<org>` path (e.g. `https://chef.example.com/organizations/myorg`). Used directly as the Chef API base. The UI validates this shape before saving. |
| `org_name` | Derived | The Chef organisation name. **Derived** from the `/organizations/<org>` segment of `chef_server_url` when omitted (an explicit value is honoured); used for the client User-Agent label. Not entered in the UI. |
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
  readiness_evaluation: 20     # Number of nodes to evaluate for upgrade readiness in parallel
```

| Setting | Default | Notes |
|---------|---------|-------|
| `organisation_collection` | `5` | Bounded by Chef server capacity and network. One goroutine per org. |
| `node_page_fetching` | `10` | Concurrent pagination requests within one org. Bounded by Chef server rate limits. |
| `git_pull` | `10` | Network-bound. Can be set higher on fast networks. |
| `cookbook_download` | `4` | Network-bound (Chef Server API). Each download fetches a manifest then individual files. Can be increased for large fleets with many pending cookbooks. |
| `cookstyle_scan` | `8` | CPU-bound but lightweight. Can typically match available CPU cores. |
| `readiness_evaluation` | `20` | Pure computation against in-memory/datastore data — can be set high. |

> **Test Kitchen concurrency is not a `concurrency` worker.** Test Kitchen run
> throughput is bounded globally under `analysis_tools.test_kitchen` by
> `max_concurrent_vms` (peak concurrency) and the VM start-rate limiter
> (`start_rate_window_minutes` / `start_rate_max_per_window`), described below.
> There is no `concurrency.test_kitchen_run` setting and no per-batch limit.

---

### Analysis Tools

Controls the behaviour of the CookStyle and Test Kitchen tools used for cookbook compatibility testing.

CookStyle, Test Kitchen, and their Ruby runtime are **not** bundled. Cookbook compatibility testing requires **Chef Workstation** installed on the host; the application resolves `cookstyle` and `kitchen` from `PATH`. See the [Packaging Specification](packaging.md) for package contents.

```yaml
analysis_tools:
  cookstyle_timeout_minutes: 10
  test_kitchen:
    enabled: true
    timeout_minutes: 30
    driver: vcenter
    driver_settings: {}
    driver_secrets: {}
    image_field_name: ""
    images: []
    platform_map: []
```

| Setting | Default | Description |
|---------|---------|-------------|
| `cookstyle_timeout_minutes` | `10` | Maximum wall-clock time for a single CookStyle scan before the process is killed and the result recorded as failed. |
| `test_kitchen.enabled` | `true` | Master toggle for Test Kitchen testing. When set to `false`, Test Kitchen is disabled regardless of whether the `kitchen` binary is available on `PATH`. When `true` (the default), Test Kitchen is enabled automatically if `kitchen` is detected at startup. Set this to `false` to turn off Test Kitchen without removing Chef Workstation from the host. |
| `test_kitchen.timeout_minutes` | `30` | Maximum wall-clock time for a single Test Kitchen converge or verify step. Replaces `test_kitchen_timeout_minutes`. |
| `test_kitchen.max_concurrent_vms` | `2` | Global ceiling on concurrent Test Kitchen VMs across all runs and batches — the single concurrency knob (there is no per-batch limit). Live-tunable from the Test Kitchen admin page; `0` falls back to the default. |
| `test_kitchen.start_rate_window_minutes` | `0` (off) | VM start-rate limiter window, set to the DHCP lease time (e.g. `60`, `90`). Bounds *cumulative* lease consumption over a window, which peak concurrency alone does not. Active only together with `start_rate_max_per_window`. See [bulk-kitchen-scanning.md](bulk-kitchen-scanning.md). |
| `test_kitchen.start_rate_max_per_window` | `0` (off) | Maximum VM starts allowed per window, set to the usable DHCP pool size (e.g. `25`, `64`). Starts are evenly paced. Both rate fields are live (no restart); the limiter is disabled unless both are > 0. |
| `test_kitchen.driver` | none (required) | Test Kitchen driver profile — the operator must choose one; there is no default. Built-in profiles: `vcenter` (production), `proxmox` (proof-of-concept) are wired to a hypervisor backend; `vra`, `ec2`, `vagrant` are UI-dropdown placeholders / overlay stubs that are not yet implemented. See [Test Kitchen Driver Abstraction](test-kitchen-drivers.md). |
| `test_kitchen.driver_settings` | `{}` | Driver connection settings as key-value pairs (plaintext). Keys are driver-specific (e.g. `vcenter_host`, `region`). |
| `test_kitchen.driver_secrets` | `{}` | Driver secret settings. Keys are driver setting names, values are credential names from the `credentials` table. |
| `test_kitchen.image_field_name` | set by profile | Driver-specific field name for the image identifier in the platform map. Required only for the `custom` profile. |
| `test_kitchen.images` | `[]` | Image registry list. Each entry defines a named image with its driver-specific identifier and optional per-image settings. See [Test Kitchen Driver Abstraction](test-kitchen-drivers.md) § ImageEntry Fields. |
| `test_kitchen.platform_map` | `[]` | Platform image mapping list. See [Test Kitchen Driver Abstraction](test-kitchen-drivers.md) § Platform Image Mapping. |

**Image Entry Fields** — each entry in `analysis_tools.test_kitchen.images[]`:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | Unique label used as the reference value in `platform_map[].image`. |
| `id` | string | required | Driver-specific image identifier (template name, AMI ID, etc.). |
| `install_method` | string | `"download"` | How Chef is installed on instances using this image. `"download"` installs from the network; `"baked_in"` means Chef is pre-installed in the image. |
| `chef_client_path` | string | `""` | Path to the chef-client binary when `install_method` is `"baked_in"` (e.g. `/opt/chef/bin/chef-client`). Required for `baked_in`; ignored for `download`. |
| `driver_settings` | map | `{}` | Per-image driver setting overrides, merged on top of top-level `driver_settings`. |
| `transport` | object | nil | Transport credentials: `username`, `password_credential`, `ssh_key_credential`. |
| `chef_download_urls` | map | `{}` | Map of `version → URL`. When set for the target version, the overlay uses `download_url` instead of `product_version`. |

> **Path resolution:** `cookstyle` and `kitchen` are resolved from `PATH`, which requires **Chef Workstation** to be installed on the host. They are not bundled with the application.

For driver-specific configuration examples (vCenter, vRA, EC2), see [Test Kitchen Driver Abstraction](test-kitchen-drivers.md) § Configuration Schema.

> **Disabling Test Kitchen:** To disable Test Kitchen without uninstalling Chef Workstation, set `analysis_tools.test_kitchen.enabled: false`. This is useful in environments where Test Kitchen runs are not wanted (e.g. resource-constrained hosts, CI pipelines that only need CookStyle results, or during initial evaluation). When disabled, the startup log emits an informational message confirming the override.
