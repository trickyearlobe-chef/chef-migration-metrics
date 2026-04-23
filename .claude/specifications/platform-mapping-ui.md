# Platform Mapping UI — Specification

## TL;DR

Enhance the existing Test Kitchen config platform map to be data-driven. Discovered platforms from the kitchen analyser and templates from the hypervisor are presented together so operators can map platforms to templates using exact names or glob patterns, configure transport per mapping, skip irrelevant platforms, and see unmapped platform warnings. Stored via existing `runtime_settings` mechanism.

## Problem

The current platform map is a manually-maintained list of `{kitchen_name, image}` pairs. Operators must know every platform name across ~2000 cookbooks and type them by hand. There is no visibility into which platforms are unmapped, no way to map groups of similar names with a single rule, and no connection to the hypervisor templates that the VMs will actually be cloned from.

## Scope

### In Scope

- Enhanced `PlatformMapEntry` with pattern support, skip flag, and transport override
- Glob pattern matching (`*` wildcard) for group mappings, first-match-wins
- API endpoint returning merged platform mapping status (discovered + mapped + templates)
- Validation: warn on unmapped discovered platforms
- Frontend: data-driven platform mapping section in `AdminTestKitchenPage.tsx`
- Backward compatibility: existing exact-match entries work unchanged

### Out of Scope

- New database tables (uses existing `runtime_settings`)
- Regex support (glob only — simpler, sufficient for platform name patterns)
- Auto-mapping suggestions (future enhancement)
- Per-cookbook mapping overrides

## Data Model

### Enhanced PlatformMapEntry

Extends the existing `config.PlatformMapEntry` struct. New fields are additive — existing configs with only `kitchen_name` + `image` continue to work.

| Field | Type | Default | Description |
|---|---|---|---|
| `kitchen_name` | string | required | Exact platform name or glob pattern (e.g. `rhel*`, `windows-201*`) |
| `image` | string | required unless skip | Name of an `ImageEntry` in the images list |
| `is_pattern` | bool | false | When true, `kitchen_name` is a glob pattern |
| `skip` | bool | false | When true, this platform is explicitly excluded from TK runs |
| `transport` | PlatformMapTransport | nil | Per-mapping transport override (overrides image-level transport) |

### Pattern Matching Semantics

- Patterns use glob syntax: `*` matches any sequence of characters, `?` matches a single character.
- Entries are evaluated **in order** — first match wins.
- Exact matches (non-pattern entries) are checked first regardless of position.
- A platform that matches no entry is "unmapped".
- A skipped entry counts as "mapped" for validation purposes (explicitly handled).

### PlatformMappingStatus (API Response)

The status endpoint returns a unified view combining three data sources:

| Field | Type | Description |
|---|---|---|
| `discovered_platforms` | []DiscoveredPlatformStatus | All platforms from kitchen analyser with mapping info |
| `templates` | []Template | Available hypervisor templates |
| `unmapped_count` | int | Number of discovered platforms with no matching entry |
| `skipped_count` | int | Number of discovered platforms matched by a skip rule |
| `mapped_count` | int | Number of discovered platforms with a non-skip mapping |

### DiscoveredPlatformStatus

| Field | Type | Description |
|---|---|---|
| `platform_name` | string | Raw platform name from kitchen configs |
| `normalised_name` | string | Normalised name from analyser |
| `os_family` | string | OS family (rhel, windows, debian, suse, other) |
| `cookbook_count` | int | Number of cookbooks using this platform |
| `transport_type` | string | Transport from kitchen configs (ssh, winrm) |
| `mapping_status` | string | `mapped`, `skipped`, or `unmapped` |
| `matched_entry_index` | int | Index of the matching PlatformMapEntry (-1 if unmapped) |
| `matched_image` | string | Image name from the matching entry (empty if unmapped/skipped) |

## API

### GET /api/v1/admin/platform-mapping/status

Returns the platform mapping status. Admin-only.

Combines:
1. Discovered platforms from `ListDiscoveredPlatforms`
2. Current platform map entries from effective TK config (DB or file)
3. Templates from the hypervisor (if configured)

Response `200 OK`:

```
{
  "discovered_platforms": [
    {
      "platform_name": "rhel7-chef16",
      "normalised_name": "rhel-7",
      "os_family": "rhel",
      "cookbook_count": 145,
      "transport_type": "ssh",
      "mapping_status": "mapped",
      "matched_entry_index": 0,
      "matched_image": "rhel7-template"
    },
    {
      "platform_name": "custom-test-box",
      "normalised_name": "custom-test-box",
      "os_family": "other",
      "cookbook_count": 2,
      "transport_type": "",
      "mapping_status": "unmapped",
      "matched_entry_index": -1,
      "matched_image": ""
    }
  ],
  "templates": [...],
  "unmapped_count": 5,
  "skipped_count": 3,
  "mapped_count": 72
}
```

### PUT /api/v1/admin/test-kitchen/config (enhanced validation)

The existing save endpoint gains additional validation warnings:
- Warn when discovered platforms are unmapped (not an error — config can still be saved)
- Warn when a pattern matches zero discovered platforms (likely typo)
- Warn when multiple patterns could match the same platform (order-dependent)

These are returned as `warnings` in the response, not blocking errors.

## Frontend

### Enhanced Platform Map Section

Replaces the simple kitchen_name + image table in `AdminTestKitchenPage.tsx`.

**Layout:**
1. **Mapping Status Banner** — summary: "72 mapped, 3 skipped, 5 unmapped" with colour coding
2. **Mapping Rules Table** — ordered list of platform map entries (exact and pattern):
   - Drag handle for reordering (affects pattern match priority)
   - Kitchen name input (text, with pattern toggle)
   - Pattern toggle (checkbox — enables glob matching)
   - Image dropdown (populated from images list)
   - Skip toggle (checkbox — disables image dropdown when checked)
   - Transport override (expandable: username, password credential dropdown, SSH key credential dropdown)
   - Remove button
3. **Unmapped Platforms Panel** — discovered platforms with no matching rule, sorted by cookbook count descending. Each row shows platform name, normalised name, OS family badge, cookbook count. Quick-action button to add an exact mapping rule for the platform.
4. **Add Rule** button — appends a new empty mapping entry

### Data Flow

On page load:
1. Fetch TK config (existing `fetchTestKitchenConfig`)
2. Fetch platform mapping status (`GET /api/v1/admin/platform-mapping/status`)
3. Fetch credentials (existing, for transport dropdowns)

On save:
1. Save via existing `PUT /api/v1/admin/test-kitchen/config`
2. Re-fetch platform mapping status to update unmapped panel

### Interaction Details

- Clicking "Add mapping" on an unmapped platform pre-fills the kitchen_name
- Pattern entries show a visual indicator (e.g. `*` icon) and a tooltip showing which discovered platforms they match
- Skipped entries are visually dimmed
- Unmapped platform count shown as a badge on the section header

## Validation

### Client-Side

- kitchen_name required on every entry
- image required when skip is false
- No duplicate exact kitchen_name values
- Pattern entries must contain at least one `*` or `?` when is_pattern is true

### Server-Side

- All client-side checks repeated
- Warn (not error) on unmapped discovered platforms
- Warn on zero-match patterns
- Warn on overlapping patterns

## Testing

### Backend

- Pattern matching: exact match, glob with `*`, glob with `?`, first-match-wins ordering, exact-match priority over patterns, skip entries, no match
- Status endpoint: empty platforms, empty config, full mapping, partial mapping, with/without hypervisor
- Enhanced validation: unmapped warning, zero-match pattern warning, overlapping pattern warning

### Frontend

- Mapping status banner renders correct counts
- Unmapped platforms panel shows correct platforms
- Add-mapping from unmapped platform pre-fills kitchen_name
- Skip toggle disables image dropdown
- Pattern toggle enables glob matching
- Reordering updates match priority

## Related Specifications

| Specification | Relevance |
|---|---|
| kitchen-refactor.md | Phase 3 scope definition |
| test-kitchen-config-ui.md | Base platform map UI and runtime_settings storage |
| kitchen-analyser.md | Discovered platforms data model |
| test-kitchen-drivers.md | Image registry and transport config |