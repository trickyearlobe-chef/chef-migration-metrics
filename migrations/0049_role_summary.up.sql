-- SPDX-License-Identifier: Apache-2.0
-- Migration 0049: Materialised per-role summary table.
--
-- role_summary stores pre-computed per-role aggregates so the roles list can
-- sort and filter by derived fields (node_count, incompatible_cookbook_count,
-- compatibility_status, tk_status) with indexed reads and SQL pagination,
-- instead of recomputing a recursive transitive-dependency CTE over ALL roles
-- on every request, which is far too slow to serve a page from.
--
-- Grain: (organisation_name, role_name) — the authoritative per-org role
-- registry, mirroring role_dependencies. The list rolls these rows up across
-- organisations (few enough that the rollup is free). Mirrors the proven
-- git_repos materialised-column pattern (migration 0032).
--
-- Columns split into version-independent (node_count, cookbook counts) and
-- active-target (compat + tk) groups, matching git_repos.
--
-- This migration creates the schema only. The table is populated and kept
-- fresh by the recompute functions (single source of truth for the derivation),
-- fired at collection completion, target-version change, cookstyle
-- rescore/reclassification, and kitchen-exclusion change. No SQL backfill here:
-- duplicating the recursive derivation in both a migration and Go would risk
-- drift, so the recompute layer owns it and runs at startup.

CREATE TABLE role_summary (
    organisation_name         TEXT        NOT NULL,
    role_name                 TEXT        NOT NULL,

    -- Version-independent (recomputed at collection).
    node_count                INTEGER     NOT NULL DEFAULT 0,
    direct_cookbook_count     INTEGER     NOT NULL DEFAULT 0,
    transitive_cookbook_count INTEGER     NOT NULL DEFAULT 0,

    -- Active-target Chef version only (like git_repos materialised columns).
    compatible_count          INTEGER     NOT NULL DEFAULT 0,
    incompatible_count        INTEGER     NOT NULL DEFAULT 0,
    untested_count            INTEGER     NOT NULL DEFAULT 0,
    compatibility_status      TEXT        NOT NULL DEFAULT 'untested',
    tk_status                 TEXT        NOT NULL DEFAULT 'untested',
    tk_passed                 INTEGER     NOT NULL DEFAULT 0,
    tk_total                  INTEGER     NOT NULL DEFAULT 0,

    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (organisation_name, role_name),

    -- Role-level status vocabularies. Roles collapse cookbook scan errors to
    -- 'untested', so (unlike git_repos) there is no 'error' compat value.
    CONSTRAINT chk_role_summary_compatibility_status
        CHECK (compatibility_status IN ('untested', 'compatible', 'incompatible')),
    CONSTRAINT chk_role_summary_tk_status
        CHECK (tk_status IN ('untested', 'passed', 'failed', 'partial'))
);

-- role_name drives the cross-org rollup grouping and the name substring filter.
CREATE INDEX idx_role_summary_role_name ON role_summary (role_name);

-- Status columns filter the list before rollup.
CREATE INDEX idx_role_summary_compatibility_status ON role_summary (compatibility_status);
CREATE INDEX idx_role_summary_tk_status ON role_summary (tk_status);
