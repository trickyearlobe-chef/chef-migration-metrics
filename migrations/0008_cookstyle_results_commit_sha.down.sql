-- ---------------------------------------------------------------------------
-- Migration 0008 (down): Remove commit_sha from cookstyle_results
-- ---------------------------------------------------------------------------

DROP INDEX IF EXISTS idx_cookstyle_results_commit_sha;

ALTER TABLE cookstyle_results DROP COLUMN IF EXISTS commit_sha;
