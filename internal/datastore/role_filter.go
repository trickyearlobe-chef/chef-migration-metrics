// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// RoleFilter holds optional filter criteria for listing roles with derived
// compatibility status. The compatibility is computed by recursively
// expanding role→role and role→cookbook dependencies, then joining each
// reachable cookbook against cookstyle results for the target Chef version.
type RoleFilter struct {
	// OrganisationNames restricts results to roles in these orgs.
	// Empty means all organisations.
	OrganisationNames []string

	// Name filters by case-insensitive substring match on role name.
	Name string

	// CompatibilityStatus filters by the derived status.
	// Values: "compatible", "incompatible", "untested", "" (all).
	CompatibilityStatus string

	// TargetChefVersion is used for the cookstyle results JOIN.
	// When empty, all roles get "untested" compatibility.
	TargetChefVersion string

	// Limit caps returned rows. 0 means no limit.
	Limit int

	// Offset for pagination.
	Offset int

	// Sort field: "name" (default), "node_count", "incompatible_cookbook_count".
	Sort string

	// SortOrder: "asc" (default) or "desc".
	SortOrder string
}

// RoleFilterRow is the result of a filtered role query.
type RoleFilterRow struct {
	RoleName                string   `json:"role_name"`
	Organisations           []string `json:"organisations"`
	NodeCount               int      `json:"node_count"`
	DirectCookbookCount     int      `json:"direct_cookbook_count"`
	TransitiveCookbookCount int      `json:"transitive_cookbook_count"`
	TotalCookbookCount      int      `json:"total_cookbook_count"`
	CompatibleCount         int      `json:"compatible_count"`
	IncompatibleCount       int      `json:"incompatible_count"`
	UntestedCount           int      `json:"untested_count"`
	CompatibilityStatus     string   `json:"compatibility_status"` // "compatible", "incompatible", "untested"
	TKStatus                string   `json:"tk_status,omitempty"`  // "passed", "failed", "partial" (set by handler)
}

// RoleFilterSummary provides aggregate counts for the summary bar.
type RoleFilterSummary struct {
	TargetChefVersion string `json:"target_chef_version"`
	CompatibleRoles   int    `json:"compatible_roles"`
	IncompatibleRoles int    `json:"incompatible_roles"`
	UntestedRoles     int    `json:"untested_roles"`
	TotalRoles        int    `json:"total_roles"`
}

// buildRoleFilterQuery constructs the SQL query and args for
// ListRolesFiltered. Uses a recursive CTE to expand role dependencies
// transitively, then joins against cookstyle results to derive
// compatibility.
//
// Returns (query, args). The query includes COUNT(*) OVER() as total_count.
func buildRoleFilterQuery(f RoleFilter) (string, []interface{}) {
	var args []interface{}
	argN := 0

	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	var sb strings.Builder

	// --- CTE 1: recursive transitive closure of role dependencies ---
	// Walks role→role edges to find all transitively reachable cookbooks.
	sb.WriteString("WITH RECURSIVE transitive_deps AS (\n")
	sb.WriteString("  -- Base case: direct dependencies\n")
	sb.WriteString("  SELECT rd.organisation_name, rd.role_name AS root_role,\n")
	sb.WriteString("         rd.role_name, rd.dependency_type, rd.dependency_name,\n")
	sb.WriteString("         1 AS depth,\n")
	sb.WriteString("         ARRAY[rd.role_name] AS visited\n")
	sb.WriteString("  FROM role_dependencies rd\n")

	// Optional org filter inside the CTE base case
	if len(f.OrganisationNames) > 0 {
		sb.WriteString("  WHERE rd.organisation_name = ANY(" + nextArg() + ")\n")
		args = append(args, pq.Array(f.OrganisationNames))
	}

	sb.WriteString("  UNION ALL\n")
	sb.WriteString("  -- Recursive case: follow role→role edges\n")
	sb.WriteString("  SELECT td.organisation_name, td.root_role,\n")
	sb.WriteString("         rd2.role_name, rd2.dependency_type, rd2.dependency_name,\n")
	sb.WriteString("         td.depth + 1,\n")
	sb.WriteString("         td.visited || rd2.role_name\n")
	sb.WriteString("  FROM transitive_deps td\n")
	sb.WriteString("  JOIN role_dependencies rd2\n")
	sb.WriteString("    ON rd2.organisation_name = td.organisation_name\n")
	sb.WriteString("    AND rd2.role_name = td.dependency_name\n")
	sb.WriteString("  WHERE td.dependency_type = 'role'\n")
	sb.WriteString("    AND td.depth < 50\n")                       // cycle/depth guard
	sb.WriteString("    AND NOT rd2.role_name = ANY(td.visited)\n") // cycle-safe
	sb.WriteString("),\n")

	// --- CTE 2: distinct cookbooks per root role ---
	sb.WriteString("role_cookbooks AS (\n")
	sb.WriteString("  SELECT DISTINCT organisation_name, root_role AS role_name, dependency_name AS cookbook_name\n")
	sb.WriteString("  FROM transitive_deps\n")
	sb.WriteString("  WHERE dependency_type = 'cookbook'\n")
	sb.WriteString("),\n")

	// --- CTE 3: direct cookbook count per role ---
	sb.WriteString("direct_counts AS (\n")
	sb.WriteString("  SELECT organisation_name, role_name,\n")
	sb.WriteString("         COUNT(*) FILTER (WHERE dependency_type = 'cookbook') AS direct_cookbook_count\n")
	sb.WriteString("  FROM role_dependencies\n")
	if len(f.OrganisationNames) > 0 {
		sb.WriteString("  WHERE organisation_name = ANY(" + nextArg() + ")\n")
		args = append(args, pq.Array(f.OrganisationNames))
	}
	sb.WriteString("  GROUP BY organisation_name, role_name\n")
	sb.WriteString("),\n")

	// --- CTE 4: compatibility per cookbook via cookstyle results ---
	sb.WriteString("cookbook_compat AS (\n")
	sb.WriteString("  SELECT rc.organisation_name, rc.role_name, rc.cookbook_name,\n")

	if f.TargetChefVersion != "" {
		sb.WriteString("    CASE\n")
		sb.WriteString("      WHEN csr.error_message IS NOT NULL AND csr.error_message != '' THEN 'untested'\n")
		sb.WriteString("      WHEN csr.passed = true THEN 'compatible'\n")
		sb.WriteString("      WHEN csr.passed = false THEN 'incompatible'\n")
		sb.WriteString("      ELSE 'untested'\n")
		sb.WriteString("    END AS cookbook_status\n")
	} else {
		sb.WriteString("    'untested' AS cookbook_status\n")
	}

	sb.WriteString("  FROM role_cookbooks rc\n")

	if f.TargetChefVersion != "" {
		p := nextArg()
		// Join to server_cookbooks to get the version, then to cookstyle results
		sb.WriteString("  LEFT JOIN server_cookbooks sc\n")
		sb.WriteString("    ON sc.organisation_name = rc.organisation_name\n")
		sb.WriteString("    AND sc.name = rc.cookbook_name\n")
		sb.WriteString("  LEFT JOIN server_cookbook_cookstyle_results csr\n")
		sb.WriteString("    ON csr.organisation_name = rc.organisation_name\n")
		sb.WriteString("    AND csr.cookbook_name = rc.cookbook_name\n")
		sb.WriteString("    AND csr.cookbook_version = sc.version\n")
		sb.WriteString("    AND csr.target_chef_version = " + p + "\n")
		args = append(args, f.TargetChefVersion)
	}

	sb.WriteString("),\n")

	// --- CTE 5: aggregate compatibility per role ---
	sb.WriteString("role_compat AS (\n")
	sb.WriteString("  SELECT organisation_name, role_name,\n")
	sb.WriteString("    COUNT(DISTINCT cookbook_name) AS total_cookbook_count,\n")
	sb.WriteString("    COUNT(DISTINCT cookbook_name) FILTER (WHERE cookbook_status = 'compatible') AS compatible_count,\n")
	sb.WriteString("    COUNT(DISTINCT cookbook_name) FILTER (WHERE cookbook_status = 'incompatible') AS incompatible_count,\n")
	sb.WriteString("    COUNT(DISTINCT cookbook_name) FILTER (WHERE cookbook_status = 'untested') AS untested_count,\n")
	sb.WriteString("    CASE\n")
	sb.WriteString("      WHEN COUNT(DISTINCT cookbook_name) FILTER (WHERE cookbook_status = 'incompatible') > 0 THEN 'incompatible'\n")
	sb.WriteString("      WHEN COUNT(DISTINCT cookbook_name) FILTER (WHERE cookbook_status = 'untested') > 0 THEN 'untested'\n")
	sb.WriteString("      WHEN COUNT(DISTINCT cookbook_name) > 0 THEN 'compatible'\n")
	sb.WriteString("      ELSE 'untested'\n") // roles with no cookbooks = untested
	sb.WriteString("    END AS compatibility_status\n")
	sb.WriteString("  FROM cookbook_compat\n")
	sb.WriteString("  GROUP BY organisation_name, role_name\n")
	sb.WriteString("),\n")

	// --- CTE 6: node counts per role from node_snapshots.roles JSONB ---
	sb.WriteString("node_counts AS (\n")
	sb.WriteString("  SELECT r.role_name, COUNT(DISTINCT ns.organisation_name || '/' || ns.node_name) AS node_count\n")
	sb.WriteString("  FROM (\n")
	sb.WriteString("    SELECT DISTINCT role_name FROM role_compat\n")
	sb.WriteString("  ) r\n")
	sb.WriteString("  LEFT JOIN node_snapshots ns\n")
	sb.WriteString("    ON ns.roles @> to_jsonb(ARRAY[r.role_name])\n")
	if len(f.OrganisationNames) > 0 {
		sb.WriteString("    AND ns.organisation_name = ANY(" + nextArg() + ")\n")
		args = append(args, pq.Array(f.OrganisationNames))
	}
	sb.WriteString("  GROUP BY r.role_name\n")
	sb.WriteString("),\n")

	// --- CTE 7: aggregate across orgs per role ---
	sb.WriteString("roles_agg AS (\n")
	sb.WriteString("  SELECT\n")
	sb.WriteString("    rc.role_name,\n")
	sb.WriteString("    ARRAY_AGG(DISTINCT rc.organisation_name ORDER BY rc.organisation_name) AS organisations,\n")
	sb.WriteString("    COALESCE(nc.node_count, 0) AS node_count,\n")
	sb.WriteString("    COALESCE(MAX(dc.direct_cookbook_count), 0) AS direct_cookbook_count,\n")
	sb.WriteString("    COALESCE(MAX(rc.total_cookbook_count), 0) AS transitive_cookbook_count,\n")
	sb.WriteString("    COALESCE(MAX(rc.total_cookbook_count), 0) AS total_cookbook_count,\n")
	sb.WriteString("    COALESCE(MAX(rc.compatible_count), 0) AS compatible_count,\n")
	sb.WriteString("    COALESCE(MAX(rc.incompatible_count), 0) AS incompatible_count,\n")
	sb.WriteString("    COALESCE(MAX(rc.untested_count), 0) AS untested_count,\n")
	sb.WriteString("    CASE\n")
	sb.WriteString("      WHEN MAX(rc.incompatible_count) > 0 THEN 'incompatible'\n")
	sb.WriteString("      WHEN MAX(rc.untested_count) > 0 THEN 'untested'\n")
	sb.WriteString("      WHEN MAX(rc.total_cookbook_count) > 0 THEN 'compatible'\n")
	sb.WriteString("      ELSE 'untested'\n")
	sb.WriteString("    END AS compatibility_status\n")
	sb.WriteString("  FROM role_compat rc\n")
	sb.WriteString("  LEFT JOIN direct_counts dc\n")
	sb.WriteString("    ON dc.organisation_name = rc.organisation_name AND dc.role_name = rc.role_name\n")
	sb.WriteString("  LEFT JOIN node_counts nc ON nc.role_name = rc.role_name\n")
	sb.WriteString("  GROUP BY rc.role_name, nc.node_count\n")
	sb.WriteString(")\n")

	// --- Outer SELECT ---
	sb.WriteString("SELECT role_name, organisations, node_count,\n")
	sb.WriteString("       direct_cookbook_count, transitive_cookbook_count,\n")
	sb.WriteString("       total_cookbook_count, compatible_count,\n")
	sb.WriteString("       incompatible_count, untested_count,\n")
	sb.WriteString("       compatibility_status,\n")
	sb.WriteString("       COUNT(*) OVER() AS total_count\n")
	sb.WriteString("FROM roles_agg\n")
	sb.WriteString("WHERE 1=1\n")

	// Name filter
	if f.Name != "" {
		sb.WriteString("  AND LOWER(role_name) LIKE '%' || LOWER(" + nextArg() + ") || '%'\n")
		args = append(args, f.Name)
	}

	// Compatibility status filter
	if f.CompatibilityStatus != "" && f.CompatibilityStatus != "all" {
		sb.WriteString("  AND compatibility_status = " + nextArg() + "\n")
		args = append(args, f.CompatibilityStatus)
	}

	// --- ORDER BY ---
	var sortExpr string
	switch f.Sort {
	case "node_count":
		sortExpr = "node_count"
	case "incompatible_cookbook_count":
		sortExpr = "incompatible_count"
	default: // "name" or empty
		sortExpr = "LOWER(role_name)"
	}

	sortDir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortDir = "DESC"
	}
	sb.WriteString("ORDER BY " + sortExpr + " " + sortDir + "\n")

	// --- Pagination ---
	if f.Limit > 0 {
		sb.WriteString("LIMIT " + nextArg() + "\n")
		args = append(args, f.Limit)
	}
	if f.Offset > 0 {
		sb.WriteString("OFFSET " + nextArg() + "\n")
		args = append(args, f.Offset)
	}

	return sb.String(), args
}

// ListRolesFiltered retrieves roles matching the given filter with derived
// compatibility status. Returns the page of matching rows, the total count,
// and the summary counts.
func (db *DB) ListRolesFiltered(ctx context.Context, f RoleFilter) ([]RoleFilterRow, int, RoleFilterSummary, error) {
	query, args := buildRoleFilterQuery(f)

	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, RoleFilterSummary{}, fmt.Errorf("datastore: querying filtered roles: %w", err)
	}
	defer rows.Close()

	var results []RoleFilterRow
	totalCount := 0
	summary := RoleFilterSummary{
		TargetChefVersion: f.TargetChefVersion,
	}

	for rows.Next() {
		var r RoleFilterRow
		var orgs pq.StringArray
		var rowTotal int

		if err := rows.Scan(
			&r.RoleName,
			&orgs,
			&r.NodeCount,
			&r.DirectCookbookCount,
			&r.TransitiveCookbookCount,
			&r.TotalCookbookCount,
			&r.CompatibleCount,
			&r.IncompatibleCount,
			&r.UntestedCount,
			&r.CompatibilityStatus,
			&rowTotal,
		); err != nil {
			return nil, 0, RoleFilterSummary{}, fmt.Errorf("datastore: scanning filtered role row: %w", err)
		}

		r.Organisations = []string(orgs)
		totalCount = rowTotal
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, RoleFilterSummary{}, fmt.Errorf("datastore: iterating filtered role rows: %w", err)
	}

	// Compute summary from unfiltered results if we have a compatibility
	// filter active. Otherwise compute from the returned rows.
	// For simplicity, we compute summary by running the query without
	// compatibility filter and without pagination. This is acceptable
	// because the number of distinct roles is typically small (< 1000).
	if f.CompatibilityStatus != "" && f.CompatibilityStatus != "all" {
		summaryFilter := f
		summaryFilter.CompatibilityStatus = ""
		summaryFilter.Limit = 0
		summaryFilter.Offset = 0
		summaryRows, _, _, err := db.ListRolesFiltered(ctx, summaryFilter)
		if err != nil {
			return results, totalCount, RoleFilterSummary{}, nil // degrade gracefully
		}
		for _, sr := range summaryRows {
			switch sr.CompatibilityStatus {
			case "compatible":
				summary.CompatibleRoles++
			case "incompatible":
				summary.IncompatibleRoles++
			default:
				summary.UntestedRoles++
			}
			summary.TotalRoles++
		}
	} else {
		for _, r := range results {
			switch r.CompatibilityStatus {
			case "compatible":
				summary.CompatibleRoles++
			case "incompatible":
				summary.IncompatibleRoles++
			default:
				summary.UntestedRoles++
			}
			summary.TotalRoles++
		}
		// When no compatibility filter, total from rows == total from DB
		if totalCount > 0 {
			summary.TotalRoles = totalCount
		}
	}

	return results, totalCount, summary, nil
}

// GetRoleTKStatuses returns the aggregate TK status per role for a set of
// role names. Uses a recursive CTE to find each role's transitive cookbook
// set, joins to git_repos and git_kitchen_results, then aggregates per role
// using worst-of logic: any failed → "failed", any partial → "partial",
// all passed → "passed", no TK data → not in map.
func (db *DB) GetRoleTKStatuses(ctx context.Context, roleNames, orgNames []string, targetVersion string) (map[string]string, error) {
	result := make(map[string]string)
	if len(roleNames) == 0 || len(orgNames) == 0 || targetVersion == "" {
		return result, nil
	}

	query := `
WITH RECURSIVE transitive_deps AS (
  SELECT rd.organisation_name, rd.role_name AS root_role,
         rd.dependency_type, rd.dependency_name,
         1 AS depth, ARRAY[rd.role_name] AS visited
  FROM role_dependencies rd
  WHERE rd.organisation_name = ANY($1)
    AND rd.role_name = ANY($2)
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
),
role_cookbooks AS (
  SELECT DISTINCT root_role AS role_name, dependency_name AS cookbook_name
  FROM transitive_deps
  WHERE dependency_type = 'cookbook'
),
cookbook_tk AS (
  SELECT rc.role_name, gkr.git_repo_name,
    COUNT(*) FILTER (WHERE gkr.passed = true) AS p,
    COUNT(*) FILTER (WHERE gkr.passed = false OR gkr.timed_out = true) AS f
  FROM role_cookbooks rc
  JOIN git_repos gr ON gr.name = rc.cookbook_name
  JOIN git_kitchen_results gkr
    ON gkr.git_repo_name = gr.name
    AND gkr.target_chef_version = $3
  GROUP BY rc.role_name, gkr.git_repo_name
),
cookbook_status AS (
  SELECT role_name,
    CASE
      WHEN p > 0 AND f > 0 THEN 'partial'
      WHEN f > 0 THEN 'failed'
      WHEN p > 0 THEN 'passed'
    END AS status
  FROM cookbook_tk
)
SELECT role_name,
  COUNT(*) FILTER (WHERE status = 'failed') AS failed_count,
  COUNT(*) FILTER (WHERE status = 'partial') AS partial_count,
  COUNT(*) FILTER (WHERE status = 'passed') AS passed_count
FROM cookbook_status
WHERE status IS NOT NULL
GROUP BY role_name`

	rows, err := db.pool.QueryContext(ctx, query,
		pq.Array(orgNames), pq.Array(roleNames), targetVersion,
	)
	if err != nil {
		return result, fmt.Errorf("datastore: querying role TK statuses: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var failed, partial, passed int
		if err := rows.Scan(&name, &failed, &partial, &passed); err != nil {
			return result, err
		}
		switch {
		case failed > 0:
			result[name] = "failed"
		case partial > 0:
			result[name] = "partial"
		case passed > 0:
			result[name] = "passed"
		}
	}
	return result, rows.Err()
}
