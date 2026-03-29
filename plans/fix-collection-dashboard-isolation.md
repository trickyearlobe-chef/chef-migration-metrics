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

### 2. Rewrite ownership-filtered trend to use metric snapshots

- **File:** `internal/webapi/handle_dashboard.go` — `handleDashboardVersionDistributionTrend`
- Both filtered and unfiltered paths read from `metric_snapshots`
- Ownership-filtered path: parse `nodes` array, apply include/exclude, re-aggregate
- Handle missing `nodes` field gracefully (skip snapshot for ownership trend)
- Write tests first in `internal/webapi/handle_dashboard_test.go`

### 3. Add mid-collection guard to summary endpoints

- **File:** `internal/webapi/handle_dashboard.go` — `handleDashboardVersionDistribution` and `handleDashboardVersionDistributionWithOwnerFilter`
- Check if any org has a running collection; if so, serve from latest completed `metric_snapshots` instead of live `node_snapshots`
- Need a `GetLatestCollectionRunByOrganisation` or similar query (check if exists)
- Write tests first

### 4. Update existing tests

- Ensure all existing trend and summary tests pass with enriched payload
- Add backward-compat test: old metric_snapshots rows without `nodes` field
- Add mid-collection scenario test

### 5. Update todo-tech-debt.md

- Note that `CountChefVersionsByCollectionRun*` are no longer used by trend handlers (but kept for other callers)
- Note readiness trend enrichment as a follow-up item

## Acceptance Criteria

- [ ] Ownership-filtered version-distribution trend returns stable counts (no sawtooth)
- [ ] Unfiltered trend behaviour unchanged
- [ ] Summary numbers match last completed collection during active collection
- [ ] Old metric_snapshots rows without `nodes` handled gracefully
- [ ] >50K node orgs degrade gracefully (log warning, skip ownership trend)
- [ ] All existing tests pass
- [ ] New tests cover all changed paths