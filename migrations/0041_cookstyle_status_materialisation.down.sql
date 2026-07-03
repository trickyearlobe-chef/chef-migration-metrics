-- SPDX-License-Identifier: Apache-2.0
-- Migration 0041 down: drop the materialised CookStyle rollup status columns.

DROP INDEX IF EXISTS idx_git_repos_cookstyle_status;

ALTER TABLE git_repos
    DROP CONSTRAINT IF EXISTS chk_git_repos_cookstyle_status;
ALTER TABLE git_repos
    DROP COLUMN IF EXISTS cookstyle_status;

ALTER TABLE git_repo_cookstyle_results
    DROP CONSTRAINT IF EXISTS chk_gr_cookstyle_results_status;
ALTER TABLE git_repo_cookstyle_results
    DROP COLUMN IF EXISTS cookstyle_status;

ALTER TABLE server_cookbook_cookstyle_results
    DROP CONSTRAINT IF EXISTS chk_sc_cookstyle_results_status;
ALTER TABLE server_cookbook_cookstyle_results
    DROP COLUMN IF EXISTS cookstyle_status;
