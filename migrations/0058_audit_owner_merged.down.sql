-- Reverting drops any owner_merged entries: the constraint cannot be narrowed
-- while rows violate it, and an audit entry for a merge has no equivalent in
-- the older vocabulary.
DELETE FROM ownership_audit_log WHERE action = 'owner_merged';

ALTER TABLE ownership_audit_log DROP CONSTRAINT ownership_audit_log_action_check;
ALTER TABLE ownership_audit_log ADD CONSTRAINT ownership_audit_log_action_check
    CHECK (action IN (
        'owner_created', 'owner_updated', 'owner_deleted',
        'assignment_created', 'assignment_deleted', 'assignment_reassigned'
    ));

ALTER TABLE ownership_audit_log DROP CONSTRAINT ownership_audit_log_action_entity;
ALTER TABLE ownership_audit_log ADD CONSTRAINT ownership_audit_log_action_entity
    CHECK (
        action IN ('owner_created', 'owner_updated', 'owner_deleted')
        OR (entity_type IS NOT NULL AND entity_key IS NOT NULL)
    );
