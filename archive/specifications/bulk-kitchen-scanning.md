# Bulk Kitchen Scanning (Phase 6)

## Goal

Connect the existing batch management UI/API to the scheduler/executor so that "Run" on a non-dry-run batch actually executes Test Kitchen converge across matched cookbooks in parallel, with live progress, cancellation, and an enabled/disabled toggle.

## Context

### What exists

- **Batch data model** — `kitchen_batches` table with lifecycle (`draft → running → completed/cancelled`), filters (cookbook names, platforms, previous status, exclusions), a `max_count` limit, and a dry-run flag. Concurrency is global, not per-batch (§ below) — there is no per-batch `max_concurrent_vms`.
- **Batch API** — full CRUD, run, cancel endpoints. Run handler validates state and computes estimate but does not invoke the scheduler.
- **Scheduler** — `RunAll(ctx, plan, config, tkConfig, progressCallback)` runs instances in parallel with semaphore-based concurrency. `RunOne` for single-instance. Both persist results via `UpsertGitKitchenResult`.
- **Executor** — `RunInstance` copies repo to temp dir, generates `.kitchen.local.yml` overlay, resolves credentials, runs the kitchen phases `converge → verify → destroy` (not `kitchen test`, whose instance-less initial destroy trips a remote `pre_destroy` hook — see `test-kitchen-drivers-overlay-generation.md`), captures output. `destroy` always runs for teardown.
- **Planner** — `PlanRepo(analysis, platformMap)` expands suites × platforms into `PlannedInstance` list with status classification (mapped/unmapped/skipped/excluded).
- **Batch resolver** — `batch.Resolver.ResolveBatch(filters, maxCount)` expands filters into matching cookbooks with per-platform VM estimates.
- **Frontend** — batch list, create/edit form, detail view with estimate, progress bar component, run/cancel/delete actions.
- **Single-instance run** — `handleGitKitchenRun` is the reference implementation: async goroutine, detached context, WebSocket event on completion.
- **TK config** — `TestKitchenConfig` with `Enabled *bool`, `IsEnabled()`, driver settings, platform map, images. Stored in `runtime_settings` table (DB override) or config file.

### Already implemented

The Run→Scheduler wiring is built — via a DB-backed queue (`kitchenqueue.Manager`), not the `Scheduler.RunAll` this spec originally imagined. `handleRunKitchenBatch` launches a detached `executeBatch` goroutine that enqueues all mapped instances; `handleCancelKitchenBatch` calls the stored cancel func and cancels queued/running items; the `IsEnabled()` 409 gate, single-running-batch guard, `GET …/progress`, and startup `CancelStaleBatches` restart-recovery are all present. References describing a stubbed scheduler are historical.

## Behaviour

### Enabled gate

Both `handleGitKitchenRun` (single instance) and `handleRunKitchenBatch` (batch) resolve the effective TK config and check `tkCfg.IsEnabled()`. If disabled, return `409 Conflict` with message "Test Kitchen is disabled." The frontend disables Run buttons when the config reports `enabled: false`.

### Batch execution

When `POST /kitchen/batches/:id/run` is called on a non-dry-run batch (current implementation):

1. Validate batch is `draft`, TK is enabled, queue is configured; enforce single-running-batch guard.
2. Transition `draft → preparing`, register a batch-level cancel func, launch a detached `executeBatch` goroutine, return 202 with batch detail + estimate.
3. `executeBatch` resolves filters → matching repos, plans instances via `PlanRepo`, transitions `preparing → running`.
4. All `mapped` instances are enqueued into the shared `kitchen_queue` (priority 5) with the batch ID; the `kitchenqueue.Manager` worker pool drains them.
5. A poll loop syncs progress (per-instance status), broadcasting `batch_progress`; on cancellation it cancels queued + running items.
6. On completion/cancellation, transition to `completed`/`cancelled`, set `completed_at`, broadcast `batch_complete`.

### Execution model

All Test Kitchen work (ad-hoc single runs, run-all, and batch instances) flows through one shared `kitchen_queue` drained by a single worker pool. There is no per-batch semaphore — global concurrency is the worker count (see § Concurrency limits), and global load is further bounded by the VM start-rate limiter (see § Capacity constraints).

### Progress tracking

No new database table. Progress is derived from `git_kitchen_results` rows that were upserted during the batch run. The progress endpoint queries results for instances that match the batch's resolved cookbook list.

`GET /kitchen/batches/:id/progress` returns:

```json
{
  "total": 24,
  "passed": 10,
  "failed": 2,
  "errored": 1,
  "timed_out": 0,
  "pending": 11
}
```

`pending = total - passed - failed - errored - timed_out`

### Live updates

Broadcast `batch_progress` WebSocket event after each instance completes:

```json
{
  "type": "batch_progress",
  "batch_id": "...",
  "instance_name": "default-ubuntu-2404",
  "git_repo_name": "my-cookbook",
  "passed": true,
  "completed": 13,
  "total": 24
}
```

Broadcast `batch_complete` when the batch finishes:

```json
{
  "type": "batch_complete",
  "batch_id": "...",
  "status": "completed",
  "passed": 20,
  "failed": 3,
  "errored": 1,
  "total": 24
}
```

### Cancellation

The router keeps a `map[string]context.CancelFunc` for running batches (protected by mutex). When `POST /kitchen/batches/:id/cancel` is called:

1. Look up the cancel func for the batch ID.
2. Call it — this cancels the scheduler's context.
3. Scheduler stops spawning new workers; in-flight workers finish their current kitchen run.
4. The background goroutine detects context cancellation and transitions batch to `cancelled`.

### Dry-run

Dry-run batches resolve and show the estimate but do not execute. This already works. No changes needed except the `IsEnabled()` gate should still apply (prevents previewing when TK is off).

### Concurrency limits

Concurrency is managed **globally**, not per batch. A batch is a selection mechanism (which cookbooks/suites land on the shared queue); load is a property of the queue's worker pool. The per-batch `max_concurrent_vms` field is being removed from the data model and UI — it was never wired and is a misleading dead knob.

- Global concurrency is `TestKitchenConfig.MaxConcurrentVMs` (via `EffectiveMaxConcurrentVMs()`), which sizes the `kitchenqueue.Manager` worker pool. Each worker runs one kitchen run (one VM) at a time, so worker count = max simultaneous VMs.
- Changing it is **dynamic**: `SetWorkerCount` is called on live config change — no restart.
- Default must be conservative and consistent across code and docs (the historical inconsistency — comment "10", code 4 — is resolved to a single source of truth).
- Only one non-dry-run batch may be `running` at a time. Attempting to run a second returns `409 Conflict`.

### Frontend changes

- Disable "Run" button when TK config `enabled` is `false` with tooltip "Test Kitchen is disabled".
- When a batch enters `running`, start polling `GET /kitchen/batches/:id/progress` every 5s (or subscribe to `batch_progress` WebSocket events).
- Update `BatchProgressBar` in real-time.
- Show per-cookbook results table in batch detail (expandable rows with instance-level pass/fail).
- "Cancel" button sends `POST /kitchen/batches/:id/cancel`, updates UI immediately.

## Hypervisor-side orphan sweep

Kitchen VMs occasionally leak when a run fails mid-phase before `destroy` completes (host crash, network timeout, process kill). The DB-based orphan tracker only catches VMs it knows about. A hypervisor-side sweep catches everything.

### Scoping (triple safety)

1. **Folder** — query only VMs in the folder from `driver_settings.targetfolder`. Never touch VMs outside this folder.
2. **Name prefix** — only match VMs whose name starts with the configured prefix (default `cmm`). Parsed via `ParseVMName`.
3. **Age threshold** — only destroy VMs whose embedded timestamp is older than a configurable threshold (default: 2× kitchen timeout, i.e. 60 minutes). Protects in-flight runs.

### Trigger

- **Scheduled** — configurable interval (default every 30 minutes, minimum 5 minutes). Runs as a background goroutine alongside the collector loop. Interval is set in config (`test_kitchen.orphan_sweep_interval`). Set to `0` or `disabled` to turn off.
- **Manual** — `POST /api/v1/kitchen/orphan-sweep` triggers an immediate sweep and returns the result. Accepts `?dry_run=true` query param to list candidates without destroying them. Exposed as a button on the admin TK config page with a "Dry Run" checkbox (checked by default).

### vCenter client extension

`ListManagedVMs` gains an optional folder filter parameter. The vSphere REST API supports `GET /api/vcenter/vm?folders=folder-123&names=cmm-*` for server-side filtering. If folder is empty, fall back to prefix-only filtering (current behaviour).

### Sweep algorithm

1. Resolve effective TK config → extract `driver_settings.targetfolder` and prefix.
2. Query vCenter: `GET /api/vcenter/vm?folders={folder}&names={prefix}-*`
3. For each VM in response:
   - Parse name via `ParseVMName(name, prefix)`.
   - If parse fails → skip (not ours).
   - If timestamp age < threshold → skip (still active).
   - Otherwise → `DestroyVM(hypervisorID)` (power-off + delete).
4. Return summary: detected count, destroyed count, errors, skipped (too young).

### Reporting

The sweep result is:
- Logged at INFO level with counts.
- Broadcast as a `orphan_sweep_complete` WebSocket event.
- Returned in the manual endpoint response.
- Visible in the admin TK config page as "last sweep: N VMs cleaned, time ago".

## Capacity constraints

The target environment has 2000+ git repos with multiple platforms/suites, but limited vCenter capacity constrained primarily by DHCP pool size. VMs frequently spin up but fail to obtain an IP address, hanging until kitchen timeout.

### Conservative concurrency

DHCP is the bottleneck, so the global concurrency default is conservative and configurable up. More concurrent VMs increases the chance of pool exhaustion without improving throughput.

### VM start-rate limit

Concurrency caps *peak simultaneous* VMs, but the binding DHCP constraint is *cumulative* lease consumption: a lease is held for its full lifetime unless explicitly released, so the limit that protects the pool is "how many VMs have started within one lease window", which concurrency alone does not bound.

A global rate limiter gates VM starts at the worker layer — before a worker boots a VM, it checks how many starts occurred in the trailing window and waits if at the cap. Two config values, both **dynamic** (live accessor, no restart — there will be on-site tuning):

- `window` — set to the DHCP lease time (e.g. 60m, 90m).
- `max starts per window` — set to the usable IP pool size (e.g. 25, 64).

The limiter counts **starts** and charges each against the window for the full duration regardless of whether the VM finished or released early. This makes it a hard worst-case guarantee — in any lease-lifetime span, no more than `pool` leases are consumed — that holds even if IP release fails. Enforcement is **evenly paced** (minimum inter-start gap ≈ `window / max`) to also smooth hypervisor load and avoid a thundering herd.

This limiter is the load-bearing guarantee against pool exhaustion and depends on nothing else working. If the customer's scopes have different lease times / pool sizes, the limiter may need a per-scope variant — open question.

### IP lease release on teardown (opt-in, best-effort)

See `test-kitchen-drivers-overlay-generation.md` § Lifecycle Hooks. This is an *opportunistic optimisation*, not a prerequisite — the rate limiter is the guarantee. It is unproven across a heterogeneous OS mix and must be engineered so that nothing it does can fail an otherwise-successful run:

- **Opt-in per platform/image** — enabled only where it is confirmed to release *and* not abend.
- **Failure-isolated** — the hook command always exits 0; a missing release binary, a non-zero result, or the release severing the transport mid-command must never abort the run. A non-zero Test Kitchen lifecycle hook aborts the action and leaks the VM + lease, which is strictly worse than doing nothing.
- **Detached from the transport** so a dropped connection is not seen as a hook failure (tradeoff: detaching races the hypervisor power-off — the release packet must leave the guest first; only empirically verifiable).
- Relies on `kitchen destroy` being a hypervisor API call, not guest-network-dependent.
- Needs empirical validation on the customer OS mix before being relied upon. Scheduled as a spike **after** the rate limiter.

### DHCP failure detection

When kitchen times out and the output contains no converge activity (no resource log lines), classify the failure as `network_timeout` rather than a test failure. This avoids counting infrastructure problems as cookbook failures. On `network_timeout`:

- Mark the instance result with `error_message: "probable DHCP/network timeout"`.
- Log a WARN with the platform name — repeated `network_timeout` on one platform suggests the DHCP pool is exhausted for that subnet.
- Do not retry automatically (retrying into an exhausted pool wastes time).

### Repo-provided setup hooks

Cookbook repos may define their own lifecycle hooks (setup scripts) in `.kitchen.yml`. The generated overlay MUST preserve these and compose with — not clobber — any phase CMM injects. Contract in `test-kitchen-drivers-overlay-generation.md` § Lifecycle Hooks.

### Inter-instance orphan sweep

Run a lightweight orphan check between batch instances (not just on a timer). After each instance completes, if `timed_out` or `network_timeout`, trigger a targeted sweep for that specific VM name before starting the next instance. This keeps the slot count honest and prevents leaked VMs from consuming DHCP leases.

### Batch prioritisation

Batches may take days to work through the full estate. Batch filters already support scoping by `previous_status`, `cookbook_names`, and `platforms`. Recommended workflow:

1. Start with high-value cookbooks (most nodes using them, or business-critical).
2. Use `previous_status: untested` to target cookbooks with no results yet.
3. Use `previous_status: failed` to retest after fixes.

### Restart resilience

Batches must survive application restarts. The batch status is `running` in the database. On startup, any batch still in `running` state with no active goroutine is transitioned to `cancelled` with a note: "interrupted by restart". The operator can re-run it. Individual instance results already persisted via upsert are not lost.

## Acceptance criteria

- Unchecking "Enabled" on the admin TK config page prevents all kitchen runs (single and batch) with a clear error message.
- Creating a batch, clicking "Run" (non-dry-run), and watching instances execute in parallel with live progress updates.
- Cancelling a running batch stops new instances from starting and transitions to `cancelled`.
- Only one batch can be `running` at a time.
- Batch results are persisted in `git_kitchen_results` and reflected on the git repos list page.
- Progress bar shows accurate counts derived from actual results.
- Orphan sweep only touches VMs in the configured folder, matching the prefix, and older than the age threshold.
- Scheduled sweep runs at the configured interval and logs results.
- Manual "Sweep Now" button on admin page triggers immediate cleanup and shows result.
- Sweep does nothing when TK is disabled or no hypervisor is configured.
- Global concurrency default is conservative and consistent across code and docs; changing it takes effect with no restart.
- Per-batch `max_concurrent_vms` is removed from the data model and UI.
- The VM start-rate limiter caps starts to `max per window` in any trailing `window`; starts are evenly paced; window/max changes take effect with no restart.
- Kitchen timeouts with no converge output are classified as network_timeout, not test failure.
- Inter-instance orphan sweep runs after timed-out instances before starting the next.
- Batches interrupted by restart are transitioned to cancelled on next startup; results already persisted are preserved.
- Repo-provided lifecycle hooks in a cookbook's `.kitchen.yml` still run; the overlay does not clobber them.
- The opt-in `pre_destroy` IP-release hook (default off, per platform) never fails or blocks a run: a failed/missing release or a dropped transport leaves the run's result unchanged and the VM destroyed.
