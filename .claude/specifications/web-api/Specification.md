# Web API - Component Specification

> **Implementation language:** Go. See `../../Claude.md` for language and concurrency rules.

> Component specification for the HTTP API layer of Chef Migration Metrics.
> See the [top-level specification](../Specification.md) for project overview and scope.

---

## TL;DR

RESTful JSON API (Go) between backend and React frontend. Mostly read-only over the datastore; write operations limited to admin actions (user management, manual rescan, auth provider config). Key endpoint groups: nodes, cookbooks, compatibility results, readiness, remediation, dependency graph, exports, notifications, logs, and admin. All list endpoints support pagination (`page`/`per_page`), filtering (org, environment, role, policy, platform, stale status, complexity label), and sorting. Auth via session cookie with RBAC middleware. CORS configurable. Export endpoints support sync (small) and async (large, returns job ID). Notification endpoints manage webhook/email channels and history. See `../auth/Specification.md` for auth details, `../datastore/Specification.md` for schema.

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
| `viewer` | Read access to all dashboard, log, and status endpoints |
| `admin` | All viewer permissions plus user management, manual rescan triggers, and configuration |

Endpoints that require `admin` are annotated below. All other authenticated endpoints require at minimum `viewer`.

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

Returns the compatibility status of each cookbook (and version) for each target Chef Client version.

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
| `compatible` | `high` | Test Kitchen converge and tests passed at HEAD |
| `incompatible` | N/A | Test Kitchen converge or tests failed at HEAD |
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

## Cookbook Endpoints

### List Cookbooks

#### `GET /api/v1/cookbooks`

Returns a paginated list of all known cookbooks with usage summary.

**Query parameters:** standard filters (including `cookbook_status`), pagination, sorting.

**Sortable fields:** `name`, `version`, `source`, `node_count`, `active`.

**Response (200):**

```json
{
  "data": [
    {
      "cookbook_name": "nginx",
      "versions": [
        {
          "version": "5.1.0",
          "source": "git",
          "organisation": null,
          "node_count": 1200,
          "active": true,
          "has_test_suite": true,
          "commit_sha": "a1b2c3d4e5f6",
          "last_tested_at": "2024-06-15T14:30:00Z"
        },
        {
          "version": "4.0.0",
          "source": "chef_server",
          "organisation": "myorg-staging",
          "node_count": 5,
          "active": true,
          "has_test_suite": false,
          "last_scanned_at": "2024-06-15T15:00:00Z"
        }
      ],
      "total_node_count": 1205
    }
  ],
  "pagination": { ... }
}
```

### Get Cookbook Detail

#### `GET /api/v1/cookbooks/:name`

Returns full detail for a specific cookbook across all versions and sources, including test history, CookStyle results, remediation guidance, complexity scores, and Policyfile references.

**Response (200):**

```json
{
  "cookbook_name": "nginx",
  "is_stale_cookbook": false,
  "first_seen_at": "2024-01-15T10:00:00Z",
  "versions": [
    {
      "version": "5.1.0",
      "source": "git",
      "organisation": null,
      "node_count": 1200,
      "active": true,
      "has_test_suite": true,
      "commit_sha": "a1b2c3d4e5f6",
      "default_branch": "main",
      "test_results": [
        {
          "target_chef_version": "18.5.0",
          "converge_passed": true,
          "tests_passed": true,
          "commit_sha": "a1b2c3d4e5f6",
          "tested_at": "2024-06-15T14:30:00Z"
        },
        {
          "target_chef_version": "19.0.0",
          "converge_passed": true,
          "tests_passed": false,
          "commit_sha": "a1b2c3d4e5f6",
          "tested_at": "2024-06-15T14:35:00Z"
        }
      ],
      "complexity": [
        {
          "target_chef_version": "19.0.0",
          "score": 30,
          "label": "medium",
          "affected_node_count": 1200,
          "affected_role_count": 3,
          "affected_policy_count": 0
        }
      ]
    },
    {
      "version": "4.0.0",
      "source": "chef_server",
      "organisation": "myorg-staging",
      "node_count": 5,
      "active": true,
      "has_test_suite": false,
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
          "affected_node_count": 5,
          "affected_role_count": 1,
          "affected_policy_count": 0
        }
      ]
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

#### `POST /api/v1/cookbooks/:name/:version/rescan`

**Requires: `admin` role.**

Triggers a re-download and re-analysis of a specific cookbook version. Used for exceptional cases such as data corruption or tooling bugs.

**Request body (optional):**

```json
{
  "organisation": "myorg-production"
}
```

If `organisation` is provided, only that organisation's copy is rescanned. If omitted, all copies across all organisations are rescanned (for Chef server-sourced cookbooks) or the git-sourced version is re-tested.

**Response (202):**

```json
{
  "message": "Rescan queued for cookbook nginx version 5.1.0.",
  "job_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
}
```

---

## Remediation Endpoints

### Get Cookbook Remediation Detail

#### `GET /api/v1/cookbooks/:name/:version/remediation`

Returns the full remediation guidance for a specific cookbook version, including auto-correct preview, enriched deprecation offenses with migration documentation, and complexity score.

**Query parameters:** `organisation` (optional, for Chef server-sourced cookbooks), `target_chef_version` (optional, defaults to all configured targets).

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

Returns all incompatible and CookStyle-flagged cookbooks sorted by a priority score that combines complexity and blast radius. This powers the remediation guidance view in the dashboard.

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

## Static Assets and Frontend

The web dashboard frontend is a single-page application (SPA) served by the Go backend. All routes not matching `/api/` are served from the embedded static assets directory, with a fallback to `index.html` for client-side routing.

```
GET /              → serves index.html
GET /dashboard     → serves index.html (client-side route)
GET /assets/*      → serves static files (JS, CSS, images)
GET /api/v1/*      → API endpoints (documented above)
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
- [Visualisation](../visualisation/Specification.md)
- [Logging](../logging/Specification.md)
- [Configuration](../configuration/Specification.md) — credential encryption key, `client_key_credential` and `bind_password_credential` settings
- [Datastore](../datastore/Specification.md) — `credentials` table encryption model, `organisations` table credential FK
- [Chef API](../chef-api/Specification.md) — credentials security requirements for API signing
- [Analysis](../analysis/Specification.md) — for remediation guidance, complexity scoring, and auto-correct preview details
- [Data Collection](../data-collection/Specification.md) — for Policyfile support and dependency graph collection