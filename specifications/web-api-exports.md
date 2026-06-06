# Web API — Export Endpoints

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
