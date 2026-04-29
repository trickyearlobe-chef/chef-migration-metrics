-- SPDX-License-Identifier: Apache-2.0

-- kitchen_instance_exclusions: manual per-repo suite+platform exclusions
-- with user-supplied reason.
CREATE TABLE kitchen_instance_exclusions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    git_repo_name  TEXT NOT NULL,
    git_repo_url   TEXT NOT NULL,
    suite_name     TEXT NOT NULL,
    platform_name  TEXT NOT NULL,
    reason         TEXT NOT NULL,
    excluded_by    TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One exclusion per instance per repo.
CREATE UNIQUE INDEX idx_kie_instance
    ON kitchen_instance_exclusions (git_repo_name, git_repo_url, suite_name, platform_name);

-- Per-repo lookups.
CREATE INDEX idx_kie_repo
    ON kitchen_instance_exclusions (git_repo_name, git_repo_url);
