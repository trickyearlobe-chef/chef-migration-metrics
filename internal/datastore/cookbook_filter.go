// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// CookbookFilter holds optional filter criteria for listing server cookbooks
// with SQL WHERE clause push-down. This replaces the in-memory filtering
// pipeline in handleCookbooks.
type CookbookFilter struct {
	// OrganisationNames restricts results to cookbooks in these orgs.
	// Empty means all organisations.
	OrganisationNames []string

	// Name filters by case-insensitive substring match on cookbook name.
	Name string

	// Active filters by is_active. nil means no filter.
	Active *bool

	// DownloadStatus filters by exact match on download_status.
	DownloadStatus string

	// Compatibility filters by computed compatibility status.
	// Values: "compatible", "incompatible", "scan_error", "untested".
	Compatibility string

	// TKStatus filters by Test Kitchen status derived from matching git repos.
	// Values: "passed", "failed", "partial", "untested", "no_repo".
	// Supports comma-separated multi-select.
	TKStatus string

	// TargetChefVersion is used for the cookstyle results JOIN.
	// When empty, all cookbooks get "untested" compatibility.
	TargetChefVersion string

	// Limit caps returned rows. 0 means use a sensible default.
	Limit int

	// Offset for pagination.
	Offset int

	// Sort field: "name" (default), "version", "compatibility", "active",
	// "download_status", "tk_status".
	Sort string

	// SortOrder: "asc" (default) or "desc".
	SortOrder string
}

// CookbookFilterRow is the result of a filtered cookbook query — just the
// columns the list endpoint needs, not the full ServerCookbook.
type CookbookFilterRow struct {
	OrganisationName string `json:"organisation_name"`
	Name             string `json:"name"`
	Version          string `json:"version"`
	IsActive         bool   `json:"is_active"`
	IsStaleCookbook  bool   `json:"is_stale_cookbook"`
	DownloadStatus   string `json:"download_status"`
	DownloadError    string `json:"download_error,omitempty"`
	Compatibility    string `json:"compatibility"`    // "compatible", "incompatible", "scan_error", "untested"
	CookstyleStatus  string `json:"cookstyle_status"` // "ready", "needs_review", "blocked", "untested" (SoT rollup)
	TKStatus         string `json:"tk_status"`        // "passed", "failed", "partial", "untested", "no_repo"
}

// buildCookbookFilterQuery constructs the SQL query and args for
// ListCookbooksFiltered. It is extracted as a standalone function to enable
// unit testing of the WHERE/ORDER/LIMIT builder without a database.
//
// Returns (query, args). The query includes COUNT(*) OVER() as total_count
// so the caller can extract pagination metadata from every row.
func buildCookbookFilterQuery(f CookbookFilter) (string, []interface{}) {
	args := []interface{}{}
	argN := 0

	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	var sb strings.Builder

	// --- CTE: compute compatibility per cookbook ---
	sb.WriteString("WITH cb AS (\n")
	sb.WriteString("  SELECT\n")
	sb.WriteString("    sc.organisation_name,\n")
	sb.WriteString("    sc.name,\n")
	sb.WriteString("    sc.version,\n")
	sb.WriteString("    sc.is_active,\n")
	sb.WriteString("    sc.is_stale_cookbook,\n")
	sb.WriteString("    sc.download_status,\n")
	sb.WriteString("    sc.download_error,\n")

	if f.TargetChefVersion != "" {
		sb.WriteString("    CASE\n")
		sb.WriteString("      WHEN csr.error_message IS NOT NULL AND csr.error_message != '' THEN 'scan_error'\n")
		sb.WriteString("      WHEN csr.passed = true THEN 'compatible'\n")
		sb.WriteString("      WHEN csr.passed = false THEN 'incompatible'\n")
		sb.WriteString("      ELSE 'untested'\n")
		sb.WriteString("    END AS compatibility,\n")
		// CookStyle rollup status (SoT, classification-derived). Scan errors and
		// missing results have no assessable rollup, so they read 'untested'.
		sb.WriteString("    CASE\n")
		sb.WriteString("      WHEN csr.error_message IS NOT NULL AND csr.error_message != '' THEN 'untested'\n")
		sb.WriteString("      ELSE COALESCE(NULLIF(csr.cookstyle_status, ''), 'untested')\n")
		sb.WriteString("    END AS cookstyle_status\n")
	} else {
		sb.WriteString("    'untested' AS compatibility,\n")
		sb.WriteString("    'untested' AS cookstyle_status\n")
	}

	sb.WriteString("  FROM server_cookbooks sc\n")

	if f.TargetChefVersion != "" {
		p := nextArg()
		sb.WriteString("  LEFT JOIN server_cookbook_cookstyle_results csr\n")
		sb.WriteString("    ON csr.organisation_name = sc.organisation_name\n")
		sb.WriteString("    AND csr.cookbook_name = sc.name\n")
		sb.WriteString("    AND csr.cookbook_version = sc.version\n")
		sb.WriteString("    AND csr.target_chef_version = " + p + "\n")
		args = append(args, f.TargetChefVersion)
	}

	// WHERE clause inside the CTE (filters on server_cookbooks columns).
	sb.WriteString("  WHERE 1=1\n")

	if len(f.OrganisationNames) > 0 {
		sb.WriteString("    AND sc.organisation_name = ANY(" + nextArg() + ")\n")
		args = append(args, pq.Array(f.OrganisationNames))
	}

	if f.Name != "" {
		sb.WriteString("    AND LOWER(sc.name) LIKE '%' || LOWER(" + nextArg() + ") || '%'\n")
		args = append(args, f.Name)
	}

	if f.Active != nil {
		sb.WriteString("    AND sc.is_active = " + nextArg() + "\n")
		args = append(args, *f.Active)
	}

	if f.DownloadStatus != "" {
		sb.WriteString("    AND sc.download_status = " + nextArg() + "\n")
		args = append(args, f.DownloadStatus)
	}

	sb.WriteString("),\n")

	// --- CTE: TK status per cookbook name from materialised git_repos column ---
	sb.WriteString("tk AS (\n")
	sb.WriteString("  SELECT gr.name,\n")
	sb.WriteString("    COALESCE(gr.tk_status, 'untested') AS tk_status\n")
	sb.WriteString("  FROM git_repos gr\n")
	sb.WriteString(")\n")

	// --- Outer SELECT with optional compatibility and TK filter ---
	sb.WriteString("SELECT cb.organisation_name, cb.name, cb.version,\n")
	sb.WriteString("       cb.is_active, cb.is_stale_cookbook, cb.download_status,\n")
	sb.WriteString("       cb.download_error, cb.compatibility, cb.cookstyle_status,\n")
	sb.WriteString("       COALESCE(tk.tk_status, 'no_repo') AS tk_status,\n")
	sb.WriteString("       COUNT(*) OVER() AS total_count\n")
	sb.WriteString("  FROM cb\n")
	sb.WriteString("  LEFT JOIN tk ON tk.name = cb.name\n")
	sb.WriteString(" WHERE 1=1\n")

	if f.Compatibility != "" {
		sb.WriteString("   AND cb.compatibility = " + nextArg() + "\n")
		args = append(args, f.Compatibility)
	}

	if f.TKStatus != "" {
		tkValues := strings.Split(f.TKStatus, ",")
		sb.WriteString("   AND COALESCE(tk.tk_status, 'no_repo') = ANY(" + nextArg() + ")\n")
		args = append(args, pq.Array(tkValues))
	}

	// --- ORDER BY ---
	var sortExpr string
	switch f.Sort {
	case "version":
		sortExpr = "LOWER(cb.name), cb.version"
	case "compatibility":
		sortExpr = "cb.compatibility"
	case "active":
		sortExpr = "cb.is_active"
	case "download_status":
		sortExpr = "cb.download_status"
	case "tk_status":
		sortExpr = "COALESCE(tk.tk_status, 'no_repo')"
	default: // "name" or empty
		sortExpr = "LOWER(cb.name), cb.version"
	}

	sortDir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortDir = "DESC"
	}
	// Append deterministic tie-breaker for sort stability.
	tieBreaker := ", LOWER(cb.name) ASC, cb.version ASC"
	sb.WriteString(" ORDER BY " + sortExpr + " " + sortDir + tieBreaker + "\n")

	// --- Pagination ---
	if f.Limit > 0 {
		sb.WriteString(" LIMIT " + nextArg() + "\n")
		args = append(args, f.Limit)
	}
	if f.Offset > 0 {
		sb.WriteString(" OFFSET " + nextArg() + "\n")
		args = append(args, f.Offset)
	}

	return sb.String(), args
}

// ListCookbooksFiltered retrieves server cookbooks matching the given filter.
// It returns:
//   - the page of matching rows,
//   - the total count of all matching rows (for pagination metadata),
//   - any error encountered.
func (db *DB) ListCookbooksFiltered(ctx context.Context, f CookbookFilter) ([]CookbookFilterRow, int, error) {
	query, args := buildCookbookFilterQuery(f)

	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: querying filtered cookbooks: %w", err)
	}
	defer rows.Close()

	var results []CookbookFilterRow
	totalCount := 0

	for rows.Next() {
		var r CookbookFilterRow
		var downloadError sql.NullString
		var rowTotal int

		if err := rows.Scan(
			&r.OrganisationName,
			&r.Name,
			&r.Version,
			&r.IsActive,
			&r.IsStaleCookbook,
			&r.DownloadStatus,
			&downloadError,
			&r.Compatibility,
			&r.CookstyleStatus,
			&r.TKStatus,
			&rowTotal,
		); err != nil {
			return nil, 0, fmt.Errorf("datastore: scanning filtered cookbook row: %w", err)
		}

		r.DownloadError = stringFromNull(downloadError)
		totalCount = rowTotal
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("datastore: iterating filtered cookbook rows: %w", err)
	}

	return results, totalCount, nil
}
