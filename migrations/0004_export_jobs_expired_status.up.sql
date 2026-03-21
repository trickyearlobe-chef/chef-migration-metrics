-- ---------------------------------------------------------------------------
-- 0004_export_jobs_expired_status
-- ---------------------------------------------------------------------------
-- The export cleanup worker sets status = 'expired' on completed export jobs
-- after their files are removed from disk, but the CHECK constraint on the
-- status column does not include 'expired'. This causes every
-- UpdateExportJobExpired call to fail with a constraint violation, leaving
-- completed jobs in place forever.
-- ---------------------------------------------------------------------------

ALTER TABLE export_jobs DROP CONSTRAINT chk_export_jobs_status;

ALTER TABLE export_jobs ADD CONSTRAINT chk_export_jobs_status CHECK (
    status IN ('pending', 'processing', 'completed', 'failed', 'expired')
);
