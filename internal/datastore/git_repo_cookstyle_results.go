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

// GitRepoCookstyleResult represents a row in the git_repo_cookstyle_results table.
type GitRepoCookstyleResult struct {
	GitRepoName         string    `json:"git_repo_name"`
	GitRepoURL          string    `json:"git_repo_url"`
	TargetChefVersion   string    `json:"target_chef_version"`
	CommitSHA           string    `json:"commit_sha,omitempty"`
	Passed              bool      `json:"passed"`
	ErrorMessage        string    `json:"error_message,omitempty"`
	OffenceCount        int       `json:"offence_count"`
	DeprecationCount    int       `json:"deprecation_count"`
	CorrectnessCount    int       `json:"correctness_count"`
	DeprecationWarnings []byte    `json:"deprecation_warnings,omitempty"` // JSONB
	Offences            []byte    `json:"offences,omitempty"`             // JSONB
	ProcessStdout       string    `json:"process_stdout,omitempty"`
	ProcessStderr       string    `json:"process_stderr,omitempty"`
	DurationSeconds     int       `json:"duration_seconds"`
	ScannedAt           time.Time `json:"scanned_at"`
	CreatedAt           time.Time `json:"created_at"`
}

// UpsertGitRepoCookstyleResultParams contains the fields needed to insert or update
// a git_repo_cookstyle_results row. The unique constraint is
// (git_repo_name, git_repo_url, target_chef_version).
type UpsertGitRepoCookstyleResultParams struct {
	GitRepoName         string
	GitRepoURL          string
	TargetChefVersion   string
	CommitSHA           string
	Passed              bool
	ErrorMessage        string
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
// Upsert
// ---------------------------------------------------------------------------

// UpsertGitRepoCookstyleResult inserts a new git repo cookstyle result or
// updates the existing one for the same (git_repo_name, git_repo_url,
// target_chef_version) combination. Returns the resulting row.
func (db *DB) UpsertGitRepoCookstyleResult(ctx context.Context, p UpsertGitRepoCookstyleResultParams) (*GitRepoCookstyleResult, error) {
	result, err := db.upsertGitRepoCookstyleResult(ctx, db.q(), p)
	if err != nil {
		return nil, err
	}
	// Recompute materialised compatibility_status column.
	if reErr := db.RecomputeGitRepoCompatibilityStatus(ctx, p.GitRepoName, p.GitRepoURL, p.TargetChefVersion); reErr != nil {
		return result, fmt.Errorf("datastore: recomputing compatibility after cookstyle upsert: %w", reErr)
	}
	return result, nil
}

func (db *DB) upsertGitRepoCookstyleResult(ctx context.Context, q queryable, p UpsertGitRepoCookstyleResultParams) (*GitRepoCookstyleResult, error) {
	const query = `
		INSERT INTO git_repo_cookstyle_results (
			git_repo_name, git_repo_url,
			target_chef_version, commit_sha, passed,
			error_message,
			offence_count, deprecation_count, correctness_count,
			deprecation_warnings, offences,
			process_stdout, process_stderr, duration_seconds,
			scanned_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (git_repo_name, git_repo_url, target_chef_version)
		DO UPDATE SET
			commit_sha          = EXCLUDED.commit_sha,
			passed              = EXCLUDED.passed,
			error_message       = EXCLUDED.error_message,
			offence_count       = EXCLUDED.offence_count,
			deprecation_count   = EXCLUDED.deprecation_count,
			correctness_count   = EXCLUDED.correctness_count,
			deprecation_warnings = EXCLUDED.deprecation_warnings,
			offences            = EXCLUDED.offences,
			process_stdout      = EXCLUDED.process_stdout,
			process_stderr      = EXCLUDED.process_stderr,
			duration_seconds    = EXCLUDED.duration_seconds,
			scanned_at          = EXCLUDED.scanned_at
		RETURNING git_repo_name, git_repo_url,
		          target_chef_version, commit_sha, passed,
		          error_message,
		          offence_count, deprecation_count, correctness_count,
		          deprecation_warnings, offences,
		          process_stdout, process_stderr, duration_seconds,
		          scanned_at, created_at
	`

	var targetVersion sql.NullString
	if p.TargetChefVersion != "" {
		targetVersion = sql.NullString{String: p.TargetChefVersion, Valid: true}
	}

	r := &GitRepoCookstyleResult{}
	var tvOut, commitSHAOut sql.NullString
	var errorMessageOut sql.NullString
	var deprecationWarnings, offences []byte
	var stdout, stderr sql.NullString
	var duration sql.NullInt64

	err := q.QueryRowContext(ctx, query,
		p.GitRepoName,
		p.GitRepoURL,
		targetVersion,
		nullString(p.CommitSHA),
		p.Passed,
		nullString(p.ErrorMessage),
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
		&r.GitRepoName,
		&r.GitRepoURL,
		&tvOut,
		&commitSHAOut,
		&r.Passed,
		&errorMessageOut,
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
		return nil, fmt.Errorf("datastore: upserting git repo cookstyle result: %w", err)
	}

	r.TargetChefVersion = stringFromNull(tvOut)
	r.CommitSHA = stringFromNull(commitSHAOut)
	r.ErrorMessage = stringFromNull(errorMessageOut)
	r.DeprecationWarnings = deprecationWarnings
	r.Offences = offences
	r.ProcessStdout = stringFromNull(stdout)
	r.ProcessStderr = stringFromNull(stderr)
	r.DurationSeconds = intFromNull(duration)

	return r, nil
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetGitRepoCookstyleResult returns the cookstyle result for the given git
// repo name, URL, and target Chef version. Returns (nil, nil) if no result
// exists.
func (db *DB) GetGitRepoCookstyleResult(ctx context.Context, gitRepoName, gitRepoURL, targetChefVersion string) (*GitRepoCookstyleResult, error) {
	return db.getGitRepoCookstyleResult(ctx, db.q(), gitRepoName, gitRepoURL, targetChefVersion)
}

func (db *DB) getGitRepoCookstyleResult(ctx context.Context, q queryable, gitRepoName, gitRepoURL, targetChefVersion string) (*GitRepoCookstyleResult, error) {
	const query = `
		SELECT git_repo_name, git_repo_url,
		       target_chef_version, commit_sha, passed,
		       error_message,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM git_repo_cookstyle_results
		 WHERE git_repo_name = $1
		   AND git_repo_url = $2
		   AND (target_chef_version = $3 OR ($3 = '' AND target_chef_version IS NULL))
	`

	var targetVersion sql.NullString
	if targetChefVersion != "" {
		targetVersion = sql.NullString{String: targetChefVersion, Valid: true}
	}

	r, err := scanGitRepoCookstyleResult(q.QueryRowContext(ctx, query, gitRepoName, gitRepoURL, targetVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting git repo cookstyle result: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListGitRepoCookstyleResults returns all cookstyle results for the given
// git repo name and URL, ordered by target_chef_version.
func (db *DB) ListGitRepoCookstyleResults(ctx context.Context, gitRepoName, gitRepoURL string) ([]GitRepoCookstyleResult, error) {
	return db.listGitRepoCookstyleResults(ctx, db.q(), gitRepoName, gitRepoURL)
}

func (db *DB) listGitRepoCookstyleResults(ctx context.Context, q queryable, gitRepoName, gitRepoURL string) ([]GitRepoCookstyleResult, error) {
	const query = `
		SELECT git_repo_name, git_repo_url,
		       target_chef_version, commit_sha, passed,
		       error_message,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM git_repo_cookstyle_results
		 WHERE git_repo_name = $1
		   AND git_repo_url = $2
		 ORDER BY target_chef_version NULLS FIRST
	`
	return scanGitRepoCookstyleResults(q.QueryContext(ctx, query, gitRepoName, gitRepoURL))
}

// ListGitRepoCookstyleResultsByName returns all cookstyle results for git
// repos with the given name. Since git repos are not org-scoped, this is the
// primary way to list results across all URLs for a cookbook name.
func (db *DB) ListGitRepoCookstyleResultsByName(ctx context.Context, name string) ([]GitRepoCookstyleResult, error) {
	return db.listGitRepoCookstyleResultsByName(ctx, db.q(), name)
}

func (db *DB) listGitRepoCookstyleResultsByName(ctx context.Context, q queryable, name string) ([]GitRepoCookstyleResult, error) {
	const query = `
		SELECT git_repo_name, git_repo_url,
		       target_chef_version, commit_sha, passed,
		       error_message,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM git_repo_cookstyle_results
		 WHERE git_repo_name = $1
		 ORDER BY git_repo_url, target_chef_version NULLS FIRST
	`
	return scanGitRepoCookstyleResults(q.QueryContext(ctx, query, name))
}

// ListAllGitRepoCookstyleResults returns all git repo cookstyle results,
// ordered by target_chef_version. This is used by dashboard and list
// handlers to determine compatibility status directly from scan results.
func (db *DB) ListAllGitRepoCookstyleResults(ctx context.Context) ([]GitRepoCookstyleResult, error) {
	return db.listAllGitRepoCookstyleResults(ctx, db.q())
}

func (db *DB) listAllGitRepoCookstyleResults(ctx context.Context, q queryable) ([]GitRepoCookstyleResult, error) {
	const query = `
		SELECT git_repo_name, git_repo_url,
		       target_chef_version, commit_sha, passed,
		       error_message,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM git_repo_cookstyle_results
		 ORDER BY target_chef_version
	`
	return scanGitRepoCookstyleResults(q.QueryContext(ctx, query))
}

// ListGitRepoCookstyleResultsByTargetVersions returns all git repo cookstyle
// results filtered by the given target chef versions. Used by the readiness
// evaluator for bulk-loading.
func (db *DB) ListGitRepoCookstyleResultsByTargetVersions(ctx context.Context, targetChefVersions []string) ([]GitRepoCookstyleResult, error) {
	if len(targetChefVersions) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(targetChefVersions))
	args := make([]any, len(targetChefVersions))
	for i, v := range targetChefVersions {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = v
	}

	query := `
		SELECT git_repo_name, git_repo_url,
		       target_chef_version, commit_sha, passed,
		       error_message,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM git_repo_cookstyle_results
		 WHERE target_chef_version IN (` + strings.Join(placeholders, ", ") + `)
	`
	return scanGitRepoCookstyleResults(db.q().QueryContext(ctx, query, args...))
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteGitRepoCookstyleResultsByRepo removes all cookstyle results for the
// given git repo name and URL.
func (db *DB) DeleteGitRepoCookstyleResultsByRepo(ctx context.Context, gitRepoName, gitRepoURL string) error {
	return db.deleteGitRepoCookstyleResultsByRepo(ctx, db.q(), gitRepoName, gitRepoURL)
}

func (db *DB) deleteGitRepoCookstyleResultsByRepo(ctx context.Context, q queryable, gitRepoName, gitRepoURL string) error {
	const query = `DELETE FROM git_repo_cookstyle_results WHERE git_repo_name = $1 AND git_repo_url = $2`
	_, err := q.ExecContext(ctx, query, gitRepoName, gitRepoURL)
	if err != nil {
		return fmt.Errorf("datastore: deleting git repo cookstyle results for repo %s (%s): %w", gitRepoName, gitRepoURL, err)
	}
	return nil
}

// DeleteAllGitRepoCookstyleResults removes all git repo cookstyle results.
// This forces a full rescan on the next collection cycle.
func (db *DB) DeleteAllGitRepoCookstyleResults(ctx context.Context) error {
	return db.deleteAllGitRepoCookstyleResults(ctx, db.q())
}

func (db *DB) deleteAllGitRepoCookstyleResults(ctx context.Context, q queryable) error {
	const query = `DELETE FROM git_repo_cookstyle_results`
	_, err := q.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("datastore: deleting all git repo cookstyle results: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanGitRepoCookstyleResult(row interface{ Scan(dest ...any) error }) (GitRepoCookstyleResult, error) {
	var r GitRepoCookstyleResult
	var tvOut, commitSHA sql.NullString
	var errorMessageOut sql.NullString
	var deprecationWarnings, offences []byte
	var stdout, stderr sql.NullString
	var duration sql.NullInt64

	err := row.Scan(
		&r.GitRepoName,
		&r.GitRepoURL,
		&tvOut,
		&commitSHA,
		&r.Passed,
		&errorMessageOut,
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
		return GitRepoCookstyleResult{}, err
	}

	r.TargetChefVersion = stringFromNull(tvOut)
	r.CommitSHA = stringFromNull(commitSHA)
	r.ErrorMessage = stringFromNull(errorMessageOut)
	r.DeprecationWarnings = deprecationWarnings
	r.Offences = offences
	r.ProcessStdout = stringFromNull(stdout)
	r.ProcessStderr = stringFromNull(stderr)
	r.DurationSeconds = intFromNull(duration)

	return r, nil
}

func scanGitRepoCookstyleResults(rows *sql.Rows, err error) ([]GitRepoCookstyleResult, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying git repo cookstyle results: %w", err)
	}
	defer rows.Close()

	var results []GitRepoCookstyleResult
	for rows.Next() {
		r, err := scanGitRepoCookstyleResult(rows)
		if err != nil {
			return nil, fmt.Errorf("datastore: scanning git repo cookstyle result row: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating git repo cookstyle result rows: %w", err)
	}
	return results, nil
}
