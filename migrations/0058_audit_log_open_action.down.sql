-- Reverting narrows the audit log back to the original six ownership actions.
-- Any entry recorded under another action — a merge, or anything added since —
-- violates the constraint being restored and is deleted: there is no equivalent
-- for it in the older vocabulary.

DELETE FROM ownership_audit_log
WHERE action NOT IN (
    'owner_created', 'owner_updated', 'owner_deleted',
    'assignment_created', 'assignment_deleted', 'assignment_reassigned'
);

ALTER TABLE ownership_audit_log DROP CONSTRAINT ownership_audit_log_entity_pair;

ALTER TABLE ownership_audit_log ADD CONSTRAINT ownership_audit_log_action_check
    CHECK (action IN (
        'owner_created', 'owner_updated', 'owner_deleted',
        'assignment_created', 'assignment_deleted', 'assignment_reassigned'
    ));

ALTER TABLE ownership_audit_log ADD CONSTRAINT ownership_audit_log_action_entity
    CHECK (
        action IN ('owner_created', 'owner_updated', 'owner_deleted')
        OR (entity_type IS NOT NULL AND entity_key IS NOT NULL)
    );
