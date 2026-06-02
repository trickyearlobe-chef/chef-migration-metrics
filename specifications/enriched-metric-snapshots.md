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
      "foodcritic": 0,
      "chefspec": 0
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
| `chefspec` | ChefSpec failed (placeholder — not yet implemented) | — |

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

## Future Extensions

- **Platform-filtered readiness**: Add `fresh.by_platform_family_readiness` cross-tab if users want readiness broken down by platform within fresh nodes
- **Re-aggregation**: Store raw `ohai_time` per node in a separate lightweight table if threshold changes need to rewrite historical `by_staleness` counts
- **Ownership on trends**: If needed, add `node_fact_snapshots` table (one row per node per day) for historical ownership joins
- **FoodCritic / ChefSpec**: When implemented, populate the placeholder keys

## Migration

- No schema migration (same `metric_snapshots` table, new `snapshot_type` value)
- New code writes `node_metrics` alongside legacy types
- Handlers prefer `node_metrics`, fall back to legacy
- After 90 days, legacy data ages out naturally via existing purge job
