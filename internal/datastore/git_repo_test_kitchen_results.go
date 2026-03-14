// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GitRepoTestKitchenResult represents a row in the git_repo_test_kitchen_results table.
type GitRepoTestKitchenResult struct {
	ID                string
	GitRepoID         string
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

// UpsertGitRepoTestKitchenResultParams contains the fields needed to insert or
// update a git_repo_test_kitchen_results row. The unique constraint is
// (git_repo_id, target_chef_version, commit_sha).
type UpsertGitRepoTestKitchenResultParams struct {
	GitRepoID         string
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

const grtkrColumns = `id, git_repo_id, target_chef_version, commit_sha,
       converge_passed, tests_passed, compatible, timed_out,
       process_stdout, process_stderr,
       converge_output, verify_output, destroy_output,
       driver_used, platform_tested, overrides_applied,
       duration_seconds, started_at, completed_at, created_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetGitRepoTestKitchenResult returns the test kitchen result for the given
// git repo ID, target Chef version, and commit SHA. Returns (nil, nil) if
// no result exists.
func (db *DB) GetGitRepoTestKitchenResult(ctx context.Context, gitRepoID, targetChefVersion, commitSHA string) (*GitRepoTestKitchenResult, error) {
	return db.getGitRepoTestKitchenResult(ctx, db.q(), gitRepoID, targetChefVersion, commitSHA)
}

func (db *DB) getGitRepoTestKitchenResult(ctx context.Context, q queryable, gitRepoID, targetChefVersion, commitSHA string) (*GitRepoTestKitchenResult, error) {
	query := `
		SELECT ` + grtkrColumns + `
		  FROM git_repo_test_kitchen_results
		 WHERE git_repo_id = $1
		   AND target_chef_version = $2
		   AND commit_sha = $3
	`

	r, err := scanGitRepoTestKitchenResult(q.QueryRowContext(ctx, query, gitRepoID, targetChefVersion, commitSHA))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting git repo test kitchen result: %w", err)
	}
	return &r, nil
}

// GetGitRepoTestKitchenResultByID returns a single test kitchen result by its
// primary key. Returns ErrNotFound if no result exists.
func (db *DB) GetGitRepoTestKitchenResultByID(ctx context.Context, id string) (*GitRepoTestKitchenResult, error) {
	query := `
		SELECT ` + grtkrColumns + `
		  FROM git_repo_test_kitchen_results
		 WHERE id = $1
	`

	r, err := scanGitRepoTestKitchenResult(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting git repo test kitchen result by id: %w", err)
	}
	return &r, nil
}

// GetLatestGitRepoTestKitchenResult returns the most recent test kitchen result
// for the given git repo ID and target Chef version, regardless of commit
// SHA. Returns (nil, nil) if no result exists.
func (db *DB) GetLatestGitRepoTestKitchenResult(ctx context.Context, gitRepoID, targetChefVersion string) (*GitRepoTestKitchenResult, error) {
	query := `
		SELECT ` + grtkrColumns + `
		  FROM git_repo_test_kitchen_results
		 WHERE git_repo_id = $1
		   AND target_chef_version = $2
		 ORDER BY started_at DESC
		 LIMIT 1
	`

	r, err := scanGitRepoTestKitchenResult(db.q().QueryRowContext(ctx, query, gitRepoID, targetChefVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting latest git repo test kitchen result: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListGitRepoTestKitchenResults returns all test kitchen results for the
// given git repo ID, ordered by target_chef_version then started_at
// descending.
func (db *DB) ListGitRepoTestKitchenResults(ctx context.Context, gitRepoID string) ([]GitRepoTestKitchenResult, error) {
	query := `
		SELECT ` + grtkrColumns + `
		  FROM git_repo_test_kitchen_results
		 WHERE git_repo_id = $1
		 ORDER BY target_chef_version, started_at DESC
	`
	return scanGitRepoTestKitchenResults(db.pool.QueryContext(ctx, query, gitRepoID))
}

// ListGitRepoTestKitchenResultsByName returns all test kitchen results for
// git repos matching the given name, ordered by target_chef_version then
// started_at descending.
func (db *DB) ListGitRepoTestKitchenResultsByName(ctx context.Context, name string) ([]GitRepoTestKitchenResult, error) {
	query := `
		SELECT tkr.id, tkr.git_repo_id, tkr.target_chef_version, tkr.commit_sha,
		       tkr.converge_passed, tkr.tests_passed, tkr.compatible, tkr.timed_out,
		       tkr.process_stdout, tkr.process_stderr,
		       tkr.converge_output, tkr.verify_output, tkr.destroy_output,
		       tkr.driver_used, tkr.platform_tested, tkr.overrides_applied,
		       tkr.duration_seconds, tkr.started_at, tkr.completed_at, tkr.created_at
		  FROM git_repo_test_kitchen_results tkr
		  JOIN git_repos gr ON gr.id = tkr.git_repo_id
		 WHERE gr.name = $1
		 ORDER BY tkr.target_chef_version, tkr.started_at DESC
	`
	return scanGitRepoTestKitchenResults(db.pool.QueryContext(ctx, query, name))
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertGitRepoTestKitchenResult inserts a new test kitchen result or updates
// the existing one for the same (git_repo_id, target_chef_version, commit_sha)
// combination. Returns the resulting row.
func (db *DB) UpsertGitRepoTestKitchenResult(ctx context.Context, p UpsertGitRepoTestKitchenResultParams) (*GitRepoTestKitchenResult, error) {
	return db.upsertGitRepoTestKitchenResult(ctx, db.q(), p)
}

func (db *DB) upsertGitRepoTestKitchenResult(ctx context.Context, q queryable, p UpsertGitRepoTestKitchenResultParams) (*GitRepoTestKitchenResult, error) {
	if p.GitRepoID == "" {
		return nil, fmt.Errorf("datastore: git_repo_id is required")
	}
	if p.TargetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target_chef_version is required")
	}
	if p.CommitSHA == "" {
		return nil, fmt.Errorf("datastore: commit_sha is required")
	}

	query := `
		INSERT INTO git_repo_test_kitchen_results (
			git_repo_id, target_chef_version, commit_sha,
			converge_passed, tests_passed, compatible, timed_out,
			process_stdout, process_stderr,
			converge_output, verify_output, destroy_output,
			driver_used, platform_tested, overrides_applied,
			duration_seconds, started_at, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (git_repo_id, target_chef_version, commit_sha)
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
		RETURNING ` + grtkrColumns + `
	`

	var completedAt sql.NullTime
	if !p.CompletedAt.IsZero() {
		completedAt = sql.NullTime{Time: p.CompletedAt, Valid: true}
	}

	r, err := scanGitRepoTestKitchenResult(q.QueryRowContext(ctx, query,
		p.GitRepoID,
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
		return nil, fmt.Errorf("datastore: upserting git repo test kitchen result: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteGitRepoTestKitchenResultsByRepo removes all test kitchen results for
// the given git repo ID. This forces a full retest on the next cycle.
func (db *DB) DeleteGitRepoTestKitchenResultsByRepo(ctx context.Context, gitRepoID string) error {
	const query = `DELETE FROM git_repo_test_kitchen_results WHERE git_repo_id = $1`
	_, err := db.pool.ExecContext(ctx, query, gitRepoID)
	if err != nil {
		return fmt.Errorf("datastore: deleting git repo test kitchen results for repo %s: %w", gitRepoID, err)
	}
	return nil
}

// DeleteGitRepoTestKitchenResultByID removes a single test kitchen result by ID.
// Returns ErrNotFound if no such result exists.
func (db *DB) DeleteGitRepoTestKitchenResultByID(ctx context.Context, id string) error {
	const query = `DELETE FROM git_repo_test_kitchen_results WHERE id = $1`
	res, err := db.pool.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting git repo test kitchen result %s: %w", id, err)
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

func scanGitRepoTestKitchenResult(row interface{ Scan(dest ...any) error }) (GitRepoTestKitchenResult, error) {
	var r GitRepoTestKitchenResult
	var stdout, stderr sql.NullString
	var convergeOut, verifyOut, destroyOut sql.NullString
	var driverUsed, platformTested sql.NullString
	var duration sql.NullInt64
	var completedAt sql.NullTime

	err := row.Scan(
		&r.ID,
		&r.GitRepoID,
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
		return GitRepoTestKitchenResult{}, err
	}

	r.ProcessStdout = stringFromNull(stdout)
	r.ProcessStderr = stringFromNull(stderr)
	r.ConvergeOutput = stringFromNull(convergeOut)
	r.VerifyOutput = stringFromNull(verifyOut)
	r.DestroyOutput = stringFromNull(destroyOut)
	r.DriverUsed = stringFromNull(driverUsed)
	r.PlatformTested = stringFromNull(platformTested)
	r.DurationSeconds = intFromNull(duration)
	r.CompletedAt = timeFromNull(completedAt)

	return r, nil
}

func scanGitRepoTestKitchenResults(rows *sql.Rows, err error) ([]GitRepoTestKitchenResult, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: listing git repo test kitchen results: %w", err)
	}
	defer rows.Close()

	var results []GitRepoTestKitchenResult
	for rows.Next() {
		var r GitRepoTestKitchenResult
		var stdout, stderr sql.NullString
		var convergeOut, verifyOut, destroyOut sql.NullString
		var driverUsed, platformTested sql.NullString
		var duration sql.NullInt64
		var completedAt sql.NullTime

		if err := rows.Scan(
			&r.ID,
			&r.GitRepoID,
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
			return nil, fmt.Errorf("datastore: scanning git repo test kitchen result row: %w", err)
		}

		r.ProcessStdout = stringFromNull(stdout)
		r.ProcessStderr = stringFromNull(stderr)
		r.ConvergeOutput = stringFromNull(convergeOut)
		r.VerifyOutput = stringFromNull(verifyOut)
		r.DestroyOutput = stringFromNull(destroyOut)
		r.DriverUsed = stringFromNull(driverUsed)
		r.PlatformTested = stringFromNull(platformTested)
		r.DurationSeconds = intFromNull(duration)
		r.CompletedAt = timeFromNull(completedAt)

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating git repo test kitchen results: %w", err)
	}
	return results, nil
}
