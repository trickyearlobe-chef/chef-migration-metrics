// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// GitRepoFilter holds optional filter criteria for listing git repos with
// SQL WHERE clause push-down. Mirrors the NodeSnapshotFilter pattern.
type GitRepoFilter struct {
	// Name filters by case-insensitive substring match on repo name.
	Name string

	// CompatibilityStatus filters by exact match on materialised column.
	// Valid: "compatible", "incompatible", "untested", "error".
	CompatibilityStatus string

	// TKStatus filters by exact match on materialised column.
	// Valid: "passed", "failed", "partial", "untested".
	TKStatus string

	// CloneStatus filters by exact match on clone_status.
	// Valid: "ok", "failed", "pending".
	CloneStatus string

	// HasTestSuite filters by has_test_suite boolean.
	// nil means no filter.
	HasTestSuite *bool

	// KitchenExcluded filters by kitchen_excluded boolean.
	// nil means no filter.
	KitchenExcluded *bool

	// Sort specifies the column to sort by. Valid values: "name",
	// "compatibility", "tk_status", "clone_status", "last_fetched_at",
	// "has_test_suite".
	// Empty defaults to "name".
	Sort string

	// SortOrder specifies the sort direction: "asc" or "desc".
	// Empty defaults to "asc".
	SortOrder string

	// Limit caps the number of returned rows. 0 means no limit.
	Limit int

	// Offset is the number of rows to skip (for pagination).
	Offset int
}

// GitRepoFilterRow is the result row from a filtered git repo query.
type GitRepoFilterRow struct {
	GitRepo
	TotalCount int // populated from COUNT(*) OVER()
}

// buildGitRepoFilterQuery constructs the SQL query and args for
// ListGitReposFiltered. Extracted for unit testing without a database.
func buildGitRepoFilterQuery(f GitRepoFilter) (query string, args []interface{}) {
	var sb strings.Builder
	argN := 0
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	sb.WriteString("SELECT ")
	sb.WriteString(gitRepoColumns)
	sb.WriteString(", COUNT(*) OVER () AS total_count")
	sb.WriteString("\n  FROM git_repos")

	// WHERE clauses.
	var wheres []string

	if f.Name != "" {
		wheres = append(wheres, "LOWER(name) LIKE LOWER("+nextArg()+")")
		args = append(args, "%"+f.Name+"%")
	}

	if f.CompatibilityStatus != "" {
		wheres = append(wheres, "compatibility_status = "+nextArg())
		args = append(args, f.CompatibilityStatus)
	}

	if f.TKStatus != "" {
		wheres = append(wheres, "tk_status = "+nextArg())
		args = append(args, f.TKStatus)
	}

	if f.CloneStatus != "" {
		wheres = append(wheres, "clone_status = "+nextArg())
		args = append(args, f.CloneStatus)
	}

	if f.HasTestSuite != nil {
		wheres = append(wheres, "has_test_suite = "+nextArg())
		args = append(args, *f.HasTestSuite)
	}

	if f.KitchenExcluded != nil {
		wheres = append(wheres, "kitchen_excluded = "+nextArg())
		args = append(args, *f.KitchenExcluded)
	}

	if len(wheres) > 0 {
		sb.WriteString("\n WHERE ")
		sb.WriteString(strings.Join(wheres, "\n   AND "))
	}

	// Sort with whitelist validation.
	sortCol := "LOWER(name)"
	switch f.Sort {
	case "name", "":
		sortCol = "LOWER(name)"
	case "compatibility":
		sortCol = "compatibility_status"
	case "tk_status":
		sortCol = "tk_status"
	case "clone_status":
		sortCol = "clone_status"
	case "last_fetched":
		sortCol = "last_fetched_at"
	case "git_url":
		sortCol = "LOWER(git_repo_url)"
	case "has_test_suite":
		sortCol = "has_test_suite"
	}

	sortDir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortDir = "DESC"
	}

	// Primary sort + deterministic tie-breaker.
	sb.WriteString("\n ORDER BY " + sortCol + " " + sortDir + ", LOWER(name) ASC")

	// Pagination.
	if f.Limit > 0 {
		sb.WriteString("\n LIMIT " + nextArg())
		args = append(args, f.Limit)
	}
	if f.Offset > 0 {
		sb.WriteString(" OFFSET " + nextArg())
		args = append(args, f.Offset)
	}

	return sb.String(), args
}

// ListGitReposFiltered retrieves git repos matching the given filter with
// SQL-level pagination. Returns the page of results and the total filtered count.
func (db *DB) ListGitReposFiltered(ctx context.Context, f GitRepoFilter) ([]GitRepo, int, error) {
	query, args := buildGitRepoFilterQuery(f)

	rows, err := db.q().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: listing filtered git repos: %w", err)
	}
	defer rows.Close()

	var repos []GitRepo
	var totalCount int

	for rows.Next() {
		var gr GitRepo
		var commitSHA, branch, cloneErr sql.NullString
		var lastFetched sql.NullTime
		var excludeReason, excludedBy sql.NullString
		var excludedAt sql.NullTime

		if err := rows.Scan(
			&gr.Name,
			&gr.GitRepoURL,
			&commitSHA,
			&branch,
			&gr.HasTestSuite,
			&gr.CloneStatus,
			&cloneErr,
			&lastFetched,
			&gr.CreatedAt,
			&gr.UpdatedAt,
			&gr.KitchenExcluded,
			&excludeReason,
			&excludedBy,
			&excludedAt,
			&gr.CompatibilityStatus,
			&gr.TKStatus,
			&gr.TKPassed,
			&gr.TKTotal,
			&totalCount,
		); err != nil {
			return nil, 0, fmt.Errorf("datastore: scanning filtered git repo row: %w", err)
		}

		gr.HeadCommitSHA = stringFromNull(commitSHA)
		gr.DefaultBranch = stringFromNull(branch)
		gr.CloneError = stringFromNull(cloneErr)
		gr.LastFetchedAt = timeFromNull(lastFetched)
		gr.KitchenExcludeReason = stringFromNull(excludeReason)
		gr.KitchenExcludedBy = stringFromNull(excludedBy)
		gr.KitchenExcludedAt = timePtrFromNull(excludedAt)
		repos = append(repos, gr)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("datastore: iterating filtered git repo rows: %w", err)
	}

	// Handle empty-page case: if no rows returned but offset > 0,
	// total is 0 from scan. Run a fallback count.
	if len(repos) == 0 && f.Offset > 0 {
		countQuery, countArgs := buildGitRepoFilterCountQuery(f)
		if err := db.q().QueryRowContext(ctx, countQuery, countArgs...).Scan(&totalCount); err != nil {
			return nil, 0, fmt.Errorf("datastore: counting filtered git repos: %w", err)
		}
	}

	return repos, totalCount, nil
}

// buildGitRepoFilterCountQuery builds a COUNT(*) query with the same WHERE
// clauses as the main filter query. Used as fallback when the page is empty.
func buildGitRepoFilterCountQuery(f GitRepoFilter) (query string, args []interface{}) {
	var sb strings.Builder
	argN := 0
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	sb.WriteString("SELECT COUNT(*) FROM git_repos")

	var wheres []string

	if f.Name != "" {
		wheres = append(wheres, "LOWER(name) LIKE LOWER("+nextArg()+")")
		args = append(args, "%"+f.Name+"%")
	}
	if f.CompatibilityStatus != "" {
		wheres = append(wheres, "compatibility_status = "+nextArg())
		args = append(args, f.CompatibilityStatus)
	}
	if f.TKStatus != "" {
		wheres = append(wheres, "tk_status = "+nextArg())
		args = append(args, f.TKStatus)
	}
	if f.CloneStatus != "" {
		wheres = append(wheres, "clone_status = "+nextArg())
		args = append(args, f.CloneStatus)
	}
	if f.HasTestSuite != nil {
		wheres = append(wheres, "has_test_suite = "+nextArg())
		args = append(args, *f.HasTestSuite)
	}
	if f.KitchenExcluded != nil {
		wheres = append(wheres, "kitchen_excluded = "+nextArg())
		args = append(args, *f.KitchenExcluded)
	}

	if len(wheres) > 0 {
		sb.WriteString("\n WHERE ")
		sb.WriteString(strings.Join(wheres, "\n   AND "))
	}

	return sb.String(), args
}
