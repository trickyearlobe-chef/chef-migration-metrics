-- ---------------------------------------------------------------------------
-- 0008_drop_usage_detail_node_names
-- ---------------------------------------------------------------------------
-- Remove the node_names JSONB column from cookbook_usage_detail.
--
-- This column stored a JSON array of every node name consuming a given
-- cookbook version. At scale (e.g. 50,000 nodes using chef-client), a single
-- row's node_names value can exceed 1 MB. Across all cookbook versions in an
-- organisation, this adds up to hundreds of megabytes of data that is:
--
--   1. Written on every collection run (delete-then-insert in TX).
--   2. Read back by ListCookbookUsageDetails in loadBlastRadii, which only
--      uses the integer node_count and policy_names — never node_names.
--   3. Never displayed in the UI (the frontend has zero references to it).
--
-- The node_count integer column already captures the useful aggregate.
-- If the actual list of node names is ever needed, it can be derived
-- on-demand from node_snapshots.cookbooks (the per-node JSONB that is
-- the source of truth).
-- ---------------------------------------------------------------------------

ALTER TABLE cookbook_usage_detail
    DROP COLUMN IF EXISTS node_names;
