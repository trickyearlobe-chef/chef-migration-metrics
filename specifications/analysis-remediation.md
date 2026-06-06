# Analysis — Remediation Guidance

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
