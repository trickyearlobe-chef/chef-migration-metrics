// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// RoleDependency represents a row in the role_dependencies table. Each row
// records that a role depends on either another role or a cookbook.
type RoleDependency struct {
	ID             string    `json:"id"`
	OrganisationID string    `json:"organisation_id"`
	RoleName       string    `json:"role_name"`
	DependencyType string    `json:"dependency_type"` // "role" or "cookbook"
	DependencyName string    `json:"dependency_name"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// MarshalJSON implements json.Marshaler for RoleDependency.
func (rd RoleDependency) MarshalJSON() ([]byte, error) {
	type Alias RoleDependency
	return json.Marshal((Alias)(rd))
}

// ---------------------------------------------------------------------------
// Insert / Upsert
// ---------------------------------------------------------------------------

// InsertRoleDependencyParams holds the fields required to insert a single
// role dependency record.
type InsertRoleDependencyParams struct {
	OrganisationID string
	RoleName       string
	DependencyType string // "role" or "cookbook"
	DependencyName string
}

// InsertRoleDependency inserts a single role dependency and returns the
// created row.
func (db *DB) InsertRoleDependency(ctx context.Context, p InsertRoleDependencyParams) (RoleDependency, error) {
	return db.insertRoleDependency(ctx, db.q(), p)
}

func (db *DB) insertRoleDependency(ctx context.Context, q queryable, p InsertRoleDependencyParams) (RoleDependency, error) {
	if err := validateRoleDependencyParams(p); err != nil {
		return RoleDependency{}, err
	}

	const query = `
		INSERT INTO role_dependencies (organisation_id, role_name, dependency_type, dependency_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT ON CONSTRAINT uq_role_dependencies DO UPDATE
			SET updated_at = now()
		RETURNING id, organisation_id, role_name, dependency_type, dependency_name, created_at, updated_at
	`

	return scanRoleDependency(q.QueryRowContext(ctx, query,
		p.OrganisationID,
		p.RoleName,
		p.DependencyType,
		p.DependencyName,
	))
}

// ---------------------------------------------------------------------------
// Bulk upsert
// ---------------------------------------------------------------------------

// BulkUpsertRoleDependencies upserts multiple role dependency records within
// a single transaction. Existing rows (matched by the unique constraint on
// organisation_id, role_name, dependency_type, dependency_name) have their
// updated_at timestamp refreshed. Returns the count of rows upserted.
func (db *DB) BulkUpsertRoleDependencies(ctx context.Context, params []InsertRoleDependencyParams) (int, error) {
	if len(params) == 0 {
		return 0, nil
	}

	upserted := 0
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		const query = `
			INSERT INTO role_dependencies (organisation_id, role_name, dependency_type, dependency_name)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT ON CONSTRAINT uq_role_dependencies DO UPDATE
				SET updated_at = now()
		`

		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return fmt.Errorf("datastore: preparing role dependency upsert: %w", err)
		}
		defer stmt.Close()

		for i, p := range params {
			if err := validateRoleDependencyParams(p); err != nil {
				return fmt.Errorf("row %d: %w", i, err)
			}

			_, err := stmt.ExecContext(ctx,
				p.OrganisationID,
				p.RoleName,
				p.DependencyType,
				p.DependencyName,
			)
			if err != nil {
				return fmt.Errorf("datastore: upserting role dependency (row %d): %w", i, err)
			}
			upserted++
		}

		return nil
	})
	if err != nil {
		return 0, err
	}
	return upserted, nil
}

// ---------------------------------------------------------------------------
// Replace all dependencies for an organisation
// ---------------------------------------------------------------------------

// ReplaceRoleDependenciesForOrg atomically replaces all role dependencies
// for the given organisation. This is the preferred approach when refreshing
// the dependency graph on each collection run — it deletes stale edges that
// no longer exist in the Chef server's role definitions.
//
// The operation runs within a single transaction: all existing rows for the
// organisation are deleted, then the new set is bulk-inserted. Returns the
// count of rows inserted.
func (db *DB) ReplaceRoleDependenciesForOrg(ctx context.Context, organisationID string, params []InsertRoleDependencyParams) (int, error) {
	if organisationID == "" {
		return 0, fmt.Errorf("datastore: organisation ID is required for role dependency replacement")
	}

	inserted := 0
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		// Delete all existing dependencies for this organisation.
		_, err := tx.ExecContext(ctx,
			`DELETE FROM role_dependencies WHERE organisation_id = $1`,
			organisationID,
		)
		if err != nil {
			return fmt.Errorf("datastore: deleting role dependencies for org: %w", err)
		}

		if len(params) == 0 {
			return nil
		}

		const insertQuery = `
			INSERT INTO role_dependencies (organisation_id, role_name, dependency_type, dependency_name)
			VALUES ($1, $2, $3, $4)
		`

		stmt, err := tx.PrepareContext(ctx, insertQuery)
		if err != nil {
			return fmt.Errorf("datastore: preparing role dependency insert: %w", err)
		}
		defer stmt.Close()

		for i, p := range params {
			if err := validateRoleDependencyParams(p); err != nil {
				return fmt.Errorf("row %d: %w", i, err)
			}

			// Override organisation ID to ensure consistency.
			_, err := stmt.ExecContext(ctx,
				organisationID,
				p.RoleName,
				p.DependencyType,
				p.DependencyName,
			)
			if err != nil {
				return fmt.Errorf("datastore: inserting role dependency (row %d): %w", i, err)
			}
			inserted++
		}

		return nil
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteRoleDependenciesByOrg removes all role dependency records for the
// given organisation. Returns the number of rows deleted.
func (db *DB) DeleteRoleDependenciesByOrg(ctx context.Context, organisationID string) (int, error) {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM role_dependencies WHERE organisation_id = $1`,
		organisationID,
	)
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting role dependencies by org: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	return int(n), nil
}

// DeleteRoleDependenciesByRole removes all dependency records for a specific
// role within an organisation. Returns the number of rows deleted.
func (db *DB) DeleteRoleDependenciesByRole(ctx context.Context, organisationID, roleName string) (int, error) {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM role_dependencies WHERE organisation_id = $1 AND role_name = $2`,
		organisationID,
		roleName,
	)
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting role dependencies by role: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	return int(n), nil
}

// ---------------------------------------------------------------------------
// Query methods
// ---------------------------------------------------------------------------

// ListRoleDependenciesByOrg returns all role dependency records for the
// given organisation, ordered by role_name then dependency_type then
// dependency_name.
func (db *DB) ListRoleDependenciesByOrg(ctx context.Context, organisationID string) ([]RoleDependency, error) {
	return db.listRoleDependenciesByOrg(ctx, db.q(), organisationID)
}

func (db *DB) listRoleDependenciesByOrg(ctx context.Context, q queryable, organisationID string) ([]RoleDependency, error) {
	const query = `
		SELECT id, organisation_id, role_name, dependency_type, dependency_name, created_at, updated_at
		FROM role_dependencies
		WHERE organisation_id = $1
		ORDER BY role_name, dependency_type, dependency_name
	`
	return scanRoleDependencies(q.QueryContext(ctx, query, organisationID))
}

// ListRoleDependenciesByRole returns all dependency records for a specific
// role within an organisation, ordered by dependency_type then
// dependency_name.
func (db *DB) ListRoleDependenciesByRole(ctx context.Context, organisationID, roleName string) ([]RoleDependency, error) {
	return db.listRoleDependenciesByRole(ctx, db.q(), organisationID, roleName)
}

func (db *DB) listRoleDependenciesByRole(ctx context.Context, q queryable, organisationID, roleName string) ([]RoleDependency, error) {
	const query = `
		SELECT id, organisation_id, role_name, dependency_type, dependency_name, created_at, updated_at
		FROM role_dependencies
		WHERE organisation_id = $1 AND role_name = $2
		ORDER BY dependency_type, dependency_name
	`
	return scanRoleDependencies(q.QueryContext(ctx, query, organisationID, roleName))
}

// ListRoleDependenciesByType returns all dependency records of a specific
// type (either "role" or "cookbook") for the given organisation.
func (db *DB) ListRoleDependenciesByType(ctx context.Context, organisationID, dependencyType string) ([]RoleDependency, error) {
	return db.listRoleDependenciesByType(ctx, db.q(), organisationID, dependencyType)
}

func (db *DB) listRoleDependenciesByType(ctx context.Context, q queryable, organisationID, dependencyType string) ([]RoleDependency, error) {
	const query = `
		SELECT id, organisation_id, role_name, dependency_type, dependency_name, created_at, updated_at
		FROM role_dependencies
		WHERE organisation_id = $1 AND dependency_type = $2
		ORDER BY role_name, dependency_name
	`
	return scanRoleDependencies(q.QueryContext(ctx, query, organisationID, dependencyType))
}

// ListRolesDependingOnCookbook returns all role names that depend on the
// given cookbook name within the given organisation.
func (db *DB) ListRolesDependingOnCookbook(ctx context.Context, organisationID, cookbookName string) ([]string, error) {
	const query = `
		SELECT DISTINCT role_name
		FROM role_dependencies
		WHERE organisation_id = $1 AND dependency_type = 'cookbook' AND dependency_name = $2
		ORDER BY role_name
	`

	rows, err := db.pool.QueryContext(ctx, query, organisationID, cookbookName)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing roles depending on cookbook: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("datastore: scanning role name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating role names: %w", err)
	}
	return names, nil
}

// ListRolesDependingOnRole returns all role names that depend on the given
// role (i.e. roles that include role[target] in their run_list) within the
// given organisation.
func (db *DB) ListRolesDependingOnRole(ctx context.Context, organisationID, targetRoleName string) ([]string, error) {
	const query = `
		SELECT DISTINCT role_name
		FROM role_dependencies
		WHERE organisation_id = $1 AND dependency_type = 'role' AND dependency_name = $2
		ORDER BY role_name
	`

	rows, err := db.pool.QueryContext(ctx, query, organisationID, targetRoleName)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing roles depending on role: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("datastore: scanning role name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating role names: %w", err)
	}
	return names, nil
}

// ---------------------------------------------------------------------------
// Aggregation queries
// ---------------------------------------------------------------------------

// RoleDependencyCount holds the result of a count-by-role aggregation.
type RoleDependencyCount struct {
	RoleName        string `json:"role_name"`
	CookbookCount   int    `json:"cookbook_count"`
	RoleCount       int    `json:"role_count"`
	TotalDependency int    `json:"total_dependency_count"`
}

// CountDependenciesByRole returns the number of cookbook and role dependencies
// for each role in the given organisation, ordered by total dependency count
// descending.
func (db *DB) CountDependenciesByRole(ctx context.Context, organisationID string) ([]RoleDependencyCount, error) {
	const query = `
		SELECT
			role_name,
			COUNT(*) FILTER (WHERE dependency_type = 'cookbook') AS cookbook_count,
			COUNT(*) FILTER (WHERE dependency_type = 'role') AS role_count,
			COUNT(*) AS total_dependency_count
		FROM role_dependencies
		WHERE organisation_id = $1
		GROUP BY role_name
		ORDER BY total_dependency_count DESC, role_name
	`

	rows, err := db.pool.QueryContext(ctx, query, organisationID)
	if err != nil {
		return nil, fmt.Errorf("datastore: counting dependencies by role: %w", err)
	}
	defer rows.Close()

	var counts []RoleDependencyCount
	for rows.Next() {
		var c RoleDependencyCount
		if err := rows.Scan(&c.RoleName, &c.CookbookCount, &c.RoleCount, &c.TotalDependency); err != nil {
			return nil, fmt.Errorf("datastore: scanning role dependency count: %w", err)
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating role dependency counts: %w", err)
	}
	return counts, nil
}

// CountRolesPerCookbook returns the number of roles that reference each
// cookbook within the given organisation, ordered by role count descending.
type CookbookRoleCount struct {
	CookbookName string `json:"cookbook_name"`
	RoleCount    int    `json:"role_count"`
}

func (db *DB) CountRolesPerCookbook(ctx context.Context, organisationID string) ([]CookbookRoleCount, error) {
	const query = `
		SELECT dependency_name AS cookbook_name, COUNT(DISTINCT role_name) AS role_count
		FROM role_dependencies
		WHERE organisation_id = $1 AND dependency_type = 'cookbook'
		GROUP BY dependency_name
		ORDER BY role_count DESC, dependency_name
	`

	rows, err := db.pool.QueryContext(ctx, query, organisationID)
	if err != nil {
		return nil, fmt.Errorf("datastore: counting roles per cookbook: %w", err)
	}
	defer rows.Close()

	var counts []CookbookRoleCount
	for rows.Next() {
		var c CookbookRoleCount
		if err := rows.Scan(&c.CookbookName, &c.RoleCount); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookbook role count: %w", err)
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookbook role counts: %w", err)
	}
	return counts, nil
}

// ---------------------------------------------------------------------------
// Validation helper
// ---------------------------------------------------------------------------

func validateRoleDependencyParams(p InsertRoleDependencyParams) error {
	if p.OrganisationID == "" {
		return fmt.Errorf("datastore: organisation ID is required for role dependency")
	}
	if p.RoleName == "" {
		return fmt.Errorf("datastore: role name is required for role dependency")
	}
	if p.DependencyType != "role" && p.DependencyType != "cookbook" {
		return fmt.Errorf("datastore: dependency type must be 'role' or 'cookbook', got %q", p.DependencyType)
	}
	if p.DependencyName == "" {
		return fmt.Errorf("datastore: dependency name is required for role dependency")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanRoleDependency(row *sql.Row) (RoleDependency, error) {
	var rd RoleDependency
	err := row.Scan(
		&rd.ID,
		&rd.OrganisationID,
		&rd.RoleName,
		&rd.DependencyType,
		&rd.DependencyName,
		&rd.CreatedAt,
		&rd.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return RoleDependency{}, ErrNotFound
		}
		return RoleDependency{}, fmt.Errorf("datastore: scanning role dependency: %w", err)
	}
	return rd, nil
}

func scanRoleDependencies(rows *sql.Rows, err error) ([]RoleDependency, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying role dependencies: %w", err)
	}
	defer rows.Close()

	var deps []RoleDependency
	for rows.Next() {
		var rd RoleDependency
		if err := rows.Scan(
			&rd.ID,
			&rd.OrganisationID,
			&rd.RoleName,
			&rd.DependencyType,
			&rd.DependencyName,
			&rd.CreatedAt,
			&rd.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning role dependency row: %w", err)
		}
		deps = append(deps, rd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating role dependency rows: %w", err)
	}
	return deps, nil
}
