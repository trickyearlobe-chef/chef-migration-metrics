# Analysis Component - Specification

> **Implementation language:** Go. See `../../Claude.md` for language and concurrency rules.

## TL;DR

This spec covers five areas: **(1) Cookbook usage analysis** — which cookbooks/versions are in use, by which nodes, roles, and policies. **(2) Cookbook compatibility testing** — Test Kitchen (git-sourced cookbooks) and CookStyle linting (server-sourced cookbooks) against target Chef Client versions, with version-specific cop profiles. **(3) Remediation guidance** — auto-correct previews (diff generation), migration doc links per deprecation cop, and cookbook complexity scoring (weighted score + blast radius). **(4) Node upgrade readiness** — per-node pass/fail per target version based on cookbook compatibility and disk space, with stale-node handling. **(5) Embedded tool resolution** — CookStyle/Test Kitchen/Ruby looked up in `analysis_tools.embedded_bin_dir` first, then `PATH`. All work is parallelised via bounded worker pools (see configuration spec).

---

## Overview

The analysis component processes data collected from Chef Infra Servers and git repositories to produce the metrics that drive the dashboard and upgrade readiness assessments. In addition to detecting compatibility issues, it provides **remediation guidance** — actionable information that helps practitioners fix problems, not just find them.

---

## Responsibilities

- Determine which cookbooks are in active use across the fleet
- Assess cookbook compatibility with target Chef Client versions
- Compute node upgrade readiness
- Generate remediation guidance for incompatible cookbooks (auto-correct previews, migration documentation links, complexity scores)
- Compute cookbook complexity scores to help teams prioritise remediation effort

---

## Sub-Components

### 1. Cookbook Usage Analysis

Derives cookbook usage statistics from collected node data.

From each node's `automatic.cookbooks` attribute (the resolved, deduplicated cookbook map), determine:

- Which cookbooks are in active use across the fleet, and which exist on the Chef server but are applied to no nodes
- Which specific versions of those cookbooks are in use
- Which roles reference those cookbooks
- Which Policyfile policy names and policy groups reference those cookbooks
- Which nodes are running each cookbook and version
- How many nodes are running each cookbook and version
- How many platform versions and platform families are running each cookbook and version
- Whether each cookbook version is **actively used** or **unused** — stored as a flag per cookbook version to support dashboard filtering

#### Concurrency

- Cookbook usage statistics must be computed by fanning out over the collected node records using goroutines, bounded by the `concurrency.readiness_evaluation` worker pool setting (see [Configuration Specification](../configuration/Specification.md)).
- Aggregation of results (node counts, platform counts, active/unused flags) must be performed safely using channels or mutex-protected accumulators.

#### Design: Computation Steps

The usage analysis runs after each node collection cycle completes and proceeds in three phases:

**Phase 1 — Per-node extraction (parallel)**

For each collected node, extract the following tuples from the `automatic.cookbooks` map:

- `(organisation, cookbook_name, cookbook_version, node_name, platform, platform_version, platform_family, chef_environment, roles[], policy_name, policy_group)`

For Policyfile nodes (where `policy_name` and `policy_group` are non-null), `roles` may be empty — the policy name and policy group serve as the primary grouping dimensions instead.

Fan out across nodes using a worker pool bounded by `concurrency.readiness_evaluation`. Each goroutine sends its extracted tuples to a shared results channel.

**Phase 2 — Aggregation (single goroutine)**

A dedicated aggregation goroutine reads from the results channel and builds the following in-memory maps:

| Map | Key | Value |
|-----|-----|-------|
| Node count per cookbook version | `(org, cookbook, version)` | count of distinct nodes |
| Nodes per cookbook version | `(org, cookbook, version)` | set of node names |
| Roles per cookbook | `(org, cookbook)` | set of role names (from each node's `roles` attribute where that cookbook appears) |
| Policy names per cookbook | `(org, cookbook)` | set of policy names (from Policyfile nodes where that cookbook appears) |
| Policy groups per cookbook | `(org, cookbook)` | set of policy groups (from Policyfile nodes where that cookbook appears) |
| Platform count per cookbook version | `(org, cookbook, version, platform, platform_version)` | count of nodes |
| Platform family count per cookbook version | `(org, cookbook, version, platform_family)` | count of nodes |

**Phase 3 — Active/unused flagging**

After aggregation, compare the set of cookbook versions observed across all nodes against the full cookbook version inventory fetched from the Chef server (via `GET /organizations/<ORG>/cookbooks?num_versions=all`). Any cookbook version present on the server but absent from the aggregated node data is flagged as **unused**.

**Persistence**

All aggregated results and active/unused flags are written to the datastore in a single transaction at the end of the analysis run. The previous analysis snapshot is retained (not overwritten) to support historical trending.

---

### 2. Cookbook Compatibility Testing

Tests cookbooks against each configured target Chef Client version and records results.

#### Git-sourced Cookbooks

- Tested using both **CookStyle** and **Test Kitchen** against multiple configured target Chef Client versions
- Only the HEAD commit of the default branch (`main` or `master`) is tested
- The HEAD commit SHA is recorded with each test result (both CookStyle and Test Kitchen)
- A CookStyle scan is skipped if the HEAD commit SHA is unchanged since the last scan for the same cookbook + target Chef Client version. When the HEAD commit changes, the cookbook is rescanned and the previous result is overwritten.
- A Test Kitchen run is skipped if the HEAD commit SHA is unchanged since the last run for the same cookbook + target Chef Client version
- Both Test Kitchen pass criteria must be met for a cookbook to be considered fully compatible:
  1. The cookbook **converges** successfully
  2. The cookbook's **tests pass**
- CookStyle results provide deprecation detection and remediation guidance (auto-correct previews, migration doc links) independently of Test Kitchen pass/fail

#### Chef Server-sourced Cookbooks

- No test suite is available; tested with **CookStyle** only for linting and deprecation warnings
- Cookbook versions are immutable on the Chef server — CookStyle scanning runs once per organisation + cookbook name + version
- Subsequent collection runs skip versions already scanned
- A manual rescan option must be provided

#### CookStyle Version Profiles

Rather than running CookStyle with its full default rule set, the analysis component should enable only the cops relevant to the **specific target Chef Client versions** being tested. CookStyle organises its cops into version-specific channels that correspond to deprecations introduced in each Chef release.

- For each configured target Chef Client version, determine the applicable set of CookStyle cops. CookStyle cops in the `ChefDeprecations` namespace include comments indicating which Chef version introduced the deprecation.
- When scanning a cookbook for a specific target version, enable only the cops that are relevant to that version and earlier. This prevents false positives from cops that flag deprecations not yet applicable to the target version.
- If the target version cannot be mapped to a specific CookStyle profile (e.g. because the version is very new), fall back to running the full `ChefDeprecations` and `ChefCorrectness` namespaces.
- The CookStyle profile mapping is maintained as a configuration data structure within the application, updated when new Chef Client versions are released.

#### Concurrency

- Test Kitchen runs are independent per cookbook + target Chef Client version combination. Each run must be dispatched as a goroutine, bounded by the `concurrency.test_kitchen_run` worker pool setting (see [Configuration Specification](../configuration/Specification.md)).
- CookStyle scans are independent per cookbook version. Scans must run in parallel using goroutines, bounded by the `concurrency.cookstyle_scan` worker pool setting (see [Configuration Specification](../configuration/Specification.md)).
- Each goroutine must capture stdout/stderr from the external process and return it alongside the pass/fail result to the coordinator. Errors must not be silently discarded.

---

#### Design: Test Kitchen Invocation

Test Kitchen is invoked as an external process. The application does **not** link against Test Kitchen as a library — it shells out to the `kitchen` CLI.

> **JSON output policy:** Where possible, external tools run by batch processes should emit JSON for easy parsing and ingestion. Test Kitchen's `list` and `diagnose` subcommands support `--format json`; these must always be invoked with that flag. The action subcommands (`converge`, `verify`, `destroy`) do **not** support a JSON formatter — they produce free-form log output. For these commands, the application captures stdout/stderr as opaque text, relying on the exit code for pass/fail determination and storing the raw output for troubleshooting.

**Embedded Tools**

Test Kitchen, the `kitchen-dokken` driver, and a self-contained Ruby runtime are **embedded** in all packaging formats under `/opt/chef-migration-metrics/embedded/`. The application resolves the `kitchen` binary from this embedded directory by default (configurable via the `embedded_bin_dir` setting — see [Configuration Specification](../configuration/Specification.md)).

The only external prerequisite is a container runtime:

- **Docker** (required by the `kitchen-dokken` driver for container-based testing)

Docker must be installed and accessible to the user running Chef Migration Metrics. The embedded `kitchen-dokken` driver creates and destroys containers for each test run — no Vagrant or VM infrastructure is needed.

> **Note:** The embedded Ruby environment is fully self-contained and does not interfere with any system Ruby, Chef Workstation, or other gem installation on the host. See the [Packaging Specification](../packaging/Specification.md) for details on how the embedded environment is built and laid out.

**Invocation sequence per cookbook + target Chef Client version**

1. **Check skip condition** — Query the datastore for the most recent test result for this cookbook + target Chef Client version. If it exists and its `commit_sha` matches the current HEAD commit SHA recorded by the data collection component, skip the test run and log at `INFO` severity.

2. **Prepare workspace** — The git repository has already been cloned/pulled by the data collection component. The analysis component operates directly on the local clone directory. No additional file copying is needed.

3. **Generate environment overlay** — Create a temporary `.kitchen.local.yml` file in the cookbook directory that overrides the Chef Client version provisioner attribute. The overlay forces the target Chef Client version regardless of what the cookbook's `.kitchen.yml` specifies:

   ```yaml
   # .kitchen.local.yml — generated by chef-migration-metrics
   # DO NOT EDIT — this file is overwritten on each test run
   provisioner:
     product_version: "<TARGET_CHEF_VERSION>"
   ```

   For `kitchen-dokken`, the overlay also sets the Chef Docker image:

   ```yaml
   provisioner:
     chef_version: "<TARGET_CHEF_VERSION>"
   ```

   The overlay file format (standard provisioner vs. dokken) is determined by inspecting the cookbook's `.kitchen.yml` to identify the driver in use.

4. **Discover instances** — Before running converge, enumerate the available Test Kitchen instances to validate the workspace:

   ```
   kitchen list --format json
   ```

   The `--format json` flag produces machine-parseable output — an array of objects with keys such as `instance`, `driver`, `provisioner`, `verifier`, `transport`, and `last_action`. Parse this to confirm at least one instance is defined and to log the instance names. If the command fails or returns an empty list, log at `ERROR` severity and skip the test run.

5. **Run converge** — Execute:

   ```
   kitchen converge --concurrency=1 --log-level=info
   ```

   `--concurrency=1` is set because parallelism is managed at the application level (across cookbooks), not within Test Kitchen. This prevents resource contention.

   Capture combined stdout/stderr into a buffer. Set a configurable timeout (default: 30 minutes). If the process times out, record the result as a failure with a `timeout` flag.

   Record the exit code:
   - Exit 0 → converge passed
   - Non-zero → converge failed

6. **Run verify** (only if converge passed) — Execute:

   ```
   kitchen verify --concurrency=1 --log-level=info
   ```

   Capture combined stdout/stderr. Same timeout and exit code handling as converge.

7. **Run destroy** (always, regardless of pass/fail) — Execute:

   ```
   kitchen destroy --concurrency=1
   ```

   This cleans up instances. Failure to destroy is logged at `WARN` but does not affect the test result.

8. **Clean up overlay** — Remove the generated `.kitchen.local.yml` file.

9. **Persist result** — Write the test result to the datastore:

   | Field | Value |
   |-------|-------|
   | `cookbook_name` | Name of the cookbook |
   | `commit_sha` | HEAD commit SHA at time of test |
   | `target_chef_version` | The target Chef Client version tested |
   | `converge_passed` | Boolean |
   | `verify_passed` | Boolean |
   | `timed_out` | Boolean |
   | `converge_output` | Captured stdout/stderr from converge |
   | `verify_output` | Captured stdout/stderr from verify |
   | `destroy_output` | Captured stdout/stderr from destroy |
   | `duration_seconds` | Wall-clock time for the full run |
   | `tested_at` | UTC timestamp |

10. **Log result** — Log the outcome at `INFO` (pass) or `ERROR` (fail) severity, including the cookbook name, target version, commit SHA, and duration. The full process output is stored in the `process_output` field of the log entry.

**Error handling**

- If the cookbook directory does not contain a `.kitchen.yml`, the cookbook is logged as `not testable` at `WARN` severity and skipped. No result is recorded.
- If Test Kitchen is not installed, the application must fail at startup (see Startup Validation).
- If an individual test run fails, the error is captured and persisted. It does not cancel other test runs.

---

#### Design: CookStyle Invocation

CookStyle is invoked as an external process against cookbooks downloaded from the Chef server.

> **JSON output policy:** CookStyle supports `--format json` which produces machine-parseable RuboCop JSON output. All CookStyle invocations (initial scan and auto-correct preview) **must** use `--format json`. Never parse CookStyle's human-readable text output.

**Embedded Tools**

CookStyle and its Ruby runtime are **embedded** in all packaging formats under `/opt/chef-migration-metrics/embedded/`. The application resolves the `cookstyle` binary from this embedded directory by default (configurable via the `embedded_bin_dir` setting — see [Configuration Specification](../configuration/Specification.md)).

No additional installation or configuration is required. CookStyle has no external runtime dependencies beyond the embedded Ruby environment.

**Invocation sequence per organisation + cookbook name + version**

1. **Check skip condition** — Query the datastore for an existing CookStyle result for this organisation + cookbook name + version. If one exists and no manual rescan has been requested, skip the scan and log at `DEBUG` severity.

2. **Prepare workspace** — The cookbook files have been downloaded by the data collection component into a directory keyed by organisation + cookbook name + version. The analysis component operates directly on this directory.

3. **Run CookStyle scan** — Execute:

   ```
   cookstyle --format json <COOKBOOK_DIRECTORY>
   ```

   The `--format json` flag produces machine-parseable output. Capture combined stdout/stderr. Set a configurable timeout (default: 10 minutes).

4. **Parse JSON output** — The CookStyle JSON output follows the RuboCop JSON formatter structure:

   ```json
   {
     "metadata": {
       "rubocop_version": "...",
       "ruby_engine": "...",
       "ruby_version": "..."
     },
     "files": [
       {
         "path": "recipes/default.rb",
         "offenses": [
           {
             "severity": "convention|warning|error|fatal",
             "message": "...",
             "cop_name": "ChefDeprecations/ResourceWithoutUnifiedTrue",
             "corrected": false,
             "location": {
               "start_line": 10,
               "start_column": 1,
               "last_line": 10,
               "last_column": 30
             }
           }
         ]
       }
     ],
     "summary": {
       "offense_count": 3,
       "target_file_count": 5,
       "inspected_file_count": 5
     }
   }
   ```

   Extract the following from the parsed output:

   | Field | Source |
   |-------|--------|
   | Total offense count | `summary.offense_count` |
   | Deprecation warnings | Offenses where `cop_name` starts with `ChefDeprecations/` |
   | Correctness errors | Offenses where `cop_name` starts with `ChefCorrectness/` |
   | All offenses (full list) | `files[*].offenses[*]` |
   | Pass/fail | Pass if zero offenses with severity `error` or `fatal` |

5. **Persist result** — Write the CookStyle result to the datastore:

   | Field | Value |
   |-------|-------|
   | `organisation` | Chef server organisation name |
   | `cookbook_name` | Cookbook name |
   | `cookbook_version` | Cookbook version |
   | `passed` | Boolean — true if no `error` or `fatal` severity offenses |
   | `offense_count` | Total number of offenses |
   | `deprecation_count` | Number of `ChefDeprecations/*` offenses |
   | `correctness_count` | Number of `ChefCorrectness/*` offenses |
   | `offenses_json` | Full offense list as JSON (for detail display in dashboard) |
   | `raw_output` | Raw stdout/stderr from the CookStyle process |
   | `scanned_at` | UTC timestamp |

6. **Log result** — Log the outcome at `INFO` (pass) or `WARN` (fail with warnings only) or `ERROR` (fail with errors) severity. Include organisation, cookbook name, version, offense count, and deprecation count.

**Deprecation detection**

CookStyle cops in the `ChefDeprecations` namespace directly correspond to features removed or changed in newer Chef Client versions. The dashboard must display these prominently as they are the primary signal that a cookbook will fail against a target Chef Client version.

The following cop namespaces are tracked:

| Namespace | Relevance |
|-----------|-----------|
| `ChefDeprecations/*` | Features removed in newer Chef versions — high relevance to migration |
| `ChefCorrectness/*` | Incorrect usage that may cause runtime failures |
| `ChefStyle/*` | Style issues — low relevance, displayed but do not affect pass/fail |
| `ChefModernize/*` | Modernisation suggestions — informational, do not affect pass/fail |
| Other | Inherited RuboCop cops — informational |

**Error handling**

- If `cookstyle` exits with a non-zero code but produces valid JSON output, the result is still parsed and recorded. CookStyle exits non-zero when offenses are found — this is normal.
- If `cookstyle` exits with a non-zero code and produces no valid JSON output (e.g. crash), log at `ERROR` severity with the raw output and record the scan as failed.
- If the cookbook directory is empty or missing, log at `ERROR` and skip.

---

### 4. Remediation Guidance

After compatibility testing and CookStyle scanning, the analysis component generates actionable remediation guidance for each incompatible cookbook. This transforms the tool from a reporting dashboard into a migration management platform that helps practitioners **fix** problems, not just find them.

#### 4.1 Auto-Correct Preview

CookStyle supports `--auto-correct` mode which can automatically fix many deprecation and style offenses. The analysis component must generate a preview of what auto-correct would change without actually modifying cookbook files.

**Invocation per cookbook version with CookStyle offenses:**

1. **Copy workspace** — Create a temporary copy of the cookbook directory to avoid modifying the original files.

2. **Run auto-correct** — Execute:

   ```
   cookstyle --auto-correct --format json <TEMP_COOKBOOK_DIRECTORY>
   ```

3. **Generate diff** — Compare the original and auto-corrected files using a diff algorithm. Produce a unified diff for each modified file.

4. **Compute auto-correct statistics:**

   | Metric | Description |
   |--------|-------------|
   | `total_offenses` | Total offenses before auto-correct |
   | `correctable_offenses` | Offenses that auto-correct can fix |
   | `remaining_offenses` | Offenses that require manual intervention |
   | `files_modified` | Number of files that would be changed |

5. **Persist result** — Store the auto-correct preview in the datastore:

   | Field | Value |
   |-------|-------|
   | `cookbook_id` | FK to the cookbook |
   | `total_offenses` | Total offense count |
   | `correctable_offenses` | Auto-correctable offense count |
   | `remaining_offenses` | Offenses requiring manual fix |
   | `files_modified` | Number of files changed |
   | `diff_output` | Unified diff of all changes |
   | `generated_at` | UTC timestamp |

6. **Clean up** — Remove the temporary copy.

**Concurrency:** Auto-correct previews run as part of the CookStyle scan pipeline. Each preview is generated immediately after the initial scan for cookbooks with offenses. No additional worker pool is needed — the `concurrency.cookstyle_scan` pool bounds both the scan and the preview.

**Skip condition:** Auto-correct previews are only generated when offenses are found. If a cookbook passes CookStyle with zero offenses, no preview is generated. Like CookStyle results, previews for Chef server-sourced cookbooks are generated once per immutable version and cached.

#### 4.2 Migration Documentation Links

Each CookStyle cop in the `ChefDeprecations` namespace corresponds to a specific Chef feature that was deprecated or removed. The analysis component must map each deprecation cop to its migration documentation.

**Cop-to-documentation mapping:**

- Maintain a built-in mapping table of `cop_name → { description, migration_url, introduced_in, removed_in }`.
- The mapping covers all `ChefDeprecations/*` and `ChefCorrectness/*` cops.
- Each entry includes:
  - `cop_name` — e.g. `ChefDeprecations/ResourceWithoutUnifiedTrue`
  - `description` — Human-readable explanation of the deprecation and what to change
  - `migration_url` — URL to the relevant Chef migration documentation (e.g. `https://docs.chef.io/deprecations_...`)
  - `introduced_in` — The Chef Client version where the deprecation warning was first emitted
  - `removed_in` — The Chef Client version where the deprecated feature was removed (if known)
  - `replacement_pattern` — A brief code example showing the old pattern and the new pattern

- When CookStyle results are persisted, each offense is enriched with the corresponding migration documentation from the mapping table.
- The mapping table is shipped as embedded data in the application binary and can be updated by releasing a new application version.

**Example enriched offense:**

```json
{
  "cop_name": "ChefDeprecations/ResourceWithoutUnifiedTrue",
  "severity": "warning",
  "message": "Set unified_mode true in Chef Infra Client 15.3+",
  "location": { "start_line": 10, "start_column": 1 },
  "remediation": {
    "description": "Custom resources should enable unified mode for compatibility with Chef 18+.",
    "migration_url": "https://docs.chef.io/unified_mode/",
    "introduced_in": "15.3",
    "removed_in": null,
    "replacement_pattern": "# Before:\nresource_name :my_resource\n\n# After:\nresource_name :my_resource\nunified_mode true"
  }
}
```

#### 4.3 Cookbook Complexity Scoring

Each cookbook is assigned a **complexity score** that estimates the relative effort required to make it compatible with a target Chef Client version. This helps teams prioritise which cookbooks to fix first.

**Scoring model:**

The complexity score is computed per cookbook per target Chef Client version as a weighted sum:

| Factor | Weight | Source |
|--------|--------|--------|
| Total CookStyle offenses with severity `error` or `fatal` | 5 per offense | CookStyle results |
| Total `ChefDeprecations/*` offenses | 3 per offense | CookStyle results |
| Total `ChefCorrectness/*` offenses | 3 per offense | CookStyle results |
| Remaining offenses after auto-correct (not auto-correctable) | 4 per offense | Auto-correct preview |
| Total `ChefModernize/*` offenses | 1 per offense | CookStyle results |
| Test Kitchen converge failure (if applicable) | 20 flat | Test Kitchen results |
| Test Kitchen test failure (converge passed but tests failed) | 10 flat | Test Kitchen results |

**Score interpretation:**

| Score Range | Label | Meaning |
|-------------|-------|---------|
| 0 | `none` | No remediation needed — cookbook is compatible |
| 1–10 | `low` | Minor issues, likely fixable with auto-correct alone |
| 11–30 | `medium` | Moderate issues, some manual intervention required |
| 31–60 | `high` | Significant issues, requires dedicated development effort |
| 61+ | `critical` | Major rewrite likely needed |

**Blast radius:**

In addition to the per-cookbook complexity score, compute a **blast radius** metric:

- `affected_node_count` — Number of nodes running this cookbook
- `affected_role_count` — Number of roles that include this cookbook (directly or transitively via the dependency graph)
- `affected_policy_count` — Number of Policyfile policy names that include this cookbook

The blast radius helps teams understand the impact of fixing (or not fixing) a given cookbook. A cookbook with a low complexity score but high blast radius should be prioritised because fixing it unblocks many nodes.

**Persistence:**

Write one complexity record per cookbook per target Chef Client version:

| Field | Value |
|-------|-------|
| `cookbook_id` | FK to the cookbook |
| `target_chef_version` | Target Chef Client version |
| `complexity_score` | Numeric score |
| `complexity_label` | One of: `none`, `low`, `medium`, `high`, `critical` |
| `error_count` | Count of error/fatal offenses |
| `deprecation_count` | Count of ChefDeprecations offenses |
| `correctness_count` | Count of ChefCorrectness offenses |
| `modernize_count` | Count of ChefModernize offenses |
| `auto_correctable_count` | Count of offenses fixable by auto-correct |
| `manual_fix_count` | Count of offenses requiring manual intervention |
| `affected_node_count` | Blast radius — nodes |
| `affected_role_count` | Blast radius — roles |
| `affected_policy_count` | Blast radius — policy names |
| `evaluated_at` | UTC timestamp |

**Scheduling:** Complexity scores are recomputed after every CookStyle scan and Test Kitchen run cycle completes, using the latest results.

---

### 5. Node Upgrade Readiness

Computes a readiness status per node per target Chef Client version.

A node is considered **ready** when ALL of the following are true:

1. All cookbooks in the node's expanded run-list are compatible with the target Chef Client version (passing Test Kitchen results)
2. Sufficient disk space is available on the node to install the Habitat-packaged Chef Client bundle (including bundled InSpec)

Blocking reasons must be recorded per node (e.g. specific incompatible cookbooks, insufficient disk space) for display in the dashboard.

Readiness status is computed and persisted after each collection and testing cycle.

#### Concurrency

- Readiness computation is independent per node per target Chef Client version. Nodes must be evaluated in parallel using goroutines, bounded by the `concurrency.readiness_evaluation` worker pool setting (see [Configuration Specification](../configuration/Specification.md)).

---

#### Design: Disk Space Evaluation

The `automatic.filesystem` attribute collected from each node contains a map of mounted filesystems with size and availability information. The structure varies by platform:

**Linux nodes:**

```json
{
  "filesystem": {
    "/dev/sda1": {
      "kb_size": "20511356",
      "kb_used": "5123456",
      "kb_available": "14340800",
      "percent_used": "26%",
      "mount": "/"
    },
    "/dev/sdb1": {
      "kb_size": "102400000",
      "kb_used": "50000000",
      "kb_available": "47360000",
      "percent_used": "51%",
      "mount": "/opt"
    }
  }
}
```

**Windows nodes:**

```json
{
  "filesystem": {
    "C:": {
      "kb_size": "104857600",
      "kb_used": "52428800",
      "kb_available": "52428800",
      "percent_used": "50%"
    }
  }
}
```

**Evaluation algorithm:**

1. **Determine the installation target mount point.** The Habitat-packaged Chef Client is installed to:
   - Linux: `/hab` (falls back to `/` if `/hab` is not a separate mount)
   - Windows: `C:\hab` (falls back to `C:` if not a separate mount)

2. **Find the matching filesystem entry.** Iterate through the `filesystem` map and find the entry whose `mount` value is the longest prefix match for the installation path. For example:
   - If `/hab` is a mount point, use that entry
   - If `/hab` is not mounted separately but `/` is, use `/`
   - If `/opt` is mounted and Habitat is configured to install under `/opt/hab`, use `/opt`

   For Windows nodes, match on the drive letter.

3. **Extract available space.** Read the `kb_available` field from the matched filesystem entry. Convert from KB to MB by dividing by 1024.

4. **Compare against threshold.** Compare the available MB against the configured `readiness.min_free_disk_mb` value (default: 2048 MB). If available space is less than the threshold, the node is blocked for this reason.

**Edge cases:**

- If the `filesystem` attribute is missing or empty (e.g. the node has not completed a recent Chef run), the disk space check is recorded as **unknown** rather than pass or fail. The dashboard must display this as a distinct state.
- If `kb_available` is missing from a filesystem entry, treat that filesystem as having 0 KB available.
- Values in the `filesystem` map are strings in some Chef Client versions and integers in others. The implementation must handle both.

---

#### Design: Stale Node Handling

Nodes flagged as **stale** by the data collection component (see [Data Collection Specification](../data-collection/Specification.md)) require special handling during readiness evaluation:

- Stale nodes are still evaluated for readiness, but their disk space data is treated as **unknown** (same as missing filesystem data) since the data may be outdated.
- The readiness result for stale nodes includes an additional `stale_data` flag set to `true`.
- The dashboard must surface stale nodes distinctly so that operators can prioritise getting these nodes to check in before attempting an upgrade.

---

#### Design: Cookbook Compatibility Evaluation

For each node, the readiness evaluator must determine whether **all** cookbooks in the node's resolved cookbook list are compatible with the target Chef Client version.

**Algorithm per node per target Chef Client version:**

1. **Get the node's cookbook list.** Read the `automatic.cookbooks` attribute, which is a map of `cookbook_name → { version, ... }`.

2. **For each cookbook + version in the map:**

   a. **Check for a Test Kitchen result.** Query the datastore for the most recent test result where:
      - `cookbook_name` matches
      - `target_chef_version` matches
      - `converge_passed = true` AND `verify_passed = true`

      If a passing result exists, the cookbook is **compatible**.

   b. **If no Test Kitchen result exists, check for a CookStyle result.** Query the datastore for a CookStyle result where:
      - `organisation` matches the node's organisation
      - `cookbook_name` matches
      - `cookbook_version` matches

      CookStyle results are treated as follows:
      - `passed = true` (no error/fatal offenses) → **compatible (CookStyle only)** — the cookbook has no detected errors but has not been integration-tested
      - `passed = false` → **incompatible (CookStyle)** — the cookbook has errors that likely indicate incompatibility
      - No CookStyle result exists → **untested**

   c. **Classify the cookbook:**

      | Status | Meaning | Blocks readiness? |
      |--------|---------|-------------------|
      | `compatible` | Test Kitchen pass | No |
      | `compatible_cookstyle_only` | CookStyle pass, no Test Kitchen | No |
      | `incompatible` | Test Kitchen fail or CookStyle error/fatal | **Yes** |
      | `untested` | No test or scan results | **Yes** |

3. **Aggregate blocking reasons.** Collect the list of cookbooks that are `incompatible` or `untested`. Each entry in the blocking list records:
   - Cookbook name and version
   - Reason (`incompatible` or `untested`)
   - Source (`test_kitchen` or `cookstyle` or `none`)
   - Complexity score and label (from the remediation guidance, if available)

4. **Combine with disk space result.** The node is **ready** only if:
   - The cookbook blocking list is empty, AND
   - Disk space is sufficient (or unknown — see note below)

   > **Note on unknown disk space:** Nodes with unknown disk space status are classified as **blocked (unknown disk space)** to err on the side of caution. The dashboard must surface these nodes distinctly so that operators can investigate.

**Persistence:**

Write one readiness record per node per target Chef Client version:

| Field | Value |
|-------|-------|
| `organisation` | Chef server organisation name |
| `node_name` | Node name |
| `target_chef_version` | Target Chef Client version |
| `ready` | Boolean |
| `disk_space_available_mb` | Available MB on the installation mount (null if unknown) |
| `disk_space_sufficient` | Boolean or null (unknown) |
| `blocking_cookbooks` | JSON array of `{ name, version, reason, source }` |
| `evaluated_at` | UTC timestamp |

---

**Persistence (updated for stale data):**

Write one readiness record per node per target Chef Client version:

| Field | Value |
|-------|-------|
| `organisation` | Chef server organisation name |
| `node_name` | Node name |
| `target_chef_version` | Target Chef Client version |
| `ready` | Boolean |
| `disk_space_available_mb` | Available MB on the installation mount (null if unknown) |
| `disk_space_sufficient` | Boolean or null (unknown) |
| `blocking_cookbooks` | JSON array of `{ name, version, reason, source, complexity_score, complexity_label }` |
| `stale_data` | Boolean — true if the node's last check-in exceeds the stale threshold |
| `evaluated_at` | UTC timestamp |

---

## Startup Validation

The analysis component must validate that required tools are available before accepting work. The following checks run at application startup. For `kitchen` and `cookstyle`, the application first looks in the configured `embedded_bin_dir` (default: `/opt/chef-migration-metrics/embedded/bin/`), then falls back to `PATH` lookup.

> **JSON output policy:** Where a tool supports JSON output for its version or info command, use it. This makes startup validation output machine-parseable and simplifies version extraction. Where JSON is not available (e.g. `git --version`), parse the single-line text output.

| Tool | Check | Failure behaviour |
|------|-------|-------------------|
| `kitchen` | Run `<embedded_bin_dir>/kitchen version` (or `kitchen version` via PATH fallback) and verify exit code 0. Parse the version string from stdout. | Log `ERROR` and disable Test Kitchen testing. CookStyle-only analysis continues. |
| `cookstyle` | Run `<embedded_bin_dir>/cookstyle --format json --version` (or `cookstyle --format json --version` via PATH fallback) and verify exit code 0. Parse the version from the output. | Log `ERROR` and disable CookStyle scanning. Test Kitchen testing continues. |
| `git` | Run `git version` and verify exit code 0. Parse the version string from stdout (format: `git version X.Y.Z`). | Fatal — the application must refuse to start, as git is required by the data collection component. |
| `docker` | Run `docker info --format json` and verify exit code 0. Parse the JSON output to extract the Docker server version and confirm the daemon is responsive. | Log `WARN` — Test Kitchen with `kitchen-dokken` requires Docker. If unavailable, Test Kitchen testing is disabled. |

If both `kitchen` and `cookstyle` are unavailable, the analysis component logs a `WARN` that no compatibility testing is possible. The application continues to run (data collection and dashboard still function) but all cookbooks will be reported as `untested`.

> **Expected state:** In a standard installation (RPM, DEB, or container image), the embedded `kitchen` and `cookstyle` binaries are always present under `/opt/chef-migration-metrics/embedded/bin/` and startup validation will pass. The fallback to `PATH` lookup is provided for development environments and source builds where the embedded tree may not be present.

---

## Scheduling and Trigger

The analysis component runs **after** the data collection component completes a collection cycle. The trigger sequence is:

1. Data collection completes (node data collected, cookbooks fetched, git repos pulled)
2. Cookbook usage analysis runs (Phase 1–3 as described above)
3. CookStyle scans run for any new/unscanned Chef server cookbook versions
4. Test Kitchen runs execute for any cookbooks where HEAD has changed
5. Node upgrade readiness evaluation runs
6. Metric snapshots are written for historical trending

Steps 3 and 4 may run concurrently since they operate on independent cookbook sets (Chef server-sourced vs. git-sourced).

---

## Data Inputs

| Input | Source |
|-------|--------|
| Node attribute data (cookbooks, platform, disk, run_list, roles, policy_name, policy_group, ohai_time) | Data collection component |
| Git repository HEAD commit SHA and test suite presence | Data collection component |
| Chef server cookbook file manifest | Data collection component |
| Full cookbook inventory from Chef server | Data collection component |
| Role dependency graph | Data collection component |
| Configured target Chef Client versions | Configuration |
| Configured disk space threshold | Configuration |
| Configured stale node threshold | Configuration |
| Cop-to-documentation mapping (embedded) | Application binary |
| CookStyle binary | Embedded Ruby environment (`/opt/chef-migration-metrics/embedded/bin/cookstyle`) |
| Test Kitchen binary | Embedded Ruby environment (`/opt/chef-migration-metrics/embedded/bin/kitchen`) |
| Ruby interpreter | Embedded Ruby environment (`/opt/chef-migration-metrics/embedded/bin/ruby`) |

## Data Outputs

| Output | Consumers |
|--------|-----------|
| Cookbook usage statistics (active/unused flag, node counts, platform counts, policy references) | Datastore → Dashboard |
| Test Kitchen results (converge + test pass/fail, keyed by cookbook + Chef Client version + commit SHA) | Datastore → Dashboard |
| CookStyle results (keyed by org + cookbook name + version) | Datastore → Dashboard |
| Auto-correct previews (diff, correctable/remaining counts) | Datastore → Dashboard |
| Remediation guidance (enriched offenses with migration docs, replacement patterns) | Datastore → Dashboard |
| Cookbook complexity scores and blast radius (per cookbook per target version) | Datastore → Dashboard |
| Node readiness status, blocking reasons, and stale data flags (per node per target version) | Datastore → Dashboard |

---

## Related Specifications

- [`../Specification.md`](../Specification.md) — top-level project specification
- [`../data-collection/Specification.md`](../data-collection/Specification.md) — data collection component
- [`../visualisation/Specification.md`](../visualisation/Specification.md) — dashboard and log viewer
- [`../chef-api/Specification.md`](../chef-api/Specification.md) — Chef Infra Server API reference
- [`../datastore/Specification.md`](../datastore/Specification.md) — database schema
- [`../configuration/Specification.md`](../configuration/Specification.md) — configuration schema (includes `embedded_bin_dir` setting)
- [`../logging/Specification.md`](../logging/Specification.md) — logging subsystem
- [`../packaging/Specification.md`](../packaging/Specification.md) — embedded Ruby environment build and layout