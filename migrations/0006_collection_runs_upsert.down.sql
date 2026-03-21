-- ---------------------------------------------------------------------------
-- 0006_collection_runs_upsert (rollback)
-- ---------------------------------------------------------------------------
-- Remove the unique constraint, reverting to append-only behaviour.
-- Note: this does NOT restore previously deleted duplicate rows.
-- ---------------------------------------------------------------------------

ALTER TABLE collection_runs
    DROP CONSTRAINT IF EXISTS uq_collection_runs_org;
