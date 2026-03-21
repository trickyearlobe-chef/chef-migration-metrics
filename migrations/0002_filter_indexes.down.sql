-- =============================================================================
-- Migration 0002: Filter Push-Down Indexes — Rollback
-- =============================================================================
-- Drops the indexes added by 0002_filter_indexes.up.sql.
-- =============================================================================

DROP INDEX IF EXISTS idx_collection_runs_org_completed_started;
DROP INDEX IF EXISTS idx_node_snapshots_policy_group_lower;
DROP INDEX IF EXISTS idx_node_snapshots_policy_name_lower;
DROP INDEX IF EXISTS idx_node_snapshots_chef_version_lower;
DROP INDEX IF EXISTS idx_node_snapshots_chef_environment_lower;
DROP INDEX IF EXISTS idx_node_snapshots_node_name_lower;
DROP INDEX IF EXISTS idx_node_snapshots_platform_combined;
DROP INDEX IF EXISTS idx_node_snapshots_roles_gin;
