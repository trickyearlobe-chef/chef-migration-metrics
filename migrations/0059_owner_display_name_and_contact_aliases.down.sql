-- Only the aliases this migration seeded are removed; one a person recorded by
-- hand against the same address is left alone, which is why the source is
-- stamped rather than inferred from the value.
DELETE FROM owner_aliases WHERE source = 'contact_email';

-- Any candidate pair found by display name goes with the ability to record it.
DELETE FROM owner_duplicate_candidates WHERE matched_on = 'display_name';

ALTER TABLE owner_duplicate_candidates DROP CONSTRAINT chk_owner_duplicate_matched_on;
ALTER TABLE owner_duplicate_candidates ADD CONSTRAINT chk_owner_duplicate_matched_on
    CHECK (matched_on IN ('name', 'alias'));

DROP INDEX IF EXISTS idx_owners_display_name_gist_trgm;
