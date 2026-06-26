# Enriched Metric Snapshots — Component Specification

## Purpose

Replace per-node JSONB blobs in `metric_snapshots` with compact pre-aggregated cross-tabulations. This enables:

1. **Staleness-filtered trends** (Bug 2) — trend data broken down by staleness tier
2. **Platform-filtered trends** — trend data broken down by platform family
3. **Blocking-reason visibility** — understand why nodes are blocked (cookstyle, TK, disk)

## Design

### Pure Aggregates — No Per-Node Data

Metric snapshots store **only aggregated counts**. No per-node arrays. This keeps rows small (~2-5 KB), fast to query, and eliminates the `maxNodesInMetricSnapshot` cap problem entirely.

Ownership filtering is out of scope for historical trends (deferred until a real use case emerges). Current-state ownership queries use `node_snapshots` + `ownership_assignments` join.

### Unified Snapshot Type

A single `node_metrics` snapshot type replaces `chef_version_distribution` and `readiness_summary`. Since there is only one active target version at any time, there is no need for per-target-version rows.

### Payload Shape

```json
{
  "total_nodes": 70000,
  "target_chef_version": "18.5.0",
  "by_staleness": {
    "fresh": 62000,
    "warning": 5000,
    "critical": 3000
  },
  "fresh": {
    "total": 62000,
    "ready": 40000,
    "blocked_total": 22000,
    "blocked_by": {
      "cookstyle": 8000,
      "test_kitchen": 5000,
      "disk": 4000,
      "foodcritic": 0
    },
    "by_version": {
      "17.10.3": 40000,
      "16.18.0": 22000
    },
    "by_platform_family": {
      "debian": 30000,
      "rhel": 20000,
      "windows": 12000
    }
  },
  "thresholds": {
    "warning_hours": 72,
    "critical_days": 7,
    "required_disk_mb": 3000
  }
}
```

### Field Reference

| Field | Purpose |
|-------|---------|
| `total_nodes` | Total nodes in org at collection time |
| `target_chef_version` | Active target at collection time |
| `by_staleness` | Node count per staleness tier |
| `fresh.total` | Fresh node count (= `by_staleness.fresh`) |
| `fresh.ready` | Fresh nodes passing all checks |
| `fresh.blocked_total` | Fresh nodes failing at least one check |
| `fresh.blocked_by.*` | Count of fresh nodes blocked by each check type (overlapping — a node can be blocked by multiple) |
| `fresh.by_version` | Chef version distribution among fresh nodes |
| `fresh.by_platform_family` | Platform family distribution among fresh nodes |
| `thresholds` | Configuration at collection time (informational) |

### Blocking Reasons

| Key | Meaning | Source |
|-----|---------|--------|
| `cookstyle` | CookStyle scan failed for at least one cookbook | Git or server cookstyle results |
| `test_kitchen` | Test Kitchen failed for at least one cookbook | Git repo TK runs |
| `disk` | Insufficient disk space (or unknown) | Node filesystem data vs `required_disk_mb` |
| `foodcritic` | FoodCritic failed (placeholder — not yet implemented) | — |

Nodes can be blocked by multiple reasons simultaneously, so `blocked_by` values may sum to more than `blocked_total`.

### Node Count Limit

The `maxNodesInMetricSnapshot` constant is **removed**. Pure aggregates have constant size regardless of node count. A 70k-node org produces the same ~2-5 KB payload as a 500-node org.

### Snapshot Type Transition

| Phase | What's written | What handlers read |
|-------|---------------|-------------------|
| Current | `chef_version_distribution` + `readiness_summary` | Legacy types |
| After this change | `node_metrics` + legacy types | `node_metrics` preferentially, fall back to legacy |
| After 90-day retention | `node_metrics` only | `node_metrics` only |

Legacy types continue to be written during the transition so rollback is safe.

## Query-Time Behaviour

### Trend Endpoints

Trend handlers read `node_metrics` snapshots and return data directly from the pre-aggregated fields:

- **Readiness trend**: `fresh.ready` / `fresh.total` per point
- **Version distribution trend**: `fresh.by_version` per point
- **Staleness trend**: `by_staleness` per point (existing stale trend chart)

### API Parameters

Add to readiness and version-distribution trend endpoints:
- `?stale=fresh,warning,critical` — which staleness tier to show readiness/version for (default: `fresh`)
- `?platform_family=debian,windows` — platform filter (future, once per-platform-per-tier aggregation added)

For the initial implementation, readiness/version data is only broken down for fresh nodes. Platform filtering within fresh requires an additional cross-tabulation (deferred — see Future Extensions).

### Legacy Fallback

When `node_metrics` snapshots don't exist for a date (pre-migration data), handlers fall back to reading `chef_version_distribution` and `readiness_summary` types. These lack staleness breakdown, so filtered requests return unfiltered data with `"filter_limited": true`.

## Collection-Time Computation

### Data Sources

The `node_metrics` snapshot requires data from two collection phases:
1. **Node collection** → staleness tiers, platform, version, disk
2. **Readiness evaluation** → ready/blocked, blocking reasons

### Writing Strategy

Write `node_metrics` after readiness evaluation completes (it runs in the same collection cycle, just later). This ensures all fields are available in one pass.

The `recordMetricSnapshots` function (currently writes version distribution) and `recordReadinessSnapshots` function (currently writes readiness) will be consolidated into a single `recordNodeMetricsSnapshot` function called after readiness evaluation.

## Offence Fingerprint History

### Why

The `node_metrics` snapshots above store **rolled-up aggregates only** (e.g. a
Ready count), not the per-offence inputs that produced them. The
`cookstyle_results` rows are current-state: they are overwritten on every rescan.
Consequently, when classification criteria change (see
[cop-classification.md](cop-classification.md) → Re-evaluation & Propagation), a
**past** trend point cannot be recomputed under the new criteria — the offence-level
inputs for that historical point no longer exist anywhere. Past points are frozen
and unrecoverable. This limitation is permanent for data captured before this
feature ships; it is stated explicitly so trend consumers do not assume historical
points reflect current classification.

To make trends recomputable **going forward**, we retain a compact,
change-deduped per-scan offence **fingerprint** for each cookstyle result. This is
the canonical home of the decision summarised in cop-classification.md → History.

### Fingerprint Shape

A fingerprint is the minimal per-scan input needed to re-derive a result's rollup
status and weighted complexity under the *current* classification — nothing more:

- `cop_name`
- `count` (number of offences for that cop in that result)
- `severity`
- `correctable`

It deliberately does **not** retain full offence messages or source locations:
re-derivation needs only the inputs the classification resolver and complexity
weighting consume (see cop-classification.md → Pass/Fail Determination and
Complexity Weighting by Classification). One result's scan produces one fingerprint
made of these per-cop entries.

The authoritative offence shape consumed today lives in code (the `offences` JSONB
on `server_cookbook_cookstyle_results` / `git_repo_cookstyle_results`); the
fingerprint is a projection of those fields, pinned by a contract test. Illustrative
projection only:

```json
{
  "result_ref": "<server|git result identity>",
  "scanned_at": "2026-06-26T00:00:00Z",
  "cops": [
    { "cop_name": "Lint/DeprecatedClassMethods", "count": 5, "severity": "warning", "correctable": true }
  ]
}
```

### Append-Only, Change-Deduped

Fingerprint history is **append-only** and **deduped on change**:

- A new fingerprint row is appended for a result only when its fingerprint
  **differs** from that same result's most recent stored fingerprint.
- If a rescan produces an identical fingerprint, nothing is appended; the existing
  row remains valid (its validity simply extends forward in time).
- Offences change only on rescan, and only when the cookbook source actually
  changes, so the number of stored rows scales with **churn**, not with snapshot
  cadence or scan frequency.

Each row is valid from its `scanned_at` until the next row for the same result (or
"now" for the latest). "Fingerprint valid at time T" = the latest row for that
result with `scanned_at ≤ T`.

### Trend Recompute Under Current Criteria

A recomputable trend point at time T is derived by:

1. Determine **membership at T** — which cookbooks/results belonged to each node /
   git repo at that time (run-list / repo membership as of T). Membership-at-T
   history does not exist (see Limitations), so this is bounded to **current
   membership**: the set of results that **still exist now** — the live
   `server_cookbook_cookstyle_results` / git-repo result set. The fingerprint feed
   MUST be intersected with that live set; a result that was fingerprinted but has
   since been removed (cookbook deleted, repo dropped) MUST NOT contribute to any
   recomputed point, even though its last fingerprint row persists in history.
2. Look up the **fingerprint valid at T** for each member result.
3. Re-derive each result's rollup **status** and weighted **complexity** from those
   fingerprints under the *current* resolved classification (the resolver in
   cop-classification.md), then roll up to node / repo / org exactly as the
   single-source-of-truth derivation does for current state.

This yields a trend recomputed under today's criteria for every point captured
**after** this feature ships. It complements — does not replace — the frozen
`node_metrics` aggregates: those remain the record of what each point reported at
the time it was collected.

### Limitations

- **Past points stay frozen.** Points captured before fingerprint history exists
  cannot be recomputed; they retain their original `node_metrics` aggregates.
  Charts that mix pre- and post-fingerprint ranges MUST make the boundary explicit
  rather than implying the whole series reflects current criteria.
- **Recompute is bounded to current membership.** Membership-at-T fidelity
  depends on the membership history available (see the
  ownership/`node_fact_snapshots` note under Future Extensions); that history is
  absent, so recompute is limited to results that exist now. "Current membership"
  is normative, not best-effort: the fingerprint history feed MUST be intersected
  with the live result set so removed-but-still-fingerprinted results are excluded
  from every recomputed point (otherwise an early point over-counts results that
  were later deleted). The recomputed series therefore answers "how would *today's*
  fleet have looked at time T under current criteria", not "what existed at T".

### Storage

Bounded by **change rate**, not snapshot cadence. At real scale (~16,900 cookstyle
results), a change-deduped per-scan fingerprint is on the order of **tens of MB per
year** — a result only contributes a new row when its offences actually change. A
churn-heavy fleet costs more rows; a stable fleet costs almost none. No per-offence
message/location text is stored, which keeps each row small.

## Future Extensions

- **Platform-filtered readiness**: Add `fresh.by_platform_family_readiness` cross-tab if users want readiness broken down by platform within fresh nodes
- **Re-aggregation**: Store raw `ohai_time` per node in a separate lightweight table if threshold changes need to rewrite historical `by_staleness` counts
- **Ownership on trends**: If needed, add `node_fact_snapshots` table (one row per node per day) for historical ownership joins
- **FoodCritic**: When implemented, populate the placeholder keys

## Migration

- No schema migration (same `metric_snapshots` table, new `snapshot_type` value)
- New code writes `node_metrics` alongside legacy types
- Handlers prefer `node_metrics`, fall back to legacy
- After 90 days, legacy data ages out naturally via existing purge job
