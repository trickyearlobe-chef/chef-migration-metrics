-- 0003_git_repo_clone_status.up.sql
--
-- Add clone_status and clone_error columns to git_repos so that failed
-- clone attempts are persisted and visible in the UI, mirroring the
-- download_status / download_error pattern on server_cookbooks.

ALTER TABLE git_repos
    ADD COLUMN clone_status TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN clone_error  TEXT;

-- Existing rows represent successfully cloned repos, so backfill them.
UPDATE git_repos SET clone_status = 'ok' WHERE clone_status = 'pending';

-- Enforce the same three-state enum used by server_cookbooks.download_status.
ALTER TABLE git_repos
    ADD CONSTRAINT chk_gr_clone_status
        CHECK (clone_status IN ('ok', 'failed', 'pending'));

CREATE INDEX idx_git_repos_clone_status ON git_repos (clone_status);
