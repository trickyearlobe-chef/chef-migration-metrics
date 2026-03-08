-- ---------------------------------------------------------------------------
-- Migration 0008: Add commit_sha to cookstyle_results
-- ---------------------------------------------------------------------------
-- CookStyle scanning now runs against git-sourced cookbooks in addition to
-- Chef server-sourced cookbooks. Git cookbook content changes with each commit,
-- so we need to track which commit was scanned and skip re-scanning when the
-- HEAD commit has not changed.
--
-- For Chef server-sourced cookbooks this column is NULL (versions are
-- immutable, so the existing immutability skip logic is sufficient).
-- ---------------------------------------------------------------------------

ALTER TABLE cookstyle_results ADD COLUMN commit_sha TEXT;

CREATE INDEX idx_cookstyle_results_commit_sha ON cookstyle_results (commit_sha)
    WHERE commit_sha IS NOT NULL;
