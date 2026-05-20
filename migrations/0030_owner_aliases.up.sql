-- Owner aliases table for identity resolution.
-- Maps alternative identifiers (emails, git names, etc.) to owners.

CREATE TABLE owner_aliases (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_name  TEXT        NOT NULL REFERENCES owners(name) ON DELETE CASCADE,
    alias_type  TEXT        NOT NULL,
    alias_value TEXT        NOT NULL,
    source      TEXT        NOT NULL DEFAULT 'manual',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_owner_alias UNIQUE (alias_type, alias_value),
    CONSTRAINT chk_alias_type CHECK (alias_type IN ('email', 'git_name', 'git_email', 'username', 'custom'))
);

CREATE INDEX idx_owner_aliases_owner ON owner_aliases (owner_name);
CREATE INDEX idx_owner_aliases_value ON owner_aliases (alias_value);

-- Enable pg_trgm extension for fuzzy matching (idempotent).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Trigram index for similarity search on alias_value.
CREATE INDEX idx_owner_aliases_trgm ON owner_aliases USING gin (alias_value gin_trgm_ops);
