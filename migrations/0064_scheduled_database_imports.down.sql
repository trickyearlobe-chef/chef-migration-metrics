-- Reverse of 0064.
--
-- NOT a true inverse: any saved import whose source_kind is 'database' has to
-- go, because the narrowed constraint cannot hold it and there is nowhere to
-- put a connection once the columns are dropped. Their schedules and their
-- run history go with them.

DROP INDEX IF EXISTS idx_ownership_import_mappings_scheduled;

ALTER TABLE ownership_import_mappings
    DROP CONSTRAINT IF EXISTS chk_ownership_import_mapping_schedulable;

DELETE FROM ownership_import_mappings WHERE source_kind = 'database';

ALTER TABLE ownership_import_mappings
    DROP COLUMN IF EXISTS db_driver,
    DROP COLUMN IF EXISTS db_credential,
    DROP COLUMN IF EXISTS db_query,
    DROP COLUMN IF EXISTS filter_column,
    DROP COLUMN IF EXISTS filter_value,
    DROP COLUMN IF EXISTS create_owners,
    DROP COLUMN IF EXISTS schedule,
    DROP COLUMN IF EXISTS schedule_enabled,
    DROP COLUMN IF EXISTS last_run_at,
    DROP COLUMN IF EXISTS last_run_status,
    DROP COLUMN IF EXISTS last_run_detail;

ALTER TABLE ownership_import_mappings
    DROP CONSTRAINT IF EXISTS chk_ownership_import_mapping_source_kind;

ALTER TABLE ownership_import_mappings
    ADD CONSTRAINT chk_ownership_import_mapping_source_kind
    CHECK (source_kind IN ('csv'));
