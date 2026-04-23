-- SPDX-License-Identifier: Apache-2.0
--
-- Migration 0012: Kitchen analysis tables

CREATE TABLE kitchen_analysis_results (
    git_repo_name        TEXT        NOT NULL,
    git_repo_url         TEXT        NOT NULL,
    analysed_at          TIMESTAMPTZ NOT NULL,
    head_commit_sha      TEXT        NOT NULL,
    kitchen_files        JSONB       NOT NULL DEFAULT '[]',
    has_local_override   BOOLEAN     NOT NULL DEFAULT false,
    local_override_keys  JSONB,
    driver_name          TEXT,
    provisioner_name     TEXT,
    require_chef_omnibus BOOLEAN,
    platforms            JSONB       NOT NULL DEFAULT '[]',
    suites               JSONB       NOT NULL DEFAULT '[]',
    transport_type       TEXT,
    extensions           JSONB,
    variant_files        JSONB,
    error_message        TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (git_repo_name, git_repo_url),

    CONSTRAINT fk_kitchen_analysis_repo
        FOREIGN KEY (git_repo_name, git_repo_url)
        REFERENCES git_repos(name, git_repo_url) ON DELETE CASCADE
);

CREATE INDEX idx_kitchen_analysis_driver ON kitchen_analysis_results (driver_name);

CREATE TABLE kitchen_discovered_platforms (
    platform_name     TEXT        NOT NULL,
    normalised_name   TEXT        NOT NULL,
    os_family         TEXT        NOT NULL,
    os_version        TEXT,
    cookbook_count     INTEGER     NOT NULL DEFAULT 0,
    has_extensions    BOOLEAN     NOT NULL DEFAULT false,
    common_extensions JSONB,
    transport_type    TEXT,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (platform_name)
);
