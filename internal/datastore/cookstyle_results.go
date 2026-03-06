// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CookstyleResult represents a row in the cookstyle_results table.
type CookstyleResult struct {
	ID                  string
	CookbookID          string
	TargetChefVersion   string
	Passed              bool
	OffenceCount        int
	DeprecationCount    int
	CorrectnessCount    int
	DeprecationWarnings []byte // JSONB
	Offences            []byte // JSONB
	ProcessStdout       string
	ProcessStderr       string
	DurationSeconds     int
	ScannedAt           time.Time
	CreatedAt           time.Time
}

// UpsertCookstyleResultParams contains the fields needed to insert or update
// a cookstyle_results row. The unique constraint is (cookbook_id, target_chef_version).
type UpsertCookstyleResultParams struct {
	CookbookID          string
	TargetChefVersion   string
	Passed              bool
	OffenceCount        int
	DeprecationCount    int
	CorrectnessCount    int
	DeprecationWarnings []byte // JSONB
	Offences            []byte // JSONB
	ProcessStdout       string
	ProcessStderr       string
	DurationSeconds     int
	ScannedAt           time.Time
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetCookstyleResult returns the cookstyle result for the given cookbook ID
// and target Chef version. Returns (nil, nil) if no result exists.
func (db *DB) GetCookstyleResult(ctx context.Context, cookbookID, targetChefVersion string) (*CookstyleResult, error) {
	return db.getCookstyleResult(ctx, db.q(), cookbookID, targetChefVersion)
}

func (db *DB) getCookstyleResult(ctx context.Context, q queryable, cookbookID, targetChefVersion string) (*CookstyleResult, error) {
	const query = `
		SELECT id, cookbook_id, target_chef_version, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM cookstyle_results
		 WHERE cookbook_id = $1
		   AND (target_chef_version = $2 OR ($2 = '' AND target_chef_version IS NULL))
	`

	var targetVersion sql.NullString
	if targetChefVersion != "" {
		targetVersion = sql.NullString{String: targetChefVersion, Valid: true}
	}

	r := &CookstyleResult{}
	var tvOut sql.NullString
	var deprecationWarnings, offences []byte
	var stdout, stderr sql.NullString
	var duration sql.NullInt64

	err := q.QueryRowContext(ctx, query, cookbookID, targetVersion).Scan(
		&r.ID,
		&r.CookbookID,
		&tvOut,
		&r.Passed,
		&r.OffenceCount,
		&r.DeprecationCount,
		&r.CorrectnessCount,
		&deprecationWarnings,
		&offences,
		&stdout,
		&stderr,
		&duration,
		&r.ScannedAt,
		&r.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting cookstyle result: %w", err)
	}

	r.TargetChefVersion = stringFromNull(tvOut)
	r.DeprecationWarnings = deprecationWarnings
	r.Offences = offences
	r.ProcessStdout = stringFromNull(stdout)
	r.ProcessStderr = stringFromNull(stderr)
	r.DurationSeconds = intFromNull(duration)

	return r, nil
}

// GetCookstyleResultByID returns a single cookstyle result by its primary key.
func (db *DB) GetCookstyleResultByID(ctx context.Context, id string) (*CookstyleResult, error) {
	const query = `
		SELECT id, cookbook_id, target_chef_version, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM cookstyle_results
		 WHERE id = $1
	`

	r := &CookstyleResult{}
	var tvOut sql.NullString
	var deprecationWarnings, offences []byte
	var stdout, stderr sql.NullString
	var duration sql.NullInt64

	err := db.q().QueryRowContext(ctx, query, id).Scan(
		&r.ID,
		&r.CookbookID,
		&tvOut,
		&r.Passed,
		&r.OffenceCount,
		&r.DeprecationCount,
		&r.CorrectnessCount,
		&deprecationWarnings,
		&offences,
		&stdout,
		&stderr,
		&duration,
		&r.ScannedAt,
		&r.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting cookstyle result by id: %w", err)
	}

	r.TargetChefVersion = stringFromNull(tvOut)
	r.DeprecationWarnings = deprecationWarnings
	r.Offences = offences
	r.ProcessStdout = stringFromNull(stdout)
	r.ProcessStderr = stringFromNull(stderr)
	r.DurationSeconds = intFromNull(duration)

	return r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListCookstyleResultsForCookbook returns all cookstyle results for the
// given cookbook ID, ordered by target_chef_version.
func (db *DB) ListCookstyleResultsForCookbook(ctx context.Context, cookbookID string) ([]CookstyleResult, error) {
	const query = `
		SELECT id, cookbook_id, target_chef_version, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM cookstyle_results
		 WHERE cookbook_id = $1
		 ORDER BY target_chef_version NULLS FIRST
	`
	return db.scanCookstyleResults(ctx, query, cookbookID)
}

// ListCookstyleResultsForOrganisation returns all cookstyle results for
// cookbooks belonging to the given organisation.
func (db *DB) ListCookstyleResultsForOrganisation(ctx context.Context, organisationID string) ([]CookstyleResult, error) {
	const query = `
		SELECT cr.id, cr.cookbook_id, cr.target_chef_version, cr.passed,
		       cr.offence_count, cr.deprecation_count, cr.correctness_count,
		       cr.deprecation_warnings, cr.offences,
		       cr.process_stdout, cr.process_stderr, cr.duration_seconds,
		       cr.scanned_at, cr.created_at
		  FROM cookstyle_results cr
		  JOIN cookbooks c ON c.id = cr.cookbook_id
		 WHERE c.organisation_id = $1
		 ORDER BY c.name, c.version, cr.target_chef_version NULLS FIRST
	`
	return db.scanCookstyleResults(ctx, query, organisationID)
}

func (db *DB) scanCookstyleResults(ctx context.Context, query string, args ...any) ([]CookstyleResult, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing cookstyle results: %w", err)
	}
	defer rows.Close()

	var results []CookstyleResult
	for rows.Next() {
		var r CookstyleResult
		var tvOut sql.NullString
		var deprecationWarnings, offences []byte
		var stdout, stderr sql.NullString
		var duration sql.NullInt64

		if err := rows.Scan(
			&r.ID,
			&r.CookbookID,
			&tvOut,
			&r.Passed,
			&r.OffenceCount,
			&r.DeprecationCount,
			&r.CorrectnessCount,
			&deprecationWarnings,
			&offences,
			&stdout,
			&stderr,
			&duration,
			&r.ScannedAt,
			&r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookstyle result row: %w", err)
		}

		r.TargetChefVersion = stringFromNull(tvOut)
		r.DeprecationWarnings = deprecationWarnings
		r.Offences = offences
		r.ProcessStdout = stringFromNull(stdout)
		r.ProcessStderr = stringFromNull(stderr)
		r.DurationSeconds = intFromNull(duration)

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookstyle results: %w", err)
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertCookstyleResult inserts a new cookstyle result or updates the
// existing one for the same (cookbook_id, target_chef_version) combination.
// Returns the resulting row.
func (db *DB) UpsertCookstyleResult(ctx context.Context, p UpsertCookstyleResultParams) (*CookstyleResult, error) {
	return db.upsertCookstyleResult(ctx, db.q(), p)
}

func (db *DB) upsertCookstyleResult(ctx context.Context, q queryable, p UpsertCookstyleResultParams) (*CookstyleResult, error) {
	const query = `
		INSERT INTO cookstyle_results (
			cookbook_id, target_chef_version, passed,
			offence_count, deprecation_count, correctness_count,
			deprecation_warnings, offences,
			process_stdout, process_stderr, duration_seconds,
			scanned_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (cookbook_id, target_chef_version)
		DO UPDATE SET
			passed              = EXCLUDED.passed,
			offence_count       = EXCLUDED.offence_count,
			deprecation_count   = EXCLUDED.deprecation_count,
			correctness_count   = EXCLUDED.correctness_count,
			deprecation_warnings = EXCLUDED.deprecation_warnings,
			offences            = EXCLUDED.offences,
			process_stdout      = EXCLUDED.process_stdout,
			process_stderr      = EXCLUDED.process_stderr,
			duration_seconds    = EXCLUDED.duration_seconds,
			scanned_at          = EXCLUDED.scanned_at
		RETURNING id, cookbook_id, target_chef_version, passed,
		          offence_count, deprecation_count, correctness_count,
		          deprecation_warnings, offences,
		          process_stdout, process_stderr, duration_seconds,
		          scanned_at, created_at
	`

	var targetVersion sql.NullString
	if p.TargetChefVersion != "" {
		targetVersion = sql.NullString{String: p.TargetChefVersion, Valid: true}
	}

	r := &CookstyleResult{}
	var tvOut sql.NullString
	var deprecationWarnings, offences []byte
	var stdout, stderr sql.NullString
	var duration sql.NullInt64

	err := q.QueryRowContext(ctx, query,
		p.CookbookID,
		targetVersion,
		p.Passed,
		p.OffenceCount,
		p.DeprecationCount,
		p.CorrectnessCount,
		p.DeprecationWarnings,
		p.Offences,
		nullString(p.ProcessStdout),
		nullString(p.ProcessStderr),
		nullInt(p.DurationSeconds),
		p.ScannedAt,
	).Scan(
		&r.ID,
		&r.CookbookID,
		&tvOut,
		&r.Passed,
		&r.OffenceCount,
		&r.DeprecationCount,
		&r.CorrectnessCount,
		&deprecationWarnings,
		&offences,
		&stdout,
		&stderr,
		&duration,
		&r.ScannedAt,
		&r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("datastore: upserting cookstyle result: %w", err)
	}

	r.TargetChefVersion = stringFromNull(tvOut)
	r.DeprecationWarnings = deprecationWarnings
	r.Offences = offences
	r.ProcessStdout = stringFromNull(stdout)
	r.ProcessStderr = stringFromNull(stderr)
	r.DurationSeconds = intFromNull(duration)

	return r, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteCookstyleResultsForCookbook removes all cookstyle results for the
// given cookbook ID. This is the manual rescan mechanism — deleting existing
// results causes the scanner to re-evaluate the cookbook on the next cycle.
func (db *DB) DeleteCookstyleResultsForCookbook(ctx context.Context, cookbookID string) error {
	const query = `DELETE FROM cookstyle_results WHERE cookbook_id = $1`
	_, err := db.pool.ExecContext(ctx, query, cookbookID)
	if err != nil {
		return fmt.Errorf("datastore: deleting cookstyle results for cookbook %s: %w", cookbookID, err)
	}
	return nil
}

// DeleteCookstyleResultsForOrganisation removes all cookstyle results for
// cookbooks belonging to the given organisation. Forces a full rescan.
func (db *DB) DeleteCookstyleResultsForOrganisation(ctx context.Context, organisationID string) error {
	const query = `
		DELETE FROM cookstyle_results
		 WHERE cookbook_id IN (
			SELECT id FROM cookbooks WHERE organisation_id = $1
		 )
	`
	_, err := db.pool.ExecContext(ctx, query, organisationID)
	if err != nil {
		return fmt.Errorf("datastore: deleting cookstyle results for organisation %s: %w", organisationID, err)
	}
	return nil
}

// DeleteCookstyleResult removes a single cookstyle result by ID.
func (db *DB) DeleteCookstyleResult(ctx context.Context, id string) error {
	const query = `DELETE FROM cookstyle_results WHERE id = $1`
	res, err := db.pool.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting cookstyle result %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
