-- ---------------------------------------------------------------------------
-- 0010: Add composite index for latestReadinessPerNode performance
-- ---------------------------------------------------------------------------
-- The DISTINCT ON (organisation_id, node_name, target_chef_version)
-- subquery used by CountNodeReadiness and all List*Readiness queries
-- performs a full table scan when PostgreSQL generates a generic plan
-- for parameterized queries. This composite index allows the planner
-- to satisfy the DISTINCT ON + ORDER BY evaluated_at DESC via an
-- index-only scan, reducing query time from minutes to milliseconds.
--
-- The INCLUDE (id) clause makes this a covering index so the subquery
-- can retrieve the row ID without touching the heap (Heap Fetches: 0).
-- ---------------------------------------------------------------------------

CREATE INDEX IF NOT EXISTS idx_node_readiness_latest
    ON node_readiness (organisation_id, node_name, target_chef_version, evaluated_at DESC)
    INCLUDE (id);
