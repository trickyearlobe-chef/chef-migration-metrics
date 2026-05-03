# Diagnostic Bundle — Component Specification

## TL;DR

Admin-only endpoint that assembles a ZIP archive of diagnostic data from the running instance and streams it as a download. Intended for capturing a point-in-time snapshot from a customer environment and transferring it to a development environment for offline analysis.

---

## Endpoint

```
GET /api/v1/admin/diagnostic-bundle
```

- **Auth:** admin role required. Returns `503` when auth is not configured (bundle would be publicly accessible).
- **Response:** `application/zip` with `Content-Disposition: attachment; filename="cmm-diagnostic-<YYYYMMDD-HHMMSS>.zip"` and `Cache-Control: no-store`.
- **Error handling:** each source is collected under a 5-second timeout. If a source fails or times out, its file is omitted and an entry is added to `errors.json`. The bundle is always returned (even if all DB sources fail, the in-memory sources are still useful).

### Query Parameters

| Parameter | Default | Description |
|---|---|---|
| `log_days` | `7` | Days of log history to include (1–30) |
| `include_identifiers` | `false` | When `true`, include org names, cookbook names, role names, git repo names |
| `include_depth_stats` | `false` | When `true`, run recursive dep-depth queries (expensive — avoid on stressed systems) |

---

## ZIP Contents

### Always included

| File | Source | Notes |
|---|---|---|
| `bundle_info.json` | in-memory | timestamp, app version, hostname |
| `config_summary.json` | in-memory `r.cfg` | see Allowlist below |
| `performance.json` | in-memory perf recorder | per-endpoint P50/P95/P99 latency |
| `system_health.json` | OS syscalls + DB (best-effort) | CPU, mem, disk, Go heap/goroutines; DB size added if reachable |
| `migrations.json` | `schema_migrations` table | version, name, applied_at for all rows |
| `organisations.json` | `ListOrganisations` | counts only by default; names added with `?include_identifiers=true` |
| `collection_run_status.json` | `ListCollectionRunsFiltered` | current run status per org |
| `performance_db.json` | `pg_stat_statements`, index stats, table stats | active queries omitted (may contain sensitive query text) |
| `logs_recent.json` | `ListLogEntries` for last `log_days` days, max 5000 rows | `process_output` field omitted |
| `logs_errors.json` | `ListLogEntries` severity=ERROR for last `log_days` days, max 1000 rows | `process_output` field omitted |
| `inventory_stats.json` | counts per org from DB | see Inventory Stats below |
| `platform_distribution.json` | `CountNodePlatformDistribution` resolved with `DefaultMappings` only | see Platform Distribution below |

### Included with `?include_depth_stats=true`

| File | Source | Notes |
|---|---|---|
| `dependency_depth_stats.json` | recursive CTEs on `role_dependencies` + cookbook JSONB | see Dependency Depth Stats below |

---

## Inventory Stats

`inventory_stats.json` contains per-org counts for all key entity types. When `?include_identifiers=true`, names are added alongside counts.

```json
{
  "nodes_by_org":            { "org-a": 1234 },
  "cookbooks_by_org":        { "org-a": 89 },
  "roles_by_org":            { "org-a": 45 },
  "role_dep_edges_by_org":   { "org-a": 312 },
  "git_repo_count":          67,
  "cookbook_names_by_org":   { "org-a": ["nginx", "apt"] },  // only with include_identifiers
  "role_names_by_org":       { "org-a": ["base", "web"] },   // only with include_identifiers
  "git_repo_names":          ["repo-a", "repo-b"]            // only with include_identifiers
}
```

Node names are never included even with `?include_identifiers=true`.

---

## Platform Distribution

`platform_distribution.json` contains per-platform node counts resolved through the built-in display name mappings only (admin-configured mappings are excluded to avoid leaking customer-specific identifiers).

```json
{
  "total_nodes": 1234,
  "distribution": [
    {
      "platform": "windows",
      "version": "10.0.20348",
      "display_name": "Win Server 2022",
      "group_key": "windows:windows-server-2022",
      "group_display_name": "Windows Server 2022",
      "count": 500
    }
  ]
}
```

Entries are sorted by count descending. The `group_key` and `group_display_name` fields use the same centralized resolver as the dashboard.

---

## Dependency Depth Stats

Only collected when `?include_depth_stats=true`. Uses recursive CTEs — can be slow on large graphs.

```json
{
  "role_dep_depth_by_org": {
    "org-a": {
      "max_depth": 8,
      "avg_depth": 2.3,
      "distribution": { "0": 12, "1-5": 28, "6-10": 4, "11+": 1 }
    }
  },
  "deepest_roles": [                // only with include_identifiers
    { "org": "org-a", "role": "base", "depth": 8 }
  ],
  "cookbook_dep_depth_by_org": {
    "org-a": { "max_depth": 5, "avg_depth": 1.8 }
  }
}
```

Each depth query runs under its own 10-second timeout (longer than standard 5s, but still bounded).

---

## Config Allowlist

`config_summary.json` is produced by a pure function `DiagnosticConfigSummary(cfg config.Config) map[string]any` using an **explicit allowlist** (not a denylist). Only the following safe fields are included:

- `target_chef_versions` — list of configured target versions
- `git_base_urls` — Git base URLs
- `collection` — all collection timing/retry settings
- `concurrency` — all worker pool sizes
- `analysis_tools` — tool paths and settings, **excluding** `driver_secrets` values (keys only), `chef_license_key_credential`
- `readiness` — readiness thresholds
- `exports` — export settings (no webhook URLs or auth)
- `logging` — log level and retention days
- `server.bind_address`, `server.port`, `server.tls.mode`
- `system_health` — all thresholds
- `performance` — window seconds
- `organisation_count` — integer count of configured orgs (names and URLs omitted)

Everything not in this list is omitted, including: DB URL, Chef server URLs, key paths, credential references, passwords, webhook URLs, notification tokens, SMTP config, auth bind passwords, LDAP config, ACME config.

---

## Security Constraints

- The endpoint returns `503 Service Unavailable` when `authMiddleware == nil` (auth not configured).
- Response includes `Cache-Control: no-store`.
- Standard sources use a 5-second per-source timeout. Depth stat queries use 10 seconds.
- `process_output` is stripped from all log entries in the bundle.
- Active query text is omitted from `performance_db.json`.
- Without `?include_identifiers=true`: org names in DB results are replaced with opaque keys (`org-1`, `org-2`, etc.) keyed consistently within the bundle so files can be cross-referenced.

---

## New Datastore Methods

### `ListAppliedMigrations`

```go
ListAppliedMigrations(ctx context.Context) ([]AppliedMigration, error)

type AppliedMigration struct {
    Version   int       `json:"version"`
    Name      string    `json:"name"`
    AppliedAt time.Time `json:"applied_at"`
}
```

Returns all rows from `schema_migrations` ordered by `version ASC`.

### `InventoryStats`

```go
InventoryStats(ctx context.Context) (InventoryStats, error)

type InventoryStats struct {
    NodesByOrg         map[string]int    `json:"nodes_by_org"`
    CookbooksByOrg     map[string]int    `json:"cookbooks_by_org"`
    RolesByOrg         map[string]int    `json:"roles_by_org"`
    RoleDepEdgesByOrg  map[string]int    `json:"role_dep_edges_by_org"`
    GitRepoCount       int               `json:"git_repo_count"`
    CookbookNamesByOrg map[string][]string `json:"cookbook_names_by_org,omitempty"`
    RoleNamesByOrg     map[string][]string `json:"role_names_by_org,omitempty"`
    GitRepoNames       []string            `json:"git_repo_names,omitempty"`
}
```

The Names fields are populated only when `includeNames bool` is true.

### `DependencyDepthStats`

```go
DependencyDepthStats(ctx context.Context, includeNames bool) (DependencyDepthStats, error)

type OrgDepthStats struct {
    MaxDepth     int              `json:"max_depth"`
    AvgDepth     float64          `json:"avg_depth"`
    Distribution map[string]int   `json:"distribution"` // "0","1-5","6-10","11+"
}

type DependencyDepthStats struct {
    RoleDepDepthByOrg      map[string]OrgDepthStats `json:"role_dep_depth_by_org"`
    CookbookDepDepthByOrg  map[string]OrgDepthStats `json:"cookbook_dep_depth_by_org"`
    DeepestRoles           []DeepestRole            `json:"deepest_roles,omitempty"` // with includeNames only
}

type DeepestRole struct {
    Org   string `json:"org"`
    Role  string `json:"role"`
    Depth int    `json:"depth"`
}
```

All three methods are added to the `DataStore` interface and the mock.

---

## Frontend

A **Download Diagnostic Bundle** button on the Logs page header (admin-only). It opens `GET /api/v1/admin/diagnostic-bundle` as a direct link, triggering a browser download. A second **Download with Identifiers** option is available via a small dropdown on the button, linking to `?include_identifiers=true`.

---

## Related Specifications

- `web-api.md` — API conventions
- `logging.md` — log entry structure
- `system-health.md` — system health snapshot
- `performance-diagnostics.md` — performance stats
