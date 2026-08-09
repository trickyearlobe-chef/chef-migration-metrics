-- Run events view: indexes for the run-centric top-level list over converge_runs
-- (see journeys/run-history.md). The existing
-- idx_converge_runs_org_node_time serves the per-node Runs tab (org + node
-- lookup); the global Run events list filters/sorts differently:
--
--   * default list  — recency across ALL orgs: ORDER BY end_time DESC
--   * default view  — failures first:           WHERE status='failure' ORDER BY end_time DESC
--
-- Both are retention-bounded (short window), but without these indexes each page
-- is a scan over the live day partitions. Defined on the partitioned parent so
-- Postgres propagates them to every existing and future day partition.
--
-- Deliberately NOT indexing chef_version alone here: that path (the CC19
-- target-version rollup) is a separate, still-open design — its index lands with
-- that build so we don't carry an unused index on the append-heavy hot path.
CREATE INDEX idx_converge_runs_end_time
    ON converge_runs (end_time DESC);

CREATE INDEX idx_converge_runs_status_end_time
    ON converge_runs (status, end_time DESC);
