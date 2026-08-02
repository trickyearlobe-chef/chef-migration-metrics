-- Two more ways to recognise that two owners are one person.
--
-- Found against real data: one person committing under two addresses produced
-- two owners with unrelated names, no aliases between them, and the identical
-- display name — and the duplicate scan could not see any of it, because it
-- compared owner names against owner names and alias values against alias
-- values and nothing else.

-- 1. Display name. Two owners under one display name is about the strongest
--    duplicate signal there is, and it is exactly what the committer path
--    produces. GiST supports the nearest-neighbour ordering the scan uses.
CREATE INDEX IF NOT EXISTS idx_owners_display_name_gist_trgm
    ON owners USING gist (display_name gist_trgm_ops)
    WHERE display_name IS NOT NULL;

-- 2. matched_on gains 'display_name'. Unlike an audit action, this one is a
--    real enum — the UI branches on it to decide what to show — so the
--    constraint earns its place and widening it is the right move.
ALTER TABLE owner_duplicate_candidates DROP CONSTRAINT chk_owner_duplicate_matched_on;
ALTER TABLE owner_duplicate_candidates ADD CONSTRAINT chk_owner_duplicate_matched_on
    CHECK (matched_on IN ('name', 'display_name', 'alias'));

-- 3. An owner's contact address becomes an alias.
--
--    It is an identity we already hold and never indexed, so it was invisible
--    to the duplicate scan, to the email-localpart signal, and to an import
--    matching a row on an address. Recorded as 'email' — the contact address
--    of record — and sourced so it can be told apart from one a person typed.
--
--    ON CONFLICT is load-bearing, not defensive: alias uniqueness is global,
--    so two owners sharing a contact address, or an address somebody has
--    already recorded by hand, would otherwise abort the migration and stop
--    the service from starting. The colliding owner keeps whatever it had.
INSERT INTO owner_aliases (owner_name, alias_type, alias_value, source)
SELECT name, 'email', contact_email, 'contact_email'
FROM owners
WHERE contact_email IS NOT NULL AND contact_email <> ''
ON CONFLICT (alias_type, alias_value) DO NOTHING;
