-- ---------------------------------------------------------------------------
-- 0004_export_jobs_expired_status (rollback)
-- ---------------------------------------------------------------------------
-- Revert the CHECK constraint to the original set of allowed statuses.
-- Note: any rows already in 'expired' status will cause this to fail.
-- They must be updated to 'completed' or deleted before rolling back.
-- ---------------------------------------------------------------------------

ALTER TABLE export_jobs DROP CONSTRAINT chk_export_jobs_status;

ALTER TABLE export_jobs ADD CONSTRAINT chk_export_jobs_status CHECK (
    status IN ('pending', 'processing', 'completed', 'failed')
);
