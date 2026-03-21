-- ---------------------------------------------------------------------------
-- 0009_drop_cookbook_node_usage (rollback)
-- ---------------------------------------------------------------------------
-- Recreate the cookbook_node_usage table (empty — no data can be restored).
-- This restores the original schema from migration 0001 including all
-- indexes.
-- ---------------------------------------------------------------------------

CREATE TABLE cookbook_node_usage (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    server_cookbook_id   UUID        NOT NULL REFERENCES server_cookbooks(id) ON DELETE CASCADE,
    node_snapshot_id    UUID        NOT NULL REFERENCES node_snapshots(id) ON DELETE CASCADE,
    cookbook_version     TEXT        NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cookbook_node_usage_server_cookbook_id ON cookbook_node_usage (server_cookbook_id);
CREATE INDEX idx_cookbook_node_usage_node_snapshot_id ON cookbook_node_usage (node_snapshot_id);
CREATE INDEX idx_cookbook_node_usage_cookbook_version ON cookbook_node_usage (server_cookbook_id, cookbook_version);
CREATE INDEX idx_cookbook_node_usage_snapshot_cookbook
    ON cookbook_node_usage (node_snapshot_id, server_cookbook_id);
