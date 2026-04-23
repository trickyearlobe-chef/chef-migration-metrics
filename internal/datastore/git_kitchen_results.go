// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// GitKitchenResult represents a row in the git_kitchen_results table.
type GitKitchenResult struct {
	ID                string     `json:"id"`
	BatchID           string     `json:"batch_id,omitempty"`
	GitRepoName       string     `json:"git_repo_name"`
	GitRepoURL        string     `json:"git_repo_url"`
	TargetChefVersion string     `json:"target_chef_version"`
	CommitSHA         string     `json:"commit_sha"`
	PlatformName      string     `json:"platform_name"`
	SuiteName         string     `json:"suite_name"`
	TemplateUsed      string     `json:"template_used,omitempty"`
	DriverUsed        string     `json:"driver_used,omitempty"`
	ConvergePassed    *bool      `json:"converge_passed"`
	TestsPassed       *bool      `json:"tests_passed"`
	TimedOut          bool       `json:"timed_out"`
	ConvergeOutput    string     `json:"converge_output,omitempty"`
	VerifyOutput      string     `json:"verify_output,omitempty"`
	DestroyOutput     string     `json:"destroy_output,omitempty"`
	DurationSeconds   *int       `json:"duration_seconds,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	VMTrackingID      string     `json:"vm_tracking_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// UpsertGitKitchenResultParams contains fields to insert or update a per-instance result.
// Upsert key: (git_repo_name, git_repo_url, target_chef_version, platform_name, suite_name).
type UpsertGitKitchenResultParams struct {
	BatchID           string
	GitRepoName       string
	GitRepoURL        string
	TargetChefVersion string
	CommitSHA         string
	PlatformName      string
	SuiteName         string
	TemplateUsed      string
	DriverUsed        string
	ConvergePassed    *bool
	TestsPassed       *bool
	TimedOut          bool
	ConvergeOutput    string
	VerifyOutput      string
	DestroyOutput     string
	DurationSeconds   *int
	ErrorMessage      string
	StartedAt         *time.Time
	CompletedAt       *time.Time
	VMTrackingID      string
}

// ---------------------------------------------------------------------------
// Column list
// ---------------------------------------------------------------------------

const gkrColumns = `id, batch_id, git_repo_name, git_repo_url, target_chef_version,
	commit_sha, platform_name, suite_name, template_used, driver_used,
	converge_passed, tests_passed, timed_out,
	converge_output, verify_output, destroy_output,
	duration_seconds, error_message, started_at, completed_at,
	vm_tracking_id, created_at`

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertGitKitchenResult inserts a new git kitchen result or updates the
// existing one for the same (git_repo_name, git_repo_url,
// target_chef_version, platform_name, suite_name) combination.
func (db *DB) UpsertGitKitchenResult(ctx context.Context, p UpsertGitKitchenResultParams) (GitKitchenResult, error) {
	return db.upsertGitKitchenResult(ctx, db.q(), p)
}

func (db *DB) upsertGitKitchenResult(ctx context.Context, q queryable, p UpsertGitKitchenResultParams) (GitKitchenResult, error) {
	if p.GitRepoName == "" {
		return GitKitchenResult{}, fmt.Errorf("datastore: git_repo_name is required")
	}
	if p.GitRepoURL == "" {
		return GitKitchenResult{}, fmt.Errorf("datastore: git_repo_url is required")
	}
	if p.TargetChefVersion == "" {
		return GitKitchenResult{}, fmt.Errorf("datastore: target_chef_version is required")
	}
	if p.PlatformName == "" {
		return GitKitchenResult{}, fmt.Errorf("datastore: platform_name is required")
	}
	if p.SuiteName == "" {
		return GitKitchenResult{}, fmt.Errorf("datastore: suite_name is required")
	}

	const query = `
		INSERT INTO git_kitchen_results (
			batch_id, git_repo_name, git_repo_url, target_chef_version,
			commit_sha, platform_name, suite_name, template_used, driver_used,
			converge_passed, tests_passed, timed_out,
			converge_output, verify_output, destroy_output,
			duration_seconds, error_message, started_at, completed_at,
			vm_tracking_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19, $20
		)
		ON CONFLICT (git_repo_name, git_repo_url, target_chef_version, platform_name, suite_name)
		DO UPDATE SET
			batch_id           = EXCLUDED.batch_id,
			commit_sha         = EXCLUDED.commit_sha,
			template_used      = EXCLUDED.template_used,
			driver_used        = EXCLUDED.driver_used,
			converge_passed    = EXCLUDED.converge_passed,
			tests_passed       = EXCLUDED.tests_passed,
			timed_out          = EXCLUDED.timed_out,
			converge_output    = EXCLUDED.converge_output,
			verify_output      = EXCLUDED.verify_output,
			destroy_output     = EXCLUDED.destroy_output,
			duration_seconds   = EXCLUDED.duration_seconds,
			error_message      = EXCLUDED.error_message,
			started_at         = EXCLUDED.started_at,
			completed_at       = EXCLUDED.completed_at,
			vm_tracking_id     = EXCLUDED.vm_tracking_id
		RETURNING ` + gkrColumns

	return scanGitKitchenResult(q.QueryRowContext(ctx, query,
		nullString(p.BatchID),
		p.GitRepoName,
		p.GitRepoURL,
		p.TargetChefVersion,
		p.CommitSHA,
		p.PlatformName,
		p.SuiteName,
		nullString(p.TemplateUsed),
		nullString(p.DriverUsed),
		nullBoolPtr(p.ConvergePassed),
		nullBoolPtr(p.TestsPassed),
		p.TimedOut,
		nullString(p.ConvergeOutput),
		nullString(p.VerifyOutput),
		nullString(p.DestroyOutput),
		nullIntPtr(p.DurationSeconds),
		nullString(p.ErrorMessage),
		nullTimePtr(p.StartedAt),
		nullTimePtr(p.CompletedAt),
		nullString(p.VMTrackingID),
	))
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetGitKitchenResult returns a git kitchen result by its UUID primary key.
// Returns ErrNotFound if no such result exists.
func (db *DB) GetGitKitchenResult(ctx context.Context, id string) (GitKitchenResult, error) {
	return db.getGitKitchenResult(ctx, db.q(), id)
}

func (db *DB) getGitKitchenResult(ctx context.Context, q queryable, id string) (GitKitchenResult, error) {
	query := `SELECT ` + gkrColumns + `
		FROM git_kitchen_results
		WHERE id = $1`
	return scanGitKitchenResult(q.QueryRowContext(ctx, query, id))
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListGitKitchenResultsByBatch returns all git kitchen results for the given
// batch, ordered by git_repo_name, platform_name, suite_name.
func (db *DB) ListGitKitchenResultsByBatch(ctx context.Context, batchID string) ([]GitKitchenResult, error) {
	return db.listGitKitchenResultsByBatch(ctx, db.q(), batchID)
}

func (db *DB) listGitKitchenResultsByBatch(ctx context.Context, q queryable, batchID string) ([]GitKitchenResult, error) {
	query := `SELECT ` + gkrColumns + `
		FROM git_kitchen_results
		WHERE batch_id = $1
		ORDER BY git_repo_name, platform_name, suite_name`
	return scanGitKitchenResults(q.QueryContext(ctx, query, batchID))
}

// ListGitKitchenResultsByRepo returns all git kitchen results for the given
// git repo name, ordered by target_chef_version, platform_name, suite_name.
func (db *DB) ListGitKitchenResultsByRepo(ctx context.Context, gitRepoName string) ([]GitKitchenResult, error) {
	return db.listGitKitchenResultsByRepo(ctx, db.q(), gitRepoName)
}

func (db *DB) listGitKitchenResultsByRepo(ctx context.Context, q queryable, gitRepoName string) ([]GitKitchenResult, error) {
	query := `SELECT ` + gkrColumns + `
		FROM git_kitchen_results
		WHERE git_repo_name = $1
		ORDER BY target_chef_version, platform_name, suite_name`
	return scanGitKitchenResults(q.QueryContext(ctx, query, gitRepoName))
}

// ListGitKitchenResults returns all git kitchen results, ordered by
// git_repo_name, target_chef_version, platform_name, suite_name.
func (db *DB) ListGitKitchenResults(ctx context.Context) ([]GitKitchenResult, error) {
	return db.listGitKitchenResults(ctx, db.q())
}

func (db *DB) listGitKitchenResults(ctx context.Context, q queryable) ([]GitKitchenResult, error) {
	query := `SELECT ` + gkrColumns + `
		FROM git_kitchen_results
		ORDER BY git_repo_name, target_chef_version, platform_name, suite_name`
	return scanGitKitchenResults(q.QueryContext(ctx, query))
}

// ---------------------------------------------------------------------------
// Count
// ---------------------------------------------------------------------------

// CountGitKitchenResultsByBatch returns aggregate counts for a batch:
// passed, failed, pending, timedOut, and errored.
func (db *DB) CountGitKitchenResultsByBatch(ctx context.Context, batchID string) (passed, failed, pending, timedOut, errored int, err error) {
	return db.countGitKitchenResultsByBatch(ctx, db.q(), batchID)
}

func (db *DB) countGitKitchenResultsByBatch(ctx context.Context, q queryable, batchID string) (passed, failed, pending, timedOut, errored int, err error) {
	const query = `
		SELECT
			COUNT(*) FILTER (WHERE converge_passed = true AND tests_passed = true),
			COUNT(*) FILTER (WHERE converge_passed = false OR tests_passed = false),
			COUNT(*) FILTER (WHERE converge_passed IS NULL),
			COUNT(*) FILTER (WHERE timed_out = true),
			COUNT(*) FILTER (WHERE error_message IS NOT NULL AND error_message != '')
		FROM git_kitchen_results
		WHERE batch_id = $1`

	err = q.QueryRowContext(ctx, query, batchID).Scan(&passed, &failed, &pending, &timedOut, &errored)
	if err != nil {
		err = fmt.Errorf("datastore: counting git kitchen results by batch: %w", err)
	}
	return
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteGitKitchenResultsByBatch removes all git kitchen results for the
// given batch.
func (db *DB) DeleteGitKitchenResultsByBatch(ctx context.Context, batchID string) error {
	return db.deleteGitKitchenResultsByBatch(ctx, db.q(), batchID)
}

func (db *DB) deleteGitKitchenResultsByBatch(ctx context.Context, q queryable, batchID string) error {
	const query = `DELETE FROM git_kitchen_results WHERE batch_id = $1`

	_, err := q.ExecContext(ctx, query, batchID)
	if err != nil {
		return fmt.Errorf("datastore: deleting git kitchen results by batch: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanGitKitchenResult(row *sql.Row) (GitKitchenResult, error) {
	var r GitKitchenResult
	var batchID, templateUsed, driverUsed sql.NullString
	var convergePassed, testsPassed sql.NullBool
	var convergeOutput, verifyOutput, destroyOutput sql.NullString
	var durationSeconds sql.NullInt64
	var errorMessage sql.NullString
	var startedAt, completedAt sql.NullTime
	var vmTrackingID sql.NullString

	err := row.Scan(
		&r.ID,
		&batchID,
		&r.GitRepoName,
		&r.GitRepoURL,
		&r.TargetChefVersion,
		&r.CommitSHA,
		&r.PlatformName,
		&r.SuiteName,
		&templateUsed,
		&driverUsed,
		&convergePassed,
		&testsPassed,
		&r.TimedOut,
		&convergeOutput,
		&verifyOutput,
		&destroyOutput,
		&durationSeconds,
		&errorMessage,
		&startedAt,
		&completedAt,
		&vmTrackingID,
		&r.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return GitKitchenResult{}, ErrNotFound
		}
		return GitKitchenResult{}, fmt.Errorf("datastore: scanning git kitchen result: %w", err)
	}

	r.BatchID = stringFromNull(batchID)
	r.TemplateUsed = stringFromNull(templateUsed)
	r.DriverUsed = stringFromNull(driverUsed)
	r.ConvergePassed = boolPtrFromNull(convergePassed)
	r.TestsPassed = boolPtrFromNull(testsPassed)
	r.ConvergeOutput = stringFromNull(convergeOutput)
	r.VerifyOutput = stringFromNull(verifyOutput)
	r.DestroyOutput = stringFromNull(destroyOutput)
	r.DurationSeconds = intPtrFromNull(durationSeconds)
	r.ErrorMessage = stringFromNull(errorMessage)
	r.StartedAt = timePtrFromNull(startedAt)
	r.CompletedAt = timePtrFromNull(completedAt)
	r.VMTrackingID = stringFromNull(vmTrackingID)

	return r, nil
}

func scanGitKitchenResults(rows *sql.Rows, err error) ([]GitKitchenResult, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying git kitchen results: %w", err)
	}
	defer rows.Close()

	var results []GitKitchenResult
	for rows.Next() {
		var r GitKitchenResult
		var batchID, templateUsed, driverUsed sql.NullString
		var convergePassed, testsPassed sql.NullBool
		var convergeOutput, verifyOutput, destroyOutput sql.NullString
		var durationSeconds sql.NullInt64
		var errorMessage sql.NullString
		var startedAt, completedAt sql.NullTime
		var vmTrackingID sql.NullString

		if err := rows.Scan(
			&r.ID,
			&batchID,
			&r.GitRepoName,
			&r.GitRepoURL,
			&r.TargetChefVersion,
			&r.CommitSHA,
			&r.PlatformName,
			&r.SuiteName,
			&templateUsed,
			&driverUsed,
			&convergePassed,
			&testsPassed,
			&r.TimedOut,
			&convergeOutput,
			&verifyOutput,
			&destroyOutput,
			&durationSeconds,
			&errorMessage,
			&startedAt,
			&completedAt,
			&vmTrackingID,
			&r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning git kitchen result row: %w", err)
		}

		r.BatchID = stringFromNull(batchID)
		r.TemplateUsed = stringFromNull(templateUsed)
		r.DriverUsed = stringFromNull(driverUsed)
		r.ConvergePassed = boolPtrFromNull(convergePassed)
		r.TestsPassed = boolPtrFromNull(testsPassed)
		r.ConvergeOutput = stringFromNull(convergeOutput)
		r.VerifyOutput = stringFromNull(verifyOutput)
		r.DestroyOutput = stringFromNull(destroyOutput)
		r.DurationSeconds = intPtrFromNull(durationSeconds)
		r.ErrorMessage = stringFromNull(errorMessage)
		r.StartedAt = timePtrFromNull(startedAt)
		r.CompletedAt = timePtrFromNull(completedAt)
		r.VMTrackingID = stringFromNull(vmTrackingID)

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating git kitchen result rows: %w", err)
	}
	return results, nil
}
