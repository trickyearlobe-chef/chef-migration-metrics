# Plan: Platform Display Names + Node List Check-Status Icons

## Goal

Implement two independent customer-requested features:
1. Platform display names — map opaque OS version strings to human-friendly labels
2. Node list per-check status icons — show disk/CookStyle/TK status at a glance

## Specs

- `platform-display-names.md` — full spec
- `node-list-enhancements.md` — full spec

## Part 1 — Platform Display Names

### Backend

1. Add default mappings constant in new `internal/platform/display_names.go`
2. Write tests for prefix matching (exact, longest-prefix-wins, case-insensitive, no-match fallback)
3. Implement `ResolveName(platform, version, mappings) string` with prefix matching
4. Add admin API endpoints (`GET/PUT /api/v1/admin/platform-display-names`, `POST .../reset`)
5. Wire storage via `config_store` table under key `platform_display_names`
6. Enrich node list/detail API responses with `platform_display_name` field
7. Enrich filter endpoint `GET /api/v1/filters/platforms` with display names per version
8. Tests for API endpoints, enrichment, and config store integration

### Frontend

9. Add `platform_display_name` to node TS types
10. Create `PlatformLabel` component — shows friendly name with raw tooltip
11. Update node list table, node detail page, dashboard platform card to use `PlatformLabel`
12. Update filter dropdowns to show friendly names
13. Create admin page for managing platform display name mappings (table + add/edit/delete/reset)
14. Tests for `PlatformLabel` component and admin page
15. Commit

### Acceptance Criteria

- Friendly names appear everywhere platform data is shown
- Raw version visible via tooltip
- Admin can add/edit/delete/reset mappings
- Built-in defaults for Windows builds and notable Linux milestones
- No-match falls back to raw `platform platform_version`

## Part 2 — Node List Check-Status Icons

### Backend

16. Add `disk_status`, `cookstyle_status`, `kitchen_status` + detail strings to node readiness response
17. Derivation: disk from `sufficient_disk_space`, cookstyle/kitchen from `blocking_cookbooks` verdicts
18. `cookstyle_status` must distinguish `"scan_error"` (tool crashed, e.g. bad `.rubocop.yml`) from `"failed"` (genuine incompatibility). The backend already stores `error_message` on CookStyle results — use it during derivation. Same for `kitchen_status`.
19. Tests for derivation logic (all status combinations, edge cases, stale nodes, scan errors)

### Frontend

20. Add per-check status fields to `NodeReadinessSummary` TS type
21. Write tests for `CheckStatusIcons` component (colour, shape overlay, tooltip, aria)
22. Implement `CheckStatusIcons` — three compact icons (disk, cookstyle, TK) with colour + overlay
23. Implement `CheckStatusIcon` (internal) — single icon with status-based colour/overlay/tooltip
24. `scan_error` status renders in orange with `!` overlay — distinct from red fail and grey unknown
25. Add "Checks" column to node list table after "Status" column
26. Also surface `scan_error` on cookbook list page — CookStyle compatibility badge should show "Scan Error" (orange) instead of "Incompatible" (red) when the scan crashed rather than finding real issues. Reuse the `scan_error` StatusBadge variant already added for `CookstyleResultRow`.
27. Tests for NodesPage integration (column renders, graceful degradation, scan error state)
28. Commit

### Acceptance Criteria

- Three icons per node row: disk, CookStyle, Test Kitchen
- Colour: green (pass), red (fail), amber (partial/warnings), orange (scan error), grey (unknown)
- Shape overlay: ✓/✗/~/!/? as secondary signal for accessibility
- Tooltips with detail text
- Graceful degradation when per-check fields absent (all-unknown)
- Scan errors clearly distinguished from genuine failures in node list AND cookbook list

## Wrap-Up

29. Update `customer-feedback.md` — mark both items implemented
30. Update `todo-visualisation.md` if applicable
31. Delete this plan