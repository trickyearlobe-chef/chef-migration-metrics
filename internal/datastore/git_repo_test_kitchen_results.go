// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// GitRepoTestKitchenResult represents a row in the git_repo_test_kitchen_results table.
type GitRepoTestKitchenResult struct {
	GitRepoName       string    `json:"git_repo_name"`
	GitRepoURL        string    `json:"git_repo_url"`
	TargetChefVersion string    `json:"target_chef_version"`
	CommitSHA         string    `json:"commit_sha"`
	ConvergePassed    bool      `json:"converge_passed"`
	TestsPassed       bool      `json:"tests_passed"`
	Compatible        bool      `json:"compatible"`
	TimedOut          bool      `json:"timed_out"`
	ProcessStdout     string    `json:"process_stdout,omitempty"`
	ProcessStderr     string    `json:"process_stderr,omitempty"`
	ConvergeOutput    string    `json:"converge_output,omitempty"`
	VerifyOutput      string    `json:"verify_output,omitempty"`
	DestroyOutput     string    `json:"destroy_output,omitempty"`
	DriverUsed        string    `json:"driver_used,omitempty"`
	PlatformTested    string    `json:"platform_tested,omitempty"`
	OverridesApplied  bool      `json:"overrides_applied"`
	DurationSeconds   int       `json:"duration_seconds"`
	StartedAt         time.Time `json:"started_at"`
	CompletedAt       time.Time `json:"completed_at"`
	CreatedAt         time.Time `json:"created_at"`
}

// UpsertGitRepoTestKitchenResultParams contains the fields needed to insert or
// update a git_repo_test_kitchen_results row. The unique constraint is
// (git_repo_name, git_repo_url, target_chef_version).
type UpsertGitRepoTestKitchenResultParams struct {
	GitRepoName       string
	GitRepoURL        string
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

const grtkrColumns = `git_repo_name, git_repo_url, target_chef_version, commit_sha,
       converge_passed, tests_passed, compatible, timed_out,
       process_stdout, process_stderr,
       converge_output, verify_output, destroy_output,
       driver_used, platform_tested, overrides_applied,
       duration_seconds, started_at, completed_at, created_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetGitRepoTestKitchenResult returns the test kitchen result for the given
// git repo name, URL, target Chef version, and commit SHA. Returns (nil, nil)
// if no result exists.
func (db *DB) GetGitRepoTestKitchenResult(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion, commitSHA string) (*GitRepoTestKitchenResult, error) {
	return db.getGitRepoTestKitchenResult(ctx, db.q(), gitRepoName, gitRepoURL, targetChefVersion, commitSHA)
}

func (db *DB) getGitRepoTestKitchenResult(ctx context.Context, q queryable, gitRepoName, gitRepoURL, targetChefVersion, commitSHA string) (*GitRepoTestKitchenResult, error) {
	query := `
		SELECT ` + grtkrColumns + `
		  FROM git_repo_test_kitchen_results
		 WHERE git_repo_name = $1
		   AND git_repo_url = $2
		   AND target_chef_version = $3
		   AND commit_sha = $4
	`

	r, err := scanGitRepoTestKitchenResult(q.QueryRowContext(ctx, query, gitRepoName, gitRepoURL, targetChefVersion, commitSHA))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting git repo test kitchen result: %w", err)
	}
	return &r, nil
}

// GetLatestGitRepoTestKitchenResult returns the most recent test kitchen result
// for the given git repo name, URL, and target Chef version, regardless of
// commit SHA. Returns (nil, nil) if no result exists.
func (db *DB) GetLatestGitRepoTestKitchenResult(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) (*GitRepoTestKitchenResult, error) {
	query := `
		SELECT ` + grtkrColumns + `
		  FROM git_repo_test_kitchen_results
		 WHERE git_repo_name = $1
		   AND git_repo_url = $2
		   AND target_chef_version = $3
		 ORDER BY started_at DESC
		 LIMIT 1
	`

	r, err := scanGitRepoTestKitchenResult(db.q().QueryRowContext(ctx, query, gitRepoName, gitRepoURL, targetChefVersion))
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
// given git repo name and URL, ordered by target_chef_version then started_at
// descending.
func (db *DB) ListGitRepoTestKitchenResults(ctx context.Context, gitRepoName, gitRepoURL string) ([]GitRepoTestKitchenResult, error) {
	query := `
		SELECT ` + grtkrColumns + `
		  FROM git_repo_test_kitchen_results
		 WHERE git_repo_name = $1
		   AND git_repo_url = $2
		 ORDER BY target_chef_version, started_at DESC
	`
	return scanGitRepoTestKitchenResults(db.pool.QueryContext(ctx, query, gitRepoName, gitRepoURL))
}

// ListGitRepoTestKitchenResultsByName returns all test kitchen results for
// git repos matching the given name, ordered by target_chef_version then
// started_at descending.
func (db *DB) ListGitRepoTestKitchenResultsByName(ctx context.Context, name string) ([]GitRepoTestKitchenResult, error) {
	query := `
		SELECT ` + grtkrColumns + `
		  FROM git_repo_test_kitchen_results
		 WHERE git_repo_name = $1
		 ORDER BY git_repo_url, target_chef_version, started_at DESC
	`
	return scanGitRepoTestKitchenResults(db.pool.QueryContext(ctx, query, name))
}

// ListAllGitRepoTestKitchenResults returns all git repo test kitchen results,
// ordered by target_chef_version.
func (db *DB) ListAllGitRepoTestKitchenResults(ctx context.Context) ([]GitRepoTestKitchenResult, error) {
	query := `
		SELECT ` + grtkrColumns + `
		  FROM git_repo_test_kitchen_results
		 ORDER BY target_chef_version
	`
	return scanGitRepoTestKitchenResults(db.pool.QueryContext(ctx, query))
}

// ListLatestGitRepoTestKitchenResults returns the latest test kitchen result
// per (git_repo_name, git_repo_url, target_chef_version) combination, filtered
// by the given target chef versions. Used by the readiness evaluator for
// bulk-loading.
func (db *DB) ListLatestGitRepoTestKitchenResults(ctx context.Context, targetChefVersions []string) ([]GitRepoTestKitchenResult, error) {
	if len(targetChefVersions) == 0 {
		return nil, nil
	}

	// Build IN clause placeholders: $1, $2, ...
	placeholders := make([]string, len(targetChefVersions))
	args := make([]any, len(targetChefVersions))
	for i, v := range targetChefVersions {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = v
	}

	query := `
		SELECT DISTINCT ON (git_repo_name, git_repo_url, target_chef_version)
		       ` + grtkrColumns + `
		  FROM git_repo_test_kitchen_results
		 WHERE target_chef_version IN (` + strings.Join(placeholders, ", ") + `)
		 ORDER BY git_repo_name, git_repo_url, target_chef_version, started_at DESC
	`
	return scanGitRepoTestKitchenResults(db.pool.QueryContext(ctx, query, args...))
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertGitRepoTestKitchenResult inserts a new test kitchen result or updates
// the existing one for the same (git_repo_name, git_repo_url,
// target_chef_version) combination. The commit_sha is updated on conflict to
// reflect the latest tested commit. Returns the resulting row.
func (db *DB) UpsertGitRepoTestKitchenResult(ctx context.Context, p UpsertGitRepoTestKitchenResultParams) (*GitRepoTestKitchenResult, error) {
	return db.upsertGitRepoTestKitchenResult(ctx, db.q(), p)
}

func (db *DB) upsertGitRepoTestKitchenResult(ctx context.Context, q queryable, p UpsertGitRepoTestKitchenResultParams) (*GitRepoTestKitchenResult, error) {
	if p.GitRepoName == "" {
		return nil, fmt.Errorf("datastore: git_repo_name is required")
	}
	if p.GitRepoURL == "" {
		return nil, fmt.Errorf("datastore: git_repo_url is required")
	}
	if p.TargetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target_chef_version is required")
	}
	if p.CommitSHA == "" {
		return nil, fmt.Errorf("datastore: commit_sha is required")
	}

	query := `
		INSERT INTO git_repo_test_kitchen_results (
			git_repo_name, git_repo_url, target_chef_version, commit_sha,
			converge_passed, tests_passed, compatible, timed_out,
			process_stdout, process_stderr,
			converge_output, verify_output, destroy_output,
			driver_used, platform_tested, overrides_applied,
			duration_seconds, started_at, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (git_repo_name, git_repo_url, target_chef_version)
		DO UPDATE SET
			commit_sha         = EXCLUDED.commit_sha,
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
		p.GitRepoName,
		p.GitRepoURL,
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
// the given git repo name and URL. This forces a full retest on the next cycle.
func (db *DB) DeleteGitRepoTestKitchenResultsByRepo(ctx context.Context, gitRepoName, gitRepoURL string) error {
	const query = `DELETE FROM git_repo_test_kitchen_results WHERE git_repo_name = $1 AND git_repo_url = $2`
	_, err := db.pool.ExecContext(ctx, query, gitRepoName, gitRepoURL)
	if err != nil {
		return fmt.Errorf("datastore: deleting git repo test kitchen results for repo %s (%s): %w", gitRepoName, gitRepoURL, err)
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
		&r.GitRepoName,
		&r.GitRepoURL,
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
			&r.GitRepoName,
			&r.GitRepoURL,
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
