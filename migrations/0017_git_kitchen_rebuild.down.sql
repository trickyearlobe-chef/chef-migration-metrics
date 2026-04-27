-- SPDX-License-Identifier: Apache-2.0

-- Reverse the git_kitchen_results reshape.
DROP INDEX IF EXISTS idx_gkr_passed;

ALTER TABLE git_kitchen_results
    DROP COLUMN IF EXISTS instance_name,
    DROP COLUMN IF EXISTS passed,
    DROP COLUMN IF EXISTS output;

ALTER TABLE git_kitchen_results
    ADD COLUMN IF NOT EXISTS batch_id UUID REFERENCES kitchen_batches(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS vm_tracking_id UUID REFERENCES vm_tracking(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS template_used TEXT,
    ADD COLUMN IF NOT EXISTS converge_passed BOOLEAN,
    ADD COLUMN IF NOT EXISTS tests_passed BOOLEAN,
    ADD COLUMN IF NOT EXISTS converge_output TEXT,
    ADD COLUMN IF NOT EXISTS verify_output TEXT,
    ADD COLUMN IF NOT EXISTS destroy_output TEXT;

CREATE INDEX IF NOT EXISTS idx_gkr_batch_id ON git_kitchen_results (batch_id) WHERE batch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_gkr_status ON git_kitchen_results (converge_passed, tests_passed);

-- Recreate the old table (empty — data is lost).
CREATE TABLE IF NOT EXISTS git_repo_test_kitchen_results (
    target_chef_version TEXT NOT NULL,
    commit_sha          TEXT NOT NULL,
    converge_passed     BOOLEAN NOT NULL,
    tests_passed        BOOLEAN NOT NULL,
    compatible          BOOLEAN NOT NULL,
    process_stdout      TEXT,
    process_stderr      TEXT,
    converge_output     TEXT,
    verify_output       TEXT,
    destroy_output      TEXT,
    timed_out           BOOLEAN NOT NULL DEFAULT false,
    driver_used         TEXT,
    platform_tested     TEXT,
    overrides_applied   BOOLEAN NOT NULL DEFAULT false,
    duration_seconds    INTEGER,
    started_at          TIMESTAMPTZ NOT NULL,
    completed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    driver              TEXT,
    platform_name       TEXT,
    git_repo_name       TEXT NOT NULL,
    git_repo_url        TEXT NOT NULL,
    PRIMARY KEY (git_repo_name, git_repo_url, target_chef_version)
);
