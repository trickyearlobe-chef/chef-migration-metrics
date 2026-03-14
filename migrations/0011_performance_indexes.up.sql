-- ---------------------------------------------------------------------------
-- 0011: Performance indexes for 100K+ node scale
-- ---------------------------------------------------------------------------
-- These composite indexes optimize the most common query patterns that
-- become bottlenecks at scale (100K nodes, 5K cookbooks, 5+ orgs).
-- ---------------------------------------------------------------------------

-- collection_runs: optimise the MAX(started_at) subquery pattern
-- used by ListNodeSnapshotsByOrganisation, trend handlers, etc.
-- Replaces bitmap-AND of three separate single-column indexes.
CREATE INDEX IF NOT EXISTS idx_collection_runs_org_status_started
    ON collection_runs (organisation_id, status, started_at DESC);

-- node_snapshots: optimise GetNodeSnapshotByName lookups
-- (organisation_id + node_name + collected_at DESC)
CREATE INDEX IF NOT EXISTS idx_node_snapshots_org_name_collected
    ON node_snapshots (organisation_id, node_name, collected_at DESC);

-- node_readiness: support owner-related queries that filter by
-- target_chef_version without organisation_id (e.g. listOwnersWithSummary)
CREATE INDEX IF NOT EXISTS idx_node_readiness_target_name_eval
    ON node_readiness (target_chef_version, node_name, evaluated_at DESC)
    INCLUDE (id, is_ready, stale_data, blocking_cookbooks);

-- cookbooks: support git-repo-based lookups in ownership queries
CREATE INDEX IF NOT EXISTS idx_cookbooks_git_repo_url
    ON cookbooks (git_repo_url) WHERE source = 'git';

-- cookbook_node_usage: composite for join + GROUP BY patterns
CREATE INDEX IF NOT EXISTS idx_cookbook_node_usage_snapshot_cookbook
    ON cookbook_node_usage (node_snapshot_id, cookbook_id);
