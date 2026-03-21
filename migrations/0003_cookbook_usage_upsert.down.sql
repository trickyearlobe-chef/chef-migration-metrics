-- ---------------------------------------------------------------------------
-- 0003_cookbook_usage_upsert (rollback)
-- ---------------------------------------------------------------------------

ALTER TABLE cookbook_usage_analysis
    DROP CONSTRAINT IF EXISTS uq_cookbook_usage_analysis_org;
