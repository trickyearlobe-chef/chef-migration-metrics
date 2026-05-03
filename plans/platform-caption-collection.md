# Phase 2: Platform Caption Collection

## Goal

Wire `platform_caption` end-to-end so the tier-2 resolver (already implemented in `resolve_info.go`) receives real data. Windows nodes get authoritative product names; Ubuntu/Debian nodes get LSB descriptions.

## Specs to Read

- `.claude/specifications/platform-display-grouping.md` (sections: Data Collection, Storage, Centralized Resolver)

## Observations from Diagnostic Bundle

- 110K nodes (51K Windows, 57K RHEL, 214+13 AIX)
- Windows `10.0.17763` currently maps ambiguously ("Win10 1809 / Server 2019") — caption resolves this
- AIX groups as `other:aix:7` instead of `aix:aix:7.2` — platform_family likely empty on old clients (Phase 1 bug, out of scope)
- No existing caption data (expected — this is what we're adding)

## Changes Required

### 1. DB Migration (0028)

Add nullable `platform_caption TEXT` column to `node_snapshots`.

### 2. Chef API — Partial Search Keys

In `NodeSearchAttributes()`, add:
- `"kernel_os_info_caption": {"kernel", "os_info", "caption"}` — Windows
- `"lsb_description": {"lsb", "description"}` — Debian/Ubuntu/some RHEL

### 3. NodeData Accessor

Add `PlatformCaption()` method to `NodeData` that returns whichever is non-empty:
- For Windows (`platform == "windows"`): use `kernel_os_info_caption`
- Otherwise: use `lsb_description`

### 4. InsertNodeSnapshotParams + Struct

Add `PlatformCaption string` to both `InsertNodeSnapshotParams` and `NodeSnapshot`.

### 5. Datastore — Upsert/Scan

Update `upsertNodeSnapshot` query and `scanNodeSnapshot` to include `platform_caption`.

### 6. Collector — Extraction

In the snapshot params builder (~line 845), set `PlatformCaption: nd.PlatformCaption()`.

### 7. API Surfaces — Pass Caption to ResolveInfo

Update all call sites currently passing `""` for caption:
- `handle_dashboard_platform.go` — needs caption from aggregation query
- `handle_filters.go` — needs caption from node data
- `export/ready_nodes.go` — needs caption from node record
- `export/blocked_nodes.go` — needs caption from node record
- `handle_admin_diagnostic.go` — needs caption from aggregation
- `handle_platform_display_names.go` — preview endpoint

**Aggregation strategy (from rubber-duck review):** Do NOT collapse caption in SQL (MAX/MIN is unsafe — same build number can be both Win10 and Server 2019). Instead, query `(platform, version, family, caption, count)` per distinct combination, call `ResolveInfo` per row, then merge results by resolved `GroupKey` in Go. This avoids mislabelling mixed Windows buckets.

Note: broader pre-computed/materialised DB work is planned for scoring and ownership — platform display will benefit later but we don't block on it here.

### 8. Tests

- `NodeData.PlatformCaption()` — Windows returns kernel caption, Linux returns lsb
- Migration up/down
- Upsert with caption, scan returns it
- ResolveInfo end-to-end with real caption data (already has test for this)
- Integration: collector builds params with caption populated

## Order of Implementation

1. Migration (standalone, no code deps)
2. NodeData accessor + tests
3. InsertNodeSnapshotParams + NodeSnapshot struct + upsert/scan + tests
4. Collector extraction
5. API surface updates (dashboard, filters, exports, diagnostic)
6. Integration test with caption data flowing through

## Acceptance Criteria

- Migration adds/removes column cleanly
- Collection populates `platform_caption` for Windows and Debian-family nodes
- `ResolveInfo` receives caption at all call sites
- Windows `10.0.17763` nodes with caption "Microsoft Windows Server 2019 Standard" display as "Windows Server 2019 Standard" (not the ambiguous mapping)
- Existing nodes without caption continue to work (fallback to tier 3/4)
- All existing tests pass
- New tests cover caption extraction, storage, and resolution

## Notes

- The Ohai attribute path for Windows caption is `kernel.os_info.caption` (nested object). In partial search, this becomes `["kernel", "os_info", "caption"]`.
- On RHEL, `lsb.description` is usually empty unless `redhat-lsb-core` is installed. The resolver handles empty caption gracefully (falls through to tier 3).
- Caption is nullable — existing rows remain NULL until next collection run. No backfill needed.
