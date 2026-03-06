-- =============================================================================
-- Migration 0004: Cookbook Download Status (rollback)
-- =============================================================================
-- Removes the download_status and download_error columns added by the up
-- migration. This reverts the cookbooks table to its pre-0004 state.
-- =============================================================================

-- Drop the index first.
DROP INDEX IF EXISTS idx_cookbooks_download_status;

-- Drop the check constraint.
ALTER TABLE cookbooks
    DROP CONSTRAINT IF EXISTS chk_cookbooks_download_status;

-- Drop the columns.
ALTER TABLE cookbooks
    DROP COLUMN IF EXISTS download_error;

ALTER TABLE cookbooks
    DROP COLUMN IF EXISTS download_status;
