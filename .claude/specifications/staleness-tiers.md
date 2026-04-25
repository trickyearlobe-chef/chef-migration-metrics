# Two-Tier Node Staleness — Component Specification

## Current State

Node staleness is binary: a single `collection.stale_node_threshold_days` (default: 7) splits nodes into fresh or stale. A node missing for 8 days looks identical to one missing for 6 months. The `is_stale` boolean is stored in `node_snapshots` and computed at collection time by comparing `ohai_time` against the threshold. The `StaleBadge` component shows a single red "Stale" badge with optional age text.

This makes triage difficult. Operators cannot distinguish transient absences (reboots, maintenance, patching) from nodes that are genuinely lost or decommissioned.

## Two-Tier Model

Three staleness states replace the binary fresh/stale split:

| Tier | Meaning | Typical Cause |
|------|---------|---------------|
| **Fresh** | Checked in within the warning threshold | Normal operation |
| **Warning** | Past the warning threshold but within the critical threshold | Transient: reboots, maintenance windows, patching, network blips |
| **Critical** | Past the critical threshold | Decommissioned, lost, or genuinely problematic |

The tier is determined solely from `ohai_time` age at query time — no new stored column is required.

### Tier Calculation

Given `age = now - ohai_time`:

- `age < warning_threshold` → **fresh**
- `warning_threshold ≤ age < critical_threshold` → **warning**
- `age ≥ critical_threshold` → **critical**

The existing `is_stale` boolean column remains for backward compatibility. It maps to `tier != fresh` (i.e. `warning` or `critical`). New code should prefer the tier enum; `is_stale` is retained so older queries and exports do not break.

## Configuration

Two new settings under `collection`, replacing the single `stale_node_threshold_days`:

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `stale_node_warning_hours` | integer | `72` | Hours since last check-in before a node enters the **warning** tier |
| `stale_node_critical_days` | integer | `7` | Days since last check-in before a node enters the **critical** tier |

### Validation

- `stale_node_warning_hours` must be a positive integer.
- `stale_node_critical_days` must be a positive integer.
- Warning threshold (converted to the same unit) must be strictly less than the critical threshold. Reject the configuration at startup and in the admin UI if violated.
- Both settings are configurable via the existing admin UI collection settings page.

### Backward Compatibility

- If only `stale_node_threshold_days` is present in config (upgrade scenario), it maps to `stale_node_critical_days`. The warning tier defaults to 72 hours.
- If both old and new keys are present, the new keys take precedence and `stale_node_threshold_days` is ignored with a deprecation log warning.
- New installations get both defaults (72 h warning, 7 d critical).

## Visual Treatment

### Badges

| Tier | Badge Colour | Icon | Label Format | Example |
|------|-------------|------|-------------|---------|
| Fresh | None (no badge, as today) | — | — | — |
| Warning | Amber/yellow | Warning triangle (⚠) | `Missing (<age>)` | `Missing (2d 4h)` |
| Critical | Red | X or skull | `Gone (<age>)` | `Gone (45d)` |

The age display uses the most significant unit: hours if under 48 h, days otherwise. This lets operators triage at a glance without opening the detail page.

### Age Formatting

- `< 1 hour` → `<N>m` (e.g. `45m`)
- `1–47 hours` → `<N>h` (e.g. `36h`)
- `48 hours – 99 days` → `<N>d` (e.g. `12d`)
- `≥ 100 days` → `<N>d` (e.g. `365d`)

## Affected Surfaces

### Dashboard Stale Card

Currently a two-segment bar (fresh / stale). Replace with a three-segment bar: fresh (green) / warning (amber) / critical (red). Each segment shows its count and percentage.

### Dashboard Stale Trend

Currently two series (stale, fresh) over time. Add a third series so the trend chart shows fresh, warning, and critical counts per snapshot.

### Node List

The `StaleBadge` component renders per row. Warning nodes show the amber badge; critical nodes show the red badge. Fresh nodes show no badge (unchanged).

### Node Detail

The header area (alongside platform/version info) shows the staleness tier badge. For warning and critical nodes, also show the exact `ohai_time` timestamp and computed age.

### Readiness Summary

Break out the current `stale_count` into `warning_count` and `critical_count`. Critical nodes are excluded from readiness calculations entirely (their data is too old to trust). Warning nodes are assessed but flagged — their readiness verdict carries a caveat that the data may be stale.

### Filters

Current options: all / stale only / fresh only. New options:

- **All** — no staleness filter
- **Fresh only** — `tier = fresh`
- **Warning only** — `tier = warning`
- **Critical only** — `tier = critical`

Multi-select (e.g. warning + critical) is deferred until the filter UX overhaul lands. Until then, the four options are mutually exclusive.

### Exports

Add a `staleness_tier` column (`fresh`, `warning`, `critical`) to all node exports. The existing `is_stale` column is retained for backward compatibility.

### Metric Snapshots

The version distribution payload currently records `stale_nodes` and `fresh_nodes`. Add `warning_nodes` and `critical_nodes` fields alongside. The `stale_nodes` field remains as the sum of warning + critical for backward compatibility with existing trend data.

## API Changes

### Node List and Detail

All node responses add a `staleness_tier` field with value `"fresh"`, `"warning"`, or `"critical"`. The existing `is_stale` boolean remains (`true` when tier is warning or critical).

### Dashboard Stale Endpoint

Returns three counts:

| Field | Description |
|-------|-------------|
| `fresh_count` | Nodes in the fresh tier |
| `warning_count` | Nodes in the warning tier |
| `critical_count` | Nodes in the critical tier |
| `total_nodes` | Sum of all three |

### Dashboard Stale Trend Endpoint

Returns three series per data point: `fresh_count`, `warning_count`, `critical_count`.

### Dashboard Readiness Endpoint

The `stale_count` field remains (sum of warning + critical). Two new fields are added: `warning_count` and `critical_count`. The `blocking_reasons.stale_data` count maps to critical only (warning nodes are still assessed).

### Filter Parameter

The `stale_status` query parameter accepts:

| Value | Meaning | Backward Compatibility |
|-------|---------|----------------------|
| `all` | No filter | Unchanged |
| `fresh` | Fresh only | Unchanged |
| `warning` | Warning only | New |
| `critical` | Critical only | New |
| `true` | Maps to `critical` | Legacy — deprecated |
| `false` | Maps to `fresh` | Legacy — deprecated |

## Migration

- No database schema migration is required. Staleness tier is computed at query time from `ohai_time`, not stored.
- The `is_stale` column in `node_snapshots` is retained and continues to be written at collection time (set to `true` when tier is warning or critical).
- Existing `stale_node_threshold_days` config maps to `stale_node_critical_days`, preserving current behaviour for upgrades.
- Historical metric snapshot payloads with only `stale_nodes`/`fresh_nodes` remain readable. New snapshots include the additional `warning_nodes`/`critical_nodes` fields. Trend chart rendering must handle both old and new payload shapes gracefully (old payloads treat all stale as critical).

## References

- Configuration: `configuration.md` § Collection Schedule
- Data collection: `data-collection.md` § 1.6 Stale Check-in Detection
- Visualisation: `visualisation.md` § Node Upgrade Readiness (stale node indicators)
- Web API: `web-api.md` § Node Endpoints, Dashboard Endpoints
- Frontend: `StatusBadge.tsx` — `StaleBadge` component