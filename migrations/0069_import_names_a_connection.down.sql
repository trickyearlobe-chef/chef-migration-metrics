-- Copyright 2025 Chef Migration Metrics Authors
-- SPDX-License-Identifier: Apache-2.0

-- Puts the two columns back, empty.
--
-- It is not a true inverse and cannot be: what a saved import named is a
-- connection, and the older shape wants the name of a credential holding a
-- whole connection string, which is a different thing that may not exist. A
-- database import restored this way names nothing to connect with and says so
-- when it runs, which is the honest outcome — quietly reconstructing a
-- credential name from a connection name would produce an import that fails
-- with a message about the wrong object.

ALTER TABLE ownership_import_mappings
    DROP CONSTRAINT IF EXISTS chk_ownership_import_mapping_schedulable;

ALTER TABLE ownership_import_mappings
    ADD COLUMN IF NOT EXISTS db_driver     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS db_credential TEXT NOT NULL DEFAULT '';

ALTER TABLE ownership_import_mappings
    DROP COLUMN IF EXISTS db_connection;

ALTER TABLE ownership_import_mappings
    ADD CONSTRAINT chk_ownership_import_mapping_schedulable
    CHECK (
        NOT schedule_enabled
        OR (source_kind = 'database' AND db_credential <> '' AND db_query <> '' AND schedule <> '')
    );
