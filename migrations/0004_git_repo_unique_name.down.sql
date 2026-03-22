-- 0004_git_repo_unique_name.down.sql
--
-- Revert the unique constraint from (name) back to (name, git_repo_url).
-- No data restoration is needed — the duplicate rows deleted in the up
-- migration cannot be recovered, but the schema change is reversible.

ALTER TABLE git_repos
    DROP CONSTRAINT git_repos_name_key;

ALTER TABLE git_repos
    ADD CONSTRAINT git_repos_name_git_repo_url_key UNIQUE (name, git_repo_url);
