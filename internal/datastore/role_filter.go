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

	// PrecomputedCompatMap is optional. When non-nil, ListRolesFiltered uses it
	// instead of calling GetRoleCompatSummary. Used by the handler to pass cached data.
	PrecomputedCompatMap map[string]string
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

// buildRoleFilterQuery constructs the SQL query and args for ListRolesFiltered.
// Uses a recursive CTE to expand role dependencies transitively, then joins
// against cookstyle results to derive compatibility.
//
// When seedRoles is non-nil and non-empty the query is "seeded": it restricts
// the recursive expansion to only those root roles and drives roles_agg from
// an all_seed_roles anchor CTE so that roles with no cookbook deps still
// appear as "untested". Seeded queries omit COUNT(*) OVER(), LIMIT, and
// OFFSET (use 0 AS total_count instead) and order by array_position to
// preserve page order.
//
// When seedRoles is nil the query uses the existing full-expansion behaviour
// with COUNT(*) OVER() as total_count and optional LIMIT/OFFSET.
func buildRoleFilterQuery(f RoleFilter, seedRoles []string) (string, []interface{}) {
	var args []interface{}
	argN := 0
	isSeeded := len(seedRoles) > 0

	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	var sb strings.Builder
	var seedArg string // placeholder reused in all seeded locations

	// --- CTE 1: recursive transitive closure of role dependencies ---
	sb.WriteString("WITH RECURSIVE transitive_deps AS (\n")
	sb.WriteString("  SELECT rd.organisation_name, rd.role_name AS root_role,\n")
	sb.WriteString("         rd.role_name, rd.dependency_type, rd.dependency_name,\n")
	sb.WriteString("         1 AS depth,\n")
	sb.WriteString("         ARRAY[rd.role_name] AS visited\n")
	sb.WriteString("  FROM role_dependencies rd\n")

	if len(f.OrganisationNames) > 0 {
		sb.WriteString("  WHERE rd.organisation_name = ANY(" + nextArg() + ")\n")
		args = append(args, pq.Array(f.OrganisationNames))
		if isSeeded {
			p := nextArg()
			seedArg = p
			args = append(args, pq.Array(seedRoles))
			sb.WriteString("    AND rd.role_name = ANY(" + p + ")\n")
		}
	} else if isSeeded {
		p := nextArg()
		seedArg = p
		args = append(args, pq.Array(seedRoles))
		sb.WriteString("  WHERE rd.role_name = ANY(" + p + ")\n")
	}

	sb.WriteString("  UNION ALL\n")
	sb.WriteString("  SELECT td.organisation_name, td.root_role,\n")
	sb.WriteString("         rd2.role_name, rd2.dependency_type, rd2.dependency_name,\n")
	sb.WriteString("         td.depth + 1,\n")
	sb.WriteString("         td.visited || rd2.role_name\n")
	sb.WriteString("  FROM transitive_deps td\n")
	sb.WriteString("  JOIN role_dependencies rd2\n")
	sb.WriteString("    ON rd2.organisation_name = td.organisation_name\n")
	sb.WriteString("    AND rd2.role_name = td.dependency_name\n")
	sb.WriteString("  WHERE td.dependency_type = 'role'\n")
	sb.WriteString("    AND td.depth < 50\n")
	sb.WriteString("    AND NOT rd2.role_name = ANY(td.visited)\n")
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
		if isSeeded {
			sb.WriteString("    AND role_name = ANY(" + seedArg + ")\n")
		}
	} else if isSeeded {
		sb.WriteString("  WHERE role_name = ANY(" + seedArg + ")\n")
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
	sb.WriteString("      ELSE 'untested'\n")
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

	if isSeeded {
		// --- CTE 7 (seeded only): anchor CTE to ensure all seed roles appear ---
		sb.WriteString("all_seed_roles AS (\n")
		sb.WriteString("  SELECT unnest(" + seedArg + "::text[]) AS role_name\n")
		sb.WriteString("),\n")
	}

	// --- Final aggregation CTE: aggregate across orgs per role ---
	sb.WriteString("roles_agg AS (\n")
	sb.WriteString("  SELECT\n")
	if isSeeded {
		sb.WriteString("    asr.role_name,\n")
		// FILTER excludes NULL org_name when a role has no deps (rc IS NULL from LEFT JOIN)
		sb.WriteString("    ARRAY_AGG(DISTINCT rc.organisation_name ORDER BY rc.organisation_name) FILTER (WHERE rc.organisation_name IS NOT NULL) AS organisations,\n")
	} else {
		sb.WriteString("    rc.role_name,\n")
		sb.WriteString("    ARRAY_AGG(DISTINCT rc.organisation_name ORDER BY rc.organisation_name) AS organisations,\n")
	}
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
	if isSeeded {
		sb.WriteString("  FROM all_seed_roles asr\n")
		sb.WriteString("  LEFT JOIN role_compat rc ON rc.role_name = asr.role_name\n")
	} else {
		sb.WriteString("  FROM role_compat rc\n")
	}
	sb.WriteString("  LEFT JOIN direct_counts dc\n")
	sb.WriteString("    ON dc.organisation_name = rc.organisation_name AND dc.role_name = rc.role_name\n")
	if isSeeded {
		sb.WriteString("  LEFT JOIN node_counts nc ON nc.role_name = asr.role_name\n")
		sb.WriteString("  GROUP BY asr.role_name, nc.node_count\n")
	} else {
		sb.WriteString("  LEFT JOIN node_counts nc ON nc.role_name = rc.role_name\n")
		sb.WriteString("  GROUP BY rc.role_name, nc.node_count\n")
	}
	sb.WriteString(")\n")

	// --- Outer SELECT ---
	sb.WriteString("SELECT role_name, organisations, node_count,\n")
	sb.WriteString("       direct_cookbook_count, transitive_cookbook_count,\n")
	sb.WriteString("       total_cookbook_count, compatible_count,\n")
	sb.WriteString("       incompatible_count, untested_count,\n")
	sb.WriteString("       compatibility_status,\n")
	if isSeeded {
		sb.WriteString("       0 AS total_count\n")
	} else {
		sb.WriteString("       COUNT(*) OVER() AS total_count\n")
	}
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

	if isSeeded {
		// Preserve page order; seedArg is reused (not re-added to args).
		sb.WriteString("ORDER BY array_position(" + seedArg + "::text[], role_name) ASC\n")
	} else {
		var sortExpr string
		switch f.Sort {
		case "node_count":
			sortExpr = "node_count"
		case "incompatible_cookbook_count":
			sortExpr = "incompatible_count"
		default:
			sortExpr = "LOWER(role_name)"
		}
		sortDir := "ASC"
		if strings.EqualFold(f.SortOrder, "desc") {
			sortDir = "DESC"
		}
		sb.WriteString("ORDER BY " + sortExpr + " " + sortDir + "\n")

		if f.Limit > 0 {
			sb.WriteString("LIMIT " + nextArg() + "\n")
			args = append(args, f.Limit)
		}
		if f.Offset > 0 {
			sb.WriteString("OFFSET " + nextArg() + "\n")
			args = append(args, f.Offset)
		}
	}

	return sb.String(), args
}

// buildRolePageQuery returns a fast query that retrieves the page of distinct
// role names (sorted by name) plus the total count of distinct roles matching
// the org and name filters. It deliberately omits compat-status filtering
// because compat data requires the full recursive CTE.
//
// The COUNT(*) OVER() window function is applied to the outer query so that
// it counts distinct roles (not raw dependency edges).
func buildRolePageQuery(f RoleFilter) (string, []interface{}) {
	var args []interface{}
	argN := 0
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	sortDir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortDir = "DESC"
	}

	var sb strings.Builder
	sb.WriteString("SELECT role_name, COUNT(*) OVER() AS total_count\n")
	sb.WriteString("FROM (\n")
	sb.WriteString("  SELECT DISTINCT role_name\n")
	sb.WriteString("  FROM role_dependencies\n")

	var conditions []string
	if len(f.OrganisationNames) > 0 {
		conditions = append(conditions, "organisation_name = ANY("+nextArg()+")")
		args = append(args, pq.Array(f.OrganisationNames))
	}
	if f.Name != "" {
		conditions = append(conditions, "LOWER(role_name) LIKE '%' || LOWER("+nextArg()+") || '%'")
		args = append(args, f.Name)
	}
	if len(conditions) > 0 {
		sb.WriteString("  WHERE " + strings.Join(conditions, "\n    AND ") + "\n")
	}

	sb.WriteString(") q\n")
	sb.WriteString("ORDER BY LOWER(role_name) " + sortDir + "\n")

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
//
// Fast path (sort by name or default): paginates with buildRolePageQuery
// first, then expands only the page roles via a seeded buildRoleFilterQuery.
// This avoids running the expensive recursive CTE over all roles before
// paginating.
//
// Slow path (sort by node_count or incompatible_cookbook_count): falls back
// to full-expansion behaviour because sort order depends on computed data.
func (db *DB) ListRolesFiltered(ctx context.Context, f RoleFilter) ([]RoleFilterRow, int, RoleFilterSummary, error) {
	if f.Sort == "" || f.Sort == "name" {
		return db.listRolesFilteredFastPath(ctx, f)
	}
	return db.listRolesFilteredSlowPath(ctx, f)
}

func (db *DB) listRolesFilteredFastPath(ctx context.Context, f RoleFilter) ([]RoleFilterRow, int, RoleFilterSummary, error) {
	summary := RoleFilterSummary{TargetChefVersion: f.TargetChefVersion}

	// Step 1: get the page of role names + total distinct role count.
	pageQuery, pageArgs := buildRolePageQuery(f)
	pageRows, err := db.pool.QueryContext(ctx, pageQuery, pageArgs...)
	if err != nil {
		return nil, 0, summary, fmt.Errorf("datastore: querying role page: %w", err)
	}
	defer pageRows.Close()

	var pageRoles []string
	totalCount := 0
	for pageRows.Next() {
		var roleName string
		var rowTotal int
		if err := pageRows.Scan(&roleName, &rowTotal); err != nil {
			return nil, 0, summary, fmt.Errorf("datastore: scanning role page row: %w", err)
		}
		pageRoles = append(pageRoles, roleName)
		totalCount = rowTotal
	}
	if err := pageRows.Err(); err != nil {
		return nil, 0, summary, fmt.Errorf("datastore: iterating role page rows: %w", err)
	}

	// Step 2: compute summary for all matching roles (no compat or pagination filter).
	// Skip when the caller has pre-computed the compat map (e.g. from the handler cache).
	if f.PrecomputedCompatMap == nil {
		summaryFilter := f
		summaryFilter.CompatibilityStatus = ""
		summaryFilter.Limit = 0
		summaryFilter.Offset = 0
		if s, _, serr := db.GetRoleCompatSummary(ctx, summaryFilter); serr == nil {
			summary = s
			summary.TargetChefVersion = f.TargetChefVersion
		}
	}

	// Step 3: nothing on this page — return early with summary.
	if len(pageRoles) == 0 {
		return nil, totalCount, summary, nil
	}

	// Step 4: seeded expand — full row data for only this page's roles.
	seedQuery, seedArgs := buildRoleFilterQuery(f, pageRoles)
	seedRows, err := db.pool.QueryContext(ctx, seedQuery, seedArgs...)
	if err != nil {
		return nil, 0, summary, fmt.Errorf("datastore: querying seeded roles: %w", err)
	}
	defer seedRows.Close()

	resultMap := make(map[string]RoleFilterRow, len(pageRoles))
	for seedRows.Next() {
		var r RoleFilterRow
		var orgs pq.StringArray
		var dummy int // 0 AS total_count placeholder
		if err := seedRows.Scan(
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
			&dummy,
		); err != nil {
			return nil, 0, summary, fmt.Errorf("datastore: scanning seeded role row: %w", err)
		}
		r.Organisations = []string(orgs)
		resultMap[r.RoleName] = r
	}
	if err := seedRows.Err(); err != nil {
		return nil, 0, summary, fmt.Errorf("datastore: iterating seeded role rows: %w", err)
	}

	// Step 5: re-sort to match page order from step 1.
	results := make([]RoleFilterRow, 0, len(pageRoles))
	for _, name := range pageRoles {
		if r, ok := resultMap[name]; ok {
			results = append(results, r)
		}
	}

	return results, totalCount, summary, nil
}

func (db *DB) listRolesFilteredSlowPath(ctx context.Context, f RoleFilter) ([]RoleFilterRow, int, RoleFilterSummary, error) {
	query, args := buildRoleFilterQuery(f, nil)

	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, RoleFilterSummary{}, fmt.Errorf("datastore: querying filtered roles: %w", err)
	}
	defer rows.Close()

	var results []RoleFilterRow
	totalCount := 0
	summary := RoleFilterSummary{TargetChefVersion: f.TargetChefVersion}

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

	// When compat filter is active: use GetRoleCompatSummary to get summary
	// counts for all matching roles (without compat filter), replacing the
	// old recursive ListRolesFiltered self-call. Skip when the caller has
	// pre-computed the compat map (e.g. from the handler cache).
	if f.CompatibilityStatus != "" && f.CompatibilityStatus != "all" && f.PrecomputedCompatMap == nil {
		summaryFilter := f
		summaryFilter.CompatibilityStatus = ""
		summaryFilter.Limit = 0
		summaryFilter.Offset = 0
		if s, _, serr := db.GetRoleCompatSummary(ctx, summaryFilter); serr == nil {
			summary = s
			summary.TargetChefVersion = f.TargetChefVersion
		}
	} else if f.CompatibilityStatus == "" || f.CompatibilityStatus == "all" {
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
		if totalCount > 0 {
			summary.TotalRoles = totalCount
		}
	}

	return results, totalCount, summary, nil
}

// GetRoleCompatSummary returns aggregate compatibility counts and a
// role_name→compat_status map for all roles matching the org and name
// filters in f (CompatibilityStatus, Limit, and Offset are ignored).
//
// When f.TargetChefVersion is empty all roles are "untested" and the
// expensive recursive CTE is skipped.
func (db *DB) GetRoleCompatSummary(ctx context.Context, f RoleFilter) (RoleFilterSummary, map[string]string, error) {
	summary := RoleFilterSummary{TargetChefVersion: f.TargetChefVersion}
	compatMap := make(map[string]string)

	var args []interface{}
	argN := 0
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	orgArg := ""
	nameArg := ""
	if len(f.OrganisationNames) > 0 {
		orgArg = nextArg()
		args = append(args, pq.Array(f.OrganisationNames))
	}
	if f.Name != "" {
		nameArg = nextArg()
		args = append(args, f.Name)
	}

	// Fast path: no target version means all roles are untested.
	if f.TargetChefVersion == "" {
		var sb strings.Builder
		sb.WriteString("SELECT DISTINCT role_name\n")
		sb.WriteString("FROM role_dependencies\n")

		var conds []string
		if orgArg != "" {
			conds = append(conds, "organisation_name = ANY("+orgArg+")")
		}
		if nameArg != "" {
			conds = append(conds, "LOWER(role_name) LIKE '%' || LOWER("+nameArg+") || '%'")
		}
		if len(conds) > 0 {
			sb.WriteString("WHERE " + strings.Join(conds, "\n  AND ") + "\n")
		}

		rows, err := db.pool.QueryContext(ctx, sb.String(), args...)
		if err != nil {
			return summary, nil, fmt.Errorf("datastore: querying role compat summary (no version): %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return summary, nil, fmt.Errorf("datastore: scanning role name: %w", err)
			}
			compatMap[name] = "untested"
			summary.UntestedRoles++
			summary.TotalRoles++
		}
		return summary, compatMap, rows.Err()
	}

	// Full path: recursive CTE to compute compat status per role.
	targetArg := nextArg()
	args = append(args, f.TargetChefVersion)

	// Base WHERE conditions are reused in both transitive_deps and all_roles
	// by referencing the same $N placeholders.
	var baseConds []string
	if orgArg != "" {
		baseConds = append(baseConds, "rd.organisation_name = ANY("+orgArg+")")
	}
	if nameArg != "" {
		baseConds = append(baseConds, "LOWER(rd.role_name) LIKE '%' || LOWER("+nameArg+") || '%'")
	}

	var sb strings.Builder

	sb.WriteString("WITH RECURSIVE transitive_deps AS (\n")
	sb.WriteString("  SELECT rd.organisation_name, rd.role_name AS root_role,\n")
	sb.WriteString("         rd.dependency_type, rd.dependency_name,\n")
	sb.WriteString("         1 AS depth, ARRAY[rd.role_name] AS visited\n")
	sb.WriteString("  FROM role_dependencies rd\n")
	if len(baseConds) > 0 {
		sb.WriteString("  WHERE " + strings.Join(baseConds, "\n    AND ") + "\n")
	}
	sb.WriteString("  UNION ALL\n")
	sb.WriteString("  SELECT td.organisation_name, td.root_role,\n")
	sb.WriteString("         rd2.dependency_type, rd2.dependency_name,\n")
	sb.WriteString("         td.depth + 1, td.visited || rd2.role_name\n")
	sb.WriteString("  FROM transitive_deps td\n")
	sb.WriteString("  JOIN role_dependencies rd2\n")
	sb.WriteString("    ON rd2.organisation_name = td.organisation_name\n")
	sb.WriteString("    AND rd2.role_name = td.dependency_name\n")
	sb.WriteString("  WHERE td.dependency_type = 'role'\n")
	sb.WriteString("    AND td.depth < 50\n")
	sb.WriteString("    AND NOT rd2.role_name = ANY(td.visited)\n")
	sb.WriteString("),\n")

	sb.WriteString("role_cookbooks AS (\n")
	sb.WriteString("  SELECT DISTINCT organisation_name, root_role, dependency_name AS cookbook_name\n")
	sb.WriteString("  FROM transitive_deps\n")
	sb.WriteString("  WHERE dependency_type = 'cookbook'\n")
	sb.WriteString("),\n")

	sb.WriteString("cookbook_compat AS (\n")
	sb.WriteString("  SELECT rc.organisation_name, rc.root_role,\n")
	sb.WriteString("    CASE\n")
	sb.WriteString("      WHEN csr.passed = false THEN 'incompatible'\n")
	sb.WriteString("      WHEN csr.passed = true THEN 'compatible'\n")
	sb.WriteString("      ELSE 'untested'\n")
	sb.WriteString("    END AS status\n")
	sb.WriteString("  FROM role_cookbooks rc\n")
	sb.WriteString("  LEFT JOIN server_cookbooks sc\n")
	sb.WriteString("    ON sc.organisation_name = rc.organisation_name\n")
	sb.WriteString("    AND sc.name = rc.cookbook_name\n")
	sb.WriteString("  LEFT JOIN server_cookbook_cookstyle_results csr\n")
	sb.WriteString("    ON csr.organisation_name = rc.organisation_name\n")
	sb.WriteString("    AND csr.cookbook_name = rc.cookbook_name\n")
	sb.WriteString("    AND csr.cookbook_version = sc.version\n")
	sb.WriteString("    AND csr.target_chef_version = " + targetArg + "\n")
	sb.WriteString("),\n")

	sb.WriteString("role_compat_status AS (\n")
	sb.WriteString("  SELECT root_role,\n")
	sb.WriteString("    CASE\n")
	sb.WriteString("      WHEN bool_or(status = 'incompatible') THEN 'incompatible'\n")
	sb.WriteString("      WHEN bool_or(status = 'untested') THEN 'untested'\n")
	sb.WriteString("      WHEN COUNT(*) > 0 THEN 'compatible'\n")
	sb.WriteString("      ELSE 'untested'\n")
	sb.WriteString("    END AS compat\n")
	sb.WriteString("  FROM cookbook_compat\n")
	sb.WriteString("  GROUP BY root_role\n")
	sb.WriteString("),\n")

	// all_roles reuses the same $orgArg and $nameArg placeholders (no extra args).
	var allRolesConds []string
	if orgArg != "" {
		allRolesConds = append(allRolesConds, "organisation_name = ANY("+orgArg+")")
	}
	if nameArg != "" {
		allRolesConds = append(allRolesConds, "LOWER(role_name) LIKE '%' || LOWER("+nameArg+") || '%'")
	}

	sb.WriteString("all_roles AS (\n")
	sb.WriteString("  SELECT DISTINCT role_name\n")
	sb.WriteString("  FROM role_dependencies\n")
	if len(allRolesConds) > 0 {
		sb.WriteString("  WHERE " + strings.Join(allRolesConds, "\n    AND ") + "\n")
	}
	sb.WriteString(")\n")

	sb.WriteString("SELECT ar.role_name, COALESCE(rs.compat, 'untested') AS compat_status\n")
	sb.WriteString("FROM all_roles ar\n")
	sb.WriteString("LEFT JOIN role_compat_status rs ON rs.root_role = ar.role_name\n")

	rows, err := db.pool.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return summary, nil, fmt.Errorf("datastore: querying role compat summary: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, compat string
		if err := rows.Scan(&name, &compat); err != nil {
			return summary, nil, fmt.Errorf("datastore: scanning role compat row: %w", err)
		}
		compatMap[name] = compat
		switch compat {
		case "compatible":
			summary.CompatibleRoles++
		case "incompatible":
			summary.IncompatibleRoles++
		default:
			summary.UntestedRoles++
		}
		summary.TotalRoles++
	}
	return summary, compatMap, rows.Err()
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
  JOIN git_kitchen_results_active gkr
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
