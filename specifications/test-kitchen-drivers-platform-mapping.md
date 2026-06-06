# Test Kitchen Drivers — Platform Image Mapping and Coverage Analysis

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
