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

- [ ] **B2 — Push cookbook-by-node filtering into SQL** —
  `handleNodesByCookbook` loads every node's full JSON (`IncludeHeavyJSON:
  true`) into memory then does `strings.Contains` substring matching, which can
  false-positive (e.g. `"apt"` matches `"apt-repo"`). Replace with a SQL JSONB
  `?` operator or a dedicated junction table query.
  Files: `handle_nodes.go` L472–474, L651–658.

- [ ] **B4a — Enrich readiness trend with metric snapshots** —
  `handleDashboardReadinessTrend` still queries live `CountNodeReadiness`
  instead of reading from `metric_snapshots`. It doesn't suffer from the
  sawtooth bug (no `collection_run_id` dependency) but is inconsistent with
  the version-distribution trend which now reads from snapshots. Follow-up to
  the collection-dashboard isolation fix. Requires recording a
  `readiness_summary` metric snapshot type in `recordMetricSnapshots`.
  Files: `handle_dashboard.go` `handleDashboardReadinessTrend`,
  `collector.go` `recordMetricSnapshots`.

- [ ] **B4 — Extract ownership filter helper** — The same ~25-line ownership
  resolution pattern (parse filter → check `Unowned` → call
  `resolveAllOwnedEntityKeys` or `resolveOwnedEntityKeys` → in-memory filter)
  is copy-pasted across ~10 handlers. Extract a single
  `resolveOwnershipKeys(ctx, req, entityType)` helper.
  Files: `handle_dashboard.go`, `handle_nodes.go`, `handle_cookbooks.go`,
  `handle_git_repos.go`.

- [ ] **B5 — Add datastore tests** — 15 of 20 datastore source files have no
  corresponding `*_test.go`. This is the most critical layer for correctness.
  `datastore_test.go`, `export_jobs_test.go`, `node_snapshot_filter_test.go`,
  `log_entries_test.go`, and `collection_runs_test.go` currently exist.
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


### Project

- [ ] **P2 — Populate or remove `internal/models/`** — CLAUDE.md specifies
  that shared domain types live in `internal/models/`, but the directory is
  empty. Types are currently scattered across `internal/datastore/` and handler
  files.

- [ ] **P3 — Implement or descope `internal/notify/`** — Webhook and email
  notification channels are referenced in `secrets-storage.md` and
  `todo-visualisation.md`, but no code exists. Either build the package or
  update the specs to remove the references.

---

## 🟢 Low Priority

### Project


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