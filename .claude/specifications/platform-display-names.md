# Platform Display Names — Specification

## TL;DR

Map opaque platform version strings (especially Windows build numbers) to human-friendly display names. A configurable lookup table translates `(platform, version_prefix)` → `display_name` with prefix matching. Ships with sensible defaults for known Windows builds. Admin-editable via the UI. Friendly names appear consistently across every surface that shows platform data.

## Problem

Ohai reports `platform` and `platform_version` as raw OS identifiers. For most Linux distributions these are tolerable (`ubuntu 22.04`, `redhat 9.3`). Windows is the worst offender — `10.0.22631` could be Win11 23H2, `10.0.19045` could be Win10 22H2, and `10.0.20348` is Server 2022. Users cannot reasonably identify these. CentOS/RHEL/Rocky rebuild numbering also causes confusion (e.g. knowing that CentOS 8 is EOL).

Without friendly names, every surface that displays platform data — dashboard cards, node lists, filters, exports, kitchen platform mapping — forces users to mentally decode version strings or look them up externally.

## Scope

### In Scope

- Lookup table mapping `(platform, platform_version_prefix)` → `display_name`
- Prefix matching so `10.0.22631` matches entries for `10.0.22631.xxxx` build variants
- Built-in default mappings for known Windows builds and notable Linux milestones
- Admin UI for managing mappings (add, edit, delete, reset to defaults)
- Friendly name display across all platform-showing surfaces
- `platform_display_name` field added to API responses alongside existing fields
- Filter endpoints return friendly names in option lists

### Out of Scope

- Automatic detection or scraping of new Windows build numbers
- Changing the raw `platform` / `platform_version` values collected from Ohai
- Per-organisation display name overrides (one global table)

## Data Model

### Mapping Entry

| Field | Type | Description |
|---|---|---|
| `platform` | string | Ohai platform value, lowercase (e.g. `windows`, `centos`) |
| `version_prefix` | string | Version prefix to match against `platform_version` |
| `display_name` | string | Human-friendly name shown in the UI |

**Matching rules:**

- A node matches an entry when `node.platform == entry.platform` AND `node.platform_version` starts with `entry.version_prefix`.
- Longer prefixes take priority (most-specific match wins). `10.0.22631` beats `10.0` beats `10`.
- Matching is case-insensitive for platform; version prefix is compared as a string prefix.
- If no entry matches, display falls back to the raw `platform platform_version` string (no change from current behaviour).

### Storage

Stored in the `config_store` table under key `platform_display_names`. Value is a JSON array of mapping entries. This is consistent with how other admin-editable configuration is stored (see encrypted-config-store spec).

### Built-in Defaults

The application ships with the following default mappings. These are loaded into `config_store` on first startup if no `platform_display_names` key exists. The admin can reset to these defaults at any time.

**Windows Desktop:**

| Platform | Version Prefix | Display Name |
|---|---|---|
| windows | 10.0.26200 | Win11 25H2 |
| windows | 10.0.26100 | Win11 24H2 / Server 2025 |
| windows | 10.0.22631 | Win11 23H2 |
| windows | 10.0.22621 | Win11 22H2 |
| windows | 10.0.22000 | Win11 21H2 |
| windows | 10.0.19045 | Win10 22H2 |
| windows | 10.0.19044 | Win10 21H2 |
| windows | 10.0.19043 | Win10 21H1 |
| windows | 10.0.19042 | Win10 20H2 |
| windows | 10.0.18363 | Win10 1909 |
| windows | 10.0.17763 | Win10 1809 / Server 2019 |
| windows | 6.3.9600 | Win8.1 / Server 2012 R2 |
| windows | 6.2.9200 | Win8 / Server 2012 |
| windows | 6.1.7601 | Win7 SP1 / Server 2008 R2 |

**Windows Server:**

| Platform | Version Prefix | Display Name |
|---|---|---|
| windows | 10.0.20348 | Win Server 2022 |
| windows | 10.0.26334 | Win Server 2025 |

**Ubuntu:**

| Platform | Version Prefix | Display Name |
|---|---|---|
| ubuntu | 26.04 | Ubuntu 26.04 LTS (Plucky) |
| ubuntu | 24.04 | Ubuntu 24.04 LTS (Noble) |
| ubuntu | 22.04 | Ubuntu 22.04 LTS (Jammy) |
| ubuntu | 20.04 | Ubuntu 20.04 LTS (Focal) |
| ubuntu | 18.04 | Ubuntu 18.04 LTS (Bionic) — EOL |

Most Linux distributions use readable major.minor versioning (e.g. `redhat 9.3`, `centos 7`) and do not need display name mappings. Ubuntu is included because its YY.MM versioning is less intuitive and codenames are widely used. Admins can add mappings for any platform via the admin UI.

Note: The `10.0.17763` prefix maps to both Win10 1809 and Server 2019 because Ohai reports the same build number for both. Similarly, `10.0.26100` is shared between Win11 24H2 and some Server 2025 builds. These display names include both possibilities. The more-specific `10.0.26334` prefix distinguishes Server 2025 builds that report a higher build number. Admins can override if their fleet is exclusively one or the other.

### Keeping Defaults Current

The built-in defaults are compiled into the application and updated with each release. Between releases, admins can add new Windows builds immediately via the admin UI as they appear in their fleet. The "Reset to Defaults" action brings back the latest shipped set — useful after upgrading to pick up newly added mappings without losing custom entries (a merge strategy may be offered in future).

## Display Behaviour

### Resolution

Everywhere a platform + version is shown, the application resolves the display name:

1. Look up `(platform, platform_version)` in the mapping table using prefix matching (longest match wins).
2. If a match is found: use `display_name` as the primary label.
3. If no match is found: use `"platform platform_version"` as today.

### Tooltip

When a friendly name is displayed, the raw `platform platform_version` value must be available via hover tooltip. This ensures the original Ohai data is never hidden — users can always see the exact version if needed.

### Consistency

Friendly names must be used on **all** surfaces that show platform data:

| Surface | Current Display | With Friendly Names |
|---|---|---|
| Dashboard platform distribution card | `windows 10.0.22631` | `Win11 23H2` |
| Node list table | `windows 10.0.22631` | `Win11 23H2` |
| Node detail page | `windows 10.0.22631` | `Win11 23H2` |
| Filter dropdowns (`FilterCombobox`) | `windows 10.0.22631` | `Win11 23H2` |
| Platform distribution charts | `windows 10.0.22631` | `Win11 23H2` |
| Data exports (CSV, JSON) | `windows 10.0.22631` | Both: `platform_display_name` + raw fields |
| Kitchen platform mapping UI | `windows 10.0.22631` | `Win11 23H2` |
| Cookbook detail `nodes_by_platform` | `windows 10.0.22631` | `Win11 23H2` |

### Grouping

Platform distribution views should group by display name, not by raw version. If two raw versions map to the same display name (unlikely but possible if an admin configures it), their node counts are combined in distribution charts.

## Kitchen Platform Mapping Integration

The platform mapping UI (see `platform-mapping-ui.md`) shows discovered platforms when configuring kitchen image mappings. With display names enabled:

- The `DiscoveredPlatformStatus` entries in the mapping UI show friendly names instead of raw versions where a mapping exists.
- Raw values remain visible in a tooltip or secondary column for disambiguation.
- This is purely a display concern — the underlying platform map entries still reference raw platform names as matched by the kitchen analyser.

## API

### Admin Endpoints

**`GET /api/v1/admin/platform-display-names`**

Returns all configured display name mappings. Admin-only.

Response `200 OK`:

| Field | Type | Description |
|---|---|---|
| `mappings` | array | List of mapping entries |
| `mappings[].platform` | string | Ohai platform value |
| `mappings[].version_prefix` | string | Version prefix |
| `mappings[].display_name` | string | Friendly display name |
| `is_default` | boolean | True if the current mappings are unmodified defaults |

**`PUT /api/v1/admin/platform-display-names`**

Bulk-replaces all display name mappings. Admin-only. Accepts the same `mappings` array structure. The entire table is replaced — omitted entries are deleted.

Request body:

| Field | Type | Required | Description |
|---|---|---|---|
| `mappings` | array | Yes | Complete list of mapping entries |

Response `200 OK`: Same shape as the GET response.

**`POST /api/v1/admin/platform-display-names/reset`**

Resets mappings to the built-in defaults. Admin-only.

Response `200 OK`: Same shape as the GET response, with `is_default: true`.

### Enrichment of Existing API Responses

All API responses that currently include `platform` and `platform_version` fields gain an additional `platform_display_name` field:

| Field | Type | Description |
|---|---|---|
| `platform_display_name` | string or null | Friendly name if a mapping exists, null otherwise |

This is additive — existing `platform` and `platform_version` fields are unchanged. Consumers that do not read `platform_display_name` are unaffected.

**Affected endpoints:**

- `GET /api/v1/nodes` — each node in the list
- `GET /api/v1/nodes/:organisation/:name` — node detail
- `GET /api/v1/dashboard/version-distribution` — if platform breakdown is included
- `GET /api/v1/cookbooks/:name` — `nodes_by_platform` entries
- `POST /api/v1/exports` — exported data includes the field

### Filter Endpoint Enhancement

`GET /api/v1/filters/platforms` currently returns:

```
[{ "platform": "windows", "versions": ["10.0.22631", "10.0.19045"] }]
```

Enhanced response adds display names per version:

| Field | Type | Description |
|---|---|---|
| `data[].platform` | string | Platform name |
| `data[].versions` | array | Version entries |
| `data[].versions[].version` | string | Raw version string |
| `data[].versions[].display_name` | string or null | Friendly name if mapped |

This is a breaking change to the versions array (string → object). The frontend must be updated in the same change. No external consumers exist for this endpoint.

## Admin UI

### Location

New section within the existing admin area. Can be implemented as:
- A tab within the platform mapping page (alongside kitchen platform mapping), or
- A standalone admin page under "Platform Display Names"

Either approach is acceptable. The key requirement is discoverability — admins should find it when looking at platform-related configuration.

### Layout

**Mapping table** with columns:

| Column | Editable | Description |
|---|---|---|
| Platform | Yes (text input) | Ohai platform value |
| Version Prefix | Yes (text input) | Prefix to match |
| Display Name | Yes (text input) | Friendly name |
| Actions | — | Edit / Delete buttons |

**Controls:**

- "Add Mapping" button — appends a new empty row
- "Reset to Defaults" button — replaces all mappings with built-in defaults (confirmation dialog required)
- "Save" button — bulk-saves via `PUT /api/v1/admin/platform-display-names`

### Validation

- Platform and version_prefix are required and must not be empty.
- Display name is required and must not be empty.
- No duplicate `(platform, version_prefix)` pairs.
- Platform value should be lowercase (auto-normalise on save).

### Preview

When editing, the admin UI should show a count of how many current nodes match each mapping entry. This helps admins verify that their prefixes are correct and that mappings are useful.

## Testing

### Backend

- Prefix matching: exact match, prefix match, longest-prefix-wins, no match fallback
- Case insensitivity for platform matching
- Default mappings loaded on first startup
- Default mappings not overwritten if custom mappings exist
- API CRUD: list, bulk update, reset to defaults
- `platform_display_name` field present in node list and node detail responses
- `platform_display_name` is null when no mapping matches
- Filter endpoint returns display names alongside versions
- Export data includes `platform_display_name` field

### Frontend

- Friendly names render in node list, node detail, dashboard cards, filters
- Tooltip shows raw version on hover over friendly name
- Filter dropdown shows friendly names and filters correctly
- Admin page: add, edit, delete mappings
- Admin page: reset to defaults with confirmation
- Admin page: validation prevents empty fields and duplicate entries
- Admin page: node count preview per mapping entry

## Related Specifications

| Specification | Relevance |
|---|---|
| platform-mapping-ui.md | Kitchen platform mapping — uses friendly names for display |
| encrypted-config-store.md | Storage mechanism (`config_store` table) |
| visualisation.md | Dashboard and chart surfaces where platform data appears |
| web-api.md | API endpoints enriched with `platform_display_name` |
| data-export.md | Export format includes friendly names |