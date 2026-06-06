# Web API — Dashboard Endpoints

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
