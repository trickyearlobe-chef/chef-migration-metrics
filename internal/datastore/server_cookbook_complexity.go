// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ServerCookbookComplexity represents a row in the server_cookbook_complexity table.
type ServerCookbookComplexity struct {
	ID                   string
	ServerCookbookID     string
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

// UpsertServerCookbookComplexityParams contains the fields needed to insert or
// update a server_cookbook_complexity row. The unique constraint is
// (server_cookbook_id, target_chef_version).
type UpsertServerCookbookComplexityParams struct {
	ServerCookbookID     string
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

const sccColumns = `id, server_cookbook_id, target_chef_version,
       complexity_score, complexity_label,
       error_count, deprecation_count, correctness_count, modernize_count,
       auto_correctable_count, manual_fix_count,
       affected_node_count, affected_role_count, affected_policy_count,
       evaluated_at, created_at, updated_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetServerCookbookComplexity returns the complexity record for the given
// server cookbook ID and target Chef version. Returns (nil, nil) if no record
// exists.
func (db *DB) GetServerCookbookComplexity(ctx context.Context, serverCookbookID, targetChefVersion string) (*ServerCookbookComplexity, error) {
	return db.getServerCookbookComplexity(ctx, db.q(), serverCookbookID, targetChefVersion)
}

func (db *DB) getServerCookbookComplexity(ctx context.Context, q queryable, serverCookbookID, targetChefVersion string) (*ServerCookbookComplexity, error) {
	query := `
		SELECT ` + sccColumns + `
		  FROM server_cookbook_complexity
		 WHERE server_cookbook_id = $1
		   AND target_chef_version = $2
	`

	r, err := scanServerCookbookComplexity(q.QueryRowContext(ctx, query, serverCookbookID, targetChefVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting server cookbook complexity: %w", err)
	}
	return &r, nil
}

// GetServerCookbookComplexityByID returns a single complexity record by its
// primary key. Returns ErrNotFound if no record exists.
func (db *DB) GetServerCookbookComplexityByID(ctx context.Context, id string) (*ServerCookbookComplexity, error) {
	query := `
		SELECT ` + sccColumns + `
		  FROM server_cookbook_complexity
		 WHERE id = $1
	`

	r, err := scanServerCookbookComplexity(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting server cookbook complexity by id: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListServerCookbookComplexitiesByCookbook returns all complexity records for
// the given server cookbook ID, ordered by target_chef_version.
func (db *DB) ListServerCookbookComplexitiesByCookbook(ctx context.Context, serverCookbookID string) ([]ServerCookbookComplexity, error) {
	query := `
		SELECT ` + sccColumns + `
		  FROM server_cookbook_complexity
		 WHERE server_cookbook_id = $1
		 ORDER BY target_chef_version
	`
	return db.scanServerCookbookComplexities(ctx, query, serverCookbookID)
}

// ListServerCookbookComplexitiesByOrganisation returns all complexity records
// for server cookbooks belonging to the given organisation, ordered by
// cookbook name, version, and target Chef version.
func (db *DB) ListServerCookbookComplexitiesByOrganisation(ctx context.Context, organisationID string) ([]ServerCookbookComplexity, error) {
	query := `
		SELECT scc.id, scc.server_cookbook_id, scc.target_chef_version,
		       scc.complexity_score, scc.complexity_label,
		       scc.error_count, scc.deprecation_count, scc.correctness_count, scc.modernize_count,
		       scc.auto_correctable_count, scc.manual_fix_count,
		       scc.affected_node_count, scc.affected_role_count, scc.affected_policy_count,
		       scc.evaluated_at, scc.created_at, scc.updated_at
		  FROM server_cookbook_complexity scc
		  JOIN server_cookbooks sc ON sc.id = scc.server_cookbook_id
		 WHERE sc.organisation_id = $1
		 ORDER BY sc.name, sc.version, scc.target_chef_version
	`
	return db.scanServerCookbookComplexities(ctx, query, organisationID)
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertServerCookbookComplexity inserts a new complexity record or updates
// the existing one for the same (server_cookbook_id, target_chef_version)
// combination. Returns the resulting row.
func (db *DB) UpsertServerCookbookComplexity(ctx context.Context, p UpsertServerCookbookComplexityParams) (*ServerCookbookComplexity, error) {
	return db.upsertServerCookbookComplexity(ctx, db.q(), p)
}

func (db *DB) upsertServerCookbookComplexity(ctx context.Context, q queryable, p UpsertServerCookbookComplexityParams) (*ServerCookbookComplexity, error) {
	if p.ServerCookbookID == "" {
		return nil, fmt.Errorf("datastore: server_cookbook_id is required")
	}
	if p.TargetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target_chef_version is required")
	}

	query := `
		INSERT INTO server_cookbook_complexity (
			server_cookbook_id, target_chef_version,
			complexity_score, complexity_label,
			error_count, deprecation_count, correctness_count, modernize_count,
			auto_correctable_count, manual_fix_count,
			affected_node_count, affected_role_count, affected_policy_count,
			evaluated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (server_cookbook_id, target_chef_version)
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
		RETURNING ` + sccColumns + `
	`

	r, err := scanServerCookbookComplexity(q.QueryRowContext(ctx, query,
		p.ServerCookbookID,
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
		return nil, fmt.Errorf("datastore: upserting server cookbook complexity: %w", err)
	}
	return &r, nil
}

// UpsertServerCookbookComplexityTx is the transactional variant of
// UpsertServerCookbookComplexity.
func (db *DB) UpsertServerCookbookComplexityTx(ctx context.Context, tx *sql.Tx, p UpsertServerCookbookComplexityParams) (*ServerCookbookComplexity, error) {
	return db.upsertServerCookbookComplexity(ctx, tx, p)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteServerCookbookComplexitiesByCookbook removes all complexity records
// for the given server cookbook ID.
func (db *DB) DeleteServerCookbookComplexitiesByCookbook(ctx context.Context, serverCookbookID string) error {
	const query = `DELETE FROM server_cookbook_complexity WHERE server_cookbook_id = $1`
	_, err := db.pool.ExecContext(ctx, query, serverCookbookID)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook complexities for cookbook %s: %w", serverCookbookID, err)
	}
	return nil
}

// DeleteServerCookbookComplexitiesByOrganisation removes all complexity
// records for server cookbooks belonging to the given organisation.
func (db *DB) DeleteServerCookbookComplexitiesByOrganisation(ctx context.Context, organisationID string) error {
	const query = `
		DELETE FROM server_cookbook_complexity
		 WHERE server_cookbook_id IN (
			SELECT id FROM server_cookbooks WHERE organisation_id = $1
		 )
	`
	_, err := db.pool.ExecContext(ctx, query, organisationID)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook complexities for organisation %s: %w", organisationID, err)
	}
	return nil
}

// DeleteAllServerCookbookComplexities removes all server cookbook complexity
// records.
func (db *DB) DeleteAllServerCookbookComplexities(ctx context.Context) error {
	const query = `DELETE FROM server_cookbook_complexity`
	_, err := db.pool.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("datastore: deleting all server cookbook complexities: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanServerCookbookComplexity(row interface{ Scan(dest ...any) error }) (ServerCookbookComplexity, error) {
	var r ServerCookbookComplexity

	err := row.Scan(
		&r.ID,
		&r.ServerCookbookID,
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
		return ServerCookbookComplexity{}, err
	}

	return r, nil
}

func (db *DB) scanServerCookbookComplexities(ctx context.Context, query string, args ...any) ([]ServerCookbookComplexity, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing server cookbook complexities: %w", err)
	}
	defer rows.Close()

	var results []ServerCookbookComplexity
	for rows.Next() {
		var r ServerCookbookComplexity

		if err := rows.Scan(
			&r.ID,
			&r.ServerCookbookID,
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
			return nil, fmt.Errorf("datastore: scanning server cookbook complexity row: %w", err)
		}

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating server cookbook complexities: %w", err)
	}
	return results, nil
}
