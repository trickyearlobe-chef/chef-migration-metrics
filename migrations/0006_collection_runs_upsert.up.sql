-- ---------------------------------------------------------------------------
-- 0006_collection_runs_upsert
-- ---------------------------------------------------------------------------
-- Convert collection_runs from append-only (one row per org per collection
-- cycle) to current-state (one row per organisation). This mirrors the
-- approach taken for node_snapshots in migration 0002.
--
-- The table tracks the current/latest collection run state for each org.
-- Historical trend data is stored in metric_snapshots (purpose-built for
-- trends), so keeping old collection_runs rows serves no purpose.
--
-- Dependent tables:
--   node_snapshots        → ON DELETE CASCADE (already upserted per org)
--   cookbook_usage_analysis → ON DELETE CASCADE (already one per org)
--   metric_snapshots      → ON DELETE SET NULL (preserves trend data)
--   log_entries           → ON DELETE SET NULL (preserves log history)
-- ---------------------------------------------------------------------------

-- Step 1: Remove old collection runs, keeping only the most recent per org.
-- This deduplicates so the UNIQUE constraint can be added.
DELETE FROM collection_runs cr
WHERE cr.id NOT IN (
    SELECT DISTINCT ON (organisation_id) id
    FROM collection_runs
    ORDER BY organisation_id, started_at DESC
);

-- Step 2: Add unique constraint on organisation_id.
ALTER TABLE collection_runs
    ADD CONSTRAINT uq_collection_runs_org UNIQUE (organisation_id);
