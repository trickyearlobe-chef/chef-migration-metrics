# Frontend Embed Wiring + Missing Filter Dropdowns

**Date:** 2026-07-14
**Components:** `frontend/embed.go`, `internal/frontend/`, `internal/webapi/handle_nodes.go`, `internal/webapi/handle_remediation.go`, `frontend/src/pages/NodesPage.tsx`, `frontend/src/pages/RemediationPage.tsx`, `frontend/src/api.ts`, `Makefile`, `Dockerfile`

## Context

The Go binary had no mechanism to embed the React SPA into itself — `internal/frontend/frontend.go` used `os.DirFS` at runtime, and the Dockerfile didn't copy `frontend/dist` into the runtime image. This meant the single-binary deployment story was broken. Additionally, four frontend filter dropdowns were missing: role, policy name, policy group (on Nodes page), and complexity label (on Remediation page). The backend filter endpoints existed but the frontend didn't use them.

## What Was Done

### 1. `go:embed` Wiring for Frontend Assets

**Problem:** `go:embed` paths are relative to the source file and cannot use `..`. The `internal/frontend/` package can't reference `../../frontend/dist`. The only place that can embed `frontend/dist` is a Go file inside `frontend/` itself.

**Solution — dual-package architecture:**

- **`frontend/embed.go`** (`package frontendfs`) — contains `//go:embed all:dist` directive. Lives alongside `package.json` so it can see `dist/` directly. Exports `DistFS()` which returns an `fs.FS` rooted at the dist contents (via `fs.Sub`).

- **`internal/frontend/frontend.go`** (`package frontend`) — rewritten with a registration pattern. Exports `RegisterEmbedFS(fs.FS)`, `HasEmbed() bool`, and `FS(dir string) fs.FS`. Resolution order: (1) registered embed FS, (2) `os.DirFS(dir)` disk fallback, (3) nil.

- **`internal/frontend/embed_init.go`** — `init()` function that imports `frontendfs`, calls `frontendfs.DistFS()`, and registers the result via `RegisterEmbedFS`. This auto-wires the embed on import — no changes needed in `main.go` beyond what already existed.

- **`cmd/chef-migration-metrics/main.go`** — updated log messages to distinguish "loaded from embedded binary" vs "loaded from disk" using `HasEmbed()`.

**Build pipeline changes:**

- **`Makefile` `build-frontend` target** — now always ensures `frontend/dist/` exists with at least a placeholder `index.html` after attempting `npm ci && npm run build`. This guarantees the `go:embed` directive succeeds even when npm is unavailable.

- **`Dockerfile`** — same logic: after the optional npm build step, `mkdir -p frontend/dist` and write a placeholder `index.html` if the real build didn't produce one.

- **`.gitignore`** — `frontend/dist/` remains gitignored (Vite overwrites it on build, Makefile creates placeholder).

### 2. Backend: `role` Filter for Nodes Endpoint

Added `?role=` query parameter support to `filterNodes()` in `handle_nodes.go`. New `nodeHasRole(n, roleName)` helper uses JSON-quoted substring matching on `n.Roles` (same pattern as `nodeUsesCookbook`). 9 new tests covering match, no-match, empty roles, nil roles, partial name rejection, exact match among similar names, and combined filters.

### 3. Backend: `complexity_label` Filter for Remediation Priority Endpoint

Added `?complexity_label=` query parameter to `handleRemediationPriority`. Filters the aggregated priority items after collection and before sorting/pagination. 3 new tests: filter match (returns only matching label), no match (returns empty), and omitted (returns all).

### 4. Frontend: Nodes Page Filter Dropdowns

Rewrote `NodesPage.tsx` filter bar to add five API-populated dropdown selectors:

| Filter | API Endpoint | Behavior |
|--------|-------------|----------|
| Environment | `GET /api/v1/filters/environments` | Dropdown; falls back to text input if API returns empty |
| Platform | `GET /api/v1/filters/platforms` | Dropdown (platform names only); falls back to text input |
| Role | `GET /api/v1/filters/roles` | Dropdown; sends `?role=` to backend |
| Policy Name | `GET /api/v1/filters/policy-names` | Dropdown; sends `?policy_name=` to backend |
| Policy Group | `GET /api/v1/filters/policy-groups` | Dropdown; sends `?policy_group=` to backend |

All dropdowns refresh when the selected organisation changes. Added a "Clear (N)" button that resets all active filters. Created a reusable `FilterCombobox` component that renders a `<select>` when options are loaded or falls back to a `<FilterInput>` text field.

Added `role` field to `NodeFilterQuery` interface in `api.ts`.

### 5. Frontend: Remediation Page Complexity Label Filter

Added complexity label dropdown to `RemediationPage.tsx` header bar, next to the existing Target Chef Version selector. Populated from `GET /api/v1/filters/complexity-labels`. Labels are title-cased in the dropdown ("Low", "Medium", "High", "Critical"). Sends `?complexity_label=` to the backend priority endpoint.

Added `complexity_label` field to `RemediationQuery` interface in `api.ts`.

## Final State

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test -count=1 ./...` — **14 packages, all pass**
  - `frontend` — 3 tests (embed FS validation)
  - `internal/frontend` — 11 tests (registration, fallback, disk, nil)
  - `internal/webapi` — includes 9 new role filter tests + 3 new complexity label tests
  - All other packages unchanged and passing
- Frontend TypeScript — `api.ts` shows 0 diagnostics; `.tsx` files show only "Cannot find module 'react'" errors due to missing `node_modules` in the editor environment (resolves with `npm ci`)

## Known Gaps

- **Frontend not built in CI for this change** — `npm` was not available in the test environment. The TypeScript changes are structurally correct (matching existing patterns) but haven't been validated through `tsc` or `vite build`.
- **Environment and platform dropdowns are now `<select>` instead of free-text** — if the filter API returns an incomplete list, users can't type arbitrary values. The `FilterCombobox` component falls back to a text input when the option list is empty, but not when it's populated but incomplete.
- **No "auto-correct preview" notice** — the `CookbookRemediationPage.tsx` still lacks a prominent "this is preview only — no cookbook source is modified" disclaimer (existing gap, not introduced here).
- **Complexity label filter is applied after aggregation** — the backend collects all complexities for the target version, then filters by label. For very large datasets this is less efficient than filtering in the DB query, but it's consistent with the existing aggregation pattern.

## Files Modified

### Production
- `frontend/embed.go` — **created** — `package frontendfs`, `//go:embed all:dist`, `DistFS()`
- `internal/frontend/frontend.go` — **rewritten** — registration pattern with `RegisterEmbedFS`, `HasEmbed`, `FS` (embed-first, disk fallback)
- `internal/frontend/embed_init.go` — **created** — `init()` auto-registers embedded assets
- `cmd/chef-migration-metrics/main.go` — updated log messages for embed vs disk source
- `internal/webapi/handle_nodes.go` — added `role` filter to `filterNodes()`, added `nodeHasRole()` helper
- `internal/webapi/handle_remediation.go` — added `complexity_label` filter to `handleRemediationPriority()`
- `frontend/src/api.ts` — added `role` to `NodeFilterQuery`, `complexity_label` to `RemediationQuery`
- `frontend/src/pages/NodesPage.tsx` — added role/policy-name/policy-group/environment/platform dropdowns, `FilterCombobox` component, clear button
- `frontend/src/pages/RemediationPage.tsx` — added complexity label dropdown
- `Makefile` — `build-frontend` target ensures `dist/` placeholder exists
- `Dockerfile` — ensures `frontend/dist/` placeholder exists before `go build`

### Tests
- `frontend/embed_test.go` — **created** — 3 tests for embedded FS
- `internal/frontend/frontend_test.go` — **created** — 11 tests for registration, fallback, disk, nil
- `internal/webapi/handle_nodes_test.go` — added 9 tests for `nodeHasRole` and role filter
- `internal/webapi/handle_remediation_test.go` — added 3 tests for `complexity_label` filter

### Documentation
- `.claude/Structure.md` — updated with `frontend/embed.go`, `internal/frontend/` description, summary entry
- `.claude/specifications/todo/visualisation.md` — marked 4 filter items as done
- `.claude/specifications/ToDo.md` — updated visualisation count to 46/86 (53%), total to 424/679 (62%)
- `.claude/summaries/archive/` — archived 2 oldest summaries

## Recommended Next Steps

### 1. RPM/DEB Packaging (medium, ~20k tokens)

**Why:** The binary now embeds frontend assets and is fully self-contained. Packaging it for RPM/DEB is the next step to make it deployable on production Linux servers without Docker.

**Read specs:** `packaging/Specification.md` §2 (RPM), §3 (DEB), §9 (Build Targets)
**Read todo:** `todo/packaging.md` — RPM + DEB + systemd sections

**Scope:**
1. Create/update `nfpm.yaml` with package metadata, file mappings, dependencies
2. Create systemd unit file at `deploy/pkg/chef-migration-metrics.service`
3. Create install scripts (pre/post install/remove) at `deploy/pkg/scripts/`
4. Verify `make package-rpm` and `make package-deb` produce installable packages
5. Update `deploy/pkg/` config template

### 2. Helm Chart Templates (medium-large, ~30k tokens)

**Why:** 30 items in `todo/packaging.md` under Helm Chart. Kubernetes deployment is a key deployment target.

**Read specs:** `packaging/Specification.md` §6
**Read todo:** `todo/packaging.md` — Helm Chart section

### 3. Data Exports (medium, ~15k tokens)

**Why:** 0/8 export tasks done. Users need CSV/JSON exports of node lists, cookbook reports, and remediation data.

**Read specs:** `web-api/Specification.md` (§ Exports)
**Read todo:** `todo/visualisation.md` — Data Exports section

### 4. Log Viewer UI (medium, ~15k tokens)

**Why:** Backend log endpoints exist and are tested. No frontend page renders them yet. 0/10 log viewer tasks done.

**Read specs:** `visualisation/Specification.md` (§ Log Viewer)
**Read todo:** `todo/visualisation.md` — Log Viewer section

### 5. Historical Trend Charts (small-medium, ~10k tokens)

**Why:** Backend trend endpoints (`/api/v1/dashboard/version-distribution/trend`, `/api/v1/dashboard/readiness/trend`) exist but the frontend doesn't render trend charts yet. 0/5 trending tasks done.

**Read todo:** `todo/visualisation.md` — Historical Trending section