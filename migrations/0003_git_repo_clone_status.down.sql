-- 0003_git_repo_clone_status.down.sql
--
-- Reverse the clone_status / clone_error additions to git_repos.

DROP INDEX IF EXISTS idx_git_repos_clone_status;

ALTER TABLE git_repos
    DROP CONSTRAINT IF EXISTS chk_gr_clone_status;

ALTER TABLE git_repos
    DROP COLUMN IF EXISTS clone_error,
    DROP COLUMN IF EXISTS clone_status;
