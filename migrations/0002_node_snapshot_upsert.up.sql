-- ---------------------------------------------------------------------------
-- 0002_node_snapshot_upsert
-- ---------------------------------------------------------------------------
-- Convert node_snapshots from append-only (one row per node per collection
-- run) to current-state (one row per node per organisation). This
-- dramatically reduces table growth — the table size is now proportional
-- to fleet size rather than fleet size × collection runs.
--
-- Dependent tables (cookbook_node_usage, node_readiness) are cleaned up
-- via ON DELETE CASCADE when duplicate snapshot rows are removed.
-- ---------------------------------------------------------------------------

-- Step 1: Remove duplicate snapshots, keeping only the most recent per
-- (organisation_id, node_name). This must happen before adding the UNIQUE
-- constraint.
DELETE FROM node_snapshots ns
WHERE ns.id NOT IN (
    SELECT DISTINCT ON (organisation_id, node_name) id
    FROM node_snapshots
    ORDER BY organisation_id, node_name, collected_at DESC
);

-- Step 2: Add the unique constraint. With duplicates removed above, this
-- will succeed.
ALTER TABLE node_snapshots
    ADD CONSTRAINT uq_node_snapshots_org_node UNIQUE (organisation_id, node_name);
