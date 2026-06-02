-- SPDX-License-Identifier: Apache-2.0
-- Rollback migration 0032: Remove materialised status columns from git_repos.

ALTER TABLE git_repos
    DROP CONSTRAINT IF EXISTS chk_git_repos_compatibility_status,
    DROP CONSTRAINT IF EXISTS chk_git_repos_tk_status;

DROP INDEX IF EXISTS idx_git_repos_compatibility_status;
DROP INDEX IF EXISTS idx_git_repos_tk_status;

ALTER TABLE git_repos
    DROP COLUMN IF EXISTS compatibility_status,
    DROP COLUMN IF EXISTS tk_status,
    DROP COLUMN IF EXISTS tk_passed,
    DROP COLUMN IF EXISTS tk_total;
