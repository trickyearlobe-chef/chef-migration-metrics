# Web API — Server Cookbook Endpoints

## Server Cookbook Endpoints

> **Note:** These endpoints serve **server cookbooks** only — cookbooks sourced from Chef Infra Server organisations. For git-sourced repositories, see [Git Repo Endpoints](#git-repo-endpoints) below.

### List Server Cookbooks

#### `GET /api/v1/cookbooks`

Returns a paginated list of all known server cookbooks with usage summary.

**Query parameters:** standard filters (including `cookbook_status`), pagination, sorting.

**Sortable fields:** `name`, `version`, `node_count`, `active`.

**Response (200):**

```json
{
  "data": [
    {
      "cookbook_name": "nginx",
      "versions": [
        {
          "version": "4.0.0",
          "organisation": "myorg-production",
          "node_count": 1200,
          "active": true,
          "is_frozen": true,
          "maintainer": "ops-team",
          "description": "Installs and configures nginx",
          "license": "Apache-2.0",
          "platforms": ["ubuntu", "centos", "redhat"],
          "dependencies": { "apt": ">= 0.0.0", "yum": ">= 0.0.0" },
          "cookstyle_results": {
            "passed": false,
            "offence_count": 7,
            "deprecation_count": 3,
            "scanned_at": "2024-06-15T15:00:00Z"
          },
          "download_status": "complete"
        },
        {
          "version": "3.2.1",
          "organisation": "myorg-staging",
          "node_count": 5,
          "active": true,
          "is_frozen": false,
          "maintainer": "ops-team",
          "description": "Installs and configures nginx",
          "license": "Apache-2.0",
          "platforms": ["ubuntu", "centos"],
          "dependencies": { "apt": ">= 0.0.0" },
          "cookstyle_results": {
            "passed": true,
            "offence_count": 0,
            "deprecation_count": 0,
            "scanned_at": "2024-06-15T14:45:00Z"
          },
          "download_status": "complete"
        }
      ],
      "total_node_count": 1205
    }
  ],
  "pagination": { ... }
}
```

### Get Server Cookbook Detail

#### `GET /api/v1/cookbooks/:name`

Returns full detail for a specific server cookbook across all versions and organisations, including CookStyle results, complexity scores, associated git repos (matched by name), and Policyfile references.

**Response (200):**

```json
{
  "cookbook_name": "nginx",
  "is_stale_cookbook": false,
  "first_seen_at": "2024-01-15T10:00:00Z",
  "server_cookbooks": [
    {
      "version": "4.0.0",
      "organisation": "myorg-production",
      "node_count": 1200,
      "active": true,
      "is_frozen": true,
      "maintainer": "ops-team",
      "description": "Installs and configures nginx",
      "long_description": "A comprehensive cookbook for installing and configuring the nginx web server.",
      "license": "Apache-2.0",
      "platforms": ["ubuntu", "centos", "redhat"],
      "dependencies": { "apt": ">= 0.0.0", "yum": ">= 0.0.0" },
      "cookstyle_results": {
        "passed": false,
        "offence_count": 7,
        "deprecation_count": 3,
        "scanned_at": "2024-06-15T15:00:00Z"
      },
      "complexity": [
        {
          "target_chef_version": "18.5.0",
          "score": 15,
          "label": "medium",
          "auto_correctable": 4,
          "manual_fix": 3,
          "affected_node_count": 1200,
          "affected_role_count": 3,
          "affected_policy_count": 0
        }
      ],
      "download_status": "complete"
    },
    {
      "version": "3.2.1",
      "organisation": "myorg-staging",
      "node_count": 5,
      "active": true,
      "is_frozen": false,
      "maintainer": "ops-team",
      "description": "Installs and configures nginx",
      "long_description": "A comprehensive cookbook for installing and configuring the nginx web server.",
      "license": "Apache-2.0",
      "platforms": ["ubuntu", "centos"],
      "dependencies": { "apt": ">= 0.0.0" },
      "cookstyle_results": {
        "passed": true,
        "offence_count": 0,
        "deprecation_count": 0,
        "scanned_at": "2024-06-15T14:45:00Z"
      },
      "complexity": [
        {
          "target_chef_version": "18.5.0",
          "score": 2,
          "label": "low",
          "auto_correctable": 0,
          "manual_fix": 0,
          "affected_node_count": 5,
          "affected_role_count": 1,
          "affected_policy_count": 0
        }
      ],
      "download_status": "complete"
    }
  ],
  "git_repos": [
    {
      "name": "nginx",
      "git_repo_url": "https://github.com/myorg/nginx-cookbook.git",
      "default_branch": "main",
      "detail_url": "/api/v1/git-repos/nginx"
    }
  ],
  "nodes_by_platform": [
    { "platform": "ubuntu", "platform_version": "22.04", "count": 800 },
    { "platform": "centos", "platform_version": "7", "count": 400 },
    { "platform": "ubuntu", "platform_version": "20.04", "count": 5 }
  ],
  "nodes_by_environment": [
    { "environment": "production", "count": 900 },
    { "environment": "staging", "count": 300 },
    { "environment": "development", "count": 5 }
  ],
  "nodes_by_role": [
    { "role": "webserver", "count": 1000 },
    { "role": "base", "count": 1205 }
  ],
  "nodes_by_policy": [
    { "policy_name": "webserver", "policy_group": "production", "count": 500 },
    { "policy_name": "webserver", "policy_group": "staging", "count": 100 }
  ]
}
```

### Trigger Manual Rescan

#### `POST /api/v1/cookbooks/:name/rescan`

**Requires: `admin` role.**

Resets the download status for a server cookbook, triggering a re-download and re-analysis on the next collection cycle. Used for exceptional cases such as data corruption or tooling bugs. For git repo rescans, see `POST /api/v1/git-repos/:name/rescan`.

**Request body (optional):**

```json
{
  "organisation": "myorg-production"
}
```

If `organisation` is provided, only that organisation's copy is rescanned. If omitted, all copies across all organisations are rescanned.

**Response (202):**

```json
{
  "message": "Download status reset for cookbook nginx. Re-download will occur on next collection cycle.",
  "cookbook_name": "nginx",
  "versions_reset": 2
}
```

---
