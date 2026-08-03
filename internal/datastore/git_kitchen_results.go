// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/tkstatus"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// GitKitchenResult represents a row in the git_kitchen_results table.
type GitKitchenResult struct {
	ID                string     `json:"id"`
	GitRepoName       string     `json:"git_repo_name"`
	GitRepoURL        string     `json:"git_repo_url"`
	TargetChefVersion string     `json:"target_chef_version"`
	CommitSHA         string     `json:"commit_sha"`
	PlatformName      string     `json:"platform_name"`
	SuiteName         string     `json:"suite_name"`
	InstanceName      string     `json:"instance_name"`
	DriverUsed        string     `json:"driver_used,omitempty"`
	Passed            *bool      `json:"passed"`
	TimedOut          bool       `json:"timed_out"`
	Output            string     `json:"output,omitempty"`
	DurationSeconds   *int       `json:"duration_seconds,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// UpsertGitKitchenResultParams contains fields to insert or update a
// per-instance result. Upsert key: (git_repo_name, git_repo_url,
// target_chef_version, platform_name, suite_name).
type UpsertGitKitchenResultParams struct {
	GitRepoName       string
	GitRepoURL        string
	TargetChefVersion string
	CommitSHA         string
	PlatformName      string
	SuiteName         string
	InstanceName      string
	DriverUsed        string
	Passed            *bool
	TimedOut          bool
	Output            string
	DurationSeconds   *int
	ErrorMessage      string
	StartedAt         *time.Time
	CompletedAt       *time.Time
}

// ---------------------------------------------------------------------------
// Column list
// ---------------------------------------------------------------------------

const gkrColumns = `id, git_repo_name, git_repo_url, target_chef_version,
	commit_sha, platform_name, suite_name, instance_name, driver_used,
	passed, timed_out, output,
	duration_seconds, error_message, started_at, completed_at, created_at`

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertGitKitchenResult inserts a new git kitchen result or updates the
// existing one for the same (git_repo_name, git_repo_url,
// target_chef_version, platform_name, suite_name) combination.
func (db *DB) UpsertGitKitchenResult(ctx context.Context, p UpsertGitKitchenResultParams) (GitKitchenResult, error) {
	result, err := db.upsertGitKitchenResult(ctx, db.q(), p)
	if err != nil {
		return result, err
	}
	// Recompute materialised tk_status/tk_passed/tk_total columns.
	if reErr := db.RecomputeGitRepoTKStatus(ctx, p.GitRepoName, p.GitRepoURL); reErr != nil {
		return result, fmt.Errorf("datastore: recomputing TK status after kitchen upsert: %w", reErr)
	}
	return result, nil
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
			git_repo_name, git_repo_url, target_chef_version,
			commit_sha, platform_name, suite_name, instance_name, driver_used,
			passed, timed_out, output,
			duration_seconds, error_message, started_at, completed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		)
		ON CONFLICT (git_repo_name, git_repo_url, target_chef_version, platform_name, suite_name)
		DO UPDATE SET
			commit_sha       = EXCLUDED.commit_sha,
			instance_name    = EXCLUDED.instance_name,
			driver_used      = EXCLUDED.driver_used,
			passed           = EXCLUDED.passed,
			timed_out        = EXCLUDED.timed_out,
			output           = EXCLUDED.output,
			duration_seconds = EXCLUDED.duration_seconds,
			error_message    = EXCLUDED.error_message,
			started_at       = EXCLUDED.started_at,
			completed_at     = EXCLUDED.completed_at
		RETURNING ` + gkrColumns

	return scanGitKitchenResult(q.QueryRowContext(ctx, query,
		p.GitRepoName,
		p.GitRepoURL,
		p.TargetChefVersion,
		p.CommitSHA,
		p.PlatformName,
		p.SuiteName,
		p.InstanceName,
		nullString(p.DriverUsed),
		nullBoolPtr(p.Passed),
		p.TimedOut,
		nullString(p.Output),
		nullIntPtr(p.DurationSeconds),
		nullString(p.ErrorMessage),
		nullTimePtr(p.StartedAt),
		nullTimePtr(p.CompletedAt),
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

// ListActiveGitKitchenResults returns results excluding user-excluded instances.
// Use this for aggregate status computation to stay consistent with SQL views.
func (db *DB) ListActiveGitKitchenResults(ctx context.Context) ([]GitKitchenResult, error) {
	query := `SELECT ` + gkrColumns + `
		FROM git_kitchen_results_active
		ORDER BY git_repo_name, target_chef_version, platform_name, suite_name`
	return scanGitKitchenResults(db.q().QueryContext(ctx, query))
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// ListGitKitchenCountsByTargetVersions returns raw pass/fail counts for every
// (git_repo_name, target_chef_version) combination. The map is keyed by
// "repoName|targetVersion". Callers use tkstatus.ComputeTKStatus to derive
// the status string.
//
// What counts as a failure — the rule every TK rollup follows:
//
//	passed = true   → a pass
//	passed = false  → a failure: the cookbook was converged and did not come up
//	passed IS NULL  → no verdict, and no evidence about the cookbook
//
// A run that timed out leaves passed NULL, and `timed_out` says only that the
// lab ran out of patience — most often waiting on a DHCP lease that never
// arrived. Counting it as a failure made readiness call the cookbook
// incompatible, overriding a CookStyle pass and blocking every node running it
// for a reason that was never about the cookbook. It is counted as neither a
// pass nor a failure, so a repo whose only evidence is a timeout reads
// untested. A cookbook that genuinely fails to converge still fails.
//
// The same rule is applied in RecomputeGitRepoTKStatus[ByName],
// getGitKitchenStatusMap and the batch-summary rollup in webapi; the Go-side
// summaries (buildTKSummaryMap, the compatibility dashboard) already skipped
// NULL, so this is also what makes the list and the detail view agree.
func (db *DB) ListGitKitchenCountsByTargetVersions(ctx context.Context, targetChefVersions []string) (map[string]tkstatus.Counts, error) {
	result := make(map[string]tkstatus.Counts)
	if len(targetChefVersions) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(targetChefVersions))
	args := make([]any, len(targetChefVersions))
	for i, v := range targetChefVersions {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = v
	}

	// passed = false only — a timeout is not a cookbook failure. See the rule
	// documented above ListGitKitchenCountsByTargetVersions.
	query := `SELECT git_repo_name, target_chef_version,
	       COUNT(*) FILTER (WHERE passed = true) AS passed_count,
	       COUNT(*) FILTER (WHERE passed = false) AS failed_count
	FROM git_kitchen_results_active
	WHERE target_chef_version IN (` + joinStrings(placeholders, ", ") + `)
	GROUP BY git_repo_name, target_chef_version
	HAVING COUNT(*) FILTER (WHERE passed IS NOT NULL) > 0`

	rows, err := db.q().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing git kitchen counts by target versions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, target string
		var passed, failed int
		if err := rows.Scan(&name, &target, &passed, &failed); err != nil {
			return nil, fmt.Errorf("datastore: scanning git kitchen count row: %w", err)
		}
		result[name+"|"+target] = tkstatus.Counts{Passed: passed, Failed: failed}
	}
	return result, rows.Err()
}

// DeleteGitKitchenResultsByRepo removes all git kitchen results for the
// given repo.
func (db *DB) DeleteGitKitchenResultsByRepo(ctx context.Context, gitRepoName string) error {
	const query = `DELETE FROM git_kitchen_results WHERE git_repo_name = $1`
	_, err := db.q().ExecContext(ctx, query, gitRepoName)
	if err != nil {
		return fmt.Errorf("datastore: deleting git kitchen results by repo: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanGitKitchenResult(row *sql.Row) (GitKitchenResult, error) {
	var r GitKitchenResult
	var driverUsed sql.NullString
	var passed sql.NullBool
	var output sql.NullString
	var durationSeconds sql.NullInt64
	var errorMessage sql.NullString
	var startedAt, completedAt sql.NullTime

	err := row.Scan(
		&r.ID,
		&r.GitRepoName,
		&r.GitRepoURL,
		&r.TargetChefVersion,
		&r.CommitSHA,
		&r.PlatformName,
		&r.SuiteName,
		&r.InstanceName,
		&driverUsed,
		&passed,
		&r.TimedOut,
		&output,
		&durationSeconds,
		&errorMessage,
		&startedAt,
		&completedAt,
		&r.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return GitKitchenResult{}, ErrNotFound
		}
		return GitKitchenResult{}, fmt.Errorf("datastore: scanning git kitchen result: %w", err)
	}

	r.DriverUsed = stringFromNull(driverUsed)
	r.Passed = boolPtrFromNull(passed)
	r.Output = stringFromNull(output)
	r.DurationSeconds = intPtrFromNull(durationSeconds)
	r.ErrorMessage = stringFromNull(errorMessage)
	r.StartedAt = timePtrFromNull(startedAt)
	r.CompletedAt = timePtrFromNull(completedAt)

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
		var driverUsed sql.NullString
		var passed sql.NullBool
		var output sql.NullString
		var durationSeconds sql.NullInt64
		var errorMessage sql.NullString
		var startedAt, completedAt sql.NullTime

		if err := rows.Scan(
			&r.ID,
			&r.GitRepoName,
			&r.GitRepoURL,
			&r.TargetChefVersion,
			&r.CommitSHA,
			&r.PlatformName,
			&r.SuiteName,
			&r.InstanceName,
			&driverUsed,
			&passed,
			&r.TimedOut,
			&output,
			&durationSeconds,
			&errorMessage,
			&startedAt,
			&completedAt,
			&r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning git kitchen result row: %w", err)
		}

		r.DriverUsed = stringFromNull(driverUsed)
		r.Passed = boolPtrFromNull(passed)
		r.Output = stringFromNull(output)
		r.DurationSeconds = intPtrFromNull(durationSeconds)
		r.ErrorMessage = stringFromNull(errorMessage)
		r.StartedAt = timePtrFromNull(startedAt)
		r.CompletedAt = timePtrFromNull(completedAt)

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating git kitchen result rows: %w", err)
	}
	return results, nil
}
