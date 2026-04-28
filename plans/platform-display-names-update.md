# Platform Display Names — Update Defaults & Complete Gaps

## Goal

Update built-in platform display name mappings per revised spec and fill remaining implementation gaps (exports, filters).

## Specs to Read

- `.claude/specifications/platform-display-names.md` (already read)

## Steps

1. Update `DefaultMappings` in `internal/platform/display_names.go`
   - Add Win11 24H2 (10.0.26100), Win11 25H2 (10.0.26200)
   - Update Server 2025 to 10.0.26334 prefix
   - Add Ubuntu LTS entries (26.04, 24.04, 22.04, 20.04, 18.04)
   - Remove CentOS, Oracle, Amazon entries
2. Update tests in `display_names_test.go` — fix CentOS-referencing test, add Ubuntu test
3. Add `PlatformDisplayName` to export `readyNodeRow` — wire resolution into export generation
4. Enhance filter endpoint to return display names alongside raw strings
5. Update frontend filter handling if needed for new response shape
6. Run full test suite, commit per logical unit

## Acceptance Criteria

- `DefaultMappings` matches revised spec
- All existing tests pass with updated defaults
- Exports include `platform_display_name` field
- Filter endpoint returns display names
