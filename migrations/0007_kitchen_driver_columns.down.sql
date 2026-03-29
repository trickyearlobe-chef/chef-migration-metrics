-- Migration 0007 (down): Remove driver tracking columns
ALTER TABLE git_repo_test_kitchen_results
    DROP COLUMN IF EXISTS platform_name,
    DROP COLUMN IF EXISTS driver;
