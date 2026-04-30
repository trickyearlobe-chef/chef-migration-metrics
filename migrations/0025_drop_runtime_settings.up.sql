-- Migration 0025: Drop the legacy runtime_settings table.
--
-- All data was migrated to config_store (key: analysis_tools) by the
-- configstore.MigrateFromLegacy() startup routine introduced alongside
-- migration 0011. The table is no longer written by any code path.
--
-- The configstore.MigrateFromLegacy() routine handles the case where this
-- table no longer exists by catching SQLSTATE 42P01 and treating it as
-- "no rows to migrate".

DROP TABLE IF EXISTS runtime_settings;
