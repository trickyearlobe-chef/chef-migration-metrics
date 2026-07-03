-- Cop classification overrides: operator-assigned migration impact level per cop per target version.
-- Priority: operator override > RemovedIn auto-seed > curated defaults > unclassified.

CREATE TABLE cop_classifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cop_name TEXT NOT NULL,
    target_chef_version TEXT NOT NULL,
    classification TEXT NOT NULL CHECK (classification IN ('blocker', 'review', 'noise')),
    reason TEXT,
    created_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cop_name, target_chef_version)
);

CREATE INDEX idx_cop_classifications_target ON cop_classifications (target_chef_version);
