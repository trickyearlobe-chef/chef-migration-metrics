-- 0009_ownership_tracking.up.sql
-- Adds ownership tracking: owners, assignments, git repo committers, and audit log.

-- ---------------------------------------------------------------------------
-- owners — named owners representing teams, individuals, or cost centres
-- ---------------------------------------------------------------------------
CREATE TABLE owners (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT        NOT NULL,
    display_name    TEXT,
    contact_email   TEXT,
    contact_channel TEXT,
    owner_type      TEXT        NOT NULL CHECK (owner_type IN ('team', 'individual', 'business_unit', 'cost_centre', 'custom')),
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT owners_name_unique UNIQUE (name),
    CONSTRAINT owners_name_format CHECK (name ~ '^[a-z0-9][a-z0-9._-]*$')
);

CREATE INDEX idx_owners_owner_type ON owners (owner_type);

-- ---------------------------------------------------------------------------
-- ownership_assignments — many-to-many links between owners and entities
-- ---------------------------------------------------------------------------
CREATE TABLE ownership_assignments (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id          UUID        NOT NULL REFERENCES owners (id) ON DELETE CASCADE,
    entity_type       TEXT        NOT NULL CHECK (entity_type IN ('node', 'cookbook', 'git_repo', 'role', 'policy')),
    entity_key        TEXT        NOT NULL,
    organisation_id   UUID        REFERENCES organisations (id) ON DELETE CASCADE,
    assignment_source TEXT        NOT NULL CHECK (assignment_source IN ('manual', 'auto_rule', 'import')),
    auto_rule_name    TEXT,
    confidence        TEXT        NOT NULL CHECK (confidence IN ('definitive', 'inferred')),
    notes             TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Unique constraint that handles nullable organisation_id correctly.
-- Two assignments differing only by one having NULL organisation are distinct.
CREATE UNIQUE INDEX idx_ownership_assignments_unique
    ON ownership_assignments (owner_id, entity_type, entity_key, COALESCE(organisation_id, '00000000-0000-0000-0000-000000000000'));

CREATE INDEX idx_ownership_assignments_owner_id     ON ownership_assignments (owner_id);
CREATE INDEX idx_ownership_assignments_entity       ON ownership_assignments (entity_type, entity_key);
CREATE INDEX idx_ownership_assignments_org          ON ownership_assignments (organisation_id) WHERE organisation_id IS NOT NULL;
CREATE INDEX idx_ownership_assignments_source       ON ownership_assignments (assignment_source);
CREATE INDEX idx_ownership_assignments_auto_rule    ON ownership_assignments (auto_rule_name) WHERE auto_rule_name IS NOT NULL;

-- ---------------------------------------------------------------------------
-- git_repo_committers — committer history extracted from git repositories
-- ---------------------------------------------------------------------------
CREATE TABLE git_repo_committers (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    git_repo_url    TEXT        NOT NULL,
    author_name     TEXT        NOT NULL,
    author_email    TEXT        NOT NULL,
    commit_count    INTEGER     NOT NULL DEFAULT 0,
    first_commit_at TIMESTAMPTZ NOT NULL,
    last_commit_at  TIMESTAMPTZ NOT NULL,
    collected_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT git_repo_committers_unique UNIQUE (git_repo_url, author_email)
);

CREATE INDEX idx_git_repo_committers_repo ON git_repo_committers (git_repo_url);

-- ---------------------------------------------------------------------------
-- ownership_audit_log — append-only log of all ownership mutations
-- ---------------------------------------------------------------------------
CREATE TABLE ownership_audit_log (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT now(),
    action          TEXT        NOT NULL CHECK (action IN (
        'owner_created', 'owner_updated', 'owner_deleted',
        'assignment_created', 'assignment_deleted', 'assignment_reassigned'
    )),
    actor           TEXT        NOT NULL,
    owner_name      TEXT        NOT NULL,
    entity_type     TEXT,
    entity_key      TEXT,
    organisation    TEXT,
    details         JSONB,

    CONSTRAINT ownership_audit_log_action_entity CHECK (
        -- owner-level actions don't need entity fields
        (action IN ('owner_created', 'owner_updated', 'owner_deleted'))
        OR
        -- assignment-level actions require entity fields
        (entity_type IS NOT NULL AND entity_key IS NOT NULL)
    )
);

CREATE INDEX idx_ownership_audit_log_timestamp  ON ownership_audit_log (timestamp DESC);
CREATE INDEX idx_ownership_audit_log_action     ON ownership_audit_log (action);
CREATE INDEX idx_ownership_audit_log_owner      ON ownership_audit_log (owner_name);
CREATE INDEX idx_ownership_audit_log_actor      ON ownership_audit_log (actor);
CREATE INDEX idx_ownership_audit_log_entity     ON ownership_audit_log (entity_type, entity_key) WHERE entity_type IS NOT NULL;

-- ---------------------------------------------------------------------------
-- node_snapshots — add custom_attributes column for auto-derivation
-- ---------------------------------------------------------------------------
ALTER TABLE node_snapshots ADD COLUMN IF NOT EXISTS custom_attributes JSONB;
