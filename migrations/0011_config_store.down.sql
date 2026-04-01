-- Migration 0011 (down): Drop config_store table
--
-- WARNING: This destroys all encrypted configuration and credential data
-- stored in config_store. Ensure credentials and runtime_settings tables
-- have been restored before running this rollback.

DROP INDEX IF EXISTS idx_config_store_key_prefix;
DROP TABLE IF EXISTS config_store;
