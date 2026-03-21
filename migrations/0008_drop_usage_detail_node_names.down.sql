-- ---------------------------------------------------------------------------
-- 0008_drop_usage_detail_node_names (rollback)
-- ---------------------------------------------------------------------------
-- Re-add the node_names JSONB column. The column will be NULL for all
-- existing rows — the data cannot be restored because it was stored only
-- in this column and is not recoverable from other tables without a full
-- re-analysis run.
-- ---------------------------------------------------------------------------

ALTER TABLE cookbook_usage_detail
    ADD COLUMN IF NOT EXISTS node_names JSONB;
