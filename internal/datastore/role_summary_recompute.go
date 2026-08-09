// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"
)

// role_summary is a materialised per-role aggregate table (grain
// organisation_name, role_name) that lets the roles list sort/filter derived
// fields via indexed reads instead of recomputing a recursive transitive-dep
// CTE over all roles on every request. These functions are the single source
// of truth for the derivation; they run in bulk (set-based, not per-request)
// at the same trigger points git_repos materialised columns recompute — see
// git_repo_status_recompute.go and journeys/role-impact.md.
//
// The column groups recompute independently so each trigger touches only what
// it invalidates:
//   - structural (node_count, cookbook counts) — version-independent, at collection
//   - compat (compatible/incompatible/untested + status) — active target, at
//     collection / rescore / reclassification / target change
//   - tk (tk_status, tk_passed, tk_total) — at collection / kitchen-exclusion change

// roleTransitiveDepsCTE is the recursive transitive closure of role→role and
// role→cookbook edges, rooted at each role. Shared by every recompute so the
// expansion logic (depth cap, cycle guard) stays identical to the live query
// derivations in role_filter.go. Must be used inside a `WITH RECURSIVE`.
const roleTransitiveDepsCTE = `
transitive_deps AS (
  SELECT rd.organisation_name, rd.role_name AS root_role,
         rd.dependency_type, rd.dependency_name,
         1 AS depth, ARRAY[rd.role_name] AS visited
  FROM role_dependencies rd
  UNION ALL
  SELECT td.organisation_name, td.root_role,
         rd2.dependency_type, rd2.dependency_name,
         td.depth + 1, td.visited || rd2.role_name
  FROM transitive_deps td
  JOIN role_dependencies rd2
    ON rd2.organisation_name = td.organisation_name
    AND rd2.role_name = td.dependency_name
  WHERE td.dependency_type = 'role'
    AND td.depth < 50
    AND NOT rd2.role_name = ANY(td.visited)
)`

// RecomputeAllRoleStructural re-materialises the version-independent columns
// (node_count, direct_cookbook_count, transitive_cookbook_count) for every
// (organisation_name, role_name) in role_dependencies, and prunes role_summary
// rows whose role no longer exists in role_dependencies.
//
// node_count counts distinct NON-STALE nodes (is_stale = false) in the role's
// own organisation whose roles array contains the role — decommissioned/ghost
// nodes are excluded from blast radius. It is computed with ONE unnest+GROUP BY
// seq scan over node_snapshots (replacing per-role GIN containment probes).
//
// Call at collection completion (role_dependencies + node_snapshots settled).
func (db *DB) RecomputeAllRoleStructural(ctx context.Context) error {
	// Prune first: rows for roles removed from role_dependencies must not linger.
	const pruneQuery = `
		DELETE FROM role_summary rs
		WHERE NOT EXISTS (
			SELECT 1 FROM role_dependencies rd
			WHERE rd.organisation_name = rs.organisation_name
			  AND rd.role_name = rs.role_name
		)`
	if _, err := db.q().ExecContext(ctx, pruneQuery); err != nil {
		return fmt.Errorf("datastore: pruning role_summary: %w", err)
	}

	const query = `
WITH RECURSIVE
all_roles AS (
  SELECT DISTINCT organisation_name, role_name FROM role_dependencies
),
` + roleTransitiveDepsCTE + `,
transitive_cookbooks AS (
  SELECT organisation_name, root_role AS role_name,
         COUNT(DISTINCT dependency_name) AS transitive_cookbook_count
  FROM transitive_deps
  WHERE dependency_type = 'cookbook'
  GROUP BY organisation_name, root_role
),
direct_counts AS (
  SELECT organisation_name, role_name,
         COUNT(*) FILTER (WHERE dependency_type = 'cookbook') AS direct_cookbook_count
  FROM role_dependencies
  GROUP BY organisation_name, role_name
),
node_counts AS (
  SELECT ns.organisation_name, elem AS role_name,
         COUNT(DISTINCT ns.node_name) AS node_count
  FROM node_snapshots ns
  CROSS JOIN LATERAL jsonb_array_elements_text(ns.roles) AS elem
  WHERE ns.is_stale = false
    AND jsonb_typeof(ns.roles) = 'array'
  GROUP BY ns.organisation_name, elem
)
INSERT INTO role_summary AS rs
  (organisation_name, role_name, node_count, direct_cookbook_count, transitive_cookbook_count)
SELECT ar.organisation_name, ar.role_name,
       COALESCE(nc.node_count, 0),
       COALESCE(dc.direct_cookbook_count, 0),
       COALESCE(tc.transitive_cookbook_count, 0)
FROM all_roles ar
LEFT JOIN node_counts nc
  ON nc.organisation_name = ar.organisation_name AND nc.role_name = ar.role_name
LEFT JOIN direct_counts dc
  ON dc.organisation_name = ar.organisation_name AND dc.role_name = ar.role_name
LEFT JOIN transitive_cookbooks tc
  ON tc.organisation_name = ar.organisation_name AND tc.role_name = ar.role_name
ON CONFLICT (organisation_name, role_name) DO UPDATE SET
  node_count                = EXCLUDED.node_count,
  direct_cookbook_count     = EXCLUDED.direct_cookbook_count,
  transitive_cookbook_count = EXCLUDED.transitive_cookbook_count,
  updated_at                = now()`

	if _, err := db.q().ExecContext(ctx, query); err != nil {
		return fmt.Errorf("datastore: recomputing role_summary structural columns: %w", err)
	}
	return nil
}

// RecomputeAllRoleCompatStatus re-materialises the active-target compatibility
// columns (compatible_count, incompatible_count, untested_count,
// compatibility_status) for every role from the transitive cookbook set joined
// to cookstyle results for targetChefVersion. Rows are created for roles that
// have no cookbook closure (→ untested) so the list membership stays complete.
//
// Call after collection, a bulk rescore, a cop reclassification, or a target
// change (mirrors RecomputeAllGitRepoCookstyleStatus).
func (db *DB) RecomputeAllRoleCompatStatus(ctx context.Context, targetChefVersion string) error {
	const query = `
WITH RECURSIVE
all_roles AS (
  SELECT DISTINCT organisation_name, role_name FROM role_dependencies
),
` + roleTransitiveDepsCTE + `,
role_cookbooks AS (
  SELECT DISTINCT organisation_name, root_role AS role_name, dependency_name AS cookbook_name
  FROM transitive_deps
  WHERE dependency_type = 'cookbook'
),
cookbook_compat AS (
  SELECT rc.organisation_name, rc.role_name, rc.cookbook_name,
    CASE
      WHEN csr.error_message IS NOT NULL AND csr.error_message != '' THEN 'untested'
      WHEN csr.passed = true THEN 'compatible'
      WHEN csr.passed = false THEN 'incompatible'
      ELSE 'untested'
    END AS cookbook_status
  FROM role_cookbooks rc
  LEFT JOIN server_cookbooks sc
    ON sc.organisation_name = rc.organisation_name
    AND sc.name = rc.cookbook_name
  LEFT JOIN server_cookbook_cookstyle_results csr
    ON csr.organisation_name = rc.organisation_name
    AND csr.cookbook_name = rc.cookbook_name
    AND csr.cookbook_version = sc.version
    AND csr.target_chef_version = $1
),
role_compat AS (
  SELECT organisation_name, role_name,
    COUNT(DISTINCT cookbook_name) FILTER (WHERE cookbook_status = 'compatible') AS compatible_count,
    COUNT(DISTINCT cookbook_name) FILTER (WHERE cookbook_status = 'incompatible') AS incompatible_count,
    COUNT(DISTINCT cookbook_name) FILTER (WHERE cookbook_status = 'untested') AS untested_count,
    COUNT(DISTINCT cookbook_name) AS total_count
  FROM cookbook_compat
  GROUP BY organisation_name, role_name
)
INSERT INTO role_summary AS rs
  (organisation_name, role_name, compatible_count, incompatible_count, untested_count, compatibility_status)
SELECT ar.organisation_name, ar.role_name,
       COALESCE(rcp.compatible_count, 0),
       COALESCE(rcp.incompatible_count, 0),
       COALESCE(rcp.untested_count, 0),
       CASE
         WHEN COALESCE(rcp.incompatible_count, 0) > 0 THEN 'incompatible'
         WHEN COALESCE(rcp.untested_count, 0) > 0 THEN 'untested'
         WHEN COALESCE(rcp.total_count, 0) > 0 THEN 'compatible'
         ELSE 'untested'
       END
FROM all_roles ar
LEFT JOIN role_compat rcp
  ON rcp.organisation_name = ar.organisation_name AND rcp.role_name = ar.role_name
ON CONFLICT (organisation_name, role_name) DO UPDATE SET
  compatible_count     = EXCLUDED.compatible_count,
  incompatible_count   = EXCLUDED.incompatible_count,
  untested_count       = EXCLUDED.untested_count,
  compatibility_status = EXCLUDED.compatibility_status,
  updated_at           = now()`

	if _, err := db.q().ExecContext(ctx, query, targetChefVersion); err != nil {
		return fmt.Errorf("datastore: recomputing role_summary compat columns: %w", err)
	}
	return nil
}

// RecomputeAllRoleTKStatus re-materialises the tk columns (tk_status,
// tk_passed, tk_total) for every role by rolling up its transitive cookbook
// set's git_repos.tk_status with worst-of logic (any failed → failed, any
// partial → partial, else any passed → passed, else untested).
// git_repos.tk_status already reflects the active target, so no target
// parameter is needed.
//
// Call after collection or a kitchen-exclusion change.
func (db *DB) RecomputeAllRoleTKStatus(ctx context.Context) error {
	const query = `
WITH RECURSIVE
all_roles AS (
  SELECT DISTINCT organisation_name, role_name FROM role_dependencies
),
` + roleTransitiveDepsCTE + `,
role_cookbooks AS (
  SELECT DISTINCT organisation_name, root_role AS role_name, dependency_name AS cookbook_name
  FROM transitive_deps
  WHERE dependency_type = 'cookbook'
),
cookbook_tk AS (
  SELECT rc.organisation_name, rc.role_name,
         gr.tk_status AS status, gr.tk_passed, gr.tk_total
  FROM role_cookbooks rc
  JOIN git_repos gr ON gr.name = rc.cookbook_name
  WHERE gr.tk_status IS NOT NULL AND gr.tk_status != 'untested'
),
role_tk AS (
  SELECT organisation_name, role_name,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed_count,
    COUNT(*) FILTER (WHERE status = 'partial') AS partial_count,
    COUNT(*) FILTER (WHERE status = 'passed') AS passed_count,
    COALESCE(SUM(tk_passed), 0) AS tk_passed,
    COALESCE(SUM(tk_total), 0) AS tk_total
  FROM cookbook_tk
  GROUP BY organisation_name, role_name
)
INSERT INTO role_summary AS rs
  (organisation_name, role_name, tk_status, tk_passed, tk_total)
SELECT ar.organisation_name, ar.role_name,
       CASE
         WHEN COALESCE(rt.failed_count, 0) > 0 THEN 'failed'
         WHEN COALESCE(rt.partial_count, 0) > 0 THEN 'partial'
         WHEN COALESCE(rt.passed_count, 0) > 0 THEN 'passed'
         ELSE 'untested'
       END,
       COALESCE(rt.tk_passed, 0),
       COALESCE(rt.tk_total, 0)
FROM all_roles ar
LEFT JOIN role_tk rt
  ON rt.organisation_name = ar.organisation_name AND rt.role_name = ar.role_name
ON CONFLICT (organisation_name, role_name) DO UPDATE SET
  tk_status  = EXCLUDED.tk_status,
  tk_passed  = EXCLUDED.tk_passed,
  tk_total   = EXCLUDED.tk_total,
  updated_at = now()`

	if _, err := db.q().ExecContext(ctx, query); err != nil {
		return fmt.Errorf("datastore: recomputing role_summary tk columns: %w", err)
	}
	return nil
}

// ResetAllRoleStatuses blanks the active-target columns (compat + tk) to their
// defaults, preserving the version-independent structural columns. Call when
// the active target Chef version changes, before the target's results are
// recomputed (mirrors ResetAllGitRepoStatuses).
func (db *DB) ResetAllRoleStatuses(ctx context.Context) error {
	const query = `
		UPDATE role_summary
		SET compatible_count     = 0,
		    incompatible_count   = 0,
		    untested_count       = 0,
		    compatibility_status = 'untested',
		    tk_status            = 'untested',
		    tk_passed            = 0,
		    tk_total             = 0,
		    updated_at           = now()`

	if _, err := db.q().ExecContext(ctx, query); err != nil {
		return fmt.Errorf("datastore: resetting all role statuses: %w", err)
	}
	return nil
}
