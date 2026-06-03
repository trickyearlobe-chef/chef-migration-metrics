# Plan: Parallel Deployment Tracking

## Goal

Track per-node deployment state (current only / staged / activated) and nightly speculative converge results. Enable the node list to show which nodes are "ready to activate".

## Specs to Read

- `specifications/parallel-deployment-tracking.md`

## Chunks

Each chunk is a self-contained unit of work suitable for one session/thread.

### Chunk 1 — Schema & Collection (backend only)

**Files**: `internal/chefapi/client.go`, `migrations/`, `internal/datastore/node_snapshots.go`, `internal/collector/`

1. Add `"chef_migration": {"chef_migration"}` to `NodeSearchAttributes()`
2. Schema migration: add 7 nullable columns to `node_snapshots`
3. Extend `NodeSnapshot` Go struct with new fields
4. Parse `chef_migration` map from search result into snapshot fields
5. Persist fields in upsert SQL
6. Tests: parsing (nil/partial/full data), DB round-trip

**Acceptance**: Go tests pass, new columns populated when `chef_migration` attributes present, nil-safe when absent.

### Chunk 2 — Node List API & Filters (backend only)

**Files**: `internal/webapi/handle_nodes.go`, `internal/datastore/node_snapshot_filter.go`

1. Add `migration_state`, `target_converge_status`, `ready_to_activate` to node list response
2. Map raw `migration_state` to UI labels in response
3. Add filter support: `migration_state`, `target_converge_status`, `ready_to_activate`
4. Tests: API response shape, filter logic

**Acceptance**: Node list API returns new fields, filters work, existing tests unbroken.

### Chunk 3 — Node List UI (frontend only) ✅ DONE

**Files**: `frontend/src/pages/NodesPage.tsx`, `frontend/src/types/nodes.ts`, `frontend/src/components/StatusBadge.tsx`

1. ✅ Add TypeScript types for new fields
2. ✅ Deployment state badge column (Current only / Staged / Activated)
3. ✅ Speculative converge status badge column (success / fail / —)
4. ✅ "Ready to Activate" row highlight
5. ✅ Filter controls for deployment state and converge status
6. ✅ Tests: component rendering, badge variants (10 new tests)

**Acceptance**: Node list shows badges, highlights ready nodes, filters work.

### Chunk 4 — Node Detail Panel (frontend + small backend) ✅ DONE

**Files**: `frontend/src/pages/NodeDetailPage.tsx`, `internal/webapi/handle_nodes.go`

1. ✅ Expose migration fields in node detail API (already on snapshot — confirmed serialisation)
2. ✅ "Deployment State" panel: state label, active version, staged version
3. ✅ Speculative converge section: status, version tested, timestamp
4. ✅ "Ready to Activate" callout when criteria met
5. ✅ Graceful nil handling (panel hidden when no migration data)
6. ✅ Tests: panel rendering (10 new tests)

**Acceptance**: Node detail shows deployment info when available, hidden when not.

### Chunk 5 — Dashboard Trend (backend + frontend) ✅ DONE

**Files**: `internal/webapi/handle_dashboard.go`, `internal/datastore/`, `frontend/src/pages/DashboardPage.tsx`

1. ✅ New API endpoint: deployment progress aggregation (count by state per collection run)
2. ✅ Query: nodes with target version present vs speculative converge passing, over time
3. ✅ Dashboard chart component: two trend lines
4. ✅ Tests: aggregation query, chart rendering (9 new tests total)

**Acceptance**: Dashboard shows deployment progress trend with two series.

## Complexity

- 5 chunks, each fits comfortably in one session
- Chunks 1→2→3 are sequential (each depends on prior)
- Chunk 4 can run after Chunk 1 (only needs schema)
- Chunk 5 can run after Chunk 2 (needs API + aggregation)

## Notes

- UI labels only — never expose raw `hab_dormant` etc. to frontend
- All columns nullable — graceful when migration cookbook not deployed
- Dynamic config not needed here (no user-configurable thresholds)
