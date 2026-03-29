# Test Kitchen Driver Abstraction — Specification

## TL;DR

Generalises Test Kitchen compatibility testing from a Docker-only (`kitchen-dokken`) model to a pluggable driver architecture. Cookbook repos keep their existing `.kitchen.yml` (LibVirt, Vagrant, or whatever they have today) untouched — the application generates a `.kitchen.local.yml` overlay that overrides the driver, injects credentials from the encrypted secret store, and remaps platform names to driver-specific images. A platform coverage analysis compares kitchen-tested platforms against actual production node data to flag untested gaps. Adding a new driver (vCenter today, vRA tomorrow, EC2 next quarter) is a configuration change, not a code change.

## Overview

The existing analysis component (analysis.md §2) hardcodes `kitchen-dokken` as the sole Test Kitchen driver. Packaging already ships 10 drivers including `kitchen-vcenter`, `kitchen-vra`, `kitchen-ec2`, `kitchen-azurerm`, `kitchen-google`, and others (packaging.md §4.5). This spec adds:

1. **Driver override** — generate `.kitchen.local.yml` overlays that replace the driver block for any supported driver, so existing cookbook repos need no reconfiguration.
2. **Credential injection** — driver passwords, access keys, and transport secrets come from the encrypted credentials table, never hardcoded.
3. **Platform image mapping** — translates kitchen platform names to driver-specific image identifiers (vSphere templates, AMI IDs, Azure images, etc.).
4. **Platform coverage analysis** — cross-references kitchen platforms against production node data to find untested gaps.
5. **Driver migration path** — switching drivers (e.g. vCenter → vRA, or vCenter → EC2) is a YAML config change with no code modifications.

## Driver Override Mechanism

The application already generates a `.kitchen.local.yml` to override the provisioner for target Chef Client versions (analysis.md §2, step 3). This spec extends that overlay to optionally replace the entire `driver:` block and per-platform driver settings.

### Driver Model

Every Test Kitchen driver needs the same three things from the application:

1. **Connection settings** — a bag of key-value pairs injected into the overlay's top-level `driver:` block. Some values are plaintext (host, username, region), others are secrets resolved at runtime.
2. **Platform image mapping** — a per-platform lookup from the kitchen platform name to a driver-specific image identifier (template name, AMI ID, Docker image, etc.).
3. **Transport settings** — optional per-platform SSH/WinRM credentials for connecting to provisioned instances.

The application does not need to understand driver semantics. It treats driver settings as opaque key-value pairs that get serialised into the `.kitchen.local.yml`. Driver-specific knowledge lives entirely in the configuration, not in code.

### Built-in Driver Profiles

To reduce configuration burden, the application ships built-in profiles for common drivers. A profile defines which config keys to expect and which image field name to use in the platform map.

| Profile | Driver Gem | Image Field | Typical Secrets |
|---------|-----------|-------------|-----------------|
| `dokken` | kitchen-dokken | `docker_image` | None (Docker socket) |
| `vcenter` | kitchen-vcenter | `template` | `vcenter_password` |
| `vra` | kitchen-vra | `password` | `password` |
| `ec2` | kitchen-ec2 | `ami` | `aws_secret_access_key` |
| `azurerm` | kitchen-azurerm | `image_urn` | `client_secret` |
| `google` | kitchen-google | `image_family` | `service_account_json` |
| `vagrant` | kitchen-vagrant | `box` | None |
| `openstack` | kitchen-openstack | `image_ref` | `os_password` |
| `custom` | any | configurable | configurable |

The `custom` profile allows any driver gem shipped in the embedded Ruby environment to be used without a built-in profile. The operator supplies all field mappings in config.

### Override Rules

- The overlay MUST replace the entire `driver:` block from the cookbook's `.kitchen.yml`, including any per-platform driver settings.
- The overlay MUST preserve the cookbook's `suites:` and `verifier:` configuration (Test Kitchen merges `.kitchen.local.yml` on top of `.kitchen.yml`; suites and verifier are not included in the overlay).
- The overlay MUST include the provisioner override for the target Chef Client version (existing behaviour).
- Platforms in the cookbook's `.kitchen.yml` that are not present in the platform map are excluded from the overlay and skipped with a `WARN` log.
- For `dokken` with no platform map configured, behaviour is unchanged (all platforms pass through, Docker images used directly).

## Credential Model

### Design Principles

Driver credentials use the existing `generic` credential type in the credentials table. There is no per-driver credential type — a password is a password regardless of whether it authenticates to vCenter, vRA, AWS, or Azure. The credential's `name` and `metadata` JSONB field provide context (e.g. `{"driver": "vcenter", "host": "vcenter.example.com"}`).

This avoids credential type proliferation as new drivers are added. The `generic` type already validates as non-empty string, which is sufficient for all driver secrets.

### Connection Secrets

Each driver configuration block has a `secrets` map. Keys are driver setting names; values are credential references.

```
test_kitchen:
  driver_settings:
    vcenter_host: vcenter.example.com
    vcenter_username: user@vsphere.local
  driver_secrets:
    vcenter_password: vcenter-password        # → credentials.name
```

At runtime each secret is resolved via the credential resolver (secrets-storage.md § Credential Resolution Precedence): database → environment variable → error. Resolved values are set as process environment variables named `CMM_TK_SECRET_<UPPER_KEY>` (e.g. `CMM_TK_SECRET_VCENTER_PASSWORD`). The overlay references them via ERB: `<%= ENV['CMM_TK_SECRET_VCENTER_PASSWORD'] %>`.

Environment variable names are derived deterministically: prefix `CMM_TK_SECRET_`, then the `driver_secrets` key uppercased with hyphens replaced by underscores.

### Transport Secrets

Per-platform transport credentials (SSH/WinRM passwords) are referenced in the platform map entry's `transport` block. They follow the same pattern — a credential name resolved at runtime, injected via environment variable.

Environment variable naming for transport secrets: `CMM_TK_TRANSPORT_<NORMALIZED_PLATFORM>` where the platform name is uppercased with hyphens and dots replaced by underscores (e.g. `ubuntu-22.04` → `CMM_TK_TRANSPORT_UBUNTU_22_04`).

### Plaintext Handling

All credential values are zeroed from memory after the `.kitchen.local.yml` is written and the environment variables are set. The environment variables persist only for the duration of the Test Kitchen child process and are cleared after the process exits (same process group cleanup as existing behaviour).

## Platform Image Mapping

### Purpose

Cookbook `.kitchen.yml` files reference platforms by names meaningful to their original driver (e.g. LibVirt image `ubuntu-22.04`). These names rarely match identifiers in the target driver (e.g. vSphere template `tmpl-ubuntu-2204-base`, AMI `ami-0abcdef1234567890`, Azure URN `Canonical:0001-com-ubuntu-server-jammy:22_04-lts:latest`).

The platform map translates between them.

### Config Location

`analysis_tools.test_kitchen.platform_map` — a list of platform mapping objects.

### Platform Map Entry

| Field | Required | Description |
|-------|----------|-------------|
| `kitchen_name` | Yes | Platform name as it appears in the cookbook's `.kitchen.yml` |
| `image` | Yes (non-dokken) | Driver-specific image identifier. Injected into the overlay under the profile's image field name. |
| `driver_settings` | No | Per-platform driver settings (datacenter, cluster, resource pool, subnet, security group, etc.). Merged on top of top-level driver settings. |
| `transport` | No | Transport override: `username`, `password_credential` (credential name), `ssh_key_credential` (credential name for PEM key). |

The `image` field is intentionally a single opaque string. The built-in profile determines which driver key it maps to (`template` for vcenter, `ami` for ec2, `image_urn` for azurerm, etc.). For the `custom` profile, `image_field_name` is set in the driver config.

### Lookup Behaviour

1. For each platform in the cookbook's `.kitchen.yml`, look up `kitchen_name` in the map.
2. If found: include the platform in the overlay with `image` and any `driver_settings` / `transport` from the entry, merged with top-level defaults.
3. If not found: exclude the platform. Log at `WARN`: `platform "<name>" has no mapping, skipping`.
4. For `dokken` with an empty platform map: all platforms pass through unchanged (backward compatible).

### Driver Migration via Platform Map

Each platform map entry has a single `image` field. When the operator switches drivers (e.g. `vcenter` → `vra`), they update the `image` values in the platform map to the new driver's identifiers and change the top-level driver profile. The map structure itself does not change. This is the mechanism that makes driver migration a config change.

For operators who want to prepare both sets of identifiers in advance, they can maintain two config files and switch between them, or use environment variable overrides on the config file path.

## Platform Coverage Analysis

### Input Data

- **Production platforms:** `(platform, platform_version, platform_family)` tuples from `node_snapshots` for nodes consuming cookbook X. Joined via `cookbook_node_usage` for server cookbooks, or via cookbook name matching against `git_repos.name` for git-sourced cookbooks.
- **Kitchen platforms:** Platform names parsed from the cookbook's `.kitchen.yml` in the git repo working directory.

### Matching Rules

Kitchen platform names are free-form strings. Production node data uses structured Ohai attributes. Matching proceeds in order:

1. **Exact parse** — split kitchen name on the last hyphen: `ubuntu-22.04` → `(ubuntu, 22.04)`. Match against `(platform, platform_version)`.
2. **Major version** — `centos-7` matches `platform_version=7.9.2009` (compare major version prefix only).
3. **Family grouping** — if the parsed OS name matches a `platform_family` from node data, count it as a family-level match. E.g. kitchen name `rhel-9` matches nodes with `platform_family=rhel` regardless of whether `platform` is `centos`, `rocky`, `alma`, or `redhat`.
4. **Unparseable names** — if a kitchen platform name does not match the `<os>-<version>` pattern, it is reported as `unmatched` in the coverage data for manual review.

### Coverage Report Per Cookbook

| Category | Meaning | Severity |
|----------|---------|----------|
| Tested and in production | Platform in both kitchen and node data | OK |
| Tested but not in production | Platform in kitchen, no matching nodes | Info |
| In production but untested | Nodes on a platform with no kitchen coverage | Gap |

### Scheduling

Coverage is recomputed after each collection + analysis cycle, using the latest node snapshots and the current `.kitchen.yml` from the git repo HEAD.

### Exposure

- API: `GET /api/v1/cookbooks/:name/platform-coverage` returns the coverage report.
- Dashboard: per-cookbook coverage summary with gap highlighting on the cookbook detail page.

## Configuration Schema

Full YAML structure under `analysis_tools.test_kitchen`:

```
analysis_tools:
  test_kitchen:
    enabled: true
    timeout_minutes: 30

    # Driver selection — matches a built-in profile or "custom"
    driver: dokken

    # Top-level driver connection settings (plaintext)
    driver_settings:
      vcenter_host: vcenter.example.com
      vcenter_username: user@vsphere.local
      vcenter_disable_ssl_verify: false
      clone_type: full

    # Top-level driver secrets — values are credential names
    driver_secrets:
      vcenter_password: vcenter-password

    # For "custom" profile only — which key to use for the image field
    # Built-in profiles set this automatically.
    image_field_name: template

    # Platform image map
    platform_map:
      - kitchen_name: ubuntu-22.04
        image: tmpl-ubuntu-2204-base
        driver_settings:
          datacenter: "Datacenter"
          cluster: "Cluster-01"
          resource_pool: "Kitchen"
          folder: "kitchen-vms"
        transport:
          username: kitchen
          password_credential: kitchen-vm-password

      - kitchen_name: centos-7
        image: tmpl-centos-7-base
        driver_settings:
          datacenter: "Datacenter"

      - kitchen_name: windows-2022
        image: tmpl-win2022-base
        driver_settings:
          datacenter: "Datacenter"
          vm_customization:
            numCPUs: 4
            memoryMB: 4096
        transport:
          username: Administrator
          password_credential: kitchen-win-password
```

### Defaults

| Setting | Default |
|---------|---------|
| `driver` | `dokken` |
| `timeout_minutes` | `30` (existing `test_kitchen_timeout_minutes`) |
| `driver_settings` | empty map |
| `driver_secrets` | empty map |
| `platform_map` | empty list |
| `image_field_name` | set by built-in profile; required for `custom` |

### Driver Change Example: vCenter → vRA

Before (vCenter):

```
driver: vcenter
driver_settings:
  vcenter_host: vcenter.example.com
  vcenter_username: user@vsphere.local
driver_secrets:
  vcenter_password: vcenter-password
platform_map:
  - kitchen_name: ubuntu-22.04
    image: tmpl-ubuntu-2204-base
```

After (vRA):

```
driver: vra
driver_settings:
  base_url: https://vra.example.com
  username: user@example.com
  tenant: "my-tenant"
driver_secrets:
  password: vra-password
platform_map:
  - kitchen_name: ubuntu-22.04
    image: ubuntu-22.04-catalog-item
```

Only the `driver`, `driver_settings`, `driver_secrets`, and `image` values change. Map structure, transport credentials, and application code are untouched.

### Driver Change Example: vCenter → EC2

```
driver: ec2
driver_settings:
  region: us-west-2
  aws_access_key_id: AKIA...
  instance_type: t3.medium
  associate_public_ip: true
driver_secrets:
  aws_secret_access_key: aws-secret-key
platform_map:
  - kitchen_name: ubuntu-22.04
    image: ami-0abcdef1234567890
  - kitchen_name: centos-7
    image: ami-0fedcba9876543210
```

## Overlay Generation

### dokken (Unchanged)

When driver is `dokken` and no platform map is configured, the overlay contains the provisioner override only (existing behaviour):

```
# .kitchen.local.yml — generated by chef-migration-metrics
provisioner:
  product_version: "<TARGET_CHEF_VERSION>"
  chef_version: "<TARGET_CHEF_VERSION>"
```

### Non-dokken Drivers (Generic)

The overlay is assembled from config data without driver-specific code paths:

```
# .kitchen.local.yml — generated by chef-migration-metrics
driver:
  name: <DRIVER_PROFILE_NAME>
  <for each key,value in merged driver_settings>
  <key>: <value>
  <for each key,credential_name in driver_secrets>
  <key>: <%= ENV['CMM_TK_SECRET_<UPPER_KEY>'] %>

provisioner:
  product_version: "<TARGET_CHEF_VERSION>"

platforms:
  <for each platform in cookbook .kitchen.yml that exists in platform_map>
  - name: <kitchen_name>
    driver:
      <profile.image_field_name>: <image>
      <for each key,value in entry.driver_settings>
      <key>: <value>
    <if entry.transport>
    transport:
      username: <transport.username>
      <if transport.password_credential>
      password: <%= ENV['CMM_TK_TRANSPORT_<NORMALIZED_PLATFORM>'] %>
      <if transport.ssh_key_credential>
      ssh_key: <%= ENV['CMM_TK_KEY_<NORMALIZED_PLATFORM>'] %>
```

This is a template description, not literal code. The overlay is a YAML document assembled from config values. No driver-specific branching exists in the generation logic — the profile determines field names, and the config supplies values.

### Overlay Lifecycle

1. Write `.kitchen.local.yml` to the cookbook working directory.
2. Set `CMM_TK_SECRET_*` and `CMM_TK_TRANSPORT_*` environment variables in the child process environment.
3. Run `kitchen converge`, `kitchen verify`, `kitchen destroy` (existing sequence).
4. Remove `.kitchen.local.yml`.
5. Clear environment variables and zero credential memory.

## Startup Validation

| Condition | Check | Failure Behaviour |
|-----------|-------|-------------------|
| `driver: dokken` | Existing Docker check (unchanged) | Disable TK, `WARN` log |
| Any non-dokken driver | All `driver_secrets` reference credentials that exist and decrypt | Disable TK, `ERROR` log |
| Any non-dokken driver | `platform_map` is non-empty | `WARN`: no platforms, TK will skip all cookbooks |
| Any non-dokken driver | Each platform map entry has `image` set | `WARN` per entry without image; entry excluded |
| `driver: custom` | `image_field_name` is configured | Disable TK, `ERROR` log |
| Transport secrets | Each referenced `password_credential` or `ssh_key_credential` exists and decrypts | `WARN` per entry; platform still usable if auth uses other methods |

When a non-dokken driver is configured, the Docker startup check is skipped (Docker is not required).

## Database Changes

### Modified: `git_repo_test_kitchen_results`

New column:

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `driver` | TEXT | Yes | `NULL` | Driver used for the test run. NULL for pre-existing rows (implies `dokken`). |
| `platform_name` | TEXT | Yes | `NULL` | Kitchen platform name for this result (enables per-platform result tracking). |

The existing unique constraint `(git_repo_id, target_chef_version, commit_sha)` is unchanged. A cookbook is tested with whichever driver is currently configured. When the driver changes, the next HEAD change triggers a retest.

### New Table: `cookbook_platform_coverage`

| Column | Type | Nullable | Description |
|--------|------|----------|-------------|
| `id` | UUID | No | Primary key |
| `git_repo_id` | UUID | Yes | FK → `git_repos.id` (NULL for server-only cookbooks) |
| `cookbook_name` | TEXT | No | Cookbook name |
| `coverage_data` | JSONB | No | Coverage report (structure below) |
| `evaluated_at` | TIMESTAMPTZ | No | When coverage was last evaluated |
| `created_at` | TIMESTAMPTZ | No | Row creation time |
| `updated_at` | TIMESTAMPTZ | No | Last update time |

**Foreign keys:** `git_repo_id` → `git_repos(id)` ON DELETE CASCADE

**Unique constraints:** `(cookbook_name)`

**Indexes:** `idx_cookbook_platform_coverage_cookbook_name` on `cookbook_name`

### Coverage Data JSONB Structure

```
{
  "kitchen_platforms": ["ubuntu-22.04", "centos-7"],
  "production_platforms": [
    {"platform": "ubuntu", "platform_version": "22.04", "platform_family": "debian", "node_count": 47},
    {"platform": "centos", "platform_version": "7.9.2009", "platform_family": "rhel", "node_count": 12},
    {"platform": "rocky", "platform_version": "9.3", "platform_family": "rhel", "node_count": 8}
  ],
  "tested_and_in_production": [
    {"kitchen_name": "ubuntu-22.04", "platform": "ubuntu", "platform_version": "22.04", "node_count": 47},
    {"kitchen_name": "centos-7", "platform": "centos", "platform_version": "7.9.2009", "node_count": 12}
  ],
  "tested_not_in_production": [],
  "in_production_not_tested": [
    {"platform": "rocky", "platform_version": "9.3", "platform_family": "rhel", "node_count": 8}
  ],
  "gap_count": 1,
  "total_production_nodes": 67,
  "covered_node_count": 59,
  "coverage_percentage": 88.1
}
```

## Immediate Deployment: VMware vCenter

The first non-dokken deployment uses `kitchen-vcenter`. This section documents the specific config for that deployment as a reference, not as a spec constraint.

### vCenter Config Example

```
analysis_tools:
  test_kitchen:
    driver: vcenter
    driver_settings:
      vcenter_host: vcenter.example.com
      vcenter_username: user@vsphere.local
      vcenter_disable_ssl_verify: false
      clone_type: full
      datacenter: "Datacenter"
    driver_secrets:
      vcenter_password: vcenter-password
    platform_map:
      - kitchen_name: ubuntu-22.04
        image: tmpl-ubuntu-2204-base
        driver_settings:
          cluster: "Cluster-01"
          resource_pool: "Kitchen"
          folder: "kitchen-vms"
        transport:
          username: kitchen
          password_credential: kitchen-vm-password
      - kitchen_name: centos-7
        image: tmpl-centos-7-base
      - kitchen_name: windows-2022
        image: tmpl-win2022-base
        driver_settings:
          vm_customization:
            numCPUs: 4
            memoryMB: 4096
        transport:
          username: Administrator
          password_credential: kitchen-win-password
```

### vCenter Credential Setup

```
# Store the vCenter password via the admin API
POST /api/v1/admin/credentials
{
  "name": "vcenter-password",
  "credential_type": "generic",
  "value": "<password>"
}
```

### vCenter → vRA Migration

When the VMware team transitions from vCenter to vRA, the operator:

1. Stores the vRA password: `POST /api/v1/admin/credentials` with name `vra-password`.
2. Updates config: `driver: vra`, replaces `driver_settings` and `driver_secrets`, updates `image` values in the platform map.
3. Restarts the application. No code changes.

## Related Specifications

| Specification | Relevance |
|---------------|-----------|
| analysis.md | Parent spec for cookbook compatibility testing (§2). Overlay generation steps 3–8 are extended by this spec. |
| configuration.md | Config schema (§ Analysis Tools). The `test_kitchen` sub-section is extended. |
| secrets-storage.md | Credential encryption, resolution precedence, `generic` credential type. |
| datastore.md | `credentials` table, `git_repo_test_kitchen_results` table, new `cookbook_platform_coverage` table. |
| packaging.md | Embedded kitchen drivers shipped in all packaging formats (§4.5). |
| data-collection.md | Node attribute collection: `platform`, `platform_version`, `platform_family` (§1.4). |