# Tech Debt — Tracking List

This file tracks identified technical debt across the Chef Migration Metrics
codebase. Items are grouped by priority and area. Each item has a checkbox so
progress can be tracked over time.

---

## 🔴 High Priority

### Backend

- [ ] **B0 — Replace UUIDs with natural keys** — Every table uses synthetic
  UUIDs as primary keys and foreign keys, but every entity has a stable natural
  key. The UUIDs add a fragile layer of indirection — when a UUID changes
  (snapshot re-collection, git repo URL change, DISTINCT ON picking a different
  row), all joins break silently. Migrate to natural composite keys as primary
  keys and foreign keys. This is XL — requires its own specification and phased
  migration plan. Affected: all tables, all queries, all handlers, all API
  responses.

- [ ] **B4a — Enrich readiness trend with metric snapshots** —
  `handleDashboardReadinessTrend` still queries live `CountNodeReadiness`
  instead of reading from `metric_snapshots`. It doesn't suffer from the
  sawtooth bug (no `collection_run_id` dependency) but is inconsistent with
  the version-distribution trend which now reads from snapshots. Requires
  recording a `readiness_summary` metric snapshot type in
  `recordMetricSnapshots`.
  Files: `handle_dashboard_readiness.go`, `collector.go`.

- [ ] **B5 — Add datastore tests** — 15 of 20 datastore source files have no
  corresponding `*_test.go`. This is the most critical layer for correctness.
  Existing tests: `datastore_test.go` (unit, pure functions),
  `functional_test.go` (integration, requires Postgres via
  `CMM_TEST_DATABASE_URL`), `export_jobs_test.go`,
  `node_snapshot_filter_test.go`, `log_entries_test.go`,
  `collection_runs_test.go`, `pg_stats_test.go`,
  `cookbook_production_platforms_test.go`.
  Directory: `internal/datastore/`.

### Project

- [x] **P1 — Create CHANGELOG.md** — Generated from git tag history. Covers
  all 46 releases (v0.0.1 → v2.2.8) in Keep a Changelog format.

---

## 🟡 Medium Priority

### Frontend

- [x] **F4 — Extract shared filter input components** — `FilterInput`,
  `FilterSelect`, and `FilterCombobox` moved from `NodesPage.tsx` to
  `frontend/src/components/FilterInputs.tsx`. Updated 6 consuming pages.

- [ ] **F6 — Split large monolithic page files** —
  `DependencyGraphPage.tsx` (1,646 lines, 7 components) and
  `DashboardPage.tsx` (1,061 lines, 11 card components) should be split into
  sub-files under `pages/dependency-graph/` and `pages/dashboard/`
  respectively. Deferred from batch 6 — moderate risk, low urgency.

- [x] **F7 — Add frontend tests** — Vitest + Testing Library installed.
  `npm test` runs 39 real tests: 13 semver, 8 useSort hook, 18 FilterInputs.

---

## 🟢 Low Priority

### Project

- [x] **P8 — Fix errcheck linter violations** — Re-enabled errcheck in
  `.golangci.yml` with `exclude-functions` for fire-and-forget patterns
  (logging, defer Close, HTTP writes). Fixed remaining violations with
  `_ =`. Also fixed 7 pre-existing govet/staticcheck issues.
  `golangci-lint run ./...` now reports 0 issues.

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