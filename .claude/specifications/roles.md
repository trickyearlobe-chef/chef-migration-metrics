# Roles — Component Specification

> Role compatibility list and role detail pages. Derives role-level upgrade readiness from transitive cookbook compatibility. Surfaces blast radius, blocking cookbooks, nested role chains, and scoped dependency graphs.

## Overview

Roles group cookbooks and other roles into reusable units assigned to nodes. A role's compatibility with a target Chef version is only as good as its weakest cookbook dependency. This component adds a top-level "Roles" section to the dashboard with a paginated list view and a detail page per role, plus three new API endpoints.

### Data Sources

- `role_dependencies` table — direct role→cookbook and role→role edges per organisation
- `node_snapshots.roles` — JSONB array of roles assigned to each node
- Cookbook compatibility results (server cookbooks + git repos) — used to derive per-role status
- Existing dependency graph endpoint (`GET /api/v1/dependency-graph`) — same graph format reused for scoped views

## Role Compatibility Page (List View)

Route: `/roles`

### Summary Bar

A stacked horizontal bar at the top showing the proportion of roles that are compatible, incompatible, and untested for the selected target Chef version. Same visual pattern as the cookbook compatibility card on the dashboard. Segments are clickable to pre-set the compatibility status filter.

### Table Columns

| Column | Description |
|--------|-------------|
| Name | Role name (link to detail page) |
| Organisation(s) | Comma-separated list of organisations where the role exists |
| Node Count | Number of nodes with this role in their `roles` array |
| Cookbook Count | Direct + transitive cookbook count (via nested role expansion) |
| Compatibility | Status badge per target Chef version |

### Compatibility Derivation

A role's compatibility status for a given target Chef version is derived, not stored:

- **Compatible** — every cookbook reachable through the role's dependency chain (direct cookbooks + cookbooks from nested roles, recursively) is compatible with the target version.
- **Incompatible** — at least one reachable cookbook is incompatible. This takes precedence over untested.
- **Untested** — no reachable cookbook is incompatible, but at least one has no test results.

Confidence follows the same high/medium model as cookbook compatibility (Test Kitchen > CookStyle-only).

### Filtering

| Filter | Type | Description |
|--------|------|-------------|
| `name` | text (type-ahead) | Substring match on role name |
| `organisation` | multi-select | Restrict to roles in selected organisations |
| `compatibility_status` | select | `compatible`, `incompatible`, `untested`, `all` (default: `all`) |
| `target_chef_version` | select | Target version for compatibility evaluation |

### Sorting

| Field | Description |
|-------|-------------|
| `name` | Alphabetical by role name (default) |
| `node_count` | Number of nodes using this role |
| `incompatible_cookbook_count` | Count of incompatible cookbooks in the transitive closure |

### Click-Through

Clicking a role name navigates to the role detail page.

## Role Detail Page

Route: `/roles/:name`

### Header

- Role name
- Organisation(s) where the role exists
- Total node count across all organisations

### Compatibility Summary

Per target Chef version, display:

| Status | Count | Meaning |
|--------|-------|---------|
| Ready | N cookbooks | All transitive cookbooks compatible |
| Blocked | N cookbooks | At least one transitive cookbook incompatible |
| Untested | N cookbooks | At least one transitive cookbook untested (none incompatible) |

Same colour coding as node readiness (green/red/grey).

### Blocking Cookbooks Section

Table of cookbooks that make this role incompatible for the selected target version:

| Column | Description |
|--------|-------------|
| Cookbook Name | Link to cookbook detail page |
| Version | Version on Chef Server |
| Complexity Score | Numeric score from analysis |
| Complexity Label | `low`, `medium`, `high`, `critical` |
| Auto-correctable | Count of auto-fixable offenses |
| Manual Fix | Count of manual-fix offenses |
| Path | Dependency chain showing how this cookbook is reached (e.g. `role:webserver → role:base → cookbook:apt`) |

Sorted by complexity score descending (hardest first) by default.

### Blast Radius Section

- **Node count** — how many nodes include this role, with a link to the nodes list filtered by this role (`/nodes?role=<name>`)
- **Breakdown by organisation** — node count per organisation
- **Breakdown by environment** — node count per Chef environment
- **Breakdown by platform** — node count per platform/version

### Dependency Graph Tab

An interactive force-directed graph scoped to this role, showing:

- The role itself as the root node
- Nested roles (recursively)
- Cookbooks referenced by each role (direct dependencies)
- Transitive cookbook dependencies resolved through nested roles
- **Cookbook→cookbook transitive dependencies** — each cookbook's own deps (from `server_cookbooks.dependencies` JSONB) are expanded recursively, so the graph includes the full transitive closure of all reachable cookbooks

Cookbook→cookbook edges use type `depends_on` to distinguish them from role→cookbook edges (`includes_cookbook`) and role→role edges (`includes_role`).

Multiple active versions of the same cookbook have their dep sets unioned. Cycles are handled with a visited guard; depth is capped at 50 to prevent runaway expansion on malformed data.

### Nested Role Chain

If this role includes other roles (directly or transitively), display the full expansion tree. **Cookbook→cookbook deps are expanded recursively**, so each cookbook node may itself have children:

```
role:webserver
├── role:base
│   ├── cookbook:base
│   └── cookbook:apt
│       └── cookbook:compat_resource   ← cookbook dep of apt
├── cookbook:nginx
│   └── cookbook:apt                   ← shared dep (not re-expanded)
└── cookbook:ssl
```

Each node in the tree is a link to the corresponding role or cookbook detail page. Incompatible cookbooks are visually highlighted (red text or icon). Untested cookbooks are shown in grey.

## API Endpoints

### `GET /api/v1/roles`

Paginated list of roles with derived compatibility status.

**Query parameters:** `name` (substring match), `organisation`, `compatibility_status` (`compatible`, `incompatible`, `untested`, `all`), `target_chef_version`, pagination (`page`, `per_page`), sorting (`sort`, `order`).

**Sortable fields:** `name`, `node_count`, `incompatible_cookbook_count`.

**Response (200):**

```json
{
  "data": [
    {
      "role_name": "webserver",
      "organisations": ["myorg-production", "myorg-staging"],
      "node_count": 500,
      "direct_cookbook_count": 3,
      "transitive_cookbook_count": 7,
      "total_cookbook_count": 7,
      "compatibility": [
        {
          "target_chef_version": "18.5.0",
          "status": "compatible",
          "compatible_count": 7,
          "incompatible_count": 0,
          "untested_count": 0
        },
        {
          "target_chef_version": "19.0.0",
          "status": "incompatible",
          "compatible_count": 5,
          "incompatible_count": 2,
          "untested_count": 0
        }
      ]
    }
  ],
  "summary": {
    "target_chef_version": "19.0.0",
    "compatible_roles": 12,
    "incompatible_roles": 8,
    "untested_roles": 3,
    "total_roles": 23
  },
  "pagination": {
    "page": 1,
    "per_page": 50,
    "total_items": 23,
    "total_pages": 1
  }
}
```

The `summary` object powers the stacked compatibility bar. It reflects the currently selected `target_chef_version` filter (or the default target version if none specified).

### `GET /api/v1/roles/:name`

Role detail with dependencies, compatibility, and blast radius.

**Query parameters:** `target_chef_version` (optional — used to select which version's blocking cookbooks to return).

**Response (200):**

```json
{
  "role_name": "webserver",
  "organisations": ["myorg-production", "myorg-staging"],
  "node_count": 500,
  "compatibility": [
    {
      "target_chef_version": "18.5.0",
      "status": "compatible",
      "compatible_count": 7,
      "incompatible_count": 0,
      "untested_count": 0
    },
    {
      "target_chef_version": "19.0.0",
      "status": "incompatible",
      "compatible_count": 5,
      "incompatible_count": 2,
      "untested_count": 0
    }
  ],
  "blocking_cookbooks": [
    {
      "cookbook_name": "nginx",
      "cookbook_version": "5.1.0",
      "target_chef_version": "19.0.0",
      "complexity_score": 30,
      "complexity_label": "medium",
      "auto_correctable": 4,
      "manual_fix": 3,
      "dependency_path": ["role:webserver", "cookbook:nginx"]
    },
    {
      "cookbook_name": "legacy-helpers",
      "cookbook_version": "1.0.0",
      "target_chef_version": "19.0.0",
      "complexity_score": 85,
      "complexity_label": "critical",
      "auto_correctable": 0,
      "manual_fix": 12,
      "dependency_path": ["role:webserver", "role:base", "cookbook:legacy-helpers"]
    }
  ],
  "direct_cookbooks": ["nginx", "ssl"],
  "direct_roles": ["base"],
  "transitive_cookbooks": ["nginx", "ssl", "base", "apt", "legacy-helpers", "compat_resource", "yum"],
  "nested_role_chain": {
    "name": "webserver",
    "type": "role",
    "children": [
      {
        "name": "base",
        "type": "role",
        "children": [
          { "name": "base", "type": "cookbook", "compatibility_status": "compatible" },
          { "name": "apt", "type": "cookbook", "compatibility_status": "compatible" },
          { "name": "legacy-helpers", "type": "cookbook", "compatibility_status": "incompatible" }
        ]
      },
      { "name": "nginx", "type": "cookbook", "compatibility_status": "incompatible" },
      { "name": "ssl", "type": "cookbook", "compatibility_status": "compatible" }
    ]
  },
  "nodes_by_organisation": [
    { "organisation": "myorg-production", "count": 480 },
    { "organisation": "myorg-staging", "count": 20 }
  ],
  "nodes_by_environment": [
    { "environment": "production", "count": 450 },
    { "environment": "staging", "count": 50 }
  ],
  "nodes_by_platform": [
    { "platform": "ubuntu", "platform_version": "22.04", "count": 400 },
    { "platform": "centos", "platform_version": "7", "count": 100 }
  ]
}
```

**Error (404):** Role not found — standard error envelope.

### `GET /api/v1/roles/:name/dependency-graph`

Returns the dependency graph scoped to a single role. Same response format as `GET /api/v1/dependency-graph`.

**Query parameters:** `organisation` (optional — scope to one org), `target_chef_version` (optional — used to set `compatibility_status` on cookbook nodes).

**Response (200):**

```json
{
  "nodes": [
    { "id": "role:webserver", "type": "role", "name": "webserver" },
    { "id": "role:base", "type": "role", "name": "base" },
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

The graph includes only nodes and edges reachable from the specified role (the transitive closure rooted at that role).

## Navigation

### Top-Level Nav

Add "Roles" as a top-level navigation item alongside the existing Nodes, Cookbooks, and Git Repos items. Position it after Cookbooks in the nav order.

### Inbound Links

The following existing views must link into the roles pages:

| Source View | Link Target | Context |
|-------------|------------|---------|
| Dependency graph — role node click | Role detail page | Clicking a role node in the graph |
| Node detail — blocking reasons | Role detail page | When a blocking cookbook is reached through a role |
| Cookbook detail — `nodes_by_role` section | Role detail page | Clicking a role name in the "used by roles" breakdown |
| Node list — roles column | Roles list filtered by name | Clicking a role badge on a node row |

### Outbound Links

| Source | Link Target | Context |
|--------|------------|---------|
| Role detail — blocking cookbook name | Cookbook detail page | Navigate to remediation guidance |
| Role detail — blast radius node count | Node list filtered by role | `/nodes?role=<name>` |
| Role detail — nested role in chain/graph | Role detail page for nested role | Navigate to nested role |
| Role list — role name | Role detail page | Primary click-through |

## Related Specifications

- [Web API](web-api.md) — existing endpoint patterns, pagination, filtering, error responses
- [Visualisation](visualisation.md) — dependency graph view, charting conventions, drill-down patterns
- [Data Collection](data-collection.md) § 5 — role dependency graph collection
- [Analysis](analysis.md) — cookbook complexity scoring, compatibility testing