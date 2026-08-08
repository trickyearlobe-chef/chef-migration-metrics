-- Migration 0065: keep the rows an import could not use.
--
-- WHY: the rejected rows are the most direct statement of source data quality
-- there is — this row names a person who is not in the staff table, that one
-- has no asset name at all. Until now they were computed into the match report,
-- shown on screen once, and discarded when the page closed. So the one thing
-- worth sending back to whoever maintains the source system was the one thing
-- that could not be exported.
--
-- **Latest run only, per import.** A rejection is a statement about the source
-- as it stands, not a history of it: once a row is fixed at source it stops
-- being a problem, and a table that accumulated every run would report fixed
-- rows forever alongside real ones. Each commit replaces the set for its
-- import, so what is here is always "what is wrong now".
--
-- import_label rather than only a foreign key, because an interactive import
-- from an uploaded file has no saved import to point at — and those are exactly
-- the runs somebody makes while judging whether a source is any good.

CREATE TABLE ownership_import_rejections (
    id            BIGSERIAL PRIMARY KEY,

    -- The saved import this came from, when there is one. ON DELETE CASCADE so
    -- deleting an import takes its rejections with it rather than leaving
    -- findings attributed to something that no longer exists.
    mapping_id    BIGINT REFERENCES ownership_import_mappings(id) ON DELETE CASCADE,

    -- What to call the run in the report. The saved import's name, or the file
    -- somebody uploaded.
    import_label  TEXT NOT NULL,

    run_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Which row of the source, so somebody can go and look at it.
    source_row    INTEGER NOT NULL,

    -- Why it could not be used, as one of ownershipimport's reason codes.
    reason        TEXT NOT NULL,

    -- What the row said, after mapping. Enough to identify the record at
    -- source without storing the whole row: a source table can be wide, and
    -- copying every column of it here would put columns nobody asked for —
    -- including ones that may be personal data — into a second place.
    owner_raw     TEXT NOT NULL DEFAULT '',
    entity_type   TEXT NOT NULL DEFAULT '',
    entity_key    TEXT NOT NULL DEFAULT ''
);

-- The two questions asked of this table: replace everything for one import, and
-- read it all back in source order for the report.
CREATE INDEX idx_ownership_import_rejections_label
    ON ownership_import_rejections (import_label);
