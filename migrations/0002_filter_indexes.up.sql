-- =============================================================================
-- Migration 0002: Filter Push-Down Indexes
-- =============================================================================
-- Adds indexes to support SQL WHERE clause push-down for node snapshot
-- filtering. Previously, all filtering happened in Go memory after loading
-- every row. These indexes allow Postgres to efficiently evaluate the
-- filter predicates directly.
--
-- See: .claude/specifications/sql-filter-pushdown.md
-- =============================================================================

-- GIN index on the roles JSONB column to support the EXISTS subquery
-- used for role filtering:
--   EXISTS (SELECT 1 FROM jsonb_array_elements_text(roles) r
--           WHERE LOWER(r) LIKE '%' || LOWER($N) || '%')
-- The GIN index allows Postgres to quickly identify rows that contain
-- a given role element without a full table scan.
CREATE INDEX IF NOT EXISTS idx_node_snapshots_roles_gin
    ON node_snapshots USING GIN (roles jsonb_path_ops);

-- Expression index on the combined platform string used for platform
-- filtering. The filter matches against "platform platform_version"
-- as a single string, mirroring the in-memory FilterNodes behaviour.
CREATE INDEX IF NOT EXISTS idx_node_snapshots_platform_combined
    ON node_snapshots (LOWER(CONCAT(platform, ' ', COALESCE(platform_version, ''))));

-- Expression index on LOWER(node_name) to support case-insensitive
-- substring search on node names.
CREATE INDEX IF NOT EXISTS idx_node_snapshots_node_name_lower
    ON node_snapshots (LOWER(node_name));

-- Expression index on LOWER(chef_environment) to support case-insensitive
-- substring search on environments.
CREATE INDEX IF NOT EXISTS idx_node_snapshots_chef_environment_lower
    ON node_snapshots (LOWER(chef_environment));

-- Expression index on LOWER(chef_version) to support case-insensitive
-- substring search on Chef client versions.
CREATE INDEX IF NOT EXISTS idx_node_snapshots_chef_version_lower
    ON node_snapshots (LOWER(chef_version));

-- Expression index on LOWER(policy_name) to support case-insensitive
-- substring search on policy names.
CREATE INDEX IF NOT EXISTS idx_node_snapshots_policy_name_lower
    ON node_snapshots (LOWER(policy_name));

-- Expression index on LOWER(policy_group) to support case-insensitive
-- substring search on policy groups.
CREATE INDEX IF NOT EXISTS idx_node_snapshots_policy_group_lower
    ON node_snapshots (LOWER(policy_group));

-- Composite index to accelerate the "latest completed collection run per org"
-- correlated subquery used in the CTE:
--   SELECT MAX(cr2.started_at) FROM collection_runs cr2
--    WHERE cr2.organisation_id = ns.organisation_id
--      AND cr2.status = 'completed'
-- The existing idx_collection_runs_org_status_started covers this, but an
-- explicit covering index with the right column order ensures the subquery
-- is always an index-only scan.
CREATE INDEX IF NOT EXISTS idx_collection_runs_org_completed_started
    ON collection_runs (organisation_id, started_at DESC)
    WHERE status = 'completed';
