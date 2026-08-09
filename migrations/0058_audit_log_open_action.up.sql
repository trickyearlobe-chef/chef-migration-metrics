-- The ownership audit log stops constraining what kind of event it will accept.
--
-- The action column listed six ownership actions in a CHECK constraint, so any
-- new kind of event needed a schema migration before it could be written. That
-- is the wrong shape for an audit log: its job is to record what happened,
-- including things nobody anticipated when the column was defined.
--
-- It was also unsafe. A rejected write is only logged as a warning by the
-- caller, so an action missing from the list produced an action that looked
-- audited and was not. Now a mislabelled event is recorded with its label —
-- visible, filterable and correctable — rather than discarded.
--
-- Nothing branches on the value: it is written as a literal and filtered on as
-- a user-supplied string. cookstyle_audit_log, added later against the same
-- pattern, never constrained its own action column.
--
-- Full intent: specifications/ownership-identity.md.

ALTER TABLE ownership_audit_log DROP CONSTRAINT ownership_audit_log_action_check;

-- The entity-fields rule stays, but stops depending on the action vocabulary.
-- What it actually protects against is half an entity reference: a key with no
-- type, or a type with no key, neither of which can be looked up. Which actions
-- carry an entity is for the caller to decide.
ALTER TABLE ownership_audit_log DROP CONSTRAINT ownership_audit_log_action_entity;
ALTER TABLE ownership_audit_log ADD CONSTRAINT ownership_audit_log_entity_pair
    CHECK (
        (entity_type IS NULL AND entity_key IS NULL)
        OR (entity_type IS NOT NULL AND entity_key IS NOT NULL)
    );
