-- =============================================================================
-- Migration 0001: Initial Schema
-- =============================================================================
-- Creates all tables, indexes, unique constraints, and foreign keys for the
-- Chef Migration Metrics application. See datastore/Specification.md for the
-- full schema documentation.
--
-- This migration is managed by golang-migrate/migrate. Do not edit after commit.
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
    )
);

CREATE INDEX idx_collection_runs_organisation_id ON collection_runs (organisation_id);
CREATE INDEX idx_collection_runs_status ON collection_runs (status);
CREATE INDEX idx_collection_runs_started_at ON collection_runs (started_at);

-- ---------------------------------------------------------------------------
-- 4. node_snapshots
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
    collected_at        TIMESTAMPTZ      NOT NULL,
    created_at          TIMESTAMPTZ      NOT NULL DEFAULT now()
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

-- ---------------------------------------------------------------------------
-- 5. cookbooks
-- ---------------------------------------------------------------------------

CREATE TABLE cookbooks (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id   UUID        REFERENCES organisations(id) ON DELETE CASCADE,
    name              TEXT        NOT NULL,
    version           TEXT,
    source            TEXT        NOT NULL,
    git_repo_url      TEXT,
    head_commit_sha   TEXT,
    default_branch    TEXT,
    has_test_suite    BOOLEAN     NOT NULL DEFAULT FALSE,
    is_active         BOOLEAN     NOT NULL DEFAULT FALSE,
    is_stale_cookbook  BOOLEAN     NOT NULL DEFAULT FALSE,
    first_seen_at     TIMESTAMPTZ,
    last_fetched_at   TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_cookbooks_source CHECK (source IN ('git', 'chef_server'))
);

-- Partial unique constraints for the two source types
CREATE UNIQUE INDEX uq_cookbooks_server ON cookbooks (organisation_id, name, version)
    WHERE source = 'chef_server';
CREATE UNIQUE INDEX uq_cookbooks_git ON cookbooks (name, git_repo_url)
    WHERE source = 'git';

CREATE INDEX idx_cookbooks_organisation_id ON cookbooks (organisation_id);
CREATE INDEX idx_cookbooks_name ON cookbooks (name);
CREATE INDEX idx_cookbooks_source ON cookbooks (source);
CREATE INDEX idx_cookbooks_is_active ON cookbooks (is_active);
CREATE INDEX idx_cookbooks_is_stale_cookbook ON cookbooks (is_stale_cookbook);
CREATE INDEX idx_cookbooks_name_version ON cookbooks (name, version);
CREATE INDEX idx_cookbooks_first_seen_at ON cookbooks (first_seen_at);

-- ---------------------------------------------------------------------------
-- 6. cookbook_node_usage
-- ---------------------------------------------------------------------------

CREATE TABLE cookbook_node_usage (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cookbook_id        UUID        NOT NULL REFERENCES cookbooks(id) ON DELETE CASCADE,
    node_snapshot_id  UUID        NOT NULL REFERENCES node_snapshots(id) ON DELETE CASCADE,
    cookbook_version   TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cookbook_node_usage_cookbook_id ON cookbook_node_usage (cookbook_id);
CREATE INDEX idx_cookbook_node_usage_node_snapshot_id ON cookbook_node_usage (node_snapshot_id);
CREATE INDEX idx_cookbook_node_usage_cookbook_version ON cookbook_node_usage (cookbook_id, cookbook_version);

-- ---------------------------------------------------------------------------
-- 7. cookbook_role_usage
-- ---------------------------------------------------------------------------

CREATE TABLE cookbook_role_usage (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cookbook_name     TEXT        NOT NULL,
    role_name        TEXT        NOT NULL,
    organisation_id  UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_cookbook_role_usage UNIQUE (cookbook_name, role_name, organisation_id)
);

CREATE INDEX idx_cookbook_role_usage_cookbook_name ON cookbook_role_usage (cookbook_name);
CREATE INDEX idx_cookbook_role_usage_role_name ON cookbook_role_usage (role_name);
CREATE INDEX idx_cookbook_role_usage_organisation_id ON cookbook_role_usage (organisation_id);

-- ---------------------------------------------------------------------------
-- 8. test_kitchen_results
-- ---------------------------------------------------------------------------

CREATE TABLE test_kitchen_results (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cookbook_id           UUID        NOT NULL REFERENCES cookbooks(id) ON DELETE CASCADE,
    target_chef_version  TEXT        NOT NULL,
    commit_sha           TEXT        NOT NULL,
    converge_passed      BOOLEAN     NOT NULL,
    tests_passed         BOOLEAN     NOT NULL,
    compatible           BOOLEAN     NOT NULL,
    process_stdout       TEXT,
    process_stderr       TEXT,
    duration_seconds     INTEGER,
    started_at           TIMESTAMPTZ NOT NULL,
    completed_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_test_kitchen_results UNIQUE (cookbook_id, target_chef_version, commit_sha)
);

CREATE INDEX idx_test_kitchen_results_cookbook_id ON test_kitchen_results (cookbook_id);
CREATE INDEX idx_test_kitchen_results_target_chef_version ON test_kitchen_results (target_chef_version);
CREATE INDEX idx_test_kitchen_results_commit_sha ON test_kitchen_results (commit_sha);
CREATE INDEX idx_test_kitchen_results_compatible ON test_kitchen_results (compatible);
CREATE INDEX idx_test_kitchen_results_cookbook_target ON test_kitchen_results (cookbook_id, target_chef_version);

-- ---------------------------------------------------------------------------
-- 9. cookstyle_results
-- ---------------------------------------------------------------------------

CREATE TABLE cookstyle_results (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cookbook_id            UUID        NOT NULL REFERENCES cookbooks(id) ON DELETE CASCADE,
    target_chef_version   TEXT,
    passed                BOOLEAN     NOT NULL,
    offence_count         INTEGER     NOT NULL DEFAULT 0,
    deprecation_count     INTEGER     NOT NULL DEFAULT 0,
    correctness_count     INTEGER     NOT NULL DEFAULT 0,
    deprecation_warnings  JSONB,
    offences              JSONB,
    process_stdout        TEXT,
    process_stderr        TEXT,
    duration_seconds      INTEGER,
    scanned_at            TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_cookstyle_results UNIQUE (cookbook_id, target_chef_version)
);

CREATE INDEX idx_cookstyle_results_cookbook_id ON cookstyle_results (cookbook_id);
CREATE INDEX idx_cookstyle_results_target_chef_version ON cookstyle_results (target_chef_version);
CREATE INDEX idx_cookstyle_results_passed ON cookstyle_results (passed);

-- ---------------------------------------------------------------------------
-- 10. autocorrect_previews
-- ---------------------------------------------------------------------------

CREATE TABLE autocorrect_previews (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cookbook_id           UUID        NOT NULL REFERENCES cookbooks(id) ON DELETE CASCADE,
    cookstyle_result_id  UUID        NOT NULL REFERENCES cookstyle_results(id) ON DELETE CASCADE,
    total_offenses       INTEGER     NOT NULL DEFAULT 0,
    correctable_offenses INTEGER     NOT NULL DEFAULT 0,
    remaining_offenses   INTEGER     NOT NULL DEFAULT 0,
    files_modified       INTEGER     NOT NULL DEFAULT 0,
    diff_output          TEXT,
    generated_at         TIMESTAMPTZ NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_autocorrect_previews_cookstyle UNIQUE (cookstyle_result_id)
);

CREATE INDEX idx_autocorrect_previews_cookbook_id ON autocorrect_previews (cookbook_id);
CREATE INDEX idx_autocorrect_previews_cookstyle_result_id ON autocorrect_previews (cookstyle_result_id);

-- ---------------------------------------------------------------------------
-- 11. cookbook_complexity
-- ---------------------------------------------------------------------------

CREATE TABLE cookbook_complexity (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cookbook_id              UUID        NOT NULL REFERENCES cookbooks(id) ON DELETE CASCADE,
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

    CONSTRAINT uq_cookbook_complexity UNIQUE (cookbook_id, target_chef_version),
    CONSTRAINT chk_cookbook_complexity_label CHECK (
        complexity_label IN ('none', 'low', 'medium', 'high', 'critical')
    )
);

CREATE INDEX idx_cookbook_complexity_cookbook_id ON cookbook_complexity (cookbook_id);
CREATE INDEX idx_cookbook_complexity_target_chef_version ON cookbook_complexity (target_chef_version);
CREATE INDEX idx_cookbook_complexity_complexity_score ON cookbook_complexity (complexity_score);
CREATE INDEX idx_cookbook_complexity_complexity_label ON cookbook_complexity (complexity_label);
CREATE INDEX idx_cookbook_complexity_affected_node_count ON cookbook_complexity (affected_node_count);

-- ---------------------------------------------------------------------------
-- 12. node_readiness
-- ---------------------------------------------------------------------------

CREATE TABLE node_readiness (
    id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    node_snapshot_id          UUID        NOT NULL REFERENCES node_snapshots(id) ON DELETE CASCADE,
    organisation_id           UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    node_name                 TEXT        NOT NULL,
    target_chef_version       TEXT        NOT NULL,
    is_ready                  BOOLEAN     NOT NULL DEFAULT FALSE,
    all_cookbooks_compatible  BOOLEAN     NOT NULL DEFAULT FALSE,
    sufficient_disk_space     BOOLEAN     NOT NULL DEFAULT FALSE,
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

-- ---------------------------------------------------------------------------
-- 13. role_dependencies
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
-- 14. metric_snapshots
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
-- 15. log_entries
-- ---------------------------------------------------------------------------

CREATE TABLE log_entries (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp           TIMESTAMPTZ NOT NULL,
    severity            TEXT        NOT NULL,
    scope               TEXT        NOT NULL,
    message             TEXT        NOT NULL,
    organisation        TEXT,
    cookbook_name        TEXT,
    cookbook_version     TEXT,
    commit_sha          TEXT,
    chef_client_version TEXT,
    process_output      TEXT,
    collection_run_id   UUID        REFERENCES collection_runs(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

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
-- 16. notification_history
-- ---------------------------------------------------------------------------

CREATE TABLE notification_history (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_name   TEXT        NOT NULL,
    channel_type   TEXT        NOT NULL,
    event_type     TEXT        NOT NULL,
    summary        TEXT        NOT NULL,
    payload        JSONB       NOT NULL,
    status         TEXT        NOT NULL,
    error_message  TEXT,
    retry_count    INTEGER     NOT NULL DEFAULT 0,
    sent_at        TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_notification_history_channel_type CHECK (
        channel_type IN ('webhook', 'email')
    ),
    CONSTRAINT chk_notification_history_event_type CHECK (
        event_type IN (
            'cookbook_status_change',
            'readiness_milestone',
            'new_incompatible_cookbook',
            'collection_failure',
            'stale_node_threshold_exceeded'
        )
    ),
    CONSTRAINT chk_notification_history_status CHECK (
        status IN ('sent', 'failed', 'retrying')
    )
);

CREATE INDEX idx_notification_history_event_type ON notification_history (event_type);
CREATE INDEX idx_notification_history_channel_name ON notification_history (channel_name);
CREATE INDEX idx_notification_history_status ON notification_history (status);
CREATE INDEX idx_notification_history_sent_at ON notification_history (sent_at);

-- ---------------------------------------------------------------------------
-- 17. export_jobs
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
        status IN ('pending', 'processing', 'completed', 'failed')
    )
);

CREATE INDEX idx_export_jobs_status ON export_jobs (status);
CREATE INDEX idx_export_jobs_export_type ON export_jobs (export_type);
CREATE INDEX idx_export_jobs_requested_by ON export_jobs (requested_by);
CREATE INDEX idx_export_jobs_expires_at ON export_jobs (expires_at);

-- ---------------------------------------------------------------------------
-- 18. users
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
-- 19. sessions
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
