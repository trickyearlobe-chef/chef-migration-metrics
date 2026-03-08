-- =============================================================================
-- Migration 0004: Cookbook Download Status
-- =============================================================================
-- Adds download_status and download_error columns to the cookbooks table to
-- support the cookbook fetching pipeline. Cookbook versions downloaded from the
-- Chef server are immutable — once successfully downloaded (status = 'ok'),
-- they are not re-downloaded on subsequent runs. Failed or pending versions
-- are retried on the next collection run.
--
-- See datastore/Specification.md § 4 (cookbooks) and
-- data-collection/Specification.md § 2.4 (Cookbook Download Failure Handling).
-- =============================================================================

-- Add download_status column. Default 'pending' for new rows and for all
-- existing rows (which have never been through the download pipeline).
ALTER TABLE cookbooks
    ADD COLUMN download_status TEXT NOT NULL DEFAULT 'pending';

-- Add download_error column. Nullable — only populated when status = 'failed'.
ALTER TABLE cookbooks
    ADD COLUMN download_error TEXT;

-- Check constraint: download_status must be one of the three valid values.
ALTER TABLE cookbooks
    ADD CONSTRAINT chk_cookbooks_download_status
        CHECK (download_status IN ('ok', 'failed', 'pending'));

-- Index for efficiently querying cookbooks by download status (e.g. finding
-- all pending/failed versions that need to be retried).
CREATE INDEX idx_cookbooks_download_status ON cookbooks (download_status);

-- Mark all existing chef_server cookbooks as 'ok' since they were already
-- accepted into the datastore before this migration. Git-sourced cookbooks
-- are also marked 'ok' since they are managed via git clone/pull, not the
-- Chef server download pipeline.
UPDATE cookbooks SET download_status = 'ok' WHERE source IN ('chef_server', 'git');
