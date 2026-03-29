-- =============================================================================
-- Migration 0009 (down): Revert Natural Keys to UUID Primary Keys
-- =============================================================================
-- Reverses the natural key migration by:
--   1. Re-adding UUID columns (with fresh UUIDs — originals are not recoverable)
--   2. Re-adding UUID FK columns and populating them via JOINs
--   3. Dropping natural key PKs and FK constraints
--   4. Dropping natural key columns that were added in the up migration
--   5. Restoring original PK, FK, UNIQUE, and index constraints
--
-- NOTE: The regenerated UUIDs will differ from the originals. This is
-- acceptable because UUIDs were synthetic and not referenced externally.
-- =============================================================================

-- Ensure pgcrypto is available for gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ===========================================================================
-- PHASE A: Re-add UUID PK columns and populate with fresh UUIDs
-- ===========================================================================

-- Tier 1 — Root entities
ALTER TABLE credentials ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE credentials SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE credentials ALTER COLUMN id SET NOT NULL;

ALTER TABLE organisations ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE organisations SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE organisations ALTER COLUMN id SET NOT NULL;

ALTER TABLE users ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE users SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE users ALTER COLUMN id SET NOT NULL;

ALTER TABLE owners ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE owners SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE owners ALTER COLUMN id SET NOT NULL;

ALTER TABLE git_repos ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE git_repos SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE git_repos ALTER COLUMN id SET NOT NULL;

-- Tier 2
ALTER TABLE collection_runs ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE collection_runs SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE collection_runs ALTER COLUMN id SET NOT NULL;

ALTER TABLE server_cookbooks ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE server_cookbooks SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE server_cookbooks ALTER COLUMN id SET NOT NULL;

ALTER TABLE node_snapshots ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE node_snapshots SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE node_snapshots ALTER COLUMN id SET NOT NULL;

ALTER TABLE role_dependencies ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE role_dependencies SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE role_dependencies ALTER COLUMN id SET NOT NULL;

ALTER TABLE git_repo_committers ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE git_repo_committers SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE git_repo_committers ALTER COLUMN id SET NOT NULL;

-- Tier 3
ALTER TABLE node_readiness ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE node_readiness SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE node_readiness ALTER COLUMN id SET NOT NULL;

ALTER TABLE server_cookbook_cookstyle_results ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE server_cookbook_cookstyle_results SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE server_cookbook_cookstyle_results ALTER COLUMN id SET NOT NULL;

ALTER TABLE server_cookbook_complexity ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE server_cookbook_complexity SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE server_cookbook_complexity ALTER COLUMN id SET NOT NULL;

ALTER TABLE git_repo_cookstyle_results ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE git_repo_cookstyle_results SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE git_repo_cookstyle_results ALTER COLUMN id SET NOT NULL;

ALTER TABLE git_repo_complexity ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE git_repo_complexity SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE git_repo_complexity ALTER COLUMN id SET NOT NULL;

ALTER TABLE git_repo_test_kitchen_results ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE git_repo_test_kitchen_results SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE git_repo_test_kitchen_results ALTER COLUMN id SET NOT NULL;

ALTER TABLE cookbook_usage_detail ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE cookbook_usage_detail SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE cookbook_usage_detail ALTER COLUMN id SET NOT NULL;

ALTER TABLE cookbook_platform_coverage ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE cookbook_platform_coverage SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE cookbook_platform_coverage ALTER COLUMN id SET NOT NULL;

-- Tier 4
ALTER TABLE server_cookbook_autocorrect_previews ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE server_cookbook_autocorrect_previews SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE server_cookbook_autocorrect_previews ALTER COLUMN id SET NOT NULL;

ALTER TABLE git_repo_autocorrect_previews ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE git_repo_autocorrect_previews SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE git_repo_autocorrect_previews ALTER COLUMN id SET NOT NULL;

-- BIGSERIAL tables: drop BIGSERIAL id, re-add UUID id
-- metric_snapshots
ALTER TABLE metric_snapshots DROP CONSTRAINT metric_snapshots_pkey;
ALTER TABLE metric_snapshots DROP COLUMN id;
ALTER TABLE metric_snapshots ADD COLUMN id UUID DEFAULT gen_random_uuid() NOT NULL;

-- log_entries
ALTER TABLE log_entries DROP CONSTRAINT log_entries_pkey;
ALTER TABLE log_entries DROP COLUMN id;
ALTER TABLE log_entries ADD COLUMN id UUID DEFAULT gen_random_uuid() NOT NULL;

-- ownership_audit_log
ALTER TABLE ownership_audit_log DROP CONSTRAINT ownership_audit_log_pkey;
ALTER TABLE ownership_audit_log DROP COLUMN id;
ALTER TABLE ownership_audit_log ADD COLUMN id UUID DEFAULT gen_random_uuid() NOT NULL;

-- ownership_assignments: drop BIGSERIAL id, re-add UUID id
ALTER TABLE ownership_assignments DROP CONSTRAINT ownership_assignments_pkey;
ALTER TABLE ownership_assignments DROP COLUMN id;
ALTER TABLE ownership_assignments ADD COLUMN id UUID DEFAULT gen_random_uuid() NOT NULL;

-- cookbook_usage_analysis: re-add UUID id
ALTER TABLE cookbook_usage_analysis ADD COLUMN id UUID DEFAULT gen_random_uuid();
UPDATE cookbook_usage_analysis SET id = gen_random_uuid() WHERE id IS NULL;
ALTER TABLE cookbook_usage_analysis ALTER COLUMN id SET NOT NULL;

-- ===========================================================================
-- PHASE B: Re-add UUID FK columns
-- ===========================================================================

-- organisations: re-add client_key_credential_id
ALTER TABLE organisations ADD COLUMN client_key_credential_id UUID;

-- collection_runs: re-add organisation_id
ALTER TABLE collection_runs ADD COLUMN organisation_id UUID;

-- server_cookbooks: re-add organisation_id
ALTER TABLE server_cookbooks ADD COLUMN organisation_id UUID;

-- node_snapshots: re-add organisation_id, collection_run_id
ALTER TABLE node_snapshots ADD COLUMN organisation_id UUID;
ALTER TABLE node_snapshots ADD COLUMN collection_run_id UUID;

-- sessions: re-add user_id
ALTER TABLE sessions ADD COLUMN user_id UUID;

-- ownership_assignments: re-add owner_id, organisation_id
ALTER TABLE ownership_assignments ADD COLUMN owner_id UUID;
ALTER TABLE ownership_assignments ADD COLUMN organisation_id UUID;

-- role_dependencies: re-add organisation_id
ALTER TABLE role_dependencies ADD COLUMN organisation_id UUID;

-- metric_snapshots: re-add organisation_id, collection_run_id
ALTER TABLE metric_snapshots ADD COLUMN organisation_id UUID;
ALTER TABLE metric_snapshots ADD COLUMN collection_run_id UUID;

-- cookbook_usage_analysis: re-add organisation_id, collection_run_id
ALTER TABLE cookbook_usage_analysis ADD COLUMN organisation_id UUID;
ALTER TABLE cookbook_usage_analysis ADD COLUMN collection_run_id UUID;

-- log_entries: re-add collection_run_id
ALTER TABLE log_entries ADD COLUMN collection_run_id UUID;

-- node_readiness: re-add node_snapshot_id, organisation_id
ALTER TABLE node_readiness ADD COLUMN node_snapshot_id UUID;
ALTER TABLE node_readiness ADD COLUMN organisation_id UUID;

-- server_cookbook_cookstyle_results: re-add server_cookbook_id
ALTER TABLE server_cookbook_cookstyle_results ADD COLUMN server_cookbook_id UUID;

-- server_cookbook_complexity: re-add server_cookbook_id
ALTER TABLE server_cookbook_complexity ADD COLUMN server_cookbook_id UUID;

-- git_repo_cookstyle_results: re-add git_repo_id
ALTER TABLE git_repo_cookstyle_results ADD COLUMN git_repo_id UUID;

-- git_repo_complexity: re-add git_repo_id
ALTER TABLE git_repo_complexity ADD COLUMN git_repo_id UUID;

-- git_repo_test_kitchen_results: re-add git_repo_id
ALTER TABLE git_repo_test_kitchen_results ADD COLUMN git_repo_id UUID;

-- cookbook_usage_detail: re-add analysis_id, organisation_id
ALTER TABLE cookbook_usage_detail ADD COLUMN analysis_id UUID;
ALTER TABLE cookbook_usage_detail ADD COLUMN organisation_id UUID;

-- cookbook_platform_coverage: re-add git_repo_id
ALTER TABLE cookbook_platform_coverage ADD COLUMN git_repo_id UUID;

-- server_cookbook_autocorrect_previews: re-add server_cookbook_id, cookstyle_result_id
ALTER TABLE server_cookbook_autocorrect_previews ADD COLUMN server_cookbook_id UUID;
ALTER TABLE server_cookbook_autocorrect_previews ADD COLUMN cookstyle_result_id UUID;

-- git_repo_autocorrect_previews: re-add git_repo_id, cookstyle_result_id
ALTER TABLE git_repo_autocorrect_previews ADD COLUMN git_repo_id UUID;
ALTER TABLE git_repo_autocorrect_previews ADD COLUMN cookstyle_result_id UUID;

-- ===========================================================================
-- PHASE C: Populate UUID FK columns via JOINs on natural keys
-- ===========================================================================

-- organisations.client_key_credential_id from credentials
UPDATE organisations o
   SET client_key_credential_id = c.id
  FROM credentials c
 WHERE o.client_key_credential_name = c.name;

-- collection_runs.organisation_id from organisations
UPDATE collection_runs cr
   SET organisation_id = o.id
  FROM organisations o
 WHERE cr.organisation_name = o.name;
ALTER TABLE collection_runs ALTER COLUMN organisation_id SET NOT NULL;

-- server_cookbooks.organisation_id from organisations
UPDATE server_cookbooks sc
   SET organisation_id = o.id
  FROM organisations o
 WHERE sc.organisation_name = o.name;
ALTER TABLE server_cookbooks ALTER COLUMN organisation_id SET NOT NULL;

-- node_snapshots.organisation_id from organisations
UPDATE node_snapshots ns
   SET organisation_id = o.id
  FROM organisations o
 WHERE ns.organisation_name = o.name;
ALTER TABLE node_snapshots ALTER COLUMN organisation_id SET NOT NULL;

-- node_snapshots.collection_run_id from collection_runs
UPDATE node_snapshots ns
   SET collection_run_id = cr.id
  FROM collection_runs cr
 WHERE ns.collection_run_org = cr.organisation_name;
ALTER TABLE node_snapshots ALTER COLUMN collection_run_id SET NOT NULL;

-- sessions.user_id from users
UPDATE sessions s
   SET user_id = u.id
  FROM users u
 WHERE s.username = u.username;

-- ownership_assignments.owner_id from owners
UPDATE ownership_assignments oa
   SET owner_id = ow.id
  FROM owners ow
 WHERE oa.owner_name = ow.name;
ALTER TABLE ownership_assignments ALTER COLUMN owner_id SET NOT NULL;

-- ownership_assignments.organisation_id from organisations
UPDATE ownership_assignments oa
   SET organisation_id = o.id
  FROM organisations o
 WHERE oa.organisation_name = o.name;
-- organisation_id stays NULL where organisation_name is NULL

-- role_dependencies.organisation_id from organisations
UPDATE role_dependencies rd
   SET organisation_id = o.id
  FROM organisations o
 WHERE rd.organisation_name = o.name;
ALTER TABLE role_dependencies ALTER COLUMN organisation_id SET NOT NULL;

-- metric_snapshots.organisation_id from organisations
UPDATE metric_snapshots ms
   SET organisation_id = o.id
  FROM organisations o
 WHERE ms.organisation_name = o.name;
ALTER TABLE metric_snapshots ALTER COLUMN organisation_id SET NOT NULL;

-- metric_snapshots.collection_run_id from collection_runs
UPDATE metric_snapshots ms
   SET collection_run_id = cr.id
  FROM collection_runs cr
 WHERE ms.collection_run_org = cr.organisation_name;

-- cookbook_usage_analysis.organisation_id from organisations
UPDATE cookbook_usage_analysis cua
   SET organisation_id = o.id
  FROM organisations o
 WHERE cua.organisation_name = o.name;
ALTER TABLE cookbook_usage_analysis ALTER COLUMN organisation_id SET NOT NULL;

-- cookbook_usage_analysis.collection_run_id from collection_runs
UPDATE cookbook_usage_analysis cua
   SET collection_run_id = cr.id
  FROM collection_runs cr
 WHERE cua.collection_run_org = cr.organisation_name;
ALTER TABLE cookbook_usage_analysis ALTER COLUMN collection_run_id SET NOT NULL;

-- log_entries.collection_run_id from collection_runs
UPDATE log_entries le
   SET collection_run_id = cr.id
  FROM collection_runs cr
 WHERE le.collection_run_org = cr.organisation_name;

-- node_readiness.organisation_id from organisations
UPDATE node_readiness nr
   SET organisation_id = o.id
  FROM organisations o
 WHERE nr.organisation_name = o.name;
ALTER TABLE node_readiness ALTER COLUMN organisation_id SET NOT NULL;

-- node_readiness.node_snapshot_id from node_snapshots
UPDATE node_readiness nr
   SET node_snapshot_id = ns.id
  FROM node_snapshots ns
 WHERE nr.organisation_name = ns.organisation_name
   AND nr.node_name = ns.node_name;
ALTER TABLE node_readiness ALTER COLUMN node_snapshot_id SET NOT NULL;

-- server_cookbook_cookstyle_results.server_cookbook_id from server_cookbooks
UPDATE server_cookbook_cookstyle_results cr
   SET server_cookbook_id = sc.id
  FROM server_cookbooks sc
 WHERE cr.organisation_name = sc.organisation_name
   AND cr.cookbook_name = sc.name
   AND cr.cookbook_version = sc.version;
ALTER TABLE server_cookbook_cookstyle_results ALTER COLUMN server_cookbook_id SET NOT NULL;

-- server_cookbook_complexity.server_cookbook_id from server_cookbooks
UPDATE server_cookbook_complexity cx
   SET server_cookbook_id = sc.id
  FROM server_cookbooks sc
 WHERE cx.organisation_name = sc.organisation_name
   AND cx.cookbook_name = sc.name
   AND cx.cookbook_version = sc.version;
ALTER TABLE server_cookbook_complexity ALTER COLUMN server_cookbook_id SET NOT NULL;

-- git_repo_cookstyle_results.git_repo_id from git_repos
UPDATE git_repo_cookstyle_results cr
   SET git_repo_id = gr.id
  FROM git_repos gr
 WHERE cr.git_repo_name = gr.name
   AND cr.git_repo_url = gr.git_repo_url;
ALTER TABLE git_repo_cookstyle_results ALTER COLUMN git_repo_id SET NOT NULL;

-- git_repo_complexity.git_repo_id from git_repos
UPDATE git_repo_complexity cx
   SET git_repo_id = gr.id
  FROM git_repos gr
 WHERE cx.git_repo_name = gr.name
   AND cx.git_repo_url = gr.git_repo_url;
ALTER TABLE git_repo_complexity ALTER COLUMN git_repo_id SET NOT NULL;

-- git_repo_test_kitchen_results.git_repo_id from git_repos
UPDATE git_repo_test_kitchen_results tk
   SET git_repo_id = gr.id
  FROM git_repos gr
 WHERE tk.git_repo_name = gr.name
   AND tk.git_repo_url = gr.git_repo_url;
ALTER TABLE git_repo_test_kitchen_results ALTER COLUMN git_repo_id SET NOT NULL;

-- cookbook_usage_detail.analysis_id from cookbook_usage_analysis
UPDATE cookbook_usage_detail cud
   SET analysis_id = cua.id
  FROM cookbook_usage_analysis cua
 WHERE cud.organisation_name = cua.organisation_name;
ALTER TABLE cookbook_usage_detail ALTER COLUMN analysis_id SET NOT NULL;

-- cookbook_usage_detail.organisation_id from organisations
UPDATE cookbook_usage_detail cud
   SET organisation_id = o.id
  FROM organisations o
 WHERE cud.organisation_name = o.name;
ALTER TABLE cookbook_usage_detail ALTER COLUMN organisation_id SET NOT NULL;

-- cookbook_platform_coverage.git_repo_id from git_repos
UPDATE cookbook_platform_coverage cpc
   SET git_repo_id = gr.id
  FROM git_repos gr
 WHERE cpc.git_repo_name = gr.name
   AND cpc.git_repo_url = gr.git_repo_url;
-- git_repo_id stays NULL where git_repo_name/git_repo_url are NULL

-- server_cookbook_autocorrect_previews.server_cookbook_id from server_cookbooks
UPDATE server_cookbook_autocorrect_previews ap
   SET server_cookbook_id = sc.id
  FROM server_cookbooks sc
 WHERE ap.organisation_name = sc.organisation_name
   AND ap.cookbook_name = sc.name
   AND ap.cookbook_version = sc.version;
ALTER TABLE server_cookbook_autocorrect_previews ALTER COLUMN server_cookbook_id SET NOT NULL;

-- server_cookbook_autocorrect_previews.cookstyle_result_id from server_cookbook_cookstyle_results
UPDATE server_cookbook_autocorrect_previews ap
   SET cookstyle_result_id = cr.id
  FROM server_cookbook_cookstyle_results cr
 WHERE ap.organisation_name = cr.organisation_name
   AND ap.cookbook_name = cr.cookbook_name
   AND ap.cookbook_version = cr.cookbook_version
   AND ap.target_chef_version = cr.target_chef_version;
ALTER TABLE server_cookbook_autocorrect_previews ALTER COLUMN cookstyle_result_id SET NOT NULL;

-- git_repo_autocorrect_previews.git_repo_id from git_repos
UPDATE git_repo_autocorrect_previews ap
   SET git_repo_id = gr.id
  FROM git_repos gr
 WHERE ap.git_repo_name = gr.name
   AND ap.git_repo_url = gr.git_repo_url;
ALTER TABLE git_repo_autocorrect_previews ALTER COLUMN git_repo_id SET NOT NULL;

-- git_repo_autocorrect_previews.cookstyle_result_id from git_repo_cookstyle_results
UPDATE git_repo_autocorrect_previews ap
   SET cookstyle_result_id = cr.id
  FROM git_repo_cookstyle_results cr
 WHERE ap.git_repo_name = cr.git_repo_name
   AND ap.git_repo_url = cr.git_repo_url
   AND ap.target_chef_version = cr.target_chef_version;
ALTER TABLE git_repo_autocorrect_previews ALTER COLUMN cookstyle_result_id SET NOT NULL;

-- ===========================================================================
-- PHASE D: Drop natural-key PK/FK constraints and indexes added in up migration
-- ===========================================================================

-- D.1: Drop FK constraints added in the up migration

ALTER TABLE git_repo_autocorrect_previews
    DROP CONSTRAINT IF EXISTS fk_gr_autocorrect_cookstyle,
    DROP CONSTRAINT IF EXISTS fk_gr_autocorrect_repo;

ALTER TABLE server_cookbook_autocorrect_previews
    DROP CONSTRAINT IF EXISTS fk_sc_autocorrect_cookstyle,
    DROP CONSTRAINT IF EXISTS fk_sc_autocorrect_cookbook;

ALTER TABLE cookbook_platform_coverage
    DROP CONSTRAINT IF EXISTS fk_cookbook_platform_coverage_repo;

ALTER TABLE cookbook_usage_detail
    DROP CONSTRAINT IF EXISTS fk_cookbook_usage_detail_analysis;

ALTER TABLE git_repo_test_kitchen_results
    DROP CONSTRAINT IF EXISTS fk_gr_test_kitchen_results_repo;

ALTER TABLE git_repo_complexity
    DROP CONSTRAINT IF EXISTS fk_gr_complexity_repo;

ALTER TABLE git_repo_cookstyle_results
    DROP CONSTRAINT IF EXISTS fk_gr_cookstyle_results_repo;

ALTER TABLE server_cookbook_complexity
    DROP CONSTRAINT IF EXISTS fk_sc_complexity_cookbook;

ALTER TABLE server_cookbook_cookstyle_results
    DROP CONSTRAINT IF EXISTS fk_sc_cookstyle_results_cookbook;

ALTER TABLE node_readiness
    DROP CONSTRAINT IF EXISTS fk_node_readiness_node_snapshot;

ALTER TABLE metric_snapshots
    DROP CONSTRAINT IF EXISTS fk_metric_snapshots_organisation;

ALTER TABLE role_dependencies
    DROP CONSTRAINT IF EXISTS fk_role_dependencies_organisation;

ALTER TABLE ownership_assignments
    DROP CONSTRAINT IF EXISTS fk_ownership_assignments_owner;

ALTER TABLE sessions
    DROP CONSTRAINT IF EXISTS fk_sessions_user;

ALTER TABLE node_snapshots
    DROP CONSTRAINT IF EXISTS fk_node_snapshots_organisation;

ALTER TABLE server_cookbooks
    DROP CONSTRAINT IF EXISTS fk_server_cookbooks_organisation;

ALTER TABLE collection_runs
    DROP CONSTRAINT IF EXISTS fk_collection_runs_organisation;

ALTER TABLE cookbook_usage_analysis
    DROP CONSTRAINT IF EXISTS fk_cookbook_usage_analysis_organisation;

ALTER TABLE organisations
    DROP CONSTRAINT IF EXISTS fk_organisations_credential;

-- D.2: Drop all indexes created in the up migration

DROP INDEX IF EXISTS idx_cookbook_platform_coverage_git_repo_name;
DROP INDEX IF EXISTS idx_cookbook_usage_detail_node_count;
DROP INDEX IF EXISTS idx_cookbook_usage_detail_is_active;
DROP INDEX IF EXISTS idx_cookbook_usage_detail_cookbook_name_version;
DROP INDEX IF EXISTS idx_cookbook_usage_detail_cookbook_name;
DROP INDEX IF EXISTS idx_cookbook_usage_analysis_collection_run_org;
DROP INDEX IF EXISTS idx_cookbook_usage_analysis_analysed_at;
DROP INDEX IF EXISTS idx_ownership_audit_log_entity;
DROP INDEX IF EXISTS idx_ownership_audit_log_actor;
DROP INDEX IF EXISTS idx_ownership_audit_log_owner;
DROP INDEX IF EXISTS idx_ownership_audit_log_action;
DROP INDEX IF EXISTS idx_ownership_audit_log_timestamp;
DROP INDEX IF EXISTS idx_git_repo_committers_repo;
DROP INDEX IF EXISTS idx_ownership_assignments_auto_rule;
DROP INDEX IF EXISTS idx_ownership_assignments_source;
DROP INDEX IF EXISTS idx_ownership_assignments_org;
DROP INDEX IF EXISTS idx_ownership_assignments_entity;
DROP INDEX IF EXISTS idx_ownership_assignments_owner_name;
DROP INDEX IF EXISTS idx_ownership_assignments_unique;
DROP INDEX IF EXISTS idx_owners_owner_type;
DROP INDEX IF EXISTS idx_users_auth_provider;
DROP INDEX IF EXISTS idx_sessions_expires_at;
DROP INDEX IF EXISTS idx_sessions_username;
DROP INDEX IF EXISTS idx_log_entries_retention;
DROP INDEX IF EXISTS idx_log_entries_collection_run_org;
DROP INDEX IF EXISTS idx_log_entries_cookbook_name;
DROP INDEX IF EXISTS idx_log_entries_organisation;
DROP INDEX IF EXISTS idx_log_entries_scope;
DROP INDEX IF EXISTS idx_log_entries_severity;
DROP INDEX IF EXISTS idx_log_entries_timestamp;
DROP INDEX IF EXISTS idx_metric_snapshots_target_chef_version;
DROP INDEX IF EXISTS idx_metric_snapshots_snapshot_at;
DROP INDEX IF EXISTS idx_metric_snapshots_snapshot_type;
DROP INDEX IF EXISTS idx_metric_snapshots_organisation_name;
DROP INDEX IF EXISTS idx_node_readiness_target_name_eval;
DROP INDEX IF EXISTS idx_node_readiness_latest;
DROP INDEX IF EXISTS idx_node_readiness_node_name;
DROP INDEX IF EXISTS idx_node_readiness_stale_data;
DROP INDEX IF EXISTS idx_node_readiness_is_ready;
DROP INDEX IF EXISTS idx_node_readiness_target_chef_version;
DROP INDEX IF EXISTS idx_role_dependencies_dependency_name;
DROP INDEX IF EXISTS idx_role_dependencies_dependency_type;
DROP INDEX IF EXISTS idx_role_dependencies_role_name;
DROP INDEX IF EXISTS idx_gr_complexity_affected_node_count;
DROP INDEX IF EXISTS idx_gr_complexity_label;
DROP INDEX IF EXISTS idx_gr_complexity_score;
DROP INDEX IF EXISTS idx_gr_complexity_target_chef_version;
DROP INDEX IF EXISTS idx_sc_complexity_affected_node_count;
DROP INDEX IF EXISTS idx_sc_complexity_label;
DROP INDEX IF EXISTS idx_sc_complexity_score;
DROP INDEX IF EXISTS idx_sc_complexity_target_chef_version;
DROP INDEX IF EXISTS idx_gr_test_kitchen_results_compatible;
DROP INDEX IF EXISTS idx_gr_test_kitchen_results_commit_sha;
DROP INDEX IF EXISTS idx_gr_test_kitchen_results_target_chef_version;
DROP INDEX IF EXISTS idx_gr_cookstyle_results_commit_sha;
DROP INDEX IF EXISTS idx_gr_cookstyle_results_passed;
DROP INDEX IF EXISTS idx_gr_cookstyle_results_target_chef_version;
DROP INDEX IF EXISTS idx_sc_cookstyle_results_org_cookbook;
DROP INDEX IF EXISTS idx_sc_cookstyle_results_passed;
DROP INDEX IF EXISTS idx_sc_cookstyle_results_target_chef_version;
DROP INDEX IF EXISTS idx_git_repos_clone_status;
DROP INDEX IF EXISTS idx_git_repos_git_repo_url;
DROP INDEX IF EXISTS idx_git_repos_name;
DROP INDEX IF EXISTS idx_server_cookbooks_download_status;
DROP INDEX IF EXISTS idx_server_cookbooks_first_seen_at;
DROP INDEX IF EXISTS idx_server_cookbooks_name_version;
DROP INDEX IF EXISTS idx_server_cookbooks_is_stale_cookbook;
DROP INDEX IF EXISTS idx_server_cookbooks_is_active;
DROP INDEX IF EXISTS idx_server_cookbooks_name;
DROP INDEX IF EXISTS idx_server_cookbooks_organisation_name;
DROP INDEX IF EXISTS idx_node_snapshots_policy_group_lower;
DROP INDEX IF EXISTS idx_node_snapshots_policy_name_lower;
DROP INDEX IF EXISTS idx_node_snapshots_chef_version_lower;
DROP INDEX IF EXISTS idx_node_snapshots_chef_environment_lower;
DROP INDEX IF EXISTS idx_node_snapshots_node_name_lower;
DROP INDEX IF EXISTS idx_node_snapshots_platform_combined;
DROP INDEX IF EXISTS idx_node_snapshots_roles_gin;
DROP INDEX IF EXISTS idx_node_snapshots_collection_run_org;
DROP INDEX IF EXISTS idx_node_snapshots_org_name_collected;
DROP INDEX IF EXISTS idx_node_snapshots_is_stale;
DROP INDEX IF EXISTS idx_node_snapshots_policy_group;
DROP INDEX IF EXISTS idx_node_snapshots_policy_name;
DROP INDEX IF EXISTS idx_node_snapshots_collected_at;
DROP INDEX IF EXISTS idx_node_snapshots_chef_environment;
DROP INDEX IF EXISTS idx_node_snapshots_platform_family;
DROP INDEX IF EXISTS idx_node_snapshots_platform;
DROP INDEX IF EXISTS idx_node_snapshots_chef_version;
DROP INDEX IF EXISTS idx_node_snapshots_node_name;
DROP INDEX IF EXISTS idx_collection_runs_org_completed_started;
DROP INDEX IF EXISTS idx_collection_runs_org_status_started;
DROP INDEX IF EXISTS idx_collection_runs_started_at;
DROP INDEX IF EXISTS idx_collection_runs_status;
DROP INDEX IF EXISTS idx_organisations_client_key_credential_name;
DROP INDEX IF EXISTS idx_organisations_server_org;
DROP INDEX IF EXISTS idx_credentials_credential_type;

-- D.3: Drop UNIQUE constraints added in up migration
ALTER TABLE credentials DROP CONSTRAINT IF EXISTS uq_credentials_type_name;

-- D.4: Drop natural-key PRIMARY KEY constraints

-- Tier 4
ALTER TABLE git_repo_autocorrect_previews DROP CONSTRAINT git_repo_autocorrect_previews_pkey;
ALTER TABLE server_cookbook_autocorrect_previews DROP CONSTRAINT server_cookbook_autocorrect_previews_pkey;

-- Tier 3
ALTER TABLE cookbook_platform_coverage DROP CONSTRAINT cookbook_platform_coverage_pkey;
ALTER TABLE cookbook_usage_detail DROP CONSTRAINT cookbook_usage_detail_pkey;
ALTER TABLE git_repo_test_kitchen_results DROP CONSTRAINT git_repo_test_kitchen_results_pkey;
ALTER TABLE git_repo_complexity DROP CONSTRAINT git_repo_complexity_pkey;
ALTER TABLE git_repo_cookstyle_results DROP CONSTRAINT git_repo_cookstyle_results_pkey;
ALTER TABLE server_cookbook_complexity DROP CONSTRAINT server_cookbook_complexity_pkey;
ALTER TABLE server_cookbook_cookstyle_results DROP CONSTRAINT server_cookbook_cookstyle_results_pkey;
ALTER TABLE node_readiness DROP CONSTRAINT node_readiness_pkey;

-- Tier 2
ALTER TABLE git_repo_committers DROP CONSTRAINT git_repo_committers_pkey;
ALTER TABLE cookbook_usage_analysis DROP CONSTRAINT cookbook_usage_analysis_pkey;
ALTER TABLE role_dependencies DROP CONSTRAINT role_dependencies_pkey;
ALTER TABLE node_snapshots DROP CONSTRAINT node_snapshots_pkey;
ALTER TABLE server_cookbooks DROP CONSTRAINT server_cookbooks_pkey;
ALTER TABLE collection_runs DROP CONSTRAINT collection_runs_pkey;

-- Tier 1
ALTER TABLE git_repos DROP CONSTRAINT git_repos_pkey;
ALTER TABLE owners DROP CONSTRAINT owners_pkey;
ALTER TABLE users DROP CONSTRAINT users_pkey;
ALTER TABLE organisations DROP CONSTRAINT organisations_pkey;
ALTER TABLE credentials DROP CONSTRAINT credentials_pkey;

-- ===========================================================================
-- PHASE E: Drop natural key columns added in the up migration
-- ===========================================================================

-- Tier 4
ALTER TABLE git_repo_autocorrect_previews
    DROP COLUMN git_repo_name,
    DROP COLUMN git_repo_url,
    DROP COLUMN target_chef_version;

ALTER TABLE server_cookbook_autocorrect_previews
    DROP COLUMN organisation_name,
    DROP COLUMN cookbook_name,
    DROP COLUMN cookbook_version,
    DROP COLUMN target_chef_version;

-- Tier 3
ALTER TABLE cookbook_platform_coverage
    DROP COLUMN git_repo_name,
    DROP COLUMN git_repo_url;

ALTER TABLE cookbook_usage_detail
    DROP COLUMN organisation_name;

ALTER TABLE git_repo_test_kitchen_results
    DROP COLUMN git_repo_name,
    DROP COLUMN git_repo_url;

ALTER TABLE git_repo_complexity
    DROP COLUMN git_repo_name,
    DROP COLUMN git_repo_url;

ALTER TABLE git_repo_cookstyle_results
    DROP COLUMN git_repo_name,
    DROP COLUMN git_repo_url;

ALTER TABLE server_cookbook_complexity
    DROP COLUMN organisation_name,
    DROP COLUMN cookbook_name,
    DROP COLUMN cookbook_version;

ALTER TABLE server_cookbook_cookstyle_results
    DROP COLUMN organisation_name,
    DROP COLUMN cookbook_name,
    DROP COLUMN cookbook_version;

ALTER TABLE node_readiness
    DROP COLUMN organisation_name;

-- Tier 2
ALTER TABLE log_entries
    DROP COLUMN collection_run_org;

ALTER TABLE cookbook_usage_analysis
    DROP COLUMN organisation_name,
    DROP COLUMN collection_run_org;

ALTER TABLE metric_snapshots
    DROP COLUMN organisation_name,
    DROP COLUMN collection_run_org;

ALTER TABLE role_dependencies
    DROP COLUMN organisation_name;

ALTER TABLE ownership_assignments
    DROP COLUMN owner_name,
    DROP COLUMN organisation_name;

ALTER TABLE node_snapshots
    DROP COLUMN organisation_name,
    DROP COLUMN collection_run_org;

ALTER TABLE server_cookbooks
    DROP COLUMN organisation_name;

ALTER TABLE collection_runs
    DROP COLUMN organisation_name;

-- Tier 1
ALTER TABLE organisations
    DROP COLUMN client_key_credential_name;

-- ===========================================================================
-- PHASE F: Restore UUID PRIMARY KEY constraints
-- ===========================================================================

-- Tier 1
ALTER TABLE credentials ADD PRIMARY KEY (id);
ALTER TABLE organisations ADD PRIMARY KEY (id);
ALTER TABLE users ADD PRIMARY KEY (id);
ALTER TABLE owners ADD PRIMARY KEY (id);
ALTER TABLE git_repos ADD PRIMARY KEY (id);

-- Tier 2
ALTER TABLE collection_runs ADD PRIMARY KEY (id);
ALTER TABLE server_cookbooks ADD PRIMARY KEY (id);
ALTER TABLE node_snapshots ADD PRIMARY KEY (id);
-- sessions: PK was never dropped
ALTER TABLE role_dependencies ADD PRIMARY KEY (id);
ALTER TABLE metric_snapshots ADD PRIMARY KEY (id);
ALTER TABLE cookbook_usage_analysis ADD PRIMARY KEY (id);
ALTER TABLE log_entries ADD PRIMARY KEY (id);
ALTER TABLE git_repo_committers ADD PRIMARY KEY (id);
ALTER TABLE ownership_audit_log ADD PRIMARY KEY (id);
ALTER TABLE ownership_assignments ADD PRIMARY KEY (id);

-- Tier 3
ALTER TABLE node_readiness ADD PRIMARY KEY (id);
ALTER TABLE server_cookbook_cookstyle_results ADD PRIMARY KEY (id);
ALTER TABLE server_cookbook_complexity ADD PRIMARY KEY (id);
ALTER TABLE git_repo_cookstyle_results ADD PRIMARY KEY (id);
ALTER TABLE git_repo_complexity ADD PRIMARY KEY (id);
ALTER TABLE git_repo_test_kitchen_results ADD PRIMARY KEY (id);
ALTER TABLE cookbook_usage_detail ADD PRIMARY KEY (id);
ALTER TABLE cookbook_platform_coverage ADD PRIMARY KEY (id);

-- Tier 4
ALTER TABLE server_cookbook_autocorrect_previews ADD PRIMARY KEY (id);
ALTER TABLE git_repo_autocorrect_previews ADD PRIMARY KEY (id);

-- ===========================================================================
-- PHASE G: Restore FOREIGN KEY constraints
-- ===========================================================================

-- Tier 1
ALTER TABLE organisations
    ADD CONSTRAINT organisations_client_key_credential_id_fkey
    FOREIGN KEY (client_key_credential_id) REFERENCES credentials(id) ON DELETE SET NULL;

-- Tier 2
ALTER TABLE collection_runs
    ADD CONSTRAINT collection_runs_organisation_id_fkey
    FOREIGN KEY (organisation_id) REFERENCES organisations(id) ON DELETE CASCADE;

ALTER TABLE server_cookbooks
    ADD CONSTRAINT server_cookbooks_organisation_id_fkey
    FOREIGN KEY (organisation_id) REFERENCES organisations(id) ON DELETE CASCADE;

ALTER TABLE node_snapshots
    ADD CONSTRAINT node_snapshots_collection_run_id_fkey
    FOREIGN KEY (collection_run_id) REFERENCES collection_runs(id) ON DELETE CASCADE;
ALTER TABLE node_snapshots
    ADD CONSTRAINT node_snapshots_organisation_id_fkey
    FOREIGN KEY (organisation_id) REFERENCES organisations(id) ON DELETE CASCADE;

ALTER TABLE sessions
    ADD CONSTRAINT sessions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ownership_assignments
    ADD CONSTRAINT ownership_assignments_owner_id_fkey
    FOREIGN KEY (owner_id) REFERENCES owners(id) ON DELETE CASCADE;
ALTER TABLE ownership_assignments
    ADD CONSTRAINT ownership_assignments_organisation_id_fkey
    FOREIGN KEY (organisation_id) REFERENCES organisations(id) ON DELETE CASCADE;

ALTER TABLE role_dependencies
    ADD CONSTRAINT role_dependencies_organisation_id_fkey
    FOREIGN KEY (organisation_id) REFERENCES organisations(id) ON DELETE CASCADE;

ALTER TABLE metric_snapshots
    ADD CONSTRAINT metric_snapshots_collection_run_id_fkey
    FOREIGN KEY (collection_run_id) REFERENCES collection_runs(id) ON DELETE SET NULL;
ALTER TABLE metric_snapshots
    ADD CONSTRAINT metric_snapshots_organisation_id_fkey
    FOREIGN KEY (organisation_id) REFERENCES organisations(id) ON DELETE CASCADE;

ALTER TABLE cookbook_usage_analysis
    ADD CONSTRAINT cookbook_usage_analysis_organisation_id_fkey
    FOREIGN KEY (organisation_id) REFERENCES organisations(id) ON DELETE CASCADE;
ALTER TABLE cookbook_usage_analysis
    ADD CONSTRAINT cookbook_usage_analysis_collection_run_id_fkey
    FOREIGN KEY (collection_run_id) REFERENCES collection_runs(id) ON DELETE CASCADE;

ALTER TABLE log_entries
    ADD CONSTRAINT log_entries_collection_run_id_fkey
    FOREIGN KEY (collection_run_id) REFERENCES collection_runs(id) ON DELETE SET NULL;

-- Tier 3
ALTER TABLE node_readiness
    ADD CONSTRAINT node_readiness_node_snapshot_id_fkey
    FOREIGN KEY (node_snapshot_id) REFERENCES node_snapshots(id) ON DELETE CASCADE;
ALTER TABLE node_readiness
    ADD CONSTRAINT node_readiness_organisation_id_fkey
    FOREIGN KEY (organisation_id) REFERENCES organisations(id) ON DELETE CASCADE;

ALTER TABLE server_cookbook_cookstyle_results
    ADD CONSTRAINT server_cookbook_cookstyle_results_server_cookbook_id_fkey
    FOREIGN KEY (server_cookbook_id) REFERENCES server_cookbooks(id) ON DELETE CASCADE;

ALTER TABLE server_cookbook_complexity
    ADD CONSTRAINT server_cookbook_complexity_server_cookbook_id_fkey
    FOREIGN KEY (server_cookbook_id) REFERENCES server_cookbooks(id) ON DELETE CASCADE;

ALTER TABLE git_repo_cookstyle_results
    ADD CONSTRAINT git_repo_cookstyle_results_git_repo_id_fkey
    FOREIGN KEY (git_repo_id) REFERENCES git_repos(id) ON DELETE CASCADE;

ALTER TABLE git_repo_complexity
    ADD CONSTRAINT git_repo_complexity_git_repo_id_fkey
    FOREIGN KEY (git_repo_id) REFERENCES git_repos(id) ON DELETE CASCADE;

ALTER TABLE git_repo_test_kitchen_results
    ADD CONSTRAINT git_repo_test_kitchen_results_git_repo_id_fkey
    FOREIGN KEY (git_repo_id) REFERENCES git_repos(id) ON DELETE CASCADE;

ALTER TABLE cookbook_usage_detail
    ADD CONSTRAINT cookbook_usage_detail_analysis_id_fkey
    FOREIGN KEY (analysis_id) REFERENCES cookbook_usage_analysis(id) ON DELETE CASCADE;
ALTER TABLE cookbook_usage_detail
    ADD CONSTRAINT cookbook_usage_detail_organisation_id_fkey
    FOREIGN KEY (organisation_id) REFERENCES organisations(id) ON DELETE CASCADE;

ALTER TABLE cookbook_platform_coverage
    ADD CONSTRAINT fk_cookbook_platform_coverage_git_repo
    FOREIGN KEY (git_repo_id) REFERENCES git_repos(id) ON DELETE CASCADE;

-- Tier 4
ALTER TABLE server_cookbook_autocorrect_previews
    ADD CONSTRAINT server_cookbook_autocorrect_previews_server_cookbook_id_fkey
    FOREIGN KEY (server_cookbook_id) REFERENCES server_cookbooks(id) ON DELETE CASCADE;
ALTER TABLE server_cookbook_autocorrect_previews
    ADD CONSTRAINT server_cookbook_autocorrect_previews_cookstyle_result_id_fkey
    FOREIGN KEY (cookstyle_result_id) REFERENCES server_cookbook_cookstyle_results(id) ON DELETE CASCADE;

ALTER TABLE git_repo_autocorrect_previews
    ADD CONSTRAINT git_repo_autocorrect_previews_git_repo_id_fkey
    FOREIGN KEY (git_repo_id) REFERENCES git_repos(id) ON DELETE CASCADE;
ALTER TABLE git_repo_autocorrect_previews
    ADD CONSTRAINT git_repo_autocorrect_previews_cookstyle_result_id_fkey
    FOREIGN KEY (cookstyle_result_id) REFERENCES git_repo_cookstyle_results(id) ON DELETE CASCADE;

-- ===========================================================================
-- PHASE H: Restore UNIQUE constraints and indexes
-- ===========================================================================

-- credentials
ALTER TABLE credentials ADD CONSTRAINT uq_credentials_name UNIQUE (name);
ALTER TABLE credentials ADD CONSTRAINT uq_credentials_type_name UNIQUE (credential_type, name);
CREATE INDEX idx_credentials_name ON credentials (name);
CREATE INDEX idx_credentials_credential_type ON credentials (credential_type);

-- organisations
ALTER TABLE organisations ADD CONSTRAINT uq_organisations_name UNIQUE (name);
ALTER TABLE organisations ADD CONSTRAINT uq_organisations_server_org UNIQUE (chef_server_url, org_name);
CREATE INDEX idx_organisations_name ON organisations (name);
CREATE INDEX idx_organisations_client_key_credential_id ON organisations (client_key_credential_id);

-- collection_runs
ALTER TABLE collection_runs ADD CONSTRAINT uq_collection_runs_org UNIQUE (organisation_id);
CREATE INDEX idx_collection_runs_organisation_id ON collection_runs (organisation_id);
CREATE INDEX idx_collection_runs_status ON collection_runs (status);
CREATE INDEX idx_collection_runs_started_at ON collection_runs (started_at);
CREATE INDEX idx_collection_runs_org_status_started
    ON collection_runs (organisation_id, status, started_at DESC);

-- node_snapshots
ALTER TABLE node_snapshots ADD CONSTRAINT uq_node_snapshots_org_node UNIQUE (organisation_id, node_name);
CREATE INDEX idx_node_snapshots_collection_run_id ON node_snapshots (collection_run_id);
CREATE INDEX idx_node_snapshots_organisation_id ON node_snapshots (organisation_id);
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
    ON node_snapshots (organisation_id, node_name, collected_at DESC);
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

-- server_cookbooks
ALTER TABLE server_cookbooks ADD CONSTRAINT server_cookbooks_organisation_id_name_version_key
    UNIQUE (organisation_id, name, version);
CREATE INDEX idx_server_cookbooks_organisation_id ON server_cookbooks (organisation_id);
CREATE INDEX idx_server_cookbooks_name ON server_cookbooks (name);
CREATE INDEX idx_server_cookbooks_is_active ON server_cookbooks (is_active);
CREATE INDEX idx_server_cookbooks_is_stale_cookbook ON server_cookbooks (is_stale_cookbook);
CREATE INDEX idx_server_cookbooks_name_version ON server_cookbooks (name, version);
CREATE INDEX idx_server_cookbooks_first_seen_at ON server_cookbooks (first_seen_at);
CREATE INDEX idx_server_cookbooks_download_status ON server_cookbooks (download_status);

-- git_repos
CREATE INDEX idx_git_repos_name ON git_repos (name);
CREATE INDEX idx_git_repos_git_repo_url ON git_repos (git_repo_url);
CREATE INDEX idx_git_repos_clone_status ON git_repos (clone_status);

-- server_cookbook_cookstyle_results
ALTER TABLE server_cookbook_cookstyle_results
    ADD CONSTRAINT server_cookbook_cookstyle_results_server_cookbook_id_target_c_key
    UNIQUE (server_cookbook_id, target_chef_version);
CREATE INDEX idx_sc_cookstyle_results_server_cookbook_id ON server_cookbook_cookstyle_results (server_cookbook_id);
CREATE INDEX idx_sc_cookstyle_results_target_chef_version ON server_cookbook_cookstyle_results (target_chef_version);
CREATE INDEX idx_sc_cookstyle_results_passed ON server_cookbook_cookstyle_results (passed);

-- git_repo_cookstyle_results
ALTER TABLE git_repo_cookstyle_results
    ADD CONSTRAINT git_repo_cookstyle_results_git_repo_id_target_chef_version_key
    UNIQUE (git_repo_id, target_chef_version);
CREATE INDEX idx_gr_cookstyle_results_git_repo_id ON git_repo_cookstyle_results (git_repo_id);
CREATE INDEX idx_gr_cookstyle_results_target_chef_version ON git_repo_cookstyle_results (target_chef_version);
CREATE INDEX idx_gr_cookstyle_results_passed ON git_repo_cookstyle_results (passed);
CREATE INDEX idx_gr_cookstyle_results_commit_sha ON git_repo_cookstyle_results (commit_sha)
    WHERE commit_sha IS NOT NULL;

-- git_repo_test_kitchen_results
ALTER TABLE git_repo_test_kitchen_results
    ADD CONSTRAINT uq_git_repo_test_kitchen_results UNIQUE (git_repo_id, target_chef_version);
CREATE INDEX idx_gr_test_kitchen_results_git_repo_id ON git_repo_test_kitchen_results (git_repo_id);
CREATE INDEX idx_gr_test_kitchen_results_target_chef_version ON git_repo_test_kitchen_results (target_chef_version);
CREATE INDEX idx_gr_test_kitchen_results_commit_sha ON git_repo_test_kitchen_results (commit_sha);
CREATE INDEX idx_gr_test_kitchen_results_compatible ON git_repo_test_kitchen_results (compatible);
CREATE INDEX idx_gr_test_kitchen_results_repo_target ON git_repo_test_kitchen_results (git_repo_id, target_chef_version);

-- server_cookbook_autocorrect_previews
ALTER TABLE server_cookbook_autocorrect_previews
    ADD CONSTRAINT uq_sc_autocorrect_previews_cookstyle UNIQUE (cookstyle_result_id);
CREATE INDEX idx_sc_autocorrect_previews_server_cookbook_id ON server_cookbook_autocorrect_previews (server_cookbook_id);
CREATE INDEX idx_sc_autocorrect_previews_cookstyle_result_id ON server_cookbook_autocorrect_previews (cookstyle_result_id);

-- git_repo_autocorrect_previews
ALTER TABLE git_repo_autocorrect_previews
    ADD CONSTRAINT uq_gr_autocorrect_previews_cookstyle UNIQUE (cookstyle_result_id);
CREATE INDEX idx_gr_autocorrect_previews_git_repo_id ON git_repo_autocorrect_previews (git_repo_id);
CREATE INDEX idx_gr_autocorrect_previews_cookstyle_result_id ON git_repo_autocorrect_previews (cookstyle_result_id);

-- server_cookbook_complexity
ALTER TABLE server_cookbook_complexity
    ADD CONSTRAINT uq_sc_cookbook_complexity UNIQUE (server_cookbook_id, target_chef_version);
CREATE INDEX idx_sc_complexity_server_cookbook_id ON server_cookbook_complexity (server_cookbook_id);
CREATE INDEX idx_sc_complexity_target_chef_version ON server_cookbook_complexity (target_chef_version);
CREATE INDEX idx_sc_complexity_score ON server_cookbook_complexity (complexity_score);
CREATE INDEX idx_sc_complexity_label ON server_cookbook_complexity (complexity_label);
CREATE INDEX idx_sc_complexity_affected_node_count ON server_cookbook_complexity (affected_node_count);

-- git_repo_complexity
ALTER TABLE git_repo_complexity
    ADD CONSTRAINT uq_gr_cookbook_complexity UNIQUE (git_repo_id, target_chef_version);
CREATE INDEX idx_gr_complexity_git_repo_id ON git_repo_complexity (git_repo_id);
CREATE INDEX idx_gr_complexity_target_chef_version ON git_repo_complexity (target_chef_version);
CREATE INDEX idx_gr_complexity_score ON git_repo_complexity (complexity_score);
CREATE INDEX idx_gr_complexity_label ON git_repo_complexity (complexity_label);
CREATE INDEX idx_gr_complexity_affected_node_count ON git_repo_complexity (affected_node_count);

-- node_readiness
ALTER TABLE node_readiness
    ADD CONSTRAINT uq_node_readiness UNIQUE (node_snapshot_id, target_chef_version);
CREATE INDEX idx_node_readiness_node_snapshot_id ON node_readiness (node_snapshot_id);
CREATE INDEX idx_node_readiness_organisation_id ON node_readiness (organisation_id);
CREATE INDEX idx_node_readiness_target_chef_version ON node_readiness (target_chef_version);
CREATE INDEX idx_node_readiness_is_ready ON node_readiness (is_ready);
CREATE INDEX idx_node_readiness_stale_data ON node_readiness (stale_data);
CREATE INDEX idx_node_readiness_node_name ON node_readiness (node_name);
CREATE INDEX idx_node_readiness_latest
    ON node_readiness (organisation_id, node_name, target_chef_version, evaluated_at DESC)
    INCLUDE (id);
CREATE INDEX idx_node_readiness_target_name_eval
    ON node_readiness (target_chef_version, node_name, evaluated_at DESC)
    INCLUDE (id, is_ready, stale_data, blocking_cookbooks);

-- role_dependencies
ALTER TABLE role_dependencies
    ADD CONSTRAINT uq_role_dependencies
    UNIQUE (organisation_id, role_name, dependency_type, dependency_name);
CREATE INDEX idx_role_dependencies_organisation_id ON role_dependencies (organisation_id);
CREATE INDEX idx_role_dependencies_role_name ON role_dependencies (role_name);
CREATE INDEX idx_role_dependencies_dependency_type ON role_dependencies (dependency_type);
CREATE INDEX idx_role_dependencies_dependency_name ON role_dependencies (dependency_name);

-- metric_snapshots
CREATE INDEX idx_metric_snapshots_organisation_id ON metric_snapshots (organisation_id);
CREATE INDEX idx_metric_snapshots_snapshot_type ON metric_snapshots (snapshot_type);
CREATE INDEX idx_metric_snapshots_snapshot_at ON metric_snapshots (snapshot_at);
CREATE INDEX idx_metric_snapshots_target_chef_version ON metric_snapshots (target_chef_version);

-- log_entries
CREATE INDEX idx_log_entries_timestamp ON log_entries (timestamp);
CREATE INDEX idx_log_entries_severity ON log_entries (severity);
CREATE INDEX idx_log_entries_scope ON log_entries (scope);
CREATE INDEX idx_log_entries_organisation ON log_entries (organisation);
CREATE INDEX idx_log_entries_cookbook_name ON log_entries (cookbook_name);
CREATE INDEX idx_log_entries_collection_run_id ON log_entries (collection_run_id);
CREATE INDEX idx_log_entries_retention ON log_entries (timestamp);

-- sessions
CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

-- users
ALTER TABLE users ADD CONSTRAINT uq_users_username UNIQUE (username);
CREATE INDEX idx_users_username ON users (username);
CREATE INDEX idx_users_auth_provider ON users (auth_provider);

-- owners
ALTER TABLE owners ADD CONSTRAINT owners_name_unique UNIQUE (name);
CREATE INDEX idx_owners_owner_type ON owners (owner_type);

-- ownership_assignments
CREATE UNIQUE INDEX idx_ownership_assignments_unique
    ON ownership_assignments (owner_id, entity_type, entity_key, COALESCE(organisation_id, '00000000-0000-0000-0000-000000000000'));
CREATE INDEX idx_ownership_assignments_owner_id ON ownership_assignments (owner_id);
CREATE INDEX idx_ownership_assignments_entity ON ownership_assignments (entity_type, entity_key);
CREATE INDEX idx_ownership_assignments_org ON ownership_assignments (organisation_id) WHERE organisation_id IS NOT NULL;
CREATE INDEX idx_ownership_assignments_source ON ownership_assignments (assignment_source);
CREATE INDEX idx_ownership_assignments_auto_rule ON ownership_assignments (auto_rule_name) WHERE auto_rule_name IS NOT NULL;

-- git_repo_committers
ALTER TABLE git_repo_committers ADD CONSTRAINT git_repo_committers_unique UNIQUE (git_repo_url, author_email);
CREATE INDEX idx_git_repo_committers_repo ON git_repo_committers (git_repo_url);

-- ownership_audit_log
CREATE INDEX idx_ownership_audit_log_timestamp ON ownership_audit_log (timestamp DESC);
CREATE INDEX idx_ownership_audit_log_action ON ownership_audit_log (action);
CREATE INDEX idx_ownership_audit_log_owner ON ownership_audit_log (owner_name);
CREATE INDEX idx_ownership_audit_log_actor ON ownership_audit_log (actor);
CREATE INDEX idx_ownership_audit_log_entity ON ownership_audit_log (entity_type, entity_key) WHERE entity_type IS NOT NULL;

-- cookbook_usage_analysis
ALTER TABLE cookbook_usage_analysis ADD CONSTRAINT uq_cookbook_usage_analysis_org UNIQUE (organisation_id);
CREATE INDEX idx_cookbook_usage_analysis_organisation_id ON cookbook_usage_analysis (organisation_id);
CREATE INDEX idx_cookbook_usage_analysis_collection_run_id ON cookbook_usage_analysis (collection_run_id);
CREATE INDEX idx_cookbook_usage_analysis_analysed_at ON cookbook_usage_analysis (analysed_at);

-- cookbook_usage_detail
CREATE INDEX idx_cookbook_usage_detail_analysis_id ON cookbook_usage_detail (analysis_id);
CREATE INDEX idx_cookbook_usage_detail_organisation_id ON cookbook_usage_detail (organisation_id);
CREATE INDEX idx_cookbook_usage_detail_cookbook_name ON cookbook_usage_detail (cookbook_name);
CREATE INDEX idx_cookbook_usage_detail_cookbook_name_version ON cookbook_usage_detail (cookbook_name, cookbook_version);
CREATE INDEX idx_cookbook_usage_detail_is_active ON cookbook_usage_detail (is_active);
CREATE INDEX idx_cookbook_usage_detail_node_count ON cookbook_usage_detail (node_count);

-- cookbook_platform_coverage
ALTER TABLE cookbook_platform_coverage ADD CONSTRAINT uq_cookbook_platform_coverage_name UNIQUE (cookbook_name);

-- export_jobs: no changes needed, was never modified
