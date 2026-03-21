-- ---------------------------------------------------------------------------
-- 0005_drop_cookbook_role_usage (rollback)
-- ---------------------------------------------------------------------------
-- Recreate the cookbook_role_usage table (empty — no data can be restored).
-- ---------------------------------------------------------------------------

CREATE TABLE cookbook_role_usage (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cookbook_name     TEXT        NOT NULL,
    role_name        TEXT        NOT NULL,
    organisation_id  UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_cookbook_role_usage UNIQUE (cookbook_name, role_name, organisation_id)
);

CREATE INDEX idx_cookbook_role_usage_cookbook_name ON cookbook_role_usage (cookbook_name);
CREATE INDEX idx_cookbook_role_usage_role_name ON cookbook_role_usage (role_name);
CREATE INDEX idx_cookbook_role_usage_organisation_id ON cookbook_role_usage (organisation_id);
