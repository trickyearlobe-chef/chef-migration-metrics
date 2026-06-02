# Kitchen Analyser — Specification

## TL;DR

Scans all cloned git repos for Test Kitchen configuration files, parses them using full YAML parsing (not line-scanning), merges `.kitchen.local.yml` overrides the way TK does, and builds a catalogue of every unique platform definition, driver configuration, suite, and transport pattern across the estate. Results are stored in the DB and exposed via API to seed the platform mapping UI with real discovery data.

## Motivation

Platform mapping requires knowing what platform names exist across the cookbook estate. Different customers have wildly different naming conventions — one customer has ~80 distinct platform names for ~8 canonical OS targets, with custom extension attributes (`x-custom-box_type`, `x-custom-size`), baked-in Chef versions in image names, and inconsistent naming (`windows2k12` vs `win2012r2-vanilla` vs `windows-2012R2`). Regex-guessing doesn't scale.

The analyser also detects `.kitchen.local.yml` files that would conflict with CMM's overlay generation, identifies cookbooks with no test suite, and flags unusual configurations before bulk runs.

## When It Runs

- **After git clone/fetch** — as part of the collection cycle, after repos are cloned/updated but before any TK execution.
- **On demand** — triggerable via API for re-analysis without a full collection cycle.
- Runs per-repo. Results are upserted (latest analysis wins).

## What It Discovers Per Cookbook

### Kitchen Files

For each git repo, scan for kitchen config files. TK's own resolution order:
1. `.kitchen.yml` (primary)
2. `.kitchen.local.yml` (local overrides, merged on top)
3. `.kitchen.*.yml` variants (alternative configs, not auto-merged)

Record which files exist and their paths.

### Merged Config

Parse `.kitchen.yml` and merge `.kitchen.local.yml` on top (if present), using TK's merge semantics: top-level keys are deep-merged, arrays are replaced. This produces the "effective config" that TK would actually use.

### Extracted Fields

From the merged config, extract and store:

| Field | Source | Example |
|---|---|---|
| `driver_name` | `driver.name` or `driver_plugin` | `vagrant`, `dokken`, `ec2` |
| `driver_settings` | `driver` or `driver_config` block (excluding secrets) | `{provider: openstack, vagrantfile_erb: ...}` |
| `provisioner_name` | `provisioner.name` | `chef_zero`, `chef_solo` |
| `provisioner_settings` | Key provisioner fields | `{require_chef_omnibus: false}` |
| `platforms` | Full platform list with all attributes | See Platform Detail below |
| `suites` | Suite names, run_lists, excludes/includes | See Suite Detail below |
| `transport_default` | Top-level `transport` block | `{ssh_key: ./.ssh/testkitchen-pem}` |
| `has_local_override` | Whether `.kitchen.local.yml` exists | `true` / `false` |
| `local_override_keys` | Which top-level keys the local override touches | `[driver, transport]` |
| `variant_files` | List of `.kitchen.*.yml` files found | `[.kitchen-vagrant_tklibvirt_windows.yml]` |

### Platform Detail

Each platform entry:

| Field | Description |
|---|---|
| `name` | Platform name as written (e.g. `rhel7-chef16`) |
| `normalised_name` | Best-effort normalised form (e.g. `rhel-7`) — see Normalisation below |
| `os_family` | Detected OS family: `rhel`, `windows`, `debian`, `suse`, `other` |
| `os_version` | Detected version if parseable |
| `extensions` | Custom extension attributes (e.g. `{x-custom-box_type: stable, x-custom-size: small}`) |
| `driver_overrides` | Per-platform driver settings (e.g. `{image_ref: Win2012R2-Vanilla-Chef}`) |
| `transport_overrides` | Per-platform transport (e.g. `{name: winrm}`) |

### Suite Detail

Each suite entry:

| Field | Description |
|---|---|
| `name` | Suite name |
| `run_list` | Recipes and/or roles |
| `excludes` | Excluded platform names |
| `includes` | Included platform names (if specified) |
| `has_inspec_tests` | Whether InSpec test path exists on disk |

### Platform Name Normalisation

Best-effort normalisation to group variants:

1. Lowercase the name
2. Strip known suffixes: `-chef16`, `-chef13`, `-x86_64`, `-stable`, `-testing`, `-small`, `-large`, `-medium`
3. Normalise OS prefixes: `windows2k` → `windows-`, `win` → `windows-`, `centos` → `centos-`
4. Normalise version formats: `2k12` → `2012`, `2k16` → `2016`, `2k19` → `2019`
5. Result is `{os_family}-{version}` where parseable, or `other-{original}` if not

This is advisory — used for grouping in the UI, not for mapping. Users map the original names.

## Aggregate Summary

Across the entire estate, compute:

| Metric | Description |
|---|---|
| `total_cookbooks_scanned` | Repos with at least one kitchen file |
| `total_without_kitchen` | Repos with no kitchen config at all |
| `platform_summary` | Each unique platform name with: count of cookbooks using it, normalised form, OS family |
| `driver_summary` | Each driver name with count |
| `transport_summary` | SSH count, WinRM count, other |
| `local_override_count` | How many cookbooks have `.kitchen.local.yml` |
| `local_override_conflicts` | Cookbooks where local override touches `driver` or `platforms` (potential conflict with CMM overlay) |
| `provisioner_summary` | `require_chef_omnibus` true/false counts, pinned version counts |

## Database Schema

### Table: `kitchen_analysis_results`

One row per git repo. Upserted on each analysis run.

| Column | Type | Description |
|---|---|---|
| `git_repo_url` | TEXT NOT NULL | FK to `git_repos` (PK part) |
| `git_repo_name` | TEXT NOT NULL | FK to `git_repos` (PK part) |
| `analysed_at` | TIMESTAMPTZ NOT NULL | When this analysis was performed |
| `head_commit_sha` | TEXT NOT NULL | Commit SHA that was analysed |
| `kitchen_files` | JSONB NOT NULL | List of discovered kitchen file paths |
| `has_local_override` | BOOLEAN NOT NULL | `.kitchen.local.yml` exists |
| `local_override_keys` | JSONB | Top-level keys touched by local override |
| `driver_name` | TEXT | Detected driver name |
| `provisioner_name` | TEXT | Detected provisioner name |
| `require_chef_omnibus` | BOOLEAN | From provisioner config |
| `platforms` | JSONB NOT NULL | Array of platform detail objects |
| `suites` | JSONB NOT NULL | Array of suite detail objects |
| `transport_type` | TEXT | Default transport: `ssh`, `winrm`, `dokken`, `unknown` |
| `extensions` | JSONB | Custom extension attributes found (e.g. `x-custom-*`) |
| `variant_files` | JSONB | List of `.kitchen.*.yml` variant file paths |
| `created_at` | TIMESTAMPTZ DEFAULT now() | Row creation |
| `updated_at` | TIMESTAMPTZ DEFAULT now() | Last update |

**Primary key:** `(git_repo_name, git_repo_url)`
**Foreign key:** `(git_repo_name, git_repo_url)` → `git_repos(name, git_repo_url)` ON DELETE CASCADE
**Index:** `idx_kitchen_analysis_driver` on `driver_name`

### Table: `kitchen_discovered_platforms`

Denormalised aggregate table. Rebuilt on each full analysis run.

| Column | Type | Description |
|---|---|---|
| `platform_name` | TEXT NOT NULL | Original platform name as written |
| `normalised_name` | TEXT NOT NULL | Normalised form |
| `os_family` | TEXT NOT NULL | Detected OS family |
| `os_version` | TEXT | Detected version |
| `cookbook_count` | INTEGER NOT NULL | How many cookbooks use this platform |
| `has_extensions` | BOOLEAN NOT NULL | Whether custom extensions are present |
| `common_extensions` | JSONB | Most common extension values for this platform |
| `transport_type` | TEXT | Predominant transport for this platform |
| `updated_at` | TIMESTAMPTZ DEFAULT now() | Last rebuild |

**Primary key:** `(platform_name)`

## API Endpoints

### `GET /api/v1/kitchen/analysis/summary`

Returns the aggregate summary (platform counts, driver counts, override conflicts, etc.)

### `GET /api/v1/kitchen/analysis/platforms`

Returns all discovered platforms with counts, normalised names, OS family. Supports query params: `?os_family=rhel`, `?min_count=5`, `?unmapped=true` (platforms not yet in the platform map).

### `GET /api/v1/kitchen/analysis/cookbooks`

Returns per-cookbook analysis results. Supports filtering by driver, platform, has_local_override. Paginated.

### `GET /api/v1/kitchen/analysis/cookbooks/:name`

Returns detailed analysis for a single cookbook.

### `POST /api/v1/kitchen/analysis/trigger`

Triggers a re-analysis of all (or specified) git repos without a full collection cycle.

## Frontend

### Admin → Kitchen Analysis page

- **Summary cards**: total cookbooks with TK, without TK, platform count, driver breakdown, override conflict count
- **Platform table**: sortable by name, count, OS family. Shows mapped/unmapped status. Click to see which cookbooks use it.
- **Conflict list**: cookbooks with `.kitchen.local.yml` that touch driver/platforms — these need attention before bulk runs.
- **Link to platform mapping**: "Map these platforms to templates →"

## Relationship to Platform Mapping

The analyser **discovers** what's in the estate. The platform mapping UI (existing, enhanced) lets the user **map** discovered platforms to hypervisor templates. The analyser seeds the mapping UI but does not create mappings automatically.

Unmapped platforms are flagged in the analyser UI with a "needs mapping" indicator and count of affected cookbooks.

## Related Specifications

- `test-kitchen-drivers.md` — Platform mapping config schema
- `test-kitchen-config-ui.md` — Admin UI for TK configuration
- `kitchen-refactor.md` — Overarching refactoring spec
- `analysis.md` — Analysis pipeline (TK is step 12)
- `datastore.md` — Database conventions