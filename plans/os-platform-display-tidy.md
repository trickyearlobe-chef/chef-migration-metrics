# OS Platform Display Tidy

## Goal

Implement centralized platform resolver with display names, grouping, and collapsible dashboard. Two-phase approach: phase 1 = display + grouping (this task), phase 2 = kitchen group-level mapping (separate).

## Specs

- `.claude/specifications/platform-display-grouping.md` (new, primary)
- `.claude/specifications/platform-display-names.md` (existing, unchanged)

## Steps

1. Add `platform_caption` column to node_snapshots (DB migration)
2. Collect `kernel.os_info.caption` / `lsb.description` during ingestion
3. Implement centralized resolver in `internal/platform/` (abbreviations, grouping, sort key)
4. Add Windows 10.0.14393 ("Win10 1607 / Server 2016") to DefaultMappings
5. Update dashboard API to return grouped response
6. Update all webapi callers to use centralized resolver
7. Update frontend dashboard card to collapsible groups
8. Update frontend node list display + sort
9. Update frontend filter dropdown to tree structure
10. Update exports with new columns

## Acceptance Criteria

- "redhat 8.10" displays as "RHEL 8.10" across all surfaces
- Dashboard groups by major version with expand/collapse
- Sort is numeric-aware (8.9 before 8.10)
- Admin display-name mappings override all automatic derivation
- Existing kitchen matching behaviour unchanged
- All tests pass
