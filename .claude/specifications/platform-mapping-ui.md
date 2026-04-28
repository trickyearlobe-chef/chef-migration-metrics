# Platform Mapping UI — Specification

## TL;DR

Data-driven platform mapping. Platforms are auto-discovered from two sources (kitchen analysis + node data) and presented in a table where each platform defaults to "skip" with a dropdown to select a VM image. No manual entry, no pattern/glob matching, no per-platform transport overrides.

## Problem

Operators must map discovered kitchen platform names to hypervisor VM images so Test Kitchen knows which template to clone. Previously this was a manual editor requiring operators to type platform names — error-prone across ~2000 cookbooks. The new design auto-discovers platforms and presents them for mapping.

## Scope

### In Scope

- Data-driven discovered platforms table (replaces manual editor)
- Two discovery sources merged: kitchen analyser platforms + node platform/version combos
- Each platform: skip (default) or map to an image via dropdown
- Explicit skip entries persisted (`skip: true`) — distinct from "not configured"
- Source badges (kitchen / nodes / both) and OS family badges
- Mapping status summary (mapped / unmapped / skipped counts)

### Out of Scope

- Pattern/glob matching (exact names only — platforms come from discovery)
- Per-platform transport overrides (auth is configured at image level)
- Manual "add platform" button (all platforms come from discovery)
- Auto-mapping suggestions (future enhancement)
- Per-cookbook mapping overrides

## Data Model

### PlatformMapEntry (unchanged)

Uses existing `config.PlatformMapEntry`. Only `kitchen_name`, `image`, and `skip` are used by the data-driven UI.

| Field | Type | Description |
|---|---|---|
| `kitchen_name` | string | Exact platform name from discovery |
| `image` | string | Name of an `ImageEntry` (empty when skip is true) |
| `skip` | bool | When true, platform is explicitly excluded from TK runs |

### PlatformMappingStatus (API Response)

| Field | Type | Description |
|---|---|---|
| `discovered_platforms` | []DiscoveredPlatformStatus | All platforms with mapping info |
| `templates` | []Template | Available hypervisor templates |
| `unmapped_count` | int | Platforms with no mapping entry |
| `skipped_count` | int | Platforms with explicit skip |
| `mapped_count` | int | Platforms mapped to an image |

### DiscoveredPlatformStatus

| Field | Type | Description |
|---|---|---|
| `platform_name` | string | Raw platform name |
| `normalised_name` | string | Normalised name from analyser |
| `os_family` | string | OS family (rhel, windows, debian, suse, other) |
| `cookbook_count` | int | Cookbooks using this platform (kitchen source) |
| `node_count` | int | Nodes with this platform (node source) |
| `source` | string | `kitchen`, `nodes`, or `both` |
| `transport_type` | string | Transport from kitchen configs (ssh, winrm) |
| `mapping_status` | string | `mapped`, `skipped`, or `unmapped` |
| `matched_entry_index` | int | Index of matching PlatformMapEntry (-1 if unmapped) |
| `matched_image` | string | Image name (empty if unmapped/skipped) |

## API

### GET /api/v1/admin/platform-mapping/status

Returns platform mapping status. Admin-only.

Combines:
1. Kitchen discovered platforms from `ListDiscoveredPlatforms`
2. Node platform/version combos from `CountNodePlatformDistribution`
3. Current platform map entries from effective TK config
4. Templates from the hypervisor (if configured)

Merge logic: kitchen platforms checked first; if a matching node platform exists, source becomes "both". Remaining node-only platforms appended sorted alphabetically.

### PUT /api/v1/admin/test-kitchen/config

Existing save endpoint. The frontend sends the full `platform_map` array containing one entry per discovered platform — either `{kitchen_name, image}` for mapped or `{kitchen_name, skip: true}` for skipped.

## Frontend

### Data-Driven Platform Map Section

Located in `AdminTestKitchenPage.tsx` as `PlatformMapSection`.

**Layout:**
1. **Status Summary** — "N mapped, N unmapped, N skipped" badges
2. **Discovered Platforms Table** — one row per discovered platform:
   - Platform name (text)
   - Source badge (kitchen / nodes / both)
   - OS family badge
   - Cookbook count and node count
   - Image dropdown: "— skip —" (default) + all configured images

### Data Flow

On page load: fetch TK config, platform mapping status, credentials.

State: `platformMappings: Record<string, string>` maps platform_name → image_name (empty string = skip).

On dropdown change: update `platformMappings`, sync to `config.platform_map` array.

On save: existing save handler sends `config.platform_map` to backend.

### Round-Trip

- Mapped platform → `{kitchen_name: "rhel-9", image: "rhel9-tmpl"}`
- Skipped platform → `{kitchen_name: "rhel-9", skip: true}`

## Testing

### Backend (13 tests)

- Status endpoint: empty platforms, empty config, full/partial mapping
- Node platform merging: node-only, kitchen+node overlap, mapped status
- Pattern matching: exact match, glob fallback, skip entries

### Frontend (9 tests)

- Discovered platforms render in table
- Source / OS family badges display correctly
- Cookbook and node counts shown
- Pre-selects mapped images; unmapped default to skip
- Image selection updates config state
- Mapping status badges render
- Empty state when no platforms discovered

## Related Specifications

| Specification | Relevance |
|---|---|
| kitchen-refactor.md | Phase 3 scope definition |
| test-kitchen-config-ui.md | Base platform map UI and runtime_settings storage |
| kitchen-analyser.md | Discovered platforms data model |
| test-kitchen-drivers.md | Image registry and transport config |