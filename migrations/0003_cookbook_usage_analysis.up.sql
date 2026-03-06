-- =============================================================================
-- Migration 0003: Cookbook Usage Analysis
-- =============================================================================
-- Creates tables for persisting cookbook usage analysis results. These tables
-- store aggregated usage statistics computed after each collection run by the
-- analysis package.
--
-- This migration is managed by golang-migrate/migrate. Do not edit after commit.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1. cookbook_usage_analysis
-- ---------------------------------------------------------------------------
-- Stores per-organisation, per-collection-run analysis snapshots. Each row
-- represents one analysis run and acts as a parent for the detail rows.

CREATE TABLE cookbook_usage_analysis (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id   UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    collection_run_id UUID        NOT NULL REFERENCES collection_runs(id) ON DELETE CASCADE,
    total_cookbooks   INTEGER     NOT NULL DEFAULT 0,
    active_cookbooks  INTEGER     NOT NULL DEFAULT 0,
    unused_cookbooks  INTEGER     NOT NULL DEFAULT 0,
    total_nodes       INTEGER     NOT NULL DEFAULT 0,
    analysed_at       TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cookbook_usage_analysis_organisation_id ON cookbook_usage_analysis (organisation_id);
CREATE INDEX idx_cookbook_usage_analysis_collection_run_id ON cookbook_usage_analysis (collection_run_id);
CREATE INDEX idx_cookbook_usage_analysis_analysed_at ON cookbook_usage_analysis (analysed_at);

-- ---------------------------------------------------------------------------
-- 2. cookbook_usage_detail
-- ---------------------------------------------------------------------------
-- Stores per-cookbook-version usage statistics within a single analysis run.
-- Each row records node counts, platform breakdown, role references, and
-- policy references for one cookbook version.

CREATE TABLE cookbook_usage_detail (
    id                    UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    analysis_id           UUID        NOT NULL REFERENCES cookbook_usage_analysis(id) ON DELETE CASCADE,
    organisation_id       UUID        NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    cookbook_name          TEXT        NOT NULL,
    cookbook_version       TEXT        NOT NULL,
    node_count            INTEGER     NOT NULL DEFAULT 0,
    is_active             BOOLEAN     NOT NULL DEFAULT FALSE,
    node_names            JSONB,
    roles                 JSONB,
    policy_names          JSONB,
    policy_groups         JSONB,
    platform_counts       JSONB,
    platform_family_counts JSONB,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cookbook_usage_detail_analysis_id ON cookbook_usage_detail (analysis_id);
CREATE INDEX idx_cookbook_usage_detail_organisation_id ON cookbook_usage_detail (organisation_id);
CREATE INDEX idx_cookbook_usage_detail_cookbook_name ON cookbook_usage_detail (cookbook_name);
CREATE INDEX idx_cookbook_usage_detail_cookbook_name_version ON cookbook_usage_detail (cookbook_name, cookbook_version);
CREATE INDEX idx_cookbook_usage_detail_is_active ON cookbook_usage_detail (is_active);
CREATE INDEX idx_cookbook_usage_detail_node_count ON cookbook_usage_detail (node_count);
