// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GitRepoCookstyleResult represents a row in the git_repo_cookstyle_results table.
type GitRepoCookstyleResult struct {
	ID                  string    `json:"id"`
	GitRepoID           string    `json:"git_repo_id"`
	TargetChefVersion   string    `json:"target_chef_version"`
	CommitSHA           string    `json:"commit_sha,omitempty"`
	Passed              bool      `json:"passed"`
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
// a git_repo_cookstyle_results row. The unique constraint is (git_repo_id, target_chef_version).
type UpsertGitRepoCookstyleResultParams struct {
	GitRepoID           string
	TargetChefVersion   string
	CommitSHA           string
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
// Upsert
// ---------------------------------------------------------------------------

// UpsertGitRepoCookstyleResult inserts a new git repo cookstyle result or
// updates the existing one for the same (git_repo_id, target_chef_version)
// combination. Returns the resulting row.
func (db *DB) UpsertGitRepoCookstyleResult(ctx context.Context, p UpsertGitRepoCookstyleResultParams) (*GitRepoCookstyleResult, error) {
	return db.upsertGitRepoCookstyleResult(ctx, db.q(), p)
}

func (db *DB) upsertGitRepoCookstyleResult(ctx context.Context, q queryable, p UpsertGitRepoCookstyleResultParams) (*GitRepoCookstyleResult, error) {
	const query = `
		INSERT INTO git_repo_cookstyle_results (
			git_repo_id, target_chef_version, commit_sha, passed,
			offence_count, deprecation_count, correctness_count,
			deprecation_warnings, offences,
			process_stdout, process_stderr, duration_seconds,
			scanned_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (git_repo_id, target_chef_version)
		DO UPDATE SET
			commit_sha          = EXCLUDED.commit_sha,
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
		RETURNING id, git_repo_id, target_chef_version, commit_sha, passed,
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
	var deprecationWarnings, offences []byte
	var stdout, stderr sql.NullString
	var duration sql.NullInt64

	err := q.QueryRowContext(ctx, query,
		p.GitRepoID,
		targetVersion,
		nullString(p.CommitSHA),
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
		&r.GitRepoID,
		&tvOut,
		&commitSHAOut,
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
		return nil, fmt.Errorf("datastore: upserting git repo cookstyle result: %w", err)
	}

	r.TargetChefVersion = stringFromNull(tvOut)
	r.CommitSHA = stringFromNull(commitSHAOut)
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
// repo ID and target Chef version. Returns (nil, nil) if no result exists.
func (db *DB) GetGitRepoCookstyleResult(ctx context.Context, gitRepoID, targetChefVersion string) (*GitRepoCookstyleResult, error) {
	return db.getGitRepoCookstyleResult(ctx, db.q(), gitRepoID, targetChefVersion)
}

func (db *DB) getGitRepoCookstyleResult(ctx context.Context, q queryable, gitRepoID, targetChefVersion string) (*GitRepoCookstyleResult, error) {
	const query = `
		SELECT id, git_repo_id, target_chef_version, commit_sha, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM git_repo_cookstyle_results
		 WHERE git_repo_id = $1
		   AND (target_chef_version = $2 OR ($2 = '' AND target_chef_version IS NULL))
	`

	var targetVersion sql.NullString
	if targetChefVersion != "" {
		targetVersion = sql.NullString{String: targetChefVersion, Valid: true}
	}

	r, err := scanGitRepoCookstyleResult(q.QueryRowContext(ctx, query, gitRepoID, targetVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting git repo cookstyle result: %w", err)
	}
	return &r, nil
}

// GetGitRepoCookstyleResultByID returns a single git repo cookstyle result
// by its primary key. Returns ErrNotFound if no such result exists.
func (db *DB) GetGitRepoCookstyleResultByID(ctx context.Context, id string) (*GitRepoCookstyleResult, error) {
	return db.getGitRepoCookstyleResultByID(ctx, db.q(), id)
}

func (db *DB) getGitRepoCookstyleResultByID(ctx context.Context, q queryable, id string) (*GitRepoCookstyleResult, error) {
	const query = `
		SELECT id, git_repo_id, target_chef_version, commit_sha, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM git_repo_cookstyle_results
		 WHERE id = $1
	`

	r, err := scanGitRepoCookstyleResult(q.QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting git repo cookstyle result by id: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListGitRepoCookstyleResults returns all cookstyle results for the given
// git repo ID, ordered by target_chef_version.
func (db *DB) ListGitRepoCookstyleResults(ctx context.Context, gitRepoID string) ([]GitRepoCookstyleResult, error) {
	return db.listGitRepoCookstyleResults(ctx, db.q(), gitRepoID)
}

func (db *DB) listGitRepoCookstyleResults(ctx context.Context, q queryable, gitRepoID string) ([]GitRepoCookstyleResult, error) {
	const query = `
		SELECT id, git_repo_id, target_chef_version, commit_sha, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM git_repo_cookstyle_results
		 WHERE git_repo_id = $1
		 ORDER BY target_chef_version NULLS FIRST
	`
	return scanGitRepoCookstyleResults(q.QueryContext(ctx, query, gitRepoID))
}

// ListGitRepoCookstyleResultsByName returns all cookstyle results for git
// repos with the given name. Since git repos are not org-scoped, this is the
// primary way to list results across all URLs for a cookbook name.
func (db *DB) ListGitRepoCookstyleResultsByName(ctx context.Context, name string) ([]GitRepoCookstyleResult, error) {
	return db.listGitRepoCookstyleResultsByName(ctx, db.q(), name)
}

func (db *DB) listGitRepoCookstyleResultsByName(ctx context.Context, q queryable, name string) ([]GitRepoCookstyleResult, error) {
	const query = `
		SELECT r.id, r.git_repo_id, r.target_chef_version, r.commit_sha, r.passed,
		       r.offence_count, r.deprecation_count, r.correctness_count,
		       r.deprecation_warnings, r.offences,
		       r.process_stdout, r.process_stderr, r.duration_seconds,
		       r.scanned_at, r.created_at
		  FROM git_repo_cookstyle_results r
		  JOIN git_repos gr ON gr.id = r.git_repo_id
		 WHERE gr.name = $1
		 ORDER BY gr.git_repo_url, r.target_chef_version NULLS FIRST
	`
	return scanGitRepoCookstyleResults(q.QueryContext(ctx, query, name))
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteGitRepoCookstyleResultsByRepo removes all cookstyle results for the
// given git repo ID.
func (db *DB) DeleteGitRepoCookstyleResultsByRepo(ctx context.Context, gitRepoID string) error {
	return db.deleteGitRepoCookstyleResultsByRepo(ctx, db.q(), gitRepoID)
}

func (db *DB) deleteGitRepoCookstyleResultsByRepo(ctx context.Context, q queryable, gitRepoID string) error {
	const query = `DELETE FROM git_repo_cookstyle_results WHERE git_repo_id = $1`
	_, err := q.ExecContext(ctx, query, gitRepoID)
	if err != nil {
		return fmt.Errorf("datastore: deleting git repo cookstyle results for repo %s: %w", gitRepoID, err)
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
	var deprecationWarnings, offences []byte
	var stdout, stderr sql.NullString
	var duration sql.NullInt64

	err := row.Scan(
		&r.ID,
		&r.GitRepoID,
		&tvOut,
		&commitSHA,
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
		return GitRepoCookstyleResult{}, err
	}

	r.TargetChefVersion = stringFromNull(tvOut)
	r.CommitSHA = stringFromNull(commitSHA)
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
