# Data Layer Revamp — ToDo

Plan: `plans/data-layer-revamp.md`

Phases 0–3 and Phase 7 are complete. Phase 4 (node filter correctness) is complete.
Open: Phase 5 (staleness/freshness), Phase 6 (export filter parity), Phase 8 (performance).

---

## Phase 5 — Staleness & Freshness Filters (Bugs 2, 9)

### Bug 2: Trend graphs don't react to staleness filters

- [ ] 5a. Choose approach: re-aggregate at query time (filter metric_snapshot data by staleness tier at each timestamp) vs. separate stale/fresh series stored at collection time
- [ ] 5b. Implement chosen approach for readiness trend handler — accept `?stale=` param and return filtered series
- [ ] 5c. Implement for version-distribution trend handler — same pattern
- [ ] 5d. Frontend: pass current staleness filter to trend API calls

### Bug 9: Cookbook "fresh" still shows inactive cookbooks

- [ ] 5e. Define "fresh cookbook" = referenced by at least one node with `is_stale = false`
- [ ] 5f. Add `used_by_fresh_nodes` filter param to cookbook endpoint (JOIN through `node_snapshots.cookbooks` JSONB)
- [ ] 5g. Wire filter into cookbook list frontend filter controls

---

## Phase 6 — Export Filter Parity (Bug 8)

> Bug 8 is marked fixed in the plan (NodesPage passes filters to ExportButton) but the items below were left unchecked. Verify and close or complete.

- [ ] 6a. Verify frontend wires current filter state into export download URL params for all export types
- [ ] 6b. Verify backend export endpoints accept and apply same filters as list endpoints

---

## Phase 8 — Performance & Indexing

- [ ] 8a. EXPLAIN ANALYZE on all list endpoints at realistic scale (120k+ nodes)
- [ ] 8b. Add composite indexes for common filter+sort patterns identified in 8a
- [ ] 8c. Evaluate connection pooling / prepared statement caching

---

### Re-specify Data Exports

The current `specifications/data-export.md` defines three mechanisms (webhook push, Elasticsearch NDJSON, direct Logstash) that lack a coherent story. Before implementing, rewrite the spec to answer:

- [ ] Decide what export formats are actually needed: CSV for ad-hoc analysis, NDJSON for Logstash/Kibana, webhook push, or some combination
- [ ] Decide whether Elasticsearch export is file-based (Logstash reads) or direct API push — current spec tries to cover both
- [ ] Define what data is exported and in what shape (which entities, which fields, one-time snapshot vs. incremental)
- [ ] Clarify the UI story: is the Exports page for one-off human downloads (CSV/JSON), background pipeline export (Elasticsearch), or both?
- [ ] Update or replace `specifications/data-export.md` with the agreed design before any implementation
