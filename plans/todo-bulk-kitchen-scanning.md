# Bulk Kitchen Scanning — ToDo

Spec: `bulk-kitchen-scanning.md`

## Run → Scheduler Wiring (DONE — already implemented via queue)

Implemented via DB-backed `kitchenqueue.Manager`, not the `Scheduler.RunAll` the spec
imagined. Verified present in code: batch run launches a detached `executeBatch` goroutine
that enqueues mapped instances; `IsEnabled()` 409 gate; single-running-batch guard;
cancel-func map + `handleCancelKitchenBatch` stops in-flight work; `GET …/progress`;
`batch_progress`/`batch_complete` events; startup `CancelStaleBatches` restart recovery.
No wiring, cancellation, progress-endpoint, or restart-resilience work remains.

## Concurrency cleanup (Chunk 1 — DONE)

- [x] Remove per-batch `max_concurrent_vms` from data model, API/handlers, and batch UI (dead knob; concurrency is global only) — migration 0036
- [x] Resolve global default inconsistency (comment "10" vs `EffectiveMaxConcurrentVMs` 4) — `DefaultMaxConcurrentVMs = 2`, comment matches code
- [x] Confirm global concurrency change is dynamic — `SetWorkerCount` scale up/down tested; live-config wiring in `handle_admin_config_analysis.go`

## VM start-rate limiter (Chunk 2 — DONE)

- [x] TDD global rate limiter: sliding window, evenly paced (min inter-start gap ≈ window/max) — `ratelimiter.go`
- [x] Config `window` + `max_starts_per_window`; both dynamic (live accessor, no restart) — disabled unless both > 0
- [x] Gate VM start at the worker layer before boot; counts starts, charges full window regardless of early finish/release
- [x] Limiter is independent of IP release working (hard worst-case guarantee)
- [x] Confirmed: single global window/max. Per-scope rate limiting DEFERRED — revisit only if the customer's subnets prove to have materially different lease times / pool sizes; would need an instance→scope mapping that does not exist yet.
- [x] Admin TK config UI exposes window + max-per-window

## Orphan Sweep

- [ ] Implement scheduled orphan sweep background goroutine (interval from `test_kitchen.orphan_sweep_interval`, default 30m)
- [ ] Implement `POST /api/v1/kitchen/orphan-sweep` manual trigger with `?dry_run=true` support
- [ ] Extend `ListManagedVMs` with optional folder filter parameter (vSphere `?folders=` param)
- [ ] Scoping: only touch VMs in configured `driver_settings.targetfolder`, matching prefix, older than age threshold
- [ ] Inter-instance orphan sweep: after a `timed_out`/`network_timeout` instance, sweep that VM before starting next
- [ ] Sweep reports: log at INFO, broadcast `orphan_sweep_complete` WebSocket event, expose on admin TK config page ("last sweep: N VMs cleaned, time ago")
- [ ] Sweep does nothing when TK is disabled or no hypervisor configured

## DHCP / Network Timeout Classification (DONE)

Implemented in `internal/gitkitchen/executor.go` (~:140) — `network_timeout` classification
when a timeout has no converge activity, `error_message: "probable DHCP/network timeout"`.
- [ ] Log WARN per platform — repeated `network_timeout` on same platform → DHCP pool likely exhausted (verify present)

## Frontend

- [ ] Disable "Run" button when TK config `enabled: false` with tooltip "Test Kitchen is disabled"
- [ ] Poll `GET /kitchen/batches/:id/progress` every 5s (or subscribe to `batch_progress` WebSocket events) when batch is `running`
- [ ] Update `BatchProgressBar` in real time
- [ ] Show per-cookbook results table in batch detail (expandable rows, instance-level pass/fail)
- [ ] "Cancel" button sends cancel request, updates UI immediately
- [ ] Show sweep status on admin TK config page

## Lifecycle Hooks

Spec: `test-kitchen-drivers-overlay-generation.md` § Lifecycle Hooks.

### Repo-provided setup hooks (preserve)

- [ ] Confirm overlay merge behaviour for `lifecycle:` — Test Kitchen replaces arrays per phase; verify which phases CMM currently clobbers
- [ ] Overlay generation MUST preserve cookbook-provided lifecycle hooks; only inject phases CMM owns
- [ ] When CMM must inject a phase the cookbook also uses, compose (append) rather than replace — read existing `.kitchen.yml`, merge arrays before writing overlay
- [ ] Document which lifecycle phases CMM reserves vs. leaves to the repo

### App-injected IP-release hook (pre_destroy) — opt-in spike (Chunk 3 — DONE)

Opportunistic optimisation, NOT the pool guarantee (rate limiter is). Spike — validate
empirically on customer OS mix before relying on it.

- [x] Opt-in per image (`ImageEntry.release_ip_on_destroy`), default off; dynamic (live accessor)
- [x] Failure-isolated: command always exits 0 (`overlay.go` `ipReleaseCommand`)
- [x] Detached from transport (`nohup`/`start /b`, backgrounded, stdio redirected)
- [x] Per-platform command: Linux tolerant of `dhclient`/`dhcpcd`/`networkctl`/`nmcli`; Windows `ipconfig /release`; OS family from `analysis.NormalisePlatformName`
- [x] Compose with any repo-provided `pre_destroy` hook (`writeLifecycleHook` + `readExistingPreDestroy`)
- [x] Admin UI per-image opt-in checkbox (`AdminTestKitchenPage.tsx`)
- [ ] EMPIRICAL (remaining): verify TK lifecycle-hook failure semantics for the installed kitchen version (non-zero `remote:` hook abort? transport drop = failure?) and that the release packet leaves the guest before power-off — see `todo-tech-debt.md` § IP-Release pre_destroy Hook

## Tests

- [x] Unit tests for rate limiter — never exceeds max per trailing window; paced gap; dynamic window/max change mid-run; ctx cancel; disabled pass-through (`ratelimiter_test.go`, `manager_test.go` gate test)
- [ ] Unit tests for global concurrency dynamic change (`SetWorkerCount` on live config)
- [ ] Unit tests for orphan sweep — scoping rules (folder, prefix, age threshold)
- [x] Unit tests for overlay lifecycle-hook composition — repo hooks preserved, CMM `pre_destroy` injected, arrays merged not clobbered (`overlay_test.go`, `executor_test.go`)
- [x] Unit tests for IP-release hook failure isolation (by construction) — injected command always `exit 0`, detached, stdio redirected (`overlay_test.go`); live simulated-failure verification remains empirical
