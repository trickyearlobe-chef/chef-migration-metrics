# Version Battery Bars — Component Specification

## Problem

The `VersionDistributionCard` shows one bar per exact Chef version (17.10.0, 17.9.2, 18.5.0, etc.). This works but makes it hard to answer "what fraction of our fleet is on Chef 17 vs 18 vs 16?" at a glance. Operators need the major-version picture first, with the ability to drill into minor versions on demand.

## Scope

### In Scope

- Battery-bar component: grouped horizontal bars with internal segments
- Integration into `VersionDistributionCard` replacing the current per-version bars
- Client-side grouping of existing `VersionDistributionResponse` data
- Expand/collapse interaction to reveal minor versions
- Colour strategy for major and minor version differentiation
- Accessibility: keyboard navigation, non-colour-only segment distinction
- Reusable component shape so platform distribution can adopt later

### Out of Scope

- Backend API changes (client-side grouping is sufficient)
- Platform distribution migration (evaluated separately after version card ships)
- Trend chart changes (the trend view retains its existing design)

## Data Model

### Input

The existing `VersionDistributionResponse` is unchanged:

| Field | Type | Description |
|---|---|---|
| `total_nodes` | number | Fleet-wide node count |
| `distribution` | VersionCount[] | `{ version, count, percent }` per exact version |

### Derived Grouping (client-side)

Group `distribution` entries by major version parsed from the semver string.

| Field | Type | Description |
|---|---|---|
| `majorVersion` | number | e.g. 18, 17, 16 |
| `totalCount` | number | Sum of `count` for all versions in this major |
| `totalPercentage` | number | `totalCount / total_nodes * 100` |
| `versions` | VersionCount[] | Original entries belonging to this major, sorted descending by version |

Sort groups by `majorVersion` descending (newest first).

## Battery Bar Design

### Layout (per major version)

```
[Label: Chef 18] [======|====|==          ] [1,234]
                  ^seg1  ^seg2 ^seg3
```

- **Left label** — "Chef {majorVersion}" (fixed width, right-aligned, matches existing `.bar-chart-label` width)
- **Bar track** — full width of the remaining space, same height as existing bars
- **Segments** — one contiguous segment per minor version within the track, ordered newest-minor-first (left to right)
- **Right count** — total node count for the major version (matches existing `.bar-chart-value` position)
- Bar track width is proportional to `totalPercentage` of the major version (same logic as current bars, with a minimum width so small groups remain visible)
- When `totalPercentage >= 8`, show the percentage inside the bar (same threshold as current)

### Segment Sizing

Each segment's width within the filled portion is proportional to its share of the major version's total count. Segments have a minimum width of 2px so they remain visible even for rare versions.

### Expanded State

When a battery bar is expanded, the minor versions appear below it as indented child rows — one row per exact version, styled like the current per-version bars but narrower and indented. Each child row:

- Shows the exact version string as label (e.g. "17.10.0")
- Has its own bar fill proportional to that version's fleet-wide percentage
- Is a `<Link>` to `/nodes?chef_version=X.Y.Z` (preserving existing navigation)
- Uses the segment colour matching its position in the parent battery bar

Only one major version should be expanded at a time (accordion behaviour) to avoid overwhelming the card. Clicking an already-expanded bar collapses it.

## Colour Strategy

### Major Version Base Colours

Assign base hue by major version using a semantic scale — newer is "healthier":

| Major Version | Base Colour | Rationale |
|---|---|---|
| Latest (e.g. 18) | Green (emerald-500) | Current/target version |
| Latest − 1 (e.g. 17) | Blue (blue-500) | Supported, upgrade encouraged |
| Latest − 2 (e.g. 16) | Amber (amber-500) | Aging, upgrade recommended |
| Latest − 3 and older | Red (red-500) | End-of-life, urgent |

The mapping is relative to the highest major version present in the data, not hardcoded to specific numbers.

### Minor Version Shades

Within a major version, segments use tints of the base colour:

- Newest minor → full base colour (e.g. emerald-600)
- Progressively older minors → lighter shades (e.g. emerald-400, emerald-300)
- Generate shades programmatically based on segment count (cap at 5–6 distinguishable shades; group the tail into an "other" segment if there are more)

### Segment Borders

Each segment has a 1px white or light border on its right edge to visually separate segments even when shades are similar.

## Interaction

### Hover

- Hovering over a segment in the battery bar shows a tooltip: "{version} — {count} nodes ({percent}%)"
- Hovering over the overall bar (outside a specific segment) shows: "Chef {major} — {totalCount} nodes ({totalPercentage}%)"

### Click / Expand

- Clicking anywhere on the battery bar row toggles the expanded state for that major version
- A visual affordance (chevron or caret icon on the left label) indicates expandability and current state (collapsed/expanded)
- Transition: child rows animate in/out with a brief slide or fade

### Keyboard Navigation

- Battery bar rows are focusable (`tabindex="0"` or rendered as `<button>`)
- Enter or Space toggles expand/collapse
- When expanded, Tab moves focus into the child version links
- Child version links are standard `<Link>` elements (already keyboard-accessible)
- Escape while focused on a child row collapses the parent and returns focus to the battery bar

### ARIA

- Battery bar row: `role="button"`, `aria-expanded="true|false"`
- Child row container: `role="region"`, `aria-labelledby` referencing the battery bar label
- Segments within the bar: `role="img"` with `aria-label` describing all segments for screen readers

## Accessibility

- Segments are bordered (see Colour Strategy) so they are distinguishable without colour alone
- When expanded, each minor version row has a text label — no colour-only information
- Tooltip content is also available via `aria-label` on each segment
- Colour contrast ratios for text inside bars and labels meet WCAG AA (same requirement as existing bars)

## CSS

### New Classes

Introduce `.battery-bar-*` classes alongside the existing `.bar-chart-*` classes:

| Class | Purpose |
|---|---|
| `.battery-bar-row` | Outer row container (replaces `.bar-chart-row` for grouped bars) |
| `.battery-bar-track` | The bar track containing segments |
| `.battery-bar-segment` | Individual coloured segment within the track |
| `.battery-bar-children` | Container for expanded child rows |
| `.battery-bar-child` | Individual child version row (indented bar-chart-row) |

Existing `.bar-chart-*` classes are not modified — they remain available for other cards.

## Component Structure

### BatteryBarChart (generic, reusable)

Accepts grouped data and renders the battery bars. Props contract:

| Prop | Type | Description |
|---|---|---|
| `groups` | GroupedBarData[] | Pre-grouped data (see Derived Grouping above) |
| `totalCount` | number | Denominator for percentage calculations |
| `baseColours` | Record<number, string> | Map of group index (0 = newest) to Tailwind colour prefix |
| `labelPrefix` | string | e.g. "Chef" — prepended to group label |
| `childLinkBuilder` | (version: string) => string | Builds the navigation URL for a child row |

### VersionDistributionCard

Continues to fetch `VersionDistributionResponse`. Adds a grouping step before rendering, then delegates to `BatteryBarChart`.

## Platform Distribution (Future)

The `BatteryBarChart` component is designed to be generic. For platform distribution:

- Group by OS family (e.g. "RHEL", "Windows", "Ubuntu") parsed from the platform string
- Each family becomes a battery bar; individual platform versions become segments
- `labelPrefix` would be empty; group label is the OS family name
- `childLinkBuilder` would produce `/nodes?platform=X`

This is out of scope for the initial implementation but the component contract should not preclude it.

## Testing

### Unit Tests — Grouping Logic

- Groups versions correctly by major version
- Sorts groups descending by major version
- Sorts versions within a group descending
- Handles single version, single group
- Handles empty distribution array
- Handles versions with no minor (e.g. "18")
- Calculates totalCount and totalPercentage correctly

### Unit Tests — Colour Assignment

- Newest major gets green, oldest gets red
- Shade generation produces distinguishable values for 1–6 minors
- Groups with > 6 minors produce an "other" tail segment

### Component Tests — BatteryBarChart

- Renders correct number of battery bar rows
- Bar width reflects totalPercentage
- Segments within a bar reflect individual version proportions
- Click toggles expanded state
- Only one group expanded at a time (accordion)
- Expanded state shows child rows with correct links
- Keyboard: Enter/Space toggles, Tab navigates children, Escape collapses
- ARIA attributes present and correct
- Tooltip shows on hover with correct content

### Component Tests — VersionDistributionCard

- Passes grouped data to BatteryBarChart
- Loading, error, and empty states unchanged
- Child row links navigate to `/nodes?chef_version=X.Y.Z`

## Migration

- The existing per-version bar rendering in `VersionDistributionCard` is replaced entirely
- No feature flag needed — the battery bar is a strict superset (all information is still accessible via expand)
- Existing `.bar-chart-*` CSS classes are retained for other consumers
- No API changes required

## References

| Specification | Relevance |
|---|---|
| visualisation.md | Parent spec for dashboard views and drill-down behaviour |
| web-api.md | API endpoint contracts |
| project-conventions.md | Frontend component and testing conventions |