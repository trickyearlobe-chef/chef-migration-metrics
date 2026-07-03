-- SPDX-License-Identifier: Apache-2.0
-- Migration 0041: Materialise the classification-derived CookStyle rollup
-- status (the single source of truth) on cookstyle results and on git_repos.
--
-- Chunk 1 derived the 4-state status (ready / needs_review / blocked) in memory
-- but only persisted the back-compat `passed` boolean. Read paths (lists,
-- remediation, badges) must consume the materialised value rather than
-- re-deriving, so the status now lives in a column written on scan and on every
-- reclassification.
--
-- The per-result tables hold ready / needs_review / blocked (untested is
-- caller-assigned when no result exists, never stored). The git_repos rollup
-- column mirrors the existing compatibility_status pattern and adds 'untested'
-- for the no-result case.

ALTER TABLE server_cookbook_cookstyle_results
    ADD COLUMN cookstyle_status TEXT NOT NULL DEFAULT '';

ALTER TABLE git_repo_cookstyle_results
    ADD COLUMN cookstyle_status TEXT NOT NULL DEFAULT '';

ALTER TABLE server_cookbook_cookstyle_results
    ADD CONSTRAINT chk_sc_cookstyle_results_status
        CHECK (cookstyle_status IN ('', 'ready', 'needs_review', 'blocked'));

ALTER TABLE git_repo_cookstyle_results
    ADD CONSTRAINT chk_gr_cookstyle_results_status
        CHECK (cookstyle_status IN ('', 'ready', 'needs_review', 'blocked'));

ALTER TABLE git_repos
    ADD COLUMN cookstyle_status TEXT NOT NULL DEFAULT 'untested';

ALTER TABLE git_repos
    ADD CONSTRAINT chk_git_repos_cookstyle_status
        CHECK (cookstyle_status IN ('untested', 'ready', 'needs_review', 'blocked'));

CREATE INDEX idx_git_repos_cookstyle_status ON git_repos (cookstyle_status);

-- Coarse backfill from the existing `passed` verdict: passed -> ready,
-- not-passed -> blocked. This cannot distinguish ready from needs_review (that
-- needs the offences + classification), so existing rows self-heal to the
-- precise value on the next scan or on any reclassification.
UPDATE server_cookbook_cookstyle_results
    SET cookstyle_status = CASE WHEN passed THEN 'ready' ELSE 'blocked' END
    WHERE cookstyle_status = '';

UPDATE git_repo_cookstyle_results
    SET cookstyle_status = CASE WHEN passed THEN 'ready' ELSE 'blocked' END
    WHERE cookstyle_status = '';

-- Backfill the git_repos rollup from the latest result per repo (coarse, as
-- above). error / no-result rows stay 'untested'.
UPDATE git_repos gr
SET cookstyle_status = CASE
    WHEN cs.error_message != '' THEN 'untested'
    WHEN cs.passed = true THEN 'ready'
    WHEN cs.passed = false THEN 'blocked'
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
