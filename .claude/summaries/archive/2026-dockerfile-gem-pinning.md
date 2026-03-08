# Dockerfile Gem Version Pinning to Chef Workstation 25.13.7

**Date:** 2026-07-14
**Components:** `Dockerfile`, `Makefile`, `packaging/Specification.md`, `todo/packaging.md`

## Context

The Dockerfile had gem version conflicts when building the embedded Ruby environment. All gems (cookstyle, test-kitchen, inspec, kitchen drivers) were unpinned and installed against Ruby 3.2, while the Chef ecosystem is built and tested against Ruby 3.1.7 with tightly coupled version constraints. The canonical source of shipped gem versions is `components/gems/Gemfile.lock` in the `chef/chef-workstation` repo.

## What Was Done

### 1. Ruby Version Downgrade (3.2 → 3.1)

Changed `FROM ruby:3.2-bookworm` to `FROM ruby:3.1-bookworm` in Dockerfile and `RUBY_BUILD_IMAGE` in Makefile. Chef Workstation 25.13.7 ships Ruby 3.1.7 (`omnibus_overrides.rb`). Using 3.2+ causes conflicts because `nokogiri >= 1.19.1` requires Ruby >= 3.2 and Chef Workstation caps it at `< 1.19.1`.

### 2. Pinned All Gem Versions to Chef Workstation 25.13.7

Source: `https://github.com/chef/chef-workstation/blob/main/components/gems/Gemfile.lock`

**Core tools:**
- `cookstyle:7.32.8` (hard-pins `rubocop:1.25.1`)
- `test-kitchen:3.9.1`
- `inspec-bin:5.24.7` / `inspec-core:5.24.7`
- `ffi:1.16.3` — installed first as ecosystem-wide ceiling (`mixlib-log` requires `ffi < 1.17.0`)

**Kitchen drivers (all version-pinned):**

| Gem | Version | Notes |
|-----|---------|-------|
| `kitchen-dokken` | 2.22.2 | From Stromweld fork (matches CW 25.x temporary override) |
| `kitchen-vagrant` | 2.2.0 | |
| `kitchen-ec2` | 3.22.1 | |
| `kitchen-azurerm` | 1.13.6 | |
| `kitchen-google` | 2.6.1 | |
| `kitchen-hyperv` | 0.10.3 | |
| `kitchen-vcenter` | 2.12.2 | Modern vSphere driver (replaces kitchen-vsphere) |
| `kitchen-vra` | 3.3.3 | |
| `kitchen-openstack` | 6.2.1 | |
| `kitchen-digitalocean` | 0.16.1 | |

**Verifier:**
- `kitchen-inspec:3.1.0` — primary verifier

**Legacy busser verifier (for older cookbook repos):**
- `busser:0.8.0`, `busser-serverspec:0.6.3`, `busser-bats:0.5.0` — installed with `--force` because busser 0.8.0 requires `thor <= 0.19.0` which conflicts with `thor 1.4.0` needed by everything else. Safe because busser uses thor only for its own CLI and TK manages busser internally.

### 3. Replaced kitchen-vsphere with kitchen-vcenter

The rubygems `kitchen-vsphere` gem (0.2.0, 2015) requires `test-kitchen ~> 1.0` — fundamentally incompatible with TK 3.x at both the dependency and runtime API level. `kitchen-vcenter` (2.12.2) is the maintained replacement using `rbvmomi2` and the vSphere REST API. Chef Workstation ships it.

### 4. Removed busser as default, re-added with --force

Initially removed busser entirely (Chef Workstation doesn't ship it), then re-added with `--force` after confirming older cookbook repos still need `busser-serverspec` and `busser-bats` test suites.

### 5. Build-Time Version Assertions

Added a Ruby script in the Dockerfile sanity check step that asserts exact versions of `cookstyle`, `test-kitchen`, `inspec-core`, `kitchen-inspec`, and `ffi` — the build fails immediately on version mismatch rather than producing a broken image.

### 6. Added Runtime Dependencies

Added `libreadline8` to the runtime stage for pry/irb in inspec shell. Also added sanity checks for all kitchen drivers, the kitchen-inspec verifier, and busser plugins.

### 7. Updated All 3.2.0 → 3.1.0 References

All gem path references (`gems/3.2.0` → `gems/3.1.0`), `RUBYLIB` paths, `GEM_HOME`/`GEM_PATH`, and binstub paths updated across Dockerfile, Makefile, spec, and todo files.

## Final State

- `go build ./...` — clean
- `go vet ./...` — clean (no Go code changed)
- Dockerfile: 7-phase gem install with pinned versions + build-time assertions
- Makefile: `_build-embedded` target matches Dockerfile gem list exactly

## Known Gaps

- **Docker image not yet built** — the Dockerfile has not been tested with `docker build` against a real build. The gem version pins and sanity checks should catch issues at build time, but first build may surface transitive dependency surprises.
- **Embedded Ruby Environment todo items** — 10 items in `todo/packaging.md` under "Embedded Ruby Environment" are implemented in the Dockerfile/Makefile but still unchecked (they describe the `make build-embedded` standalone flow which mirrors the Dockerfile).
- **kitchen-dokken from git** — uses `gem specific_install` from the Stromweld fork. If that repo goes away or changes branch, the build breaks. Consider pinning to a specific commit SHA.

## Files Modified

### Production
- `Dockerfile` — Ruby 3.1, 7-phase pinned gem install, busser with --force, version assertions, all sanity checks
- `Makefile` — `RUBY_BUILD_IMAGE` → `ruby:3.1-bookworm`, `_build-embedded` target with pinned gems

### Documentation
- `.claude/specifications/packaging/Specification.md` — §4.2 Dockerfile example rewritten, §4.5 new "Gem Version Pinning" and "Kitchen Drivers and Verifiers" sections including legacy busser, §4.6.1 embedded table updated, §9.1 build-embedded description updated
- `.claude/specifications/todo/packaging.md` — container image items updated with implementation details

## Recommended Next Steps

### 1. Bootstrap React Frontend + Core Dashboard Views (large, ~40k tokens)

**Why:** The backend is fully wired end-to-end — all REST handlers serve real data from the datastore, the WebSocket event hub is running, the collection pipeline flows from Chef server through analysis to persistence. There is no frontend at all (`frontend/` directory does not exist). Building the UI is the highest-value next step because it lets you point the tool at a real Chef server and see actual data.

**Read specs:**
- `visualisation/Specification.md` — TL;DR + Dashboard Views section (§ Chef Client Version Distribution, § Cookbook Compatibility Status, § Node Upgrade Readiness)
- `web-api/Specification.md` — TL;DR + endpoint signatures for the endpoints you'll call

**Read todo:** `todo/visualisation.md` — first ~25 items (Dashboard section)

**Scope — build these in order:**

1. **Scaffold `frontend/`** — Vite + React + TypeScript. Must produce `frontend/package.json` so the Dockerfile's conditional `npm ci && npm run build` step activates. Use a simple CSS framework (Tailwind or similar) — no heavy component library needed yet. Add a `frontend/src/api.ts` module wrapping `fetch()` calls to `/api/v1/*` with typed responses.

2. **Layout shell** — App shell with sidebar/top-nav linking pages. Organisation selector dropdown populated from `GET /api/v1/organisations`. Selected org stored in React state/context and passed as `?organisation=` query param to all API calls.

3. **Dashboard page** — Three summary cards/panels:
   - **Version distribution** — `GET /api/v1/dashboard/version-distribution` → bar chart or table of Chef Client versions vs node counts
   - **Node readiness** — `GET /api/v1/dashboard/readiness` → ready/blocked/stale counts per target Chef version
   - **Cookbook compatibility** — `GET /api/v1/dashboard/cookbook-compatibility` → compatible/incompatible/untested/cookstyle-only counts

4. **Nodes list page** — Paginated table from `GET /api/v1/nodes?page=&per_page=` with filter dropdowns for environment, platform, chef_version, stale. Each row links to node detail (`GET /api/v1/nodes/:org/:name`). Colour-code stale nodes.

5. **Cookbooks list page** — Table from `GET /api/v1/cookbooks` showing name, version, source, compatibility status per target version. Colour-code: green (TK pass), amber (CookStyle-only), red (incompatible), grey (untested). Link to cookbook detail.

6. **Health indicator** — Small status badge calling `GET /api/v1/health` on an interval, showing green/red for DB connectivity.

**Existing backend endpoints that already return real data (all implemented with tests):**
- `GET /api/v1/organisations` — org list
- `GET /api/v1/nodes` — paginated nodes with query filters
- `GET /api/v1/nodes/:org/:name` — node detail + readiness
- `GET /api/v1/nodes/by-version/:version` — nodes by Chef version
- `GET /api/v1/nodes/by-cookbook/:name` — nodes using a cookbook
- `GET /api/v1/cookbooks` — cookbook list
- `GET /api/v1/cookbooks/:name` — cookbook detail + analysis results
- `GET /api/v1/dashboard/version-distribution` — version distribution data
- `GET /api/v1/dashboard/readiness` — readiness summary
- `GET /api/v1/dashboard/cookbook-compatibility` — compatibility summary
- `GET /api/v1/filters/*` — filter option values (environments, roles, platforms, etc.)
- `GET /api/v1/health` — health check with DB ping
- `GET /api/v1/version` — version string

**Frontend serves from Go binary:** The Dockerfile already has a conditional frontend build step. The `handleFrontendFallback` in `router.go` currently returns plain text — once `frontend/build/` or `frontend/dist/` exists, wire it up via `go:embed` to serve the SPA. This can be a follow-up or done as part of this task.

**What NOT to build yet:** Dependency graph visualisation, remediation diff viewer, export/notification UI, auth login flow, WebSocket real-time updates, historical trend charts. These are all important but the core data views come first.

### 2. RPM/DEB Packaging (medium, ~20k tokens)
- `nfpm.yaml`, systemd unit, install scripts, `make package-rpm`/`make package-deb`
- **Spec:** `packaging/Specification.md` §2, §3, §9
- **Todo:** `todo/packaging.md` — RPM + DEB + Embedded Ruby sections

### 3. Helm Chart Templates (medium-large, ~30k tokens)
- 30 items in `todo/packaging.md` under Helm Chart
- **Spec:** `packaging/Specification.md` §6

### 4. Wire `go:embed` for Frontend Assets (small, ~5k tokens)
- After the React frontend exists, embed `frontend/dist/` into the Go binary
- Update `handleFrontendFallback` in `router.go` to serve from embedded FS
- Can be combined with task 1 or done separately