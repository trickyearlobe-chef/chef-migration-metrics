// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// CookbookPlatformCoverage represents a row in the cookbook_platform_coverage table.
type CookbookPlatformCoverage struct {
	ID           string    `json:"id"`
	GitRepoID    string    `json:"git_repo_id,omitempty"`
	CookbookName string    `json:"cookbook_name"`
	CoverageData any       `json:"coverage_data"` // JSON object, decoded from JSONB
	EvaluatedAt  time.Time `json:"evaluated_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UpsertCookbookPlatformCoverageParams holds the fields for insert or update.
type UpsertCookbookPlatformCoverageParams struct {
	GitRepoID    string
	CookbookName string
	CoverageData any // Will be marshalled to JSON for the JSONB column
}

// ---------------------------------------------------------------------------
// Column lists — shared across all queries
// ---------------------------------------------------------------------------

const cpcColumns = `id, git_repo_id, cookbook_name, coverage_data, evaluated_at, created_at, updated_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetCookbookPlatformCoverage returns the coverage row for the given
// cookbook_name. Returns (nil, nil) if not found.
func (db *DB) GetCookbookPlatformCoverage(ctx context.Context, cookbookName string) (*CookbookPlatformCoverage, error) {
	query := `
		SELECT ` + cpcColumns + `
		  FROM cookbook_platform_coverage
		 WHERE cookbook_name = $1
	`

	r, err := scanCookbookPlatformCoverage(db.q().QueryRowContext(ctx, query, cookbookName))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting cookbook platform coverage: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListCookbookPlatformCoverages returns all cookbook platform coverage rows,
// ordered by cookbook_name.
func (db *DB) ListCookbookPlatformCoverages(ctx context.Context) ([]CookbookPlatformCoverage, error) {
	query := `
		SELECT ` + cpcColumns + `
		  FROM cookbook_platform_coverage
		 ORDER BY cookbook_name
	`
	return scanCookbookPlatformCoverages(db.pool.QueryContext(ctx, query))
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertCookbookPlatformCoverage inserts a new cookbook platform coverage row
// or updates the existing one for the same cookbook_name. Returns the
// resulting row.
func (db *DB) UpsertCookbookPlatformCoverage(ctx context.Context, p UpsertCookbookPlatformCoverageParams) (*CookbookPlatformCoverage, error) {
	if p.CookbookName == "" {
		return nil, fmt.Errorf("datastore: cookbook_name is required")
	}

	var coverageJSON []byte
	if p.CoverageData == nil {
		coverageJSON = []byte("{}")
	} else {
		var err error
		coverageJSON, err = json.Marshal(p.CoverageData)
		if err != nil {
			return nil, fmt.Errorf("datastore: marshalling coverage_data: %w", err)
		}
	}

	query := `
		INSERT INTO cookbook_platform_coverage (git_repo_id, cookbook_name, coverage_data, evaluated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (cookbook_name) DO UPDATE SET
			git_repo_id   = EXCLUDED.git_repo_id,
			coverage_data = EXCLUDED.coverage_data,
			evaluated_at  = NOW(),
			updated_at    = NOW()
		RETURNING ` + cpcColumns + `
	`

	r, err := scanCookbookPlatformCoverage(db.q().QueryRowContext(ctx, query,
		nullString(p.GitRepoID),
		p.CookbookName,
		coverageJSON,
	))
	if err != nil {
		return nil, fmt.Errorf("datastore: upserting cookbook platform coverage: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteCookbookPlatformCoverage removes a cookbook platform coverage row by
// cookbook_name. Returns ErrNotFound if no such row exists.
func (db *DB) DeleteCookbookPlatformCoverage(ctx context.Context, cookbookName string) error {
	const query = `DELETE FROM cookbook_platform_coverage WHERE cookbook_name = $1`
	res, err := db.pool.ExecContext(ctx, query, cookbookName)
	if err != nil {
		return fmt.Errorf("datastore: deleting cookbook platform coverage %s: %w", cookbookName, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanCookbookPlatformCoverage(row interface{ Scan(dest ...any) error }) (CookbookPlatformCoverage, error) {
	var r CookbookPlatformCoverage
	var gitRepoID sql.NullString
	var coverageJSON []byte

	err := row.Scan(&r.ID, &gitRepoID, &r.CookbookName, &coverageJSON, &r.EvaluatedAt, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return CookbookPlatformCoverage{}, err
	}

	r.GitRepoID = stringFromNull(gitRepoID)
	if len(coverageJSON) > 0 {
		if err := json.Unmarshal(coverageJSON, &r.CoverageData); err != nil {
			return CookbookPlatformCoverage{}, fmt.Errorf("corrupt coverage_data JSON for %s: %w", r.CookbookName, err)
		}
	}
	return r, nil
}

func scanCookbookPlatformCoverages(rows *sql.Rows, err error) ([]CookbookPlatformCoverage, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: listing cookbook platform coverages: %w", err)
	}
	defer rows.Close()

	var results []CookbookPlatformCoverage
	for rows.Next() {
		var r CookbookPlatformCoverage
		var gitRepoID sql.NullString
		var coverageJSON []byte

		if err := rows.Scan(&r.ID, &gitRepoID, &r.CookbookName, &coverageJSON, &r.EvaluatedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookbook platform coverage row: %w", err)
		}

		r.GitRepoID = stringFromNull(gitRepoID)
		if len(coverageJSON) > 0 {
			if err := json.Unmarshal(coverageJSON, &r.CoverageData); err != nil {
				return nil, fmt.Errorf("corrupt coverage_data JSON for %s: %w", r.CookbookName, err)
			}
		}

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookbook platform coverages: %w", err)
	}
	return results, nil
}
