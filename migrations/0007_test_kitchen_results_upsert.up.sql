-- ---------------------------------------------------------------------------
-- 0007_test_kitchen_results_upsert
-- ---------------------------------------------------------------------------
-- Convert git_repo_test_kitchen_results from accumulating one row per commit
-- to current-state (one row per git_repo_id + target_chef_version).
--
-- The table's original UNIQUE key (git_repo_id, target_chef_version, commit_sha)
-- meant every new commit created a new row, even though only the latest
-- result matters. All callers already use GetLatestGitRepoTestKitchenResult
-- (ORDER BY started_at DESC LIMIT 1), confirming that historical per-commit
-- rows serve no purpose. The commit_sha column is retained — it records
-- which commit was tested — but is no longer part of the uniqueness key.
--
-- Step 1: Deduplicate — keep only the most recent result per
--         (git_repo_id, target_chef_version), determined by started_at.
-- Step 2: Drop the old 3-column unique constraint.
-- Step 3: Add the new 2-column unique constraint.
-- ---------------------------------------------------------------------------

-- Step 1: Remove old results, keeping only the most recent per pair.
DELETE FROM git_repo_test_kitchen_results tkr
WHERE tkr.id NOT IN (
    SELECT DISTINCT ON (git_repo_id, target_chef_version) id
    FROM git_repo_test_kitchen_results
    ORDER BY git_repo_id, target_chef_version, started_at DESC
);

-- Step 2: Drop the old constraint.
ALTER TABLE git_repo_test_kitchen_results
    DROP CONSTRAINT uq_git_repo_test_kitchen_results;

-- Step 3: Add the new constraint (without commit_sha).
ALTER TABLE git_repo_test_kitchen_results
    ADD CONSTRAINT uq_git_repo_test_kitchen_results UNIQUE (git_repo_id, target_chef_version);
