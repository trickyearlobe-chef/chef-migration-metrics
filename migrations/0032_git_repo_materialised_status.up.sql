-- SPDX-License-Identifier: Apache-2.0
-- Migration 0032: Add materialised status columns to git_repos.
--
-- These columns store pre-computed compatibility and TK status for the active
-- target Chef version. They are updated by recomputation functions whenever
-- cookstyle or kitchen results change. This eliminates the need to load ALL
-- cookstyle and kitchen results into memory for the git repos list endpoint.

ALTER TABLE git_repos
    ADD COLUMN compatibility_status TEXT NOT NULL DEFAULT 'untested',
    ADD COLUMN tk_status            TEXT NOT NULL DEFAULT 'untested',
    ADD COLUMN tk_passed            INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN tk_total             INTEGER NOT NULL DEFAULT 0;

-- CHECK constraints to enforce vocabulary.
ALTER TABLE git_repos
    ADD CONSTRAINT chk_git_repos_compatibility_status
        CHECK (compatibility_status IN ('untested', 'compatible', 'incompatible', 'error')),
    ADD CONSTRAINT chk_git_repos_tk_status
        CHECK (tk_status IN ('untested', 'passed', 'failed', 'partial'));

-- Index for filtering and sorting by status columns.
CREATE INDEX idx_git_repos_compatibility_status ON git_repos (compatibility_status);
CREATE INDEX idx_git_repos_tk_status ON git_repos (tk_status);

-- Backfill compatibility_status from existing cookstyle results.
-- Uses the most recent result per (git_repo_name, git_repo_url) for the
-- currently configured target version. Since we cannot read app config from
-- SQL, we backfill ALL target versions found and the app will re-run this
-- for the active target on startup if needed.
UPDATE git_repos gr
SET compatibility_status = CASE
    WHEN cs.error_message != '' THEN 'error'
    WHEN cs.passed = true THEN 'compatible'
    WHEN cs.passed = false THEN 'incompatible'
    ELSE 'untested'
END
FROM (
    SELECT DISTINCT ON (git_repo_name, git_repo_url)
        git_repo_name, git_repo_url, passed, error_message
    FROM git_repo_cookstyle_results
    ORDER BY git_repo_name, git_repo_url, scanned_at DESC
) cs
WHERE gr.name = cs.git_repo_name
  AND gr.git_repo_url = cs.git_repo_url;

-- Backfill tk_status, tk_passed, tk_total from active kitchen results.
UPDATE git_repos gr
SET tk_passed = counts.passed_count,
    tk_total  = counts.total_count,
    tk_status = CASE
        WHEN counts.passed_count > 0 AND counts.failed_count > 0 THEN 'partial'
        WHEN counts.failed_count > 0 THEN 'failed'
        WHEN counts.passed_count > 0 THEN 'passed'
        ELSE 'untested'
    END
FROM (
    SELECT
        git_repo_name,
        git_repo_url,
        COUNT(*) FILTER (WHERE passed = true) AS passed_count,
        COUNT(*) FILTER (WHERE passed = false OR timed_out = true) AS failed_count,
        COUNT(*) FILTER (WHERE passed IS NOT NULL OR timed_out = true) AS total_count
    FROM git_kitchen_results_active
    GROUP BY git_repo_name, git_repo_url
) counts
WHERE gr.name = counts.git_repo_name
  AND gr.git_repo_url = counts.git_repo_url;
