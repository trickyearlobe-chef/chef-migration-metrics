# Web API — Dependency Graph Endpoints

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
