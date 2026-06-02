# Filter UX Overhaul — Component Specification

## TL;DR

Redesign the filter system into three tiers — global filters in the top bar, multi-select per-page filters with tag/chip UX, and type-ahead search for high-cardinality dimensions. Replace single-select dropdowns with multi-select controls. Promote staleness to a shared `GlobalFilterContext`. Add backend `?q=` search support for high-cardinality filter endpoints. Display the active target Chef version as a read-only indicator (not a filter — there is only one active target, set via admin config).

## Problem

- All filters are single-select. Users cannot express "show me Windows 2019 AND Windows 2022" or "incompatible AND untested" without toggling back and forth.
- Global filters (target Chef version, staleness) are managed independently on every page via local `useState` + URL params. Changing staleness on the node list does not carry over to the cookbook list.
- **Note:** Target Chef version is a system-wide admin setting, not a user-selectable filter. There is only one active target at any time; changing it invalidates all cookstyle/TK results and triggers a rescan. It should be displayed as a read-only indicator, not a filter control.
- High-cardinality dropdowns (roles) load thousands of items into a `FilterCombobox`, which is slow to render and impossible to scroll through.
- Client-side option filtering in `FilterCombobox` may use substring matching while the backend uses `^<string>.*` prefix matching on version fields, producing confusing suggestions where a user sees a match in the dropdown but gets no results from the API. (Under investigation — may already be fixed, but the contract must be explicit either way.)

## Scope

### In Scope

- Three-tier filter taxonomy (global, multi-select, type-ahead)
- `GlobalFilterContext` React context for cross-cutting filters
- New and enhanced filter components (`FilterMultiCheckbox`, `FilterTypeAhead`, enhanced `FilterCombobox`)
- Backend `?q=` search parameter on high-cardinality filter endpoints
- Multi-select query param contract (comma-separated, OR within dimension, AND across dimensions)
- Matching-strategy consistency between client suggestions and backend queries

### Out of Scope

- Saved/named filter presets (future enhancement)
- Filter-based notification rules
- Changes to the `OrgSelector` component itself (it stays as-is; new global filters sit alongside it)
- Database schema changes (filtering uses existing indexed columns)
- Ownership filters (already specified in ownership spec)

## Filter Tiers

### Tier 1 — Global Filters (Top Bar)

Filters that affect every page and persist across navigation.

| Filter | Current State | Target State |
|---|---|---|
| Organisation | Global via `OrgContext` | Unchanged |
| Target Chef version | Per-page via `useTargetChefVersion` | **Read-only indicator** in top bar (not a filter — single system-wide value set in admin config) |
| Staleness tier | Per-page `stale_status` param | Promote to `GlobalFilterContext` (once two-tier staleness lands) |

These render in the top bar, next to the existing org selector.

### Tier 2 — Multi-Select Filters (Per-Page Filter Bar)

Low-to-medium cardinality dimensions where the full option set can be loaded into the client.

| Filter | Control Type | Pages |
|---|---|---|
| Platform / platform version | Multi-select checkbox dropdown | Nodes |
| Chef client version | Multi-select with prefix search | Nodes |
| Compatibility status | Multi-select checkboxes (compatible / incompatible / cookstyle-only / untested) | Cookbooks, Git repos |
| Readiness status | Multi-select checkboxes (ready / blocked) | Nodes |
| Complexity label | Multi-select checkboxes (low / medium / high / critical) | Cookbooks, Git repos |
| Environment | Multi-select checkbox dropdown (falls back to type-ahead if >200 options) | Nodes |
| Policy name | Multi-select checkbox dropdown (falls back to type-ahead if >200 options) | Nodes |
| Policy group | Multi-select checkbox dropdown (falls back to type-ahead if >200 options) | Nodes |

Selected values display as removable tags/chips below the filter bar. A "Clear all" action resets all per-page filters.

### Tier 3 — Type-Ahead Search (Per-Page Filter Bar)

High-cardinality dimensions where loading all options is impractical.

| Filter | Match Type | Pages |
|---|---|---|
| Role | Prefix search via backend `?q=` — returns top N matches | Nodes |
| Node name | Freeform substring text input (no dropdown) | Nodes |
| Cookbook name | Freeform substring text input | Cookbooks, Git repos |

Type-ahead filters issue debounced requests to backend search endpoints as the user types. They never load a full option list.

## Backend Contract

### Multi-Value Query Parameters

Already defined in the web-api spec: filter parameters accept comma-separated values. The semantic contract:

- **OR within a dimension**: `?platform=windows,centos` → nodes on Windows OR CentOS
- **AND across dimensions**: `?platform=windows&status=incompatible` → Windows nodes AND incompatible status
- Single-value parameters continue to work unchanged

### Search Parameter for Filter Endpoints

High-cardinality filter option endpoints gain an optional `?q=` parameter:

| Endpoint | `?q=` Behaviour |
|---|---|
| `GET /api/v1/filters/roles` | Returns roles matching `q` prefix, limited to 50 results. Without `q`, returns all (existing behaviour preserved). |
| `GET /api/v1/filters/environments` | Same pattern — prefix search, limit 50. Only needed if environment count exceeds the cardinality threshold. |
| `GET /api/v1/filters/policy-names` | Same pattern. |
| `GET /api/v1/filters/policy-groups` | Same pattern. |

Response shape is unchanged — `{ "data": [...] }`. The `?q=` parameter filters and limits server-side.

### Matching Strategy Contract

| Field Type | Backend Match | Client Suggestion Match |
|---|---|---|
| Version fields (Chef version, platform version) | Prefix: `^q.*` | Prefix: `startsWith(q)` |
| Name fields (role, environment, policy, cookbook, node) | Prefix for `?q=` search endpoint; substring for freeform text filter on list endpoints | Prefix for type-ahead suggestions; substring for freeform text inputs |

Client-side filtering in combobox/multi-select components MUST use the same strategy as the backend for that field type. Mismatched strategies produce suggestions that return zero results.

## Global Filter Context

### Contract

New React context: `GlobalFilterContext`, provided at the app root alongside `AuthContext` and `OrgContext`.

**Stored state:**

| Key | Type | Default | Source |
|---|---|---|---|
| `staleStatus` | `"all" \| "warning" \| "critical" \| "fresh"` | `"all"` | User selection |

**Target Chef version** is NOT part of filter state. It is a system-wide admin setting (single active value). The context exposes it as a read-only value fetched from `/api/v1/admin/config` for display purposes only. It cannot be changed from the filter bar — only from admin config.

**Persistence:** Global filter values are reflected in URL query params (`?stale_status=fresh`). Navigating to a URL with these params restores the global filter state. Bookmarking and link-sharing work.

**Page integration:** Pages read global filters from context and include them in API calls. Pages no longer manage staleness in local state.

**`useTargetChefVersion` hook:** Retired. Pages that need to display the target version read it from `GlobalFilterContext` (read-only). No filter param is sent to the backend — the backend always uses the single configured target.

## Component Changes

### New: `FilterMultiCheckbox`

Multi-select control for low-cardinality dimensions. Renders as a dropdown panel of checkboxes. Trigger button shows count of selected items (e.g. "Platform (3)"). Selected values appear as removable chips.

**Props:**
- `label` — display name
- `options` — array of `{ value, label, count? }` (count enables showing "centos (142)")
- `selected` — array of selected values
- `onChange` — callback with updated selection array

### New: `FilterTypeAhead`

Debounced search input for high-cardinality dimensions. Sends `?q=` requests to a backend endpoint as the user types. Shows a dropdown of matching results. Supports single or multi-select mode.

**Props:**
- `label` — display name
- `endpoint` — URL to search (e.g. `/api/v1/filters/roles`)
- `debounceMs` — debounce interval (default 300ms)
- `minChars` — minimum characters before searching (default 2)
- `selected` — array of selected values
- `onChange` — callback with updated selection array
- `matchType` — `"prefix"` or `"substring"` (controls visual hint to user)

### Enhanced: `FilterCombobox`

Gains multi-select mode. When `multiSelect` is true, selected items render as chips and the dropdown stays open after selection. Client-side filtering uses the match strategy specified by a new `matchType` prop (default `"prefix"` for version fields, `"substring"` for name fields).

### Unchanged: `FilterInput`

Freeform text input. Still needed for node name and similar substring-search fields.

### Deprecated: `FilterSelect`

Single-select dropdown. Replaced by `FilterMultiCheckbox` for enum-like dimensions. Existing usages migrated. Component kept temporarily for backward compatibility but marked deprecated.

### Top Bar Layout

The top bar layout is extended with slots for global filter controls and the target version indicator to the right of the org selector. Layout order: Organisation selector → Target Chef version indicator (read-only, e.g. "Target: 18.5.0") → Staleness toggle. Responsive: on narrow viewports, global filters collapse into a "Filters" dropdown.

## Per-Page Filter Mapping

### Node List (8 filters)

| Filter | Tier | Component |
|---|---|---|
| Staleness | Global | Top bar toggle |
| Platform | Multi-select | `FilterMultiCheckbox` |
| Chef client version | Multi-select | `FilterCombobox` (multi, prefix match) |
| Environment | Multi-select | `FilterMultiCheckbox` or `FilterTypeAhead` (cardinality-dependent) |
| Readiness status | Multi-select | `FilterMultiCheckbox` |
| Role | Type-ahead | `FilterTypeAhead` |
| Policy name | Multi-select | `FilterMultiCheckbox` or `FilterTypeAhead` |
| Node name | Text | `FilterInput` |

### Cookbook List (3 filters)

| Filter | Tier | Component |
|---|---|---|
| Compatibility status | Multi-select | `FilterMultiCheckbox` |
| Complexity label | Multi-select | `FilterMultiCheckbox` |
| Cookbook name | Text | `FilterInput` |

### Git Repos (3 filters)

| Filter | Tier | Component |
|---|---|---|
| Compatibility status | Multi-select | `FilterMultiCheckbox` |
| Complexity label | Multi-select | `FilterMultiCheckbox` |
| Repo name | Text | `FilterInput` |

## URL Query Param Behaviour

- Global filters use fixed param names: `stale_status`
- Per-page multi-select filters use comma-separated values: `?platform=windows,centos&complexity_label=high,critical`
- Navigating between pages preserves global params and drops per-page params
- Browser back/forward updates filter state from URL
- Empty/default values are omitted from the URL to keep it clean

## Cardinality Threshold

Filters in the multi-select tier automatically degrade to type-ahead behaviour when the option count exceeds a configurable threshold (default: 200). The component fetches the option count from the filter endpoint and switches rendering mode accordingly. This handles deployments where environments or policy names are unexpectedly numerous.

## Accessibility

- All filter controls are keyboard-navigable (arrow keys, Enter, Escape)
- Multi-select checkboxes use `role="listbox"` with `aria-multiselectable="true"`
- Chip/tag removal buttons have `aria-label` describing the action (e.g. "Remove filter: centos")
- Type-ahead results use `aria-live="polite"` to announce result count to screen readers

## Testing

### Frontend

- `GlobalFilterContext`: persists across navigation, reflects in URL, restores from URL on load
- `FilterMultiCheckbox`: renders options, toggles selection, shows chips, clears all, keyboard navigation
- `FilterTypeAhead`: debounces requests, shows results, handles empty state, respects `minChars`
- `FilterCombobox` multi-select mode: tag rendering, stays open after selection, match strategy correctness
- Per-page filter bars: correct filters rendered per page, API calls include all active filters
- URL round-trip: set filters → copy URL → navigate to URL → filters restored

### Backend

- `?q=` parameter: returns prefix-matched subset, respects limit, returns all when omitted
- Comma-separated multi-value params: correct OR semantics within dimension
- Cross-dimension AND: multiple filter params combine with AND
- Empty and single-value params: backward compatible

## Migration Path

1. Implement `GlobalFilterContext` with staleness filter + read-only target version indicator
2. Add `FilterMultiCheckbox` and `FilterTypeAhead` components
3. Migrate node list filters (highest filter count, biggest UX pain)
4. Migrate cookbook and git repo filters
5. Add `?q=` backend support for roles endpoint (highest cardinality)
6. Extend `?q=` to other endpoints as needed
7. Deprecate `FilterSelect`
8. Remove `useTargetChefVersion` hook (target version comes from admin config, not user selection)

Each step is independently shippable and testable.

## Related Specifications

| Specification | Relevance |
|---|---|
| web-api.md | Filter query param contract, filter option endpoints |
| visualisation.md | Interactive filter dimensions (§ Interactive Filters) |
| project-conventions.md | Frontend conventions, naming |