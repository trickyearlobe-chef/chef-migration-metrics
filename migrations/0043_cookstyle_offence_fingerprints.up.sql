-- Append-only, change-deduped per-scan offence fingerprint history. Retains the
-- minimal per-scan inputs (cop_name, count, severity, correctable) needed to
-- re-derive a cookstyle result's rollup status and weighted complexity under the
-- CURRENT classification — making trends recomputable for data captured AFTER this
-- ships (retroactive-forward). Past points stay frozen (raw inputs never existed).
-- See specifications/estate-progress.md for why trends must be recomputable.
--
-- One row = one result's fingerprint, valid from scanned_at until the next row for
-- the same result. A new row is appended only when the fingerprint differs from
-- that result's most recent stored fingerprint (dedupe on change), so row count
-- scales with churn, not scan cadence.

CREATE TABLE cookstyle_offence_fingerprints (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    -- 'server_cookbook' | 'git_repo' — discriminates the result identity below.
    result_kind         TEXT        NOT NULL,
    -- Server-cookbook result identity (NULL for git results).
    organisation_name   TEXT,
    cookbook_name       TEXT,
    cookbook_version    TEXT,
    -- Git-repo result identity (NULL for server-cookbook results).
    git_repo_name       TEXT,
    git_repo_url        TEXT,
    -- Shared identity component (NULL == the untargeted/legacy result).
    target_chef_version TEXT,
    -- sha256 hex of the canonical cops projection; used for cheap change-dedupe.
    fingerprint_hash    TEXT        NOT NULL,
    -- [{cop_name, count, severity, correctable}, ...] — the minimal projection.
    cops                JSONB       NOT NULL,
    scanned_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Latest-fingerprint-for-a-result lookups (dedupe write + valid-at-T recompute):
-- one index per result kind, identity columns + scanned_at DESC.
CREATE INDEX idx_csofp_server
    ON cookstyle_offence_fingerprints (organisation_name, cookbook_name, cookbook_version, target_chef_version, scanned_at DESC)
 WHERE result_kind = 'server_cookbook';

CREATE INDEX idx_csofp_git
    ON cookstyle_offence_fingerprints (git_repo_name, git_repo_url, target_chef_version, scanned_at DESC)
 WHERE result_kind = 'git_repo';

-- Time-range scans for trend recompute across all results.
CREATE INDEX idx_csofp_scanned_at ON cookstyle_offence_fingerprints (scanned_at);
