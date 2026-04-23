# Kitchen Refactoring — Specification

## TL;DR

Refactors Test Kitchen from a single-mode bulk scanner into a multi-component system with two execution modes (Git Kitchens and Node Kitchens), hypervisor integration for template discovery and VM lifecycle management, and batch controls for safe operation at scale. Supersedes `test-kitchen-mvp.md`. Extends `test-kitchen-drivers.md` with per-instance results, batch definitions, and cookbook exclusions.

## Motivation

A customer with ~2000 git cookbooks needs to migrate from LibVirt to kitchen-vcenter. The current implementation has critical gaps for this use case:

1. **No visibility** into what's in the existing kitchen configs — platform names, drivers, overrides, `.kitchen.local.yml` conflicts.
2. **No per-instance results** — one row per cookbook loses platform/suite granularity. A cookbook may pass on RHEL 7 but fail on Windows.
3. **No batch control** — all-or-nothing execution with no subset selection, dry-run, or exclusions. At 2000 cookbooks, failures are expensive.
4. **No VM lifecycle management** — orphaned VMs from failed `kitchen destroy` require manual cleanup. At scale, this is operationally dangerous.
5. **No end-to-end node testing** — can't test a node's full run_list as a unit, only individual cookbooks.

## Architecture Overview

Six components, built in dependency order:

| Component | Purpose | Depends On |
|---|---|---|
| **Kitchen Analyser** | Discover platforms, drivers, suites across the estate | Git repos (existing) |
| **Hypervisor Integration** | Template discovery, VM inventory, orphan management | Credential store (existing) |
| **Platform Mapping UI** | Map discovered platforms → hypervisor templates | Analyser, Hypervisor |
| **Node Kitchens** | End-to-end node config testing (on-demand, low volume) | Hypervisor, Platform Mapping |
| **Batch Definition** | Subset selection, exclusions, dry-run, concurrency limits | — |
| **Git Kitchens** | Bulk cookbook testing with per-instance results | Batch Definition, Hypervisor, Platform Mapping |

## Hypervisor Integration

### Purpose

Abstract hypervisor-specific operations behind a common interface. vCenter is the production target. Proxmox is used only as a minimal proof-of-concept to validate the interface design — keep Proxmox effort to the bare minimum needed to prove the abstractions work. The interface is deliberately narrow — CMM does not manage VMs directly (TK does that). CMM needs three things from the hypervisor: template discovery, VM inventory, and VM destruction (for orphan cleanup).

### Operations

| Operation | Description |
|---|---|
| `ListTemplates` | Return available VM templates (name, OS type, notes/description) |
| `ListManagedVMs` | Return VMs matching CMM's naming convention with creation time, power state, and resource consumption |
| `DestroyVM` | Force-destroy a specific VM by identifier (for orphan cleanup) |

### VM Naming Convention

All VMs created via CMM-generated TK overlays use a predictable naming pattern so they can be identified later:

Format: `cmm-{short-cookbook}-{suite}-{platform}-{unix-timestamp}`

The naming convention is injected into the `.kitchen.local.yml` overlay via the driver's `vm_name` or equivalent field.

### VM Tracking Table

CMM maintains its own record of VM lifecycle, independent of the hypervisor:

| Column | Type | Description |
|---|---|---|
| `id` | UUID PK | |
| `vm_name` | TEXT NOT NULL | Full VM name per convention |
| `hypervisor_id` | TEXT | Hypervisor-assigned identifier |
| `cookbook_name` | TEXT NOT NULL | Source cookbook |
| `suite_name` | TEXT NOT NULL | Kitchen suite |
| `platform_name` | TEXT NOT NULL | Kitchen platform |
| `batch_id` | UUID | FK to batch_runs if part of a batch |
| `created_at` | TIMESTAMPTZ NOT NULL | When CMM requested creation |
| `expected_destroy_at` | TIMESTAMPTZ | created_at + timeout |
| `actual_destroy_at` | TIMESTAMPTZ | When destruction confirmed |
| `status` | TEXT NOT NULL | `creating`, `running`, `destroying`, `destroyed`, `orphaned` |
| `updated_at` | TIMESTAMPTZ | |

**Orphan detection:** A VM is flagged `orphaned` if `status != destroyed` and `now() > expected_destroy_at`. A periodic sweep (or on-demand trigger) queries the hypervisor to confirm whether orphaned VMs still exist, and offers cleanup.

### TTL Safety Net

Configurable maximum VM lifetime (default: 4 hours). Any CMM-created VM exceeding this TTL is flagged for cleanup regardless of its kitchen run status. This prevents resource leaks from crashed processes, network partitions, or hung converges.

## Node Kitchens

### Concept

"End-to-end testing" — take a real node's configuration, assemble it into a Test Kitchen project, and run it on a matching hypervisor template. Tests whether a node's actual cookbook set will converge on the target Chef version.

### Trigger Model

On-demand only. User selects a node and triggers a run from the UI or API. Not part of the periodic collection cycle. Can be re-run with different parameters (cookbook source, target Chef version, platform override).

### Node Selection

User selects a node from `node_snapshots` by:
- Node name (with search/autocomplete)
- Organisation
- Platform filter (e.g. "show me RHEL 7 nodes")
- Browse from node list with sorting/filtering

### Run_list Expansion

From the selected node's `run_list` (array of `role[X]` and `recipe[Y::Z]` entries):

1. **Parse entries** — separate roles from recipes.
2. **Expand roles** — for each `role[X]`, fetch the role detail from the Chef Server API (`GET /organizations/{org}/roles/{name}`). Extract the role's `run_list` and `env_run_lists`. Recursively expand nested roles.
3. **Collect cookbooks** — from the expanded recipe list, determine which cookbooks are needed. Cross-reference with `node_snapshots.cookbooks` for the exact versions the node uses.
4. **Resolve transitive dependencies** — each cookbook's `dependencies` may pull in additional cookbooks not directly in the run_list. Expand using cookbook metadata from `server_cookbooks.dependencies` JSONB.

The result is: an ordered run_list (for the kitchen suite) and a complete cookbook set (for assembly).

### Cookbook Source

User chooses one of:

| Mode | Description | Answers |
|---|---|---|
| **As-is (Chef Server)** | Use exact cookbook versions currently on the server | "Will my current config survive the upgrade?" |
| **Known-good (Git)** | Use latest (or specified) versions from git repos | "Will the updated cookbooks work for this node?" |
| **Hybrid** | Git where available, Chef Server for the rest | "Best available version of everything" |

For Chef Server sourced cookbooks: download via Chef API (existing `DownloadFileContent` mechanism). For git-sourced: use the local clone.

### Synthetic Kitchen Config

Generate a `.kitchen.yml` for the node:

- **One platform** — the node's OS, mapped to a hypervisor template via platform mapping.
- **One suite** — the node's full expanded run_list.
- **Provisioner** — `chef_zero` with `product_version` set to the target Chef version. `require_chef_omnibus: true`.
- **Roles directory** — populated with the expanded role JSON files.
- **Cookbooks directory** — populated with resolved cookbooks.
- **Attributes** — optionally include the node's `custom_attributes` to reproduce its environment.

### Working Directory

Each Node Kitchen run operates in a temporary directory:

```
/tmp/cmm-node-kitchen-{node_name}-{timestamp}/
├── .kitchen.yml
├── .kitchen.local.yml      (driver/platform/credential overlay)
├── cookbooks/
│   ├── nginx/
│   ├── base/
│   └── ...
├── roles/
│   ├── webserver.json
│   └── base.json
└── data_bags/              (if available)
```

### Result Storage

Node Kitchen results are stored separately from Git Kitchen results:

| Column | Type | Description |
|---|---|---|
| `id` | UUID PK | |
| `node_name` | TEXT NOT NULL | Source node |
| `organisation_name` | TEXT NOT NULL | Node's organisation |
| `target_chef_version` | TEXT NOT NULL | Chef version tested against |
| `cookbook_source` | TEXT NOT NULL | `server`, `git`, or `hybrid` |
| `platform_name` | TEXT NOT NULL | Platform tested |
| `template_used` | TEXT | Hypervisor template name |
| `run_list` | JSONB NOT NULL | The expanded run_list used |
| `cookbook_versions` | JSONB NOT NULL | Map of cookbook name → version used |
| `converge_passed` | BOOLEAN | |
| `verify_passed` | BOOLEAN | NULL if no tests available |
| `converge_output` | TEXT | |
| `verify_output` | TEXT | |
| `destroy_output` | TEXT | |
| `duration_seconds` | INTEGER | |
| `error_message` | TEXT | |
| `started_at` | TIMESTAMPTZ | |
| `completed_at` | TIMESTAMPTZ | |
| `vm_tracking_id` | UUID | FK to vm_tracking |
| `created_at` | TIMESTAMPTZ DEFAULT now() | |

**Unique constraint:** `(node_name, organisation_name, target_chef_version, cookbook_source)` — latest result per combination, upserted.

### API

| Endpoint | Description |
|---|---|
| `POST /api/v1/kitchen/node-run` | Trigger a Node Kitchen run. Body: `{node_name, organisation, target_chef_version, cookbook_source}` |
| `GET /api/v1/kitchen/node-runs` | List Node Kitchen results. Filter by node, org, status. |
| `GET /api/v1/kitchen/node-runs/:id` | Get detailed result including output. |
| `DELETE /api/v1/kitchen/node-runs/:id` | Delete a result. |

## Batch Definition

### Purpose

Control what runs in bulk Git Kitchen scans. Prevents all-or-nothing execution. Supports gradual ramp-up.

### Batch Model

| Field | Type | Description |
|---|---|---|
| `id` | UUID PK | |
| `name` | TEXT | User-friendly name (e.g. "RHEL 7 first pass") |
| `filters` | JSONB NOT NULL | Selection criteria (see below) |
| `max_count` | INTEGER | Cap on number of cookbooks to include |
| `max_concurrent_vms` | INTEGER | VM concurrency limit for this batch |
| `dry_run` | BOOLEAN DEFAULT false | Preview only, don't execute |
| `status` | TEXT | `draft`, `previewing`, `running`, `completed`, `cancelled` |
| `created_by` | TEXT | |
| `created_at` | TIMESTAMPTZ | |
| `started_at` | TIMESTAMPTZ | |
| `completed_at` | TIMESTAMPTZ | |

### Filter Criteria

All filters are AND-combined:

| Filter | Description | Example |
|---|---|---|
| `cookbook_names` | Explicit list or glob patterns | `["b_win_*", "aett_fx_*"]` |
| `platforms` | Only run cookbooks that test on these platforms | `["rhel7*", "windows2k16"]` |
| `exclude_cookbooks` | Explicit exclusion list | `["legacy_broken_cookbook"]` |
| `has_test_suite` | Only cookbooks with kitchen configs | `true` |
| `previous_status` | Only re-run based on last result | `"failed"`, `"untested"` |
| `target_chef_versions` | Chef versions to test against | `["18.5.0"]` |

### Persistent Cookbook Exclusions

Separate from per-batch filters. A cookbook can be permanently excluded from all bulk scans:

| Column (on git_repos or new table) | Type | Description |
|---|---|---|
| `kitchen_excluded` | BOOLEAN DEFAULT false | Excluded from bulk scans |
| `kitchen_exclude_reason` | TEXT | Why (deprecated, no infra deps, manually validated, etc.) |
| `kitchen_excluded_by` | TEXT | Who excluded it |
| `kitchen_excluded_at` | TIMESTAMPTZ | When |

Exclusions can be overridden on a per-batch basis (`include_excluded: true` in batch filters).

### Dry-Run

When `dry_run = true`:
1. Resolve all filters and exclusions
2. List the cookbooks that would be included, with their platforms and suites
3. Show estimated VM count (cookbooks × platforms × suites)
4. Do not create VMs or run kitchen

### API

| Endpoint | Description |
|---|---|
| `POST /api/v1/kitchen/batches` | Create a batch definition |
| `GET /api/v1/kitchen/batches` | List batches |
| `GET /api/v1/kitchen/batches/:id` | Get batch detail including resolved cookbook list |
| `POST /api/v1/kitchen/batches/:id/run` | Execute (or dry-run) a batch |
| `POST /api/v1/kitchen/batches/:id/cancel` | Cancel a running batch |
| `DELETE /api/v1/kitchen/batches/:id` | Delete a batch definition |

## Git Kitchens (Per-Instance Results)

### Schema Change

Replace the current single-row-per-cookbook model with per-instance results.

New table `git_kitchen_results` (replaces or extends `git_repo_test_kitchen_results`):

| Column | Type | Description |
|---|---|---|
| `id` | UUID PK | |
| `batch_id` | UUID | FK to batch_runs (NULL for ad-hoc) |
| `git_repo_name` | TEXT NOT NULL | |
| `git_repo_url` | TEXT NOT NULL | |
| `target_chef_version` | TEXT NOT NULL | |
| `commit_sha` | TEXT NOT NULL | |
| `platform_name` | TEXT NOT NULL | Kitchen platform name |
| `suite_name` | TEXT NOT NULL | Kitchen suite name |
| `template_used` | TEXT | Hypervisor template used |
| `driver_used` | TEXT | |
| `converge_passed` | BOOLEAN | |
| `tests_passed` | BOOLEAN | |
| `timed_out` | BOOLEAN DEFAULT false | |
| `converge_output` | TEXT | |
| `verify_output` | TEXT | |
| `destroy_output` | TEXT | |
| `duration_seconds` | INTEGER | |
| `error_message` | TEXT | |
| `started_at` | TIMESTAMPTZ | |
| `completed_at` | TIMESTAMPTZ | |
| `vm_tracking_id` | UUID | FK to vm_tracking |
| `created_at` | TIMESTAMPTZ DEFAULT now() | |

**Unique constraint:** `(git_repo_name, git_repo_url, target_chef_version, platform_name, suite_name)` — upsert latest result per instance.

### `.kitchen.local.yml` Conflict Handling

If a cookbook already has a `.kitchen.local.yml`:

1. **Read it** — parse and record its contents (Kitchen Analyser already does this).
2. **Back it up** — rename to `.kitchen.local.yml.bak` before generating CMM's overlay.
3. **Merge selectively** — if the local override contains keys CMM doesn't touch (e.g. custom `attributes`, `data_bags_path`), include them in CMM's generated overlay.
4. **Restore** — after the run, restore the original `.kitchen.local.yml`.
5. **Log a warning** — always log when a local override is displaced, with details of what was in it.

### Chef Version Override

For non-dokken drivers where the base template is a clean OS (no Chef baked in):

```
provisioner:
  require_chef_omnibus: true
  product_name: chef
  product_version: 18.5.0    # target Chef version
```

This overrides the `require_chef_omnibus: false` found in most existing kitchen configs.

For Chef >= 19 (`chef_ice`):
```
provisioner:
  require_chef_omnibus: true  
  product_name: chef_ice
  product_version: 19.0.0
```

## Hypervisor Template Discovery

### vCenter

Query the vCenter API for VM templates in the configured datacenter/folder. Return:
- Template name
- Guest OS type (from VMware's guest ID)
- Annotation/notes
- Last modified date

### Proxmox (Proof-of-Concept Only)

Minimal implementation to validate the hypervisor interface. Query Proxmox API for templates (VMs with `template: 1` flag) and VM inventory. No polish, no edge cases — just enough to prove the abstraction before building the production vCenter backend.

### API

| Endpoint | Description |
|---|---|
| `GET /api/v1/hypervisor/templates` | List available templates from the configured hypervisor |
| `GET /api/v1/hypervisor/vms` | List CMM-managed VMs (from VM tracking table, enriched with live hypervisor state) |
| `POST /api/v1/hypervisor/vms/:id/destroy` | Force-destroy an orphaned VM |
| `POST /api/v1/hypervisor/cleanup` | Destroy all orphaned VMs |

## Configuration Additions

Under `analysis_tools.test_kitchen`:

| Key | Type | Default | Description |
|---|---|---|---|
| `hypervisor_type` | string | `""` | `vcenter`, `proxmox`, or empty (auto-detect from driver) |
| `vm_ttl_hours` | integer | 4 | Maximum VM lifetime before flagging as orphan |
| `vm_name_prefix` | string | `cmm` | Prefix for VM naming convention |
| `max_concurrent_vms` | integer | 10 | Global ceiling on concurrent VMs across all batches |

Under `analysis_tools.test_kitchen.node_kitchens`:

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | boolean | true | Enable Node Kitchen feature |
| `cookbook_fetch_concurrency` | integer | 4 | Concurrent cookbook downloads during assembly |
| `include_node_attributes` | boolean | false | Include node's custom attributes in the kitchen run |
| `default_cookbook_source` | string | `hybrid` | Default cookbook source mode |

## Dashboard Impact

### Git Kitchen Results

- Replace single pass/fail per cookbook with **expandable per-instance breakdown**.
- Platform/suite matrix view: rows = cookbooks, columns = platforms. Cells show pass/fail/untested.
- Batch view: group results by batch run with aggregate stats.
- Filter by: platform, status, batch, cookbook pattern.

### Node Kitchen Results

- Node detail page gains a "Kitchen Test" section.
- Trigger button with cookbook source selector and target Chef version.
- Results show: run_list used, cookbook versions, converge pass/fail, output (expandable).
- History of previous runs for this node.

### Hypervisor Status

- Admin page: running VMs, orphaned VMs, available templates.
- Quick action buttons: destroy orphan, destroy all orphans, refresh template list.

## Migration Path

### From Current Schema

The existing `git_repo_test_kitchen_results` table is retained for backward compatibility during migration. The new `git_kitchen_results` table runs alongside it. Once the new system is validated:

1. Stop writing to the old table.
2. Migrate any useful historical data (one row per cookbook → one row per cookbook with `platform_name = 'unknown'`, `suite_name = 'default'`).
3. Drop the old table in a later migration.

### From Current Execution Model

The existing `KitchenScanner.TestGitRepos()` continues to work for dokken-based testing. The new batch-controlled execution is a separate code path. The old path is deprecated once the new system is validated at scale.

## Related Specifications

- `kitchen-analyser.md` — Discovery component (prerequisite)
- `test-kitchen-drivers.md` — Driver profiles, credential model, overlay generation (still valid, extended)
- `test-kitchen-config-ui.md` — Admin UI (extended with analyser + template discovery)
- `node-kitchen-archive.md` — Predecessor concept (superseded by Node Kitchens live execution)
- `test-kitchen-mvp.md` — Previous validation plan (superseded)
- `analysis.md` — Analysis pipeline integration
- `datastore.md` — Database conventions
- `configuration.md` — Config schema conventions