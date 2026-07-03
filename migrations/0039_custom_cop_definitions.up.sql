-- Custom cop definitions: operator-defined pattern matchers for issues not covered by cookstyle.
-- Scanned during analysis alongside cookstyle; offenses stored in same JSONB format.

CREATE TABLE custom_cop_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cop_name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL,
    pattern_type TEXT NOT NULL CHECK (pattern_type IN ('regex', 'literal')),
    pattern TEXT NOT NULL,
    file_glob TEXT NOT NULL DEFAULT '*.rb',
    target_chef_version_min TEXT,
    removed_in TEXT,
    classification TEXT NOT NULL DEFAULT 'blocker' CHECK (classification IN ('blocker', 'review', 'noise')),
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
