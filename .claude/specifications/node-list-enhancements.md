# Node List Per-Check Status Icons — Component Specification

## Problem

The node list shows an overall readiness status (`is_ready` boolean) but not which individual checks passed or failed. Users must click into each node detail page to discover whether the blocker is disk space, CookStyle, Test Kitchen, or a combination. At scale (thousands of nodes), this click-per-node workflow is tedious and slows triage.

## Scope

### In Scope

- New "Checks" column in the node list table showing per-check status icons
- Three check types: disk space, CookStyle, Test Kitchen
- Icon design: compact, colour-coded with shape as secondary signal
- Hover tooltips with check detail
- API extension to `GET /api/v1/nodes` to include per-check summary fields
- Performance considerations for 100k-node lists
- Sorting by overall check status
- Filter integration (deferred to multi-select filter work)

### Out of Scope

- Changes to the node detail page
- New check types beyond disk/CookStyle/TK
- Batch remediation actions from the node list
- Changes to the readiness evaluation algorithm itself

## Data Model

### New Fields on Node List Response

`GET /api/v1/nodes` response items gain three fields alongside the existing `readiness` array. These are per-target-version, nested inside each `NodeReadinessSummary` entry:

| Field | Type | Values |
|---|---|---|
| `disk_status` | string | `"sufficient"`, `"insufficient"`, `"unknown"` |
| `cookstyle_status` | string | `"passed"`, `"failed"`, `"warnings"`, `"unknown"` |
| `kitchen_status` | string | `"passed"`, `"failed"`, `"partial"`, `"unknown"` |
| `disk_detail` | string or null | Human-readable summary, e.g. `"4.2 GB free (threshold: 2 GB)"` |
| `cookstyle_detail` | string or null | e.g. `"3/5 cookbooks passed"` |
| `kitchen_detail` | string or null | e.g. `"2/5 cookbooks tested, all passed"` |

### TypeScript Type Extension

`NodeReadinessSummary` gains these fields:

| Field | TypeScript Type |
|---|---|
| `disk_status` | `"sufficient" \| "insufficient" \| "unknown"` |
| `cookstyle_status` | `"passed" \| "failed" \| "warnings" \| "unknown"` |
| `kitchen_status` | `"passed" \| "failed" \| "partial" \| "unknown"` |
| `disk_detail` | `string \| null` |
| `cookstyle_detail` | `string \| null` |
| `kitchen_detail` | `string \| null` |

### Derivation Rules

These fields are derived from data already computed during readiness evaluation (see `analysis.md` § Node Upgrade Readiness). They do not require new data collection.

**Disk status:**

| Condition | `disk_status` |
|---|---|
| `sufficient_disk_space === true` | `"sufficient"` |
| `sufficient_disk_space === false` | `"insufficient"` |
| `sufficient_disk_space === null` or stale node | `"unknown"` |

**CookStyle status** (aggregated across all cookbooks in the node's run list):

| Condition | `cookstyle_status` |
|---|---|
| All cookbooks have a passing CookStyle result (server or git) | `"passed"` |
| One or more cookbooks have CookStyle failures (incompatible) | `"failed"` |
| All scanned cookbooks pass but some have deprecation warnings | `"warnings"` |
| Not all cookbooks have been scanned | `"unknown"` |

**Test Kitchen status** (aggregated across all cookbooks):

| Condition | `kitchen_status` |
|---|---|
| All cookbooks have passing TK results | `"passed"` |
| One or more cookbooks fail TK | `"failed"` |
| Some cookbooks tested and passing, others not yet tested | `"partial"` |
| No TK results available for any cookbook | `"unknown"` |

## Per-Check Status Icons

### Column Placement

A new "Checks" column appears after the existing "Status" (stale badge) column. It contains three small icons grouped tightly as a "status cluster".

### Icon Specifications

Each check is represented by a small icon (16×16px or equivalent). The three icons are arranged horizontally with 4px gaps.

| Check | Icon Shape | Purpose |
|---|---|---|
| Disk | Hard drive / disk | Disk space sufficiency |
| CookStyle | Code brackets or lint icon | Static analysis results |
| Test Kitchen | Flask / beaker | Integration test results |

### Colour Mapping

| Status Value | Colour | Icon Overlay | Meaning |
|---|---|---|---|
| `sufficient` / `passed` | Green (`text-green-600`) | Checkmark (✓) | Check passed |
| `insufficient` / `failed` | Red (`text-red-600`) | X mark (✗) | Check failed |
| `warnings` / `partial` | Amber (`text-amber-500`) | Tilde (~) | Partial / warnings |
| `unknown` | Grey (`text-gray-400`) | Question mark (?) | No data available |

Colour is the primary signal. Shape overlay (✓/✗/~/?) is the secondary signal for accessibility — users who cannot distinguish colours can still read status from icon shape.

### Hover Tooltips

Each icon has an independent tooltip on hover:

- **Disk:** Content from `disk_detail`, e.g. `"Disk: 4.2 GB free (threshold: 2 GB)"` or `"Disk: insufficient (1.1 GB free, need 2 GB)"` or `"Disk: unknown (stale node)"`
- **CookStyle:** Content from `cookstyle_detail`, e.g. `"CookStyle: 5/5 cookbooks passed"` or `"CookStyle: 3/5 cookbooks passed, 2 failed"`
- **Test Kitchen:** Content from `kitchen_detail`, e.g. `"Test Kitchen: 4/5 cookbooks tested, all passed"` or `"Test Kitchen: not tested"`

If the detail field is null, fall back to a generic label: `"Disk: unknown"`, `"CookStyle: unknown"`, `"Test Kitchen: unknown"`.

### Visual Grouping

The three icons render as a tight cluster — visually read as a single unit. Use a thin separator or consistent spacing so they feel grouped but individually identifiable. The cluster should not exceed ~60px total width to avoid blowing out the table.

## Component Structure

### CheckStatusIcons

A new component that renders the three-icon cluster for a single node row.

**Props contract:**

| Prop | Type | Description |
|---|---|---|
| `diskStatus` | `"sufficient" \| "insufficient" \| "unknown"` | Disk check result |
| `cookstyleStatus` | `"passed" \| "failed" \| "warnings" \| "unknown"` | CookStyle result |
| `kitchenStatus` | `"passed" \| "failed" \| "partial" \| "unknown"` | Test Kitchen result |
| `diskDetail` | `string \| null` | Tooltip text for disk icon |
| `cookstyleDetail` | `string \| null` | Tooltip text for CookStyle icon |
| `kitchenDetail` | `string \| null` | Tooltip text for TK icon |

### CheckStatusIcon (internal)

Renders a single icon with colour, shape overlay, and tooltip. Reusable across the three check types.

**Props contract:**

| Prop | Type | Description |
|---|---|---|
| `status` | `"passed" \| "failed" \| "warnings" \| "partial" \| "sufficient" \| "insufficient" \| "unknown"` | Determines colour and overlay |
| `icon` | React element or icon name | The base shape (disk, code, flask) |
| `tooltip` | `string` | Hover text |
| `ariaLabel` | `string` | Accessible label |

## API Changes

### `GET /api/v1/nodes` Response Extension

Each item in the `readiness` array gains the six new fields defined in the Data Model section. The existing fields (`is_ready`, `all_cookbooks_compatible`, `sufficient_disk_space`, `blocking_cookbook_count`, `stale_data`) are unchanged.

### Backend Derivation

The per-check status fields are computed from data already persisted during readiness evaluation. The readiness record (see `analysis.md` § Persistence) contains:

- `disk_space_sufficient` → maps directly to `disk_status`
- `disk_space_available_mb` + configured threshold → generates `disk_detail`
- `blocking_cookbooks` JSON array with per-source verdicts → used to derive `cookstyle_status`, `kitchen_status`, and their detail strings

The derivation should happen at query time from the persisted readiness record, or be pre-computed and stored as additional columns on the readiness table.

## Performance

### Concern

At 100k nodes, the node list query already joins with readiness data. Adding per-check status fields must not degrade query performance.

### Preferred Approach: Pre-compute During Evaluation

Compute `disk_status`, `cookstyle_status`, and `kitchen_status` during the readiness evaluation cycle (when `blocking_cookbooks` and `disk_space_sufficient` are already being determined) and store them as indexed columns on the readiness table. This avoids runtime JSON parsing of `blocking_cookbooks` during list queries.

The detail strings (`disk_detail`, `cookstyle_detail`, `kitchen_detail`) can also be pre-computed and stored, or generated at query time from the existing numeric fields — they are short strings and the computation is trivial.

### Alternative: Derive from Existing Fields

If pre-computation is deferred, the backend can derive `disk_status` from `sufficient_disk_space` (already a column) with zero cost. `cookstyle_status` and `kitchen_status` require scanning the `blocking_cookbooks` JSON, which is acceptable for small result sets (paginated queries) but may need caching for bulk exports.

## Sorting

### Check Status Sort

The "Checks" column header supports sorting. Sort priority is a composite score:

| Status | Score |
|---|---|
| All three checks green | 0 (best) |
| Mixed green + unknown (no failures) | 1 |
| Any amber/warnings/partial | 2 |
| Any red/failed | 3 |
| All unknown | 4 |

Sort ascending puts all-green nodes first. Sort descending puts failed nodes first (useful for triage).

The sort is implemented server-side using the pre-computed status columns. The API accepts `sort=check_status` as a sortable field.

## Filtering

### Current State

The readiness filter dropdown already supports `ready`, `blocked`, `cookbooks_blocked`, `disk_blocked`, and `disk_unknown` values (see `NodesPage.tsx` `ReadinessFilter` type).

### Future Enhancement

When multi-select filters land, add individual filters for each check type:

- Disk: sufficient / insufficient / unknown
- CookStyle: passed / failed / warnings / unknown
- TK: passed / failed / partial / unknown

This is deferred — the existing readiness filter covers the most common triage scenarios.

## Accessibility

- Each icon has an `aria-label` describing the check and its status: e.g. `"Disk space: sufficient — 4.2 GB free"`
- Colour is never the sole indicator — shape overlays (✓/✗/~/?) provide redundant encoding
- Icons are not interactive (no focus needed) — they are decorative with `role="img"`
- Tooltip content is duplicated in `aria-label` for screen readers
- Colour contrast ratios for icon fills against the table row background meet WCAG AA

## Testing

### Unit Tests — Status Derivation (backend)

- Disk status derived correctly from `sufficient_disk_space` boolean/null
- CookStyle status derived correctly from blocking_cookbooks verdicts
- Kitchen status derived correctly from blocking_cookbooks verdicts
- Detail strings generated with correct counts and formatting
- Stale nodes produce `"unknown"` disk status
- Nodes with no readiness data produce all-unknown statuses
- Edge case: node with zero cookbooks produces `"passed"` for CookStyle and TK

### Component Tests — CheckStatusIcons

- Renders three icons for each status combination
- Correct colour classes applied per status value
- Correct shape overlay rendered per status value
- Tooltip displays detail text on hover
- Tooltip falls back to generic text when detail is null
- `aria-label` set correctly on each icon
- Icons do not exceed expected width

### Component Tests — NodesPage Integration

- Checks column renders when readiness data includes per-check fields
- Checks column degrades gracefully when per-check fields are absent (shows all-unknown)
- Sort by check status sends correct query parameter
- Selected target version drives which readiness entry is used for icon display

### API Tests

- `GET /api/v1/nodes` response includes per-check fields in readiness array
- Values are correct for known test fixtures
- Performance: list query with 1000 nodes completes within acceptable threshold

## Migration

- The new fields are additive — existing API consumers ignore unknown fields
- Frontend renders all-unknown icons if the backend has not yet been updated (graceful degradation)
- No feature flag needed — the icons provide strictly more information than the current view
- Existing `NodeReadinessSummary` type gains optional fields; no breaking change
- Pre-computed columns can be backfilled by re-running readiness evaluation

## References

| Specification | Relevance |
|---|---|
| `analysis.md` § Node Upgrade Readiness | Readiness evaluation algorithm and persistence schema |
| `web-api.md` § Node Endpoints | Current `GET /api/v1/nodes` response contract |
| `visualisation.md` § Node Upgrade Readiness | Dashboard readiness views |
| `project-conventions.md` | Frontend component and testing conventions |
| `version-battery-bars.md` | Precedent for compact visual status indicators |