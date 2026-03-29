// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ServerCookbookComplexity represents a row in the server_cookbook_complexity table.
type ServerCookbookComplexity struct {
	OrganisationName     string
	CookbookName         string
	CookbookVersion      string
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
// (organisation_name, cookbook_name, cookbook_version, target_chef_version).
type UpsertServerCookbookComplexityParams struct {
	OrganisationName     string
	CookbookName         string
	CookbookVersion      string
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

const sccColumns = `organisation_name, cookbook_name, cookbook_version,
       target_chef_version,
       complexity_score, complexity_label,
       error_count, deprecation_count, correctness_count, modernize_count,
       auto_correctable_count, manual_fix_count,
       affected_node_count, affected_role_count, affected_policy_count,
       evaluated_at, created_at, updated_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetServerCookbookComplexity returns the complexity record for the given
// organisation, cookbook, and target Chef version. Returns (nil, nil) if no
// record exists.
func (db *DB) GetServerCookbookComplexity(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*ServerCookbookComplexity, error) {
	return db.getServerCookbookComplexity(ctx, db.q(), orgName, cookbookName, cookbookVersion, targetChefVersion)
}

func (db *DB) getServerCookbookComplexity(ctx context.Context, q queryable, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*ServerCookbookComplexity, error) {
	query := `
		SELECT ` + sccColumns + `
		  FROM server_cookbook_complexity
		 WHERE organisation_name = $1
		   AND cookbook_name = $2
		   AND cookbook_version = $3
		   AND target_chef_version = $4
	`

	r, err := scanServerCookbookComplexity(q.QueryRowContext(ctx, query, orgName, cookbookName, cookbookVersion, targetChefVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting server cookbook complexity: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListServerCookbookComplexitiesByCookbook returns all complexity records for
// the given organisation, cookbook name, and cookbook version, ordered by
// target_chef_version.
func (db *DB) ListServerCookbookComplexitiesByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]ServerCookbookComplexity, error) {
	query := `
		SELECT ` + sccColumns + `
		  FROM server_cookbook_complexity
		 WHERE organisation_name = $1
		   AND cookbook_name = $2
		   AND cookbook_version = $3
		 ORDER BY target_chef_version
	`
	return db.scanServerCookbookComplexities(ctx, query, orgName, cookbookName, cookbookVersion)
}

// ListServerCookbookComplexitiesByOrganisation returns all complexity records
// for server cookbooks belonging to the given organisation, ordered by
// cookbook name, version, and target Chef version.
func (db *DB) ListServerCookbookComplexitiesByOrganisation(ctx context.Context, orgName string) ([]ServerCookbookComplexity, error) {
	query := `
		SELECT ` + sccColumns + `
		  FROM server_cookbook_complexity
		 WHERE organisation_name = $1
		 ORDER BY cookbook_name, cookbook_version, target_chef_version
	`
	return db.scanServerCookbookComplexities(ctx, query, orgName)
}

// ListServerCookbookComplexities returns all complexity records for server
// cookbooks belonging to the given organisation, filtered by the specified
// target Chef versions.
func (db *DB) ListServerCookbookComplexities(ctx context.Context, orgName string, targetChefVersions []string) ([]ServerCookbookComplexity, error) {
	if len(targetChefVersions) == 0 {
		return nil, nil
	}

	// $1 is orgName; $2, $3, ... are the target versions.
	placeholders := make([]string, len(targetChefVersions))
	args := make([]any, 0, 1+len(targetChefVersions))
	args = append(args, orgName)
	for i, v := range targetChefVersions {
		args = append(args, v)
		placeholders[i] = "$" + strconv.Itoa(i+2)
	}

	query := `
		SELECT ` + sccColumns + `
		  FROM server_cookbook_complexity
		 WHERE organisation_name = $1
		   AND target_chef_version IN (` + strings.Join(placeholders, ", ") + `)
	`
	return db.scanServerCookbookComplexities(ctx, query, args...)
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertServerCookbookComplexity inserts a new complexity record or updates
// the existing one for the same (organisation_name, cookbook_name,
// cookbook_version, target_chef_version) combination. Returns the resulting row.
func (db *DB) UpsertServerCookbookComplexity(ctx context.Context, p UpsertServerCookbookComplexityParams) (*ServerCookbookComplexity, error) {
	return db.upsertServerCookbookComplexity(ctx, db.q(), p)
}

func (db *DB) upsertServerCookbookComplexity(ctx context.Context, q queryable, p UpsertServerCookbookComplexityParams) (*ServerCookbookComplexity, error) {
	if p.OrganisationName == "" {
		return nil, fmt.Errorf("datastore: organisation_name is required")
	}
	if p.CookbookName == "" {
		return nil, fmt.Errorf("datastore: cookbook_name is required")
	}
	if p.TargetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target_chef_version is required")
	}

	query := `
		INSERT INTO server_cookbook_complexity (
			organisation_name, cookbook_name, cookbook_version,
			target_chef_version,
			complexity_score, complexity_label,
			error_count, deprecation_count, correctness_count, modernize_count,
			auto_correctable_count, manual_fix_count,
			affected_node_count, affected_role_count, affected_policy_count,
			evaluated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (organisation_name, cookbook_name, cookbook_version, target_chef_version)
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
		p.OrganisationName,
		p.CookbookName,
		p.CookbookVersion,
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
// for the given organisation, cookbook name, and cookbook version.
func (db *DB) DeleteServerCookbookComplexitiesByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) error {
	const query = `DELETE FROM server_cookbook_complexity WHERE organisation_name = $1 AND cookbook_name = $2 AND cookbook_version = $3`
	_, err := db.pool.ExecContext(ctx, query, orgName, cookbookName, cookbookVersion)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook complexities for cookbook %s/%s@%s: %w", orgName, cookbookName, cookbookVersion, err)
	}
	return nil
}

// DeleteServerCookbookComplexitiesByOrganisation removes all complexity
// records for server cookbooks belonging to the given organisation.
func (db *DB) DeleteServerCookbookComplexitiesByOrganisation(ctx context.Context, orgName string) error {
	const query = `DELETE FROM server_cookbook_complexity WHERE organisation_name = $1`
	_, err := db.pool.ExecContext(ctx, query, orgName)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook complexities for organisation %s: %w", orgName, err)
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
		&r.OrganisationName,
		&r.CookbookName,
		&r.CookbookVersion,
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
			&r.OrganisationName,
			&r.CookbookName,
			&r.CookbookVersion,
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
