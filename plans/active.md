# Active — config live-reload

Branch: `refactor/config-live-reload`. Source: `todo-configuration.md` Restart/Reload
Audit. Guardrails commit `aa70d9f` (no renames/removals; values byte-identical).

## Chunk A — applied-granularity handler swaps (`r.cfg` → `r.liveConfig()`) ✅ DONE

Done: all 9 handler files swapped; zero non-test `r.cfg.` reads remain in handlers
(router.go setup-time reads intentionally left). Two anchor live-reload tests added
(node staleness tier via list endpoint; performance `window_seconds`) — red before,
green after. `go vet` + full webapi suite green.

## Chunk B — invert `restart_required` (derive, don't declare) ✅ DONE

`postReload` → `Applier`; `restart_required` derived from the worst applier
granularity; sections with no applier default pessimistically to `process`. New
`config_apply.go` (`ReloadGranularity` applied<subsystem<listener<process,
`ApplyResult`, `Applier`, `appliedApplier`, `subsystemApplier`,
`applyKitchenWorkerCount`, `worstGranularity`). `storeAdminConfigSection` rewritten
to take `appliers ...Applier`; `putConfigResponse.Reload` (additive, omitempty)
surfaces the real granularity. Call sites assigned by today's TRUE liveness:
- **applied/false:** collection, target_chef_versions, git_base_urls, readiness.
- **subsystem/false:** organisations (applied + reconcile hook), analysis_tools
  (kitchen pool resize — also closed the full-section PUT's resize gap).
- **process/true, pending subsystem applier:** logging, backup, concurrency,
  exports, auth (auth was already true).
- **untouched:** server/tls (own path, hardcoded true).

New `config_apply_test.go` (granularity ordering + derivation) + handler flip
tests. Full module suite + `go vet` green. Only the collection flag-asserting test
was contract-affected; it stays false.

## Chunk C — `logging.level` SetLevel applier [subsystem] ✅ DONE

First real subsystem applier; established the live-setter + Router-wiring pattern
the later appliers (scheduler reschedule, pool resize) reuse. `Logger.level` →
`atomic.Int32` + `SetLevel` (`Level()`/`log()` read atomically; race-clean).
webapi `logLevelApplier` (subsystem) registered only when `WithLogLevelSetter` is
wired — a `func(string) error` callback, so webapi still doesn't import `logging`.
main.go wires it via `ParseSeverity` + `logger.SetLevel`. Logging section now
reports subsystem/false when wired, process/true otherwise (no-setter contract
tests unchanged). Full logging (incl. `-race`) + webapi suites + `go vet` green.

### Next chunk candidates (later sessions)
- **New appliers (subsystem):** collection.schedule reschedule; backup
  reschedule/start/stop; concurrency.{cookstyle_scan,readiness_evaluation,cookbook_download} resize.
- **Exports cleanup-ticker re-point** (`main.go:1059`) — the subsystem half of the
  exports section (read swaps already done here).

---

### (completed) Chunk A scope

The audit's first step: trivial, independent handler swaps that unblock honest
`restart_required:false` immediately. `liveConfig()` falls back to `r.cfg` when no
holder, so behaviour is identical without a holder — existing tests stay green; the
new value is that a live config change is now reflected without restart.

Out of scope (later chunks): the inverted-applier structural fix; new appliers
(logging SetLevel, cron reschedule, concurrency resize, backup); the exports
cleanup-ticker re-point (`main.go:1059`); `router.go` setup-time reads.

### Scope (files — webapi handler reads only)
- `handle_nodes.go:52,53,144,215` — Collection stale thresholds (**the real bug**:
  Collection is declared live but read static).
- `handle_performance.go:75` — `Performance.WindowSeconds` (echoed in response).
- `handle_system_health.go:24` — `SystemHealth`.
- `handle_admin_users.go:122,302` — `Auth.MinPasswordLength`.
- `handle_saml_admin.go:55` — `Auth.Providers`.
- `handle_auth.go:74,86` — `Server.TrustedProxy`.
- `handle_exports.go:103,108,146,206` — `Exports.{AsyncThreshold,MaxRows,RetentionHours,OutputDirectory}`
  (read swap only; cleanup-ticker applier deferred).
- `handle_git_repos.go:592,604`, `handle_git_repo_files.go:185,198`,
  `handle_cookbook_reset_git.go:92,93` — `Storage.GitCookbookDir` (bootstrap-only;
  harmless consistency swap, leaves zero static `r.cfg` reads in handlers).

### Steps
1. TDD: add live-reload tests proving the swap where assertable — seed `r.cfg` and a
   `ConfigHolder` with **different** values, assert the handler reflects the holder.
   Anchor on `handle_performance` (window echoed) + a node staleness test for the bug.
2. Swap each site `r.cfg.X` → `r.liveConfig().X`.
3. `go test ./internal/webapi/...` green.

### Acceptance
- No non-test `r.cfg.` reads remain in webapi handler files (router.go setup excluded).
- New tests fail before the swap, pass after.
- Full webapi suite green; no behaviour change when no holder is wired.
