-- Migration 0064: a saved ownership import can name its own source and run on
-- a schedule.
--
-- WHY: an import that runs unattended has to know what to read. Until now a
-- saved mapping held only the shape of the source (0056) — the connection, the
-- query and the row filter lived in the browser form and died with the page.
-- So the mapping could be replayed only by a person sitting in front of it,
-- re-choosing the credential and re-typing the query, which is precisely the
-- work a schedule exists to remove.
--
-- The schedule belongs to the saved import rather than to a global setting
-- because the query does. One estate has several systems of record, and a
-- single "ownership sync time" would force them to share a query or a cadence.
--
-- The connection is stored as a CREDENTIAL NAME, never as a connection string.
-- The rule that a DSN is only ever resolved from the credential store holds for
-- a scheduled run exactly as it does for an interactive one — otherwise this
-- table becomes the one place in the product where a password sits in plain
-- text in the database.
--
-- source_kind was CHECK-constrained to 'csv' alone (0056), deliberately, so
-- that widening it would be an explicit act. This is that act.

ALTER TABLE ownership_import_mappings
    DROP CONSTRAINT chk_ownership_import_mapping_source_kind;

ALTER TABLE ownership_import_mappings
    ADD CONSTRAINT chk_ownership_import_mapping_source_kind
    CHECK (source_kind IN ('csv', 'database'));

ALTER TABLE ownership_import_mappings
    -- Where to read from. Empty for a file import, which has no stored source:
    -- somebody has to bring the file, so it can never be scheduled.
    ADD COLUMN db_driver     TEXT NOT NULL DEFAULT '',
    ADD COLUMN db_credential TEXT NOT NULL DEFAULT '',
    ADD COLUMN db_query      TEXT NOT NULL DEFAULT '',

    -- The row filter is part of the import, not a convenience of the screen. A
    -- source table holding several kinds of asset is imported once per kind,
    -- and an unattended run that dropped the filter would import every kind
    -- under whichever entity type the mapping names.
    ADD COLUMN filter_column TEXT NOT NULL DEFAULT '',
    ADD COLUMN filter_value  TEXT NOT NULL DEFAULT '',

    -- Whether a person the import has never seen is created or the row is
    -- rejected. Stored because it changes what an unattended run does to the
    -- owner catalogue, and the answer must not depend on a default that moves.
    ADD COLUMN create_owners BOOLEAN NOT NULL DEFAULT TRUE,

    -- Standard 5-field cron. Empty means "not scheduled", which is the state
    -- every existing row is in and the state a saved mapping keeps unless
    -- somebody asks for otherwise.
    ADD COLUMN schedule         TEXT    NOT NULL DEFAULT '',
    ADD COLUMN schedule_enabled BOOLEAN NOT NULL DEFAULT FALSE,

    -- What the last unattended run did. An import nobody watched has to leave
    -- a mark, or "it is scheduled" and "it is working" become the same claim.
    -- The per-row detail is in the ownership audit log; this is the summary the
    -- list needs so a silently failing import is visible without reading it.
    ADD COLUMN last_run_at      TIMESTAMPTZ,
    ADD COLUMN last_run_status  TEXT NOT NULL DEFAULT '',
    ADD COLUMN last_run_detail  TEXT NOT NULL DEFAULT '';

-- A schedule cannot run without something to run against. This is enforced
-- here rather than only in the API because an unrunnable enabled schedule is
-- indistinguishable from a broken scheduler when somebody comes to ask why
-- nothing happened.
ALTER TABLE ownership_import_mappings
    ADD CONSTRAINT chk_ownership_import_mapping_schedulable
    CHECK (
        NOT schedule_enabled
        OR (source_kind = 'database' AND db_credential <> '' AND db_query <> '' AND schedule <> '')
    );

-- The scheduler asks one question on every tick: which imports are due? Kept
-- partial so it indexes only the handful of rows that are actually scheduled.
CREATE INDEX idx_ownership_import_mappings_scheduled
    ON ownership_import_mappings (id)
    WHERE schedule_enabled;
