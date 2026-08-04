# Web API — Log Endpoints

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
