# Active Plan — Kitchen Scan Load Control

Goal: make the bulk kitchen scan safe to run at the customer (DHCP-constrained).
Spec: `bulk-kitchen-scanning.md` (§ Concurrency limits, § Capacity constraints),
`test-kitchen-drivers-overlay-generation.md` (§ Lifecycle Hooks).
Design rationale: memory `kitchen-concurrency-model`.

Branch: `feature/kitchen-vm-rate-limiter`.

Note: Run→Scheduler wiring is already implemented (queue-based). No wiring work.

## Chunk 1 — Concurrency cleanup — DONE (commits 134147e, 434c3fb)

- Removed dead per-batch `max_concurrent_vms` (migration 0036, datastore, frontend type/fixtures).
- `DefaultMaxConcurrentVMs = 2` single source of truth; comment fixed.
- Dynamic worker-count already wired + tested.

## Chunk 2 — VM start-rate limiter (core deliverable) — DONE

Decision: single global window/max (per-scope deferred to backlog).
Delivered:
- `internal/kitchenqueue/ratelimiter.go` — global sliding-window + even-pacing limiter
  (`RateLimiter`), live config accessor read each `Wait`, charges full window, lock-serialised
  decision-and-record. Tests in `ratelimiter_test.go` (virtual clock).
- `manager.go` — `WithRateLimiter` option + `StartLimiter` gate in `worker` after claim,
  before boot; `m.ctx` cancelled on `Stop` to unblock a waiting worker. Test in `manager_test.go`.
- `config.go` — `StartRateWindowMinutes` + `StartRateMaxPerWindow` (both dynamic, disabled
  unless both > 0), `StartRateLimit()` accessor, non-negative + partial-config validation.
- `main.go` — live limiter wired into `kitchenqueue.New` via `configHolder.Get()`.
- Admin TK config UI: rate-limit section in `AdminTestKitchenPage.tsx` + `TestKitchenConfig`
  type/test fixtures; handler-level validation in `handle_admin_config_analysis.go`.

## Chunk 3 — IP-release pre_destroy hook (spike, AFTER chunk 2)

Scope: `internal/gitkitchen/overlay.go`, image/platform config, admin UI.
- Opt-in per platform/image (default off), dynamic config.
- Inject failure-isolated `pre_destroy` hook: always exit 0; detached from transport;
  tolerant of absent/variant Linux release binaries; Windows `ipconfig /release`.
- Preserve/compose with repo-provided lifecycle hooks (do not clobber).
- Validate empirically on customer OS mix before relying on it.
Acceptance: a failed/missing release or dropped transport never changes a run's result
and never blocks destroy (tests with simulated hook failure).

## Open questions

- TK lifecycle-hook failure semantics for the installed kitchen version (does a non-zero
  `remote:` hook abort? does a transport drop count as failure?) — verify, don't assume.
- Per-scope rate limiting — deferred. Single global limiter shipped (Chunk 2). Revisit only
  if the customer's subnets prove to have materially different lease times / pool sizes;
  would need an instance→scope mapping that does not exist yet.
