# Web API — Export Endpoints

## Export Endpoints

### Request Export

#### `POST /api/v1/exports`

**Requires: `viewer` role (or higher).**

Requests an export of **the current filtered list** for one of the four list views.
There is one export type per list view. The export is **always streamed
synchronously** to the response as a file download — regardless of size — because
the encoder holds only one page in memory at a time (see Scale). There is no
asynchronous job path in the active flow; the `export_jobs` table and the
status/download endpoints below are dormant scaffolding reserved for a future
pipeline export (logstash / elasticsearch / observe).

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

**Scale.** A full unfiltered `nodes` export can exceed 120,000 rows. The handler
streams the filtered result set directly to the HTTP response — paging the shared
datastore query (keyset pagination for nodes) and writing each page straight to the
response body — so the full set is never held in memory and the `csv`/`json` row set
is complete rather than capped. The data comes from local Postgres, so the streamed
download completes quickly even at full scale.

**Export formats:**

| Format | Description |
|--------|-------------|
| `csv` | Comma-separated values (all list views) |
| `json` | JSON array of objects (all list views) |
| `chef_search_query` | Chef search query string — **`nodes` only** — `name:<node> OR …` for the node names in the current filtered set, for use with `knife ssh` / search |

**Response (200):** The export data is streamed directly with `Content-Type`
(`text/csv`, `application/json`, or `text/plain` for `chef_search_query`) and a
`Content-Disposition: attachment` filename (`<export_type>_<date>.<ext>`). This is
the only success response — there is no `202`/job path.

**Response (400):** Unknown `export_type` or `format`, or `chef_search_query`
requested for a non-`nodes` export.

### Reserved (future): async job endpoints

The `export_jobs` table and the endpoints below exist as dormant scaffolding for a
future async / pipeline export (logstash / elasticsearch / observe). They are **not
part of the active export flow** — `POST /api/v1/exports` never creates a job — and
are documented here only so the reservation is explicit. Do not build against them
until that feature is specified.

- `GET /api/v1/exports/:job_id` — job status
- `GET /api/v1/exports/:job_id/download` — completed-file download

---
