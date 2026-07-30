# Runtime Observability

Make a Go heap/goroutine analysis obtainable from a running instance without shell
access, and make runtime memory legible over time rather than as a bare instantaneous
gauge.

## Why

Production incident 2026-07-29/30 (host OOM, 22GB heap, ~134k nodes). Diagnosis took
hours of manual poll-and-report because:

- **No way to get a profile out.** `performance.pprof_enabled` exists in
  `config.PerformanceConfig` but cannot be set by any means — no config endpoint (see
  the list at `router.go:863-877`), no UI, no env override, no CLI flag, and
  `config_store` values are AES-GCM encrypted so the row cannot be hand-edited. Even if
  set, the routes are registered once at `router.go:567`, so it needs a restart.
  Curl against `/debug/pprof/` also needs an admin session, impractical at a VDI-only
  site.
- **No history.** `go_heap_bytes` is instantaneous (`syshealth.go:140`), so
  leak-vs-spike could only be answered by a human sampling the page repeatedly.
- **No context.** `HeapInuse` alone is misleading: a 1.3↔2.6GB oscillation read as a
  leak until it was identified as ordinary GOGC=100 sawtooth (collect at 2× live).
- **No collection progress.** Whether a run was in flight, on which org, at which step
  had to be inferred from `collection_runs` start-time gaps.

## Design intent

Diagnostics leave the customer site as a **file**, not an exposed endpoint — the site is
VDI/file-transfer only. The existing admin diagnostic bundle
(`handle_admin_diagnostic.go`, a downloadable ZIP) is the delivery mechanism.

Decision: **remove `performance.pprof_enabled` and the `/debug/pprof` routes** rather
than wire them up. Passive cost of the endpoints is zero, but `trace` and `profile` are
expensive to invoke, `heap` forces a stop-the-world GC (material at 12–22GB), and they
disclose paths/symbols to anyone holding an admin session. The bundle delivers the same
capability through an already-audited download path with no permanent debug surface.

## Chunk 1 — profile capture in the diagnostic bundle

Scope: `internal/webapi/handle_admin_diagnostic.go`, new admin download routes,
`specifications/diagnostic-bundle.md` (needs owner approval before editing).

- `goroutine.txt` — full stack traces for every goroutine. **Highest value item.**
  Answers "is it hung, and where" definitively; tonight that question stayed open for
  several hours. Tiny at normal goroutine counts.
- `heap.pprof` — analysed offline with `go tool pprof`. Sampled, so a few hundred KB
  even at a 12GB heap.
- Standalone one-click downloads for each, not bundle-only: when something is wrong the
  profile is wanted immediately, and a full bundle is slower to produce.

Acceptance: both artefacts present in the bundle; `go tool pprof` opens the heap file;
goroutine dump shows collector stacks during a run; no new dependencies.

## Chunk 2 — threshold-triggered auto-capture

A profile is only useful if taken at the peak. The 2026-07-30 peak lasted minutes inside
a 90-minute run; a bundle pulled while idle shows nothing. This is the chunk that would
have reduced the incident to a ten-minute investigation.

- When heap crosses a configurable threshold, write `heap.pprof` + `goroutine.txt` to
  disk, retain the last N, expose for download.
- Reuse the existing threshold/alert machinery in `syshealth.evaluateAlerts` — same
  inputs, new action.

Acceptance: crossing the threshold in a test produces retained artefacts; retention cap
holds; capture failure never affects collection.

## Chunk 3 — runtime stats history and context

Scope: `internal/syshealth/syshealth.go`, `AdminSystemStatsPage.tsx`, `metric_snapshots`.

- Time series for heap, goroutines and RSS — reuse the `metric_snapshots` pattern
  (already 90-day purged, used for dashboard trends). A sparkline answers leak-vs-spike
  at a glance.
- Add `HeapAlloc` (live bytes after last GC — the number people think they are reading),
  `NextGC` (makes the sawtooth self-explanatory), `GCCPUFraction` (shows GC thrash), and
  the **effective `GOGC` / `GOMEMLIMIT`** so the operative limits are visible.
- Process RSS from the cgroup (`memory.current`), not just host-wide `mem_avail_bytes`.
  RSS is what `MemoryMax` kills on, and Go can hold freed spans.

Implementation note: switch from `runtime.ReadMemStats` to the `runtime/metrics`
package. `ReadMemStats` stops the world and the pause scales with heap size — the System
Stats page already polls every 10s (`AdminSystemStatsPage.tsx:151`), which is not free at
a 22GB heap.

## Chunk 4 — collection progress state

Publish current-run state (org N of M, current step, elapsed) so the UI can show it.
`collection_runs.completed_at` is stamped early at Step 4b (`collector.go:1022`), so its
duration column covers only the node-snapshot phase and excludes Steps 5–14 — which is
where the time actually goes. Either publish true end-to-end progress or document the
column's meaning.

Depends on: nothing. Touches the collector, so keep it separate from Chunks 1–3.

## Related fixes owned elsewhere

- Retain ~30 days of `collection_runs` — `PurgeOldCollectionRuns` (`collector.go:689`)
  keeps only the latest terminal run per org, so there is no duration trend to diagnose
  regressions against. Tracked in `active.md`.
