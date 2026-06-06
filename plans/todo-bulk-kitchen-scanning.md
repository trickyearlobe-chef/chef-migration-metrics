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

## VM start-rate limiter (active — Chunk 2, core deliverable)

- [ ] TDD global rate limiter: sliding window, evenly paced (min inter-start gap ≈ window/max)
- [ ] Config `window` (= DHCP lease time) + `max_starts_per_window` (= pool size); both dynamic (live accessor, no restart)
- [ ] Gate VM start at the worker layer before boot; counts starts, charges full window regardless of early finish/release
- [ ] Limiter is independent of IP release working (hard worst-case guarantee)
- [ ] Confirm: single global window/max, or per-scope if subnets differ in lease time / pool size (open question)

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

### App-injected IP-release hook (pre_destroy) — opt-in spike, AFTER rate limiter

Opportunistic optimisation, NOT the pool guarantee (rate limiter is). Spike — validate
empirically on customer OS mix before relying on it.

- [ ] Opt-in per platform/image, default off; config dynamic (live accessor, no restart)
- [ ] Failure-isolated: command always exits 0 (missing/variant binary, non-zero result must never abend the run — a non-zero kitchen hook aborts the action and leaks the VM+lease)
- [ ] Detached from transport (`setsid`/`nohup`, backgrounded, redirected) so a dropped link is not a hook failure (note: races hypervisor power-off — verify empirically)
- [ ] Per-platform command: Linux tolerant of `dhclient`/`dhcpcd`/`networkctl`/`nmcli` absence; Windows `ipconfig /release`; OS family from resolved image/platform entry
- [ ] Compose with any repo-provided `pre_destroy` hook (see preservation item above)
- [ ] Verify TK lifecycle-hook failure semantics for the installed kitchen version (non-zero `remote:` hook abort? transport drop = failure?)

## Tests

- [ ] Unit tests for rate limiter — never exceeds max per trailing window; paced gap; dynamic window/max change mid-run
- [ ] Unit tests for global concurrency dynamic change (`SetWorkerCount` on live config)
- [ ] Unit tests for orphan sweep — scoping rules (folder, prefix, age threshold)
- [ ] Unit tests for overlay lifecycle-hook composition — repo hooks preserved, CMM `pre_destroy` injected, arrays merged not clobbered
- [ ] Unit tests for IP-release hook failure isolation — simulated hook failure/transport drop leaves result unchanged and VM destroyed
