# Platform Display and Grouping — Specification

## TL;DR

Centralized platform resolver producing `display_name`, `group_key`, `group_display_name`, and `sort_key` for every node. Extends the existing display-name mapping system with: (a) collection of OS caption data from Ohai, (b) platform abbreviation fallback, (c) platform-family-aware major-version grouping. Consistent presentation across dashboard, node list, filters, kitchen mapping UI, and exports. No changes to kitchen execution matching (phase 2).

## Problem

The dashboard shows raw Ohai identifiers ("redhat 8.10", "windows 10.0.20348", "aix 7.2"). The existing display-name mapping solves Windows build numbers but:

- RHEL/AIX/CentOS/Rocky/SUSE show lowercase raw names with no proper capitalisation
- No grouping concept exists — operators cannot see "all RHEL 8" at a glance
- The dashboard lists every minor version individually, creating a long tail of tiny bars
- Different OS families need different grouping logic (RHEL by major, AIX standalone, Windows by product generation, Ubuntu by LTS release)
- Ohai provides `kernel.os_info.caption` (Windows) and `lsb.description` (Debian family) which definitively identify the OS but we don't collect them

## Scope

### In Scope

- New `platform_caption` column collected from Ohai
- Centralized `ResolveplatformInfo` function returning display name, group key, group display name, and sort key
- Platform abbreviation map for RHEL family, AIX, SUSE (compiled-in)
- Platform-family-aware grouping rules
- Collapsible group display on dashboard platform distribution card
- Grouped filter dropdowns
- Node list display names and numeric-aware sorting

### Out of Scope

- Kitchen group-level image mapping (phase 2 — separate spec)
- Changes to kitchen execution matching or `kitchen_name` config keys
- Per-organisation display overrides
- Auto-detection of new Windows builds
- Cross-distro merging (e.g. redhat + rocky into one group)

## Data Collection

### New Ohai Fields

Collect during node ingestion alongside existing `platform`, `platform_version`, `platform_family`:

| Source field | Condition | Description |
|---|---|---|
| `kernel.os_info.caption` | `platform == "windows"` | e.g. "Microsoft Windows Server 2022 Datacenter" |
| `lsb.description` | `platform_family in (debian, rhel, suse)` | e.g. "Ubuntu 24.04.4 LTS" (often empty on RHEL) |

### Storage

New nullable column on `node_snapshots`:

| Column | Type | Description |
|---|---|---|
| `platform_caption` | text, nullable | Raw caption from Ohai (whichever source field was populated) |

Existing nodes without caption continue to work — resolution falls back through other tiers.

## Centralized Resolver

### Function Signature (Go)

```
type PlatformInfo struct {
    DisplayName      string // "RHEL 8.10", "Windows Server 2022"
    GroupKey         string // Stable machine key: "rhel:redhat:8", "windows-server:2022"
    GroupDisplayName string // "RHEL 8", "Windows Server 2022"
    SortKey          string // "rhel:redhat:00008:00010" (numeric-padded)
}
```

All surfaces call this single resolver. No surface resolves display names independently.

### Resolution Precedence (display name)

1. **Explicit admin mapping** — `(platform, version_prefix)` table (existing, admin-editable)
2. **Caption-derived** — parsed from `platform_caption` if present (strip "Microsoft ", strip edition suffix for Windows; use as-is for lsb.description)
3. **Abbreviation fallback** — compiled-in map producing `Abbreviation + " " + version`
4. **Raw fallback** — `"platform platform_version"`

Admin mappings always win. This ensures operator-curated names override any automatic derivation.

### Abbreviation Map (compiled-in)

| `platform` | Abbreviation |
|---|---|
| `redhat` | RHEL |
| `centos` | CentOS |
| `rocky` | Rocky |
| `almalinux` | AlmaLinux |
| `oracle` | Oracle Linux |
| `amazon` | Amazon Linux |
| `aix` | AIX |
| `sles` | SLES |
| `suse` | SUSE |
| `opensuse` | openSUSE |
| `fedora` | Fedora |
| `mac_os_x` | macOS |
| `macos` | macOS |

Platforms not in this map and not matching a mapping entry fall through to raw `"platform version"`.

## Grouping Rules

Group key derivation depends on `platform_family`. Each distro stays separate (no cross-distro merging).

### RHEL Family (`platform_family == "rhel"`)

- Group by: `Abbreviation(platform)` + major version (first integer before first `.`)
- Group key: `rhel:<platform>:<major>` (e.g. `rhel:redhat:8`, `rhel:centos:7`)
- Group display: `"RHEL 8"`, `"CentOS 7"`, `"AlmaLinux 9"`
- Sub-items: minor versions (8.6, 8.7, 8.8, 8.9, 8.10)
- Example: `redhat 8.10` → group "RHEL 8", display "RHEL 8.10"

### Windows (`platform_family == "windows"`)

- Group by: product generation derived from caption or display-name mapping
- Parse logic: strip "Microsoft ", strip edition suffix (Datacenter, Standard, Pro, Education, Enterprise, Home, N variants)
- Group key: `windows:<generation>` (e.g. `windows:server-2022`, `windows:10`, `windows:11`)
- Group display: `"Windows Server 2022"`, `"Windows 10"`, `"Windows 11"`
- Sub-items: editions (if multiple exist in fleet) or build variants
- Fallback (no caption, no mapping): group by prefix of `platform_version` using `product_type` if available

### Debian Family (`platform_family == "debian"`)

- Ubuntu: group by `YY.MM` (the LTS release)
  - Group key: `debian:ubuntu:<YY.MM>` (e.g. `debian:ubuntu:24.04`)
  - Group display: `"Ubuntu 24.04"`
  - Sub-items: point releases (24.04.1, 24.04.4)
- Debian proper: group by major version
  - Group key: `debian:debian:<major>`
  - Group display: `"Debian 12"`

### AIX (`platform_family == "aix"`)

- Each `M.m` release is standalone (released far apart)
- Group key: `aix:aix:<M.m>` (e.g. `aix:aix:7.2`)
- Group display: `"AIX 7.2"`
- No sub-items (group = item)

### SUSE (`platform_family == "suse"`)

- Group by major version
- Group key: `suse:<platform>:<major>` (e.g. `suse:sles:15`)
- Group display: `"SLES 15"`, `"openSUSE 15"`
- Sub-items: service packs / minor versions

### macOS (`platform_family == "mac_os_x"`)

- Group by major version
- Group key: `macos:mac_os_x:<major>`
- Group display: `"macOS 14"`, `"macOS 15"`

### Default (unknown families)

- Group by: `platform + major_version`
- Group key: `other:<platform>:<major>`
- Group display: `"platform major"`

## Sort Key

Construct a sort key enabling correct ordering:

```
<platform_family>:<platform>:<zero-padded-major>:<zero-padded-minor>:<zero-padded-patch>
```

Example: `redhat 8.10` → `rhel:redhat:00008:00010:00000`

This ensures numeric ordering (8.9 before 8.10) and groups same-family distros together.

## Surface Behaviour

### Dashboard Platform Distribution Card

- Default view: collapsed groups, one row per group with total count/percentage
- Click to expand: reveals child rows (minor versions) with individual counts
- Accordion behaviour (one group open at a time)
- Reuses the battery-bar component pattern from version distribution
- Group row links to node list filtered by group; child rows link filtered by exact version

### Node List

- Platform column shows `display_name` ("RHEL 8.10", "Windows Server 2022 Datacenter")
- Tooltip shows raw `platform platform_version`
- Server-side sort by `sort_key` for correct numeric ordering

### Filter Dropdown

- Tree structure grouped by `group_display_name`
- Selecting a group filters to all versions in that group
- Expandable to select specific versions
- Search matches against both display name and raw value

### Kitchen Platform Mapping UI (display only)

- Shows display names instead of raw values
- Grouped by `group_display_name` for visual organisation
- Raw value shown in secondary column or tooltip
- Matching logic unchanged — still uses exact `kitchen_name`

### Data Exports

- Existing `platform` and `platform_version` columns unchanged
- New `platform_display_name` column added
- New `platform_group` column added

## API Changes

### Enriched Fields

All API responses that include platform data gain:

| Field | Type | Description |
|---|---|---|
| `platform_display_name` | string | Resolved friendly name |
| `platform_group_key` | string | Stable machine key for the group |
| `platform_group_display_name` | string | Friendly group label |

### Dashboard Platform Distribution Response

```json
{
  "total_nodes": 110009,
  "groups": [
    {
      "group_key": "rhel:redhat:8",
      "group_display_name": "RHEL 8",
      "total_count": 32493,
      "total_percent": 29.5,
      "versions": [
        { "platform": "redhat 8.10", "display_name": "RHEL 8.10", "count": 31777, "percent": 28.9 },
        { "platform": "redhat 8.9", "display_name": "RHEL 8.9", "count": 542, "percent": 0.5 },
        { "platform": "redhat 8.8", "display_name": "RHEL 8.8", "count": 142, "percent": 0.1 }
      ]
    }
  ]
}
```

The existing flat `distribution` array remains available for backward compatibility.

## Migration

- Existing `platform_display_names` config store entries remain valid and take highest priority
- New `platform_caption` column added via migration (nullable, no backfill required)
- Nodes gain caption data naturally on next collection run
- No breaking changes to existing API consumers (new fields are additive)
- Frontend updated to use grouped response when available, flat as fallback

## Testing

### Backend

- Resolver: all precedence tiers exercised (admin mapping wins over caption wins over abbreviation wins over raw)
- Grouping: correct group key for each platform family
- Sort key: numeric ordering verified (8.9 < 8.10, 7.1 < 7.2)
- Windows caption parsing: strips "Microsoft", strips editions correctly
- Edge cases: empty version, empty platform, unknown family, nil caption
- API: grouped response structure validated
- Migration: column addition, nullable handling

### Frontend

- Dashboard: groups render collapsed, expand on click, accordion behaviour
- Node list: display names shown, tooltip with raw value
- Filters: tree structure, group selection expands to all children
- Kitchen mapping UI: display names shown, raw in tooltip
- Exports: new columns present

## Related Specifications

| Spec | Relevance |
|---|---|
| `platform-display-names.md` | Existing admin-editable mapping table (unchanged, tier 1) |
| `version-battery-bars.md` | Battery bar component reused for group expand/collapse |
| `platform-mapping-ui.md` | Kitchen mapping display (phase 1: display only) |
| `visualisation.md` | Dashboard card surfaces |
| `data-collection.md` | New Ohai fields collected |
