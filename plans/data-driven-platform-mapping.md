# Data-Driven Platform Mapping

## Goal

Replace manual platform map with auto-discovered platforms table. Each platform defaults to skip, with image dropdown for mapping.

## Specs

- `.claude/specifications/platform-mapping-ui.md`
- `.claude/specifications/test-kitchen-config-ui.md`

## Phases

### Phase 1 — CSS Fix

Fix invisible platform name input in AdminTestKitchenPage (flex layout `w-full` conflict).

### Phase 2 — Node Platforms in Mapping API

Add node platform/version combos to `GET /api/v1/admin/platform-mapping/status` alongside git kitchen discovered platforms.

### Phase 3 — Data-Driven Platform Map UI

Replace manual platform map editor with table of all discovered platforms. Each row: platform name (read-only), source, OS family, count, image dropdown (skip default).

### Phase 4 — Clean Up

Remove pattern/glob UI, old plan file, update spec.

## Acceptance Criteria

- Platform name input visible (CSS fix)
- All discovered platforms appear in mapping table
- Each has skip/image dropdown
- Save persists via runtime_settings
- Git kitchen planner/overlay unaffected
- All tests pass
