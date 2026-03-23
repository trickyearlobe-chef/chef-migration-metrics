# Tech Debt — Tracking List

This file tracks identified technical debt across the Chef Migration Metrics
codebase. Items are grouped by priority and area. Each item has a checkbox so
progress can be tracked over time.

---

## 🔴 High Priority

### Frontend

- [ ] **F1 — Extract shared sort logic** — 7+ separate implementations of
  `handleSort` / `sortIndicator` / `SortHeader` / `SortableHeader` /
  `SortableColHeader` exist across page files, each slightly different.
  Extract a shared `useSort` hook (`frontend/src/hooks/useSort.ts`) and a
  `SortableColumnHeader` component (`frontend/src/components/SortableColumnHeader.tsx`).
  Affected files: `NodesPage`, `CookbooksPage`, `GitReposPage`,
  `AdminSystemStatsPage`, `DependencyGraphPage`, `OwnersPage`,
  `NodeDiskDetailPage`, `CookbookCommittersPage`.

- [ ] **F2 — Add React Error Boundary** — No error boundary exists anywhere in
  the application. Any rendering exception (bad API data, null reference)
  white-screens the entire app. Add a top-level `ErrorBoundary` in `App.tsx`
  wrapping the route tree, and consider a per-page boundary around
  `DependencyGraphPage`'s `ForceGraph` component (650 lines of physics
  simulation / SVG rendering — highest crash risk).

### Backend

- [ ] **B0 — Replace UUIDs with natural keys** — Every table uses synthetic
  UUIDs as primary keys and foreign keys, but every entity has a stable natural
  key: organisations have `name`, nodes have `(organisation_name, node_name)`,
  server cookbooks have `(organisation_name, name, version)`, git repos have
  `name`, readiness records have `(organisation_name, node_name, target_chef_version)`,
  CookStyle results have `(entity_name, target_chef_version)`. The UUIDs add a
  fragile layer of indirection — when a UUID changes (snapshot re-collection,
  git repo URL change, DISTINCT ON picking a different row), all joins break
  silently: readiness records become invisible, CookStyle results don't match,
  compatibility shows "untested" for scanned repos. Three bugs in this session
  traced directly to UUID drift: (1) git repo compatibility using complexity
  records keyed by stale git_repo_id, (2) node readiness invisible on detail
  page because node_snapshot_id changed between collection and evaluation,
  (3) dashboard/list discrepancy because dashboard queries by name but detail
  queries by UUID. Migrate to natural composite keys as primary keys and
  foreign keys. This is a large schema change touching every table, every
  query, every handler, and every API response — but it eliminates an entire
  class of bugs. Start with a specification, then execute as a series of
  migrations. Affected: all tables in `migrations/`, all files in
  `internal/datastore/`, all handlers in `internal/webapi/`, readiness
  evaluator in `internal/analysis/`, collector in `internal/collector/`.

- [ ] **B1 — Fix N+1 readiness queries in web handlers** — `handleNodes`
  fires an individual `ListNodeReadinessByNodeName` query per node on the
  page (50 queries per request). `handleDashboardReadiness` does the same
  for all owned nodes. Replace with a bulk query that loads readiness for
  all node names at once.
  Files: `handle_nodes.go` L89–100, `handle_dashboard.go` L438–461.
  **Note:** The far worse N+1 in the readiness *evaluator* (~12M queries
  per run at 60K nodes) was resolved in `refactor/readiness-bulk-load` —
  all lookup data is now bulk-loaded into an in-memory cache before the
  fan-out loop. This B1 item covers only the remaining web handler N+1.

- [ ] **B2 — Push cookbook-by-node filtering into SQL** —
  `handleNodesByCookbook` loads every node's full JSON (`IncludeHeavyJSON:
  true`) into memory then does `strings.Contains` substring matching, which can
  false-positive (e.g. `"apt"` matches `"apt-repo"`). Replace with a SQL JSONB
  `?` operator or a dedicated junction table query.
  Files: `handle_nodes.go` L472–474, L651–658.

- [ ] **B3 — Deduplicate node snapshot filter query builders** —
  `buildNodeSnapshotFilterQuery` and `buildNodeSnapshotFilterParts` share ~80%
  of their CTE and WHERE clause logic. Refactor so the full-query builder calls
  the parts builder internally.
  File: `node_snapshot_filter.go` L83–244 and L500–582.

- [ ] **B4 — Extract ownership filter helper** — The same ~25-line ownership
  resolution pattern (parse filter → check `Unowned` → call
  `resolveAllOwnedEntityKeys` or `resolveOwnedEntityKeys` → in-memory filter)
  is copy-pasted across ~10 handlers. Extract a single
  `resolveOwnershipKeys(ctx, req, entityType)` helper.
  Files: `handle_dashboard.go`, `handle_nodes.go`, `handle_cookbooks.go`,
  `handle_git_repos.go`.

- [ ] **B5 — Add datastore tests** — 18 of 20 datastore source files have no
  corresponding `*_test.go`. This is the most critical layer for correctness.
  Only `datastore_test.go`, `export_jobs_test.go`, and
  `node_snapshot_filter_test.go` currently exist.
  Directory: `internal/datastore/`.

### Project

- [ ] **P1 — Create CHANGELOG.md** — 36 releases (v0.0.1 → v2.0.1) have
  shipped with no changelog. Generate from git tags/history so users can track
  upgrade impact, especially across the v1 → v2 major version boundary.

---

## 🟡 Medium Priority

### Frontend

- [ ] **F3 — Unify sort indicator visuals** — Some pages use ↕/↑/↓ text
  arrows, `OwnersPage` uses SVG chevrons, `DependencyGraphPage` shows nothing
  for inactive columns. Standardise when extracting the shared sort component
  (F1). Affected: 8 page files.

- [ ] **F4 — Extract shared filter input components** — `NodesPage` has
  `FilterInput`, `FilterSelect`, and `FilterCombobox` helper components, but
  they are local to that file. The same `className` string for `<input>` and
  `<select>` is copy-pasted across 6 other pages. Move to
  `frontend/src/components/`.

- [ ] **F5 — Extract `useTargetChefVersion` hook** — The same
  load-versions-and-pick-highest pattern is repeated in `NodesPage`,
  `CookbooksPage`, `GitReposPage`, and `RemediationPage`.

- [ ] **F6 — Split large monolithic page files** —
  `DependencyGraphPage.tsx` (1,523 lines, 7 components) and
  `DashboardPage.tsx` (1,019 lines, 11 card components) should be split into
  sub-files under `pages/dependency-graph/` and `pages/dashboard/` (or
  `components/dashboard/`) respectively.

- [ ] **F7 — Add frontend tests** — `npm test` currently just echoes
  `"No frontend tests yet"`. CI always passes, which is deceptive. Add at
  least unit tests for shared hooks and utility functions.



### Backend

- [ ] **B6 — Deduplicate `nodeResp` struct** — Defined identically twice in
  the same file (`handle_nodes.go` L102–118 and L247–263). Promote to a
  package-level type.

- [ ] **B7 — Extract generic `PaginateSlice[T]` helper** — The same 6-line
  in-memory pagination pattern (compute start/end from offset and limit, clamp
  to slice bounds) is duplicated in `handle_cookbooks.go`,
  `handle_git_repos.go`, `handle_logs.go`, and `handle_nodes.go`.

- [ ] **B8 — Deduplicate log entry filter building** — `ListLogEntries` and
  `CountLogEntries` in `log_entries.go` duplicate the entire WHERE clause
  construction. Extract a `buildLogEntryFilterQuery` helper, mirroring the
  pattern used for node snapshots.

- [ ] **B9 — Log swallowed error in `WriteJSON`** — `response.go` L138–139
  discards JSON encoding errors with `_ = err`. At minimum, log the error so
  corrupted responses are detectable.

- [ ] **B10 — Add SQL push-down for collection runs** —
  `handleCollectionRuns` loads ALL historical runs across all orgs into memory,
  then paginates. For long-running installations this grows unboundedly.
  File: `handle_logs.go` L142–146.

- [ ] **B11 — Reconcile `operator` role** — `handle_ownership.go` L123
  references an `operator` role, but `handle_admin_users.go` L125 only allows
  creating `admin` and `viewer` roles. Either add `operator` to the admin
  validation or remove the dead code path.

### Project

- [ ] **P2 — Populate or remove `internal/models/`** — CLAUDE.md specifies
  that shared domain types live in `internal/models/`, but the directory is
  empty. Types are currently scattered across `internal/datastore/` and handler
  files.

- [ ] **P3 — Implement or descope `internal/notify/`** — Webhook and email
  notification channels are referenced in `secrets-storage.md` and
  `todo-visualisation.md`, but no code exists. Either build the package or
  update the specs to remove the references.

- [ ] **P4 — Decompose `main.go` `run()` function** — At 860 lines
  (L48–909), `run()` handles CLI parsing, config loading, database setup, auth,
  secrets, key rotation, TLS, collector, and HTTP server setup. Split into
  named functions for each phase.

---

## 🟢 Low Priority

### Frontend

- [ ] **F9 — Add `GitRepoFilterQuery` type** — `GitReposPage.tsx` uses an
  inline anonymous type for its filter query instead of a named interface in
  `api.ts` like `CookbookFilterQuery` and `NodeFilterQuery`.

- [ ] **F10 — Remove unused `_currentOrder` prop** — `SortableHeader` in
  `DependencyGraphPage.tsx` L1360 accepts `currentOrder` but immediately
  discards it as `_currentOrder`. Remove the prop or use it.

- [ ] **F11 — Remove unused `_jobId` state** — `ExportButton.tsx` L70 has
  `const [_jobId, setJobId] = useState(...)` where the state value is never
  read. The job ID is already captured in a closure.

- [ ] **F12 — Centralise `perPage` constants** — Hardcoded `50` appears in 7
  files and `25` in 3 files. Define `DEFAULT_PAGE_SIZE` and `SMALL_PAGE_SIZE`
  in a shared constants file.

### Backend

- [ ] **B12 — Remove deprecated `filterNodes` function** —
  `handle_nodes.go` L590–602 is explicitly marked deprecated in its docstring.
  Verify the export system no longer uses it, then remove.

- [ ] **B13 — Remove useless compile-time check** — `handle_cookbooks.go`
  L488 has `var _ = errors.Is(nil, datastore.ErrNotFound)` which verifies
  nothing useful (always returns `false`).

- [ ] **B14 — Add timeout bounds to background contexts** —
  `handle_admin_rescan_all.go` and `handle_exports.go` use
  `context.Background()` for long-running goroutines without any timeout. Add
  `context.WithTimeout` to prevent runaway jobs.

- [ ] **B15 — Make DB pool settings configurable** — `datastore.go` L61–64
  hardcodes `MaxOpenConns(25)`, `MaxIdleConns(5)`, `ConnMaxLifetime(5m)`,
  `ConnMaxIdleTime(1m)`. These should be configurable via `DatastoreConfig`.

### Project

- [ ] **P5 — Add ACME patterns to `.helmignore`** — `.gitignore` and
  `.dockerignore` include `acme/`, `.lego/`, `.certmagic/` patterns, but
  `.helmignore` does not. Keep consistent even though ACME is not yet
  implemented.

- [ ] **P6 — Add frontend build artifacts to `.gitignore`** —
  `tsconfig.tsbuildinfo`, `tsconfig.node.tsbuildinfo`, and `vite.config.d.ts`
  are generated files that should be ignored.

- [ ] **P7 — Split `handle_dashboard.go`** — At 1,597 lines with 12
  independent endpoint handlers, this should be split into focused files like
  `handle_dashboard_readiness.go`, `handle_dashboard_compatibility.go`, etc.

- [ ] **P8 — Fix errcheck linter violations** — The `errcheck` linter is
  disabled in `.golangci.yml` due to 50+ violations. Re-enable and fix
  incrementally (tracked in `todo-project-setup.md`).

---

## ✅ Positive Findings (no action needed)

- **No SQL injection risks** — all dynamic SQL uses parameterised queries with
  whitelist switches for sort columns.
- **Lean Go dependency tree** — only 4 direct dependencies.
- **Migrations are clean** — sequential, paired up/down, correct FK ordering.
- **CI/CD is comprehensive** — lint, test, security scan, multi-arch release,
  Helm OCI packaging, Dependabot.
- **Ignore files are well-maintained** — secrets covered in all three files.
- **Good security posture** — `SECURITY.md`, `govulncheck` in CI, non-root
  Docker user.