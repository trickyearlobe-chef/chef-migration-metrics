-- Migration 0007: Add driver tracking columns to test kitchen results
--
-- Records which driver was used for each Test Kitchen run and the specific
-- platform name tested. Enables per-driver and per-platform result filtering.

ALTER TABLE git_repo_test_kitchen_results
    ADD COLUMN IF NOT EXISTS driver TEXT,
    ADD COLUMN IF NOT EXISTS platform_name TEXT;

COMMENT ON COLUMN git_repo_test_kitchen_results.driver IS
    'Test Kitchen driver used (e.g. dokken, vcenter, ec2). NULL for pre-existing rows (implies dokken).';
COMMENT ON COLUMN git_repo_test_kitchen_results.platform_name IS
    'Kitchen platform name tested. Enables per-platform result tracking for non-dokken drivers.';
