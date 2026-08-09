// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"time"
)

// ScanScopeExclusion is one operator decision about whether a file is cookbook
// code. It layers over the curated seed list in the analysis package: a row
// either names a path the seed cannot reach, or overturns a seeded pattern the
// operator disagrees with.
//
// See journeys/scan-trust.md ("The repository is not the cookbook").
type ScanScopeExclusion struct {
	ID string `json:"id"`

	// Pattern matches a repo-relative path. A pattern ending in "/*" covers that
	// directory and everything beneath it; any other pattern is a shell glob
	// anchored at the repository root.
	Pattern string `json:"pattern"`

	// Excluded is the direction of the decision. True asserts the file does not
	// execute during a converge. False asserts that it DOES — overturning a
	// seeded pattern for this estate. False is a recorded decision, not an
	// absent one, so it can be seen and reversed.
	Excluded bool `json:"excluded"`

	// Reason is why. Required in both directions: an exclusion nobody can argue
	// with is an exclusion nobody can check.
	Reason string `json:"reason"`

	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListScanScopeExclusions returns every operator decision, ordered by pattern.
func (db *DB) ListScanScopeExclusions(ctx context.Context) ([]ScanScopeExclusion, error) {
	const query = `
		SELECT id, pattern, excluded, reason, COALESCE(created_by, ''), created_at, updated_at
		FROM scan_scope_exclusions
		ORDER BY pattern`

	rows, err := db.pool.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ScanScopeExclusion
	for rows.Next() {
		var e ScanScopeExclusion
		if err := rows.Scan(&e.ID, &e.Pattern, &e.Excluded, &e.Reason,
			&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// UpsertScanScopeExclusion records or revises the decision for a pattern.
func (db *DB) UpsertScanScopeExclusion(ctx context.Context, pattern string, excluded bool, reason, createdBy string) error {
	const query = `
		INSERT INTO scan_scope_exclusions (pattern, excluded, reason, created_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (pattern) DO UPDATE SET
			excluded = EXCLUDED.excluded,
			reason = EXCLUDED.reason,
			created_by = EXCLUDED.created_by,
			updated_at = now()`

	_, err := db.pool.ExecContext(ctx, query, pattern, excluded, reason, createdBy)
	return err
}

// DeleteScanScopeExclusion removes the operator decision for a pattern. A
// seeded pattern reverts to its curated behaviour; an operator-only pattern
// stops applying entirely.
func (db *DB) DeleteScanScopeExclusion(ctx context.Context, pattern string) error {
	const query = `DELETE FROM scan_scope_exclusions WHERE pattern = $1`
	_, err := db.pool.ExecContext(ctx, query, pattern)
	return err
}
