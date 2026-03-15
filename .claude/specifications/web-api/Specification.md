# Web API - Component Specification

> **Implementation language:** Go. See `../../Claude.md` for language and concurrency rules.

> Component specification for the HTTP API layer of Chef Migration Metrics.
> See the [top-level specification](../Specification.md) for project overview and scope.

---

## TL;DR

RESTful JSON API (Go) between backend and React frontend. Mostly read-only over the datastore; write operations limited to admin actions (user management, manual rescan, auth provider config) and operator actions (ownership management, bulk import/reassignment). Key endpoint groups: nodes, server cookbooks, git repos, compatibility results, readiness, remediation, ownership, dependency graph, exports, notifications, logs, and admin. Cookbooks are split into **server cookbooks** (sourced from Chef Infra Server) and **git repos** (cloned from Git), each with their own endpoints. Dashboard and remediation priority endpoints aggregate across both sources. All list endpoints support pagination (`page`/`per_page`), filtering (org, environment, role, policy, platform, stale status, complexity label, owner), and sorting. Auth via session cookie with RBAC middleware (viewer / operator / admin). CORS configurable. Export endpoints support sync (small) and async (large, returns job ID). Notification endpoints manage webhook/email channels and history. Ownership endpoints manage owners, assignments, bulk reassignment, audit log, and committer-to-owner workflows (see [Ownership Specification](../ownership/Specification.md)). See `../auth/Specification.md` for auth details, `../datastore/Specification.md` for schema.

---

## Overview

The Web API is the HTTP layer between the Go backend and the web dashboard frontend. It exposes a RESTful JSON API that the frontend consumes to render all dashboard views, filters, drill-downs, log viewer, and administrative functions.

This component is purely a read/query layer over the datastore for dashboard data. The only write operations are administrative (user management, manual rescan triggers, configuration of authentication providers).

All endpoints require authentication unless explicitly marked as public. See the [Authentication specification](../auth/Specification.md) for provider details and the [Session Management](#session-management) section below for how sessions are enforced.

---

## Base URL

All API endpoints are served under the `/api/v1` prefix:

```
https://<HOST>:<PORT>/api/v1
```

The application listens on a configurable port (default: `8080`). HTTPS termination may be handled by a reverse proxy or natively by the application if TLS certificate paths are configured.

---

## Content Type

- All request bodies must be `application/json`.
- All response bodies are `application/json`.
- Responses include `Content-Type: application/json; charset=utf-8`.

---

## Authentication and Session Management

### Login

#### `POST /api/v1/auth/login`

Authenticates a user via the local provider and returns a session token.

**Request:**

```json
{
  "username": "alice",
  "password": "s3cret"
}
```

**Response (200):**

```json
{
  "token": "<signed-session-token>",
  "expires_at": "2024-07-01T08:00:00Z",
  "user": {
    "username": "alice",
    "display_name": "Alice Smith",
    "role": "admin"
  }
}
```

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | Missing or malformed request body |
| `401` | Invalid credentials |
| `423` | Account locked due to excessive failed attempts |
| `429` | Rate limited — too many login attempts from this source |

#### `POST /api/v1/auth/saml/acs`

SAML Assertion Consumer Service endpoint. Receives the SAML response from the IdP, validates the assertion, establishes a session, and redirects to the dashboard.

**Public** — no existing session required.

#### `GET /api/v1/auth/saml/metadata`

Returns the SAML Service Provider metadata XML.

**Public** — no existing session required.

#### `GET /api/v1/auth/saml/login`

Initiates the SAML authentication flow by redirecting to the configured IdP.

**Public** — no existing session required.

### Logout

#### `POST /api/v1/auth/logout`

Invalidates the current session.

**Response (204):** No content.

### Session Enforcement

- All endpoints except those marked **Public** require a valid session token.
- The session token must be sent in the `Authorization` header as a Bearer token:
  ```
  Authorization: Bearer <token>
  ```
- Alternatively, the token may be sent in a secure, HTTP-only cookie named `session`.
- If the token is missing, expired, or invalid, the API returns `401 Unauthorized`.
- Session expiry is configured via `auth.session_expiry` (see [Configuration specification](../configuration/Specification.md)).

### Current User

#### `GET /api/v1/auth/me`

Returns the authenticated user's profile and role.

**Response (200):**

```json
{
  "username": "alice",
  "display_name": "Alice Smith",
  "email": "alice@example.com",
  "role": "admin",
  "provider": "local"
}
```

---

## Authorisation Middleware

After authentication, the middleware checks the user's role against the endpoint's required permission level:

| Role | Permissions |
|------|-------------|
| `viewer` | Read access to all dashboard, log, ownership, and status endpoints |
| `operator` | All viewer permissions plus create/update owners and assignments, bulk import, bulk reassignment (without delete-source-owner) |
| `admin` | All operator permissions plus user management, owner deletion, bulk reassignment with delete-source-owner, manual rescan triggers, and configuration |

Endpoints that require `admin` or `operator` are annotated below. All other authenticated endpoints require at minimum `viewer`.

Unauthorised requests return `403 Forbidden`:

```json
{
  "error": "forbidden",
  "message": "This action requires the admin role."
}
```

---

## Common Patterns

### Pagination

All list endpoints support cursor-based or offset-based pagination. The default and maximum page size are configurable; defaults are shown below.

**Query parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | integer | `1` | Page number (1-indexed) |
| `per_page` | integer | `50` | Items per page (max: `500`) |

**Response envelope:**

All paginated responses use a consistent envelope:

```json
{
  "data": [ ... ],
  "pagination": {
    "page": 1,
    "per_page": 50,
    "total_items": 4832,
    "total_pages": 97
  }
}
```

### Filtering

Filter parameters are passed as query string parameters. Multiple values for the same filter are comma-separated. All filters are applied server-side; the full dataset is never returned to the client.

**Common filter parameters (available on most list endpoints):**

| Parameter | Type | Description |
|-----------|------|-------------|
| `organisation` | string | Comma-separated list of organisation names |
| `environment` | string | Comma-separated list of Chef environment names |
| `role` | string | Comma-separated list of role names |
| `policy_name` | string | Comma-separated list of Policyfile policy names |
| `policy_group` | string | Comma-separated list of Policyfile policy groups |
| `platform` | string | Comma-separated list of platform names |
| `platform_version` | string | Comma-separated list of platform versions |
| `target_chef_version` | string | Target Chef Client version for readiness evaluation |
| `cookbook_status` | string | `active`, `unused`, or `all` (default: `active`) |
| `stale_status` | string | `all`, `stale`, or `fresh` (default: `all`). Filters nodes by stale check-in status. |
| `complexity_label` | string | Comma-separated list of complexity labels: `low`, `medium`, `high`, `critical`. Filters cookbooks by remediation complexity. |
| `owner` | string | Comma-separated list of owner names. Filters to entities owned by specified owners (see [Ownership Specification](../ownership/Specification.md) § 4.5). Only active when ownership is enabled. |
| `unowned` | boolean | When `true`, filters to entities with no resolved owner. Cannot be combined with `owner`. Only active when ownership is enabled. |

### Sorting

Sortable endpoints accept:

| Parameter | Type | Description |
|-----------|------|-------------|
| `sort` | string | Field name to sort by |
| `order` | string | `asc` or `desc` (default: `asc`) |

### Error Responses

All error responses use a consistent structure:

```json
{
  "error": "not_found",
  "message": "Node 'web-01' was not found in organisation 'myorg-production'."
}
```

Standard error codes:

| Status | `error` value | Meaning |
|--------|---------------|---------|
| `400` | `bad_request` | Malformed request, invalid parameters |
| `401` | `unauthorized` | Missing or invalid session token |
| `403` | `forbidden` | Authenticated but insufficient role |
| `404` | `not_found` | Requested resource does not exist |
| `422` | `validation_error` | Request body fails validation |
| `429` | `rate_limited` | Too many requests |
| `500` | `internal_error` | Unexpected server error |

---

## Dashboard Endpoints

### Chef Client Version Distribution

#### `GET /api/v1/dashboard/version-distribution`

Returns the count and percentage of nodes running each Chef Client version, scoped by active filters.

**Query parameters:** standard filters (organisation, environment, role, platform, platform_version).

**Response (200):**

```json
{
  "data": [
    {
      "chef_version": "17.10.0",
      "node_count": 1200,
      "percentage": 48.0
    },
    {
      "chef_version": "18.5.0",
      "node_count": 800,
      "percentage": 32.0
    },
    {
      "chef_version": "16.17.4",
      "node_count": 500,
      "percentage": 20.0
    }
  ],
  "total_nodes": 2500
}
```

#### `GET /api/v1/dashboard/version-distribution/trend`

Returns the version distribution over time as a time series for trend charts.

**Additional query parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `from` | ISO-8601 date | 30 days ago | Start of time range |
| `to` | ISO-8601 date | now | End of time range |

**Response (200):**

```json
{
  "data": [
    {
      "timestamp": "2024-06-01T00:00:00Z",
      "versions": {
        "17.10.0": 1300,
        "18.5.0": 700,
        "16.17.4": 600
      },
      "total_nodes": 2600
    },
    {
      "timestamp": "2024-06-02T00:00:00Z",
      "versions": {
        "17.10.0": 1200,
        "18.5.0": 800,
        "16.17.4": 500
      },
      "total_nodes": 2500
    }
  ]
}
```

### Node Upgrade Readiness

#### `GET /api/v1/dashboard/readiness`

Returns a summary of how many nodes are ready vs. blocked for each target Chef Client version.

**Query parameters:** standard filters plus `target_chef_version` (required).

**Response (200):**

```json
{
  "target_chef_version": "19.0.0",
  "ready_count": 1800,
  "blocked_count": 700,
  "stale_count": 45,
  "total_nodes": 2500,
  "blocking_reasons": {
    "incompatible_cookbook": 580,
    "insufficient_disk_space": 200,
    "both": 80,
    "stale_data": 45
  }
}
```

#### `GET /api/v1/dashboard/readiness/trend`

Returns readiness counts over time for trend charts.

**Additional query parameters:** `from`, `to`, `target_chef_version` (required).

**Response (200):**

```json
{
  "target_chef_version": "19.0.0",
  "data": [
    {
      "timestamp": "2024-06-01T00:00:00Z",
      "ready_count": 1500,
      "blocked_count": 1000,
      "total_nodes": 2500
    },
    {
      "timestamp": "2024-06-02T00:00:00Z",
      "ready_count": 1800,
      "blocked_count": 700,
      "total_nodes": 2500
    }
  ]
}
```

### Cookbook Compatibility

#### `GET /api/v1/dashboard/cookbook-compatibility`

Returns the compatibility status of each cookbook (and version) for each target Chef Client version. This endpoint aggregates results from both **server cookbooks** and **git repos**. The `source` field in each entry indicates the origin (`"chef_server"` or `"git"`).

**Query parameters:** standard filters, plus pagination and sorting. Supports `complexity_label` filter.

**Sortable fields:** `cookbook_name`, `node_count`, `complexity_score`, `status`.

**Response (200):**

```json
{
  "data": [
    {
      "cookbook_name": "nginx",
      "cookbook_version": "5.1.0",
      "source": "git",
      "organisation": null,
      "compatibility": [
        {
          "target_chef_version": "18.5.0",
          "status": "compatible",
          "confidence": "high",
          "converge_passed": true,
          "tests_passed": true,
          "commit_sha": "a1b2c3d4e5f6",
          "tested_at": "2024-06-15T14:30:00Z"
        },
        {
          "target_chef_version": "19.0.0",
          "status": "incompatible",
          "confidence": null,
          "converge_passed": true,
          "tests_passed": false,
          "commit_sha": "a1b2c3d4e5f6",
          "tested_at": "2024-06-15T14:35:00Z",
          "complexity": {
            "score": 30,
            "label": "medium",
            "auto_correctable": 0,
            "manual_fix": 0,
            "affected_node_count": 1200,
            "affected_role_count": 3
          }
        }
      ],
      "node_count": 1200,
      "active": true,
      "is_stale_cookbook": false
    },
    {
      "cookbook_name": "legacy-app",
      "cookbook_version": "2.0.0",
      "source": "chef_server",
      "organisation": "myorg-production",
      "compatibility": [
        {
          "target_chef_version": "18.5.0",
          "status": "cookstyle_only",
          "confidence": "medium",
          "cookstyle_passed": false,
          "deprecation_warnings": 3,
          "scanned_at": "2024-06-15T15:00:00Z",
          "complexity": {
            "score": 15,
            "label": "medium",
            "auto_correctable": 2,
            "manual_fix": 1,
            "affected_node_count": 50,
            "affected_role_count": 1
          }
        }
      ],
      "node_count": 50,
      "active": true,
      "is_stale_cookbook": true
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 50,
    "total_items": 312,
    "total_pages": 7
  }
}
```

**Compatibility status values:**

| Status | Confidence | Meaning |
|--------|------------|---------|
| `compatible` | `high` | Test Kitchen converge and tests passed at HEAD (git repos) |
| `incompatible` | N/A | Test Kitchen converge or tests failed at HEAD (git repos) |
| `cookstyle_only` | `medium` | Chef server-sourced; CookStyle results only — no integration test |
| `untested` | N/A | No test or scan results available yet |

---

## Node Endpoints

### List Nodes

#### `GET /api/v1/nodes`

Returns a paginated, filterable list of nodes.

**Query parameters:** standard filters (including `policy_name`, `policy_group`, `stale_status`), pagination, sorting.

**Sortable fields:** `name`, `chef_version`, `platform`, `platform_version`, `chef_environment`, `policy_name`, `policy_group`, `last_collected_at`, `ohai_time`.

**Response (200):**

```json
{
  "data": [
    {
      "name": "web-node-01",
      "organisation": "myorg-production",
      "chef_environment": "production",
      "chef_version": "17.10.0",
      "platform": "ubuntu",
      "platform_version": "22.04",
      "platform_family": "debian",
      "roles": ["base", "webserver"],
      "policy_name": null,
      "policy_group": null,
      "cookbook_count": 12,
      "is_stale": false,
      "last_checkin_age_hours": 2.5,
      "last_collected_at": "2024-06-15T12:00:00Z"
    }
  ],
  "pagination": { ... }
}
```

### Get Node Detail

#### `GET /api/v1/nodes/:organisation/:name`

Returns full detail for a single node, including readiness status per target version, blocking reasons with complexity scores, Policyfile metadata, and stale data status.

**Response (200):**

```json
{
  "name": "web-node-01",
  "organisation": "myorg-production",
  "chef_environment": "production",
  "chef_version": "17.10.0",
  "platform": "ubuntu",
  "platform_version": "22.04",
  "platform_family": "debian",
  "roles": ["base", "webserver"],
  "policy_name": null,
  "policy_group": null,
  "run_list": ["role[base]", "recipe[nginx::default]", "recipe[nginx::config]"],
  "cookbooks": {
    "nginx": "5.1.0",
    "base": "1.3.2",
    "apt": "7.4.0"
  },
  "disk_space": {
    "root_partition_free_mb": 4096,
    "threshold_mb": 2048,
    "sufficient": true
  },
  "is_stale": false,
  "last_checkin_age_hours": 2.5,
  "ohai_time": "2024-06-15T09:30:00Z",
  "readiness": [
    {
      "target_chef_version": "18.5.0",
      "ready": true,
      "stale_data": false,
      "blocking_reasons": []
    },
    {
      "target_chef_version": "19.0.0",
      "ready": false,
      "stale_data": false,
      "blocking_reasons": [
        {
          "type": "incompatible_cookbook",
          "cookbook_name": "nginx",
          "cookbook_version": "5.1.0",
          "detail": "Test Kitchen tests failed against Chef Client 19.0.0",
          "complexity_score": 30,
          "complexity_label": "medium"
        }
      ]
    }
  ],
  "last_collected_at": "2024-06-15T12:00:00Z"
}
```

### List Nodes by Chef Version

#### `GET /api/v1/nodes/by-version/:chef_version`

Returns nodes running a specific Chef Client version. This supports drill-down from the version distribution view.

**Query parameters:** standard filters, pagination.

**Response:** Same structure as `GET /api/v1/nodes`.

### List Nodes by Cookbook

#### `GET /api/v1/nodes/by-cookbook/:cookbook_name`

Returns nodes running a specific cookbook (any version).

**Additional query parameter:** `cookbook_version` (optional, filters to a specific version).

**Query parameters:** standard filters, pagination.

**Response:** Same structure as `GET /api/v1/nodes`.

---

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

## Git Repo Endpoints

> **Note:** These endpoints serve **git repos** — cookbook source repositories cloned from Git. For Chef server-sourced cookbooks, see [Server Cookbook Endpoints](#server-cookbook-endpoints) above.

### List Git Repos

#### `GET /api/v1/git-repos`

Returns a paginated list of all known git repos with optional name filtering.

**Query parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Substring filter on repo name |
| `page` | integer | Page number (default: 1) |
| `per_page` | integer | Items per page (default: 50) |

**Response (200):**

```json
{
  "data": [
    {
      "id": 42,
      "name": "nginx",
      "git_repo_url": "https://github.com/myorg/nginx-cookbook.git",
      "head_commit_sha": "a1b2c3d4e5f67890abcdef1234567890abcdef12",
      "default_branch": "main",
      "has_test_suite": true,
      "last_fetched_at": "2024-06-15T14:30:00Z"
    },
    {
      "id": 43,
      "name": "base",
      "git_repo_url": "https://github.com/myorg/base-cookbook.git",
      "head_commit_sha": "b2c3d4e5f67890abcdef1234567890abcdef1234",
      "default_branch": "main",
      "has_test_suite": false,
      "last_fetched_at": "2024-06-15T14:32:00Z"
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

## Dependency Graph Endpoints

### Get Role Dependency Graph

#### `GET /api/v1/dependency-graph`

Returns the role-to-role and role-to-cookbook dependency graph for use in the interactive graph view.

**Query parameters:** `organisation` (required), `cookbook` (optional — filter to subgraph involving a specific cookbook), `role` (optional — filter to subgraph reachable from a specific role), `compatibility_status` (optional — `incompatible`, `untested`, or `all`; default: `all`).

**Response (200):**

```json
{
  "nodes": [
    { "id": "role:base", "type": "role", "name": "base" },
    { "id": "role:webserver", "type": "role", "name": "webserver" },
    { "id": "cookbook:nginx", "type": "cookbook", "name": "nginx", "compatibility_status": "incompatible", "complexity_label": "medium" },
    { "id": "cookbook:base", "type": "cookbook", "name": "base", "compatibility_status": "compatible", "complexity_label": "none" },
    { "id": "cookbook:apt", "type": "cookbook", "name": "apt", "compatibility_status": "compatible", "complexity_label": "none" }
  ],
  "edges": [
    { "from": "role:webserver", "to": "role:base", "type": "includes_role" },
    { "from": "role:webserver", "to": "cookbook:nginx", "type": "includes_cookbook" },
    { "from": "role:base", "to": "cookbook:base", "type": "includes_cookbook" },
    { "from": "role:base", "to": "cookbook:apt", "type": "includes_cookbook" }
  ],
  "metadata": {
    "total_roles": 2,
    "total_cookbooks": 3,
    "incompatible_cookbooks": 1
  }
}
```

### Get Role Dependency Table

#### `GET /api/v1/dependency-graph/table`

Returns a flat table view of role dependencies for users who prefer a list format over a graph.

**Query parameters:** `organisation` (required), `target_chef_version` (optional), pagination, sorting.

**Sortable fields:** `role_name`, `direct_cookbook_count`, `transitive_cookbook_count`, `incompatible_count`.

**Response (200):**

```json
{
  "data": [
    {
      "role_name": "webserver",
      "organisation": "myorg-production",
      "direct_cookbooks": ["nginx"],
      "direct_roles": ["base"],
      "transitive_cookbooks": ["nginx", "base", "apt"],
      "total_cookbook_count": 3,
      "incompatible_cookbooks": ["nginx"],
      "incompatible_count": 1,
      "affected_node_count": 500
    }
  ],
  "pagination": { ... }
}
```

---

## Export Endpoints

### Request Export

#### `POST /api/v1/exports`

**Requires: `viewer` role (or higher).**

Requests a data export. Small exports are returned synchronously; large exports (exceeding the configured `exports.async_threshold`) are processed asynchronously.

**Request body:**

```json
{
  "export_type": "ready_nodes",
  "format": "csv",
  "target_chef_version": "19.0.0",
  "filters": {
    "organisation": "myorg-production",
    "environment": "production",
    "platform": "ubuntu"
  }
}
```

**Export types:**

| Type | Description |
|------|-------------|
| `ready_nodes` | Nodes ready to upgrade for the specified target version |
| `blocked_nodes` | Blocked nodes with blocking reasons and complexity scores |
| `cookbook_remediation` | Full remediation report for all incompatible cookbooks |

**Export formats:**

| Format | Description |
|--------|-------------|
| `csv` | Comma-separated values |
| `json` | JSON array of objects |
| `chef_search_query` | Chef search query string (only for `ready_nodes`) — e.g. `name:web-node-01 OR name:web-node-02 OR ...` |

**Response (200) — synchronous (small export):**

Returns the export data directly with the appropriate `Content-Type` header (`text/csv`, `application/json`, or `text/plain`).

**Response (202) — asynchronous (large export):**

```json
{
  "job_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "status": "pending",
  "message": "Export queued. Poll GET /api/v1/exports/a1b2c3d4-... for status."
}
```

### Get Export Status

#### `GET /api/v1/exports/:job_id`

Returns the status of an asynchronous export job.

**Response (200):**

```json
{
  "job_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "export_type": "ready_nodes",
  "format": "csv",
  "status": "completed",
  "row_count": 1800,
  "file_size_bytes": 245760,
  "download_url": "/api/v1/exports/a1b2c3d4-.../download",
  "requested_at": "2024-06-15T16:00:00Z",
  "completed_at": "2024-06-15T16:00:15Z",
  "expires_at": "2024-06-16T16:00:15Z"
}
```

### Download Export

#### `GET /api/v1/exports/:job_id/download`

Downloads the completed export file.

**Response (200):** File download with appropriate `Content-Type` and `Content-Disposition` headers.

**Response (404):** Export not found or expired.
**Response (409):** Export not yet completed.

---

## Notification Endpoints

### List Notification History

#### `GET /api/v1/notifications`

Returns a paginated list of sent notifications.

**Query parameters:** `event_type`, `channel_name`, `status`, `from`, `to`, pagination.

**Response (200):**

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440099",
      "channel_name": "slack-ops",
      "channel_type": "webhook",
      "event_type": "cookbook_status_change",
      "summary": "Cookbook 'nginx' is now compatible with Chef Client 19.0.0",
      "status": "sent",
      "sent_at": "2024-06-15T15:00:00Z"
    }
  ],
  "pagination": { ... }
}
```

### Get Notification Detail

#### `GET /api/v1/notifications/:id`

Returns the full detail of a sent notification, including the payload.

**Response (200):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440099",
  "channel_name": "slack-ops",
  "channel_type": "webhook",
  "event_type": "cookbook_status_change",
  "summary": "Cookbook 'nginx' is now compatible with Chef Client 19.0.0",
  "payload": {
    "cookbook_name": "nginx",
    "previous_status": "incompatible",
    "new_status": "compatible",
    "target_chef_version": "19.0.0",
    "commit_sha": "f1e2d3c4b5a6"
  },
  "status": "sent",
  "error_message": null,
  "retry_count": 0,
  "sent_at": "2024-06-15T15:00:00Z"
}
```

---

## Ownership Endpoints

Ownership tracking endpoints allow managing owners, ownership assignments, bulk reassignment, bulk import, audit log, and committer-to-owner workflows. These endpoints are fully specified in the [Ownership Specification](../ownership/Specification.md) § 4 and are summarised here for cross-reference.

| Endpoint | Method | Description | Auth |
|----------|--------|-------------|------|
| `/api/v1/owners` | GET | List all owners | viewer+ |
| `/api/v1/owners` | POST | Create an owner | operator+ |
| `/api/v1/owners/:name` | GET | Get owner detail with migration progress | viewer+ |
| `/api/v1/owners/:name` | PUT | Update an owner | operator+ |
| `/api/v1/owners/:name` | DELETE | Delete an owner (cascades) | admin |
| `/api/v1/owners/:name/assignments` | GET | List assignments for an owner | viewer+ |
| `/api/v1/owners/:name/assignments` | POST | Create assignments | operator+ |
| `/api/v1/owners/:name/assignments/:id` | DELETE | Delete an assignment | operator+ |
| `/api/v1/ownership/reassign` | POST | Bulk reassign between owners | operator+ |
| `/api/v1/ownership/import` | POST | Bulk import from CSV/JSON | operator+ |
| `/api/v1/ownership/lookup` | GET | Look up ownership for an entity | viewer+ |
| `/api/v1/ownership/audit-log` | GET | Query ownership audit log | viewer+ |
| `/api/v1/cookbooks/:name/committers` | GET | List git committers for a cookbook | viewer+ |
| `/api/v1/cookbooks/:name/committers/assign` | POST | Assign committers as owners | operator+ |

→ Full endpoint specifications: [Ownership Specification § 4](../ownership/Specification.md)

---

## Organisation Endpoints

### List Organisations

#### `GET /api/v1/organisations`

Returns all configured Chef Infra Server organisations.

**Response (200):**

```json
{
  "data": [
    {
      "name": "myorg-production",
      "chef_server_url": "https://chef.example.com",
      "org_name": "myorg-production",
      "client_name": "chef-migration-metrics",
      "credential_source": "file",
      "source": "config",
      "node_count": 2000,
      "last_collected_at": "2024-06-15T12:00:00Z",
      "last_collection_status": "success"
    },
    {
      "name": "myorg-staging",
      "chef_server_url": "https://chef.example.com",
      "org_name": "myorg-staging",
      "client_name": "chef-migration-metrics",
      "credential_source": "database",
      "credential_name": "myorg-staging-key",
      "source": "api",
      "node_count": 500,
      "last_collected_at": "2024-06-15T12:00:00Z",
      "last_collection_status": "partial_failure"
    }
  ]
}
```

| Field | Description |
|-------|-------------|
| `credential_source` | One of `file`, `database`, `environment`. Indicates how the Chef API key is supplied. |
| `credential_name` | Name of the database credential (only present when `credential_source` is `database`). |
| `source` | One of `config`, `api`. Whether the org was defined in the YAML config or created via the Web API. |

> **Security note:** The API never returns the private key value, file path, or any portion of the key material. Only the credential source and name are disclosed.

### Create Organisation

#### `POST /api/v1/organisations`

Creates a new Chef Infra Server organisation with its API credentials stored in the database. Requires the `admin` role.

The private key is encrypted using AES-256-GCM before storage. After this call, the plaintext key is not retrievable via any API endpoint.

**Request:**

```json
{
  "name": "myorg-staging",
  "chef_server_url": "https://chef.example.com",
  "org_name": "myorg-staging",
  "client_name": "chef-migration-metrics",
  "client_key_pem": "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----\n"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique friendly name for this organisation |
| `chef_server_url` | Yes | Base URL of the Chef Infra Server |
| `org_name` | Yes | Organisation name on the Chef server |
| `client_name` | Yes | Chef API client name |
| `client_key_pem` | Yes | PEM-encoded RSA private key. Validated as a parseable RSA key before storage. |

**Response (201):**

```json
{
  "name": "myorg-staging",
  "chef_server_url": "https://chef.example.com",
  "org_name": "myorg-staging",
  "client_name": "chef-migration-metrics",
  "credential_source": "database",
  "credential_name": "myorg-staging-key",
  "source": "api"
}
```

The `client_key_pem` value is **never** included in the response.

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | `client_key_pem` is not a valid PEM-encoded RSA private key |
| `409` | An organisation with this `name` or `(chef_server_url, org_name)` already exists |
| `422` | Required fields missing or invalid |
| `503` | Credential encryption key (`CMM_CREDENTIAL_ENCRYPTION_KEY`) is not configured — database credential storage is unavailable |

### Update Organisation

#### `PUT /api/v1/organisations/:name`

Updates an existing organisation. Requires the `admin` role. Only API-sourced organisations can be fully updated; config-sourced organisations allow only credential rotation (the `client_key_pem` field).

**Request:**

```json
{
  "chef_server_url": "https://new-chef.example.com",
  "org_name": "myorg-staging",
  "client_name": "chef-migration-metrics-v2",
  "client_key_pem": "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----\n"
}
```

All fields are optional. Only provided fields are updated. If `client_key_pem` is provided, the stored credential is re-encrypted with the new value.

**Response (200):** Updated organisation object (same shape as the create response).

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | `client_key_pem` is not a valid PEM-encoded RSA private key |
| `403` | Attempted to modify `chef_server_url`, `org_name`, or `client_name` on a config-sourced organisation |
| `404` | Organisation not found |
| `503` | Credential encryption key not configured |

### Test Organisation Credentials

#### `POST /api/v1/organisations/:name/test`

Validates that the stored credentials can successfully authenticate to the Chef Infra Server by making a lightweight API call (`GET /organizations/<org>/nodes?rows=0`). Requires the `admin` role.

**Response (200):**

```json
{
  "name": "myorg-staging",
  "status": "ok",
  "message": "Successfully authenticated to Chef Infra Server",
  "server_version": "15.9.38"
}
```

**Response (200, failure):**

```json
{
  "name": "myorg-staging",
  "status": "error",
  "message": "Authentication failed: 401 Unauthorized"
}
```

> This endpoint always returns 200 (the *test* succeeded in running) — the `status` field indicates whether the *credentials* are valid. This avoids ambiguity between "test couldn't run" (5xx) and "credentials are bad" (status: error).

### Delete Organisation

#### `DELETE /api/v1/organisations/:name`

Deletes an API-sourced organisation and its associated database credential. Config-sourced organisations cannot be deleted via the API. Requires the `admin` role.

**Response (204):** No content.

**Errors:**

| Status | Condition |
|--------|-----------|
| `403` | Organisation is config-sourced (must be removed from the YAML config file) |
| `404` | Organisation not found |

> **Cascade behaviour:** Deleting an organisation removes all associated `collection_runs`, `node_snapshots`, `cookbooks`, analysis results, and other dependent data via foreign key cascades. The associated credential row in the `credentials` table is also deleted. This is an irreversible operation — the API should require confirmation (e.g. a `confirm=true` query parameter).

---

## Filter Option Endpoints

These endpoints return the distinct values available for each filter dimension, enabling the frontend to populate filter dropdowns dynamically.

### `GET /api/v1/filters/environments`

**Query parameters:** `organisation` (optional, scopes to one or more orgs).

**Response (200):**

```json
{
  "data": ["production", "staging", "development", "qa"]
}
```

### `GET /api/v1/filters/roles`

**Query parameters:** `organisation` (optional).

**Response (200):**

```json
{
  "data": ["base", "webserver", "database", "monitoring"]
}
```

### `GET /api/v1/filters/policy-names`

**Query parameters:** `organisation` (optional).

**Response (200):**

```json
{
  "data": ["webserver", "database", "base"]
}
```

### `GET /api/v1/filters/policy-groups`

**Query parameters:** `organisation` (optional).

**Response (200):**

```json
{
  "data": ["production", "staging", "development"]
}
```

### `GET /api/v1/filters/platforms`

**Query parameters:** `organisation` (optional).

**Response (200):**

```json
{
  "data": [
    { "platform": "ubuntu", "versions": ["20.04", "22.04"] },
    { "platform": "centos", "versions": ["7", "8"] },
    { "platform": "windows", "versions": ["2019", "2022"] }
  ]
}
```

### `GET /api/v1/filters/target-chef-versions`

Returns the configured target Chef Client versions.

**Response (200):**

```json
{
  "data": ["18.5.0", "19.0.0"]
}
```

---

### `GET /api/v1/filters/complexity-labels`

Returns the available complexity labels for filtering.

**Response (200):**

```json
{
  "data": ["low", "medium", "high", "critical"]
}
```

---

## Log Endpoints

### List Log Entries

#### `GET /api/v1/logs`

Returns a paginated, filterable list of log entries.

**Query parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `scope` | string | Filter by log scope: `collection_run`, `git_operation`, `test_kitchen_run`, `cookstyle_scan` |
| `organisation` | string | Filter by organisation name |
| `cookbook_name` | string | Filter by cookbook name |
| `severity` | string | Minimum severity: `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `from` | ISO-8601 datetime | Start of time range |
| `to` | ISO-8601 datetime | End of time range |
| `has_errors` | boolean | If `true`, return only entries with severity `ERROR` |
| `page` | integer | Page number |
| `per_page` | integer | Items per page |

**Response (200):**

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "timestamp": "2024-06-15T12:01:23Z",
      "severity": "ERROR",
      "scope": "collection_run",
      "message": "Failed to authenticate with Chef server for organisation myorg-staging",
      "organisation": "myorg-staging",
      "cookbook_name": null,
      "cookbook_version": null,
      "commit_sha": null,
      "chef_client_version": null,
      "has_process_output": false
    }
  ],
  "pagination": { ... }
}
```

### Get Log Entry Detail

#### `GET /api/v1/logs/:id`

Returns a single log entry including the full `process_output` field (which may be large and is therefore excluded from list responses).

**Response (200):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440001",
  "timestamp": "2024-06-15T14:35:12Z",
  "severity": "ERROR",
  "scope": "test_kitchen_run",
  "message": "Test Kitchen tests failed for cookbook nginx against Chef Client 19.0.0",
  "organisation": null,
  "cookbook_name": "nginx",
  "cookbook_version": null,
  "commit_sha": "a1b2c3d4e5f6",
  "chef_client_version": "19.0.0",
  "process_output": "-----> Starting Test Kitchen ...\n       ... (full stdout/stderr) ...\n>>>>>> Kitchen finished. 1 test, 1 failure"
}
```

### List Collection Runs

#### `GET /api/v1/logs/collection-runs`

Returns a summary list of collection job runs with their status and log entry counts.

**Query parameters:** `organisation`, `from`, `to`, pagination.

**Response (200):**

```json
{
  "data": [
    {
      "run_id": "run-20240615-120000",
      "organisation": "myorg-production",
      "started_at": "2024-06-15T12:00:00Z",
      "completed_at": "2024-06-15T12:03:45Z",
      "status": "success",
      "nodes_collected": 2000,
      "log_entry_count": 15,
      "error_count": 0
    },
    {
      "run_id": "run-20240615-120000",
      "organisation": "myorg-staging",
      "started_at": "2024-06-15T12:00:00Z",
      "completed_at": "2024-06-15T12:01:12Z",
      "status": "failed",
      "nodes_collected": 0,
      "log_entry_count": 3,
      "error_count": 1
    }
  ],
  "pagination": { ... }
}
```

---

## Admin Endpoints

All endpoints in this section require the `admin` role.

### Credential Management

These endpoints manage encrypted credentials stored in the database. Credentials are used for Chef API private keys, LDAP bind passwords, SMTP passwords, and webhook URLs. All credential values are encrypted at the application layer using AES-256-GCM before storage — see the [Datastore Specification](../datastore/Specification.md) for the encryption model.

> **Security principles:**
> - The API **never** returns the plaintext or encrypted value of a credential in any response.
> - Credential values can be created and replaced but never read back.
> - A "test" endpoint validates that a credential works without revealing its value.
> - All credential operations are logged at `INFO` severity with the credential name, type, and acting user — but never the value.

#### `GET /api/v1/admin/credentials`

Lists all stored credentials (metadata only, never values).

**Query parameters:**

| Parameter | Description |
|-----------|-------------|
| `type` | Filter by `credential_type` (e.g. `chef_client_key`, `ldap_bind_password`) |

**Response (200):**

```json
{
  "data": [
    {
      "name": "myorg-production-key",
      "credential_type": "chef_client_key",
      "metadata": { "key_format": "pkcs1", "bits": 2048 },
      "referenced_by": ["organisation:myorg-production"],
      "last_rotated_at": "2024-06-01T10:00:00Z",
      "created_by": "alice",
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-06-01T10:00:00Z"
    },
    {
      "name": "ldap-bind-password",
      "credential_type": "ldap_bind_password",
      "metadata": { "host": "ldap.example.com" },
      "referenced_by": ["config:auth.providers[1].bind_password_credential"],
      "last_rotated_at": null,
      "created_by": "alice",
      "created_at": "2024-01-15T10:00:00Z",
      "updated_at": "2024-01-15T10:00:00Z"
    }
  ]
}
```

| Field | Description |
|-------|-------------|
| `referenced_by` | List of entities that reference this credential. Helps identify impact before rotation or deletion. Format: `organisation:<name>` for Chef keys, `config:<path>` for YAML config references. |
| `last_rotated_at` | When the credential value was last changed (null if never rotated since creation). |

#### `POST /api/v1/admin/credentials`

Creates a new encrypted credential.

**Request:**

```json
{
  "name": "myorg-staging-key",
  "credential_type": "chef_client_key",
  "value": "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----\n",
  "metadata": { "key_format": "pkcs1", "bits": 2048 }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique name for this credential |
| `credential_type` | Yes | One of: `chef_client_key`, `ldap_bind_password`, `smtp_password`, `webhook_url`, `generic` |
| `value` | Yes | The plaintext credential value. Validated per type (e.g. RSA key must be parseable, URL must be valid). Encrypted before storage. **Never logged.** |
| `metadata` | No | Non-sensitive metadata object. Must not contain the credential value. |

**Response (201):**

```json
{
  "name": "myorg-staging-key",
  "credential_type": "chef_client_key",
  "metadata": { "key_format": "pkcs1", "bits": 2048 },
  "created_by": "alice",
  "created_at": "2024-06-15T16:00:00Z"
}
```

The `value` field is **never** included in the response.

**Validation per credential type:**

| `credential_type` | Validation |
|--------------------|------------|
| `chef_client_key` | Must be a PEM-encoded RSA private key. Parsed to extract key size for metadata. |
| `ldap_bind_password` | Non-empty string. |
| `smtp_password` | Non-empty string. |
| `webhook_url` | Must be a valid URL with `http` or `https` scheme. |
| `generic` | Non-empty string. No format validation. |

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | Value fails type-specific validation (e.g. invalid PEM) |
| `409` | A credential with this name already exists |
| `422` | Required fields missing or `credential_type` is not a recognised value |
| `503` | Credential encryption key (`CMM_CREDENTIAL_ENCRYPTION_KEY`) is not configured |

#### `PUT /api/v1/admin/credentials/:name`

Rotates (replaces) the value of an existing credential. The new value is encrypted and stored; the old ciphertext is overwritten.

**Request:**

```json
{
  "value": "-----BEGIN RSA PRIVATE KEY-----\nMIIE...(new key)...\n-----END RSA PRIVATE KEY-----\n",
  "metadata": { "key_format": "pkcs1", "bits": 4096 }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `value` | Yes | The new plaintext credential value. Validated and encrypted before storage. |
| `metadata` | No | Updated metadata. If omitted, existing metadata is preserved. |

**Response (200):**

```json
{
  "name": "myorg-staging-key",
  "credential_type": "chef_client_key",
  "metadata": { "key_format": "pkcs1", "bits": 4096 },
  "last_rotated_at": "2024-06-15T17:00:00Z",
  "updated_by": "alice",
  "updated_at": "2024-06-15T17:00:00Z"
}
```

The `value` field is **never** included in the response.

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | Value fails type-specific validation |
| `404` | Credential not found |
| `503` | Credential encryption key not configured |

> **Note:** The `credential_type` cannot be changed after creation. To change the type, delete and re-create the credential.

#### `DELETE /api/v1/admin/credentials/:name`

Deletes a credential. The encrypted value is permanently removed from the database.

**Request query parameters:**

| Parameter | Required | Description |
|-----------|----------|-------------|
| `confirm` | Yes | Must be `true`. Prevents accidental deletion. |

**Response (204):** No content.

**Errors:**

| Status | Condition |
|--------|-----------|
| `400` | `confirm=true` not provided |
| `404` | Credential not found |
| `409` | Credential is still referenced by one or more organisations or config entries. The response body lists the references. Must unlink before deleting. |

**Response (409):**

```json
{
  "error": "conflict",
  "message": "Credential is still referenced and cannot be deleted",
  "referenced_by": ["organisation:myorg-staging"]
}
```

#### `POST /api/v1/admin/credentials/:name/test`

Tests a stored credential by performing a lightweight validation appropriate to its type. The credential is decrypted in memory for the duration of the test and then discarded.

| `credential_type` | Test action |
|--------------------|------------|
| `chef_client_key` | Parse the RSA key and verify it can produce a valid signature. If linked to an organisation, optionally make a test API call to the Chef server. |
| `ldap_bind_password` | Attempt an LDAP bind using the configured LDAP settings and this password. |
| `smtp_password` | Attempt an SMTP `AUTH` handshake using the configured SMTP settings and this password. |
| `webhook_url` | Send an HTTP `HEAD` request to the URL and verify a 2xx or 3xx response. |
| `generic` | Verify the credential can be decrypted (confirms master key is correct). |

**Response (200):**

```json
{
  "name": "myorg-staging-key",
  "credential_type": "chef_client_key",
  "status": "ok",
  "message": "RSA key is valid (2048-bit). Chef server authentication succeeded."
}
```

**Response (200, failure):**

```json
{
  "name": "ldap-bind-password",
  "credential_type": "ldap_bind_password",
  "status": "error",
  "message": "LDAP bind failed: invalid credentials"
}
```

> Like the organisation test endpoint, this always returns HTTP 200 — the `status` field indicates whether the credential is valid.

---

### User Management

#### `GET /api/v1/admin/users`

Returns a list of local user accounts.

**Response (200):**

```json
{
  "data": [
    {
      "username": "alice",
      "display_name": "Alice Smith",
      "email": "alice@example.com",
      "role": "admin",
      "provider": "local",
      "locked": false,
      "created_at": "2024-01-15T10:00:00Z",
      "last_login_at": "2024-06-15T08:30:00Z"
    }
  ],
  "pagination": { ... }
}
```

#### `POST /api/v1/admin/users`

Creates a new local user account.

**Request:**

```json
{
  "username": "bob",
  "display_name": "Bob Jones",
  "email": "bob@example.com",
  "password": "s3cur3Pa$$w0rd",
  "role": "viewer"
}
```

**Response (201):**

```json
{
  "username": "bob",
  "display_name": "Bob Jones",
  "email": "bob@example.com",
  "role": "viewer",
  "provider": "local",
  "locked": false,
  "created_at": "2024-06-15T16:00:00Z"
}
```

**Errors:**

| Status | Condition |
|--------|-----------|
| `409` | Username already exists |
| `422` | Validation error (password too short, invalid role, etc.) |

#### `PUT /api/v1/admin/users/:username`

Updates an existing user account (display name, email, role, locked status). Password changes use a separate endpoint.

**Request:**

```json
{
  "display_name": "Robert Jones",
  "role": "admin",
  "locked": false
}
```

**Response (200):** Updated user object.

#### `PUT /api/v1/admin/users/:username/password`

Resets a user's password.

**Request:**

```json
{
  "password": "newS3cur3Pa$$w0rd"
}
```

**Response (204):** No content.

#### `DELETE /api/v1/admin/users/:username`

Deletes a local user account. LDAP and SAML users cannot be deleted via this endpoint.

**Response (204):** No content.

### System Status

#### `GET /api/v1/admin/status`

Returns the system health status including datastore connectivity, credential encryption status, last collection run times, and pending job information.

**Response (200):**

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "datastore": {
    "status": "connected",
    "pending_migrations": 0
  },
  "credential_storage": {
    "encryption_key_configured": true,
    "total_credentials": 5,
    "credential_types": {
      "chef_client_key": 3,
      "ldap_bind_password": 1,
      "smtp_password": 1
    },
    "orphaned_credentials": 0
  },
  "collection": {
    "next_run_at": "2024-06-15T13:00:00Z",
    "last_run_at": "2024-06-15T12:00:00Z",
    "last_run_status": "success"
  },
  "organisations": [
    {
      "name": "myorg-production",
      "credential_source": "file",
      "last_collected_at": "2024-06-15T12:00:00Z",
      "status": "success",
      "node_count": 2000
    },
    {
      "name": "myorg-staging",
      "credential_source": "database",
      "last_collected_at": "2024-06-15T12:00:00Z",
      "status": "success",
      "node_count": 500
    }
  ]
}
```

| Field | Description |
|-------|-------------|
| `credential_storage.encryption_key_configured` | Whether `CMM_CREDENTIAL_ENCRYPTION_KEY` is set and valid. If `false`, database credential operations are unavailable. |
| `credential_storage.total_credentials` | Total number of credentials in the database. |
| `credential_storage.credential_types` | Breakdown by `credential_type`. |
| `credential_storage.orphaned_credentials` | Credentials not referenced by any organisation or config. May be candidates for cleanup. |

---

## WebSocket Real-Time Events

### Overview

The API provides a WebSocket endpoint for pushing real-time event notifications to connected dashboard clients. This eliminates the need for polling on views that benefit from live updates (dashboard summaries, log viewer, export progress).

The WebSocket channel carries **lightweight event notifications only** — it does not duplicate query logic. When a client receives an event, it re-fetches the relevant REST endpoint to get the updated data. This keeps the WebSocket layer thin and the REST API as the single source of truth.

### Endpoint

#### `GET /api/v1/ws`

Upgrades the HTTP connection to a WebSocket connection.

**Authentication:** The session token must be provided as a query parameter (`?token=<session-token>`) or via the `session` cookie. The token is validated during the HTTP upgrade handshake. If the token is missing, expired, or invalid, the server rejects the upgrade with `401 Unauthorized`.

**Protocol:** The server uses JSON text frames. Binary frames are not used.

**Subprotocol:** None required. Clients should not request a subprotocol.

### Connection Lifecycle

1. The client opens a WebSocket connection to `/api/v1/ws?token=<session-token>`.
2. The server validates the session and upgrades the connection.
3. The server sends a `connected` event to confirm the session is active.
4. The server pushes events as they occur. The client does not send application-level messages (the channel is server-to-client only).
5. The client should send WebSocket ping frames periodically (recommended: every 30 seconds) to keep the connection alive. The server responds with pong frames automatically.
6. The server sends a WebSocket close frame if the session expires or the server is shutting down.
7. The client should implement automatic reconnection with exponential backoff (recommended: 1s, 2s, 4s, 8s, max 30s).

### Event Envelope

All events use a consistent JSON envelope:

```json
{
  "event": "collection_complete",
  "timestamp": "2024-06-15T14:30:00Z",
  "data": { ... }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `event` | string | Event type identifier (see table below) |
| `timestamp` | ISO-8601 string | When the event occurred |
| `data` | object | Event-specific payload (may be empty `{}` for simple signals) |

### Event Types

#### Connection Events

| Event | Trigger | Data |
|-------|---------|------|
| `connected` | WebSocket connection established | `{ "session_expires_at": "..." }` |

#### Collection Events

| Event | Trigger | Data |
|-------|---------|------|
| `collection_started` | A collection run begins for an organisation | `{ "organisation": "myorg", "run_id": "..." }` |
| `collection_progress` | Periodic progress update during collection | `{ "organisation": "myorg", "run_id": "...", "nodes_collected": 150, "total_nodes": 2500 }` |
| `collection_complete` | A collection run completes successfully | `{ "organisation": "myorg", "run_id": "...", "nodes_collected": 2500 }` |
| `collection_failed` | A collection run fails | `{ "organisation": "myorg", "run_id": "...", "error": "..." }` |

#### Analysis Events

| Event | Trigger | Data |
|-------|---------|------|
| `cookbook_status_changed` | A cookbook's compatibility status changed | `{ "cookbook_name": "nginx", "cookbook_version": "5.1.0", "target_chef_version": "19.0.0", "previous_status": "untested", "new_status": "compatible" }` |
| `readiness_updated` | Readiness counts changed after analysis | `{ "organisation": "myorg", "target_chef_version": "19.0.0" }` |
| `complexity_updated` | Complexity scores recalculated for cookbooks | `{ "cookbook_name": "nginx", "cookbook_version": "5.1.0" }` |
| `rescan_started` | A manual cookbook rescan was triggered | `{ "cookbook_name": "nginx", "cookbook_version": "5.1.0", "job_id": "..." }` |
| `rescan_complete` | A manual cookbook rescan finished | `{ "cookbook_name": "nginx", "cookbook_version": "5.1.0", "job_id": "..." }` |
| `git_repo_status_changed` | Git repo rescan/reset requested | `{ "git_repo_name": "nginx", "action": "rescan", "repos_affected": 1 }` |

#### Export Events

| Event | Trigger | Data |
|-------|---------|------|
| `export_started` | An async export job was created | `{ "job_id": "...", "export_type": "ready_nodes", "format": "csv" }` |
| `export_complete` | An export job finished and is ready for download | `{ "job_id": "...", "export_type": "ready_nodes", "format": "csv", "row_count": 1800, "download_url": "/api/v1/exports/.../download" }` |
| `export_failed` | An export job failed | `{ "job_id": "...", "error": "..." }` |

#### Log Events

| Event | Trigger | Data |
|-------|---------|------|
| `log_entry` | A new log entry was persisted (scoped to collection, analysis, or export) | `{ "id": "...", "severity": "ERROR", "scope": "collection", "message": "...", "organisation": "myorg" }` |

#### Notification Events

| Event | Trigger | Data |
|-------|---------|------|
| `notification_sent` | A notification was dispatched to a channel | `{ "id": "...", "channel_name": "slack-ops", "event_type": "cookbook_status_changed", "status": "sent" }` |
| `notification_failed` | A notification delivery failed | `{ "id": "...", "channel_name": "slack-ops", "event_type": "cookbook_status_changed", "status": "failed", "error": "..." }` |
| `ownership_assigned` | An ownership assignment was created | `{ "owner_name": "web-platform", "entity_type": "cookbook", "entity_key": "acme-web", "assignment_source": "manual" }` |
| `ownership_removed` | An ownership assignment was removed | `{ "owner_name": "web-platform", "entity_type": "cookbook", "entity_key": "acme-web" }` |
| `ownership_reassigned` | Assignments were bulk-reassigned between owners | `{ "from_owner": "old-team", "to_owner": "new-team", "reassigned": 47 }` |

### Server-Side Architecture

The WebSocket layer uses a **hub-and-spoke** pattern:

- A single `EventHub` goroutine manages all connected clients.
- When an application event occurs (collection completes, export finishes, etc.), the originating component calls `EventHub.Broadcast(event)`.
- The hub fans out the event to all connected client send channels.
- Each client connection has a dedicated write goroutine that drains its send channel and writes JSON frames to the WebSocket.
- If a client's send channel is full (slow consumer), the server closes the connection rather than blocking the hub. The client is expected to reconnect.

**Concurrency guarantees:**

- `Broadcast()` is safe to call from any goroutine.
- Client registration and deregistration are serialised through the hub's event loop.
- The hub does not block on slow clients — bounded send channels with drop-on-full semantics.

### Client Reconnection

Clients must handle disconnections gracefully:

- On WebSocket close or error, wait and reconnect with exponential backoff.
- On reconnection, re-fetch all visible REST endpoints to catch any events missed during the disconnection window.
- The server does not maintain event history or replay missed events. The REST API is the source of truth.

### Configuration

WebSocket behaviour is configured under the `server` key:

```yaml
server:
  websocket:
    enabled: true                  # Enable/disable WebSocket endpoint (default: true)
    max_connections: 100           # Maximum concurrent WebSocket connections (default: 100)
    send_buffer_size: 64           # Per-client send channel buffer size (default: 64)
    write_timeout_seconds: 10      # Timeout for writing a single frame (default: 10)
    ping_interval_seconds: 30      # Server-initiated ping interval (default: 30)
    pong_timeout_seconds: 60       # Time to wait for pong before closing (default: 60)
```

| Setting | Default | Description |
|---------|---------|-------------|
| `enabled` | `true` | Set to `false` to disable the WebSocket endpoint entirely. The REST API continues to function normally. |
| `max_connections` | `100` | Maximum number of simultaneous WebSocket connections. New connections are rejected with `503 Service Unavailable` when the limit is reached. |
| `send_buffer_size` | `64` | Size of each client's outbound event buffer. If the buffer is full, the client is disconnected (slow consumer protection). |
| `write_timeout_seconds` | `10` | Maximum time to write a single WebSocket frame before closing the connection. |
| `ping_interval_seconds` | `30` | How often the server sends ping frames to detect dead connections. |
| `pong_timeout_seconds` | `60` | How long the server waits for a pong response before closing the connection. Must be greater than `ping_interval_seconds`. |

### Dependency

The WebSocket implementation uses the `github.com/coder/websocket` package (pure Go, no C dependencies, actively maintained). This is the successor to the now-deprecated `nhooyr.io/websocket` — same author, same API, maintained by Coder. This is the only additional dependency required.

---

## Static Assets and Frontend

The web dashboard frontend is a single-page application (SPA) served by the Go backend. All routes not matching `/api/` are served from the embedded static assets directory, with a fallback to `index.html` for client-side routing.

```
GET /              → serves index.html
GET /dashboard     → serves index.html (client-side route)
GET /assets/*      → serves static files (JS, CSS, images)
GET /api/v1/*      → API endpoints (documented above)
GET /api/v1/ws     → WebSocket endpoint (documented above)
```

---

## Rate Limiting

- The login endpoint (`POST /api/v1/auth/login`) must be rate-limited per source IP to prevent brute-force attacks.
- API endpoints may optionally be rate-limited per authenticated user to prevent abuse, but this is not required for the initial implementation.

---

## CORS

If the frontend is served from the same origin as the API (recommended), no CORS configuration is needed. If a separate frontend origin is used, the API must support configurable CORS headers:

- `Access-Control-Allow-Origin`
- `Access-Control-Allow-Methods`
- `Access-Control-Allow-Headers`
- `Access-Control-Allow-Credentials`

---

## Related Specifications

- [Top-level Specification](../Specification.md)
- [Authentication and Authorisation](../auth/Specification.md)
- [Ownership](../ownership/Specification.md) — owner management, assignments, bulk reassignment, audit log, committer workflows
- [Visualisation](../visualisation/Specification.md)
- [Logging](../logging/Specification.md)
- [Configuration](../configuration/Specification.md) — credential encryption key, `client_key_credential` and `bind_password_credential` settings
- [Datastore](../datastore/Specification.md) — `credentials` table encryption model, `organisations` table credential FK
- [Chef API](../chef-api/Specification.md) — credentials security requirements for API signing
- [Analysis](../analysis/Specification.md) — for remediation guidance, complexity scoring, and auto-correct preview details
- [Data Collection](../data-collection/Specification.md) — for Policyfile support and dependency graph collection