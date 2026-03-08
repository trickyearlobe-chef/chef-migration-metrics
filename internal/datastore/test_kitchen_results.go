// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// TestKitchenResult represents a row in the test_kitchen_results table.
type TestKitchenResult struct {
	ID                string
	CookbookID        string
	TargetChefVersion string
	CommitSHA         string
	ConvergePassed    bool
	TestsPassed       bool
	Compatible        bool
	TimedOut          bool
	ProcessStdout     string
	ProcessStderr     string
	ConvergeOutput    string
	VerifyOutput      string
	DestroyOutput     string
	DriverUsed        string
	PlatformTested    string
	OverridesApplied  bool
	DurationSeconds   int
	StartedAt         time.Time
	CompletedAt       time.Time
	CreatedAt         time.Time
}

// UpsertTestKitchenResultParams contains the fields needed to insert or
// update a test_kitchen_results row. The unique constraint is
// (cookbook_id, target_chef_version, commit_sha).
type UpsertTestKitchenResultParams struct {
	CookbookID        string
	TargetChefVersion string
	CommitSHA         string
	ConvergePassed    bool
	TestsPassed       bool
	Compatible        bool
	TimedOut          bool
	ProcessStdout     string
	ProcessStderr     string
	ConvergeOutput    string
	VerifyOutput      string
	DestroyOutput     string
	DriverUsed        string
	PlatformTested    string
	OverridesApplied  bool
	DurationSeconds   int
	StartedAt         time.Time
	CompletedAt       time.Time
}

// ---------------------------------------------------------------------------
// Column lists — shared across all queries
// ---------------------------------------------------------------------------

const tkrColumns = `id, cookbook_id, target_chef_version, commit_sha,
       converge_passed, tests_passed, compatible, timed_out,
       process_stdout, process_stderr,
       converge_output, verify_output, destroy_output,
       driver_used, platform_tested, overrides_applied,
       duration_seconds, started_at, completed_at, created_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetTestKitchenResult returns the test kitchen result for the given
// cookbook ID, target Chef version, and commit SHA. Returns (nil, nil) if
// no result exists.
func (db *DB) GetTestKitchenResult(ctx context.Context, cookbookID, targetChefVersion, commitSHA string) (*TestKitchenResult, error) {
	return db.getTestKitchenResult(ctx, db.q(), cookbookID, targetChefVersion, commitSHA)
}

func (db *DB) getTestKitchenResult(ctx context.Context, q queryable, cookbookID, targetChefVersion, commitSHA string) (*TestKitchenResult, error) {
	query := `
		SELECT ` + tkrColumns + `
		  FROM test_kitchen_results
		 WHERE cookbook_id = $1
		   AND target_chef_version = $2
		   AND commit_sha = $3
	`

	r, err := scanTestKitchenResult(q.QueryRowContext(ctx, query, cookbookID, targetChefVersion, commitSHA))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting test kitchen result: %w", err)
	}
	return &r, nil
}

// GetTestKitchenResultByID returns a single test kitchen result by its
// primary key. Returns ErrNotFound if no result exists.
func (db *DB) GetTestKitchenResultByID(ctx context.Context, id string) (*TestKitchenResult, error) {
	query := `
		SELECT ` + tkrColumns + `
		  FROM test_kitchen_results
		 WHERE id = $1
	`

	r, err := scanTestKitchenResult(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting test kitchen result by id: %w", err)
	}
	return &r, nil
}

// GetLatestTestKitchenResult returns the most recent test kitchen result
// for the given cookbook ID and target Chef version, regardless of commit
// SHA. Returns (nil, nil) if no result exists.
func (db *DB) GetLatestTestKitchenResult(ctx context.Context, cookbookID, targetChefVersion string) (*TestKitchenResult, error) {
	query := `
		SELECT ` + tkrColumns + `
		  FROM test_kitchen_results
		 WHERE cookbook_id = $1
		   AND target_chef_version = $2
		 ORDER BY started_at DESC
		 LIMIT 1
	`

	r, err := scanTestKitchenResult(db.q().QueryRowContext(ctx, query, cookbookID, targetChefVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting latest test kitchen result: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListTestKitchenResultsForCookbook returns all test kitchen results for
// the given cookbook ID, ordered by target_chef_version then started_at
// descending.
func (db *DB) ListTestKitchenResultsForCookbook(ctx context.Context, cookbookID string) ([]TestKitchenResult, error) {
	query := `
		SELECT ` + tkrColumns + `
		  FROM test_kitchen_results
		 WHERE cookbook_id = $1
		 ORDER BY target_chef_version, started_at DESC
	`
	return db.scanTestKitchenResults(ctx, query, cookbookID)
}

// ListTestKitchenResultsForOrganisation returns all test kitchen results
// for git-sourced cookbooks whose names match cookbooks belonging to the
// given organisation. This is a cross-reference join because git cookbooks
// don't have an organisation_id but are linked by name.
func (db *DB) ListTestKitchenResultsForOrganisation(ctx context.Context, organisationID string) ([]TestKitchenResult, error) {
	// The aliased query needs unqualified column names in the select.
	query := `
		SELECT tkr.id, tkr.cookbook_id, tkr.target_chef_version, tkr.commit_sha,
		       tkr.converge_passed, tkr.tests_passed, tkr.compatible, tkr.timed_out,
		       tkr.process_stdout, tkr.process_stderr,
		       tkr.converge_output, tkr.verify_output, tkr.destroy_output,
		       tkr.driver_used, tkr.platform_tested, tkr.overrides_applied,
		       tkr.duration_seconds, tkr.started_at, tkr.completed_at, tkr.created_at
		  FROM test_kitchen_results tkr
		  JOIN cookbooks c ON c.id = tkr.cookbook_id
		 WHERE c.source = 'git'
		   AND c.name IN (
		       SELECT DISTINCT cs.name FROM cookbooks cs
		        WHERE cs.organisation_id = $1 AND cs.source = 'chef_server'
		   )
		 ORDER BY c.name, tkr.target_chef_version, tkr.started_at DESC
	`
	return db.scanTestKitchenResults(ctx, query, organisationID)
}

// ListCompatibleTestKitchenResults returns all test kitchen results where
// compatible = TRUE for the given cookbook, ordered by target_chef_version.
func (db *DB) ListCompatibleTestKitchenResults(ctx context.Context, cookbookID string) ([]TestKitchenResult, error) {
	query := `
		SELECT ` + tkrColumns + `
		  FROM test_kitchen_results
		 WHERE cookbook_id = $1
		   AND compatible = TRUE
		 ORDER BY target_chef_version, started_at DESC
	`
	return db.scanTestKitchenResults(ctx, query, cookbookID)
}

// ListFailedTestKitchenResults returns all test kitchen results where
// compatible = FALSE for the given cookbook, ordered by started_at descending.
func (db *DB) ListFailedTestKitchenResults(ctx context.Context, cookbookID string) ([]TestKitchenResult, error) {
	query := `
		SELECT ` + tkrColumns + `
		  FROM test_kitchen_results
		 WHERE cookbook_id = $1
		   AND compatible = FALSE
		 ORDER BY started_at DESC
	`
	return db.scanTestKitchenResults(ctx, query, cookbookID)
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertTestKitchenResult inserts a new test kitchen result or updates the
// existing one for the same (cookbook_id, target_chef_version, commit_sha)
// combination. Returns the resulting row.
func (db *DB) UpsertTestKitchenResult(ctx context.Context, p UpsertTestKitchenResultParams) (*TestKitchenResult, error) {
	return db.upsertTestKitchenResult(ctx, db.q(), p)
}

func (db *DB) upsertTestKitchenResult(ctx context.Context, q queryable, p UpsertTestKitchenResultParams) (*TestKitchenResult, error) {
	if p.CookbookID == "" {
		return nil, fmt.Errorf("datastore: cookbook_id is required")
	}
	if p.TargetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target_chef_version is required")
	}
	if p.CommitSHA == "" {
		return nil, fmt.Errorf("datastore: commit_sha is required")
	}

	query := `
		INSERT INTO test_kitchen_results (
			cookbook_id, target_chef_version, commit_sha,
			converge_passed, tests_passed, compatible, timed_out,
			process_stdout, process_stderr,
			converge_output, verify_output, destroy_output,
			driver_used, platform_tested, overrides_applied,
			duration_seconds, started_at, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (cookbook_id, target_chef_version, commit_sha)
		DO UPDATE SET
			converge_passed    = EXCLUDED.converge_passed,
			tests_passed       = EXCLUDED.tests_passed,
			compatible         = EXCLUDED.compatible,
			timed_out          = EXCLUDED.timed_out,
			process_stdout     = EXCLUDED.process_stdout,
			process_stderr     = EXCLUDED.process_stderr,
			converge_output    = EXCLUDED.converge_output,
			verify_output      = EXCLUDED.verify_output,
			destroy_output     = EXCLUDED.destroy_output,
			driver_used        = EXCLUDED.driver_used,
			platform_tested    = EXCLUDED.platform_tested,
			overrides_applied  = EXCLUDED.overrides_applied,
			duration_seconds   = EXCLUDED.duration_seconds,
			started_at         = EXCLUDED.started_at,
			completed_at       = EXCLUDED.completed_at
		RETURNING ` + tkrColumns + `
	`

	var completedAt sql.NullTime
	if !p.CompletedAt.IsZero() {
		completedAt = sql.NullTime{Time: p.CompletedAt, Valid: true}
	}

	r, err := scanTestKitchenResult(q.QueryRowContext(ctx, query,
		p.CookbookID,
		p.TargetChefVersion,
		p.CommitSHA,
		p.ConvergePassed,
		p.TestsPassed,
		p.Compatible,
		p.TimedOut,
		nullString(p.ProcessStdout),
		nullString(p.ProcessStderr),
		nullString(p.ConvergeOutput),
		nullString(p.VerifyOutput),
		nullString(p.DestroyOutput),
		nullString(p.DriverUsed),
		nullString(p.PlatformTested),
		p.OverridesApplied,
		nullInt(p.DurationSeconds),
		p.StartedAt,
		completedAt,
	))
	if err != nil {
		return nil, fmt.Errorf("datastore: upserting test kitchen result: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteTestKitchenResultsForCookbook removes all test kitchen results for
// the given cookbook ID. This forces a full retest on the next cycle.
func (db *DB) DeleteTestKitchenResultsForCookbook(ctx context.Context, cookbookID string) error {
	const query = `DELETE FROM test_kitchen_results WHERE cookbook_id = $1`
	_, err := db.pool.ExecContext(ctx, query, cookbookID)
	if err != nil {
		return fmt.Errorf("datastore: deleting test kitchen results for cookbook %s: %w", cookbookID, err)
	}
	return nil
}

// DeleteTestKitchenResult removes a single test kitchen result by ID.
// Returns ErrNotFound if no such result exists.
func (db *DB) DeleteTestKitchenResult(ctx context.Context, id string) error {
	const query = `DELETE FROM test_kitchen_results WHERE id = $1`
	res, err := db.pool.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting test kitchen result %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTestKitchenResultsForCookbookAndVersion removes all test kitchen
// results for the given cookbook ID and target Chef version. This forces a
// retest for that specific target version.
func (db *DB) DeleteTestKitchenResultsForCookbookAndVersion(ctx context.Context, cookbookID, targetChefVersion string) error {
	const query = `DELETE FROM test_kitchen_results WHERE cookbook_id = $1 AND target_chef_version = $2`
	_, err := db.pool.ExecContext(ctx, query, cookbookID, targetChefVersion)
	if err != nil {
		return fmt.Errorf("datastore: deleting test kitchen results for cookbook %s version %s: %w", cookbookID, targetChefVersion, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanTestKitchenResult(row interface{ Scan(dest ...any) error }) (TestKitchenResult, error) {
	var r TestKitchenResult
	var stdout, stderr sql.NullString
	var convergeOut, verifyOut, destroyOut sql.NullString
	var driverUsed, platformTested sql.NullString
	var duration sql.NullInt64
	var completedAt sql.NullTime

	err := row.Scan(
		&r.ID,
		&r.CookbookID,
		&r.TargetChefVersion,
		&r.CommitSHA,
		&r.ConvergePassed,
		&r.TestsPassed,
		&r.Compatible,
		&r.TimedOut,
		&stdout,
		&stderr,
		&convergeOut,
		&verifyOut,
		&destroyOut,
		&driverUsed,
		&platformTested,
		&r.OverridesApplied,
		&duration,
		&r.StartedAt,
		&completedAt,
		&r.CreatedAt,
	)
	if err != nil {
		return TestKitchenResult{}, err
	}

	r.ProcessStdout = stringFromNull(stdout)
	r.ProcessStderr = stringFromNull(stderr)
	r.ConvergeOutput = stringFromNull(convergeOut)
	r.VerifyOutput = stringFromNull(verifyOut)
	r.DestroyOutput = stringFromNull(destroyOut)
	r.DriverUsed = stringFromNull(driverUsed)
	r.PlatformTested = stringFromNull(platformTested)
	r.DurationSeconds = intFromNull(duration)
	if completedAt.Valid {
		r.CompletedAt = completedAt.Time
	}

	return r, nil
}

func (db *DB) scanTestKitchenResults(ctx context.Context, query string, args ...any) ([]TestKitchenResult, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing test kitchen results: %w", err)
	}
	defer rows.Close()

	var results []TestKitchenResult
	for rows.Next() {
		var r TestKitchenResult
		var stdout, stderr sql.NullString
		var convergeOut, verifyOut, destroyOut sql.NullString
		var driverUsed, platformTested sql.NullString
		var duration sql.NullInt64
		var completedAt sql.NullTime

		if err := rows.Scan(
			&r.ID,
			&r.CookbookID,
			&r.TargetChefVersion,
			&r.CommitSHA,
			&r.ConvergePassed,
			&r.TestsPassed,
			&r.Compatible,
			&r.TimedOut,
			&stdout,
			&stderr,
			&convergeOut,
			&verifyOut,
			&destroyOut,
			&driverUsed,
			&platformTested,
			&r.OverridesApplied,
			&duration,
			&r.StartedAt,
			&completedAt,
			&r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning test kitchen result row: %w", err)
		}

		r.ProcessStdout = stringFromNull(stdout)
		r.ProcessStderr = stringFromNull(stderr)
		r.ConvergeOutput = stringFromNull(convergeOut)
		r.VerifyOutput = stringFromNull(verifyOut)
		r.DestroyOutput = stringFromNull(destroyOut)
		r.DriverUsed = stringFromNull(driverUsed)
		r.PlatformTested = stringFromNull(platformTested)
		r.DurationSeconds = intFromNull(duration)
		if completedAt.Valid {
			r.CompletedAt = completedAt.Time
		}

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating test kitchen results: %w", err)
	}
	return results, nil
}
