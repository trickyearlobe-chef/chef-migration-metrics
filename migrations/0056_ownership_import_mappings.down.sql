-- Migration 0056 (down): Drop the saved ownership import mappings.
--
-- Nothing references this table, so there is no cascade to consider. Saved
-- mappings are templates, not a record of what was imported, so dropping them
-- loses convenience and no history.

DROP TABLE IF EXISTS ownership_import_mappings;
