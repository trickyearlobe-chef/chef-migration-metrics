-- Widening again is safe: every existing row is already a kind the wider
-- constraint accepts. The rows converted on the way up are not converted back,
-- because which of them began as 'user' is not recorded — and an owner is what
-- they are.
ALTER TABLE failure_register_entries
    DROP CONSTRAINT failure_register_entries_holder_type_check;

ALTER TABLE failure_register_entries
    ADD CONSTRAINT failure_register_entries_holder_type_check
        CHECK (holder_type IN ('owner', 'user', 'ticket'));
