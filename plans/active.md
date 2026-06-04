# Plan: Deployment Dashboard (Per-Version)

## Goal

Add a dedicated "Deployment" tab to the dashboard showing per-version parallel deployment progress. Supports overlapping minor version rollouts without conflating with TK/readiness single-target analysis.

## Specs to Read

- `specifications/deployment-dashboard.md`
- `specifications/parallel-deployment-tracking.md` (background — already implemented)

## Chunks

### Chunk 1 — Collector: per-version deployment breakdown

**Files**: `internal/collector/node_metrics_snapshot.go`, `internal/collector/node_metrics_snapshot_test.go`

1. Add `deploymentVersionBreakdown` struct (Staged, Activated, ConvergePassing, ConvergeFailing)
2. Add `ByVersion map[string]deploymentVersionBreakdown` to `deploymentBreakdown`
3. Count per-version stats in `buildNodeMetricsPayload` loop
4. Tests: multiple versions, empty, single version

**Acceptance**: node_metrics JSON payload includes `deployment.by_version` map. Backward-compatible (aggregate fields remain).

### Chunk 2 — Backend: deployment status + trend endpoints

**Files**: `internal/webapi/handle_dashboard_deployment.go`, `internal/datastore/node_snapshot_filter.go`, `internal/webapi/store.go`

1. New `GET /api/v1/dashboard/deployment/status` — live GROUP BY query on node_snapshots
2. Add `CountNodesByDeploymentVersion` to datastore interface
3. Update `GET /api/v1/dashboard/deployment/trend` to return per-version data
4. Tests: both endpoints (happy path, empty, multi-version)

**Acceptance**: Both endpoints return per-version data. Status is live, trend is historical.

### Chunk 3 — Frontend: Deployment tab + current status card

**Files**: `frontend/src/pages/dashboard/index.tsx`, `frontend/src/pages/dashboard/DeploymentCards.tsx`, `frontend/src/types/dashboard.ts`, `frontend/src/api/dashboard.ts`

1. Add "Deployment" as 3rd dashboard tab
2. Move `DeploymentTrendCard` from Trends tab to Deployment tab
3. Add `DeploymentStatusCard` — per-version battery bars or grouped bars
4. TypeScript types and API function for deployment status
5. Tests: card rendering, tab switching

**Acceptance**: Deployment tab shows current per-version status + trend chart. Trends tab no longer has deployment.

## Dependencies

- Chunk 1 → Chunk 2 → Chunk 3 (sequential — each builds on prior)
- No DB migration required
- No changes to TK/readiness/complexity analysis

## Schema Impact

**None.** All changes are:
- JSON payload shape in metric_snapshots (schemaless)
- Live aggregation query on existing columns
- Frontend UI

## Notes

- Keep aggregate fields in deployment breakdown (backward-compatible)
- Organisation filter should work on deployment tab (reuse global filter)
- Battery bar vs grouped bar — prototype one, iterate with customer feedback
- TK/CookStyle stays on single static target — no cross-contamination
