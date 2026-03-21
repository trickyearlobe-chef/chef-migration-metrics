-- ---------------------------------------------------------------------------
-- 0007_test_kitchen_results_upsert (rollback)
-- ---------------------------------------------------------------------------
-- Revert the unique constraint back to the original 3-column form that
-- includes commit_sha. Note: this does NOT restore previously deduplicated
-- rows — only the most recent result per (git_repo_id, target_chef_version)
-- will remain.
-- ---------------------------------------------------------------------------

ALTER TABLE git_repo_test_kitchen_results
    DROP CONSTRAINT IF EXISTS uq_git_repo_test_kitchen_results;

ALTER TABLE git_repo_test_kitchen_results
    ADD CONSTRAINT uq_git_repo_test_kitchen_results UNIQUE (git_repo_id, target_chef_version, commit_sha);
