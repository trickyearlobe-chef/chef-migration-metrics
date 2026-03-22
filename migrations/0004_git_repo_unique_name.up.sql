-- 0004_git_repo_unique_name.up.sql
--
-- A git repo is identified by its cookbook name. There is one repo per name.
-- The git_repo_url records where it was cloned from but is not part of the
-- identity. Previously the unique constraint was (name, git_repo_url) which
-- allowed multiple rows per cookbook name when different base URLs were tried.
-- This migration collapses duplicates and enforces one row per name.

-- Step 1: Delete duplicate rows, keeping the most recently fetched row per
-- name. Cascading FK deletes clean up cookstyle results, test kitchen
-- results, autocorrect previews, and complexity records for the removed rows.
DELETE FROM git_repos
 WHERE id NOT IN (
    SELECT DISTINCT ON (name) id
      FROM git_repos
     ORDER BY name, last_fetched_at DESC NULLS LAST, created_at DESC
 );

-- Step 2: Drop the old composite unique constraint.
ALTER TABLE git_repos
    DROP CONSTRAINT git_repos_name_git_repo_url_key;

-- Step 3: Add the new unique constraint on name alone.
ALTER TABLE git_repos
    ADD CONSTRAINT git_repos_name_key UNIQUE (name);
