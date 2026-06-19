# Active Plan

Two chunks: (1) wire the orphan-sweep ticker [current], (2) UI revamp follow-up
cleanup [queued, return after the ticker]. SAML follow-ups parked at the bottom.

Admin-status endpoint + frontend Status tab are DONE/merged (commits up to 61ab9be).

## Chunk 1 — orphan-sweep ticker wiring [CURRENT]

Spec: `specifications/bulk-kitchen-scanning.md` § "Hypervisor-side orphan sweep".
Branch: `feature/orphan-sweep-ticker`.

Problem: `StartSweepTicker` (`internal/hypervisor/sweep_ticker.go`) is implemented
and tested but **never called** — `grep` finds no call site outside the file. The
scheduled sweep silently never runs. The *manual* path is already live
(`handleOrphanSweep`, `POST /api/v1/kitchen/orphan-sweep`, router.go:805).

Wire the ticker at startup in `cmd/chef-migration-metrics/main.go`, near the
kitchen-queue start (~main.go:1424-1475), gated on TK enabled + hypervisor
configured. Register its `stop()` with the graceful-shutdown path (`awaitShutdown`,
main.go:2450 — "stop the scheduler first" region ~2500).

Config sources (`config.TestKitchenConfig`): `IsEnabled()`, `VMNamePrefix`,
`OrphanSweepIntervalMinutes`, `OrphanSweepAgeMinutes` (helper
`EffectiveOrphanSweepAge()` exists; confirm/add an interval helper). Spec defaults:
interval 30m (min 5m), age 60m; interval `0`/disabled turns the ticker off.

### Design decisions to settle BEFORE coding (record the choice in the spec/PR)

1. **Dynamic config (CLAUDE.md mandate).** `StartSweepTicker` captures
   `hyp`/`prefix`/`age`/`interval` at construction — static. A live change to
   interval/age/enabled must take effect without restart. Decide: read config live
   each tick (preferred — matches `handleOrphanSweep`, which calls `Get()` per
   request) vs. tear down + restart the ticker on config change. Likely needs a
   signature/closure change to `StartSweepTicker` (or a thin live wrapper).
2. **Live Hypervisor.** There is no long-lived `Hypervisor`; clients are built
   on-demand from live config via the credential resolver (main.go:1329). The
   ticker must obtain a freshly-built client each tick (resolve creds → build
   client) rather than holding one. Define how (factory/closure).
3. **Enable/disable transitions.** TK toggled off at runtime → ticker must stop
   sweeping; toggled on → must start. Tie to (1).

### vSphere folder scoping — acceptance gap (decide: fold in here or split out)

Spec acceptance: "Orphan sweep only touches VMs **in the configured folder**,
matching the prefix, and older than the age threshold." But `ListManagedVMs(ctx,
prefix)` has **no folder parameter** (proxmox.go:122, vcenter.go:188) — the spec
says it "gains an optional folder filter". The production customer is **vSphere**
(`folders=` server-side filter), see [[lab-vs-customer-hypervisor]]. Wiring an
unscoped sweep against vSphere risks touching VMs outside the CMM folder. Either
add the folder param first (prereq) or explicitly defer + gate the scheduled sweep
to prefix-only with a logged caveat. Do NOT ship an unscoped auto-destroy loop.

### TDD

- New/changed ticker behaviour: table tests with a stub `Hypervisor` (existing
  `sweep_test.go` mocks) — covers disabled→no-sweep, enabled→sweep, live interval
  change, dry-run never on the scheduled path.
- Startup wiring: assert ticker started only when TK enabled + hypervisor
  configured; stop() invoked on shutdown.

Acceptance: `go test ./...`, `go vet`, `golangci-lint`, `-race` green. Scheduled
sweep observably runs (log line / WS `orphan_sweep_complete`). Folder-scoping
decision recorded.

## Chunk 2 — UI revamp follow-up cleanup [QUEUED — return after Chunk 1]

UI Revamp Phase 1 is DONE in main (audited 2026-06-19; nav restructure + polish all
verified). Two divergences from the original plan remain — captured in
`plans/todo-ui-polish.md` § "Follow-up cleanup". This chunk = **decide how to
refactor**, not a prescribed change. Relevant now because it touches the System
Health hub we just extended with the Status tab (`/admin/system-stats`).

Open the todo, then decide per item: (a) System Health sub-tab structure — actual
`Overview|Performance|Status` vs planned `Overview|API|Database|Actions`; is
"Actions" staying top-level intended? (b) 5 orphaned-but-live Kitchen sub-routes
(no nav link, no redirect) — mirror the `/admin/performance`→hub redirect pattern,
or leave as deep links. Reconcile the plan/roadmap so the divergence is recorded,
not silent (CLAUDE.md: never silently diverge from a spec).

## Parked — SAML config follow-ups (lower priority)

- Warn when a SAML provider has empty `username_attr` (transient-NameID footgun;
  breaks login anchoring + ownership matching — `plans/todo-ownership.md`).
- Turn the local-user username collision (`ErrAlreadyExists` → opaque 500) into a
  clear, actionable message.
