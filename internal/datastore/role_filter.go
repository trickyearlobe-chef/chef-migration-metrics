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
// compatibility status. Derived fields are read from the materialised
// role_summary table (grain organisation_name, role_name) and rolled up across
// organisations to one row per role — no per-request recursive dependency
// expansion. role_summary is kept fresh by the bulk recompute functions (see
// role_summary_recompute.go).
type RoleFilter struct {
	// OrganisationNames restricts results to roles in these orgs.
	// Empty means all organisations.
	OrganisationNames []string

	// Name filters by case-insensitive substring match on role name.
	Name string

	// CompatibilityStatus filters by the rolled-up status.
	// Values: "compatible", "incompatible", "untested", "all"/"" (all).
	CompatibilityStatus string

	// TKStatuses filters by the rolled-up TK status. Empty means no filter.
	// Values: "passed", "failed", "partial", "untested".
	TKStatuses []string

	// TargetChefVersion labels the summary; role_summary already reflects the
	// active target Chef version, so it does not drive the query itself.
	TargetChefVersion string

	// Limit caps returned rows. 0 means no limit.
	Limit int

	// Offset for pagination.
	Offset int

	// Sort field: "name" (default), "node_count",
	// "incompatible_cookbook_count", "tk_status".
	Sort string

	// SortOrder: "asc" (default) or "desc".
	SortOrder string

	// PrecomputedCompatMap is optional. When non-nil, ListRolesFiltered skips
	// its internal GetRoleCompatSummary call. Used by the handler to pass its
	// cached summary.
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
	TKStatus                string   `json:"tk_status,omitempty"`  // "passed", "failed", "partial", "untested"
}

// RoleFilterSummary provides aggregate counts for the summary bar.
type RoleFilterSummary struct {
	TargetChefVersion string `json:"target_chef_version"`
	CompatibleRoles   int    `json:"compatible_roles"`
	IncompatibleRoles int    `json:"incompatible_roles"`
	UntestedRoles     int    `json:"untested_roles"`
	TotalRoles        int    `json:"total_roles"`
}

// buildRoleListQuery constructs the SQL for ListRolesFiltered. It rolls the
// per-(org,role) role_summary rows up to one row per role_name:
//   - cookbook + compat counts → MAX across orgs
//   - compatibility_status → worst-of (incompatible > untested > compatible)
//   - node_count → SUM across orgs
//   - organisations → array_agg
//   - tk_status → worst-of (failed > partial > passed > untested)
//
// Org and name filters apply before the rollup (so organisations and node_count
// reflect only the selected orgs). CompatibilityStatus and TKStatuses filter the
// rolled-up values. Sort and pagination happen in SQL.
func buildRoleListQuery(f RoleFilter) (string, []interface{}) {
	var args []interface{}
	argN := 0
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	var sb strings.Builder
	sb.WriteString("WITH rolled AS (\n")
	sb.WriteString("  SELECT role_name,\n")
	sb.WriteString("    ARRAY_AGG(DISTINCT organisation_name ORDER BY organisation_name) AS organisations,\n")
	sb.WriteString("    COALESCE(SUM(node_count), 0) AS node_count,\n")
	sb.WriteString("    COALESCE(MAX(direct_cookbook_count), 0) AS direct_cookbook_count,\n")
	sb.WriteString("    COALESCE(MAX(transitive_cookbook_count), 0) AS transitive_cookbook_count,\n")
	sb.WriteString("    COALESCE(MAX(compatible_count), 0) AS compatible_count,\n")
	sb.WriteString("    COALESCE(MAX(incompatible_count), 0) AS incompatible_count,\n")
	sb.WriteString("    COALESCE(MAX(untested_count), 0) AS untested_count,\n")
	sb.WriteString("    CASE\n")
	sb.WriteString("      WHEN bool_or(compatibility_status = 'incompatible') THEN 'incompatible'\n")
	sb.WriteString("      WHEN bool_or(compatibility_status = 'untested') THEN 'untested'\n")
	sb.WriteString("      ELSE 'compatible'\n")
	sb.WriteString("    END AS compatibility_status,\n")
	sb.WriteString("    CASE\n")
	sb.WriteString("      WHEN bool_or(tk_status = 'failed') THEN 'failed'\n")
	sb.WriteString("      WHEN bool_or(tk_status = 'partial') THEN 'partial'\n")
	sb.WriteString("      WHEN bool_or(tk_status = 'passed') THEN 'passed'\n")
	sb.WriteString("      ELSE 'untested'\n")
	sb.WriteString("    END AS tk_status\n")
	sb.WriteString("  FROM role_summary\n")

	var preConds []string
	if len(f.OrganisationNames) > 0 {
		preConds = append(preConds, "organisation_name = ANY("+nextArg()+")")
		args = append(args, pq.Array(f.OrganisationNames))
	}
	if f.Name != "" {
		preConds = append(preConds, "LOWER(role_name) LIKE '%' || LOWER("+nextArg()+") || '%'")
		args = append(args, f.Name)
	}
	if len(preConds) > 0 {
		sb.WriteString("  WHERE " + strings.Join(preConds, "\n    AND ") + "\n")
	}
	sb.WriteString("  GROUP BY role_name\n")
	sb.WriteString(")\n")

	sb.WriteString("SELECT role_name, organisations, node_count,\n")
	sb.WriteString("       direct_cookbook_count, transitive_cookbook_count,\n")
	sb.WriteString("       transitive_cookbook_count AS total_cookbook_count,\n")
	sb.WriteString("       compatible_count, incompatible_count, untested_count,\n")
	sb.WriteString("       compatibility_status, tk_status,\n")
	sb.WriteString("       COUNT(*) OVER() AS total_count\n")
	sb.WriteString("FROM rolled\n")
	sb.WriteString("WHERE 1=1\n")

	if f.CompatibilityStatus != "" && f.CompatibilityStatus != "all" {
		sb.WriteString("  AND compatibility_status = " + nextArg() + "\n")
		args = append(args, f.CompatibilityStatus)
	}
	if len(f.TKStatuses) > 0 {
		sb.WriteString("  AND tk_status = ANY(" + nextArg() + ")\n")
		args = append(args, pq.Array(f.TKStatuses))
	}

	var sortExpr string
	switch f.Sort {
	case "node_count":
		sortExpr = "node_count"
	case "incompatible_cookbook_count":
		sortExpr = "incompatible_count"
	case "tk_status":
		// Rank matches the old in-memory sort: failed < partial < passed < untested.
		sortExpr = "CASE tk_status WHEN 'failed' THEN 0 WHEN 'partial' THEN 1 WHEN 'passed' THEN 2 ELSE 3 END"
	default:
		sortExpr = "LOWER(role_name)"
	}
	sortDir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortDir = "DESC"
	}
	sb.WriteString("ORDER BY " + sortExpr + " " + sortDir + ", LOWER(role_name) ASC\n")

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
// compatibility and TK status read from the materialised role_summary table.
// Returns the page of matching rows, the total count of matching roles, and the
// summary counts. Filtering, sorting, and pagination all happen in SQL.
func (db *DB) ListRolesFiltered(ctx context.Context, f RoleFilter) ([]RoleFilterRow, int, RoleFilterSummary, error) {
	summary := RoleFilterSummary{TargetChefVersion: f.TargetChefVersion}

	query, args := buildRoleListQuery(f)
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, summary, fmt.Errorf("datastore: querying filtered roles: %w", err)
	}
	defer rows.Close()

	var results []RoleFilterRow
	totalCount := 0
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
			&r.TKStatus,
			&rowTotal,
		); err != nil {
			return nil, 0, summary, fmt.Errorf("datastore: scanning filtered role row: %w", err)
		}
		r.Organisations = []string(orgs)
		totalCount = rowTotal
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, summary, fmt.Errorf("datastore: iterating filtered role rows: %w", err)
	}

	// Summary reflects all roles matching org+name (ignoring compat/tk filters
	// and pagination). Skip when the caller pre-computed it (handler cache).
	if f.PrecomputedCompatMap == nil {
		summaryFilter := f
		summaryFilter.CompatibilityStatus = ""
		summaryFilter.TKStatuses = nil
		summaryFilter.Limit = 0
		summaryFilter.Offset = 0
		if s, _, serr := db.GetRoleCompatSummary(ctx, summaryFilter); serr == nil {
			summary = s
			summary.TargetChefVersion = f.TargetChefVersion
		}
	}

	return results, totalCount, summary, nil
}

// GetRoleCompatSummary returns aggregate compatibility counts and a
// role_name→compat_status map for all roles matching the org and name filters
// in f (CompatibilityStatus, TKStatuses, Limit, and Offset are ignored). The
// status is the cross-org worst-of rollup of role_summary.compatibility_status
// for the active target — an O(roles) indexed aggregate, no recursive CTE.
func (db *DB) GetRoleCompatSummary(ctx context.Context, f RoleFilter) (RoleFilterSummary, map[string]string, error) {
	summary := RoleFilterSummary{TargetChefVersion: f.TargetChefVersion}
	compatMap := make(map[string]string)

	var args []interface{}
	argN := 0
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	var sb strings.Builder
	sb.WriteString("SELECT role_name,\n")
	sb.WriteString("  CASE\n")
	sb.WriteString("    WHEN bool_or(compatibility_status = 'incompatible') THEN 'incompatible'\n")
	sb.WriteString("    WHEN bool_or(compatibility_status = 'untested') THEN 'untested'\n")
	sb.WriteString("    ELSE 'compatible'\n")
	sb.WriteString("  END AS compat\n")
	sb.WriteString("FROM role_summary\n")

	var conds []string
	if len(f.OrganisationNames) > 0 {
		conds = append(conds, "organisation_name = ANY("+nextArg()+")")
		args = append(args, pq.Array(f.OrganisationNames))
	}
	if f.Name != "" {
		conds = append(conds, "LOWER(role_name) LIKE '%' || LOWER("+nextArg()+") || '%'")
		args = append(args, f.Name)
	}
	if len(conds) > 0 {
		sb.WriteString("WHERE " + strings.Join(conds, "\n  AND ") + "\n")
	}
	sb.WriteString("GROUP BY role_name")

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
