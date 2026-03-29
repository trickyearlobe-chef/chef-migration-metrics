-- Migration 0008: Create cookbook platform coverage analysis table
--
-- Stores platform coverage reports comparing kitchen-tested platforms
-- against production node platforms. Refreshed after each analysis cycle.
-- See test-kitchen-drivers.md § Platform Coverage Analysis.

CREATE TABLE IF NOT EXISTS cookbook_platform_coverage (
    id               UUID        NOT NULL DEFAULT gen_random_uuid(),
    git_repo_id      UUID,
    cookbook_name     TEXT        NOT NULL,
    coverage_data    JSONB       NOT NULL DEFAULT '{}',
    evaluated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (id),
    CONSTRAINT fk_cookbook_platform_coverage_git_repo
        FOREIGN KEY (git_repo_id) REFERENCES git_repos(id) ON DELETE CASCADE,
    CONSTRAINT uq_cookbook_platform_coverage_name
        UNIQUE (cookbook_name)
);

COMMENT ON TABLE cookbook_platform_coverage IS
    'Platform coverage analysis per cookbook: compares kitchen-tested platforms against production node data.';
COMMENT ON COLUMN cookbook_platform_coverage.coverage_data IS
    'JSONB coverage report: kitchen_platforms, production_platforms, tested_and_in_production, in_production_not_tested, coverage_percentage.';
