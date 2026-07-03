-- CookStyle criteria-change audit log: records cop reclassifications and
-- custom-cop changes (the events that trigger the re-evaluation propagation
-- closure) for explainability. Mirrors the ownership_audit_log pattern but is
-- cop-centric rather than owner-centric.

CREATE TABLE cookstyle_audit_log (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp           TIMESTAMPTZ NOT NULL DEFAULT now(),
    action              TEXT        NOT NULL,
    actor               TEXT        NOT NULL,
    cop_name            TEXT        NOT NULL,
    target_chef_version TEXT,
    details             JSONB
);

CREATE INDEX idx_cookstyle_audit_log_timestamp ON cookstyle_audit_log (timestamp DESC);
CREATE INDEX idx_cookstyle_audit_log_cop       ON cookstyle_audit_log (cop_name);
CREATE INDEX idx_cookstyle_audit_log_action    ON cookstyle_audit_log (action);
