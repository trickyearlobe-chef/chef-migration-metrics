-- Copyright 2025 Chef Migration Metrics Authors
-- SPDX-License-Identifier: Apache-2.0

-- A saved import names a connection, not a credential holding a whole
-- connection string.
--
-- See journeys/ownership-connection.md. The connection is configuration now —
-- the address, the database and the account readable, only the password held
-- as a credential — and the connection itself says which database reads it. So
-- one column replaces two: db_credential, which named a secret that was the
-- entire connection, and db_driver, which asked a second time what the
-- connection already answers.
--
-- Nothing is carried across. There were no saved database imports anywhere
-- when this ran, and a credential holding a whole connection cannot be split
-- into a connection and a password without decrypting it and guessing which
-- part was the secret — which is the guessing this whole change exists to end.
-- A connection that existed as a credential is set up again, once, by hand.

ALTER TABLE ownership_import_mappings
    DROP CONSTRAINT IF EXISTS chk_ownership_import_mapping_schedulable;

ALTER TABLE ownership_import_mappings
    ADD COLUMN IF NOT EXISTS db_connection TEXT NOT NULL DEFAULT '';

ALTER TABLE ownership_import_mappings
    DROP COLUMN IF EXISTS db_credential,
    DROP COLUMN IF EXISTS db_driver;

-- A schedule still cannot run without something to run against.
ALTER TABLE ownership_import_mappings
    ADD CONSTRAINT chk_ownership_import_mapping_schedulable
    CHECK (
        NOT schedule_enabled
        OR (source_kind = 'database' AND db_connection <> '' AND db_query <> '' AND schedule <> '')
    );
