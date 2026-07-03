# Web API — Export Endpoints

## Export Endpoints

### Request Export

#### `POST /api/v1/exports`

**Requires: `viewer` role (or higher).**

Requests an export of **the current filtered list** for one of the four list views.
There is one export type per list view. Small exports are returned synchronously;
large exports (exceeding the configured `exports.async_threshold`) are processed
asynchronously.

**Filter parity (invariant).** The export MUST return exactly the set of entities the
corresponding list endpoint returns for the same filters. This is guaranteed by
construction: the export carries **the same query parameters** as the list endpoint
(e.g. `GET /api/v1/nodes?…`) and runs **the same datastore query**. A filter honoured
by the list can never be silently dropped by the export. `export_type` and `format`
are supplied as additional query parameters; every other parameter is the list view's
own filter/sort vocabulary (see [web-api.md](web-api.md) for each list endpoint's
parameters).

**Request** — the list view's query string plus `export_type` and `format`:

```
POST /api/v1/exports?export_type=nodes&format=csv&organisation=example-corp&environment=production&platform=ubuntu&readiness_filter=blocked&target_chef_version=19.0.0
```

**Export types (one per list view):**

| Type | List view | Query parameters |
|------|-----------|------------------|
| `nodes` | Nodes (`GET /api/v1/nodes`) | all node list filters (org, environment, platform, chef_version, role, policy, readiness/cookstyle/kitchen, migration/converge, stale, target_chef_version, …) |
| `cookbooks` | Server Cookbooks (`GET /api/v1/cookbooks`) | org, name, active, cookstyle_status, download_status, tk_status, target_chef_version |
| `roles` | Roles (`GET /api/v1/roles`) | org, name, compatibility_status, tk_status, target_chef_version |
| `git_repos` | Git Repos (`GET /api/v1/git-repos`) | name, compatibility, cookstyle_status, tk_status, clone_status, has_test_suite, target_chef_version |

Each export carries the list view's columns plus the migration-useful fields: the
three-state CookStyle rollup and readiness `status`, Test Kitchen status, and — for
`nodes` — the disk detail (`available_disk_mb`, `required_disk_mb`,
`sufficient_disk_space`, `install_path`). Row shapes are owned by the export Go types
(with their json tags) — not copied here.

The CookStyle vocabulary in export rows is canonical from
[cop-classification.md](cop-classification.md): per-item rollup is **Ready /
Needs review / Blocked / Untested**; node readiness is the three-state
`ready` / `needs_review` / `blocked`. Complexity scores are
**classification-weighted**. CS and TK stay separate signals — a row never
merges them into one verdict. Back-compat: rows retain any boolean `passed`/`ready`
field (`passed = status not in {Blocked}`) alongside `status` so existing downstream
automation keeps working; new consumers read `status`.

**Scale.** A full unfiltered `nodes` export can exceed 120,000 rows. Exports stream
the filtered result set (paging the shared datastore query and writing each page
straight to the output); the full set is never held in memory, and the `csv`/`json`
row set is complete rather than capped. Any safety ceiling that is ever hit is
reported (never a silent truncation).

**Export formats:**

| Format | Description |
|--------|-------------|
| `csv` | Comma-separated values (all list views) |
| `json` | JSON array of objects (all list views) |
| `chef_search_query` | Chef search query string — **`nodes` only** — `name:<node> OR …` for the node names in the current filtered set, for use with `knife ssh` / search |

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
  "export_type": "nodes",
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
