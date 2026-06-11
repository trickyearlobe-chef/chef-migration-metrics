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

## Chunk D — `collection.schedule` reschedule applier [subsystem] ✅ DONE

First scheduler-reschedule applier; established the `Reschedule` + signal-the-loop
pattern the backup reschedule applier reuses. `Scheduler.Reschedule(CronParser)`
swaps `s.schedule` under `s.mu` and signals a buffered `reschedule chan struct{}`;
the loop reads `s.schedule` under lock each iteration and a new `<-s.reschedule`
select case stops the pending timer and recomputes `Next` from the new schedule.
webapi `collectionScheduleApplier` (subsystem) registered only when
`WithCollectionRescheduler` is wired — a `func(string) error` callback, so webapi
still doesn't import `collector`. main.go wires it via `ParseSchedule` +
`sched.Reschedule` (sched non-nil by Phase 16; scheduler starts Phase 14). Collection
section reports subsystem/false when wired, applied/false otherwise (thresholds always
apply live). Two scheduler tests (live swap + before-Start) `-race` green; two webapi
tests (subsystem reload + applier-error 500). Full collector + webapi suites + vet green.

## Chunk E — backup reschedule/start/stop applier [subsystem] ✅ DONE

Reused Chunk D's `Reschedule` + signal-the-loop pattern, plus an `enabled`
start/stop lifecycle the collector didn't need (backup scheduler is nil when
disabled at boot). `backup.Scheduler` gained `mu sync.Mutex` + buffered
`reschedule chan` + `Reschedule(collector.CronParser)`; the loop reads
`s.schedule` under mu each iteration and a new `<-s.reschedule` select case stops
the timer and recomputes. webapi `backupApplier` (subsystem) registered only when
`WithBackupReconciler` is wired — a parameterless `func() error` (not a
value-passing callback): the reconciler reads the reloaded holder and resolves the
`BackupSchedule()` default itself, so no "0 2 * * *" literal is duplicated in
webapi/main. main.go wires it inside the backup block; the closure reconciles
`app.backupSched` (start when enabled+nil, reschedule when enabled+running, stop+nil
when disabled), guarded by a new `app.backupMu` (also now guards the restore hook's
access). Backup section reports subsystem/false when wired, process/true otherwise.
Two backup `-race` tests (live swap + nil-ignored); three webapi tests (process
default, subsystem reload, reconciler-error 500). Full backup (incl. `-race`) +
webapi + cmd suites + `go vet` green.

## Chunk F — exports `appliedApplier` + dead-param cleanup [applied] ✅ DONE

**Trace correction (supersedes the todo's "re-point cleanup ticker" premise):**
`CleanupExpiredExports` deletes each file by its stored per-job `FilePath`
(absolute, computed from the live `outputDir` at job creation, `handle_exports.go:215`),
NOT by joining a dir. The `outputDir` param threaded through `CleanupExpiredExports`
and `StartCleanupTicker` is read nowhere — vestigial. The ticker was never stale
w.r.t. `output_directory`. After Chunk A's read swaps the exports section is already
fully live (handlers read `RetentionHours`/`OutputDirectory` live; export writers
`MkdirAll` on demand). Only defect left: it still *reports* `process/true` (no-applier
default) when it's `applied/false`.

### Scope
- `internal/webapi/handle_admin_config_exports.go` — replace the "no applier yet"
  comment + bare `storeAdminConfigSection(...)` with `appliedApplier`.
- `internal/export/cleanup.go` — drop the unused `outputDir` param from
  `CleanupExpiredExports` and `StartCleanupTicker`.
- `cmd/chef-migration-metrics/main.go:1086` — drop the `exportOutputDir` arg from the
  ticker call (boot `MkdirAll`+log of the default dir at :1063–1071 stays).
- `internal/export/export_test.go` — drop the dir arg from the 3 `CleanupExpiredExports` calls.

### Steps (TDD)
1. Add `TestAdminConfigExports_PUT_AppliedReload` (handle_admin_config_test.go): PUT a
   valid exports body, assert 200, `restart_required:false`, `Reload == applied`.
   Red before (process/true), green after.
2. Register `appliedApplier`; update the 3 export test calls + signatures.
3. `go test ./internal/webapi/... ./internal/export/...` + `go vet` green.

### Acceptance
- Exports section reports `applied`/false. New test red→green.
- No `outputDir` param on the two cleanup funcs; full module suite + vet green.

## Chunk G — concurrency live-read appliers [applied] ✅ DONE

**Trace correction (supersedes the todo's "[subsystem] resize, mirror SetWorkerCount"
premise):** the cookstyle scanner and readiness evaluator build a *fresh* semaphore
(`make(chan struct{}, concurrency)`) at the start of each batch — there is no
persistent pool to resize (unlike the kitchen queue). The bug was purely that
`concurrency` was snapshotted at construction, so the collector's run-start `c.cfg`
refresh never reached it. Fix = read the value live at batch start (the same
pull-per-run model the collector already uses for org_collection/node_page/git_pull),
which is **applied** granularity, not subsystem.

Done: `WithCookstyleConcurrencyFunc` + `effectiveConcurrency()` (cookstyle.go);
`WithReadinessConcurrencyFunc` + `effectiveConcurrency()` (readiness.go);
`RunnerFactory.Concurrency int` → `ConcurrencyFn func() int` resolved per-Run
(nodekitchen/factory.go — node-kitchen factory was the lone baked `cookbook_download`
consumer; the collector's own `cookbook_download`/`git_pull` reads were already live).
main.go wires all three from `app.configHolder.Get().Concurrency.*`. webapi concurrency
section registers `appliedApplier` → applied/false (was the pessimistic process/true).
Unit tests per component (live override + <1 fallback) + webapi applied-reload flip
test. Full module suite + vet green.

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
