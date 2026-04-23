# Platform Mapping UI — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Specification

- [x] Write spec: `platform-mapping-ui.md`
- [x] Write plan: `plans/platform-mapping-ui.md`
- [x] Create todo tracking file

## Backend — Data Model

- [x] Add `IsPattern` bool field to `PlatformMapEntry` in `config.go`
- [x] Add `Skip` bool field to `PlatformMapEntry` in `config.go`
- [x] Add `Transport` override field to `PlatformMapEntry` in `config.go`
- [x] Update JSON/YAML tags for new fields
- [x] Backward compatibility: existing configs without new fields still work

## Backend — Pattern Matching

- [x] `MatchPlatform(name string, entries []PlatformMapEntry) MatchResult` function
- [x] Glob pattern support (`*` and `?` wildcards)
- [x] First-match-wins ordering with exact-match priority
- [x] Skip entries return matched but with skip flag
- [x] Tests: exact match, glob `*`, glob `?`, first-match-wins, exact priority, skip, no match

## Backend — Platform Mapping Status API

- [x] `DiscoveredPlatformStatus` response type
- [x] `PlatformMappingStatusResponse` response type
- [x] `handlePlatformMappingStatus` handler — GET /api/v1/admin/platform-mapping/status
- [x] Merge discovered platforms + current config mappings + hypervisor templates
- [x] Compute unmapped/skipped/mapped counts
- [x] Route registration in `router.go` (admin-only)
- [x] Handler tests: empty platforms, empty config, full mapping, partial mapping, no hypervisor

## Backend — Enhanced Validation

- [x] Warn on unmapped discovered platforms (not blocking)
- [x] Warn on zero-match patterns
- [x] Skip entries allow empty image field
- [x] Pattern entries must contain wildcards
- [x] Pattern entries skip duplicate kitchen_name check
- [x] Update `validateTestKitchenConfig` with new rules
- [x] `generatePlatformMappingWarnings` method on Router
- [x] Tests for all validation and warning cases

## Frontend — Types and API

- [x] Add `is_pattern`, `skip`, `transport` to `PlatformMapEntry` type in `types.ts`
- [x] Add `DiscoveredPlatformStatus` type
- [x] Add `PlatformMappingStatusResponse` type
- [x] Add `HypervisorTemplate` type
- [x] Add `fetchPlatformMappingStatus` function in `api.ts`

## Frontend — Enhanced Platform Map UI

- [x] Mapping status banner (mapped/skipped/unmapped counts)
- [x] Mapping rules table with pattern toggle, skip toggle, transport override
- [x] Image dropdown populated from images list
- [x] Transport override section (expandable per entry via PlatformTransportEditor)
- [x] Unmapped platforms panel with quick-add button
- [x] OsFamilyBadge helper component
- [x] Add Rule button
- [x] Re-fetch status after save

## Integration

- [ ] End-to-end manual test: save mapping → re-fetch status → verify unmapped count updates
- [ ] Verify backward compatibility with existing platform_map configs