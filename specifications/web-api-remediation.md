# Web API — Remediation Endpoints

## Remediation Endpoints

### Get Server Cookbook Remediation Detail

#### `GET /api/v1/cookbooks/:name/:version/remediation`

Returns the full remediation guidance for a specific server cookbook version, including auto-correct preview, enriched deprecation offenses with migration documentation, and complexity score. This endpoint serves **server cookbooks only**. For git repo remediation, see `GET /api/v1/git-repos/:name/:version/remediation`.

**Query parameters:** `organisation` (optional, scopes to a specific organisation's copy), `target_chef_version` (optional, defaults to all configured targets).

**Response (200):**

```json
{
  "cookbook_name": "legacy-app",
  "cookbook_version": "2.0.0",
  "organisation": "myorg-production",
  "complexity": {
    "score": 15,
    "label": "medium",
    "error_count": 1,
    "deprecation_count": 3,
    "correctness_count": 0,
    "modernize_count": 2,
    "auto_correctable_count": 4,
    "manual_fix_count": 2,
    "affected_node_count": 50,
    "affected_role_count": 1,
    "affected_policy_count": 0
  },
  "auto_correct_preview": {
    "total_offenses": 6,
    "correctable_offenses": 4,
    "remaining_offenses": 2,
    "files_modified": 3,
    "diff": "--- a/recipes/default.rb\n+++ b/recipes/default.rb\n@@ -10,1 +10,2 @@\n-resource_name :my_resource\n+resource_name :my_resource\n+unified_mode true\n"
  },
  "offenses": [
    {
      "cop_name": "ChefDeprecations/ResourceWithoutUnifiedTrue",
      "severity": "warning",
      "message": "Set unified_mode true in Chef Infra Client 15.3+",
      "file": "resources/my_resource.rb",
      "location": { "start_line": 10, "start_column": 1, "last_line": 10, "last_column": 30 },
      "corrected_by_autocorrect": true,
      "remediation": {
        "description": "Custom resources should enable unified mode for compatibility with Chef 18+.",
        "migration_url": "https://docs.chef.io/unified_mode/",
        "introduced_in": "15.3",
        "removed_in": null,
        "replacement_pattern": "# Before:\nresource_name :my_resource\n\n# After:\nresource_name :my_resource\nunified_mode true"
      }
    },
    {
      "cop_name": "ChefDeprecations/Cheffile",
      "severity": "error",
      "message": "Cheffile is deprecated. Use a Policyfile or Berkshelf instead.",
      "file": "Cheffile",
      "location": { "start_line": 1, "start_column": 1, "last_line": 1, "last_column": 1 },
      "corrected_by_autocorrect": false,
      "remediation": {
        "description": "The Cheffile dependency format is no longer supported. Migrate to Policyfile.rb or Berksfile.",
        "migration_url": "https://docs.chef.io/policyfile/",
        "introduced_in": "14.0",
        "removed_in": "15.0",
        "replacement_pattern": null
      }
    }
  ],
  "offenses_by_cop": [
    {
      "cop_name": "ChefDeprecations/ResourceWithoutUnifiedTrue",
      "count": 3,
      "all_auto_correctable": true,
      "remediation": {
        "description": "Custom resources should enable unified mode for compatibility with Chef 18+.",
        "migration_url": "https://docs.chef.io/unified_mode/",
        "introduced_in": "15.3",
        "removed_in": null,
        "replacement_pattern": "# Before:\nresource_name :my_resource\n\n# After:\nresource_name :my_resource\nunified_mode true"
      }
    }
  ]
}
```

### List Cookbooks by Remediation Priority

#### `GET /api/v1/remediation/priority`

Returns all incompatible and CookStyle-flagged cookbooks sorted by a priority score that combines complexity and blast radius. This powers the remediation guidance view in the dashboard. This endpoint aggregates results from both **server cookbooks** and **git repos**. The `source` field in each entry (`"chef_server"` or `"git"`) indicates the origin.

**Query parameters:** `target_chef_version` (required), standard filters, pagination.

**Response (200):**

```json
{
  "data": [
    {
      "cookbook_name": "base",
      "cookbook_version": "1.3.2",
      "source": "chef_server",
      "organisation": "myorg-production",
      "complexity_score": 8,
      "complexity_label": "low",
      "affected_node_count": 2000,
      "affected_role_count": 5,
      "affected_policy_count": 3,
      "auto_correctable_count": 6,
      "manual_fix_count": 2,
      "priority_score": 16008,
      "top_deprecations": ["ChefDeprecations/ResourceWithoutUnifiedTrue", "ChefDeprecations/Cheffile"]
    }
  ],
  "summary": {
    "total_cookbooks_needing_remediation": 15,
    "estimated_quick_wins": 5,
    "estimated_manual_fixes": 10,
    "total_blocked_nodes": 700,
    "projected_unblocked_if_all_fixed": 650
  },
  "pagination": { ... }
}
```

### Remediation Effort Summary

#### `GET /api/v1/remediation/summary`

Returns an aggregate effort estimation for the remediation view header.

**Query parameters:** `target_chef_version` (required), standard filters.

**Response (200):**

```json
{
  "target_chef_version": "19.0.0",
  "total_cookbooks_needing_remediation": 15,
  "quick_wins": 5,
  "manual_fixes_needed": 10,
  "total_blocked_nodes": 700,
  "total_auto_correctable_offenses": 42,
  "total_manual_fix_offenses": 18,
  "complexity_distribution": {
    "low": 5,
    "medium": 6,
    "high": 3,
    "critical": 1
  }
}
```

---
