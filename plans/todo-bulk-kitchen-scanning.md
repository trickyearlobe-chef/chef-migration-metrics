# Bulk Kitchen Scanning — ToDo

## An environmental failure is counted as a cookbook failure — and it blocks nodes

**Found 2026-08-02, confirmed in code.** The most consequential open fault in the
compatibility signals.

Every rollup counted a failure as `passed = false OR timed_out = true`, with nothing to
distinguish a cookbook that genuinely fails to converge from a run that never got that far —
an auth failure, an exhausted DHCP pool, a timeout. All landed as `failed`.

That rolls up through `ComputeTKStatus` to `tk_status = "failed"`, and
`checkCookbookCompatibility` treats **any** Test Kitchen failure as `StatusIncompatible`,
overriding a CookStyle pass. So a lab that could not hand out an IP address marked the
cookbook incompatible and blocked every node running it. **The counting is fixed below; what
remains is that nothing yet shows a reader how much of the signal was the lab.**

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

**DECIDED 2026-08-03: a config switch, not a smarter signal.** Test Kitchen works on vSphere
when it is set up correctly. It is not set up right now — DHCP went, then the credentials
changed, then the hardware was repurposed — so the answer is to stop Test Kitchen feeding
blocking while that is true, and turn it back on when it is not.

- **`tk_blocks_readiness` toggle**, beside `review_blocks_readiness` on Admin → Readiness.
  Ships **on**, so nothing changes for anyone until it is turned off. It is a `*bool` for the
  same reason `TestKitchen.Enabled` is: a plain bool cannot tell "not set" from "set to
  false", and the default has to be on.

  Off does not hide anything — the Test Kitchen verdict is still collected and still shown
  next to the others. It simply stops counting, so a failed run no longer outranks a CookStyle
  pass. Turn it off at the customer site while vSphere access is gone.

**A finer fix was built and reverted on 2026-08-03** — classifying each failure by the phase
Test Kitchen names in its output, so only converge and verify failures counted against a
cookbook. It worked and was fully tested, but it was more machinery than the situation needs:
with the switch off it does nothing at all. Reverted along with migrations 0064-0067, none of
which ever reached the customer. It is in git history if Test Kitchen comes back and the
finer distinction is wanted then.

**What the estate's kitchen results actually say — measured 2026-08-03, read-only.** About 230
instance results exist, across 116 repos out of 2,210. Roughly: 19% passed, 4.2% were a real
converge failure, 31% failed to build the VM, 41% died before Chef started, 4.2% could not be
classified.

**So 89% of the Test Kitchen failure signal was never about a cookbook**, and every one of
those failures was marking its cookbook incompatible over a CookStyle pass. That is the number
that justifies the switch. Coverage is low because the batches were deliberately small while
the feature was being proven, and then the target went away — not because it does not work.

The query that produced this is a plain `CASE` over `git_kitchen_results.output`, needing no
schema change, so it can be re-run on any database at any version. Rebuild it from the phase
markers Test Kitchen prints: `#create action`, `#converge action`, `#verify action`,
`#destroy action`.

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

**Buildable in the Proxmox lab; the vSphere specifics cannot be validated until customer
access is restored.** The folder filter below is a vSphere parameter.

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
**ACTION — on-site validation (blocks "rely on it", not "ship it"):** run before trusting the
hook. **Blocked: this needs the customer OS mix on vSphere, and that access is gone.** Per platform/image in the customer OS mix:

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
