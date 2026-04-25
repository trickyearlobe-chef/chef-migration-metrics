# Tech Debt

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Bugs

- [ ] Complexity trend card shows only today's data — backend queries live cookbook complexity instead of `metric_snapshots`; no `complexity_summary` snapshots are written during collection; frontend synthesises fake timestamps. Fix: add snapshot writes in collector, update `handleDashboardComplexityTrend` to read from `metric_snapshots`, add `completed_at` to `ComplexityTrendPoint` type. Files: `handle_dashboard_trends.go` L27–85, `types.ts` L155–165, `TrendCards.tsx` L206–223.
- [ ] Readiness trend card uses fake timestamps despite backend sending real ones — `ReadinessTrendPoint` in `types.ts` L138–145 is missing `completed_at` field; frontend synthesises `Date.now()`-based timestamps (`TrendCards.tsx` L108–132); stale comment on L113 claims endpoint doesn't return timestamps but it does now. Fix: add `completed_at` to type, use real timestamps like version-distribution and stale cards do.
- [ ] `CompatibilityBadge` has dead branch — both arms of the `confidence` ternary produce `"Compatible"` (`StatusBadge.tsx` L171–176).

## Frontend — Large Files

- [ ] `api.ts` (~2230 lines) exceeds the 500-line guideline — split by domain (nodes, cookbooks, owners, admin, auth, kitchen, exports).
- [ ] `types.ts` (~1585 lines) exceeds the 500-line guideline — split to match `api.ts` domains.
- [ ] `NodeDetailPage.tsx` (~1136 lines) contains 10+ sub-components — extract `DiskSpacePanel`, `CookbookCompatibilityTable`, `ReadinessCard`, `ReadinessSection`, `InfoCard` into separate files.
- [ ] `DependencyGraphPage.tsx` (~1646 lines) — extract force-directed simulation, table view, and selected-node panel into separate files.
- [ ] `StatusCards.tsx` (~860 lines) — 7 cards each repeat the same fetch-load-error pattern; extract a `useFetch<T>` hook to eliminate boilerplate.

## Frontend — Inconsistency

- [ ] `RemediationPage.tsx` uses hand-rolled sort logic (L67–68, L151–164) instead of `useSort` hook + `SortableColumnHeader` that every other sortable page uses.
- [ ] `DownloadStatusBadge` (CookbooksPage) and `CloneStatusBadge` (GitReposPage) are near-identical — unify into a shared component.
- [ ] ~25 mutation functions in `api.ts` duplicate a 12-line error-handling pattern instead of using `apiFetch` — extend `apiFetch` to support void responses, then migrate.
- [ ] Dead CSS badge classes in `index.css` (`.badge-compatible`, `.badge-incompatible`, `.badge-untested`, `.badge-stale`, `.badge-ready`, `.badge-blocked`) are superseded by `StatusBadge` component — only `.badge-cookstyle` is still used. Remove dead classes.

## Backend — Code Smells

- [ ] `DataStore` interface has 138 methods (`webapi/store.go`) — consider splitting into domain-specific sub-interfaces (nodes, cookbooks, kitchen, auth, config, etc.) composed into the full interface.
- [ ] `fetcher.go` has ~300 lines marked `//nolint:unused` — cookbook download-from-Chef-Server code that was superseded by `server_cookbook_pipeline.go`. Decide: wire it in or delete it.
- [ ] `ResetAllResults()` in `analysis/kitchen.go` L905–909 returns a hardcoded `"not implemented"` error — implement or remove.
- [ ] Git repo exclusion handlers (`handleListExcludedGitRepos`, `handleGitRepoExclude`, `handleGitRepoClearExclusion`) live in `handle_kitchen_batches.go` L379–459 instead of `handle_git_repos.go` — move to correct file.
- [ ] Duplicate `type orgResult struct` definitions in `collector.go` at L439–444 and L580–585 — extract to a single package-level type.

## Database

- [ ] Migrations 0001–0009 establish natural composite keys; migrations 0013–0016 reintroduce UUID PKs for `vm_tracking`, `node_kitchen_runs`, `kitchen_batches`, `git_kitchen_results`. This is a deliberate choice (these tables model ephemeral operational records, not domain entities) but should be documented in `project-conventions.md` under a "Primary Key Strategy" section.

## Phasing Notes

These are not debt — they are deliberate holds awaiting prerequisites.

- Batch kitchen executor wiring (Router field, `WithBatchExecutor` option, `main.go` instantiation, background goroutine in `handleRunKitchenBatch`) is deliberately deferred until manual "node kitchen" and "git kitchen" triggers are validated in production. Bulk scanning without confidence in single-run reliability risks orphaned VMs in vSphere. The `batch.Executor`, `KitchenRunner`, and `Resolver` are implemented and tested — only the HTTP→Executor connection is held.
- SAML authentication endpoints return 501 — waiting for customer environment access to test.
- Notification subsystem (`internal/notify/`) not yet implemented — entire feature deferred.
- TLS ACME mode logged as "not yet implemented" in `main.go`.

## Housekeeping

- [ ] Delete stale local branches: `dependabot/go_modules/github.com/lib/pq-1.12.0`, `dependabot/npm_and_yarn/frontend/dev-tooling-75e389f64b`, `dependabot/npm_and_yarn/frontend/react-3a0834b017`, `dependabot/npm_and_yarn/frontend/react-router-dom-7.13.2`, `fix/purge-metric-snapshots-on-version-removal`.