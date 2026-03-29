# Changelog

All notable changes to Chef Migration Metrics are documented in this file.
Generated from git tag history. Format follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Fixed
- Re-enable errcheck linter and fix all 50 violations.

### Changed
- Extract ownership filter helpers, shared `useSort` hook, `SortableColumnHeader`, `useTargetChefVersion` hook.
- Replace substring cookbook matching with JSON parse.
- Descope `internal/notify/` — mark notification items as future/planned.

## [v2.2.8] — 2025

### Changed
- Bulk-load all lookup data into in-memory cache for readiness evaluator.

### Added
- Bulk-load query methods for readiness evaluator.

## [v2.2.7] — 2025

### Fixed
- Persist readiness results with background context.

## [v2.2.6] — 2025

### Fixed
- Look up node readiness by `(org, name)` instead of snapshot ID.

## [v2.2.5] — 2025

### Added
- Real-time log streaming via WebSocket (subscribe/unsubscribe, severity filtering).
- Clone Status filter dropdown on git repos page.
- Show why git repos are untested on dashboard.

### Changed
- Simplify git_repos to one row per name.
- Determine git repo compatibility from CookStyle results.

### Fixed
- Enforce one git repo row per cookbook name.

## [v2.2.4] — 2025

### Changed
- Cookbooks page shows per-version rows (removed collapsing).
- Exclude git repos from server cookbook list page.

## [v2.2.3] — 2025

### Fixed
- Use CookStyle results directly for cookbook compatibility.

## [v2.2.2] — 2025

### Added
- Configurable cookbook download worker count.

### Fixed
- Deduplicate node snapshots before bulk upsert.

## [v2.2.1] — 2025

### Changed
- Run server cookbook, git repo, and analysis pipelines concurrently.

## [v2.2.0] — 2025

### Changed
- Concurrent download-and-scan pipeline for server cookbooks.

### Fixed
- Scan already-downloaded cookbooks that haven't been analysed.

## [v2.1.0] — 2025

### Added
- Track git repo clone failures and cookbook download errors in UI.
- Sortable column headers on list pages (nodes, cookbooks, git repos).
- Server-side sort support for nodes, cookbooks, and git repos APIs.
- Untested breakdown (inactive vs unscanned) on dashboard compatibility cards.
- Test Kitchen status filter and dashboard clickthroughs.
- Clear-filters buttons on list pages.
- Trigger immediate collection run on rescan requests.

### Fixed
- Shared Pagination component and URL param clearing.
- Dashboard card consistency cleanup.
- Skip complexity scoring for unscanned cookbooks.

## [v2.0.1] — 2025

### Added
- Admin System Stats page with host-level system health metrics.
- Per-table database size breakdown in system health endpoint.

### Fixed
- Handle `platform=unknown` and `chef_version=unknown` filters.
- Download server cookbooks to persistent cache directory.

## [v2.0.0] — 2025

### Added
- Dashboard split into "Current Status" and "Trends" tabs.
- Test Kitchen compatibility summary card on dashboard.
- Node disk detail page with filesystem table.
- Dashboard readiness counts link to filtered nodes page.
- React ErrorBoundary to prevent white-screen crashes.
- Metric snapshots for dashboard trend data.
- Periodic expired session cleanup.
- Platform coverage analysis with fuzzy matching.
- Credential management admin page and API.
- Performance diagnostics dashboard with PostgreSQL stats views.
- CookStyle error handling for exit code 2 (crash detection).
- Pluggable Test Kitchen driver architecture (replaces hardcoded Dokken).

### Changed
- Flatten specifications directory structure.
- Consolidate all schema migrations into 0001.
- Decompose `run()` into `serverApp` with named phases.
- Split `handle_dashboard.go` into focused files.

### Fixed
- Path traversal prevention in cookbook/git-repo filesystem operations.
- Deduplicate multi-version cookbooks in remediation priority list.
- Deprecation warnings no longer mark cookbooks incompatible.
- Orphaned node snapshot cleanup for decommissioned nodes.
- Upsert semantics in entity datastore code.
- Guard role queries against JSONB null values.
- Log swallowed JSON encoding error in `WriteJSON`.
- CookStyle crash (exit code ≥ 2) now detected as error, not incompatibility.

### Removed
- Dead `cookbook_node_usage` table and bloated `node_names` column.
- Unused `notification_history` table.
- Embedded Ruby from Docker image and RPM/DEB (uses Chef Workstation).

## [v1.0.1] — 2025

### Added
- Security scanning in CI pipeline.

## [v1.0.0] — 2025

### Added
- Streaming server cookbook pipeline — download, scan, delete one at a time.
- `skip_server_cookbook_download` config option.

### Changed
- Complete `server_cookbooks`/`git_repos` split — backend and frontend.

### Fixed
- Cookbook list version count excludes git repos.
- Rescan handlers reset `download_status` for streaming pipeline.
- Keep `config.yml` as `config|noreplace` in RPM.

## [v0.2.4] — 2025

### Changed
- Scale optimisations for 100K+ nodes.
- Configurable cookbook and git storage paths.

### Fixed
- Prevent RPM upgrade from clobbering `config.yml`.

## [v0.2.3] — 2025

### Fixed
- Derive session cookie `Secure` flag from request TLS state.

## [v0.2.2] — 2025

### Added
- Embed SQL migrations into the binary.

### Fixed
- O(N²) query performance in node readiness dashboard.

## [v0.2.1] — 2025

### Added
- Cookbook detail page improvements.

## [v0.2.0] — 2025

### Changed
- Remove embedded Ruby from Docker image and RPM/DEB; use Chef Workstation.

## [v0.1.8] — 2025

### Added
- Arm64 builds optional via `BUILD_ARM64` repository variable.

## [v0.1.7] — 2025

### Fixed
- Use `sudo chmod` for root-owned embedded Ruby files in CI.

## [v0.1.6] — 2025

### Fixed
- `chmod` embedded Ruby tree after Docker build for nFPM packaging.

## [v0.1.5] — 2025

### Fixed
- Use `config.yml.example` in nfpm packaging (`config.yml` is gitignored).

## [v0.1.4] — 2025

### Added
- Container image build optional via `BUILD_CONTAINER_IMAGE` variable.

## [v0.1.3] — 2025

### Fixed
- Add `helm dependency build` step before packaging in release workflow.

## [v0.1.2] — 2025

### Fixed
- Parallelize release workflow so packages don't wait for container image.

## [v0.1.1] — 2025

### Fixed
- Use correct `go build` path for RPM/DEB packages.

## [v0.1.0] — 2025

### Added
- Sortable owners table with inline readiness bars.
- Complete Reset Git feature and fix git repo URL storage.

### Fixed
- Resolve CI lint failures.

## [v0.0.18] — 2025

### Fixed
- Helm v4 compatibility.
- Only download cookbooks used by active nodes; complete run early for UI.
- ESLint 9 flat config and lint errors.

## [v0.0.17] — 2025

### Fixed
- Use Docker format for per-arch image exports so `docker load` works.

## [v0.0.16] — 2025

### Added
- `cookstyle_enabled` option to disable CookStyle scans.
- `ssl_verify` option to disable TLS verification per organisation.
- Docker export target for airgap environments.

## [v0.0.15] — 2025

### Fixed
- All staticcheck findings resolved.
- Remove unused types, functions, and methods.

### Changed
- Disable errcheck linter temporarily (tracked for re-enablement).

## [v0.0.14] — 2025

### Added
- CookStyle rescan endpoint and UI button.
- Pass `--target-chef-version` to CookStyle.

### Fixed
- Convert `ohai_time` from unix seconds to JS milliseconds.
- Correct CookStyle cop namespace prefixes and target version mechanism.
- Deduplicate git cookbooks by name.
- Use `--json` flag for `kitchen list`; sanitise Bundler environment.
- Log organisation name instead of UUID in readiness evaluation.
- Remove stale directory before git clone.

## [v0.0.13] — 2025

### Added
- Helm chart for Kubernetes deployment.

### Fixed
- Install Node.js 20 from NodeSource instead of Debian's Node 18.

## [v0.0.12] — 2025

### Fixed
- Add committed example config for distribution archives.

## [v0.0.11] — 2025

### Fixed
- Add `nfpm.yaml` and packaging files; fix Makefile `build-embedded`.
- Pin dry-rb gems for Ruby 3.1 compatibility.

## [v0.0.10] — 2025

### Fixed
- Create placeholder `frontend/dist` before `golangci-lint`.

## [v0.0.9] — 2025

### Changed
- Remove duplicate lint and test jobs from release workflow.

## [v0.0.8] — 2025

### Fixed
- Add placeholder test script and node engine constraint for frontend.

## [v0.0.7] — 2025

### Fixed
- Eliminate data race in TLS test log collector.
- Eliminate data race in `InMemoryCredentialStore.Get/GetMetadata`.

## [v0.0.6] — 2025

### Fixed
- Create placeholder `frontend/dist` in release workflow test job.

## [v0.0.5] — 2025

### Fixed
- Push branch before tag in Makefile `_push-tag` target.

## [v0.0.4] — 2025

### Fixed
- Create placeholder `frontend/dist` before `go test` in CI.
- Scope `embedded/` gitignore pattern to top-level only.

## [v0.0.3] — 2025

### Fixed
- Build `golangci-lint` from source to match Go 1.25.4.

## [v0.0.2] — 2025

### Changed
- Disable gitleaks secret scanning in CI and release workflows.

## [v0.0.1] — 2025

### Added
- Initial release with cross-platform distribution archives.
- Login page, admin users page, and protected routing.
- Authentication middleware and session management.
- Pre-commit hook and CI secret scanning.
- CookStyle and Test Kitchen analysis.
- Node readiness evaluation.
- Chef API client with partial search support.
