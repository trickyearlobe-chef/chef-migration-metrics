// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// CookstyleRescoreRow is a lightweight projection of a cookstyle result row,
// containing only the fields needed for re-scoring: the composite ID (to
// identify the row for update), the stored offences JSON, the current passed
// verdict, and the error_message (to skip inconclusive scans).
type CookstyleRescoreRow struct {
	// ID is a pipe-delimited composite key that uniquely identifies the row.
	// For server results: "org|cookbook|version|target_chef_version"
	// For git results: "repo_name|repo_url|target_chef_version"
	ID           string
	Offences     []byte
	ErrorMessage string
	Passed       bool
}

// CookstylePassedUpdate carries a row ID and its new passed verdict.
type CookstylePassedUpdate struct {
	ID     string
	Passed bool
}

// ListServerCookstyleResultsForRescore returns lightweight rows for all server
// cookbook cookstyle results that have non-null offences and no error_message.
func (db *DB) ListServerCookstyleResultsForRescore(ctx context.Context) ([]CookstyleRescoreRow, error) {
	const query = `
		SELECT
			organisation_name || '|' || cookbook_name || '|' || cookbook_version || '|' || COALESCE(target_chef_version, '') AS id,
			offences,
			COALESCE(error_message, '') AS error_message,
			passed
		FROM server_cookbook_cookstyle_results
		WHERE offences IS NOT NULL
	`
	return scanCookstyleRescoreRows(db.q().QueryContext(ctx, query))
}

// ListGitRepoCookstyleResultsForRescore returns lightweight rows for all git
// repo cookstyle results that have non-null offences and no error_message.
func (db *DB) ListGitRepoCookstyleResultsForRescore(ctx context.Context) ([]CookstyleRescoreRow, error) {
	const query = `
		SELECT
			git_repo_name || '|' || git_repo_url || '|' || COALESCE(target_chef_version, '') AS id,
			offences,
			COALESCE(error_message, '') AS error_message,
			passed
		FROM git_repo_cookstyle_results
		WHERE offences IS NOT NULL
	`
	return scanCookstyleRescoreRows(db.q().QueryContext(ctx, query))
}

func scanCookstyleRescoreRows(rows *sql.Rows, err error) ([]CookstyleRescoreRow, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying cookstyle results for rescore: %w", err)
	}
	defer rows.Close()

	var results []CookstyleRescoreRow
	for rows.Next() {
		var r CookstyleRescoreRow
		if err := rows.Scan(&r.ID, &r.Offences, &r.ErrorMessage, &r.Passed); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookstyle rescore row: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookstyle rescore rows: %w", err)
	}
	return results, nil
}

// BatchUpdateServerCookstylePassed updates the passed column for the given
// server cookbook cookstyle result rows. The ID format is
// "org|cookbook|version|target_chef_version".
func (db *DB) BatchUpdateServerCookstylePassed(ctx context.Context, updates []CookstylePassedUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	// Build a batch UPDATE using a VALUES list joined to the table.
	// This avoids N individual UPDATE statements.
	var sb strings.Builder
	sb.WriteString(`
		UPDATE server_cookbook_cookstyle_results AS t
		SET passed = v.new_passed
		FROM (VALUES `)

	args := make([]any, 0, len(updates)*5)
	for i, u := range updates {
		parts := splitRescoreID(u.ID)
		if len(parts) != 4 {
			return fmt.Errorf("datastore: invalid server cookstyle rescore ID: %q", u.ID)
		}
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i * 5
		fmt.Fprintf(&sb, "($%d, $%d, $%d, $%d, $%d::boolean)",
			base+1, base+2, base+3, base+4, base+5)
		args = append(args, parts[0], parts[1], parts[2], parts[3], u.Passed)
	}

	sb.WriteString(`) AS v(org, cb, ver, tcv, new_passed)
		WHERE t.organisation_name = v.org
		  AND t.cookbook_name = v.cb
		  AND t.cookbook_version = v.ver
		  AND (t.target_chef_version = v.tcv OR (v.tcv = '' AND t.target_chef_version IS NULL))`)

	_, err := db.q().ExecContext(ctx, sb.String(), args...)
	if err != nil {
		return fmt.Errorf("datastore: batch updating server cookstyle passed: %w", err)
	}
	return nil
}

// BatchUpdateGitRepoCookstylePassed updates the passed column for the given
// git repo cookstyle result rows. The ID format is
// "repo_name|repo_url|target_chef_version".
func (db *DB) BatchUpdateGitRepoCookstylePassed(ctx context.Context, updates []CookstylePassedUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(`
		UPDATE git_repo_cookstyle_results AS t
		SET passed = v.new_passed
		FROM (VALUES `)

	args := make([]any, 0, len(updates)*4)
	for i, u := range updates {
		parts := splitRescoreID(u.ID)
		if len(parts) != 3 {
			return fmt.Errorf("datastore: invalid git repo cookstyle rescore ID: %q", u.ID)
		}
		if i > 0 {
			sb.WriteString(", ")
		}
		base := i * 4
		fmt.Fprintf(&sb, "($%d, $%d, $%d, $%d::boolean)",
			base+1, base+2, base+3, base+4)
		args = append(args, parts[0], parts[1], parts[2], u.Passed)
	}

	sb.WriteString(`) AS v(name, url, tcv, new_passed)
		WHERE t.git_repo_name = v.name
		  AND t.git_repo_url = v.url
		  AND (t.target_chef_version = v.tcv OR (v.tcv = '' AND t.target_chef_version IS NULL))`)

	_, err := db.q().ExecContext(ctx, sb.String(), args...)
	if err != nil {
		return fmt.Errorf("datastore: batch updating git repo cookstyle passed: %w", err)
	}
	return nil
}

// splitRescoreID splits a pipe-delimited rescore ID into its parts.
func splitRescoreID(id string) []string {
	return strings.Split(id, "|")
}
