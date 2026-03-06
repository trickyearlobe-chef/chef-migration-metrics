# Handler Implementation Progress

## Overview

27 routes across 8 handler groups implemented for the Chef Migration Metrics Web API.
`main.go` now uses `webapi.NewRouter` instead of a raw `http.ServeMux`.

## ✅ All Groups Complete

### Group 1: Organisations (wired) + Nodes (4 routes)

**Organisations** — `handle_organisations.go` already existed. Wired `handleOrganisations` and `handleOrganisationDetail` into `router.go`, replacing `handleNotImplemented`. Fixed a pre-existing `%%q` → `%q` bug in `handleOrganisationDetail`.

**Nodes** — `handle_nodes.go` + `handle_nodes_test.go`

| Route | Handler | Status |
|-------|---------|--------|
| `GET /api/v1/organisations` | `handleOrganisations` | ✅ Wired |
| `GET /api/v1/organisations/:name` | `handleOrganisationDetail` | ✅ Wired |
| `GET /api/v1/nodes` | `handleNodes` | ✅ Implemented |
| `GET /api/v1/nodes/:organisation/:name` | `handleNodeDetail` | ✅ Implemented |
| `GET /api/v1/nodes/by-version/:chef_version` | `handleNodesByVersion` | ✅ Implemented |
| `GET /api/v1/nodes/by-cookbook/:cookbook_name` | `handleNodesByCookbook` | ✅ Implemented |

### Group 2: Cookbooks (2 routes) + Filters (7 routes)

**Cookbooks** — `handle_cookbooks.go` + `handle_cookbooks_test.go`

| Route | Handler | Status |
|-------|---------|--------|
| `GET /api/v1/cookbooks` | `handleCookbooks` | ✅ Implemented |
| `GET /api/v1/cookbooks/:name` | `handleCookbookDetail` | ✅ Implemented |

**Filters** — `handle_filters.go` + `handle_filters_test.go`

| Route | Handler | Status |
|-------|---------|--------|
| `GET /api/v1/filters/environments` | `handleFilterEnvironments` | ✅ Implemented |
| `GET /api/v1/filters/roles` | `handleFilterRoles` | ✅ Implemented |
| `GET /api/v1/filters/policy-names` | `handleFilterPolicyNames` | ✅ Implemented |
| `GET /api/v1/filters/policy-groups` | `handleFilterPolicyGroups` | ✅ Implemented |
| `GET /api/v1/filters/platforms` | `handleFilterPlatforms` | ✅ Implemented |
| `GET /api/v1/filters/target-chef-versions` | `handleFilterTargetChefVersions` | ✅ Implemented |
| `GET /api/v1/filters/complexity-labels` | `handleFilterComplexityLabels` | ✅ Implemented |

### Group 3: Logs (3 routes)

**Logs** — `handle_logs.go` + `handle_logs_test.go`

| Route | Handler | Status |
|-------|---------|--------|
| `GET /api/v1/logs` | `handleLogs` | ✅ Implemented |
| `GET /api/v1/logs/:id` | `handleLogDetail` | ✅ Implemented |
| `GET /api/v1/logs/collection-runs` | `handleCollectionRuns` | ✅ Implemented |

### Group 4: Dashboard (5 routes)

**Dashboard** — `handle_dashboard.go` + `handle_dashboard_test.go`

| Route | Handler | Status |
|-------|---------|--------|
| `GET /api/v1/dashboard/version-distribution` | `handleDashboardVersionDistribution` | ✅ Implemented |
| `GET /api/v1/dashboard/version-distribution/trend` | `handleDashboardVersionDistributionTrend` | ✅ Implemented |
| `GET /api/v1/dashboard/readiness` | `handleDashboardReadiness` | ✅ Implemented |
| `GET /api/v1/dashboard/readiness/trend` | `handleDashboardReadinessTrend` | ✅ Implemented |
| `GET /api/v1/dashboard/cookbook-compatibility` | `handleDashboardCookbookCompatibility` | ✅ Implemented |

### Group 5: Remediation (2 routes) — NEW

**Remediation** — `handle_remediation.go` + `handle_remediation_test.go`

| Route | Handler | Status |
|-------|---------|--------|
| `GET /api/v1/remediation/priority` | `handleRemediationPriority` | ✅ Implemented |
| `GET /api/v1/remediation/summary` | `handleRemediationSummary` | ✅ Implemented |

**Priority endpoint** returns cookbooks sorted by `complexity_score × affected_node_count` (priority score), with per-cookbook auto-correctable counts, manual fix counts, deprecation counts, and error counts. Supports `?organisation=`, `?target_chef_version=`, `?sort=`, `?order=`, and pagination. Defaults to descending sort for numeric fields.

**Summary endpoint** returns aggregate remediation metrics: total cookbooks evaluated, total needing remediation, quick wins (auto-correctable only), manual fixes, blocked nodes by complexity, blocked nodes by readiness. Supports `?organisation=` and `?target_chef_version=` filters.

Both endpoints share a `resolveOrganisationFilter` helper that accepts a repeatable `?organisation=` query parameter and falls back to all organisations when omitted.

### Group 6: Dependency Graph (2 routes) — NEW

**Dependency Graph** — `handle_dependency_graph.go` + `handle_dependency_graph_test.go`

| Route | Handler | Status |
|-------|---------|--------|
| `GET /api/v1/dependency-graph` | `handleDependencyGraph` | ✅ Implemented |
| `GET /api/v1/dependency-graph/table` | `handleDependencyGraphTable` | ✅ Implemented |

**Graph endpoint** returns `nodes` and `edges` arrays suitable for D3 force-directed or Cytoscape visualisation. Nodes have `id`, `name`, `type` ("role" or "cookbook"). Edges have `source`, `target`, `dependency_type`. Includes a `summary` object with `total_nodes`, `total_edges`, `role_count`, `cookbook_count`. Requires `?organisation=` parameter.

**Table endpoint** returns a flat paginated list of roles with `cookbook_count`, `role_count`, `total_dependencies`, `depended_on_by` (transitive reverse count), and inline `dependencies` array. Also returns `shared_cookbooks` (top 20 cookbooks used by ≥ 2 roles). Supports `?sort=`, `?order=`, and pagination. Defaults to descending sort for numeric fields.

### Wiring: main.go → webapi.Router — NEW

Replaced the hand-written `http.ServeMux` in `cmd/chef-migration-metrics/main.go` with `webapi.NewRouter(db, cfg, hub, ...)`. The manual `/api/v1/health`, `/api/v1/version`, and `/` handlers are now served by the Router's own implementations which include Content-Type headers, WebSocket client counts, and proper SPA fallback.

| Change | Detail |
|--------|--------|
| `webapi.NewEventHub()` + `go hub.Run()` | EventHub created and started in `main.go` |
| `webapi.NewRouter(db, cfg, hub, opts...)` | Replaces `http.NewServeMux()` |
| `webapi.WithVersion(version)` | Build-time version wired through |
| `webapi.WithLogger(fn)` | Logger callback bridges to `logging.ScopeWebAPI` |
| `srv.Handler = apiRouter` | `*http.Server` uses the Router directly |
| `logging.ScopeWebAPI` | New scope constant added to `internal/logging/logging.go` |

## Still Placeholder (Not Yet Implemented)

| Route Group | Routes | Notes |
|-------------|--------|-------|
| Auth | `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, `GET /api/v1/auth/me`, SAML endpoints | Requires auth package |
| Admin | `GET/POST /api/v1/admin/credentials`, `GET/POST /api/v1/admin/users`, `GET /api/v1/admin/status` | Requires RBAC |
| Exports | `GET/POST /api/v1/exports`, `GET /api/v1/exports/:id` | Requires export pipeline |
| Notifications | `GET/POST /api/v1/notifications`, `GET /api/v1/notifications/:id` | Requires notification pipeline |

## Files Created / Modified

| File | Action |
|------|--------|
| `cmd/chef-migration-metrics/main.go` | Modified — replaced manual mux with `webapi.NewRouter` |
| `internal/logging/logging.go` | Modified — added `ScopeWebAPI` constant and `validScopes` entry |
| `internal/webapi/router.go` | Modified — wired all 27 routes, replaced 4 `handleNotImplemented` with real handlers |
| `internal/webapi/store.go` | Modified — extended `DataStore` interface with 4 new methods |
| `internal/webapi/store_mock_test.go` | Modified — added 4 new mock function fields and methods |
| `internal/webapi/handle_remediation.go` | Created |
| `internal/webapi/handle_remediation_test.go` | Created |
| `internal/webapi/handle_dependency_graph.go` | Created |
| `internal/webapi/handle_dependency_graph_test.go` | Created |
| `internal/webapi/handle_organisations.go` | Fixed `%%q` bug (prior session) |
| `internal/webapi/handle_nodes.go` | Created (prior session) |
| `internal/webapi/handle_nodes_test.go` | Created (prior session) |
| `internal/webapi/handle_cookbooks.go` | Created (prior session) |
| `internal/webapi/handle_cookbooks_test.go` | Created (prior session) |
| `internal/webapi/handle_filters.go` | Created (prior session) |
| `internal/webapi/handle_filters_test.go` | Created (prior session) |
| `internal/webapi/handle_logs.go` | Created (prior session) |
| `internal/webapi/handle_logs_test.go` | Created (prior session) |
| `internal/webapi/handle_dashboard.go` | Created (prior session) |
| `internal/webapi/handle_dashboard_test.go` | Created (prior session) |
| `HANDLER_PROGRESS.md` | Updated (this file) |

## DataStore Interface Extensions

New methods added to `internal/webapi/store.go` (with corresponding `mockStore` stubs):

| Method | Used By |
|--------|---------|
| `ListCookbookComplexitiesForOrganisation(ctx, orgID)` | `handleRemediationPriority`, `handleRemediationSummary` |
| `ListRoleDependenciesByOrg(ctx, orgID)` | `handleDependencyGraph`, `handleDependencyGraphTable` |
| `CountDependenciesByRole(ctx, orgID)` | `handleDependencyGraphTable` |
| `CountRolesPerCookbook(ctx, orgID)` | `handleDependencyGraphTable` |

All four methods already existed on `*datastore.DB` — only the interface declaration and compile-time assertion were updated.

## Test Helpers

- `testConfig()` — builds a minimal `*config.Config` with WebSocket enabled.
- `testRouter()` — builds a `*Router` with nil DB for route-wiring / method-check tests.
- `newTestRouterWithMock(store)` — builds a `*Router` with mock DB and default config.
- `newTestRouterWithMockAndConfig(store, cfg)` — builds a `*Router` with mock DB and custom config.
- `resolveOrganisationFilter(req)` — shared helper returning filtered or all organisations.
- `isNotFound(err)` — helper checking for `datastore.ErrNotFound` or `"not found"` error strings.
- `filterNodes()` — pure function, fully unit-testable without DB.
- `nodeUsesCookbook()` — pure function, fully unit-testable without DB.
- `filterCookbooks()` — pure function, fully unit-testable without DB.

## Design Notes

- Handlers follow the existing pattern in `handle_organisations.go`: methods on `*Router`, call `r.db.<Method>()` directly.
- Remediation handlers aggregate complexity data from `ListCookbookComplexitiesForOrganisation` filtered by `target_chef_version` — this avoids a new query method.
- Priority score formula: `complexity_score × max(affected_node_count, 1)` — ensures unused cookbooks still appear.
- Dependency graph handler builds a deduplicated node/edge graph from `RoleDependency` rows; node IDs use `type:name` format.
- Dependency table handler computes transitive (reverse) counts by scanning all dependencies and counting how many roles reference each role.
- Shared cookbooks list is capped at 20 and only includes cookbooks used by ≥ 2 roles.
- Numeric sort fields default to descending order when no explicit `?order=` is provided; string fields default to ascending.
- `CountRolesPerCookbook` failure in the table handler is non-fatal — the response omits `shared_cookbooks` data but still returns 200.
- `/api/v1/dependency-graph/table` is registered before `/api/v1/dependency-graph` in the ServeMux so the longer path matches first.
- `/api/v1/logs/collection-runs` is registered before `/api/v1/logs/` so the ServeMux matches it first.
- Log handler validates `since`/`until` RFC3339 params before touching DB.
- Dashboard handlers aggregate across all organisations using existing datastore methods.

## All Tests Pass

```
go test -count=1 ./...
ok  github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis    0.605s
ok  github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi    10.002s
ok  github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector   1.660s
ok  github.com/trickyearlobe-chef/chef-migration-metrics/internal/config      1.292s
ok  github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore   1.045s
ok  github.com/trickyearlobe-chef/chef-migration-metrics/internal/embedded    1.732s
ok  github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging     1.975s
ok  github.com/trickyearlobe-chef/chef-migration-metrics/internal/remediation 1.551s
ok  github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets     2.894s
ok  github.com/trickyearlobe-chef/chef-migration-metrics/internal/webapi      2.575s
```
