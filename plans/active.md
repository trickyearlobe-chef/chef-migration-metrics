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

## Chunk 2 — VM start-rate limiter (core deliverable) — NEXT

Scope: `internal/kitchenqueue/` (worker layer), `internal/config/config.go`, admin TK config UI.
Depends on: nothing.
- TDD a global rate limiter: sliding window, evenly paced (min inter-start gap ≈ window/max).
  Config: `window` (= lease time) + `max_starts_per_window` (= pool size). Both dynamic.
- Gate VM start at the worker, before boot: if starts-in-trailing-window ≥ max, wait.
- Counts starts, charges full window regardless of early finish/release (worst-case guarantee).
- Read params via live accessor each cycle — never cache at construction.
Acceptance (tests): never exceeds max starts in any trailing window; paced gap enforced;
window/max change mid-run takes effect; limiter independent of IP-release working.
Open question to confirm before build: single global window/max, or per-scope (if the
customer's subnets have different lease times / pool sizes)?

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
- Per-scope rate limiting (see Chunk 2).
