# Data Visualisation — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Dashboard

- [x] Choose and set up web framework — React 18 + Vite + Tailwind CSS in `frontend/`; `AppLayout` with sidebar nav, `OrgSelector`, `HealthBadge`; React Router for client-side routing; Vite dev proxy to Go backend
- [x] Implement Chef Client version distribution view with trend over time — `DashboardPage.tsx` → `VersionDistributionCard` with CSS bar chart calling `fetchVersionDistribution()`; trend endpoint wired in backend (`/api/v1/dashboard/version-distribution/trend`) but trend chart not yet rendered in frontend
- [x] Implement cookbook compatibility status view (per cookbook, version, target Chef Client version) — `DashboardPage.tsx` → `CookbookCompatibilityCard` with stacked progress bars (compatible/incompatible/untested) calling `fetchCookbookCompatibility()`; `CookbookDetailPage.tsx` shows per-version CookStyle results and complexity scores
- [x] Implement confidence indicators — green for Test Kitchen pass (high), amber for CookStyle-only pass (medium), red for incompatible, grey for untested — `StatusBadge.tsx` with `compatible`/`cookstyle_only`/`incompatible`/`untested` variants, dot colour indicators, tooltips with confidence descriptions; `CompatibilityBadge` convenience wrapper
- [x] Implement cookbook complexity score display alongside compatibility status — `ComplexityBadge` component in `StatusBadge.tsx` with `low`/`medium`/`high`/`critical` colour coding and optional score number; used in `CookbookDetailPage.tsx` and `CookbookRemediationPage.tsx`
- [x] Implement stale cookbook indicator (badge/icon for cookbooks not updated in configured threshold) — `CookbookDetailPage.tsx` renders `StatusBadge variant="stale"` when `cb.is_stale_cookbook` is true; `CookbooksPage.tsx` shows active/inactive badges per cookbook
- [x] Implement node upgrade readiness summary (ready vs. blocked vs. stale counts) — `DashboardPage.tsx` → `ReadinessCard` with stacked progress bars (green=ready, red=blocked) per target Chef version; total node counts
- [x] Implement stale node indicators with last check-in age display — `StaleBadge` component with `isStale`/`ageHours` props, renders age as hours or days; used in `NodesPage.tsx` table rows and `NodeDetailPage.tsx` header
- [x] Implement per-node blocking reason detail view with complexity scores per blocking cookbook — `NodeDetailPage.tsx` → Upgrade Readiness section shows ready/blocked badge per target version, `blocking_reasons` list, and `blocking_cookbooks` as links to cookbook detail
- [x] Implement interactive filters:
  - [x] Filter by Chef server organisation — `OrgSelector` component in header, scopes all API calls via `useOrg()` context
  - [x] Filter by environment — `NodesPage.tsx` environment text filter passed as `?environment=` query param
  - [x] Filter by role — `NodesPage.tsx` role dropdown populated from `/api/v1/filters/roles`; backend `filterNodes` supports `?role=` query param with `nodeHasRole` JSON substring match
  - [x] Filter by Policyfile policy name — `NodesPage.tsx` policy name dropdown populated from `/api/v1/filters/policy-names`; backend `filterNodes` already supported `?policy_name=`
  - [x] Filter by Policyfile policy group — `NodesPage.tsx` policy group dropdown populated from `/api/v1/filters/policy-groups`; backend `filterNodes` already supported `?policy_group=`
  - [x] Filter by platform / platform version — `NodesPage.tsx` platform text filter passed as `?platform=` query param
  - [x] Filter by target Chef Client version — `RemediationPage.tsx` and `CookbookRemediationPage.tsx` both have target version selector dropdowns populated from `/api/v1/filters/target-chef-versions`
  - [x] Filter by active/unused cookbook status — `CookbooksPage.tsx` active/inactive select filter
  - [x] Filter by stale node status (all, stale, fresh) — `NodesPage.tsx` stale status select filter with all/stale/fresh options
  - [x] Filter by complexity label (low, medium, high, critical) — `RemediationPage.tsx` complexity dropdown populated from `/api/v1/filters/complexity-labels`; backend `handleRemediationPriority` supports `?complexity_label=` query param filtering after aggregation
- [x] Implement drill-down from summary to node detail — `NodesPage.tsx` links each row to `/nodes/:org/:name` → `NodeDetailPage.tsx`
- [x] Implement drill-down from summary to cookbook detail — `CookbooksPage.tsx` links each row to `/cookbooks/:name` → `CookbookDetailPage.tsx`
- [x] Implement drill-down from blocking cookbook to remediation guidance — `CookbookDetailPage.tsx` → "View Remediation Detail →" link per complexity entry → `/cookbooks/:name/:version/remediation` → `CookbookRemediationPage.tsx`; `NodeDetailPage.tsx` blocking cookbooks also link to cookbook detail
- [x] Implement drill-down from dependency graph nodes to cookbook/role detail — `DependencyGraphPage.tsx` selected-node panel links cookbook nodes to `/cookbooks/:name`; table view expanded rows link cookbook dependencies to detail page
- [ ] Ensure dashboard performs acceptably with many thousands of nodes

## Dependency Graph View

- [x] Implement interactive directed graph rendering (roles and cookbooks as nodes, includes as edges) — `DependencyGraphPage.tsx` → `ForceGraph` component with pure TypeScript force-directed simulation (repulsion, spring attraction, center gravity, damping), SVG rendering via `requestAnimationFrame`; squares for roles (blue), circles for cookbooks (emerald); pan/zoom/drag support
- [ ] Colour-code cookbook nodes by compatibility status (green=compatible, red=incompatible, grey=untested, amber=CookStyle-only) — nodes are currently coloured by type only (role=blue, cookbook=green); compatibility status is not fetched or applied
- [x] Highlight incompatible cookbooks and the roles that depend on them — click any node to highlight its direct connections and dim the rest; adjacency-based highlighting via `connectedToSelected` set
- [x] Support filtering by specific cookbook (show subgraph involving that cookbook) — search box filters nodes by name; clicking a cookbook node highlights its connected subgraph
- [x] Support filtering by specific role (show subgraph reachable from that role) — search box + click to highlight; type filter buttons to show only roles
- [ ] Support filtering by compatibility status (show only paths involving incompatible/untested cookbooks) — not implemented; would require fetching compatibility data and joining with graph nodes
- [x] Implement search/filter for large graphs to focus on a subset — real-time text search dims non-matching nodes; type filter toggles (All/Roles/Cookbooks)
- [ ] Implement lazy loading or level-of-detail rendering for large graphs
- [x] Implement alternative table view showing roles with direct and transitive cookbook dependencies — `DependencyGraphPage.tsx` table view with sortable columns, expandable rows showing full dependency lists, shared-cookbooks bar chart, pagination
- [x] Link cookbook nodes to cookbook detail view — graph selected-node panel and table expanded rows both link to `/cookbooks/:name`
- [ ] Link role nodes to node list filtered by that role — not yet wired (would link to `/nodes?role=ROLE_NAME`)

## Remediation Guidance View

- [x] Implement remediation priority list — incompatible cookbooks sorted by priority score (complexity × blast radius) — `RemediationPage.tsx` → `PriorityTable` with sortable columns (priority_score, complexity, affected nodes, auto/manual counts); `PriorityScoreBar` visual indicator; pagination
- [x] Display per-cookbook: complexity score/label, blast radius, auto-correctable vs. manual-fix count, top deprecations — `RemediationPage.tsx` → `PriorityRow` renders `ComplexityBadge`, affected node/role counts, auto vs manual counts, deprecation/error counts; links to cookbook detail
- [x] Implement auto-correct preview display with unified diff viewer — `CookbookRemediationPage.tsx` → `AutocorrectPreviewCard` with expand/collapse, syntax-coloured diff (green=additions, red=deletions, cyan=hunks), correctable/remaining/files-modified stats
- [x] Display auto-correct statistics (total offenses, correctable, remaining, files modified) — `AutocorrectPreviewCard` header shows `preview.correctable_offenses` of `preview.total_offenses` correctable, files modified, remaining after auto-correct
- [ ] Include prominent notice that auto-correct is preview only — tool does not modify cookbook source — not yet rendered in `AutocorrectPreviewCard`
- [x] Implement migration documentation display per deprecation offense:
  - [x] Human-readable description — `OffenseGroupCard` → remediation guidance section with `group.remediation.description`
  - [x] Link to Chef migration docs — "Migration Documentation" external link from `group.remediation.migration_url`
  - [x] Chef version where deprecation was introduced/removed — "Introduced in Chef X" / "Removed in Chef Y" from `group.remediation.introduced_in` / `removed_in`
  - [x] Before/after replacement pattern code example — `group.remediation.replacement_pattern` rendered in syntax-highlighted `<pre>` block
- [x] Group deprecation offenses by cop name for consolidated view — `CookbookRemediationPage.tsx` renders `data.offense_groups` (grouped by `cop_name`); expand/collapse all controls; individual offense locations listed inside each group
- [x] Implement effort estimation summary at top of remediation view:
  - [x] Total cookbooks needing remediation — `SummaryHeader` stat card "Need Remediation" with `summary.total_needing_remediation` of `summary.total_cookbooks_evaluated`
  - [x] Estimated quick wins (auto-correct only) — "Quick Wins" stat card with `summary.quick_wins`
  - [x] Estimated manual fixes needed — "Manual Fixes" stat card with `summary.manual_fixes` and auto/manual issue breakdown
  - [x] Total blocked nodes and projected unblocked count — "Blocked Nodes" stat card with `summary.blocked_nodes_by_readiness` and `blocked_nodes_by_complexity`

## Data Exports

Specifications: `visualisation/Specification.md` § Data Exports, `web-api/Specification.md` § Export Endpoints, `datastore/Specification.md` § Table 16 `export_jobs`, `configuration/Specification.md` § Exports.

**Existing infrastructure already in place:**
- Database table `export_jobs` — created in `migrations/0001_initial_schema.up.sql` L468–500 with columns: `id` (UUID PK), `export_type` (ready_nodes|blocked_nodes|cookbook_remediation), `format` (csv|json|chef_search_query), `filters` (JSONB), `status` (pending|processing|completed|failed), `row_count`, `file_path`, `file_size_bytes`, `error_message`, `requested_by`, `requested_at`, `completed_at`, `expires_at`, `created_at`; indexes on status, export_type, requested_by, expires_at.
- Configuration `ExportsConfig` struct in `internal/config/config.go` L229–234 with `MaxRows` (default 100000), `AsyncThreshold` (default 10000), `OutputDirectory` (default `/var/lib/chef-migration-metrics/exports`), `RetentionHours` (default 24); validation in `validateExports()` L840–858.
- Router placeholder routes `r.mux.HandleFunc("/api/v1/exports", r.handleNotImplemented)` and `"/api/v1/exports/"` in `internal/webapi/router.go` L190–191.
- Existing `filterNodes()` function in `internal/webapi/handle_nodes.go` L256+ that filters `[]datastore.NodeSnapshot` by environment, platform, chef_version, policy_name, policy_group, role, stale — reusable for export filtering.
- Existing `resolveOrganisationFilter()` in `internal/webapi/handle_remediation.go` L309+ that resolves `?organisation=` query param to `[]datastore.Organisation` — reusable.
- `DataStore` interface in `internal/webapi/store.go` with `ListNodeSnapshotsByOrganisation`, `ListNodeReadinessForSnapshot`, `ListCookbookComplexitiesForOrganisation`, `ListCookbooksByOrganisation` — all needed for export data assembly.
- `mockStore` in `internal/webapi/store_mock_test.go` — add new Fn fields for any new DataStore methods.
- Frontend types for log entries already reference `export_job_id`; `LogsPage.tsx` scope filter already includes `export_job`.
- Project structure note from `Structure.md`: export generation code goes in `internal/export/`.

**Suggested implementation order:** Items 7 → 1 → 2 → 3 → 4 → 5 → 6 → 8 (datastore first, then generators, then API handlers, then frontend).

- [x] Implement export job status tracking in `export_jobs` table — **Do this first.** Create `internal/datastore/export_jobs.go` with: `ExportJob` struct matching the 16 columns in the `export_jobs` table; `InsertExportJob(ctx, params) (*ExportJob, error)` inserting a new pending job; `GetExportJob(ctx, id) (*ExportJob, error)` by UUID; `UpdateExportJobStatus(ctx, id, status, rowCount, filePath, fileSizeBytes, errorMessage)` for processing→completed/failed transitions; `ListExportJobsByStatus(ctx, status) ([]ExportJob, error)` for the cleanup worker; `ListExpiredExportJobs(ctx, now) ([]ExportJob, error)` selecting where `expires_at < now AND status = 'completed'`; `UpdateExportJobExpired(ctx, id)` setting `status = 'expired'`. Add corresponding methods to the `DataStore` interface in `internal/webapi/store.go` and `mockStore` in `store_mock_test.go`. Write unit tests in `internal/datastore/export_jobs_test.go` (will need a test DB or can follow the pattern used by other datastore tests — check `datastore_test.go` for the test harness).
- [x] Implement ready node export (CSV, JSON, Chef search query string) — Create `internal/export/` package. `ready_nodes.go`: `GenerateReadyNodeExport(ctx, db DataStore, params ReadyNodeExportParams) (*ExportResult, error)` where params include `TargetChefVersion`, `Format` (csv|json|chef_search_query), `Filters` (org, env, role, platform, policy_name, policy_group), `MaxRows`, `OutputPath`. Logic: for each org (respecting org filter), call `ListNodeSnapshotsByOrganisation` → `filterNodes` equivalent → for each node call `ListNodeReadinessForSnapshot` → keep nodes where `ready == true` for the target version. CSV columns per spec: node_name, organisation, environment, platform, platform_version, chef_version, policy_name, policy_group. JSON: array of objects with same fields. Chef search query: `name:node1 OR name:node2 OR ...` (text/plain). Write to `OutputPath`, return `ExportResult{RowCount, FilePath, FileSizeBytes}`. Tests in `internal/export/ready_nodes_test.go` with a fake DataStore.
- [x] Implement blocked node export (CSV, JSON) with blocking reasons and complexity scores — `internal/export/blocked_nodes.go`: `GenerateBlockedNodeExport(ctx, db, params)`. Same org/node iteration as ready export but keep nodes where `ready == false`. CSV/JSON columns: node_name, organisation, environment, platform, platform_version, chef_version, policy_name, policy_group, target_chef_version, blocking_cookbooks (semicolon-delimited in CSV, array in JSON), blocking_reasons (semicolon-delimited in CSV, array in JSON), complexity_score (sum of blocking cookbook complexity scores — lookup via `ListCookbookComplexitiesForOrganisation`). Per spec: "Include blocking cookbook names, versions, complexity scores, and disk space status." Tests in `internal/export/blocked_nodes_test.go`.
- [x] Implement cookbook remediation report export (CSV, JSON) — `internal/export/cookbook_remediation.go`: `GenerateCookbookRemediationExport(ctx, db, params)`. For each org, call `ListCookbooksByOrganisation` → `ListCookbookComplexitiesForOrganisation` → join. CSV/JSON columns per spec: cookbook_name, version, organisation, target_chef_version, complexity_score, complexity_label, affected_node_count, affected_role_count, auto_correctable_count, manual_fix_count, deprecation_count, error_count. Respects org filter and optional target_chef_version filter. Tests in `internal/export/cookbook_remediation_test.go`.
- [x] Ensure all exports respect currently active filters — Each `Generate*Export` function accepts a `Filters` struct with optional fields: `Organisation`, `Environment`, `Role`, `Platform`, `PolicyName`, `PolicyGroup`, `TargetChefVersion`, `StaleStatus`, `ComplexityLabel`. Node exports apply the same filtering logic as `filterNodes()` in `handle_nodes.go` (extract into a shared `internal/webapi/node_filter.go` or pass filter params into the export package). Cookbook remediation export filters by org and target version. The `POST /api/v1/exports` request body `filters` JSONB is stored in `export_jobs.filters` and passed through to the generator. Add tests verifying each filter is applied (e.g. export with `environment=production` only returns production nodes).
- [x] Implement synchronous export for small result sets — In the `POST /api/v1/exports` handler (replace `handleNotImplemented` in router.go): parse request body (`export_type`, `format`, `target_chef_version`, `filters`), validate export_type ∈ {ready_nodes, blocked_nodes, cookbook_remediation}, validate format ∈ {csv, json, chef_search_query} (chef_search_query only valid for ready_nodes). Estimate row count (quick count query or use org node counts). If estimated rows ≤ `cfg.Exports.AsyncThreshold`: generate inline, stream response with `Content-Type` (`text/csv`, `application/json`, or `text/plain`) and `Content-Disposition: attachment; filename="ready_nodes_2025-06-15.csv"`. Return HTTP 200. No `export_jobs` row needed for sync exports (or optionally create one with status=completed for audit). Add handler tests with mockStore for each export type × format combination, plus validation error cases.
- [x] Implement asynchronous export for large result sets (return job ID, poll for completion, download link) — If estimated rows > `cfg.Exports.AsyncThreshold`: insert `export_jobs` row with status=pending, return HTTP 202 with `{job_id, status: "pending", message: "..."}`. Launch a goroutine (or submit to a worker pool) that: updates status→processing, calls the appropriate `Generate*Export` to write to `cfg.Exports.OutputDirectory/<job_id>.<format>`, on success updates status→completed with row_count/file_path/file_size_bytes/completed_at, on failure updates status→failed with error_message. Implement `GET /api/v1/exports/:job_id` handler: lookup `export_jobs` by id, return JSON with job_id, export_type, format, status, row_count, file_size_bytes, download_url (`/api/v1/exports/<id>/download`), requested_at, completed_at, expires_at. Implement `GET /api/v1/exports/:job_id/download` handler: check status==completed, check not expired, serve file with `Content-Disposition` and appropriate `Content-Type`; return 404 if not found/expired, 409 if not yet completed. If WebSocket is enabled, broadcast `export_complete` or `export_failed` event via `r.Hub()` (EventHub already exists). Add comprehensive handler tests. Frontend: add an `ExportButton` component that POSTs to `/api/v1/exports`, shows a spinner for sync, shows a progress/poll state for async (202), and opens the download link on completion. Wire export buttons into `NodesPage.tsx` (ready/blocked node exports), `RemediationPage.tsx` (cookbook remediation export). Add TypeScript types for export request/response in `types.ts` and API functions in `api.ts`.
- [x] Implement export file retention and cleanup based on `exports.retention_hours` — Create a cleanup function (e.g. `internal/export/cleanup.go`: `CleanupExpiredExports(ctx, db DataStore, outputDir string) error`) that: calls `ListExpiredExportJobs(ctx, time.Now())`, for each job deletes the file at `job.FilePath` (if exists), then calls `UpdateExportJobExpired(ctx, job.ID)`. Wire this into the collection scheduler or a dedicated ticker in `cmd/chef-migration-metrics/main.go` running every hour (or at the collection schedule interval). Log cleanup actions via the structured logger with scope `export_job`. Add tests in `internal/export/cleanup_test.go` using a temp directory and mock datastore.

## Notifications

- [ ] Implement webhook notification channel (HTTP POST with JSON payload)
- [ ] Implement email notification channel (SMTP)
- [ ] Implement notification trigger: cookbook status change
- [ ] Implement notification trigger: readiness milestone (configurable percentage thresholds)
- [ ] Implement notification trigger: new incompatible cookbook detected
- [ ] Implement notification trigger: collection failure
- [ ] Implement notification trigger: stale node threshold exceeded
- [ ] Implement notification filtering by organisation and cookbook
- [ ] Implement notification history display in the dashboard
- [ ] Implement notification delivery retry with configurable backoff
- [ ] Persist notification history to `notification_history` table

## Historical Trending

- [ ] Store timestamped metric snapshots during each collection run
- [x] Implement trend charts for Chef Client version adoption over time — `TrendChart` SVG component with multi-series line/area rendering, auto-scaling axes, hover tooltips; `VersionDistributionTrendCard` in `DashboardPage.tsx` calls `fetchVersionDistributionTrend()` and transforms data via `breakdownToSeries()`; each Chef version is a coloured line showing node count over collection runs
- [x] Implement trend charts for node readiness counts over time — `ReadinessTrendCard` in `DashboardPage.tsx` calls `fetchReadinessTrend()` and renders two `TrendChart` instances: one showing ready % per target Chef version (area chart, 0–100% scale) and one showing absolute ready/blocked node counts per version
- [x] Implement trend charts for aggregate complexity score over time — `ComplexityTrendCard` in `DashboardPage.tsx` calls `fetchComplexityTrend()` → `GET /api/v1/dashboard/complexity/trend` (new backend handler `handleDashboardComplexityTrend` aggregating `ListCookbookComplexitiesForOrganisation` per org × target version); renders average complexity score line chart and low/medium/high/critical label breakdown; 7 new Go tests covering method checks, happy path, empty, DB error, and multi-org/version scenarios
- [x] Implement trend charts for stale node count over time — `StaleTrendCard` in `DashboardPage.tsx` calls `fetchStaleTrend()` → `GET /api/v1/dashboard/stale/trend` (new backend handler `handleDashboardStaleTrend` iterating completed collection runs and counting `IsStale` on node snapshots); renders stale (red) vs fresh (green) dual-line area chart over collection runs; 8 new Go tests covering method checks, happy path, empty, skips-non-completed-runs, DB error, and multi-run scenarios

## Log Viewer

- [x] Implement log viewer in the web UI — `LogsPage.tsx` with two tabs (Log Entries, Collection Runs); paginated tables; expandable detail panel per log entry; route `/logs`; sidebar nav item added to `AppLayout.tsx`
- [x] Scope and display logs per collection job run (per organisation) — scope filter dropdown includes `collection_run`; organisation filter via `OrgSelector` context; Collection Runs tab shows runs per org with status/duration/node counts
- [x] Scope and display logs per cookbook git operation (clone/pull) — scope filter dropdown includes `git_operation`; log detail panel shows cookbook name, version, and commit SHA metadata
- [x] Scope and display logs per Test Kitchen run (per cookbook + target Chef Client version) — scope filter dropdown includes `test_kitchen_run`; log detail panel shows cookbook name, version, and chef_client_version metadata
- [x] Scope and display logs per CookStyle scan (per cookbook version) — scope filter dropdown includes `cookstyle_scan`; log detail panel shows cookbook name and version metadata
- [x] Scope and display logs per notification dispatch (per channel) — scope filter dropdown includes `notification_dispatch`; log detail panel shows notification_channel metadata
- [x] Scope and display logs per export job — scope filter dropdown includes `export_job`; log detail panel shows export_job_id metadata
- [x] Implement log filtering by job type, organisation, cookbook name, and date/time — min severity filter (DEBUG/INFO/WARN/ERROR), scope dropdown (12 scopes), organisation via OrgSelector; backend supports `since`/`until`/`cookbook_name` params
- [x] Capture and store stdout/stderr from external processes (Test Kitchen, CookStyle, git) — `process_output` field displayed in log detail panel as a terminal-styled `<pre>` block (dark bg, green text, scrollable, max-height 256px)
- [ ] Implement log retention purge based on configured retention period