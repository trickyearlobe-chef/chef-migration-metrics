-- =============================================================================
-- Migration 0001: Initial Schema (Consolidated)
-- =============================================================================
-- Creates all tables, indexes, unique constraints, and foreign keys for the
-- Chef Migration Metrics application.
--
-- This consolidated migration incorporates changes from the original
-- incremental migrations (0002–0010):
--
--   • node_snapshots: UNIQUE (organisation_id, node_name) — entity semantics
--   • collection_runs: UNIQUE (organisation_id) — one row per org
--   • cookbook_usage_analysis: UNIQUE (organisation_id) — one row per org
--   • git_repo_test_kitchen_results: UNIQUE (git_repo_id, target_chef_version)
--     instead of 3-column key including commit_sha
--   • export_jobs: status CHECK includes 'expired'
--   • cookbook_usage_detail: node_names column removed
--   • cookbook_node_usage table removed (dead code)
--   • cookbook_role_usage table removed (dead code)
--   • notification_history table removed (dead code)
--
-- Key design principle: Entity tables store current state (one row per
-- logical thing). Only metric_snapshots is intentionally append-only
-- (timeseries with 90-day retention).
--
-- See datastore/Specification.md for full schema documentation.
-- =============================================================================

-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ---------------------------------------------------------------------------
-- 1. credentials
-- ---------------------------------------------------------------------------
-- Stores encrypted credentials (private keys, passwords, tokens). All
-- sensitive material is encrypted at the application layer using AES-256-GCM
-- before being written. The database never sees plaintext secrets.
-- ---------------------------------------------------------------------------

CREATE TABLE credentials (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT        NOT NULL,
    credential_type  TEXT        NOT NULL,
    encrypted_value  TEXT        NOT NULL,
    metadata         JSONB,
    last_rotated_at  TIMESTAMPTZ,
    created_by       TEXT        NOT NULL,
    updated_by       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_credentials_name UNIQUE (name),
    CONSTRAINT uq_credentials_type_name UNIQUE (credential_type, name),
    CONSTRAINT chk_credentials_type CHECK (
        credential_type IN ('chef_client_key', 'ldap_bind_password', 'smtp_password', 'webhook_url', 'generic')
    )
);

CREATE INDEX idx_credentials_name ON credentials (name);
CREATE INDEX idx_credentials_credential_type ON credentials (credential_type);

-- ---------------------------------------------------------------------------
-- 2. organisations
-- ---------------------------------------------------------------------------

CREATE TABLE organisations (
    id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                      TEXT        NOT NULL,
    chef_server_url           TEXT        NOT NULL,
    org_name                  TEXT        NOT NULL,
    client_name               TEXT        NOT NULL,
    client_key_credential_id  UUID        REFERENCES credentials(id) ON DELETE SET NULL,
    source                    TEXT        NOT NULL,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_organisations_name UNIQUE (name),
    CONSTRAINT uq_organisations_server_org UNIQUE (chef_server_url, org_name),
    CONSTRAINT chk_organisations_source CHECK (source IN ('config', 'api'))
);

CREATE INDEX idx_organisations_name ON organisations (name);
CREATE INDEX idx_organisations_client_key_credential_id ON organisations (client_key_credential_id);

-- ---------------------------------------------------------------------------
-- 3. collection_runs
-- ---------------------------------------------------------------------------
-- Each organisation has at most one collection_runs row (entity semantics).
-- The row is upserted on each collection cycle, resetting status to
-- 'running'. Historical trend data lives in metric_snapshots.
-- ---------------------------------------------------------------------------

CREATE TABLE collection_runs (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id   UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    status            TEXT        NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL,
    completed_at      TIMESTAMPTZ,
    total_nodes       INTEGER,
    nodes_collected   INTEGER,
    checkpoint_start  INTEGER,
    error_message     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_collection_runs_status CHECK (
        status IN ('running', 'completed', 'failed', 'interrupted')
    ),
    CONSTRAINT uq_collection_runs_org UNIQUE (organisation_id)
);

CREATE INDEX idx_collection_runs_organisation_id ON collection_runs (organisation_id);
CREATE INDEX idx_collection_runs_status ON collection_runs (status);
CREATE INDEX idx_collection_runs_started_at ON collection_runs (started_at);
CREATE INDEX idx_collection_runs_org_status_started
    ON collection_runs (organisation_id, status, started_at DESC);

-- ---------------------------------------------------------------------------
-- 4. node_snapshots
-- ---------------------------------------------------------------------------
-- Current-state table: one row per (organisation_id, node_name). Upserted
-- on each collection run. Orphaned rows (decommissioned nodes) are cleaned
-- up by the collector after each run.
-- ---------------------------------------------------------------------------

CREATE TABLE node_snapshots (
    id                  UUID             PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_run_id   UUID             NOT NULL REFERENCES collection_runs(id) ON DELETE CASCADE,
    organisation_id     UUID             NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    node_name           TEXT             NOT NULL,
    chef_environment    TEXT,
    chef_version        TEXT,
    platform            TEXT,
    platform_version    TEXT,
    platform_family     TEXT,
    filesystem          JSONB,
    cookbooks           JSONB,
    run_list            JSONB,
    roles               JSONB,
    policy_name         TEXT,
    policy_group        TEXT,
    ohai_time           DOUBLE PRECISION,
    is_stale            BOOLEAN          NOT NULL DEFAULT FALSE,
    custom_attributes   JSONB,
    collected_at        TIMESTAMPTZ      NOT NULL,
    created_at          TIMESTAMPTZ      NOT NULL DEFAULT now(),

    CONSTRAINT uq_node_snapshots_org_node UNIQUE (organisation_id, node_name)
);

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

-- ---------------------------------------------------------------------------
-- 5. server_cookbooks
-- ---------------------------------------------------------------------------
-- Cookbook versions fetched from Chef Infra Server. Each row represents a
-- single name+version pair scoped to an organisation. Metadata fields
-- (maintainer, description, license, platforms, dependencies) are populated
-- after the cookbook manifest is fetched during the streaming pipeline.
-- ---------------------------------------------------------------------------

CREATE TABLE server_cookbooks (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id   UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    name              TEXT        NOT NULL,
    version           TEXT        NOT NULL,
    is_active         BOOLEAN     NOT NULL DEFAULT FALSE,
    is_stale_cookbook  BOOLEAN     NOT NULL DEFAULT FALSE,
    is_frozen         BOOLEAN     NOT NULL DEFAULT FALSE,
    download_status   TEXT        NOT NULL DEFAULT 'pending',
    download_error    TEXT,
    maintainer        TEXT,
    description       TEXT,
    long_description  TEXT,
    license           TEXT,
    platforms         JSONB,
    dependencies      JSONB,
    first_seen_at     TIMESTAMPTZ,
    last_fetched_at   TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_sc_download_status CHECK (download_status IN ('ok', 'failed', 'pending')),
    UNIQUE (organisation_id, name, version)
);

CREATE INDEX idx_server_cookbooks_organisation_id ON server_cookbooks (organisation_id);
CREATE INDEX idx_server_cookbooks_name ON server_cookbooks (name);
CREATE INDEX idx_server_cookbooks_is_active ON server_cookbooks (is_active);
CREATE INDEX idx_server_cookbooks_is_stale_cookbook ON server_cookbooks (is_stale_cookbook);
CREATE INDEX idx_server_cookbooks_name_version ON server_cookbooks (name, version);
CREATE INDEX idx_server_cookbooks_first_seen_at ON server_cookbooks (first_seen_at);
CREATE INDEX idx_server_cookbooks_download_status ON server_cookbooks (download_status);

-- ---------------------------------------------------------------------------
-- 6. git_repos
-- ---------------------------------------------------------------------------
-- Cookbook source repositories cloned from Git. Not org-scoped — matched by
-- name across organisations. Each row represents a unique cookbook name +
-- git URL combination.
-- ---------------------------------------------------------------------------

CREATE TABLE git_repos (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT        NOT NULL,
    git_repo_url      TEXT        NOT NULL,
    head_commit_sha   TEXT,
    default_branch    TEXT,
    has_test_suite    BOOLEAN     NOT NULL DEFAULT FALSE,
    last_fetched_at   TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (name, git_repo_url)
);

CREATE INDEX idx_git_repos_name ON git_repos (name);
CREATE INDEX idx_git_repos_git_repo_url ON git_repos (git_repo_url);

-- ---------------------------------------------------------------------------
-- 7. server_cookbook_cookstyle_results
-- ---------------------------------------------------------------------------
-- Cookstyle scan results for server cookbook versions. Server cookbook versions
-- are immutable, so re-scanning is skipped when a result already exists for
-- the same cookbook_id + target_chef_version.
-- ---------------------------------------------------------------------------

CREATE TABLE server_cookbook_cookstyle_results (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    server_cookbook_id     UUID        NOT NULL REFERENCES server_cookbooks(id) ON DELETE CASCADE,
    target_chef_version   TEXT        NOT NULL,
    passed                BOOLEAN     NOT NULL DEFAULT FALSE,
    offence_count         INTEGER     NOT NULL DEFAULT 0,
    deprecation_count     INTEGER     NOT NULL DEFAULT 0,
    correctness_count     INTEGER     NOT NULL DEFAULT 0,
    deprecation_warnings  JSONB,
    offences              JSONB,
    process_stdout        TEXT,
    process_stderr        TEXT,
    duration_seconds      INTEGER     NOT NULL DEFAULT 0,
    scanned_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (server_cookbook_id, target_chef_version)
);

CREATE INDEX idx_sc_cookstyle_results_server_cookbook_id ON server_cookbook_cookstyle_results (server_cookbook_id);
CREATE INDEX idx_sc_cookstyle_results_target_chef_version ON server_cookbook_cookstyle_results (target_chef_version);
CREATE INDEX idx_sc_cookstyle_results_passed ON server_cookbook_cookstyle_results (passed);

-- ---------------------------------------------------------------------------
-- 8. git_repo_cookstyle_results
-- ---------------------------------------------------------------------------
-- Cookstyle scan results for git repos. Git repo content changes with each
-- commit, so re-scanning is skipped only when HEAD commit has not changed.
-- ---------------------------------------------------------------------------

CREATE TABLE git_repo_cookstyle_results (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    git_repo_id           UUID        NOT NULL REFERENCES git_repos(id) ON DELETE CASCADE,
    target_chef_version   TEXT        NOT NULL,
    commit_sha            TEXT,
    passed                BOOLEAN     NOT NULL DEFAULT FALSE,
    offence_count         INTEGER     NOT NULL DEFAULT 0,
    deprecation_count     INTEGER     NOT NULL DEFAULT 0,
    correctness_count     INTEGER     NOT NULL DEFAULT 0,
    deprecation_warnings  JSONB,
    offences              JSONB,
    process_stdout        TEXT,
    process_stderr        TEXT,
    duration_seconds      INTEGER     NOT NULL DEFAULT 0,
    scanned_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (git_repo_id, target_chef_version)
);

CREATE INDEX idx_gr_cookstyle_results_git_repo_id ON git_repo_cookstyle_results (git_repo_id);
CREATE INDEX idx_gr_cookstyle_results_target_chef_version ON git_repo_cookstyle_results (target_chef_version);
CREATE INDEX idx_gr_cookstyle_results_passed ON git_repo_cookstyle_results (passed);
CREATE INDEX idx_gr_cookstyle_results_commit_sha ON git_repo_cookstyle_results (commit_sha)
    WHERE commit_sha IS NOT NULL;

-- ---------------------------------------------------------------------------
-- 9. git_repo_test_kitchen_results
-- ---------------------------------------------------------------------------
-- Test Kitchen is only run against git repos (which have a .kitchen.yml).
-- Server cookbooks do not have test suites. Entity semantics: one row per
-- (git_repo_id, target_chef_version). commit_sha records which commit was
-- tested but is not part of the uniqueness key.
-- ---------------------------------------------------------------------------

CREATE TABLE git_repo_test_kitchen_results (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    git_repo_id          UUID        NOT NULL REFERENCES git_repos(id) ON DELETE CASCADE,
    target_chef_version  TEXT        NOT NULL,
    commit_sha           TEXT        NOT NULL,
    converge_passed      BOOLEAN     NOT NULL,
    tests_passed         BOOLEAN     NOT NULL,
    compatible           BOOLEAN     NOT NULL,
    process_stdout       TEXT,
    process_stderr       TEXT,
    converge_output      TEXT,
    verify_output        TEXT,
    destroy_output       TEXT,
    timed_out            BOOLEAN     NOT NULL DEFAULT FALSE,
    driver_used          TEXT,
    platform_tested      TEXT,
    overrides_applied    BOOLEAN     NOT NULL DEFAULT FALSE,
    duration_seconds     INTEGER,
    started_at           TIMESTAMPTZ NOT NULL,
    completed_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_git_repo_test_kitchen_results UNIQUE (git_repo_id, target_chef_version)
);

CREATE INDEX idx_gr_test_kitchen_results_git_repo_id ON git_repo_test_kitchen_results (git_repo_id);
CREATE INDEX idx_gr_test_kitchen_results_target_chef_version ON git_repo_test_kitchen_results (target_chef_version);
CREATE INDEX idx_gr_test_kitchen_results_commit_sha ON git_repo_test_kitchen_results (commit_sha);
CREATE INDEX idx_gr_test_kitchen_results_compatible ON git_repo_test_kitchen_results (compatible);
CREATE INDEX idx_gr_test_kitchen_results_repo_target ON git_repo_test_kitchen_results (git_repo_id, target_chef_version);

-- ---------------------------------------------------------------------------
-- 10. server_cookbook_autocorrect_previews
-- ---------------------------------------------------------------------------

CREATE TABLE server_cookbook_autocorrect_previews (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    server_cookbook_id    UUID        NOT NULL REFERENCES server_cookbooks(id) ON DELETE CASCADE,
    cookstyle_result_id  UUID        NOT NULL REFERENCES server_cookbook_cookstyle_results(id) ON DELETE CASCADE,
    total_offenses       INTEGER     NOT NULL DEFAULT 0,
    correctable_offenses INTEGER     NOT NULL DEFAULT 0,
    remaining_offenses   INTEGER     NOT NULL DEFAULT 0,
    files_modified       INTEGER     NOT NULL DEFAULT 0,
    diff_output          TEXT,
    generated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_sc_autocorrect_previews_cookstyle UNIQUE (cookstyle_result_id)
);

CREATE INDEX idx_sc_autocorrect_previews_server_cookbook_id ON server_cookbook_autocorrect_previews (server_cookbook_id);
CREATE INDEX idx_sc_autocorrect_previews_cookstyle_result_id ON server_cookbook_autocorrect_previews (cookstyle_result_id);

-- ---------------------------------------------------------------------------
-- 11. git_repo_autocorrect_previews
-- ---------------------------------------------------------------------------

CREATE TABLE git_repo_autocorrect_previews (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    git_repo_id          UUID        NOT NULL REFERENCES git_repos(id) ON DELETE CASCADE,
    cookstyle_result_id  UUID        NOT NULL REFERENCES git_repo_cookstyle_results(id) ON DELETE CASCADE,
    total_offenses       INTEGER     NOT NULL DEFAULT 0,
    correctable_offenses INTEGER     NOT NULL DEFAULT 0,
    remaining_offenses   INTEGER     NOT NULL DEFAULT 0,
    files_modified       INTEGER     NOT NULL DEFAULT 0,
    diff_output          TEXT,
    generated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_gr_autocorrect_previews_cookstyle UNIQUE (cookstyle_result_id)
);

CREATE INDEX idx_gr_autocorrect_previews_git_repo_id ON git_repo_autocorrect_previews (git_repo_id);
CREATE INDEX idx_gr_autocorrect_previews_cookstyle_result_id ON git_repo_autocorrect_previews (cookstyle_result_id);

-- ---------------------------------------------------------------------------
-- 12. server_cookbook_complexity
-- ---------------------------------------------------------------------------

CREATE TABLE server_cookbook_complexity (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    server_cookbook_id       UUID        NOT NULL REFERENCES server_cookbooks(id) ON DELETE CASCADE,
    target_chef_version     TEXT        NOT NULL,
    complexity_score        INTEGER     NOT NULL DEFAULT 0,
    complexity_label        TEXT        NOT NULL DEFAULT 'none',
    error_count             INTEGER     NOT NULL DEFAULT 0,
    deprecation_count       INTEGER     NOT NULL DEFAULT 0,
    correctness_count       INTEGER     NOT NULL DEFAULT 0,
    modernize_count         INTEGER     NOT NULL DEFAULT 0,
    auto_correctable_count  INTEGER     NOT NULL DEFAULT 0,
    manual_fix_count        INTEGER     NOT NULL DEFAULT 0,
    affected_node_count     INTEGER     NOT NULL DEFAULT 0,
    affected_role_count     INTEGER     NOT NULL DEFAULT 0,
    affected_policy_count   INTEGER     NOT NULL DEFAULT 0,
    evaluated_at            TIMESTAMPTZ NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_sc_cookbook_complexity UNIQUE (server_cookbook_id, target_chef_version),
    CONSTRAINT chk_sc_cookbook_complexity_label CHECK (
        complexity_label IN ('none', 'low', 'medium', 'high', 'critical')
    )
);

CREATE INDEX idx_sc_complexity_server_cookbook_id ON server_cookbook_complexity (server_cookbook_id);
CREATE INDEX idx_sc_complexity_target_chef_version ON server_cookbook_complexity (target_chef_version);
CREATE INDEX idx_sc_complexity_score ON server_cookbook_complexity (complexity_score);
CREATE INDEX idx_sc_complexity_label ON server_cookbook_complexity (complexity_label);
CREATE INDEX idx_sc_complexity_affected_node_count ON server_cookbook_complexity (affected_node_count);

-- ---------------------------------------------------------------------------
-- 13. git_repo_complexity
-- ---------------------------------------------------------------------------

CREATE TABLE git_repo_complexity (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    git_repo_id             UUID        NOT NULL REFERENCES git_repos(id) ON DELETE CASCADE,
    target_chef_version     TEXT        NOT NULL,
    complexity_score        INTEGER     NOT NULL DEFAULT 0,
    complexity_label        TEXT        NOT NULL DEFAULT 'none',
    error_count             INTEGER     NOT NULL DEFAULT 0,
    deprecation_count       INTEGER     NOT NULL DEFAULT 0,
    correctness_count       INTEGER     NOT NULL DEFAULT 0,
    modernize_count         INTEGER     NOT NULL DEFAULT 0,
    auto_correctable_count  INTEGER     NOT NULL DEFAULT 0,
    manual_fix_count        INTEGER     NOT NULL DEFAULT 0,
    affected_node_count     INTEGER     NOT NULL DEFAULT 0,
    affected_role_count     INTEGER     NOT NULL DEFAULT 0,
    affected_policy_count   INTEGER     NOT NULL DEFAULT 0,
    evaluated_at            TIMESTAMPTZ NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_gr_cookbook_complexity UNIQUE (git_repo_id, target_chef_version),
    CONSTRAINT chk_gr_cookbook_complexity_label CHECK (
        complexity_label IN ('none', 'low', 'medium', 'high', 'critical')
    )
);

CREATE INDEX idx_gr_complexity_git_repo_id ON git_repo_complexity (git_repo_id);
CREATE INDEX idx_gr_complexity_target_chef_version ON git_repo_complexity (target_chef_version);
CREATE INDEX idx_gr_complexity_score ON git_repo_complexity (complexity_score);
CREATE INDEX idx_gr_complexity_label ON git_repo_complexity (complexity_label);
CREATE INDEX idx_gr_complexity_affected_node_count ON git_repo_complexity (affected_node_count);

-- ---------------------------------------------------------------------------
-- 14. node_readiness
-- ---------------------------------------------------------------------------

CREATE TABLE node_readiness (
    id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    node_snapshot_id          UUID        NOT NULL REFERENCES node_snapshots(id) ON DELETE CASCADE,
    organisation_id           UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    node_name                 TEXT        NOT NULL,
    target_chef_version       TEXT        NOT NULL,
    is_ready                  BOOLEAN     NOT NULL DEFAULT FALSE,
    all_cookbooks_compatible  BOOLEAN     NOT NULL DEFAULT FALSE,
    sufficient_disk_space     BOOLEAN,
    blocking_cookbooks        JSONB,
    available_disk_mb         INTEGER,
    required_disk_mb          INTEGER,
    stale_data                BOOLEAN     NOT NULL DEFAULT FALSE,
    evaluated_at              TIMESTAMPTZ NOT NULL,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_node_readiness UNIQUE (node_snapshot_id, target_chef_version)
);

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

-- ---------------------------------------------------------------------------
-- 15. role_dependencies
-- ---------------------------------------------------------------------------

CREATE TABLE role_dependencies (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id  UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    role_name        TEXT        NOT NULL,
    dependency_type  TEXT        NOT NULL,
    dependency_name  TEXT        NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_role_dependencies UNIQUE (organisation_id, role_name, dependency_type, dependency_name),
    CONSTRAINT chk_role_dependencies_type CHECK (dependency_type IN ('role', 'cookbook'))
);

CREATE INDEX idx_role_dependencies_organisation_id ON role_dependencies (organisation_id);
CREATE INDEX idx_role_dependencies_role_name ON role_dependencies (role_name);
CREATE INDEX idx_role_dependencies_dependency_type ON role_dependencies (dependency_type);
CREATE INDEX idx_role_dependencies_dependency_name ON role_dependencies (dependency_name);

-- ---------------------------------------------------------------------------
-- 16. metric_snapshots
-- ---------------------------------------------------------------------------
-- The only intentionally append-only (timeseries) table. Pre-aggregated
-- data for dashboard trend charts. Retention: 90 days, purged by the
-- collector at the end of each run.
-- ---------------------------------------------------------------------------

CREATE TABLE metric_snapshots (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_run_id    UUID        REFERENCES collection_runs(id) ON DELETE SET NULL,
    organisation_id      UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    snapshot_type        TEXT        NOT NULL,
    target_chef_version  TEXT,
    data                 JSONB       NOT NULL,
    snapshot_at          TIMESTAMPTZ NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_metric_snapshots_type CHECK (
        snapshot_type IN ('chef_version_distribution', 'readiness_summary', 'cookbook_compatibility')
    )
);

CREATE INDEX idx_metric_snapshots_organisation_id ON metric_snapshots (organisation_id);
CREATE INDEX idx_metric_snapshots_snapshot_type ON metric_snapshots (snapshot_type);
CREATE INDEX idx_metric_snapshots_snapshot_at ON metric_snapshots (snapshot_at);
CREATE INDEX idx_metric_snapshots_target_chef_version ON metric_snapshots (target_chef_version);

-- ---------------------------------------------------------------------------
-- 17. log_entries
-- ---------------------------------------------------------------------------

CREATE TABLE log_entries (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp             TIMESTAMPTZ NOT NULL,
    severity              TEXT        NOT NULL,
    scope                 TEXT        NOT NULL,
    message               TEXT        NOT NULL,
    organisation          TEXT,
    cookbook_name          TEXT,
    cookbook_version       TEXT,
    commit_sha            TEXT,
    chef_client_version   TEXT,
    process_output        TEXT,
    notification_channel  TEXT,
    export_job_id         TEXT,
    tls_domain            TEXT,
    collection_run_id     UUID        REFERENCES collection_runs(id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_log_entries_severity CHECK (
        severity IN ('DEBUG', 'INFO', 'WARN', 'ERROR')
    )
);

CREATE INDEX idx_log_entries_timestamp ON log_entries (timestamp);
CREATE INDEX idx_log_entries_severity ON log_entries (severity);
CREATE INDEX idx_log_entries_scope ON log_entries (scope);
CREATE INDEX idx_log_entries_organisation ON log_entries (organisation);
CREATE INDEX idx_log_entries_cookbook_name ON log_entries (cookbook_name);
CREATE INDEX idx_log_entries_collection_run_id ON log_entries (collection_run_id);
CREATE INDEX idx_log_entries_retention ON log_entries (timestamp);

-- ---------------------------------------------------------------------------
-- 18. export_jobs
-- ---------------------------------------------------------------------------

CREATE TABLE export_jobs (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    export_type      TEXT        NOT NULL,
    format           TEXT        NOT NULL,
    filters          JSONB       NOT NULL DEFAULT '{}',
    status           TEXT        NOT NULL DEFAULT 'pending',
    row_count        INTEGER,
    file_path        TEXT,
    file_size_bytes  BIGINT,
    error_message    TEXT,
    requested_by     TEXT        NOT NULL,
    requested_at     TIMESTAMPTZ NOT NULL,
    completed_at     TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_export_jobs_type CHECK (
        export_type IN ('ready_nodes', 'blocked_nodes', 'cookbook_remediation')
    ),
    CONSTRAINT chk_export_jobs_format CHECK (
        format IN ('csv', 'json', 'chef_search_query')
    ),
    CONSTRAINT chk_export_jobs_status CHECK (
        status IN ('pending', 'processing', 'completed', 'failed', 'expired')
    )
);

CREATE INDEX idx_export_jobs_status ON export_jobs (status);
CREATE INDEX idx_export_jobs_export_type ON export_jobs (export_type);
CREATE INDEX idx_export_jobs_requested_by ON export_jobs (requested_by);
CREATE INDEX idx_export_jobs_expires_at ON export_jobs (expires_at);

-- ---------------------------------------------------------------------------
-- 19. users
-- ---------------------------------------------------------------------------

CREATE TABLE users (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username               TEXT        NOT NULL,
    display_name           TEXT,
    email                  TEXT,
    password_hash          TEXT        NOT NULL,
    role                   TEXT        NOT NULL DEFAULT 'viewer',
    auth_provider          TEXT        NOT NULL DEFAULT 'local',
    is_locked              BOOLEAN     NOT NULL DEFAULT FALSE,
    failed_login_attempts  INTEGER     NOT NULL DEFAULT 0,
    last_login_at          TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_users_username UNIQUE (username),
    CONSTRAINT chk_users_role CHECK (role IN ('admin', 'viewer')),
    CONSTRAINT chk_users_auth_provider CHECK (auth_provider IN ('local', 'ldap', 'saml'))
);

CREATE INDEX idx_users_username ON users (username);
CREATE INDEX idx_users_auth_provider ON users (auth_provider);

-- ---------------------------------------------------------------------------
-- 20. sessions
-- ---------------------------------------------------------------------------

CREATE TABLE sessions (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID        REFERENCES users(id) ON DELETE CASCADE,
    username       TEXT        NOT NULL,
    auth_provider  TEXT        NOT NULL,
    role           TEXT        NOT NULL,
    expires_at     TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_sessions_auth_provider CHECK (auth_provider IN ('local', 'ldap', 'saml')),
    CONSTRAINT chk_sessions_role CHECK (role IN ('admin', 'viewer'))
);

CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

-- ---------------------------------------------------------------------------
-- 21. cookbook_usage_analysis
-- ---------------------------------------------------------------------------
-- Current-state: one row per organisation. Upserted on each analysis run.
-- Acts as a parent for the detail rows (cascade delete).
-- ---------------------------------------------------------------------------

CREATE TABLE cookbook_usage_analysis (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id   UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    collection_run_id UUID        NOT NULL REFERENCES collection_runs(id) ON DELETE CASCADE,
    total_cookbooks   INTEGER     NOT NULL DEFAULT 0,
    active_cookbooks  INTEGER     NOT NULL DEFAULT 0,
    unused_cookbooks  INTEGER     NOT NULL DEFAULT 0,
    total_nodes       INTEGER     NOT NULL DEFAULT 0,
    analysed_at       TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_cookbook_usage_analysis_org UNIQUE (organisation_id)
);

CREATE INDEX idx_cookbook_usage_analysis_organisation_id ON cookbook_usage_analysis (organisation_id);
CREATE INDEX idx_cookbook_usage_analysis_collection_run_id ON cookbook_usage_analysis (collection_run_id);
CREATE INDEX idx_cookbook_usage_analysis_analysed_at ON cookbook_usage_analysis (analysed_at);

-- ---------------------------------------------------------------------------
-- 22. cookbook_usage_detail
-- ---------------------------------------------------------------------------
-- Per-cookbook-version usage statistics within a single analysis run.
-- References cookbooks by name (strings), not by FK to server_cookbooks.
-- The node_names JSONB column was removed — node_count (integer) captures
-- the aggregate, and the full node list is derivable from
-- node_snapshots.cookbooks if ever needed.
-- ---------------------------------------------------------------------------

CREATE TABLE cookbook_usage_detail (
    id                     UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    analysis_id            UUID        NOT NULL REFERENCES cookbook_usage_analysis(id) ON DELETE CASCADE,
    organisation_id        UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    cookbook_name           TEXT        NOT NULL,
    cookbook_version        TEXT        NOT NULL,
    node_count             INTEGER     NOT NULL DEFAULT 0,
    is_active              BOOLEAN     NOT NULL DEFAULT FALSE,
    roles                  JSONB,
    policy_names           JSONB,
    policy_groups          JSONB,
    platform_counts        JSONB,
    platform_family_counts JSONB,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cookbook_usage_detail_analysis_id ON cookbook_usage_detail (analysis_id);
CREATE INDEX idx_cookbook_usage_detail_organisation_id ON cookbook_usage_detail (organisation_id);
CREATE INDEX idx_cookbook_usage_detail_cookbook_name ON cookbook_usage_detail (cookbook_name);
CREATE INDEX idx_cookbook_usage_detail_cookbook_name_version ON cookbook_usage_detail (cookbook_name, cookbook_version);
CREATE INDEX idx_cookbook_usage_detail_is_active ON cookbook_usage_detail (is_active);
CREATE INDEX idx_cookbook_usage_detail_node_count ON cookbook_usage_detail (node_count);

-- ---------------------------------------------------------------------------
-- 23. owners
-- ---------------------------------------------------------------------------
-- Named owners representing teams, individuals, or cost centres.
-- ---------------------------------------------------------------------------

CREATE TABLE owners (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT        NOT NULL,
    display_name    TEXT,
    contact_email   TEXT,
    contact_channel TEXT,
    owner_type      TEXT        NOT NULL CHECK (owner_type IN ('team', 'individual', 'business_unit', 'cost_centre', 'custom')),
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT owners_name_unique UNIQUE (name),
    CONSTRAINT owners_name_format CHECK (name ~ '^[a-z0-9][a-z0-9._-]*$')
);

CREATE INDEX idx_owners_owner_type ON owners (owner_type);

-- ---------------------------------------------------------------------------
-- 24. ownership_assignments
-- ---------------------------------------------------------------------------
-- Many-to-many links between owners and entities (nodes, cookbooks, git
-- repos, roles, policies).
-- ---------------------------------------------------------------------------

CREATE TABLE ownership_assignments (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id          UUID        NOT NULL REFERENCES owners (id) ON DELETE CASCADE,
    entity_type       TEXT        NOT NULL CHECK (entity_type IN ('node', 'cookbook', 'git_repo', 'role', 'policy')),
    entity_key        TEXT        NOT NULL,
    organisation_id   UUID        REFERENCES organisations (id) ON DELETE CASCADE,
    assignment_source TEXT        NOT NULL CHECK (assignment_source IN ('manual', 'auto_rule', 'import')),
    auto_rule_name    TEXT,
    confidence        TEXT        NOT NULL CHECK (confidence IN ('definitive', 'inferred')),
    notes             TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_ownership_assignments_unique
    ON ownership_assignments (owner_id, entity_type, entity_key, COALESCE(organisation_id, '00000000-0000-0000-0000-000000000000'));

CREATE INDEX idx_ownership_assignments_owner_id     ON ownership_assignments (owner_id);
CREATE INDEX idx_ownership_assignments_entity       ON ownership_assignments (entity_type, entity_key);
CREATE INDEX idx_ownership_assignments_org          ON ownership_assignments (organisation_id) WHERE organisation_id IS NOT NULL;
CREATE INDEX idx_ownership_assignments_source       ON ownership_assignments (assignment_source);
CREATE INDEX idx_ownership_assignments_auto_rule    ON ownership_assignments (auto_rule_name) WHERE auto_rule_name IS NOT NULL;

-- ---------------------------------------------------------------------------
-- 25. git_repo_committers
-- ---------------------------------------------------------------------------
-- Committer history extracted from git repositories.
-- ---------------------------------------------------------------------------

CREATE TABLE git_repo_committers (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    git_repo_url    TEXT        NOT NULL,
    author_name     TEXT        NOT NULL,
    author_email    TEXT        NOT NULL,
    commit_count    INTEGER     NOT NULL DEFAULT 0,
    first_commit_at TIMESTAMPTZ NOT NULL,
    last_commit_at  TIMESTAMPTZ NOT NULL,
    collected_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT git_repo_committers_unique UNIQUE (git_repo_url, author_email)
);

CREATE INDEX idx_git_repo_committers_repo ON git_repo_committers (git_repo_url);

-- ---------------------------------------------------------------------------
-- 26. ownership_audit_log
-- ---------------------------------------------------------------------------
-- Append-only log of all ownership mutations. Retention: configurable,
-- default 365 days.
-- ---------------------------------------------------------------------------

CREATE TABLE ownership_audit_log (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT now(),
    action          TEXT        NOT NULL CHECK (action IN (
        'owner_created', 'owner_updated', 'owner_deleted',
        'assignment_created', 'assignment_deleted', 'assignment_reassigned'
    )),
    actor           TEXT        NOT NULL,
    owner_name      TEXT        NOT NULL,
    entity_type     TEXT,
    entity_key      TEXT,
    organisation    TEXT,
    details         JSONB,

    CONSTRAINT ownership_audit_log_action_entity CHECK (
        -- owner-level actions don't need entity fields
        (action IN ('owner_created', 'owner_updated', 'owner_deleted'))
        OR
        -- assignment-level actions require entity fields
        (entity_type IS NOT NULL AND entity_key IS NOT NULL)
    )
);

CREATE INDEX idx_ownership_audit_log_timestamp  ON ownership_audit_log (timestamp DESC);
CREATE INDEX idx_ownership_audit_log_action     ON ownership_audit_log (action);
CREATE INDEX idx_ownership_audit_log_owner      ON ownership_audit_log (owner_name);
CREATE INDEX idx_ownership_audit_log_actor      ON ownership_audit_log (actor);
CREATE INDEX idx_ownership_audit_log_entity     ON ownership_audit_log (entity_type, entity_key) WHERE entity_type IS NOT NULL;
