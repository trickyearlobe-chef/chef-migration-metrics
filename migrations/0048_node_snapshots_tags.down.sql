-- =============================================================================
-- Migration 0048: Node Snapshot Tags — Rollback
-- =============================================================================
-- Drops the tags column and its GIN index added by 0048_node_snapshots_tags.up.sql.
-- =============================================================================

DROP INDEX IF EXISTS idx_node_snapshots_tags_gin;

ALTER TABLE node_snapshots
    DROP COLUMN IF EXISTS tags;
