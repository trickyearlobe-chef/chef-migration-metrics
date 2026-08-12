-- Migration 0068: record what made an entry, not only who it belongs to.
--
-- WHY: the register exists so a later reader can weigh who said what. raised_by
-- answers whose judgement it is. It does not answer whether a person typed it
-- at a screen or a tool holding that person's credential wrote it — and those
-- read identically today, which means a note an assistant produced would appear
-- under its owner's name, worded like something they decided.
--
-- That is the condition that has to hold before a credential is allowed to
-- write at all, so the two are one change and not two.
--
-- Settled at sign-in, attached by the service, never sent by the caller. A tool
-- can say it is anything, so nothing it says about itself is worth recording;
-- what is worth recording is what somebody made on purpose — a login at a
-- screen, or a credential they created and named.
--
-- Existing rows are 'screen'. Every one of them predates any credential
-- existing, so there is no row this back-fill could be wrong about.

ALTER TABLE failure_register_entries
    ADD COLUMN raised_origin TEXT NOT NULL DEFAULT 'screen',
    -- The credential's name, when one made the entry. Empty for a screen. Its
    -- name rather than its id, because this is read by a person deciding
    -- whether to trust the entry, and an id tells them nothing.
    ADD COLUMN raised_origin_name TEXT NOT NULL DEFAULT '';

ALTER TABLE failure_register_entries
    ADD CONSTRAINT chk_failure_register_entries_raised_origin
        CHECK (raised_origin IN ('screen', 'credential'));
