-- ---------------------------------------------------------------------------
-- 0003_cookbook_usage_upsert
-- ---------------------------------------------------------------------------
-- Convert cookbook_usage_analysis from append-only to current-state
-- (one row per organisation). Detail rows cascade-delete when the
-- parent analysis row is replaced.
-- ---------------------------------------------------------------------------

-- Step 1: Remove old analysis rows, keeping only the most recent per org.
DELETE FROM cookbook_usage_analysis cua
WHERE cua.id NOT IN (
    SELECT DISTINCT ON (organisation_id) id
    FROM cookbook_usage_analysis
    ORDER BY organisation_id, analysed_at DESC
);

-- Step 2: Add unique constraint on organisation_id.
ALTER TABLE cookbook_usage_analysis
    ADD CONSTRAINT uq_cookbook_usage_analysis_org UNIQUE (organisation_id);
