-- =============================================================================
-- Migration 0009: Convert UUID Primary Keys to Natural Keys
-- =============================================================================
-- Converts all tables from synthetic UUID primary keys to composite natural
-- keys. Processes in phased approach so UUID relationships remain available
-- for JOIN-based population of new columns before being dropped.
--
-- Phase A: Add new natural key columns (nullable initially)
-- Phase B: Populate new columns via UPDATE ... FROM ... JOIN
-- Phase C: Set NOT NULL constraints on new columns
-- Phase D: Drop existing FK constraints, UNIQUE constraints, and indexes
-- Phase E: Drop UUID columns (old PKs and old FK columns)
-- Phase F: Add new PRIMARY KEY constraints
-- Phase G: Add new FOREIGN KEY constraints
-- Phase H: Recreate indexes
-- =============================================================================

-- ===========================================================================
-- PHASE A: Add new natural key columns (nullable initially)
-- ===========================================================================

-- organisations: needs client_key_credential_name
ALTER TABLE organisations
    ADD COLUMN client_key_credential_name TEXT;

-- collection_runs: needs organisation_name
ALTER TABLE collection_runs
    ADD COLUMN organisation_name TEXT;

-- server_cookbooks: needs organisation_name
ALTER TABLE server_cookbooks
    ADD COLUMN organisation_name TEXT;

-- node_snapshots: needs organisation_name, collection_run_org
ALTER TABLE node_snapshots
    ADD COLUMN organisation_name TEXT,
    ADD COLUMN collection_run_org TEXT;

-- sessions: needs username column — already has one from 0001, so no change

-- ownership_assignments: needs owner_name, organisation_name
ALTER TABLE ownership_assignments
    ADD COLUMN owner_name TEXT,
    ADD COLUMN organisation_name TEXT;

-- role_dependencies: needs organisation_name
ALTER TABLE role_dependencies
    ADD COLUMN organisation_name TEXT;

-- metric_snapshots: needs organisation_name, collection_run_org
ALTER TABLE metric_snapshots
    ADD COLUMN organisation_name TEXT,
    ADD COLUMN collection_run_org TEXT;

-- cookbook_usage_analysis: needs organisation_name, collection_run_org
ALTER TABLE cookbook_usage_analysis
    ADD COLUMN organisation_name TEXT,
    ADD COLUMN collection_run_org TEXT;

-- log_entries: needs collection_run_org
ALTER TABLE log_entries
    ADD COLUMN collection_run_org TEXT;

-- node_readiness: needs organisation_name (to replace organisation_id)
ALTER TABLE node_readiness
    ADD COLUMN organisation_name TEXT;

-- server_cookbook_cookstyle_results: needs organisation_name, cookbook_name, cookbook_version
ALTER TABLE server_cookbook_cookstyle_results
    ADD COLUMN organisation_name TEXT,
    ADD COLUMN cookbook_name TEXT,
    ADD COLUMN cookbook_version TEXT;

-- server_cookbook_complexity: needs organisation_name, cookbook_name, cookbook_version
ALTER TABLE server_cookbook_complexity
    ADD COLUMN organisation_name TEXT,
    ADD COLUMN cookbook_name TEXT,
    ADD COLUMN cookbook_version TEXT;

-- git_repo_cookstyle_results: needs git_repo_name, git_repo_url
ALTER TABLE git_repo_cookstyle_results
    ADD COLUMN git_repo_name TEXT,
    ADD COLUMN git_repo_url TEXT;

-- git_repo_complexity: needs git_repo_name, git_repo_url
ALTER TABLE git_repo_complexity
    ADD COLUMN git_repo_name TEXT,
    ADD COLUMN git_repo_url TEXT;

-- git_repo_test_kitchen_results: needs git_repo_name, git_repo_url
ALTER TABLE git_repo_test_kitchen_results
    ADD COLUMN git_repo_name TEXT,
    ADD COLUMN git_repo_url TEXT;

-- cookbook_usage_detail: needs organisation_name
ALTER TABLE cookbook_usage_detail
    ADD COLUMN organisation_name TEXT;

-- cookbook_platform_coverage: needs git_repo_name, git_repo_url
ALTER TABLE cookbook_platform_coverage
    ADD COLUMN git_repo_name TEXT,
    ADD COLUMN git_repo_url TEXT;

-- server_cookbook_autocorrect_previews: needs organisation_name, cookbook_name, cookbook_version, target_chef_version
ALTER TABLE server_cookbook_autocorrect_previews
    ADD COLUMN organisation_name TEXT,
    ADD COLUMN cookbook_name TEXT,
    ADD COLUMN cookbook_version TEXT,
    ADD COLUMN target_chef_version TEXT;

-- git_repo_autocorrect_previews: needs git_repo_name, git_repo_url, target_chef_version
ALTER TABLE git_repo_autocorrect_previews
    ADD COLUMN git_repo_name TEXT,
    ADD COLUMN git_repo_url TEXT,
    ADD COLUMN target_chef_version TEXT;


-- ===========================================================================
-- PHASE B: Populate new columns via JOINs through existing UUID FKs
-- ===========================================================================

-- --- Tier 1: Root entities (no FK columns to populate, only cross-references) ---

-- organisations.client_key_credential_name from credentials
UPDATE organisations o
   SET client_key_credential_name = c.name
  FROM credentials c
 WHERE o.client_key_credential_id = c.id;

-- --- Tier 2: Depend on Tier 1 ---

-- collection_runs.organisation_name from organisations
UPDATE collection_runs cr
   SET organisation_name = o.name
  FROM organisations o
 WHERE cr.organisation_id = o.id;

-- Delete orphaned collection_runs with no matching organisation
DELETE FROM collection_runs
 WHERE organisation_name IS NULL;

-- server_cookbooks.organisation_name from organisations
UPDATE server_cookbooks sc
   SET organisation_name = o.name
  FROM organisations o
 WHERE sc.organisation_id = o.id;

DELETE FROM server_cookbooks
 WHERE organisation_name IS NULL;

-- node_snapshots.organisation_name from organisations, collection_run_org from collection_runs
UPDATE node_snapshots ns
   SET organisation_name = o.name
  FROM organisations o
 WHERE ns.organisation_id = o.id;

UPDATE node_snapshots ns
   SET collection_run_org = cr.organisation_name
  FROM collection_runs cr
 WHERE ns.collection_run_id = cr.id;

DELETE FROM node_snapshots
 WHERE organisation_name IS NULL;

-- sessions: username column already populated from 0001 schema — no update needed

-- ownership_assignments.owner_name from owners, organisation_name from organisations
UPDATE ownership_assignments oa
   SET owner_name = ow.name
  FROM owners ow
 WHERE oa.owner_id = ow.id;

UPDATE ownership_assignments oa
   SET organisation_name = o.name
  FROM organisations o
 WHERE oa.organisation_id = o.id;
-- organisation_name stays NULL where organisation_id was NULL — that is correct

DELETE FROM ownership_assignments
 WHERE owner_name IS NULL;

-- role_dependencies.organisation_name from organisations
UPDATE role_dependencies rd
   SET organisation_name = o.name
  FROM organisations o
 WHERE rd.organisation_id = o.id;

DELETE FROM role_dependencies
 WHERE organisation_name IS NULL;

-- metric_snapshots.organisation_name from organisations, collection_run_org from collection_runs
UPDATE metric_snapshots ms
   SET organisation_name = o.name
  FROM organisations o
 WHERE ms.organisation_id = o.id;

UPDATE metric_snapshots ms
   SET collection_run_org = cr.organisation_name
  FROM collection_runs cr
 WHERE ms.collection_run_id = cr.id;
-- collection_run_org stays NULL where collection_run_id was NULL

DELETE FROM metric_snapshots
 WHERE organisation_name IS NULL;

-- cookbook_usage_analysis.organisation_name from organisations, collection_run_org from collection_runs
UPDATE cookbook_usage_analysis cua
   SET organisation_name = o.name
  FROM organisations o
 WHERE cua.organisation_id = o.id;

UPDATE cookbook_usage_analysis cua
   SET collection_run_org = cr.organisation_name
  FROM collection_runs cr
 WHERE cua.collection_run_id = cr.id;

DELETE FROM cookbook_usage_analysis
 WHERE organisation_name IS NULL;

-- log_entries.collection_run_org from collection_runs
UPDATE log_entries le
   SET collection_run_org = cr.organisation_name
  FROM collection_runs cr
 WHERE le.collection_run_id = cr.id;
-- collection_run_org stays NULL where collection_run_id was NULL

-- git_repo_committers: no new columns needed — already has (git_repo_url, author_email)

-- --- Tier 3: Depend on Tier 2 ---

-- node_readiness.organisation_name from organisations
UPDATE node_readiness nr
   SET organisation_name = o.name
  FROM organisations o
 WHERE nr.organisation_id = o.id;

DELETE FROM node_readiness
 WHERE organisation_name IS NULL;

-- server_cookbook_cookstyle_results from server_cookbooks
UPDATE server_cookbook_cookstyle_results cr
   SET organisation_name = sc.organisation_name,
       cookbook_name = sc.name,
       cookbook_version = sc.version
  FROM server_cookbooks sc
 WHERE cr.server_cookbook_id = sc.id;

DELETE FROM server_cookbook_cookstyle_results
 WHERE organisation_name IS NULL;

-- server_cookbook_complexity from server_cookbooks
UPDATE server_cookbook_complexity cx
   SET organisation_name = sc.organisation_name,
       cookbook_name = sc.name,
       cookbook_version = sc.version
  FROM server_cookbooks sc
 WHERE cx.server_cookbook_id = sc.id;

DELETE FROM server_cookbook_complexity
 WHERE organisation_name IS NULL;

-- git_repo_cookstyle_results from git_repos
UPDATE git_repo_cookstyle_results cr
   SET git_repo_name = gr.name,
       git_repo_url = gr.git_repo_url
  FROM git_repos gr
 WHERE cr.git_repo_id = gr.id;

DELETE FROM git_repo_cookstyle_results
 WHERE git_repo_name IS NULL;

-- git_repo_complexity from git_repos
UPDATE git_repo_complexity cx
   SET git_repo_name = gr.name,
       git_repo_url = gr.git_repo_url
  FROM git_repos gr
 WHERE cx.git_repo_id = gr.id;

DELETE FROM git_repo_complexity
 WHERE git_repo_name IS NULL;

-- git_repo_test_kitchen_results from git_repos
UPDATE git_repo_test_kitchen_results tk
   SET git_repo_name = gr.name,
       git_repo_url = gr.git_repo_url
  FROM git_repos gr
 WHERE tk.git_repo_id = gr.id;

DELETE FROM git_repo_test_kitchen_results
 WHERE git_repo_name IS NULL;

-- cookbook_usage_detail.organisation_name from organisations
UPDATE cookbook_usage_detail cud
   SET organisation_name = o.name
  FROM organisations o
 WHERE cud.organisation_id = o.id;

DELETE FROM cookbook_usage_detail
 WHERE organisation_name IS NULL;

-- cookbook_platform_coverage from git_repos
UPDATE cookbook_platform_coverage cpc
   SET git_repo_name = gr.name,
       git_repo_url = gr.git_repo_url
  FROM git_repos gr
 WHERE cpc.git_repo_id = gr.id;
-- git_repo_name/git_repo_url stay NULL where git_repo_id was NULL

-- --- Tier 4: Depend on Tier 3 ---

-- server_cookbook_autocorrect_previews from server_cookbooks and cookstyle_results
UPDATE server_cookbook_autocorrect_previews ap
   SET organisation_name = sc.organisation_name,
       cookbook_name = sc.name,
       cookbook_version = sc.version,
       target_chef_version = cr.target_chef_version
  FROM server_cookbooks sc,
       server_cookbook_cookstyle_results cr
 WHERE cr.id = ap.cookstyle_result_id
   AND sc.id = ap.server_cookbook_id;

DELETE FROM server_cookbook_autocorrect_previews
 WHERE organisation_name IS NULL;

-- git_repo_autocorrect_previews from git_repos and cookstyle_results
UPDATE git_repo_autocorrect_previews ap
   SET git_repo_name = gr.name,
       git_repo_url = gr.git_repo_url,
       target_chef_version = cr.target_chef_version
  FROM git_repos gr,
       git_repo_cookstyle_results cr
 WHERE cr.id = ap.cookstyle_result_id
   AND gr.id = ap.git_repo_id;

DELETE FROM git_repo_autocorrect_previews
 WHERE git_repo_name IS NULL;


-- ===========================================================================
-- PHASE C: Set NOT NULL constraints on new columns (where appropriate)
-- ===========================================================================

-- Tier 1
-- organisations.client_key_credential_name — nullable (mirrors old FK ON DELETE SET NULL)

-- Tier 2
ALTER TABLE collection_runs
    ALTER COLUMN organisation_name SET NOT NULL;

ALTER TABLE server_cookbooks
    ALTER COLUMN organisation_name SET NOT NULL;

ALTER TABLE node_snapshots
    ALTER COLUMN organisation_name SET NOT NULL;
-- node_snapshots.collection_run_org — nullable (collection run may be purged)

ALTER TABLE ownership_assignments
    ALTER COLUMN owner_name SET NOT NULL;
-- ownership_assignments.organisation_name — nullable (not all entities are org-scoped)

ALTER TABLE role_dependencies
    ALTER COLUMN organisation_name SET NOT NULL;

ALTER TABLE metric_snapshots
    ALTER COLUMN organisation_name SET NOT NULL;
-- metric_snapshots.collection_run_org — nullable

ALTER TABLE cookbook_usage_analysis
    ALTER COLUMN organisation_name SET NOT NULL;
-- cookbook_usage_analysis.collection_run_org — nullable (shouldn't be, but defensive)

-- log_entries.collection_run_org — nullable (added as nullable, no change needed)

-- Tier 3
ALTER TABLE node_readiness
    ALTER COLUMN organisation_name SET NOT NULL;

ALTER TABLE server_cookbook_cookstyle_results
    ALTER COLUMN organisation_name SET NOT NULL,
    ALTER COLUMN cookbook_name SET NOT NULL,
    ALTER COLUMN cookbook_version SET NOT NULL;

ALTER TABLE server_cookbook_complexity
    ALTER COLUMN organisation_name SET NOT NULL,
    ALTER COLUMN cookbook_name SET NOT NULL,
    ALTER COLUMN cookbook_version SET NOT NULL;

ALTER TABLE git_repo_cookstyle_results
    ALTER COLUMN git_repo_name SET NOT NULL,
    ALTER COLUMN git_repo_url SET NOT NULL;

ALTER TABLE git_repo_complexity
    ALTER COLUMN git_repo_name SET NOT NULL,
    ALTER COLUMN git_repo_url SET NOT NULL;

ALTER TABLE git_repo_test_kitchen_results
    ALTER COLUMN git_repo_name SET NOT NULL,
    ALTER COLUMN git_repo_url SET NOT NULL;

ALTER TABLE cookbook_usage_detail
    ALTER COLUMN organisation_name SET NOT NULL;

-- cookbook_platform_coverage.git_repo_name, git_repo_url — nullable (git_repo_id was nullable)

-- Tier 4
ALTER TABLE server_cookbook_autocorrect_previews
    ALTER COLUMN organisation_name SET NOT NULL,
    ALTER COLUMN cookbook_name SET NOT NULL,
    ALTER COLUMN cookbook_version SET NOT NULL,
    ALTER COLUMN target_chef_version SET NOT NULL;

ALTER TABLE git_repo_autocorrect_previews
    ALTER COLUMN git_repo_name SET NOT NULL,
    ALTER COLUMN git_repo_url SET NOT NULL,
    ALTER COLUMN target_chef_version SET NOT NULL;


-- ===========================================================================
-- PHASE D: Drop existing FK constraints, UNIQUE constraints, and indexes
-- ===========================================================================

-- ---------------------------------------------------------------------------
-- D.1: Drop FOREIGN KEY constraints (children before parents)
-- ---------------------------------------------------------------------------

-- Tier 4
ALTER TABLE server_cookbook_autocorrect_previews
    DROP CONSTRAINT IF EXISTS server_cookbook_autocorrect_previews_server_cookbook_id_fkey,
    DROP CONSTRAINT IF EXISTS server_cookbook_autocorrect_previews_cookstyle_result_id_fkey;

ALTER TABLE git_repo_autocorrect_previews
    DROP CONSTRAINT IF EXISTS git_repo_autocorrect_previews_git_repo_id_fkey,
    DROP CONSTRAINT IF EXISTS git_repo_autocorrect_previews_cookstyle_result_id_fkey;

-- Tier 3
ALTER TABLE node_readiness
    DROP CONSTRAINT IF EXISTS node_readiness_node_snapshot_id_fkey,
    DROP CONSTRAINT IF EXISTS node_readiness_organisation_id_fkey;

ALTER TABLE server_cookbook_cookstyle_results
    DROP CONSTRAINT IF EXISTS server_cookbook_cookstyle_results_server_cookbook_id_fkey;

ALTER TABLE server_cookbook_complexity
    DROP CONSTRAINT IF EXISTS server_cookbook_complexity_server_cookbook_id_fkey;

ALTER TABLE git_repo_cookstyle_results
    DROP CONSTRAINT IF EXISTS git_repo_cookstyle_results_git_repo_id_fkey;

ALTER TABLE git_repo_complexity
    DROP CONSTRAINT IF EXISTS git_repo_complexity_git_repo_id_fkey;

ALTER TABLE git_repo_test_kitchen_results
    DROP CONSTRAINT IF EXISTS git_repo_test_kitchen_results_git_repo_id_fkey;

ALTER TABLE cookbook_usage_detail
    DROP CONSTRAINT IF EXISTS cookbook_usage_detail_analysis_id_fkey,
    DROP CONSTRAINT IF EXISTS cookbook_usage_detail_organisation_id_fkey;

ALTER TABLE cookbook_platform_coverage
    DROP CONSTRAINT IF EXISTS fk_cookbook_platform_coverage_git_repo;

-- Tier 2
ALTER TABLE collection_runs
    DROP CONSTRAINT IF EXISTS collection_runs_organisation_id_fkey;

ALTER TABLE server_cookbooks
    DROP CONSTRAINT IF EXISTS server_cookbooks_organisation_id_fkey;

ALTER TABLE node_snapshots
    DROP CONSTRAINT IF EXISTS node_snapshots_collection_run_id_fkey,
    DROP CONSTRAINT IF EXISTS node_snapshots_organisation_id_fkey;

ALTER TABLE sessions
    DROP CONSTRAINT IF EXISTS sessions_user_id_fkey;

ALTER TABLE ownership_assignments
    DROP CONSTRAINT IF EXISTS ownership_assignments_owner_id_fkey,
    DROP CONSTRAINT IF EXISTS ownership_assignments_organisation_id_fkey;

ALTER TABLE role_dependencies
    DROP CONSTRAINT IF EXISTS role_dependencies_organisation_id_fkey;

ALTER TABLE metric_snapshots
    DROP CONSTRAINT IF EXISTS metric_snapshots_collection_run_id_fkey,
    DROP CONSTRAINT IF EXISTS metric_snapshots_organisation_id_fkey;

ALTER TABLE cookbook_usage_analysis
    DROP CONSTRAINT IF EXISTS cookbook_usage_analysis_organisation_id_fkey,
    DROP CONSTRAINT IF EXISTS cookbook_usage_analysis_collection_run_id_fkey;

ALTER TABLE log_entries
    DROP CONSTRAINT IF EXISTS log_entries_collection_run_id_fkey;

-- Tier 1
ALTER TABLE organisations
    DROP CONSTRAINT IF EXISTS organisations_client_key_credential_id_fkey;

-- ---------------------------------------------------------------------------
-- D.2: Drop UNIQUE constraints
-- ---------------------------------------------------------------------------

ALTER TABLE credentials
    DROP CONSTRAINT IF EXISTS uq_credentials_name,
    DROP CONSTRAINT IF EXISTS uq_credentials_type_name;

ALTER TABLE organisations
    DROP CONSTRAINT IF EXISTS uq_organisations_name,
    DROP CONSTRAINT IF EXISTS uq_organisations_server_org;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS uq_users_username;

ALTER TABLE owners
    DROP CONSTRAINT IF EXISTS owners_name_unique;

ALTER TABLE git_repos
    DROP CONSTRAINT IF EXISTS git_repos_name_key;

ALTER TABLE collection_runs
    DROP CONSTRAINT IF EXISTS uq_collection_runs_org;

ALTER TABLE server_cookbooks
    DROP CONSTRAINT IF EXISTS server_cookbooks_organisation_id_name_version_key;

ALTER TABLE node_snapshots
    DROP CONSTRAINT IF EXISTS uq_node_snapshots_org_node;

ALTER TABLE server_cookbook_cookstyle_results
    DROP CONSTRAINT IF EXISTS server_cookbook_cookstyle_results_server_cookbook_id_target_c_key;

ALTER TABLE server_cookbook_complexity
    DROP CONSTRAINT IF EXISTS uq_sc_cookbook_complexity;

ALTER TABLE git_repo_cookstyle_results
    DROP CONSTRAINT IF EXISTS git_repo_cookstyle_results_git_repo_id_target_chef_version_key;

ALTER TABLE git_repo_complexity
    DROP CONSTRAINT IF EXISTS uq_gr_cookbook_complexity;

ALTER TABLE git_repo_test_kitchen_results
    DROP CONSTRAINT IF EXISTS uq_git_repo_test_kitchen_results;

ALTER TABLE server_cookbook_autocorrect_previews
    DROP CONSTRAINT IF EXISTS uq_sc_autocorrect_previews_cookstyle;

ALTER TABLE git_repo_autocorrect_previews
    DROP CONSTRAINT IF EXISTS uq_gr_autocorrect_previews_cookstyle;

ALTER TABLE node_readiness
    DROP CONSTRAINT IF EXISTS uq_node_readiness;

ALTER TABLE cookbook_usage_analysis
    DROP CONSTRAINT IF EXISTS uq_cookbook_usage_analysis_org;

ALTER TABLE cookbook_platform_coverage
    DROP CONSTRAINT IF EXISTS uq_cookbook_platform_coverage_name;

ALTER TABLE role_dependencies
    DROP CONSTRAINT IF EXISTS uq_role_dependencies;

ALTER TABLE git_repo_committers
    DROP CONSTRAINT IF EXISTS git_repo_committers_unique;

-- ---------------------------------------------------------------------------
-- D.3: Drop UNIQUE indexes (those created as CREATE UNIQUE INDEX)
-- ---------------------------------------------------------------------------

DROP INDEX IF EXISTS idx_ownership_assignments_unique;

-- ---------------------------------------------------------------------------
-- D.4: Drop all indexes that reference UUID columns being removed
-- ---------------------------------------------------------------------------

-- credentials
DROP INDEX IF EXISTS idx_credentials_name;
DROP INDEX IF EXISTS idx_credentials_credential_type;

-- organisations
DROP INDEX IF EXISTS idx_organisations_name;
DROP INDEX IF EXISTS idx_organisations_client_key_credential_id;

-- collection_runs
DROP INDEX IF EXISTS idx_collection_runs_organisation_id;
DROP INDEX IF EXISTS idx_collection_runs_status;
DROP INDEX IF EXISTS idx_collection_runs_started_at;
DROP INDEX IF EXISTS idx_collection_runs_org_status_started;
DROP INDEX IF EXISTS idx_collection_runs_org_completed_started;

-- node_snapshots
DROP INDEX IF EXISTS idx_node_snapshots_collection_run_id;
DROP INDEX IF EXISTS idx_node_snapshots_organisation_id;
DROP INDEX IF EXISTS idx_node_snapshots_node_name;
DROP INDEX IF EXISTS idx_node_snapshots_chef_version;
DROP INDEX IF EXISTS idx_node_snapshots_platform;
DROP INDEX IF EXISTS idx_node_snapshots_platform_family;
DROP INDEX IF EXISTS idx_node_snapshots_chef_environment;
DROP INDEX IF EXISTS idx_node_snapshots_collected_at;
DROP INDEX IF EXISTS idx_node_snapshots_policy_name;
DROP INDEX IF EXISTS idx_node_snapshots_policy_group;
DROP INDEX IF EXISTS idx_node_snapshots_is_stale;
DROP INDEX IF EXISTS idx_node_snapshots_org_name_collected;
DROP INDEX IF EXISTS idx_node_snapshots_roles_gin;
DROP INDEX IF EXISTS idx_node_snapshots_platform_combined;
DROP INDEX IF EXISTS idx_node_snapshots_node_name_lower;
DROP INDEX IF EXISTS idx_node_snapshots_chef_environment_lower;
DROP INDEX IF EXISTS idx_node_snapshots_chef_version_lower;
DROP INDEX IF EXISTS idx_node_snapshots_policy_name_lower;
DROP INDEX IF EXISTS idx_node_snapshots_policy_group_lower;

-- server_cookbooks
DROP INDEX IF EXISTS idx_server_cookbooks_organisation_id;
DROP INDEX IF EXISTS idx_server_cookbooks_name;
DROP INDEX IF EXISTS idx_server_cookbooks_is_active;
DROP INDEX IF EXISTS idx_server_cookbooks_is_stale_cookbook;
DROP INDEX IF EXISTS idx_server_cookbooks_name_version;
DROP INDEX IF EXISTS idx_server_cookbooks_first_seen_at;
DROP INDEX IF EXISTS idx_server_cookbooks_download_status;

-- git_repos
DROP INDEX IF EXISTS idx_git_repos_name;
DROP INDEX IF EXISTS idx_git_repos_git_repo_url;
DROP INDEX IF EXISTS idx_git_repos_clone_status;

-- server_cookbook_cookstyle_results
DROP INDEX IF EXISTS idx_sc_cookstyle_results_server_cookbook_id;
DROP INDEX IF EXISTS idx_sc_cookstyle_results_target_chef_version;
DROP INDEX IF EXISTS idx_sc_cookstyle_results_passed;

-- git_repo_cookstyle_results
DROP INDEX IF EXISTS idx_gr_cookstyle_results_git_repo_id;
DROP INDEX IF EXISTS idx_gr_cookstyle_results_target_chef_version;
DROP INDEX IF EXISTS idx_gr_cookstyle_results_passed;
DROP INDEX IF EXISTS idx_gr_cookstyle_results_commit_sha;

-- git_repo_test_kitchen_results
DROP INDEX IF EXISTS idx_gr_test_kitchen_results_git_repo_id;
DROP INDEX IF EXISTS idx_gr_test_kitchen_results_target_chef_version;
DROP INDEX IF EXISTS idx_gr_test_kitchen_results_commit_sha;
DROP INDEX IF EXISTS idx_gr_test_kitchen_results_compatible;
DROP INDEX IF EXISTS idx_gr_test_kitchen_results_repo_target;

-- server_cookbook_autocorrect_previews
DROP INDEX IF EXISTS idx_sc_autocorrect_previews_server_cookbook_id;
DROP INDEX IF EXISTS idx_sc_autocorrect_previews_cookstyle_result_id;

-- git_repo_autocorrect_previews
DROP INDEX IF EXISTS idx_gr_autocorrect_previews_git_repo_id;
DROP INDEX IF EXISTS idx_gr_autocorrect_previews_cookstyle_result_id;

-- server_cookbook_complexity
DROP INDEX IF EXISTS idx_sc_complexity_server_cookbook_id;
DROP INDEX IF EXISTS idx_sc_complexity_target_chef_version;
DROP INDEX IF EXISTS idx_sc_complexity_score;
DROP INDEX IF EXISTS idx_sc_complexity_label;
DROP INDEX IF EXISTS idx_sc_complexity_affected_node_count;

-- git_repo_complexity
DROP INDEX IF EXISTS idx_gr_complexity_git_repo_id;
DROP INDEX IF EXISTS idx_gr_complexity_target_chef_version;
DROP INDEX IF EXISTS idx_gr_complexity_score;
DROP INDEX IF EXISTS idx_gr_complexity_label;
DROP INDEX IF EXISTS idx_gr_complexity_affected_node_count;

-- node_readiness
DROP INDEX IF EXISTS idx_node_readiness_node_snapshot_id;
DROP INDEX IF EXISTS idx_node_readiness_organisation_id;
DROP INDEX IF EXISTS idx_node_readiness_target_chef_version;
DROP INDEX IF EXISTS idx_node_readiness_is_ready;
DROP INDEX IF EXISTS idx_node_readiness_stale_data;
DROP INDEX IF EXISTS idx_node_readiness_node_name;
DROP INDEX IF EXISTS idx_node_readiness_latest;
DROP INDEX IF EXISTS idx_node_readiness_target_name_eval;

-- role_dependencies
DROP INDEX IF EXISTS idx_role_dependencies_organisation_id;
DROP INDEX IF EXISTS idx_role_dependencies_role_name;
DROP INDEX IF EXISTS idx_role_dependencies_dependency_type;
DROP INDEX IF EXISTS idx_role_dependencies_dependency_name;

-- metric_snapshots
DROP INDEX IF EXISTS idx_metric_snapshots_organisation_id;
DROP INDEX IF EXISTS idx_metric_snapshots_snapshot_type;
DROP INDEX IF EXISTS idx_metric_snapshots_snapshot_at;
DROP INDEX IF EXISTS idx_metric_snapshots_target_chef_version;

-- log_entries
DROP INDEX IF EXISTS idx_log_entries_timestamp;
DROP INDEX IF EXISTS idx_log_entries_severity;
DROP INDEX IF EXISTS idx_log_entries_scope;
DROP INDEX IF EXISTS idx_log_entries_organisation;
DROP INDEX IF EXISTS idx_log_entries_cookbook_name;
DROP INDEX IF EXISTS idx_log_entries_collection_run_id;
DROP INDEX IF EXISTS idx_log_entries_retention;

-- sessions
DROP INDEX IF EXISTS idx_sessions_user_id;
DROP INDEX IF EXISTS idx_sessions_expires_at;

-- users
DROP INDEX IF EXISTS idx_users_username;
DROP INDEX IF EXISTS idx_users_auth_provider;

-- owners
DROP INDEX IF EXISTS idx_owners_owner_type;

-- ownership_assignments
DROP INDEX IF EXISTS idx_ownership_assignments_owner_id;
DROP INDEX IF EXISTS idx_ownership_assignments_entity;
DROP INDEX IF EXISTS idx_ownership_assignments_org;
DROP INDEX IF EXISTS idx_ownership_assignments_source;
DROP INDEX IF EXISTS idx_ownership_assignments_auto_rule;

-- git_repo_committers
DROP INDEX IF EXISTS idx_git_repo_committers_repo;

-- ownership_audit_log
DROP INDEX IF EXISTS idx_ownership_audit_log_timestamp;
DROP INDEX IF EXISTS idx_ownership_audit_log_action;
DROP INDEX IF EXISTS idx_ownership_audit_log_owner;
DROP INDEX IF EXISTS idx_ownership_audit_log_actor;
DROP INDEX IF EXISTS idx_ownership_audit_log_entity;

-- cookbook_usage_analysis
DROP INDEX IF EXISTS idx_cookbook_usage_analysis_organisation_id;
DROP INDEX IF EXISTS idx_cookbook_usage_analysis_collection_run_id;
DROP INDEX IF EXISTS idx_cookbook_usage_analysis_analysed_at;

-- cookbook_usage_detail
DROP INDEX IF EXISTS idx_cookbook_usage_detail_analysis_id;
DROP INDEX IF EXISTS idx_cookbook_usage_detail_organisation_id;
DROP INDEX IF EXISTS idx_cookbook_usage_detail_cookbook_name;
DROP INDEX IF EXISTS idx_cookbook_usage_detail_cookbook_name_version;
DROP INDEX IF EXISTS idx_cookbook_usage_detail_is_active;
DROP INDEX IF EXISTS idx_cookbook_usage_detail_node_count;

-- export_jobs (no UUID FK indexes to drop, but no changes needed)

-- cookbook_platform_coverage (no separate indexes were created in 0008)


-- ===========================================================================
-- PHASE E: Drop UUID columns (old PKs and old FK columns)
-- ===========================================================================

-- ---------------------------------------------------------------------------
-- E.1: Drop PRIMARY KEY constraints (must drop PK before dropping column)
-- ---------------------------------------------------------------------------

-- Tier 4
ALTER TABLE server_cookbook_autocorrect_previews DROP CONSTRAINT server_cookbook_autocorrect_previews_pkey;
ALTER TABLE git_repo_autocorrect_previews DROP CONSTRAINT git_repo_autocorrect_previews_pkey;

-- Tier 3
ALTER TABLE node_readiness DROP CONSTRAINT node_readiness_pkey;
ALTER TABLE server_cookbook_cookstyle_results DROP CONSTRAINT server_cookbook_cookstyle_results_pkey;
ALTER TABLE server_cookbook_complexity DROP CONSTRAINT server_cookbook_complexity_pkey;
ALTER TABLE git_repo_cookstyle_results DROP CONSTRAINT git_repo_cookstyle_results_pkey;
ALTER TABLE git_repo_complexity DROP CONSTRAINT git_repo_complexity_pkey;
ALTER TABLE git_repo_test_kitchen_results DROP CONSTRAINT git_repo_test_kitchen_results_pkey;
ALTER TABLE cookbook_usage_detail DROP CONSTRAINT cookbook_usage_detail_pkey;
ALTER TABLE cookbook_platform_coverage DROP CONSTRAINT cookbook_platform_coverage_pkey;

-- Tier 2
ALTER TABLE collection_runs DROP CONSTRAINT collection_runs_pkey;
ALTER TABLE server_cookbooks DROP CONSTRAINT server_cookbooks_pkey;
ALTER TABLE node_snapshots DROP CONSTRAINT node_snapshots_pkey;
ALTER TABLE ownership_assignments DROP CONSTRAINT ownership_assignments_pkey;
ALTER TABLE role_dependencies DROP CONSTRAINT role_dependencies_pkey;
ALTER TABLE metric_snapshots DROP CONSTRAINT metric_snapshots_pkey;
ALTER TABLE cookbook_usage_analysis DROP CONSTRAINT cookbook_usage_analysis_pkey;
ALTER TABLE log_entries DROP CONSTRAINT log_entries_pkey;
ALTER TABLE git_repo_committers DROP CONSTRAINT git_repo_committers_pkey;
ALTER TABLE ownership_audit_log DROP CONSTRAINT ownership_audit_log_pkey;

-- Tier 1
ALTER TABLE credentials DROP CONSTRAINT credentials_pkey;
ALTER TABLE organisations DROP CONSTRAINT organisations_pkey;
ALTER TABLE users DROP CONSTRAINT users_pkey;
ALTER TABLE owners DROP CONSTRAINT owners_pkey;
ALTER TABLE git_repos DROP CONSTRAINT git_repos_pkey;

-- ---------------------------------------------------------------------------
-- E.2: Drop UUID columns — Tier 4 (deepest children first)
-- ---------------------------------------------------------------------------

ALTER TABLE server_cookbook_autocorrect_previews
    DROP COLUMN id,
    DROP COLUMN server_cookbook_id,
    DROP COLUMN cookstyle_result_id;

ALTER TABLE git_repo_autocorrect_previews
    DROP COLUMN id,
    DROP COLUMN git_repo_id,
    DROP COLUMN cookstyle_result_id;

-- ---------------------------------------------------------------------------
-- E.3: Drop UUID columns — Tier 3
-- ---------------------------------------------------------------------------

ALTER TABLE node_readiness
    DROP COLUMN id,
    DROP COLUMN node_snapshot_id,
    DROP COLUMN organisation_id;

ALTER TABLE server_cookbook_cookstyle_results
    DROP COLUMN id,
    DROP COLUMN server_cookbook_id;

ALTER TABLE server_cookbook_complexity
    DROP COLUMN id,
    DROP COLUMN server_cookbook_id;

ALTER TABLE git_repo_cookstyle_results
    DROP COLUMN id,
    DROP COLUMN git_repo_id;

ALTER TABLE git_repo_complexity
    DROP COLUMN id,
    DROP COLUMN git_repo_id;

ALTER TABLE git_repo_test_kitchen_results
    DROP COLUMN id,
    DROP COLUMN git_repo_id;

ALTER TABLE cookbook_usage_detail
    DROP COLUMN id,
    DROP COLUMN analysis_id,
    DROP COLUMN organisation_id;

ALTER TABLE cookbook_platform_coverage
    DROP COLUMN id,
    DROP COLUMN git_repo_id;

-- ---------------------------------------------------------------------------
-- E.4: Drop UUID columns — Tier 2
-- ---------------------------------------------------------------------------

ALTER TABLE collection_runs
    DROP COLUMN id,
    DROP COLUMN organisation_id;

ALTER TABLE server_cookbooks
    DROP COLUMN id,
    DROP COLUMN organisation_id;

ALTER TABLE node_snapshots
    DROP COLUMN id,
    DROP COLUMN collection_run_id,
    DROP COLUMN organisation_id;

-- sessions: drop user_id FK column only, keep UUID PK
ALTER TABLE sessions
    DROP COLUMN user_id;

ALTER TABLE ownership_assignments
    DROP COLUMN id,
    DROP COLUMN owner_id,
    DROP COLUMN organisation_id;

ALTER TABLE role_dependencies
    DROP COLUMN id,
    DROP COLUMN organisation_id;

-- metric_snapshots: drop old UUID id and FK columns, will add BIGSERIAL
ALTER TABLE metric_snapshots
    DROP COLUMN id,
    DROP COLUMN collection_run_id,
    DROP COLUMN organisation_id;

ALTER TABLE cookbook_usage_analysis
    DROP COLUMN id,
    DROP COLUMN organisation_id,
    DROP COLUMN collection_run_id;

-- log_entries: drop old UUID id and FK column, will add BIGSERIAL
ALTER TABLE log_entries
    DROP COLUMN id,
    DROP COLUMN collection_run_id;

ALTER TABLE git_repo_committers
    DROP COLUMN id;

-- ownership_audit_log: drop old UUID id, will add BIGSERIAL
ALTER TABLE ownership_audit_log
    DROP COLUMN id;

-- ---------------------------------------------------------------------------
-- E.5: Drop UUID columns — Tier 1
-- ---------------------------------------------------------------------------

ALTER TABLE credentials
    DROP COLUMN id;

ALTER TABLE organisations
    DROP COLUMN id,
    DROP COLUMN client_key_credential_id;

ALTER TABLE users
    DROP COLUMN id;

ALTER TABLE owners
    DROP COLUMN id;

ALTER TABLE git_repos
    DROP COLUMN id;


-- ===========================================================================
-- PHASE F: Add new PRIMARY KEY constraints
-- ===========================================================================

-- ---------------------------------------------------------------------------
-- F.1: Tier 1 — Root entities
-- ---------------------------------------------------------------------------

ALTER TABLE credentials
    ADD PRIMARY KEY (name);

ALTER TABLE organisations
    ADD PRIMARY KEY (name);

ALTER TABLE users
    ADD PRIMARY KEY (username);

ALTER TABLE owners
    ADD PRIMARY KEY (name);

ALTER TABLE git_repos
    ADD PRIMARY KEY (name, git_repo_url);

-- ---------------------------------------------------------------------------
-- F.2: Tier 2
-- ---------------------------------------------------------------------------

ALTER TABLE collection_runs
    ADD PRIMARY KEY (organisation_name);

ALTER TABLE server_cookbooks
    ADD PRIMARY KEY (organisation_name, name, version);

ALTER TABLE node_snapshots
    ADD PRIMARY KEY (organisation_name, node_name);

-- sessions: PK unchanged (UUID)

-- ownership_assignments: nullable organisation_name prevents composite PK.
-- Use BIGSERIAL surrogate + UNIQUE index.
ALTER TABLE ownership_assignments
    ADD COLUMN id BIGSERIAL;
ALTER TABLE ownership_assignments
    ADD PRIMARY KEY (id);

ALTER TABLE role_dependencies
    ADD PRIMARY KEY (organisation_name, role_name, dependency_type, dependency_name);

-- metric_snapshots: BIGSERIAL PK
ALTER TABLE metric_snapshots
    ADD COLUMN id BIGSERIAL;
ALTER TABLE metric_snapshots
    ADD PRIMARY KEY (id);

ALTER TABLE cookbook_usage_analysis
    ADD PRIMARY KEY (organisation_name);

-- log_entries: BIGSERIAL PK
ALTER TABLE log_entries
    ADD COLUMN id BIGSERIAL;
ALTER TABLE log_entries
    ADD PRIMARY KEY (id);

ALTER TABLE git_repo_committers
    ADD PRIMARY KEY (git_repo_url, author_email);

-- ownership_audit_log: BIGSERIAL PK
ALTER TABLE ownership_audit_log
    ADD COLUMN id BIGSERIAL;
ALTER TABLE ownership_audit_log
    ADD PRIMARY KEY (id);

-- ---------------------------------------------------------------------------
-- F.3: Tier 3
-- ---------------------------------------------------------------------------

ALTER TABLE node_readiness
    ADD PRIMARY KEY (organisation_name, node_name, target_chef_version);

ALTER TABLE server_cookbook_cookstyle_results
    ADD PRIMARY KEY (organisation_name, cookbook_name, cookbook_version, target_chef_version);

ALTER TABLE server_cookbook_complexity
    ADD PRIMARY KEY (organisation_name, cookbook_name, cookbook_version, target_chef_version);

ALTER TABLE git_repo_cookstyle_results
    ADD PRIMARY KEY (git_repo_name, git_repo_url, target_chef_version);

ALTER TABLE git_repo_complexity
    ADD PRIMARY KEY (git_repo_name, git_repo_url, target_chef_version);

ALTER TABLE git_repo_test_kitchen_results
    ADD PRIMARY KEY (git_repo_name, git_repo_url, target_chef_version);

ALTER TABLE cookbook_usage_detail
    ADD PRIMARY KEY (organisation_name, cookbook_name, cookbook_version);

ALTER TABLE cookbook_platform_coverage
    ADD PRIMARY KEY (cookbook_name);

-- ---------------------------------------------------------------------------
-- F.4: Tier 4
-- ---------------------------------------------------------------------------

ALTER TABLE server_cookbook_autocorrect_previews
    ADD PRIMARY KEY (organisation_name, cookbook_name, cookbook_version, target_chef_version);

ALTER TABLE git_repo_autocorrect_previews
    ADD PRIMARY KEY (git_repo_name, git_repo_url, target_chef_version);


-- ===========================================================================
-- PHASE G: Add new FOREIGN KEY constraints
-- ===========================================================================

-- ---------------------------------------------------------------------------
-- G.1: Tier 1 cross-references
-- ---------------------------------------------------------------------------

ALTER TABLE organisations
    ADD CONSTRAINT fk_organisations_credential
        FOREIGN KEY (client_key_credential_name) REFERENCES credentials(name) ON DELETE SET NULL;

-- ---------------------------------------------------------------------------
-- G.2: Tier 2
-- ---------------------------------------------------------------------------

ALTER TABLE collection_runs
    ADD CONSTRAINT fk_collection_runs_organisation
        FOREIGN KEY (organisation_name) REFERENCES organisations(name) ON DELETE CASCADE;

ALTER TABLE server_cookbooks
    ADD CONSTRAINT fk_server_cookbooks_organisation
        FOREIGN KEY (organisation_name) REFERENCES organisations(name) ON DELETE CASCADE;

ALTER TABLE node_snapshots
    ADD CONSTRAINT fk_node_snapshots_organisation
        FOREIGN KEY (organisation_name) REFERENCES organisations(name) ON DELETE CASCADE;
-- node_snapshots.collection_run_org — no FK (collection_runs may be purged)

ALTER TABLE sessions
    ADD CONSTRAINT fk_sessions_user
        FOREIGN KEY (username) REFERENCES users(username) ON DELETE CASCADE;

ALTER TABLE ownership_assignments
    ADD CONSTRAINT fk_ownership_assignments_owner
        FOREIGN KEY (owner_name) REFERENCES owners(name) ON DELETE CASCADE;
-- ownership_assignments.organisation_name — no FK to preserve nullable semantics cleanly

ALTER TABLE role_dependencies
    ADD CONSTRAINT fk_role_dependencies_organisation
        FOREIGN KEY (organisation_name) REFERENCES organisations(name) ON DELETE CASCADE;

ALTER TABLE metric_snapshots
    ADD CONSTRAINT fk_metric_snapshots_organisation
        FOREIGN KEY (organisation_name) REFERENCES organisations(name) ON DELETE CASCADE;
-- metric_snapshots.collection_run_org — no FK (collection_runs may be purged)

ALTER TABLE cookbook_usage_analysis
    ADD CONSTRAINT fk_cookbook_usage_analysis_organisation
        FOREIGN KEY (organisation_name) REFERENCES organisations(name) ON DELETE CASCADE;
-- cookbook_usage_analysis.collection_run_org — no FK

-- log_entries.collection_run_org — no FK constraint (collection_runs may be purged)

-- ---------------------------------------------------------------------------
-- G.3: Tier 3
-- ---------------------------------------------------------------------------

ALTER TABLE node_readiness
    ADD CONSTRAINT fk_node_readiness_node_snapshot
        FOREIGN KEY (organisation_name, node_name) REFERENCES node_snapshots(organisation_name, node_name) ON DELETE CASCADE;

ALTER TABLE server_cookbook_cookstyle_results
    ADD CONSTRAINT fk_sc_cookstyle_results_cookbook
        FOREIGN KEY (organisation_name, cookbook_name, cookbook_version)
        REFERENCES server_cookbooks(organisation_name, name, version) ON DELETE CASCADE;

ALTER TABLE server_cookbook_complexity
    ADD CONSTRAINT fk_sc_complexity_cookbook
        FOREIGN KEY (organisation_name, cookbook_name, cookbook_version)
        REFERENCES server_cookbooks(organisation_name, name, version) ON DELETE CASCADE;

ALTER TABLE git_repo_cookstyle_results
    ADD CONSTRAINT fk_gr_cookstyle_results_repo
        FOREIGN KEY (git_repo_name, git_repo_url) REFERENCES git_repos(name, git_repo_url) ON DELETE CASCADE;

ALTER TABLE git_repo_complexity
    ADD CONSTRAINT fk_gr_complexity_repo
        FOREIGN KEY (git_repo_name, git_repo_url) REFERENCES git_repos(name, git_repo_url) ON DELETE CASCADE;

ALTER TABLE git_repo_test_kitchen_results
    ADD CONSTRAINT fk_gr_test_kitchen_results_repo
        FOREIGN KEY (git_repo_name, git_repo_url) REFERENCES git_repos(name, git_repo_url) ON DELETE CASCADE;

ALTER TABLE cookbook_usage_detail
    ADD CONSTRAINT fk_cookbook_usage_detail_analysis
        FOREIGN KEY (organisation_name) REFERENCES cookbook_usage_analysis(organisation_name) ON DELETE CASCADE;

ALTER TABLE cookbook_platform_coverage
    ADD CONSTRAINT fk_cookbook_platform_coverage_repo
        FOREIGN KEY (git_repo_name, git_repo_url) REFERENCES git_repos(name, git_repo_url) ON DELETE CASCADE;

-- ---------------------------------------------------------------------------
-- G.4: Tier 4
-- ---------------------------------------------------------------------------

ALTER TABLE server_cookbook_autocorrect_previews
    ADD CONSTRAINT fk_sc_autocorrect_cookbook
        FOREIGN KEY (organisation_name, cookbook_name, cookbook_version)
        REFERENCES server_cookbooks(organisation_name, name, version) ON DELETE CASCADE;

ALTER TABLE server_cookbook_autocorrect_previews
    ADD CONSTRAINT fk_sc_autocorrect_cookstyle
        FOREIGN KEY (organisation_name, cookbook_name, cookbook_version, target_chef_version)
        REFERENCES server_cookbook_cookstyle_results(organisation_name, cookbook_name, cookbook_version, target_chef_version) ON DELETE CASCADE;

ALTER TABLE git_repo_autocorrect_previews
    ADD CONSTRAINT fk_gr_autocorrect_repo
        FOREIGN KEY (git_repo_name, git_repo_url)
        REFERENCES git_repos(name, git_repo_url) ON DELETE CASCADE;

ALTER TABLE git_repo_autocorrect_previews
    ADD CONSTRAINT fk_gr_autocorrect_cookstyle
        FOREIGN KEY (git_repo_name, git_repo_url, target_chef_version)
        REFERENCES git_repo_cookstyle_results(git_repo_name, git_repo_url, target_chef_version) ON DELETE CASCADE;


-- ===========================================================================
-- PHASE H: Recreate indexes
-- ===========================================================================

-- ---------------------------------------------------------------------------
-- H.1: credentials
-- ---------------------------------------------------------------------------
CREATE INDEX idx_credentials_credential_type ON credentials (credential_type);
-- name is now PK, no separate index needed

-- ---------------------------------------------------------------------------
-- H.2: organisations
-- ---------------------------------------------------------------------------
CREATE UNIQUE INDEX idx_organisations_server_org ON organisations (chef_server_url, org_name);
CREATE INDEX idx_organisations_client_key_credential_name ON organisations (client_key_credential_name);
-- name is now PK, no separate index needed

-- ---------------------------------------------------------------------------
-- H.3: collection_runs
-- ---------------------------------------------------------------------------
-- organisation_name is PK, covers equality lookups
CREATE INDEX idx_collection_runs_status ON collection_runs (status);
CREATE INDEX idx_collection_runs_started_at ON collection_runs (started_at);
CREATE INDEX idx_collection_runs_org_status_started
    ON collection_runs (organisation_name, status, started_at DESC);
CREATE INDEX idx_collection_runs_org_completed_started
    ON collection_runs (organisation_name, started_at DESC)
    WHERE status = 'completed';

-- ---------------------------------------------------------------------------
-- H.4: node_snapshots
-- ---------------------------------------------------------------------------
-- PK is (organisation_name, node_name)
CREATE INDEX idx_node_snapshots_node_name ON node_snapshots (node_name);
CREATE INDEX idx_node_snapshots_chef_version ON node_snapshots (chef_version);
CREATE INDEX idx_node_snapshots_platform ON node_snapshots (platform, platform_version);
CREATE INDEX idx_node_snapshots_platform_family ON node_snapshots (platform_family);
CREATE INDEX idx_node_snapshots_chef_environment ON node_snapshots (chef_environment);
CREATE INDEX idx_node_snapshots_collected_at ON node_snapshots (collected_at);
CREATE INDEX idx_node_snapshots_policy_name ON node_snapshots (policy_name);
CREATE INDEX idx_node_snapshots_policy_group ON node_snapshots (policy_group);
CREATE INDEX idx_node_snapshots_is_stale ON node_snapshots (is_stale);
CREATE INDEX idx_node_snapshots_org_name_collected
    ON node_snapshots (organisation_name, node_name, collected_at DESC);
CREATE INDEX idx_node_snapshots_collection_run_org ON node_snapshots (collection_run_org);
-- Filter push-down indexes (from migration 0002)
CREATE INDEX idx_node_snapshots_roles_gin
    ON node_snapshots USING GIN (roles jsonb_path_ops);
CREATE INDEX idx_node_snapshots_platform_combined
    ON node_snapshots (LOWER(platform || ' ' || COALESCE(platform_version, '')));
CREATE INDEX idx_node_snapshots_node_name_lower
    ON node_snapshots (LOWER(node_name));
CREATE INDEX idx_node_snapshots_chef_environment_lower
    ON node_snapshots (LOWER(chef_environment));
CREATE INDEX idx_node_snapshots_chef_version_lower
    ON node_snapshots (LOWER(chef_version));
CREATE INDEX idx_node_snapshots_policy_name_lower
    ON node_snapshots (LOWER(policy_name));
CREATE INDEX idx_node_snapshots_policy_group_lower
    ON node_snapshots (LOWER(policy_group));

-- ---------------------------------------------------------------------------
-- H.5: server_cookbooks
-- ---------------------------------------------------------------------------
-- PK is (organisation_name, name, version)
CREATE INDEX idx_server_cookbooks_organisation_name ON server_cookbooks (organisation_name);
CREATE INDEX idx_server_cookbooks_name ON server_cookbooks (name);
CREATE INDEX idx_server_cookbooks_is_active ON server_cookbooks (is_active);
CREATE INDEX idx_server_cookbooks_is_stale_cookbook ON server_cookbooks (is_stale_cookbook);
CREATE INDEX idx_server_cookbooks_name_version ON server_cookbooks (name, version);
CREATE INDEX idx_server_cookbooks_first_seen_at ON server_cookbooks (first_seen_at);
CREATE INDEX idx_server_cookbooks_download_status ON server_cookbooks (download_status);

-- ---------------------------------------------------------------------------
-- H.6: git_repos
-- ---------------------------------------------------------------------------
-- PK is (name, git_repo_url)
CREATE INDEX idx_git_repos_name ON git_repos (name);
CREATE INDEX idx_git_repos_git_repo_url ON git_repos (git_repo_url);
CREATE INDEX idx_git_repos_clone_status ON git_repos (clone_status);

-- ---------------------------------------------------------------------------
-- H.7: server_cookbook_cookstyle_results
-- ---------------------------------------------------------------------------
-- PK is (organisation_name, cookbook_name, cookbook_version, target_chef_version)
CREATE INDEX idx_sc_cookstyle_results_target_chef_version ON server_cookbook_cookstyle_results (target_chef_version);
CREATE INDEX idx_sc_cookstyle_results_passed ON server_cookbook_cookstyle_results (passed);
CREATE INDEX idx_sc_cookstyle_results_org_cookbook
    ON server_cookbook_cookstyle_results (organisation_name, cookbook_name, cookbook_version);

-- ---------------------------------------------------------------------------
-- H.8: git_repo_cookstyle_results
-- ---------------------------------------------------------------------------
-- PK is (git_repo_name, git_repo_url, target_chef_version)
CREATE INDEX idx_gr_cookstyle_results_target_chef_version ON git_repo_cookstyle_results (target_chef_version);
CREATE INDEX idx_gr_cookstyle_results_passed ON git_repo_cookstyle_results (passed);
CREATE INDEX idx_gr_cookstyle_results_commit_sha ON git_repo_cookstyle_results (commit_sha)
    WHERE commit_sha IS NOT NULL;

-- ---------------------------------------------------------------------------
-- H.9: git_repo_test_kitchen_results
-- ---------------------------------------------------------------------------
-- PK is (git_repo_name, git_repo_url, target_chef_version)
CREATE INDEX idx_gr_test_kitchen_results_target_chef_version ON git_repo_test_kitchen_results (target_chef_version);
CREATE INDEX idx_gr_test_kitchen_results_commit_sha ON git_repo_test_kitchen_results (commit_sha);
CREATE INDEX idx_gr_test_kitchen_results_compatible ON git_repo_test_kitchen_results (compatible);

-- ---------------------------------------------------------------------------
-- H.10: server_cookbook_autocorrect_previews
-- ---------------------------------------------------------------------------
-- PK is (organisation_name, cookbook_name, cookbook_version, target_chef_version)
-- PK covers most lookups; add any supplementary indexes as needed.

-- ---------------------------------------------------------------------------
-- H.11: git_repo_autocorrect_previews
-- ---------------------------------------------------------------------------
-- PK is (git_repo_name, git_repo_url, target_chef_version)

-- ---------------------------------------------------------------------------
-- H.12: server_cookbook_complexity
-- ---------------------------------------------------------------------------
-- PK is (organisation_name, cookbook_name, cookbook_version, target_chef_version)
CREATE INDEX idx_sc_complexity_target_chef_version ON server_cookbook_complexity (target_chef_version);
CREATE INDEX idx_sc_complexity_score ON server_cookbook_complexity (complexity_score);
CREATE INDEX idx_sc_complexity_label ON server_cookbook_complexity (complexity_label);
CREATE INDEX idx_sc_complexity_affected_node_count ON server_cookbook_complexity (affected_node_count);

-- ---------------------------------------------------------------------------
-- H.13: git_repo_complexity
-- ---------------------------------------------------------------------------
-- PK is (git_repo_name, git_repo_url, target_chef_version)
CREATE INDEX idx_gr_complexity_target_chef_version ON git_repo_complexity (target_chef_version);
CREATE INDEX idx_gr_complexity_score ON git_repo_complexity (complexity_score);
CREATE INDEX idx_gr_complexity_label ON git_repo_complexity (complexity_label);
CREATE INDEX idx_gr_complexity_affected_node_count ON git_repo_complexity (affected_node_count);

-- ---------------------------------------------------------------------------
-- H.14: node_readiness
-- ---------------------------------------------------------------------------
-- PK is (organisation_name, node_name, target_chef_version)
CREATE INDEX idx_node_readiness_target_chef_version ON node_readiness (target_chef_version);
CREATE INDEX idx_node_readiness_is_ready ON node_readiness (is_ready);
CREATE INDEX idx_node_readiness_stale_data ON node_readiness (stale_data);
CREATE INDEX idx_node_readiness_node_name ON node_readiness (node_name);
CREATE INDEX idx_node_readiness_latest
    ON node_readiness (organisation_name, node_name, target_chef_version, evaluated_at DESC);
CREATE INDEX idx_node_readiness_target_name_eval
    ON node_readiness (target_chef_version, node_name, evaluated_at DESC)
    INCLUDE (is_ready, stale_data, blocking_cookbooks);

-- ---------------------------------------------------------------------------
-- H.15: role_dependencies
-- ---------------------------------------------------------------------------
-- PK is (organisation_name, role_name, dependency_type, dependency_name)
CREATE INDEX idx_role_dependencies_role_name ON role_dependencies (role_name);
CREATE INDEX idx_role_dependencies_dependency_type ON role_dependencies (dependency_type);
CREATE INDEX idx_role_dependencies_dependency_name ON role_dependencies (dependency_name);

-- ---------------------------------------------------------------------------
-- H.16: metric_snapshots
-- ---------------------------------------------------------------------------
CREATE INDEX idx_metric_snapshots_organisation_name ON metric_snapshots (organisation_name);
CREATE INDEX idx_metric_snapshots_snapshot_type ON metric_snapshots (snapshot_type);
CREATE INDEX idx_metric_snapshots_snapshot_at ON metric_snapshots (snapshot_at);
CREATE INDEX idx_metric_snapshots_target_chef_version ON metric_snapshots (target_chef_version);

-- ---------------------------------------------------------------------------
-- H.17: log_entries
-- ---------------------------------------------------------------------------
CREATE INDEX idx_log_entries_timestamp ON log_entries (timestamp);
CREATE INDEX idx_log_entries_severity ON log_entries (severity);
CREATE INDEX idx_log_entries_scope ON log_entries (scope);
CREATE INDEX idx_log_entries_organisation ON log_entries (organisation);
CREATE INDEX idx_log_entries_cookbook_name ON log_entries (cookbook_name);
CREATE INDEX idx_log_entries_collection_run_org ON log_entries (collection_run_org);
CREATE INDEX idx_log_entries_retention ON log_entries (timestamp);

-- ---------------------------------------------------------------------------
-- H.18: sessions
-- ---------------------------------------------------------------------------
CREATE INDEX idx_sessions_username ON sessions (username);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

-- ---------------------------------------------------------------------------
-- H.19: users
-- ---------------------------------------------------------------------------
-- username is now PK, no separate index needed
CREATE INDEX idx_users_auth_provider ON users (auth_provider);

-- ---------------------------------------------------------------------------
-- H.20: owners
-- ---------------------------------------------------------------------------
-- name is now PK, no separate index needed
CREATE INDEX idx_owners_owner_type ON owners (owner_type);

-- ---------------------------------------------------------------------------
-- H.21: ownership_assignments
-- ---------------------------------------------------------------------------
CREATE UNIQUE INDEX idx_ownership_assignments_unique
    ON ownership_assignments (owner_name, entity_type, entity_key, COALESCE(organisation_name, '__none__'));
CREATE INDEX idx_ownership_assignments_owner_name ON ownership_assignments (owner_name);
CREATE INDEX idx_ownership_assignments_entity ON ownership_assignments (entity_type, entity_key);
CREATE INDEX idx_ownership_assignments_org ON ownership_assignments (organisation_name) WHERE organisation_name IS NOT NULL;
CREATE INDEX idx_ownership_assignments_source ON ownership_assignments (assignment_source);
CREATE INDEX idx_ownership_assignments_auto_rule ON ownership_assignments (auto_rule_name) WHERE auto_rule_name IS NOT NULL;

-- ---------------------------------------------------------------------------
-- H.22: git_repo_committers
-- ---------------------------------------------------------------------------
-- PK is (git_repo_url, author_email)
CREATE INDEX idx_git_repo_committers_repo ON git_repo_committers (git_repo_url);

-- ---------------------------------------------------------------------------
-- H.23: ownership_audit_log
-- ---------------------------------------------------------------------------
CREATE INDEX idx_ownership_audit_log_timestamp ON ownership_audit_log (timestamp DESC);
CREATE INDEX idx_ownership_audit_log_action ON ownership_audit_log (action);
CREATE INDEX idx_ownership_audit_log_owner ON ownership_audit_log (owner_name);
CREATE INDEX idx_ownership_audit_log_actor ON ownership_audit_log (actor);
CREATE INDEX idx_ownership_audit_log_entity ON ownership_audit_log (entity_type, entity_key) WHERE entity_type IS NOT NULL;

-- ---------------------------------------------------------------------------
-- H.24: export_jobs
-- ---------------------------------------------------------------------------
-- No changes — keeps UUID PK, indexes are unchanged.

-- ---------------------------------------------------------------------------
-- H.25: cookbook_usage_analysis
-- ---------------------------------------------------------------------------
-- PK is (organisation_name)
CREATE INDEX idx_cookbook_usage_analysis_analysed_at ON cookbook_usage_analysis (analysed_at);
CREATE INDEX idx_cookbook_usage_analysis_collection_run_org ON cookbook_usage_analysis (collection_run_org);

-- ---------------------------------------------------------------------------
-- H.26: cookbook_usage_detail
-- ---------------------------------------------------------------------------
-- PK is (organisation_name, cookbook_name, cookbook_version)
CREATE INDEX idx_cookbook_usage_detail_cookbook_name ON cookbook_usage_detail (cookbook_name);
CREATE INDEX idx_cookbook_usage_detail_cookbook_name_version ON cookbook_usage_detail (cookbook_name, cookbook_version);
CREATE INDEX idx_cookbook_usage_detail_is_active ON cookbook_usage_detail (is_active);
CREATE INDEX idx_cookbook_usage_detail_node_count ON cookbook_usage_detail (node_count);

-- ---------------------------------------------------------------------------
-- H.27: cookbook_platform_coverage
-- ---------------------------------------------------------------------------
-- PK is (cookbook_name)
CREATE INDEX idx_cookbook_platform_coverage_git_repo_name ON cookbook_platform_coverage (git_repo_name);

-- ---------------------------------------------------------------------------
-- Restore UNIQUE constraints that are not PKs
-- ---------------------------------------------------------------------------

-- credentials: (credential_type, name) — name is PK, but this composite is still useful
ALTER TABLE credentials
    ADD CONSTRAINT uq_credentials_type_name UNIQUE (credential_type, name);

-- organisations: (chef_server_url, org_name)
-- Already created as unique index above: idx_organisations_server_org

-- owners: name format check constraint still valid (name is PK now)
-- Constraint owners_name_format carries over unchanged.
