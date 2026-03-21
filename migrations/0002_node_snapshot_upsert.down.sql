-- ---------------------------------------------------------------------------
-- 0002_node_snapshot_upsert (rollback)
-- ---------------------------------------------------------------------------
-- Remove the unique constraint, reverting to append-only behaviour.
-- Note: this does NOT restore previously deleted duplicate rows.
-- ---------------------------------------------------------------------------

ALTER TABLE node_snapshots
    DROP CONSTRAINT IF EXISTS uq_node_snapshots_org_node;
