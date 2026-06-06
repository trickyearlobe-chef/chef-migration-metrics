# Bulk Kitchen Scanning — ToDo

Spec: `bulk-kitchen-scanning.md`

## Scheduler Refactor (prerequisite)

The batch scheduling logic needs a design pass before wiring. Current state is messy. Key design concerns:

- [ ] Review and clarify responsibilities between `batch.Resolver`, `Scheduler`, and the batch run handler — boundaries are currently blurry
- [ ] Add a **rate limiter / time-window throttle** to cap how many VM slots can be started within a rolling time window (e.g. max N new VMs per minute). This is distinct from `max_concurrent_vms` (which caps simultaneous running VMs) — the rate limiter prevents flooding the DHCP pool when many VMs are starting/finishing in rapid succession
- [ ] Design the rate limiter as a configurable parameter: `test_kitchen.vm_start_rate_per_minute` (or similar) — default conservative (e.g. 2/min)
- [ ] Document the interaction between `max_concurrent_vms`, `vm_start_rate`, and `network_timeout` detection in `bulk-kitchen-scanning.md`

## Run → Scheduler Wiring (core stub)

- [ ] Implement `Scheduler.RunBatch(ctx, []PlanResult, config, progressCallback)` — flattens instances across repos into one work queue with shared semaphore
- [ ] Wire `handleRunKitchenBatch` to call `RunBatch` in a detached background goroutine
- [ ] Store cancel func in router-level `map[string]context.CancelFunc` (mutex-protected)
- [ ] Gate both `handleGitKitchenRun` and `handleRunKitchenBatch` on `tkCfg.IsEnabled()` — return 409 when disabled
- [ ] Enforce single running batch constraint — return 409 if another batch is already `running`
- [ ] On completion/cancellation, transition batch to `completed`/`cancelled`, set `completed_at`, broadcast `batch_complete` WebSocket event

## Cancellation

- [ ] Wire `handleCancelKitchenBatch` to call the stored cancel func (currently sets DB state only, no goroutine stop)

## Progress Endpoint

- [ ] Restore `GET /kitchen/batches/:id/progress` — derive counts from `git_kitchen_results` rows matching the batch's resolved cookbook list
- [ ] Broadcast `batch_progress` WebSocket event after each instance completes (batch_id, instance_name, git_repo_name, passed, completed, total)

## Restart Resilience

- [ ] On startup, transition any batch stuck in `running` to `cancelled` with note "interrupted by restart"

## Orphan Sweep

- [ ] Implement scheduled orphan sweep background goroutine (interval from `test_kitchen.orphan_sweep_interval`, default 30m)
- [ ] Implement `POST /api/v1/kitchen/orphan-sweep` manual trigger with `?dry_run=true` support
- [ ] Extend `ListManagedVMs` with optional folder filter parameter (vSphere `?folders=` param)
- [ ] Scoping: only touch VMs in configured `driver_settings.targetfolder`, matching prefix, older than age threshold
- [ ] Inter-instance orphan sweep: after a `timed_out`/`network_timeout` instance, sweep that VM before starting next
- [ ] Sweep reports: log at INFO, broadcast `orphan_sweep_complete` WebSocket event, expose on admin TK config page ("last sweep: N VMs cleaned, time ago")
- [ ] Sweep does nothing when TK is disabled or no hypervisor configured

## DHCP / Network Timeout Classification

- [ ] Classify kitchen timeout with no converge output as `network_timeout` (not test failure)
- [ ] Store `error_message: "probable DHCP/network timeout"` on result
- [ ] Log WARN per platform — repeated `network_timeout` on same platform → DHCP pool likely exhausted

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

### App-injected IP-release hook (pre_destroy)

- [ ] Inject a `pre_destroy` lifecycle hook into the overlay that releases the DHCP lease on the target before destroy
- [ ] Run remotely over the existing transport (`remote:` command), not on the CMM host
- [ ] Per-platform command: Linux (e.g. `dhclient -r`) / Windows (`ipconfig /release`) — derive OS family from resolved image/platform entry
- [ ] Best-effort: a failed release MUST NOT block or fail `kitchen destroy`
- [ ] Compose with any repo-provided `pre_destroy` hook (see preservation item above)

## Tests

- [ ] Unit tests for `RunBatch` — concurrency, cancellation, progress callback
- [ ] Unit tests for orphan sweep — scoping rules (folder, prefix, age threshold)
- [ ] Unit tests for `network_timeout` classification
- [ ] Unit tests for startup cancelled-batch recovery
- [ ] Unit tests for overlay lifecycle-hook composition — repo hooks preserved, CMM `pre_destroy` injected, arrays merged not clobbered
- [ ] Unit tests for per-platform IP-release command selection (Linux vs Windows)
