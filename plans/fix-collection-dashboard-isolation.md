# Plan: Fix Collection–Dashboard Isolation

## Goal

Fix three related bugs where dashboard reads are disrupted by in-progress collection writes: sawtooth trend graphs (#1), summary/detail mismatch (#2), and context timeout amplification (#3).

## Specs to Read

- `.claude/specifications/collection-dashboard-isolation.md` (primary)
- `.claude/specifications/datastore.md` (metric_snapshots schema)
- `.claude/specifications/web-api.md` (dashboard endpoint contracts)

## Steps

### 1. Enrich metric snapshot payload in collector

- **File:** `internal/collector/collector.go` — `recordMetricSnapshots`
- Add `nodes` array (name + version) to `chef_version_distribution` JSONB payload
- Add 50K node cap with warning log
- Write tests first in `internal/collector/collector_test.go`
- ✅ Done — extracted `buildVersionDistributionPayload` pure function, 5 tests

### 2. Rewrite ownership-filtered trend to use metric snapshots

- **File:** `internal/webapi/handle_dashboard.go` — `handleDashboardVersionDistributionTrend`
- Both filtered and unfiltered paths read from `metric_snapshots`
- Ownership-filtered path: parse `nodes` array, apply include/exclude, re-aggregate
- Handle missing `nodes` field gracefully (skip snapshot for ownership trend)
- Write tests first in `internal/webapi/handle_dashboard_test.go`
- ✅ Done — 4 new tests (owner include, unowned exclude, nodes_omitted, backward compat)

### 3. Add mid-collection guard to summary endpoints

- **File:** `internal/webapi/handle_dashboard.go` — `handleDashboardVersionDistribution` and `handleDashboardVersionDistributionWithOwnerFilter`
- Check if any org has a running collection; if so, serve from latest completed `metric_snapshots` instead of live `node_snapshots`
- Uses existing `GetLatestCollectionRun` query
- Write tests first
- ✅ Done — 4 new tests (running guard, completed passthrough, no-snapshot fallback, ownership+running)

### 4. Update existing tests

- Ensure all existing trend and summary tests pass with enriched payload
- Add backward-compat test: old metric_snapshots rows without `nodes` field
- Add mid-collection scenario test
- ✅ Done — all existing tests pass, all new tests pass, full `go test ./...` clean

### 5. Update todo-tech-debt.md

- Note that `CountChefVersionsByCollectionRun*` are no longer used by trend handlers (but kept for other callers)
- Note readiness trend enrichment as a follow-up item
- ✅ Done — added B4a (readiness trend enrichment) and B4b (dead code removal)

## Acceptance Criteria

- [x] Ownership-filtered version-distribution trend returns stable counts (no sawtooth)
- [x] Unfiltered trend behaviour unchanged
- [x] Summary numbers match last completed collection during active collection
- [x] Old metric_snapshots rows without `nodes` handled gracefully
- [x] >50K node orgs degrade gracefully (log warning, skip ownership trend)
- [x] All existing tests pass
- [x] New tests cover all changed paths