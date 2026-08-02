-- A commitment holder is an owner or a ticket. There is no separate "user".
--
-- Decided by the product owner, 2026-08-02: everything person-shaped is an
-- owner, and other identities — including one sourced from SAML — reach an
-- owner through an alias. Offering "user" alongside "owner" created a second
-- identity space for the same thing, which is the conflation
-- specifications/ownership-identity.md already records as the alias model's
-- central fault. One hub, reached by many identifiers.
--
-- The rows are converted before the constraint is tightened. A blind tighten
-- would abort on any existing row, roll back the migration, and stop the
-- service starting — the failure mode this project has already recorded once.
-- A converted row keeps its reference exactly as it was; the reference was
-- free text under both kinds, so nothing is lost and nothing is invented.
UPDATE failure_register_entries
SET holder_type = 'owner'
WHERE holder_type = 'user';

ALTER TABLE failure_register_entries
    DROP CONSTRAINT failure_register_entries_holder_type_check;

ALTER TABLE failure_register_entries
    ADD CONSTRAINT failure_register_entries_holder_type_check
        CHECK (holder_type IN ('owner', 'ticket'));
