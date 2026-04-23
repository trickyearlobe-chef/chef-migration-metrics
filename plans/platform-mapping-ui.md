# Plan: Platform Mapping UI (Phase 3)

## Goal

Enhance the platform mapping section of the TK config UI to be data-driven: show discovered platforms from the analyser alongside hypervisor templates, support glob pattern matching for group mappings, add per-mapping transport config and skip flags, and flag unmapped platforms.

## Specs to Read

- `.claude/specifications/kitchen-refactor.md` §Phase 3 (plan scope)
- `.claude/specifications/test-kitchen-config-ui.md` (existing UI spec)
- `.claude/specifications/kitchen-analyser.md` (discovered platforms data model)

## Spec Work

Write `platform-mapping-ui.md` spec before implementation — covers:
- Enhanced `PlatformMapEntry` data model (pattern support, skip flag, transport override)
- Pattern matching semantics (glob with `*` wildcard, evaluated in order, first match wins)
- New API endpoint: `GET /api/v1/admin/platform-mapping/status` (merged view of discovered platforms + current mappings + templates)
- Validation rules for unmapped platforms
- Frontend layout for the enhanced mapping UI

## Steps

1. Write spec: `.claude/specifications/platform-mapping-ui.md`
2. Create todo: `.claude/specifications/todo-platform-mapping-ui.md`
3. Backend — Enhance `PlatformMapEntry` type in `config.go` (add `pattern` bool, `skip` bool, `transport` override)
4. Backend — Pattern matching: `MatchPlatform(name string, entries []PlatformMapEntry)` with glob support + tests
5. Backend — New API handler: `GET /api/v1/admin/platform-mapping/status` — joins discovered platforms, current config mappings, and hypervisor templates into a unified response
6. Backend — Enhance validation in `handle_test_kitchen_config.go` — warn on unmapped discovered platforms
7. Frontend — Add types for new API response and enhanced `PlatformMapEntry`
8. Frontend — Add API function for platform mapping status
9. Frontend — Rewrite platform map section of `AdminTestKitchenPage.tsx` — discovered platforms list, template dropdowns, pattern editor, skip toggle, unmapped warnings
10. Tests throughout each step (TDD)

## Acceptance Criteria

- User sees all discovered platforms with their cookbook counts and current mapping status
- User can map platforms to hypervisor templates via dropdown (or to images if no hypervisor configured)
- Glob patterns (e.g. `rhel*`) match multiple discovered platforms; first-match-wins ordering
- User can mark platforms as "skip" (explicitly unmapped)
- Unmapped platforms are flagged with a warning count
- Transport override (username, password credential, SSH key credential) per mapping entry
- All mappings stored via existing `runtime_settings` mechanism (no new DB tables)
- Existing platform map entries without patterns continue to work (backward compatible)