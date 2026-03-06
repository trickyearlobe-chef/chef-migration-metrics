-- ---------------------------------------------------------------------------
-- 0002: Add notification_channel, export_job_id, and tls_domain columns to
--       log_entries table. These columns were specified in the logging
--       component specification but omitted from the initial schema.
-- ---------------------------------------------------------------------------

ALTER TABLE log_entries ADD COLUMN notification_channel TEXT;
ALTER TABLE log_entries ADD COLUMN export_job_id TEXT;
ALTER TABLE log_entries ADD COLUMN tls_domain TEXT;
