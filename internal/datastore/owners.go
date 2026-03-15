// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Owner represents a row in the owners table.
type Owner struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	DisplayName    string          `json:"display_name,omitempty"`
	ContactEmail   string          `json:"contact_email,omitempty"`
	ContactChannel string          `json:"contact_channel,omitempty"`
	OwnerType      string          `json:"owner_type"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// InsertOwnerParams holds the fields required to create a new owner.
type InsertOwnerParams struct {
	Name           string
	DisplayName    string
	ContactEmail   string
	ContactChannel string
	OwnerType      string
	Metadata       json.RawMessage // nil means NULL
}

// UpdateOwnerParams holds the fields that can be updated on an owner. The
// Name field identifies the owner (it is immutable).
type UpdateOwnerParams struct {
	DisplayName    *string // nil means don't change
	ContactEmail   *string
	ContactChannel *string
	OwnerType      *string
	Metadata       *json.RawMessage // nil means don't change; pointer to nil means set NULL
}

// OwnerListFilter holds the query parameters for listing owners.
type OwnerListFilter struct {
	OwnerType string
	Search    string
	SortField string // allowed: "name", "owner_type", "created_at", "updated_at"
	SortDir   string // "asc" or "desc"
	Limit     int
	Offset    int
}

// InsertOwner creates a new owner. Returns ErrAlreadyExists if the name is
// taken.
func (db *DB) InsertOwner(ctx context.Context, p InsertOwnerParams) (Owner, error) {
	return db.insertOwner(ctx, db.q(), p)
}

func (db *DB) insertOwner(ctx context.Context, q queryable, p InsertOwnerParams) (Owner, error) {
	if p.Name == "" {
		return Owner{}, fmt.Errorf("datastore: owner name is required")
	}
	if p.OwnerType == "" {
		return Owner{}, fmt.Errorf("datastore: owner_type is required")
	}

	const query = `
		INSERT INTO owners (name, display_name, contact_email, contact_channel, owner_type, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, display_name, contact_email, contact_channel,
		          owner_type, metadata, created_at, updated_at
	`

	var metaBytes []byte
	if p.Metadata != nil {
		metaBytes = p.Metadata
	}

	owner, err := scanOwner(q.QueryRowContext(ctx, query,
		p.Name,
		nullString(p.DisplayName),
		nullString(p.ContactEmail),
		nullString(p.ContactChannel),
		p.OwnerType,
		metaBytes,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return Owner{}, ErrAlreadyExists
		}
		return Owner{}, fmt.Errorf("datastore: inserting owner %q: %w", p.Name, err)
	}
	return owner, nil
}

// GetOwnerByName returns the owner with the given name. Returns ErrNotFound
// if no such owner exists.
func (db *DB) GetOwnerByName(ctx context.Context, name string) (Owner, error) {
	return db.getOwnerByName(ctx, db.q(), name)
}

func (db *DB) getOwnerByName(ctx context.Context, q queryable, name string) (Owner, error) {
	const query = `
		SELECT id, name, display_name, contact_email, contact_channel,
		       owner_type, metadata, created_at, updated_at
		FROM owners
		WHERE name = $1
	`
	return scanOwner(q.QueryRowContext(ctx, query, name))
}

// GetOwner returns the owner with the given UUID. Returns ErrNotFound if no
// such owner exists.
func (db *DB) GetOwner(ctx context.Context, id string) (Owner, error) {
	return db.getOwner(ctx, db.q(), id)
}

func (db *DB) getOwner(ctx context.Context, q queryable, id string) (Owner, error) {
	const query = `
		SELECT id, name, display_name, contact_email, contact_channel,
		       owner_type, metadata, created_at, updated_at
		FROM owners
		WHERE id = $1
	`
	return scanOwner(q.QueryRowContext(ctx, query, id))
}

// ListOwners returns owners matching the given filter, ordered by name.
func (db *DB) ListOwners(ctx context.Context, f OwnerListFilter) ([]Owner, int, error) {
	return db.listOwners(ctx, db.q(), f)
}

func (db *DB) listOwners(ctx context.Context, q queryable, f OwnerListFilter) ([]Owner, int, error) {
	where := "WHERE 1=1"
	args := []any{}
	argN := 1

	if f.OwnerType != "" {
		where += fmt.Sprintf(" AND owner_type = $%d", argN)
		args = append(args, f.OwnerType)
		argN++
	}
	if f.Search != "" {
		where += fmt.Sprintf(" AND (name ILIKE $%d OR display_name ILIKE $%d)", argN, argN)
		args = append(args, "%"+f.Search+"%")
		argN++
	}

	// Count total matches.
	countQuery := "SELECT COUNT(*) FROM owners " + where
	var total int
	if err := q.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("datastore: counting owners: %w", err)
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

	// Determine sort order.
	orderBy := "name"
	switch f.SortField {
	case "name", "owner_type", "created_at", "updated_at":
		orderBy = f.SortField
	}
	dir := "ASC"
	if strings.EqualFold(f.SortDir, "desc") {
		dir = "DESC"
	}

	dataQuery := fmt.Sprintf(`
		SELECT id, name, display_name, contact_email, contact_channel,
		       owner_type, metadata, created_at, updated_at
		FROM owners
		%s
		ORDER BY %s %s, name ASC
		LIMIT $%d OFFSET $%d
	`, where, orderBy, dir, argN, argN+1)
	args = append(args, limit, offset)

	rows, err := q.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: listing owners: %w", err)
	}
	defer rows.Close()

	owners, err := scanOwners(rows)
	if err != nil {
		return nil, 0, err
	}

	return owners, total, nil
}

// OwnerWithSummary extends Owner with pre-computed assignment counts and
// readiness summary data. Used by the list endpoint to avoid N+1 queries.
type OwnerWithSummary struct {
	Owner
	NodeCount     int `json:"node_count"`
	CookbookCount int `json:"cookbook_count"`
	GitRepoCount  int `json:"git_repo_count"`
	RoleCount     int `json:"role_count"`
	PolicyCount   int `json:"policy_count"`
	// Readiness fields (zero when no target version or no node assignments).
	ReadyNodes   int `json:"ready_nodes"`
	BlockedNodes int `json:"blocked_nodes"`
	StaleNodes   int `json:"stale_nodes"`
	TotalNodes   int `json:"total_nodes"`
}

// ListOwnersWithSummary returns owners with assignment counts and readiness
// data computed in a single SQL query. The targetChefVersion is used to look
// up the latest node_readiness records for owned nodes; pass "" to skip
// readiness enrichment.
func (db *DB) ListOwnersWithSummary(ctx context.Context, f OwnerListFilter, targetChefVersion string) ([]OwnerWithSummary, int, error) {
	return db.listOwnersWithSummary(ctx, db.q(), f, targetChefVersion)
}

func (db *DB) listOwnersWithSummary(ctx context.Context, q queryable, f OwnerListFilter, targetChefVersion string) ([]OwnerWithSummary, int, error) {
	where := "WHERE 1=1"
	args := []any{}
	argN := 1

	if f.OwnerType != "" {
		where += fmt.Sprintf(" AND o.owner_type = $%d", argN)
		args = append(args, f.OwnerType)
		argN++
	}
	if f.Search != "" {
		where += fmt.Sprintf(" AND (o.name ILIKE $%d OR o.display_name ILIKE $%d)", argN, argN)
		args = append(args, "%"+f.Search+"%")
		argN++
	}

	// Count total matches.
	countQuery := "SELECT COUNT(*) FROM owners o " + where
	var total int
	if err := q.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("datastore: counting owners: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 25
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	// Determine sort order. We support sorting by owner columns AND by
	// computed columns (counts, readiness) — all done in SQL.
	orderBy := "o.name"
	switch f.SortField {
	case "name":
		orderBy = "o.name"
	case "owner_type":
		orderBy = "o.owner_type"
	case "created_at":
		orderBy = "o.created_at"
	case "updated_at":
		orderBy = "o.updated_at"
	case "nodes":
		orderBy = "node_count"
	case "cookbooks":
		orderBy = "cookbook_count"
	case "git_repos":
		orderBy = "git_repo_count"
	case "ready":
		orderBy = "ready_nodes"
	case "blocked":
		orderBy = "blocked_nodes"
	}
	dir := "ASC"
	if strings.EqualFold(f.SortDir, "desc") {
		dir = "DESC"
	}

	// Build the readiness CTE. When no target version is given we still
	// need the CTE to exist (as empty) so the LEFT JOIN works.
	readinessCTE := `
		readiness AS (
			SELECT 'none' AS entity_key, false AS is_ready, false AS stale_data
			WHERE false
		)`
	if targetChefVersion != "" {
		// Use a parameter placeholder for the target version.
		tvParam := fmt.Sprintf("$%d", argN)
		args = append(args, targetChefVersion)
		argN++
		readinessCTE = fmt.Sprintf(`
		readiness AS (
			SELECT nr.node_name AS entity_key, nr.is_ready, nr.stale_data
			FROM node_readiness nr
			WHERE nr.target_chef_version = %s
			  AND nr.id IN (
				SELECT DISTINCT ON (nr2.node_name) nr2.id
				FROM node_readiness nr2
				WHERE nr2.target_chef_version = %s
				ORDER BY nr2.node_name, nr2.evaluated_at DESC
			  )
		)`, tvParam, tvParam)
	}

	limitParam := fmt.Sprintf("$%d", argN)
	args = append(args, limit)
	argN++
	offsetParam := fmt.Sprintf("$%d", argN)
	args = append(args, offset)

	dataQuery := fmt.Sprintf(`
		WITH
		counts AS (
			SELECT
				oa.owner_id,
				COUNT(*) FILTER (WHERE oa.entity_type = 'node')     AS node_count,
				COUNT(*) FILTER (WHERE oa.entity_type = 'cookbook')  AS cookbook_count,
				COUNT(*) FILTER (WHERE oa.entity_type = 'git_repo') AS git_repo_count,
				COUNT(*) FILTER (WHERE oa.entity_type = 'role')     AS role_count,
				COUNT(*) FILTER (WHERE oa.entity_type = 'policy')   AS policy_count
			FROM ownership_assignments oa
			GROUP BY oa.owner_id
		),
		node_keys AS (
			SELECT oa.owner_id, oa.entity_key
			FROM ownership_assignments oa
			WHERE oa.entity_type = 'node'
		),
		%s,
		owner_readiness AS (
			SELECT
				nk.owner_id,
				COUNT(*)                              AS total_nodes,
				COUNT(*) FILTER (WHERE r.is_ready AND NOT r.stale_data)  AS ready_nodes,
				COUNT(*) FILTER (WHERE NOT r.is_ready AND NOT r.stale_data) AS blocked_nodes,
				COUNT(*) FILTER (WHERE r.stale_data)  AS stale_nodes
			FROM node_keys nk
			LEFT JOIN readiness r ON r.entity_key = nk.entity_key
			GROUP BY nk.owner_id
		)
		SELECT
			o.id, o.name, o.display_name, o.contact_email, o.contact_channel,
			o.owner_type, o.metadata, o.created_at, o.updated_at,
			COALESCE(c.node_count, 0),
			COALESCE(c.cookbook_count, 0),
			COALESCE(c.git_repo_count, 0),
			COALESCE(c.role_count, 0),
			COALESCE(c.policy_count, 0),
			COALESCE(orr.ready_nodes, 0),
			COALESCE(orr.blocked_nodes, 0),
			COALESCE(orr.stale_nodes, 0),
			COALESCE(orr.total_nodes, 0)
		FROM owners o
		LEFT JOIN counts c ON c.owner_id = o.id
		LEFT JOIN owner_readiness orr ON orr.owner_id = o.id
		%s
		ORDER BY %s %s, o.name ASC
		LIMIT %s OFFSET %s
	`, readinessCTE, where, orderBy, dir, limitParam, offsetParam)

	rows, err := q.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: listing owners with summary: %w", err)
	}
	defer rows.Close()

	var results []OwnerWithSummary
	for rows.Next() {
		var o OwnerWithSummary
		var displayName, contactEmail, contactChannel sql.NullString
		var metadata []byte

		if err := rows.Scan(
			&o.ID,
			&o.Name,
			&displayName,
			&contactEmail,
			&contactChannel,
			&o.OwnerType,
			&metadata,
			&o.CreatedAt,
			&o.UpdatedAt,
			&o.NodeCount,
			&o.CookbookCount,
			&o.GitRepoCount,
			&o.RoleCount,
			&o.PolicyCount,
			&o.ReadyNodes,
			&o.BlockedNodes,
			&o.StaleNodes,
			&o.TotalNodes,
		); err != nil {
			return nil, 0, fmt.Errorf("datastore: scanning owner with summary: %w", err)
		}

		o.DisplayName = stringFromNull(displayName)
		o.ContactEmail = stringFromNull(contactEmail)
		o.ContactChannel = stringFromNull(contactChannel)
		if metadata != nil {
			o.Metadata = json.RawMessage(metadata)
		}
		results = append(results, o)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("datastore: iterating owner summary rows: %w", err)
	}

	return results, total, nil
}

// UpdateOwner updates an existing owner by name. Returns ErrNotFound if no
// such owner exists.
func (db *DB) UpdateOwner(ctx context.Context, name string, p UpdateOwnerParams) (Owner, error) {
	return db.updateOwner(ctx, db.q(), name, p)
}

func (db *DB) updateOwner(ctx context.Context, q queryable, name string, p UpdateOwnerParams) (Owner, error) {
	// Build a dynamic SET clause based on which fields are provided.
	setClauses := []string{"updated_at = now()"}
	args := []any{}
	argN := 1

	if p.DisplayName != nil {
		setClauses = append(setClauses, fmt.Sprintf("display_name = $%d", argN))
		args = append(args, nullString(*p.DisplayName))
		argN++
	}
	if p.ContactEmail != nil {
		setClauses = append(setClauses, fmt.Sprintf("contact_email = $%d", argN))
		args = append(args, nullString(*p.ContactEmail))
		argN++
	}
	if p.ContactChannel != nil {
		setClauses = append(setClauses, fmt.Sprintf("contact_channel = $%d", argN))
		args = append(args, nullString(*p.ContactChannel))
		argN++
	}
	if p.OwnerType != nil {
		setClauses = append(setClauses, fmt.Sprintf("owner_type = $%d", argN))
		args = append(args, *p.OwnerType)
		argN++
	}
	if p.Metadata != nil {
		if *p.Metadata == nil {
			setClauses = append(setClauses, fmt.Sprintf("metadata = $%d", argN))
			args = append(args, nil)
		} else {
			setClauses = append(setClauses, fmt.Sprintf("metadata = $%d", argN))
			args = append(args, []byte(*p.Metadata))
		}
		argN++
	}

	query := fmt.Sprintf(`
		UPDATE owners SET %s
		WHERE name = $%d
		RETURNING id, name, display_name, contact_email, contact_channel,
		          owner_type, metadata, created_at, updated_at
	`, joinStrings(setClauses, ", "), argN)
	args = append(args, name)

	owner, err := scanOwner(q.QueryRowContext(ctx, query, args...))
	if err != nil {
		return Owner{}, fmt.Errorf("datastore: updating owner %q: %w", name, err)
	}
	return owner, nil
}

// DeleteOwner removes an owner by name. Returns ErrNotFound if no such owner
// exists. Cascading deletes remove all ownership_assignments for this owner.
// Returns the number of cascaded assignments.
func (db *DB) DeleteOwner(ctx context.Context, name string) (int, error) {
	return db.deleteOwner(ctx, db.q(), name)
}

func (db *DB) deleteOwner(ctx context.Context, q queryable, name string) (int, error) {
	// Count assignments that will be cascaded.
	var assignmentCount int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ownership_assignments WHERE owner_id = (SELECT id FROM owners WHERE name = $1)`,
		name,
	).Scan(&assignmentCount)
	if err != nil {
		return 0, fmt.Errorf("datastore: counting assignments for owner %q: %w", name, err)
	}

	res, err := q.ExecContext(ctx, `DELETE FROM owners WHERE name = $1`, name)
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting owner %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	if n == 0 {
		return 0, ErrNotFound
	}
	return assignmentCount, nil
}

// CountAssignmentsByOwner returns the assignment count per entity type for
// the given owner name.
func (db *DB) CountAssignmentsByOwner(ctx context.Context, ownerName string) (map[string]int, error) {
	return db.countAssignmentsByOwner(ctx, db.q(), ownerName)
}

func (db *DB) countAssignmentsByOwner(ctx context.Context, q queryable, ownerName string) (map[string]int, error) {
	const query = `
		SELECT oa.entity_type, COUNT(*)
		FROM ownership_assignments oa
		JOIN owners o ON o.id = oa.owner_id
		WHERE o.name = $1
		GROUP BY oa.entity_type
	`
	rows, err := q.QueryContext(ctx, query, ownerName)
	if err != nil {
		return nil, fmt.Errorf("datastore: counting assignments for owner %q: %w", ownerName, err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var entityType string
		var count int
		if err := rows.Scan(&entityType, &count); err != nil {
			return nil, fmt.Errorf("datastore: scanning assignment count: %w", err)
		}
		counts[entityType] = count
	}
	return counts, rows.Err()
}

// ---------------------------------------------------------------------------
// Owner detail summaries (spec § 4.1)
// ---------------------------------------------------------------------------

// OwnerReadinessSummary holds migration readiness data for nodes owned by an owner.
type OwnerReadinessSummary struct {
	TargetChefVersion string                    `json:"target_chef_version"`
	TotalNodes        int                       `json:"total_nodes"`
	Ready             int                       `json:"ready"`
	Blocked           int                       `json:"blocked"`
	Stale             int                       `json:"stale"`
	BlockingCookbooks []BlockingCookbookSummary `json:"blocking_cookbooks"`
}

// BlockingCookbookSummary holds a blocking cookbook entry within an owner's
// readiness summary.
type BlockingCookbookSummary struct {
	CookbookName      string `json:"cookbook_name"`
	ComplexityLabel   string `json:"complexity_label"`
	AffectedNodeCount int    `json:"affected_node_count"`
}

// parseBlockingCookbookNames extracts cookbook names from the blocking_cookbooks
// JSONB column. It handles two formats:
//   - Legacy: simple string array, e.g. ["apt", "nginx"]
//   - Multi-source: array of structured objects with a "name" field,
//     e.g. [{"name":"apt","version":"7.4.0","reason":"incompatible",...}]
func parseBlockingCookbookNames(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}

	// Try simple string array first (legacy format).
	var names []string
	if err := json.Unmarshal(raw, &names); err == nil {
		return names
	}

	// Try structured array with "name" field (multi-source format).
	var objs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &objs); err == nil {
		result := make([]string, 0, len(objs))
		for _, o := range objs {
			if o.Name != "" {
				result = append(result, o.Name)
			}
		}
		return result
	}

	return nil
}

// OwnerCookbookSummary holds compatibility data for cookbooks owned by an owner.
type OwnerCookbookSummary struct {
	Total        int `json:"total"`
	Compatible   int `json:"compatible"`
	Incompatible int `json:"incompatible"`
	Untested     int `json:"untested"`
}

// OwnerGitRepoSummary holds compatibility data for git repos owned by an owner.
type OwnerGitRepoSummary struct {
	Total        int `json:"total"`
	Compatible   int `json:"compatible"`
	Incompatible int `json:"incompatible"`
}

// GetOwnerReadinessSummary computes migration readiness data for all nodes
// assigned to the given owner. It joins ownership_assignments with
// node_readiness to produce counts of ready, blocked, and stale nodes, plus
// blocking cookbook details.
func (db *DB) GetOwnerReadinessSummary(ctx context.Context, ownerName, targetChefVersion string) (OwnerReadinessSummary, error) {
	return db.getOwnerReadinessSummary(ctx, db.q(), ownerName, targetChefVersion)
}

func (db *DB) getOwnerReadinessSummary(ctx context.Context, q queryable, ownerName, targetChefVersion string) (OwnerReadinessSummary, error) {
	summary := OwnerReadinessSummary{
		TargetChefVersion: targetChefVersion,
		BlockingCookbooks: []BlockingCookbookSummary{},
	}

	// Step 1: Get the node entity keys assigned to this owner.
	const nodeKeysQuery = `
		SELECT oa.entity_key
		FROM ownership_assignments oa
		JOIN owners o ON o.id = oa.owner_id
		WHERE o.name = $1 AND oa.entity_type = 'node'
	`
	rows, err := q.QueryContext(ctx, nodeKeysQuery, ownerName)
	if err != nil {
		return summary, fmt.Errorf("datastore: listing owned nodes for %q: %w", ownerName, err)
	}
	defer rows.Close()

	var nodeNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return summary, fmt.Errorf("datastore: scanning owned node name: %w", err)
		}
		nodeNames = append(nodeNames, name)
	}
	if err := rows.Err(); err != nil {
		return summary, fmt.Errorf("datastore: iterating owned node names: %w", err)
	}

	if len(nodeNames) == 0 {
		return summary, nil
	}

	// Step 2: For those nodes, query latest readiness records.
	// Build a parameterised IN clause.
	placeholders := make([]string, len(nodeNames))
	args := make([]any, 0, len(nodeNames)+1)
	args = append(args, targetChefVersion)
	for i, n := range nodeNames {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, n)
	}

	readinessQuery := fmt.Sprintf(`
		SELECT nr.is_ready, nr.stale_data, nr.blocking_cookbooks
		FROM node_readiness nr
		WHERE nr.target_chef_version = $1
		  AND nr.node_name IN (%s)
		  AND nr.id IN (
		      SELECT DISTINCT ON (nr2.node_name)
		             nr2.id
		        FROM node_readiness nr2
		       WHERE nr2.target_chef_version = $1
		         AND nr2.node_name IN (%s)
		       ORDER BY nr2.node_name, nr2.evaluated_at DESC
		  )
	`, strings.Join(placeholders, ", "), strings.Join(placeholders, ", "))

	rRows, err := q.QueryContext(ctx, readinessQuery, args...)
	if err != nil {
		return summary, fmt.Errorf("datastore: querying readiness for owned nodes: %w", err)
	}
	defer rRows.Close()

	// Track blocking cookbooks across all blocked nodes.
	blockingCounts := map[string]int{} // cookbook_name -> affected node count

	for rRows.Next() {
		var isReady, staleData bool
		var blockingCookbooks []byte

		if err := rRows.Scan(&isReady, &staleData, &blockingCookbooks); err != nil {
			return summary, fmt.Errorf("datastore: scanning node readiness row: %w", err)
		}

		summary.TotalNodes++
		switch {
		case staleData:
			summary.Stale++
		case isReady:
			summary.Ready++
		default:
			summary.Blocked++

			// Parse blocking cookbooks JSON array. The JSONB column may
			// contain either a simple string array (legacy) or an array
			// of structured BlockingCookbook objects (multi-source).
			if len(blockingCookbooks) > 0 {
				names := parseBlockingCookbookNames(blockingCookbooks)
				for _, cb := range names {
					blockingCounts[cb]++
				}
			}
		}
	}
	if err := rRows.Err(); err != nil {
		return summary, fmt.Errorf("datastore: iterating readiness rows: %w", err)
	}

	// Also count nodes that had no readiness record at all as stale.
	noRecordCount := len(nodeNames) - summary.TotalNodes
	if noRecordCount > 0 {
		summary.TotalNodes += noRecordCount
		summary.Stale += noRecordCount
	}

	// Step 3: Look up complexity labels for blocking cookbooks.
	for cbName, count := range blockingCounts {
		label := ""
		// Find the cookbook by name and look up its complexity label.
		const labelQuery = `
			SELECT cc.complexity_label
			FROM cookbook_complexity cc
			JOIN cookbooks c ON c.id = cc.cookbook_id
			WHERE c.name = $1 AND cc.target_chef_version = $2
			ORDER BY cc.evaluated_at DESC
			LIMIT 1
		`
		var complexityLabel sql.NullString
		err := q.QueryRowContext(ctx, labelQuery, cbName, targetChefVersion).Scan(&complexityLabel)
		if err == nil && complexityLabel.Valid {
			label = complexityLabel.String
		}

		summary.BlockingCookbooks = append(summary.BlockingCookbooks, BlockingCookbookSummary{
			CookbookName:      cbName,
			ComplexityLabel:   label,
			AffectedNodeCount: count,
		})
	}

	return summary, nil
}

// GetOwnerCookbookSummary computes compatibility data for cookbooks assigned
// to the given owner. A cookbook is "compatible" if its cookbook_complexity
// record has error_count = 0, "incompatible" if error_count > 0, and
// "untested" if no complexity record exists for the target version.
func (db *DB) GetOwnerCookbookSummary(ctx context.Context, ownerName, targetChefVersion string) (OwnerCookbookSummary, error) {
	return db.getOwnerCookbookSummary(ctx, db.q(), ownerName, targetChefVersion)
}

func (db *DB) getOwnerCookbookSummary(ctx context.Context, q queryable, ownerName, targetChefVersion string) (OwnerCookbookSummary, error) {
	var summary OwnerCookbookSummary

	// Step 1: Get cookbook entity keys assigned to this owner.
	const cbKeysQuery = `
		SELECT oa.entity_key
		FROM ownership_assignments oa
		JOIN owners o ON o.id = oa.owner_id
		WHERE o.name = $1 AND oa.entity_type = 'cookbook'
	`
	rows, err := q.QueryContext(ctx, cbKeysQuery, ownerName)
	if err != nil {
		return summary, fmt.Errorf("datastore: listing owned cookbooks for %q: %w", ownerName, err)
	}
	defer rows.Close()

	var cookbookNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return summary, fmt.Errorf("datastore: scanning owned cookbook name: %w", err)
		}
		cookbookNames = append(cookbookNames, name)
	}
	if err := rows.Err(); err != nil {
		return summary, fmt.Errorf("datastore: iterating owned cookbook names: %w", err)
	}

	summary.Total = len(cookbookNames)
	if summary.Total == 0 {
		return summary, nil
	}

	// Step 2: For each cookbook, check complexity status.
	for _, cbName := range cookbookNames {
		const statusQuery = `
			SELECT cc.error_count
			FROM cookbook_complexity cc
			JOIN cookbooks c ON c.id = cc.cookbook_id
			WHERE c.name = $1 AND cc.target_chef_version = $2
			ORDER BY cc.evaluated_at DESC
			LIMIT 1
		`
		var errorCount int
		err := q.QueryRowContext(ctx, statusQuery, cbName, targetChefVersion).Scan(&errorCount)
		if err == sql.ErrNoRows {
			summary.Untested++
			continue
		}
		if err != nil {
			// Treat query errors as untested to avoid failing the whole summary.
			summary.Untested++
			continue
		}
		if errorCount > 0 {
			summary.Incompatible++
		} else {
			summary.Compatible++
		}
	}

	return summary, nil
}

// GetOwnerGitRepoSummary computes compatibility data for git repos assigned
// to the given owner. For each git repo, it looks up cookbooks by
// git_repo_url and checks their complexity records.
func (db *DB) GetOwnerGitRepoSummary(ctx context.Context, ownerName, targetChefVersion string) (OwnerGitRepoSummary, error) {
	return db.getOwnerGitRepoSummary(ctx, db.q(), ownerName, targetChefVersion)
}

func (db *DB) getOwnerGitRepoSummary(ctx context.Context, q queryable, ownerName, targetChefVersion string) (OwnerGitRepoSummary, error) {
	var summary OwnerGitRepoSummary

	// Step 1: Get git_repo entity keys assigned to this owner.
	const repoKeysQuery = `
		SELECT oa.entity_key
		FROM ownership_assignments oa
		JOIN owners o ON o.id = oa.owner_id
		WHERE o.name = $1 AND oa.entity_type = 'git_repo'
	`
	rows, err := q.QueryContext(ctx, repoKeysQuery, ownerName)
	if err != nil {
		return summary, fmt.Errorf("datastore: listing owned git repos for %q: %w", ownerName, err)
	}
	defer rows.Close()

	var repoURLs []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return summary, fmt.Errorf("datastore: scanning owned git repo URL: %w", err)
		}
		repoURLs = append(repoURLs, url)
	}
	if err := rows.Err(); err != nil {
		return summary, fmt.Errorf("datastore: iterating owned git repo URLs: %w", err)
	}

	summary.Total = len(repoURLs)
	if summary.Total == 0 {
		return summary, nil
	}

	// Step 2: For each git repo, check git_repo_complexity
	// whether any has a failing complexity record.
	for _, repoURL := range repoURLs {
		const compatQuery = `
			SELECT COALESCE(MIN(grc.error_count), -1)
			FROM git_repos gr
			LEFT JOIN git_repo_complexity grc
			       ON grc.git_repo_id = gr.id AND grc.target_chef_version = $2
			WHERE gr.git_repo_url = $1
		`
		var minErrors int
		err := q.QueryRowContext(ctx, compatQuery, repoURL, targetChefVersion).Scan(&minErrors)
		if err != nil || minErrors < 0 {
			// No cookbooks or no complexity records — treat as incompatible
			// since we can't confirm compatibility.
			summary.Incompatible++
			continue
		}
		if minErrors > 0 {
			summary.Incompatible++
		} else {
			summary.Compatible++
		}
	}

	return summary, nil
}

// ---------------------------------------------------------------------------
// Auto-rule management helpers
// ---------------------------------------------------------------------------

// ListDistinctAutoRuleNames returns all distinct auto_rule_name values
// currently stored in ownership_assignments.
func (db *DB) ListDistinctAutoRuleNames(ctx context.Context) ([]string, error) {
	return db.listDistinctAutoRuleNames(ctx, db.q())
}

func (db *DB) listDistinctAutoRuleNames(ctx context.Context, q queryable) ([]string, error) {
	const query = `
		SELECT DISTINCT auto_rule_name
		FROM ownership_assignments
		WHERE auto_rule_name IS NOT NULL
		ORDER BY auto_rule_name
	`
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing distinct auto rule names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("datastore: scanning auto rule name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// DeleteAutoRuleAssignmentsByName deletes all ownership assignments with the
// given auto_rule_name and returns the number of deleted rows. This is used
// during startup cleanup to remove assignments for rules that have been
// removed from the configuration.
func (db *DB) DeleteAutoRuleAssignmentsByName(ctx context.Context, autoRuleName string) (int, error) {
	return db.deleteAutoRuleAssignmentsByName(ctx, db.q(), autoRuleName)
}

func (db *DB) deleteAutoRuleAssignmentsByName(ctx context.Context, q queryable, autoRuleName string) (int, error) {
	const query = `DELETE FROM ownership_assignments WHERE auto_rule_name = $1`
	res, err := q.ExecContext(ctx, query, autoRuleName)
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting auto-rule assignments for %q: %w", autoRuleName, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	return int(n), nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanOwner(row *sql.Row) (Owner, error) {
	var o Owner
	var displayName, contactEmail, contactChannel sql.NullString
	var metadata []byte

	err := row.Scan(
		&o.ID,
		&o.Name,
		&displayName,
		&contactEmail,
		&contactChannel,
		&o.OwnerType,
		&metadata,
		&o.CreatedAt,
		&o.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Owner{}, ErrNotFound
		}
		return Owner{}, fmt.Errorf("datastore: scanning owner: %w", err)
	}

	o.DisplayName = stringFromNull(displayName)
	o.ContactEmail = stringFromNull(contactEmail)
	o.ContactChannel = stringFromNull(contactChannel)
	if metadata != nil {
		o.Metadata = json.RawMessage(metadata)
	}
	return o, nil
}

func scanOwners(rows *sql.Rows) ([]Owner, error) {
	var owners []Owner
	for rows.Next() {
		var o Owner
		var displayName, contactEmail, contactChannel sql.NullString
		var metadata []byte

		if err := rows.Scan(
			&o.ID,
			&o.Name,
			&displayName,
			&contactEmail,
			&contactChannel,
			&o.OwnerType,
			&metadata,
			&o.CreatedAt,
			&o.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning owner row: %w", err)
		}

		o.DisplayName = stringFromNull(displayName)
		o.ContactEmail = stringFromNull(contactEmail)
		o.ContactChannel = stringFromNull(contactChannel)
		if metadata != nil {
			o.Metadata = json.RawMessage(metadata)
		}
		owners = append(owners, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating owner rows: %w", err)
	}
	return owners, nil
}
