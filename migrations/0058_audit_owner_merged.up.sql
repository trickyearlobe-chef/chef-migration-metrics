-- Folding one owner into another is a new kind of ownership change, and it
-- deletes a person. The audit table's action column is constrained to a fixed
-- list, so without this the merge audit entry is rejected — and the handler
-- only logs that, which would leave merges silently unaudited.
--
-- owner_merged is an owner-level action: it names the surviving owner and has
-- no single entity, so it joins the list the entity-fields check exempts.

ALTER TABLE ownership_audit_log DROP CONSTRAINT ownership_audit_log_action_check;
ALTER TABLE ownership_audit_log ADD CONSTRAINT ownership_audit_log_action_check
    CHECK (action IN (
        'owner_created', 'owner_updated', 'owner_deleted', 'owner_merged',
        'assignment_created', 'assignment_deleted', 'assignment_reassigned'
    ));

ALTER TABLE ownership_audit_log DROP CONSTRAINT ownership_audit_log_action_entity;
ALTER TABLE ownership_audit_log ADD CONSTRAINT ownership_audit_log_action_entity
    CHECK (
        action IN ('owner_created', 'owner_updated', 'owner_deleted', 'owner_merged')
        OR (entity_type IS NOT NULL AND entity_key IS NOT NULL)
    );
