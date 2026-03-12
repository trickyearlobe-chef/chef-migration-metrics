// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// OwnershipAssignment represents a row in the ownership_assignments table.
type OwnershipAssignment struct {
	ID               string    `json:"id"`
	OwnerID          string    `json:"owner_id"`
	EntityType       string    `json:"entity_type"`
	EntityKey        string    `json:"entity_key"`
	OrganisationID   string    `json:"organisation_id,omitempty"`
	AssignmentSource string    `json:"assignment_source"`
	AutoRuleName     string    `json:"auto_rule_name,omitempty"`
	Confidence       string    `json:"confidence"`
	Notes            string    `json:"notes,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// InsertAssignmentParams holds the fields required to create a new ownership
// assignment.
type InsertAssignmentParams struct {
	OwnerID          string
	EntityType       string
	EntityKey        string
	OrganisationID   string // empty = NULL (cross-org)
	AssignmentSource string // "manual", "auto_rule", "import"
	AutoRuleName     string // only for auto_rule
	Confidence       string // "definitive" or "inferred"
	Notes            string
}

// AssignmentListFilter holds the query parameters for listing assignments.
type AssignmentListFilter struct {
	OwnerName        string
	EntityType       string
	OrganisationName string
	AssignmentSource string
	Limit            int
	Offset           int
}

// InsertAssignment creates a new ownership assignment. Returns
// ErrAlreadyExists if a duplicate assignment exists.
func (db *DB) InsertAssignment(ctx context.Context, p InsertAssignmentParams) (OwnershipAssignment, error) {
	return db.insertAssignment(ctx, db.q(), p)
}

func (db *DB) insertAssignment(ctx context.Context, q queryable, p InsertAssignmentParams) (OwnershipAssignment, error) {
	const query = `
		INSERT INTO ownership_assignments
			(owner_id, entity_type, entity_key, organisation_id, assignment_source, auto_rule_name, confidence, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, owner_id, entity_type, entity_key, organisation_id,
		          assignment_source, auto_rule_name, confidence, notes,
		          created_at, updated_at
	`

	a, err := scanAssignment(q.QueryRowContext(ctx, query,
		p.OwnerID,
		p.EntityType,
		p.EntityKey,
		nullStringPtr(p.OrganisationID),
		p.AssignmentSource,
		nullString(p.AutoRuleName),
		p.Confidence,
		nullString(p.Notes),
	))
	if err != nil {
		if isUniqueViolation(err) {
			return OwnershipAssignment{}, ErrAlreadyExists
		}
		return OwnershipAssignment{}, fmt.Errorf("datastore: inserting assignment: %w", err)
	}
	return a, nil
}

// ListAssignmentsByOwner returns assignments for the given owner name, with
// optional filters.
func (db *DB) ListAssignmentsByOwner(ctx context.Context, f AssignmentListFilter) ([]OwnershipAssignment, int, error) {
	return db.listAssignmentsByOwner(ctx, db.q(), f)
}

func (db *DB) listAssignmentsByOwner(ctx context.Context, q queryable, f AssignmentListFilter) ([]OwnershipAssignment, int, error) {
	where := "WHERE o.name = $1"
	args := []any{f.OwnerName}
	argN := 2

	if f.EntityType != "" {
		where += fmt.Sprintf(" AND oa.entity_type = $%d", argN)
		args = append(args, f.EntityType)
		argN++
	}
	if f.OrganisationName != "" {
		where += fmt.Sprintf(" AND org.name = $%d", argN)
		args = append(args, f.OrganisationName)
		argN++
	}
	if f.AssignmentSource != "" {
		where += fmt.Sprintf(" AND oa.assignment_source = $%d", argN)
		args = append(args, f.AssignmentSource)
		argN++
	}

	fromClause := `
		FROM ownership_assignments oa
		JOIN owners o ON o.id = oa.owner_id
		LEFT JOIN organisations org ON org.id = oa.organisation_id
	`

	// Count total.
	countQuery := "SELECT COUNT(*) " + fromClause + where
	var total int
	if err := q.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("datastore: counting assignments: %w", err)
	}

	// Fetch page.
	limit := f.Limit
	if limit <= 0 {
		limit = 25
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	dataQuery := fmt.Sprintf(`
		SELECT oa.id, oa.owner_id, oa.entity_type, oa.entity_key, oa.organisation_id,
		       oa.assignment_source, oa.auto_rule_name, oa.confidence, oa.notes,
		       oa.created_at, oa.updated_at
		%s
		%s
		ORDER BY oa.created_at DESC
		LIMIT $%d OFFSET $%d
	`, fromClause, where, argN, argN+1)
	args = append(args, limit, offset)

	rows, err := q.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: listing assignments: %w", err)
	}
	defer rows.Close()

	assignments, err := scanAssignments(rows)
	if err != nil {
		return nil, 0, err
	}
	return assignments, total, nil
}

// GetAssignment returns a single assignment by ID. Returns ErrNotFound if
// not found.
func (db *DB) GetAssignment(ctx context.Context, id string) (OwnershipAssignment, error) {
	return db.getAssignment(ctx, db.q(), id)
}

func (db *DB) getAssignment(ctx context.Context, q queryable, id string) (OwnershipAssignment, error) {
	const query = `
		SELECT id, owner_id, entity_type, entity_key, organisation_id,
		       assignment_source, auto_rule_name, confidence, notes,
		       created_at, updated_at
		FROM ownership_assignments
		WHERE id = $1
	`
	return scanAssignment(q.QueryRowContext(ctx, query, id))
}

// DeleteAssignment removes an assignment by ID. Returns ErrNotFound if no
// such assignment exists.
func (db *DB) DeleteAssignment(ctx context.Context, id string) error {
	return db.deleteAssignment(ctx, db.q(), id)
}

func (db *DB) deleteAssignment(ctx context.Context, q queryable, id string) error {
	res, err := q.ExecContext(ctx, `DELETE FROM ownership_assignments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting assignment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReassignOwnership moves all (or filtered) assignments from one owner to
// another. Returns the count of reassigned and skipped (duplicate)
// assignments.
func (db *DB) ReassignOwnership(ctx context.Context, fromOwnerID, toOwnerID string, entityType, organisationID string) (reassigned, skipped int, err error) {
	err = db.Tx(ctx, func(tx *sql.Tx) error {
		// Build filter clause.
		where := "WHERE owner_id = $1"
		args := []any{fromOwnerID}
		argN := 2

		if entityType != "" {
			where += fmt.Sprintf(" AND entity_type = $%d", argN)
			args = append(args, entityType)
			argN++
		}
		if organisationID != "" {
			where += fmt.Sprintf(" AND organisation_id = $%d", argN)
			args = append(args, organisationID)
			argN++
		}

		// Fetch matching assignments from the source owner.
		selectQuery := fmt.Sprintf(`
			SELECT id, entity_type, entity_key, organisation_id, notes
			FROM ownership_assignments
			%s
		`, where)

		rows, err := tx.QueryContext(ctx, selectQuery, args...)
		if err != nil {
			return fmt.Errorf("listing assignments for reassignment: %w", err)
		}
		defer rows.Close()

		type assignmentInfo struct {
			id             string
			entityType     string
			entityKey      string
			organisationID sql.NullString
			notes          sql.NullString
		}
		var toMove []assignmentInfo
		for rows.Next() {
			var a assignmentInfo
			if err := rows.Scan(&a.id, &a.entityType, &a.entityKey, &a.organisationID, &a.notes); err != nil {
				return fmt.Errorf("scanning assignment for reassignment: %w", err)
			}
			toMove = append(toMove, a)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterating assignments for reassignment: %w", err)
		}

		for _, a := range toMove {
			// Check if target owner already has this assignment.
			var exists bool
			err := tx.QueryRowContext(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM ownership_assignments
					WHERE owner_id = $1 AND entity_type = $2 AND entity_key = $3
					AND COALESCE(organisation_id, '00000000-0000-0000-0000-000000000000') =
					    COALESCE($4, '00000000-0000-0000-0000-000000000000')
				)
			`, toOwnerID, a.entityType, a.entityKey, a.organisationID).Scan(&exists)
			if err != nil {
				return fmt.Errorf("checking duplicate assignment: %w", err)
			}

			if exists {
				// Delete the source assignment (duplicate).
				if _, err := tx.ExecContext(ctx, `DELETE FROM ownership_assignments WHERE id = $1`, a.id); err != nil {
					return fmt.Errorf("deleting duplicate assignment: %w", err)
				}
				skipped++
			} else {
				// Move the assignment to the target owner.
				_, err := tx.ExecContext(ctx, `
					UPDATE ownership_assignments
					SET owner_id = $1,
					    assignment_source = 'manual',
					    confidence = 'definitive',
					    auto_rule_name = NULL,
					    updated_at = now()
					WHERE id = $2
				`, toOwnerID, a.id)
				if err != nil {
					return fmt.Errorf("reassigning assignment: %w", err)
				}
				reassigned++
			}
		}

		return nil
	})
	return reassigned, skipped, err
}

// LookupOwnership returns all owners for a given entity, using the
// resolution precedence from the specification: direct assignments first,
// then inherited (git_repo for cookbooks, policy for nodes).
func (db *DB) LookupOwnership(ctx context.Context, entityType, entityKey, organisationID string) ([]OwnershipLookupResult, error) {
	return db.lookupOwnership(ctx, db.q(), entityType, entityKey, organisationID)
}

// OwnershipLookupResult is a single resolved owner for an entity.
type OwnershipLookupResult struct {
	OwnerName        string         `json:"name"`
	DisplayName      string         `json:"display_name,omitempty"`
	AssignmentSource string         `json:"assignment_source"`
	Confidence       string         `json:"confidence"`
	Resolution       string         `json:"resolution"` // "direct", "git_repo_inherited", "policy_inherited"
	InheritedFrom    *InheritedFrom `json:"inherited_from,omitempty"`
}

// InheritedFrom describes the entity from which ownership was inherited.
type InheritedFrom struct {
	EntityType string `json:"entity_type"`
	EntityKey  string `json:"entity_key"`
}

func (db *DB) lookupOwnership(ctx context.Context, q queryable, entityType, entityKey, organisationID string) ([]OwnershipLookupResult, error) {
	var results []OwnershipLookupResult

	// 1. Direct assignments.
	directQuery := `
		SELECT o.name, o.display_name, oa.assignment_source, oa.confidence
		FROM ownership_assignments oa
		JOIN owners o ON o.id = oa.owner_id
		WHERE oa.entity_type = $1 AND oa.entity_key = $2
		AND (oa.organisation_id IS NULL OR oa.organisation_id = $3 OR $3 = '')
		ORDER BY
			CASE oa.assignment_source
				WHEN 'manual' THEN 1
				WHEN 'import' THEN 2
				WHEN 'auto_rule' THEN 3
			END
	`
	rows, err := q.QueryContext(ctx, directQuery, entityType, entityKey, nullStringPtr(organisationID))
	if err != nil {
		return nil, fmt.Errorf("datastore: looking up direct ownership: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r OwnershipLookupResult
		var displayName sql.NullString
		if err := rows.Scan(&r.OwnerName, &displayName, &r.AssignmentSource, &r.Confidence); err != nil {
			return nil, fmt.Errorf("datastore: scanning ownership lookup: %w", err)
		}
		r.DisplayName = stringFromNull(displayName)
		r.Resolution = "direct"
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating ownership lookup rows: %w", err)
	}

	// 2. Git-repo-inherited (cookbooks only) — if no direct owners found.
	if entityType == "cookbook" && len(results) == 0 {
		inherited, err := db.lookupGitRepoInheritedOwnership(ctx, q, entityKey, organisationID)
		if err != nil {
			return nil, err
		}
		results = append(results, inherited...)
	}

	// 3. Policy-inherited (nodes only) — if no direct owners found.
	if entityType == "node" && len(results) == 0 {
		inherited, err := db.lookupPolicyInheritedOwnership(ctx, q, entityKey, organisationID)
		if err != nil {
			return nil, err
		}
		results = append(results, inherited...)
	}

	return results, nil
}

func (db *DB) lookupGitRepoInheritedOwnership(ctx context.Context, q queryable, cookbookName, organisationID string) ([]OwnershipLookupResult, error) {
	// Find the git repo URL for this cookbook.
	var gitRepoURL sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT git_repo_url FROM cookbooks
		WHERE name = $1 AND source = 'git' AND git_repo_url IS NOT NULL
		LIMIT 1
	`, cookbookName).Scan(&gitRepoURL)
	if err != nil || !gitRepoURL.Valid {
		return nil, nil // not git-sourced or doesn't exist
	}

	// Look up git_repo ownership.
	query := `
		SELECT o.name, o.display_name, oa.assignment_source, oa.confidence
		FROM ownership_assignments oa
		JOIN owners o ON o.id = oa.owner_id
		WHERE oa.entity_type = 'git_repo' AND oa.entity_key = $1
		AND (oa.organisation_id IS NULL OR oa.organisation_id = $2 OR $2 = '')
		ORDER BY
			CASE oa.assignment_source
				WHEN 'manual' THEN 1
				WHEN 'import' THEN 2
				WHEN 'auto_rule' THEN 3
			END
	`
	rows, err := q.QueryContext(ctx, query, gitRepoURL.String, nullStringPtr(organisationID))
	if err != nil {
		return nil, fmt.Errorf("datastore: looking up git_repo inherited ownership: %w", err)
	}
	defer rows.Close()

	var results []OwnershipLookupResult
	for rows.Next() {
		var r OwnershipLookupResult
		var displayName sql.NullString
		if err := rows.Scan(&r.OwnerName, &displayName, &r.AssignmentSource, &r.Confidence); err != nil {
			return nil, fmt.Errorf("datastore: scanning git_repo inherited: %w", err)
		}
		r.DisplayName = stringFromNull(displayName)
		r.Resolution = "git_repo_inherited"
		r.InheritedFrom = &InheritedFrom{
			EntityType: "git_repo",
			EntityKey:  gitRepoURL.String,
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (db *DB) lookupPolicyInheritedOwnership(ctx context.Context, q queryable, nodeName, organisationID string) ([]OwnershipLookupResult, error) {
	// Find the policy name for this node.
	var policyName sql.NullString
	query := `SELECT policy_name FROM node_snapshots WHERE node_name = $1`
	args := []any{nodeName}
	if organisationID != "" {
		query += " AND organisation_id = $2"
		args = append(args, organisationID)
	}
	query += " ORDER BY collected_at DESC LIMIT 1"

	err := q.QueryRowContext(ctx, query, args...).Scan(&policyName)
	if err != nil || !policyName.Valid || policyName.String == "" {
		return nil, nil // no policy
	}

	// Look up policy ownership.
	lookupQuery := `
		SELECT o.name, o.display_name, oa.assignment_source, oa.confidence
		FROM ownership_assignments oa
		JOIN owners o ON o.id = oa.owner_id
		WHERE oa.entity_type = 'policy' AND oa.entity_key = $1
		AND (oa.organisation_id IS NULL OR oa.organisation_id = $2 OR $2 = '')
		ORDER BY
			CASE oa.assignment_source
				WHEN 'manual' THEN 1
				WHEN 'import' THEN 2
				WHEN 'auto_rule' THEN 3
			END
	`
	rows, err := q.QueryContext(ctx, lookupQuery, policyName.String, nullStringPtr(organisationID))
	if err != nil {
		return nil, fmt.Errorf("datastore: looking up policy inherited ownership: %w", err)
	}
	defer rows.Close()

	var results []OwnershipLookupResult
	for rows.Next() {
		var r OwnershipLookupResult
		var displayName sql.NullString
		if err := rows.Scan(&r.OwnerName, &displayName, &r.AssignmentSource, &r.Confidence); err != nil {
			return nil, fmt.Errorf("datastore: scanning policy inherited: %w", err)
		}
		r.DisplayName = stringFromNull(displayName)
		r.Resolution = "policy_inherited"
		r.InheritedFrom = &InheritedFrom{
			EntityType: "policy",
			EntityKey:  policyName.String,
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ListAutoRuleAssignments returns all ownership assignments created by the
// given auto_rule_name, optionally scoped to an organisation. This is used
// by the ownership evaluator to detect stale assignments that should be
// removed after re-evaluation.
func (db *DB) ListAutoRuleAssignments(ctx context.Context, autoRuleName, organisationID string) ([]OwnershipAssignment, error) {
	return db.listAutoRuleAssignments(ctx, db.q(), autoRuleName, organisationID)
}

func (db *DB) listAutoRuleAssignments(ctx context.Context, q queryable, autoRuleName, organisationID string) ([]OwnershipAssignment, error) {
	where := "WHERE assignment_source = 'auto_rule' AND auto_rule_name = $1"
	args := []any{autoRuleName}
	argN := 2

	if organisationID != "" {
		where += fmt.Sprintf(" AND organisation_id = $%d", argN)
		args = append(args, organisationID)
		argN++
	}

	query := fmt.Sprintf(`
		SELECT id, owner_id, entity_type, entity_key, organisation_id,
		       assignment_source, auto_rule_name, confidence, notes,
		       created_at, updated_at
		FROM ownership_assignments
		%s
		ORDER BY entity_type, entity_key
	`, where)

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing auto-rule assignments for %q: %w", autoRuleName, err)
	}
	defer rows.Close()
	return scanAssignments(rows)
}

// DeleteStaleAutoRuleAssignments removes auto_rule assignments for the given
// rule name (and optional org) that are NOT in the currentMatchKeys set. Each
// key in currentMatchKeys is "entity_type:entity_key". Returns the number of
// deleted rows.
func (db *DB) DeleteStaleAutoRuleAssignments(ctx context.Context, autoRuleName, organisationID string, currentMatchKeys map[string]bool) (int, error) {
	existing, err := db.ListAutoRuleAssignments(ctx, autoRuleName, organisationID)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, a := range existing {
		key := a.EntityType + ":" + a.EntityKey
		if !currentMatchKeys[key] {
			if err := db.DeleteAssignment(ctx, a.ID); err != nil {
				return deleted, fmt.Errorf("datastore: deleting stale auto-rule assignment %s: %w", a.ID, err)
			}
			deleted++
		}
	}
	return deleted, nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanAssignment(row *sql.Row) (OwnershipAssignment, error) {
	var a OwnershipAssignment
	var orgID, autoRuleName, notes sql.NullString

	err := row.Scan(
		&a.ID,
		&a.OwnerID,
		&a.EntityType,
		&a.EntityKey,
		&orgID,
		&a.AssignmentSource,
		&autoRuleName,
		&a.Confidence,
		&notes,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return OwnershipAssignment{}, ErrNotFound
		}
		return OwnershipAssignment{}, fmt.Errorf("datastore: scanning assignment: %w", err)
	}

	a.OrganisationID = stringFromNull(orgID)
	a.AutoRuleName = stringFromNull(autoRuleName)
	a.Notes = stringFromNull(notes)
	return a, nil
}

func scanAssignments(rows *sql.Rows) ([]OwnershipAssignment, error) {
	var assignments []OwnershipAssignment
	for rows.Next() {
		var a OwnershipAssignment
		var orgID, autoRuleName, notes sql.NullString

		if err := rows.Scan(
			&a.ID,
			&a.OwnerID,
			&a.EntityType,
			&a.EntityKey,
			&orgID,
			&a.AssignmentSource,
			&autoRuleName,
			&a.Confidence,
			&notes,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning assignment row: %w", err)
		}

		a.OrganisationID = stringFromNull(orgID)
		a.AutoRuleName = stringFromNull(autoRuleName)
		a.Notes = stringFromNull(notes)
		assignments = append(assignments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating assignment rows: %w", err)
	}
	return assignments, nil
}

// nullStringPtr returns a *string suitable for passing as a nullable UUID
// parameter. Empty strings are treated as nil.
func nullStringPtr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
