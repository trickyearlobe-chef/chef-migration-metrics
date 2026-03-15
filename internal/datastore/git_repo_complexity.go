// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GitRepoComplexity represents a row in the git_repo_complexity table.
type GitRepoComplexity struct {
	ID                   string    `json:"id"`
	GitRepoID            string    `json:"git_repo_id"`
	TargetChefVersion    string    `json:"target_chef_version"`
	ComplexityScore      int       `json:"complexity_score"`
	ComplexityLabel      string    `json:"complexity_label"`
	ErrorCount           int       `json:"error_count"`
	DeprecationCount     int       `json:"deprecation_count"`
	CorrectnessCount     int       `json:"correctness_count"`
	ModernizeCount       int       `json:"modernize_count"`
	AutoCorrectableCount int       `json:"auto_correctable_count"`
	ManualFixCount       int       `json:"manual_fix_count"`
	AffectedNodeCount    int       `json:"affected_node_count"`
	AffectedRoleCount    int       `json:"affected_role_count"`
	AffectedPolicyCount  int       `json:"affected_policy_count"`
	EvaluatedAt          time.Time `json:"evaluated_at"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// UpsertGitRepoComplexityParams contains the fields needed to insert or
// update a git_repo_complexity row. The unique constraint is
// (git_repo_id, target_chef_version).
type UpsertGitRepoComplexityParams struct {
	GitRepoID            string
	TargetChefVersion    string
	ComplexityScore      int
	ComplexityLabel      string
	ErrorCount           int
	DeprecationCount     int
	CorrectnessCount     int
	ModernizeCount       int
	AutoCorrectableCount int
	ManualFixCount       int
	AffectedNodeCount    int
	AffectedRoleCount    int
	AffectedPolicyCount  int
	EvaluatedAt          time.Time
}

// ---------------------------------------------------------------------------
// Column list — shared across all queries
// ---------------------------------------------------------------------------

const grcColumns = `id, git_repo_id, target_chef_version,
       complexity_score, complexity_label,
       error_count, deprecation_count, correctness_count, modernize_count,
       auto_correctable_count, manual_fix_count,
       affected_node_count, affected_role_count, affected_policy_count,
       evaluated_at, created_at, updated_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetGitRepoComplexity returns the complexity record for the given git repo
// ID and target Chef version. Returns (nil, nil) if no record exists.
func (db *DB) GetGitRepoComplexity(ctx context.Context, gitRepoID, targetChefVersion string) (*GitRepoComplexity, error) {
	return db.getGitRepoComplexity(ctx, db.q(), gitRepoID, targetChefVersion)
}

func (db *DB) getGitRepoComplexity(ctx context.Context, q queryable, gitRepoID, targetChefVersion string) (*GitRepoComplexity, error) {
	query := `
		SELECT ` + grcColumns + `
		  FROM git_repo_complexity
		 WHERE git_repo_id = $1
		   AND target_chef_version = $2
	`

	r, err := scanGitRepoComplexity(q.QueryRowContext(ctx, query, gitRepoID, targetChefVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting git repo complexity: %w", err)
	}
	return &r, nil
}

// GetGitRepoComplexityByID returns a single complexity record by its
// primary key. Returns ErrNotFound if no record exists.
func (db *DB) GetGitRepoComplexityByID(ctx context.Context, id string) (*GitRepoComplexity, error) {
	query := `
		SELECT ` + grcColumns + `
		  FROM git_repo_complexity
		 WHERE id = $1
	`

	r, err := scanGitRepoComplexity(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting git repo complexity by id: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListGitRepoComplexitiesByRepo returns all complexity records for the
// given git repo ID, ordered by target_chef_version.
func (db *DB) ListGitRepoComplexitiesByRepo(ctx context.Context, gitRepoID string) ([]GitRepoComplexity, error) {
	query := `
		SELECT ` + grcColumns + `
		  FROM git_repo_complexity
		 WHERE git_repo_id = $1
		 ORDER BY target_chef_version
	`
	return db.scanGitRepoComplexities(ctx, query, gitRepoID)
}

// ListGitRepoComplexitiesByName returns all complexity records for git repos
// with the given name, ordered by target Chef version.
func (db *DB) ListGitRepoComplexitiesByName(ctx context.Context, name string) ([]GitRepoComplexity, error) {
	query := `
		SELECT grc.id, grc.git_repo_id, grc.target_chef_version,
		       grc.complexity_score, grc.complexity_label,
		       grc.error_count, grc.deprecation_count, grc.correctness_count, grc.modernize_count,
		       grc.auto_correctable_count, grc.manual_fix_count,
		       grc.affected_node_count, grc.affected_role_count, grc.affected_policy_count,
		       grc.evaluated_at, grc.created_at, grc.updated_at
		  FROM git_repo_complexity grc
		  JOIN git_repos gr ON gr.id = grc.git_repo_id
		 WHERE gr.name = $1
		 ORDER BY grc.target_chef_version
	`
	return db.scanGitRepoComplexities(ctx, query, name)
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertGitRepoComplexity inserts a new complexity record or updates the
// existing one for the same (git_repo_id, target_chef_version) combination.
// Returns the resulting row.
func (db *DB) UpsertGitRepoComplexity(ctx context.Context, p UpsertGitRepoComplexityParams) (*GitRepoComplexity, error) {
	return db.upsertGitRepoComplexity(ctx, db.q(), p)
}

func (db *DB) upsertGitRepoComplexity(ctx context.Context, q queryable, p UpsertGitRepoComplexityParams) (*GitRepoComplexity, error) {
	if p.GitRepoID == "" {
		return nil, fmt.Errorf("datastore: git_repo_id is required")
	}
	if p.TargetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target_chef_version is required")
	}

	query := `
		INSERT INTO git_repo_complexity (
			git_repo_id, target_chef_version,
			complexity_score, complexity_label,
			error_count, deprecation_count, correctness_count, modernize_count,
			auto_correctable_count, manual_fix_count,
			affected_node_count, affected_role_count, affected_policy_count,
			evaluated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (git_repo_id, target_chef_version)
		DO UPDATE SET
			complexity_score       = EXCLUDED.complexity_score,
			complexity_label       = EXCLUDED.complexity_label,
			error_count            = EXCLUDED.error_count,
			deprecation_count      = EXCLUDED.deprecation_count,
			correctness_count      = EXCLUDED.correctness_count,
			modernize_count        = EXCLUDED.modernize_count,
			auto_correctable_count = EXCLUDED.auto_correctable_count,
			manual_fix_count       = EXCLUDED.manual_fix_count,
			affected_node_count    = EXCLUDED.affected_node_count,
			affected_role_count    = EXCLUDED.affected_role_count,
			affected_policy_count  = EXCLUDED.affected_policy_count,
			evaluated_at           = EXCLUDED.evaluated_at,
			updated_at             = now()
		RETURNING ` + grcColumns + `
	`

	r, err := scanGitRepoComplexity(q.QueryRowContext(ctx, query,
		p.GitRepoID,
		p.TargetChefVersion,
		p.ComplexityScore,
		p.ComplexityLabel,
		p.ErrorCount,
		p.DeprecationCount,
		p.CorrectnessCount,
		p.ModernizeCount,
		p.AutoCorrectableCount,
		p.ManualFixCount,
		p.AffectedNodeCount,
		p.AffectedRoleCount,
		p.AffectedPolicyCount,
		p.EvaluatedAt,
	))
	if err != nil {
		return nil, fmt.Errorf("datastore: upserting git repo complexity: %w", err)
	}
	return &r, nil
}

// UpsertGitRepoComplexityTx is the transactional variant of
// UpsertGitRepoComplexity.
func (db *DB) UpsertGitRepoComplexityTx(ctx context.Context, tx *sql.Tx, p UpsertGitRepoComplexityParams) (*GitRepoComplexity, error) {
	return db.upsertGitRepoComplexity(ctx, tx, p)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteGitRepoComplexitiesByRepo removes all complexity records for the
// given git repo ID. Forces recomputation on the next analysis cycle.
func (db *DB) DeleteGitRepoComplexitiesByRepo(ctx context.Context, gitRepoID string) error {
	const query = `DELETE FROM git_repo_complexity WHERE git_repo_id = $1`
	_, err := db.pool.ExecContext(ctx, query, gitRepoID)
	if err != nil {
		return fmt.Errorf("datastore: deleting git repo complexities for repo %s: %w", gitRepoID, err)
	}
	return nil
}

// DeleteAllGitRepoComplexities removes all git repo complexity records.
func (db *DB) DeleteAllGitRepoComplexities(ctx context.Context) error {
	const query = `DELETE FROM git_repo_complexity`
	_, err := db.pool.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("datastore: deleting all git repo complexities: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanGitRepoComplexity(row interface{ Scan(dest ...any) error }) (GitRepoComplexity, error) {
	var r GitRepoComplexity

	err := row.Scan(
		&r.ID,
		&r.GitRepoID,
		&r.TargetChefVersion,
		&r.ComplexityScore,
		&r.ComplexityLabel,
		&r.ErrorCount,
		&r.DeprecationCount,
		&r.CorrectnessCount,
		&r.ModernizeCount,
		&r.AutoCorrectableCount,
		&r.ManualFixCount,
		&r.AffectedNodeCount,
		&r.AffectedRoleCount,
		&r.AffectedPolicyCount,
		&r.EvaluatedAt,
		&r.CreatedAt,
		&r.UpdatedAt,
	)
	if err != nil {
		return GitRepoComplexity{}, err
	}

	return r, nil
}

func (db *DB) scanGitRepoComplexities(ctx context.Context, query string, args ...any) ([]GitRepoComplexity, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing git repo complexities: %w", err)
	}
	defer rows.Close()

	var results []GitRepoComplexity
	for rows.Next() {
		var r GitRepoComplexity

		if err := rows.Scan(
			&r.ID,
			&r.GitRepoID,
			&r.TargetChefVersion,
			&r.ComplexityScore,
			&r.ComplexityLabel,
			&r.ErrorCount,
			&r.DeprecationCount,
			&r.CorrectnessCount,
			&r.ModernizeCount,
			&r.AutoCorrectableCount,
			&r.ManualFixCount,
			&r.AffectedNodeCount,
			&r.AffectedRoleCount,
			&r.AffectedPolicyCount,
			&r.EvaluatedAt,
			&r.CreatedAt,
			&r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning git repo complexity row: %w", err)
		}

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating git repo complexities: %w", err)
	}
	return results, nil
}
