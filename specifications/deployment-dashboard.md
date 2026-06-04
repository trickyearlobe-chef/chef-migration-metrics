# Deployment Dashboard — Component Specification

> **TL;DR** — A dedicated "Deployment" tab on the dashboard showing per-version parallel deployment progress. Separates empirical deployment tracking (which versions are physically deployed and converging) from theoretical compatibility analysis (TK/CookStyle against a static target).

## Context

The customer is moving to a faster upgrade cadence with overlapping minor version rollouts. The fleet may have multiple Chef versions staged simultaneously (e.g., 19.3.5 on some nodes, 19.3.15 on others). The existing single-target readiness model remains valid for TK/CookStyle analysis, but the deployment view needs per-version granularity.

## Design Principles

- **Deployment is empirical** — "is this version actually working on nodes?" Based on ohai data from the migration cookbook.
- **Readiness is theoretical** — "can our cookbooks handle version X?" Based on static analysis. Stays on the existing single-target model.
- **No 4D graphs** — TK/CookStyle analysis stays single-target. Deployment gets its own tab.

## Data Source

All data comes from existing `node_snapshots` columns (migration 0035):
- `migration_state` — omnibus_only / hab_dormant / hab_active
- `active_chef_version` — currently running version
- `dormant_chef_version` — staged version (the key for per-version grouping)
- `target_converge_status` — success / fail (speculative converge result)
- `target_version` — version used for speculative converge

**No additional DB migration required.**

## Dashboard Tab: "Deployment"

Third tab alongside "Current Status" and "Trends".

### Current Status View

Per-version breakdown of fleet deployment state. For each version that has been deployed (i.e., appears as `dormant_chef_version` or `active_chef_version` where `migration_state != 'omnibus_only'`):

| Metric | Meaning |
|--------|---------|
| Staged | Nodes with this version installed but dormant |
| Activated | Nodes where this version is now active |
| Converge Passing | Nodes where speculative converge succeeded for this version |
| Converge Failing | Nodes where speculative converge failed for this version |

**Visualisation**: Battery bars or grouped bar chart — one group per deployed version.

### Trend View

Per-version progress over time. For each deployed version, two series:

1. **Staged + Activated** — count of nodes with this version present
2. **Converge Passing** — count of nodes where speculative converge passes

The gap between lines shows nodes needing investigation for that specific version.

**Visualisation**: Multi-series line chart, grouped by version (colours per version).

## API Contracts

### GET /api/v1/dashboard/deployment/status

Returns current per-version deployment state. Live query of `node_snapshots`.

```json
{
  "data": [
    {
      "version": "19.3.15",
      "staged": 5,
      "activated": 2,
      "converge_passing": 4,
      "converge_failing": 1,
      "total": 7
    },
    {
      "version": "19.3.5",
      "staged": 0,
      "activated": 8,
      "converge_passing": 8,
      "converge_failing": 0,
      "total": 8
    }
  ],
  "total_nodes": 100
}
```

### GET /api/v1/dashboard/deployment/trend (updated)

Returns per-version trend data from `node_metrics` snapshots.

```json
{
  "data": [
    {
      "completed_at": "2025-06-14T12:00:00Z",
      "total_nodes": 100,
      "by_version": {
        "19.3.5": { "staged_or_activated": 10, "converge_passing": 8 },
        "19.3.15": { "staged_or_activated": 5, "converge_passing": 3 }
      }
    }
  ]
}
```

## Metric Snapshot Payload Extension

The `deployment` field in `node_metrics` payload becomes:

```json
{
  "deployment": {
    "staged_or_activated": 15,
    "converge_passing": 11,
    "by_version": {
      "19.3.5": { "staged": 2, "activated": 8, "converge_passing": 8 },
      "19.3.15": { "staged": 5, "activated": 0, "converge_passing": 3 }
    }
  }
}
```

Backward-compatible — aggregate fields remain for older consumers.

## What Does NOT Change

- TK/CookStyle/Readiness analysis — stays on single static target
- Node list deployment columns — already version-agnostic (shows state + converge)
- Node detail panel — already shows the specific versions for that node
- Global "Target" selector — remains for readiness, does not affect deployment tab

## Schema Impact

**None.** No DB migration. Data sources:
- Live query: existing `node_snapshots` columns (GROUP BY)
- Trend: existing `metric_snapshots` JSON payload (extended shape)
- Optional future: `CREATE INDEX` on `dormant_chef_version` if live query slow

## Open Questions

- Battery bars vs grouped bar chart for current status — build one, iterate
- Should the deployment tab filter by organisation? (Probably yes, reuse global filter)
- Should "activated" nodes that were on an older version be highlighted for re-staging?
