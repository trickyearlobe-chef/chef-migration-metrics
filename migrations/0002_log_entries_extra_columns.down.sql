-- ---------------------------------------------------------------------------
-- 0002 (down): Remove notification_channel, export_job_id, and tls_domain
--              columns from log_entries table.
-- ---------------------------------------------------------------------------

ALTER TABLE log_entries DROP COLUMN IF EXISTS tls_domain;
ALTER TABLE log_entries DROP COLUMN IF EXISTS export_job_id;
ALTER TABLE log_entries DROP COLUMN IF EXISTS notification_channel;
