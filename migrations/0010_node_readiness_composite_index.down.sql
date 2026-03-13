-- ---------------------------------------------------------------------------
-- 0010: Remove composite index for latestReadinessPerNode
-- ---------------------------------------------------------------------------

DROP INDEX IF EXISTS idx_node_readiness_latest;
