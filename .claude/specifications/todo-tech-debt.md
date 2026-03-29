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

- [x] **B4a — Enrich readiness trend with metric snapshots** — Added
  `buildReadinessSnapshotPayload` and `recordReadinessSnapshots` to collector
  (called after Step 14). Rewrote `handleDashboardReadinessTrend` to read from
  `ListMetricSnapshotsByOrganisationAndVersion` with fallback to live
  `CountNodeReadiness`. Supports ownership filtering.

- [x] **B5 — Add datastore tests** — Added 68 validation and pure-function
  tests across 7 new test files (metric_snapshots, node_readiness,
  node_snapshots, organisations, owners, role_dependencies,
  cookbook_usage_analysis). Integration tests requiring live Postgres remain
  a separate effort.

### Project

- [x] **P1 — Create CHANGELOG.md** — Generated from git tag history. Covers
  all 46 releases (v0.0.1 → v2.2.8) in Keep a Changelog format.

---

## 🟡 Medium Priority

### Frontend

- [x] **F4 — Extract shared filter input components** — `FilterInput`,
  `FilterSelect`, and `FilterCombobox` moved from `NodesPage.tsx` to
  `frontend/src/components/FilterInputs.tsx`. Updated 6 consuming pages.

- [x] **F6 — Split large monolithic page files** — DashboardPage (1,061
  lines) split into `dashboard/index.tsx` (shell), `StatusCards.tsx` (6
  cards), `TrendCards.tsx` (4 cards). DependencyGraphPage moved to
  `dependency-graph/` directory with index re-export.

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