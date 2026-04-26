# Plan: Role Pages

## Goal

Add role compatibility list page and role detail page per `roles.md` spec. Three backend endpoints, two frontend pages, nav integration.

## Specs to Read

- `.claude/specifications/roles.md` (primary)
- `.claude/specifications/project-conventions.md`
- `.claude/specifications/filter-ux-overhaul.md` (filter component contracts)

## Steps

### Phase 1 — Backend: Data Layer

1. Add `RoleFilter` struct and `ListRolesFiltered` query to `internal/datastore/role_filter.go`
   - Transitive expansion via recursive CTE over `role_dependencies`
   - JOIN to `server_cookbook_cookstyle_results` for compatibility derivation
   - JOIN to `node_snapshots` (roles JSONB) for node counts
   - Filtering: name, organisation, compatibility_status
   - Sorting: name, node_count, incompatible_cookbook_count
   - Pagination via LIMIT/OFFSET with COUNT(*) OVER()
2. Add `GetRoleDetail` query to `internal/datastore/role_detail.go`
   - Transitive cookbook/role expansion (recursive CTE, cycle-safe)
   - Blocking cookbooks with dependency paths
   - Blast radius: nodes by org, environment, platform
   - Nested role chain tree structure
3. Add `GetRoleDependencyGraph` query to `internal/datastore/role_graph.go`
   - Scoped transitive closure from a single role
   - Returns nodes + edges in same format as existing dependency graph
4. Add new methods to `DataStore` interface in `internal/webapi/store.go`
5. Add mock implementations to `internal/webapi/store_mock_test.go`
6. Write tests for each new datastore file

### Phase 2 — Backend: API Handlers

7. Create `internal/webapi/handle_roles.go` with:
   - `handleRoles` — GET /api/v1/roles (list with summary bar data)
   - `handleRoleDetail` — GET /api/v1/roles/:name (detail with all sections)
   - `handleRoleDependencyGraph` — GET /api/v1/roles/:name/dependency-graph
8. Register routes in `router.go` (after cookbooks, before git-repos)
9. Write handler tests in `internal/webapi/handle_roles_test.go`

### Phase 3 — Frontend: API + Types

10. Add role types to `frontend/src/types.ts`
11. Add `fetchRoles`, `fetchRoleDetail`, `fetchRoleDependencyGraph` to `frontend/src/api.ts`

### Phase 4 — Frontend: Pages

12. Create `frontend/src/pages/RolesPage.tsx` — list with summary bar, table, filters
13. Create `frontend/src/pages/RoleDetailPage.tsx` — detail with all sections
14. Add routes to `App.tsx`
15. Add "Roles" nav item to `AppLayout.tsx` (after Cookbooks)

### Phase 5 — Integration

16. Wire inbound/outbound links per spec
17. Update todo-tech-debt.md if any tactical decisions made
18. Run all tests, verify clean build

## Acceptance Criteria

- `GET /api/v1/roles` returns paginated roles with compatibility derived from transitive cookbooks
- `GET /api/v1/roles/:name` returns full detail including blocking cookbooks, blast radius, nested role chain
- `GET /api/v1/roles/:name/dependency-graph` returns scoped graph
- `/roles` page shows summary bar, filterable/sortable table
- `/roles/:name` page shows all spec sections
- "Roles" appears in nav after Cookbooks
- All existing tests still pass
- New handler tests pass