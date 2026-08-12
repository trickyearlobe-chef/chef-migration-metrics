-- Reverses 0068.
--
-- NOT a true inverse: dropping the columns loses which entries a tool wrote,
-- and an older binary reading the register afterwards presents those as their
-- owner's own judgement. That is the exact confusion 0068 exists to prevent, so
-- take a dump before rolling back and do not run this to "tidy up".

ALTER TABLE failure_register_entries
    DROP CONSTRAINT IF EXISTS chk_failure_register_entries_raised_origin;

ALTER TABLE failure_register_entries
    DROP COLUMN IF EXISTS raised_origin,
    DROP COLUMN IF EXISTS raised_origin_name;
