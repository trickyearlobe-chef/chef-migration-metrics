-- SPDX-License-Identifier: Apache-2.0

-- git_kitchen_results: per-instance Test Kitchen results replacing the
-- single-row-per-cookbook model in git_repo_test_kitchen_results.
-- The old table is retained for backward compatibility during migration.
CREATE TABLE git_kitchen_results (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id              UUID REFERENCES kitchen_batches(id) ON DELETE SET NULL,
    git_repo_name         TEXT NOT NULL,
    git_repo_url          TEXT NOT NULL,
    target_chef_version   TEXT NOT NULL,
    commit_sha            TEXT NOT NULL,
    platform_name         TEXT NOT NULL,
    suite_name            TEXT NOT NULL,
    template_used         TEXT,
    driver_used           TEXT,
    converge_passed       BOOLEAN,
    tests_passed          BOOLEAN,
    timed_out             BOOLEAN NOT NULL DEFAULT false,
    converge_output       TEXT,
    verify_output         TEXT,
    destroy_output        TEXT,
    duration_seconds      INTEGER,
    error_message         TEXT,
    started_at            TIMESTAMPTZ,
    completed_at          TIMESTAMPTZ,
    vm_tracking_id        UUID REFERENCES vm_tracking(id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Upsert latest result per instance.
CREATE UNIQUE INDEX idx_gkr_instance
    ON git_kitchen_results (git_repo_name, git_repo_url, target_chef_version, platform_name, suite_name);

-- Batch result queries.
CREATE INDEX idx_gkr_batch_id ON git_kitchen_results (batch_id) WHERE batch_id IS NOT NULL;

-- Per-repo result lookups.
CREATE INDEX idx_gkr_repo_name ON git_kitchen_results (git_repo_name);

-- Dashboard status filtering.
CREATE INDEX idx_gkr_status ON git_kitchen_results (converge_passed, tests_passed);
