# Plan — CookStyle Violations Browser

Spec: `specifications/cookstyle-violations-browser.md`
Branch: `feature/cookstyle-violations-browser`

## Chunk 1 — API endpoint (backend)

Scope: `internal/webapi/`, `internal/datastore/`
Dependencies: none

Steps:
1. Add datastore method: `ListCookstyleViolations(ctx, params) ([]CookstyleViolationRow, total int, error)` — paginated query against `server_cookbook_cookstyle_results` joined to `server_cookbooks` for name/version/org, with optional JSONB filters for namespace/severity/cop
2. Add equivalent for git repos: `ListGitRepoCookstyleViolations(ctx, params)`
3. Create `internal/webapi/handle_cookstyle_violations.go`:
   - `GET /api/v1/cookstyle/violations` handler
   - Parse query params, validate target_chef_version required
   - Call datastore, deserialise offences JSONB per row to compute namespace_counts/severity_counts/top_cops
   - Return paginated JSON response
4. Wire route in `router.go`
5. Tests: mock DB, verify filtering, pagination, response shape

Acceptance:
- `go test ./internal/webapi/ ./internal/datastore/...` passes
- Endpoint returns paginated results with summary counts
- Filters narrow results correctly

## Chunk 2 — Frontend page

Scope: `frontend/src/`
Dependencies: Chunk 1

Steps:
1. Add types: `CookstyleViolationItem`, `CookstyleViolationsResponse`
2. Add API client function: `fetchCookstyleViolations(params)`
3. Create `CookstyleViolationsTab` component:
   - Source toggle: Server Cookbooks / Git Repos
   - Filter bar: target version, namespace/severity multi-select, cop typeahead, status filter
   - Results table with sortable columns
   - Server-side pagination
   - URL query param persistence for filters
4. Convert Remediation page to a tabbed layout (Priority | CookStyle Violations)
   - Wrap existing `RemediationPage` content as the default "Priority" tab
   - Add "CookStyle Violations" tab rendering `CookstyleViolationsTab`
   - Tab selection persisted via `?tab=` query param
5. Tests: filter interactions, source toggle, empty state, pagination controls, tab switching

Acceptance:
- Remediation page shows two tabs: Priority (default) and CookStyle Violations
- Source toggle switches between server cookbook and git repo results
- Filters update URL and reload results
- Cookbook/repo links navigate to existing detail/remediation pages
- Empty state shown when no results match filters
