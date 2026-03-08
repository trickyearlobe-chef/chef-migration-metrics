// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CookbookComplexity represents a row in the cookbook_complexity table.
type CookbookComplexity struct {
	ID                   string
	CookbookID           string
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
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// UpsertCookbookComplexityParams contains the fields needed to insert or
// update a cookbook_complexity row. The unique constraint is
// (cookbook_id, target_chef_version).
type UpsertCookbookComplexityParams struct {
	CookbookID           string
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

const ccColumns = `id, cookbook_id, target_chef_version,
       complexity_score, complexity_label,
       error_count, deprecation_count, correctness_count, modernize_count,
       auto_correctable_count, manual_fix_count,
       affected_node_count, affected_role_count, affected_policy_count,
       evaluated_at, created_at, updated_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetCookbookComplexity returns the complexity record for the given cookbook
// ID and target Chef version. Returns (nil, nil) if no record exists.
func (db *DB) GetCookbookComplexity(ctx context.Context, cookbookID, targetChefVersion string) (*CookbookComplexity, error) {
	return db.getCookbookComplexity(ctx, db.q(), cookbookID, targetChefVersion)
}

func (db *DB) getCookbookComplexity(ctx context.Context, q queryable, cookbookID, targetChefVersion string) (*CookbookComplexity, error) {
	query := `
		SELECT ` + ccColumns + `
		  FROM cookbook_complexity
		 WHERE cookbook_id = $1
		   AND target_chef_version = $2
	`

	r, err := scanCookbookComplexity(q.QueryRowContext(ctx, query, cookbookID, targetChefVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting cookbook complexity: %w", err)
	}
	return &r, nil
}

// GetCookbookComplexityByID returns a single complexity record by its
// primary key. Returns ErrNotFound if no record exists.
func (db *DB) GetCookbookComplexityByID(ctx context.Context, id string) (*CookbookComplexity, error) {
	query := `
		SELECT ` + ccColumns + `
		  FROM cookbook_complexity
		 WHERE id = $1
	`

	r, err := scanCookbookComplexity(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting cookbook complexity by id: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListCookbookComplexitiesForCookbook returns all complexity records for the
// given cookbook ID, ordered by target_chef_version.
func (db *DB) ListCookbookComplexitiesForCookbook(ctx context.Context, cookbookID string) ([]CookbookComplexity, error) {
	query := `
		SELECT ` + ccColumns + `
		  FROM cookbook_complexity
		 WHERE cookbook_id = $1
		 ORDER BY target_chef_version
	`
	return db.scanCookbookComplexities(ctx, query, cookbookID)
}

// ListCookbookComplexitiesForOrganisation returns all complexity records for
// cookbooks belonging to the given organisation, ordered by cookbook name,
// version, and target Chef version.
func (db *DB) ListCookbookComplexitiesForOrganisation(ctx context.Context, organisationID string) ([]CookbookComplexity, error) {
	query := `
		SELECT cc.id, cc.cookbook_id, cc.target_chef_version,
		       cc.complexity_score, cc.complexity_label,
		       cc.error_count, cc.deprecation_count, cc.correctness_count, cc.modernize_count,
		       cc.auto_correctable_count, cc.manual_fix_count,
		       cc.affected_node_count, cc.affected_role_count, cc.affected_policy_count,
		       cc.evaluated_at, cc.created_at, cc.updated_at
		  FROM cookbook_complexity cc
		  JOIN cookbooks c ON c.id = cc.cookbook_id
		 WHERE c.organisation_id = $1
		 ORDER BY c.name, c.version, cc.target_chef_version
	`
	return db.scanCookbookComplexities(ctx, query, organisationID)
}

// ListCookbookComplexitiesByLabel returns all complexity records for the
// given organisation and complexity label, ordered by complexity_score
// descending.
func (db *DB) ListCookbookComplexitiesByLabel(ctx context.Context, organisationID, label string) ([]CookbookComplexity, error) {
	query := `
		SELECT cc.id, cc.cookbook_id, cc.target_chef_version,
		       cc.complexity_score, cc.complexity_label,
		       cc.error_count, cc.deprecation_count, cc.correctness_count, cc.modernize_count,
		       cc.auto_correctable_count, cc.manual_fix_count,
		       cc.affected_node_count, cc.affected_role_count, cc.affected_policy_count,
		       cc.evaluated_at, cc.created_at, cc.updated_at
		  FROM cookbook_complexity cc
		  JOIN cookbooks c ON c.id = cc.cookbook_id
		 WHERE c.organisation_id = $1
		   AND cc.complexity_label = $2
		 ORDER BY cc.complexity_score DESC, c.name, c.version
	`
	return db.scanCookbookComplexities(ctx, query, organisationID, label)
}

// ListCookbookComplexitiesByTargetVersion returns all complexity records
// for the given organisation and target Chef version, ordered by
// complexity_score descending (highest priority first).
func (db *DB) ListCookbookComplexitiesByTargetVersion(ctx context.Context, organisationID, targetChefVersion string) ([]CookbookComplexity, error) {
	query := `
		SELECT cc.id, cc.cookbook_id, cc.target_chef_version,
		       cc.complexity_score, cc.complexity_label,
		       cc.error_count, cc.deprecation_count, cc.correctness_count, cc.modernize_count,
		       cc.auto_correctable_count, cc.manual_fix_count,
		       cc.affected_node_count, cc.affected_role_count, cc.affected_policy_count,
		       cc.evaluated_at, cc.created_at, cc.updated_at
		  FROM cookbook_complexity cc
		  JOIN cookbooks c ON c.id = cc.cookbook_id
		 WHERE c.organisation_id = $1
		   AND cc.target_chef_version = $2
		 ORDER BY cc.complexity_score DESC, c.name, c.version
	`
	return db.scanCookbookComplexities(ctx, query, organisationID, targetChefVersion)
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertCookbookComplexity inserts a new complexity record or updates the
// existing one for the same (cookbook_id, target_chef_version) combination.
// Returns the resulting row.
func (db *DB) UpsertCookbookComplexity(ctx context.Context, p UpsertCookbookComplexityParams) (*CookbookComplexity, error) {
	return db.upsertCookbookComplexity(ctx, db.q(), p)
}

func (db *DB) upsertCookbookComplexity(ctx context.Context, q queryable, p UpsertCookbookComplexityParams) (*CookbookComplexity, error) {
	if p.CookbookID == "" {
		return nil, fmt.Errorf("datastore: cookbook_id is required")
	}
	if p.TargetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target_chef_version is required")
	}

	query := `
		INSERT INTO cookbook_complexity (
			cookbook_id, target_chef_version,
			complexity_score, complexity_label,
			error_count, deprecation_count, correctness_count, modernize_count,
			auto_correctable_count, manual_fix_count,
			affected_node_count, affected_role_count, affected_policy_count,
			evaluated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (cookbook_id, target_chef_version)
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
		RETURNING ` + ccColumns + `
	`

	r, err := scanCookbookComplexity(q.QueryRowContext(ctx, query,
		p.CookbookID,
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
		return nil, fmt.Errorf("datastore: upserting cookbook complexity: %w", err)
	}
	return &r, nil
}

// UpsertCookbookComplexityTx is the transactional variant of
// UpsertCookbookComplexity.
func (db *DB) UpsertCookbookComplexityTx(ctx context.Context, tx *sql.Tx, p UpsertCookbookComplexityParams) (*CookbookComplexity, error) {
	return db.upsertCookbookComplexity(ctx, tx, p)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteCookbookComplexitiesForCookbook removes all complexity records for
// the given cookbook ID. Forces recomputation on the next analysis cycle.
func (db *DB) DeleteCookbookComplexitiesForCookbook(ctx context.Context, cookbookID string) error {
	const query = `DELETE FROM cookbook_complexity WHERE cookbook_id = $1`
	_, err := db.pool.ExecContext(ctx, query, cookbookID)
	if err != nil {
		return fmt.Errorf("datastore: deleting cookbook complexities for cookbook %s: %w", cookbookID, err)
	}
	return nil
}

// DeleteCookbookComplexitiesForOrganisation removes all complexity records
// for cookbooks belonging to the given organisation.
func (db *DB) DeleteCookbookComplexitiesForOrganisation(ctx context.Context, organisationID string) error {
	const query = `
		DELETE FROM cookbook_complexity
		 WHERE cookbook_id IN (
			SELECT id FROM cookbooks WHERE organisation_id = $1
		 )
	`
	_, err := db.pool.ExecContext(ctx, query, organisationID)
	if err != nil {
		return fmt.Errorf("datastore: deleting cookbook complexities for organisation %s: %w", organisationID, err)
	}
	return nil
}

// DeleteCookbookComplexity removes a single complexity record by ID.
// Returns ErrNotFound if no such record exists.
func (db *DB) DeleteCookbookComplexity(ctx context.Context, id string) error {
	const query = `DELETE FROM cookbook_complexity WHERE id = $1`
	res, err := db.pool.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting cookbook complexity %s: %w", id, err)
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

func scanCookbookComplexity(row interface{ Scan(dest ...any) error }) (CookbookComplexity, error) {
	var r CookbookComplexity

	err := row.Scan(
		&r.ID,
		&r.CookbookID,
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
		return CookbookComplexity{}, err
	}

	return r, nil
}

func (db *DB) scanCookbookComplexities(ctx context.Context, query string, args ...any) ([]CookbookComplexity, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing cookbook complexities: %w", err)
	}
	defer rows.Close()

	var results []CookbookComplexity
	for rows.Next() {
		var r CookbookComplexity

		if err := rows.Scan(
			&r.ID,
			&r.CookbookID,
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
			return nil, fmt.Errorf("datastore: scanning cookbook complexity row: %w", err)
		}

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookbook complexities: %w", err)
	}
	return results, nil
}
