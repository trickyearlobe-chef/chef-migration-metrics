# Bulk Kitchen Scanning — ToDo

## An environmental failure is counted as a cookbook failure — and it blocks nodes

**Found 2026-08-02, confirmed in code.** The most consequential open fault in the
compatibility signals.

`git_kitchen_results.go:228` counts a failure as `passed = false OR timed_out = true`.
Nothing distinguishes a cookbook that genuinely fails to converge from a run that never
got that far — an auth failure, an exhausted DHCP pool, a timeout. All land as `failed`.

That rolls up through `ComputeTKStatus` to `tk_status = "failed"`, and
`checkCookbookCompatibility` treats **any** Test Kitchen failure as `StatusIncompatible`,
overriding a CookStyle pass. So a lab that could not hand out an IP address marks the
cookbook incompatible and blocks every node running it.

**Why this matters more than it looks.** The product owner reports that on the real estate
a proper Test Kitchen run has only succeeded for a small fraction of cookbooks; the rest are
auth or DHCP failures, or untested. So this is not an edge case — it is most of the Test
Kitchen signal, and it is feeding the readiness number the whole product is judged on.

**It also corrupts the blocking-and-unowned measurement.** The 126 recorded in
`plans/todo-ownership.md` derive from `cookstyle_status = 'blocked'`, and the product owner
reports CookStyle offences are badly curated for this work too. Both signals are unreliable
in the same direction — over-blocking — so that 126 bounds the work rather than describing it.

**Do not fix this by weakening the rollup blindly.** A cookbook that genuinely fails to
converge must still block. What is missing is the distinction, not the severity.

Shape to aim for, cheapest first:

- **Timeouts no longer count as cookbook failures.** A run with `passed IS NULL` is neither a
  pass nor a failure and is not in `tk_total`; the rule is documented on
  `ListGitKitchenCountsByTargetVersions` and applied by every rollup. Migration 0064
  re-materialises `git_repos.tk_*`, because those queries otherwise only run when a kitchen
  result is written. **The effect on the real estate has not been measured** — the dev DB
  holds 27 lab results and no timeouts at all, so the before/after has to be taken where the
  estate lives.
- [ ] **Record *why* a run failed**, rather than only that it did. A run that never reached
  converge is not evidence about the cookbook and should not be a verdict about it.

  **The dominant class does not time out — it exits non-zero, so the timeout fix does not
  touch it.** Of 15 failed instances in the lab DB, 3 are converge failures (the only ones
  that are evidence about a cookbook), 3 failed to *create* the VM (a 300s clone timeout on
  the hypervisor), 8 died on a tooling error before any instance existed. All 15 are recorded
  identically: `passed = false`, `timed_out = false`, `error_message` empty. That is lab data,
  not the estate — but it is the same failure shape the estate reports.

  Test Kitchen names the phase itself, in the output already stored: `Failed to complete
  #create action` / `#converge` / `#verify` / `#destroy`. That is the signature to classify
  on — not vendor-specific DHCP or auth text, which varies by driver. The executor also
  already knows which phase it ran and computes `NetworkTimeout`, then **throws it away**:
  `git_executor.go` never persists it, and `git_kitchen_results` has no column for it.
- [ ] **Report the environmental share** so a reader can see how much of the Test Kitchen
  signal is about the lab. Until then the failure register is the only instrument that can
  correct it, one repo at a time, by hand.

**Interim, and available today:** a human verdict in the failure register outranks Test
Kitchen, so a cookbook wrongly blocked by a lab failure can be recorded `not_broken` and
will stop blocking nodes immediately. That is the correct use of it, and every such entry
is also a measurement of how wrong the automated signal is.


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

- [ ] Extend `ListManagedVMs` with optional folder filter parameter (vSphere `?folders=` param)
- [ ] Scoping: only touch VMs in configured `driver_settings.targetfolder`, matching prefix, older than age threshold
- [ ] Inter-instance orphan sweep: after a `timed_out`/`network_timeout` instance, sweep that VM before starting next
- [ ] Sweep reports: log at INFO, expose on admin TK config page ("last sweep: N VMs cleaned, time ago")

## DHCP / Network Timeout Classification (DONE)

Implemented in `internal/gitkitchen/executor.go` (~:140) — `network_timeout` classification
when a timeout has no converge activity, `error_message: "probable DHCP/network timeout"`.
- [ ] Log WARN per platform — repeated `network_timeout` on same platform → DHCP pool likely exhausted (verify present)

## Frontend

- [x] Disable "Run" button when TK config `enabled: false` with tooltip (Batch UX Chunk 1)
- [x] Poll `GET /kitchen/batches/:id/progress` every 5s + subscribe to `batch_progress` WebSocket events when `running` (Batch UX Chunks 2–3)
- [x] Update `BatchProgressBar` in real time (Batch UX Chunk 3)
- [x] Show per-cookbook results table in batch detail (expandable rows, instance-level pass/fail) (Batch UX Chunk 2)
- [x] "Cancel" button sends cancel request, updates UI immediately — confirm dialog + optimistic "Cancelling…" + refetch (Batch UX Chunk 4)
- [ ] Show sweep status on admin TK config page

## Lifecycle Hooks

Spec: `test-kitchen-drivers-overlay-generation.md` § Lifecycle Hooks.

### Repo-provided setup hooks (preserve) — Chunk 2 DONE 2026-06-07

- [x] Confirm overlay merge behaviour for `lifecycle:` — TK replaces arrays per phase; overlay names `pre_destroy` only, so all other phases survive untouched (regression-locked in `overlay_test.go`, `executor_test.go`)
- [x] Overlay generation MUST preserve cookbook-provided lifecycle hooks; only inject phases CMM owns (CMM reserves `pre_destroy` only)
- [x] When CMM must inject a phase the cookbook also uses, compose (append) rather than replace — `pre_destroy` composes via `readExistingPreDestroy` + `writeLifecycleHook`
- [x] Document which lifecycle phases CMM reserves vs. leaves to the repo — spec § Reserved vs repo-owned phases

### App-injected IP-release hook (pre_destroy) — opt-in spike (Chunk 3 — DONE)

Opportunistic optimisation, NOT the pool guarantee (rate limiter is). Spike — validate
empirically on customer OS mix before relying on it.

- [x] Opt-in per image (`ImageEntry.release_ip_on_destroy`), default off; dynamic (live accessor)
- [x] Failure-isolated: command always exits 0 (`overlay.go` `ipReleaseCommand`)
- [x] Detached from transport (`nohup`/`start /b`, backgrounded, stdio redirected)
- [x] Per-platform command: Linux tolerant of `dhclient`/`dhcpcd`/`networkctl`/`nmcli`; Windows `ipconfig /release`; OS family from `analysis.NormalisePlatformName`
- [x] Compose with any repo-provided `pre_destroy` hook (`writeLifecycleHook` + `readExistingPreDestroy`)
- [x] Admin UI per-image opt-in checkbox (`AdminTestKitchenPage.tsx`)
**ACTION — on-site validation (blocks "rely on it", not "ship it"):** run before trusting the hook. Per platform/image in the customer OS mix:

- [ ] Enable `release_ip_on_destroy` on one image; run a single kitchen instance; confirm the run result is **unchanged** vs the same run with it off (pass stays pass).
- [ ] Confirm the DHCP lease is actually released (check the DHCP server's lease table / pool count drops for that IP after destroy) — i.e. the release packet left the guest before power-off.
- [ ] Force the hook to fail (e.g. point at an image where the release binary is absent / transport user lacks rights) and confirm the run still **passes and the VM is destroyed** — no leak, no abort.
- [ ] Note the transport user: if non-root, the Linux release is a no-op (no `sudo`) — record which images need a `sudo -n` prefix (tech-debt item).
- [ ] Confirm TK lifecycle-hook failure semantics for the installed kitchen version (does a non-zero `remote:` hook abort? does a transport drop count as failure?).

See `todo-tech-debt.md` § Test Kitchen — IP-Release pre_destroy Hook for the known limitations.

## Tests

- [x] Unit tests for rate limiter — never exceeds max per trailing window; paced gap; dynamic window/max change mid-run; ctx cancel; disabled pass-through (`ratelimiter_test.go`, `manager_test.go` gate test)
- [ ] Unit tests for orphan sweep — scoping rules (folder, prefix, age threshold)
- [x] Unit tests for overlay lifecycle-hook composition — repo hooks preserved, CMM `pre_destroy` injected, arrays merged not clobbered (`overlay_test.go`, `executor_test.go`)
- [x] Unit tests for IP-release hook failure isolation (by construction) — injected command always `exit 0`, detached, stdio redirected (`overlay_test.go`); live simulated-failure verification remains empirical
