# Web API — Git Repo Endpoints

## Git Repo Endpoints

> **Note:** These endpoints serve **git repos** — cookbook source repositories cloned from Git. For Chef server-sourced cookbooks, see [Server Cookbook Endpoints](#server-cookbook-endpoints) above.

### List Git Repos

#### `GET /api/v1/git-repos`

Returns a paginated list of all known git repos with optional filtering.

**Query parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Substring filter on repo name |
| `tk_status` | string | Comma-separated TK status filter: `passed`, `partial`, `failed`, `untested` |
| `target_chef_version` | string | Filter cookstyle results by target Chef version |
| `sort` | string | Sort field: `name` (default) |
| `order` | string | Sort direction: `asc` (default), `desc` |
| `page` | integer | Page number (default: 1) |
| `per_page` | integer | Items per page (default: 50) |

**Response (200):**

```json
{
  "data": [
    {
      "id": "nginx",
      "name": "nginx",
      "git_repo_url": "https://github.com/example-corp/nginx-cookbook.git",
      "head_commit_sha": "a1b2c3d4",
      "default_branch": "main",
      "has_test_suite": true,
      "clone_status": "ok",
      "last_fetched_at": "2024-06-15T14:30:00Z",
      "compatibility": "compatible",
      "target_chef_version": "18.0.0",
      "tk_status": "partial",
      "tk_passed": 3,
      "tk_total": 5
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 50,
    "total_items": 87,
    "total_pages": 2
  }
}
```

**TK Status values:**

| Value | Meaning |
|-------|---------|
| `passed` | All Test Kitchen instances passed |
| `partial` | Mix of passed and failed instances |
| `failed` | All tested instances failed (none passed) |
| `untested` | No Test Kitchen results available |

### Get Git Repo Detail

#### `GET /api/v1/git-repos/:name`

Returns full detail for a specific git repo, including CookStyle results, Test Kitchen results, and complexity scores.

**Response (200):**

```json
{
  "name": "nginx",
  "git_repos": [
    {
      "git_repo": {
        "id": 42,
        "name": "nginx",
        "git_repo_url": "https://github.com/myorg/nginx-cookbook.git",
        "head_commit_sha": "a1b2c3d4e5f67890abcdef1234567890abcdef12",
        "default_branch": "main",
        "has_test_suite": true,
        "last_fetched_at": "2024-06-15T14:30:00Z"
      },
      "cookstyle": [
        {
          "target_chef_version": "18.5.0",
          "passed": true,
          "offence_count": 0,
          "deprecation_count": 0,
          "scanned_at": "2024-06-15T14:35:00Z"
        },
        {
          "target_chef_version": "19.0.0",
          "passed": false,
          "offence_count": 4,
          "deprecation_count": 2,
          "scanned_at": "2024-06-15T14:36:00Z"
        }
      ],
      "test_kitchen": [
        {
          "target_chef_version": "18.5.0",
          "converge_passed": true,
          "tests_passed": true,
          "commit_sha": "a1b2c3d4e5f67890abcdef1234567890abcdef12",
          "tested_at": "2024-06-15T14:40:00Z"
        },
        {
          "target_chef_version": "19.0.0",
          "converge_passed": true,
          "tests_passed": false,
          "commit_sha": "a1b2c3d4e5f67890abcdef1234567890abcdef12",
          "tested_at": "2024-06-15T14:42:00Z"
        }
      ],
      "complexity": [
        {
          "target_chef_version": "19.0.0",
          "score": 30,
          "label": "medium",
          "auto_correctable": 2,
          "manual_fix": 2,
          "affected_node_count": 1200,
          "affected_role_count": 3,
          "affected_policy_count": 0
        }
      ]
    }
  ]
}
```

### Trigger Git Repo Rescan

#### `POST /api/v1/git-repos/:name/rescan`

**Requires: authenticated user.**

Invalidates all CookStyle results, complexity scores, and autocorrect previews for the named git repo. The next collection cycle will re-run analysis from the current HEAD.

**Response (200):**

```json
{
  "git_repo_name": "nginx",
  "repos_invalidated": 1,
  "message": "All analysis results invalidated for git repo nginx. Re-analysis will occur on next collection cycle."
}
```

### Reset Git Repo

#### `POST /api/v1/git-repos/:name/reset`

**Requires: `operator` or `admin` role.**

Deletes all git repo data (analysis results, committer records) and removes the local clone from disk. The repo will be re-cloned and re-analysed on the next collection cycle.

**Response (200):**

```json
{
  "git_repo_name": "nginx",
  "repos_deleted": 1,
  "committers_deleted": 12,
  "repo_urls_removed": ["https://github.com/myorg/nginx-cookbook.git"],
  "local_clone_removed": true,
  "message": "Git repo nginx fully reset. Re-clone will occur on next collection cycle."
}
```

### List Git Repo Committers

#### `GET /api/v1/git-repos/:name/committers`

Returns a list of committers for the named git repo.

**Response (200):**

```json
{
  "git_repo_name": "nginx",
  "committers": [
    {
      "name": "Jane Smith",
      "email": "jane.smith@example.com",
      "commit_count": 47,
      "last_commit_at": "2024-06-14T09:15:00Z",
      "is_owner": true
    },
    {
      "name": "John Doe",
      "email": "john.doe@example.com",
      "commit_count": 12,
      "last_commit_at": "2024-05-20T16:30:00Z",
      "is_owner": false
    }
  ]
}
```

### Assign Git Repo Committers as Owners

#### `POST /api/v1/git-repos/:name/committers/assign`

**Requires: `operator` or `admin` role.**

Assigns one or more committers as owners of the git repo for ownership tracking.

**Request body:**

```json
{
  "emails": ["jane.smith@example.com", "john.doe@example.com"]
}
```

**Response (200):**

```json
{
  "git_repo_name": "nginx",
  "assigned": 2,
  "message": "2 committers assigned as owners for git repo nginx."
}
```

### Get Git Repo Remediation Detail

#### `GET /api/v1/git-repos/:name/:version/remediation`

Returns the full remediation guidance for a specific git repo version, including offense groups, autocorrect preview, and cop-level remediation guidance.

**Query parameters:** `target_chef_version` (optional, defaults to all configured targets).

**Response (200):**

```json
{
  "git_repo_name": "nginx",
  "version": "5.1.0",
  "target_chef_version": "19.0.0",
  "source": "git",
  "complexity_score": 30,
  "complexity_label": "medium",
  "statistics": {
    "error_count": 1,
    "deprecation_count": 2,
    "correctness_count": 0,
    "modernize_count": 1,
    "auto_correctable_count": 2,
    "manual_fix_count": 2
  },
  "offense_groups": [
    {
      "cop_name": "ChefDeprecations/ResourceWithoutUnifiedTrue",
      "severity": "warning",
      "count": 2,
      "all_auto_correctable": true,
      "offenses": [
        {
          "message": "Set unified_mode true in Chef Infra Client 15.3+",
          "file": "resources/my_resource.rb",
          "location": { "start_line": 10, "start_column": 1, "last_line": 10, "last_column": 30 },
          "corrected_by_autocorrect": true
        },
        {
          "message": "Set unified_mode true in Chef Infra Client 15.3+",
          "file": "resources/other_resource.rb",
          "location": { "start_line": 5, "start_column": 1, "last_line": 5, "last_column": 28 },
          "corrected_by_autocorrect": true
        }
      ],
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
      "count": 1,
      "all_auto_correctable": false,
      "offenses": [
        {
          "message": "Cheffile is deprecated. Use a Policyfile or Berkshelf instead.",
          "file": "Cheffile",
          "location": { "start_line": 1, "start_column": 1, "last_line": 1, "last_column": 1 },
          "corrected_by_autocorrect": false
        }
      ],
      "remediation": {
        "description": "The Cheffile dependency format is no longer supported. Migrate to Policyfile.rb or Berksfile.",
        "migration_url": "https://docs.chef.io/policyfile/",
        "introduced_in": "14.0",
        "removed_in": "15.0",
        "replacement_pattern": null
      }
    }
  ],
  "autocorrect_preview": {
    "total_offenses": 4,
    "correctable_offenses": 2,
    "remaining_offenses": 2,
    "files_modified": 2,
    "diff": "--- a/resources/my_resource.rb\n+++ b/resources/my_resource.rb\n@@ -10,1 +10,2 @@\n-resource_name :my_resource\n+resource_name :my_resource\n+unified_mode true\n"
  }
}
```

---
