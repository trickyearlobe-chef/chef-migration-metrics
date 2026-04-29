// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// NodeReadiness represents a row in the node_readiness table. Each record
// captures whether a specific node is ready for upgrade to a specific target
// Chef Client version.
type NodeReadiness struct {
	OrganisationName       string          `json:"organisation_name"`
	NodeName               string          `json:"node_name"`
	TargetChefVersion      string          `json:"target_chef_version"`
	IsReady                bool            `json:"is_ready"`
	AllCookbooksCompatible bool            `json:"all_cookbooks_compatible"`
	SufficientDiskSpace    *bool           `json:"sufficient_disk_space"` // nil = unknown
	BlockingCookbooks      json.RawMessage `json:"blocking_cookbooks"`    // JSONB array
	AvailableDiskMB        *int            `json:"available_disk_mb"`     // nil = unknown
	RequiredDiskMB         *int            `json:"required_disk_mb"`      // nil = not set
	StaleData              bool            `json:"stale_data"`
	CookstyleStatus        string          `json:"cookstyle_status"`
	KitchenStatus          string          `json:"kitchen_status"`
	EvaluatedAt            time.Time       `json:"evaluated_at"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

// UpsertNodeReadinessParams contains the fields needed to insert or update
// a node_readiness row. The unique constraint is
// (organisation_name, node_name, target_chef_version).
type UpsertNodeReadinessParams struct {
	OrganisationName       string
	NodeName               string
	TargetChefVersion      string
	IsReady                bool
	AllCookbooksCompatible bool
	SufficientDiskSpace    *bool           // nil = unknown
	BlockingCookbooks      json.RawMessage // JSONB array
	AvailableDiskMB        *int            // nil = unknown
	RequiredDiskMB         *int            // nil = not set
	StaleData              bool
	CookstyleStatus        string // "passed", "failed", "unknown"
	KitchenStatus          string // "passed", "failed", "partial", "unknown"
	EvaluatedAt            time.Time
}

// ---------------------------------------------------------------------------
// Column list — shared across all queries
// ---------------------------------------------------------------------------

const nrColumns = `organisation_name, node_name,
       target_chef_version, is_ready, all_cookbooks_compatible,
       sufficient_disk_space, blocking_cookbooks, available_disk_mb,
       required_disk_mb, stale_data, cookstyle_status, kitchen_status,
       evaluated_at, created_at, updated_at`

// latestReadinessForOrg returns a SQL fragment that restricts results to the
// single most recent node_readiness row for each (node_name, target_chef_version)
// combination within the specified organisation. The orgParam argument is the
// SQL parameter placeholder for the organisation_name (e.g. "$1").
func latestReadinessForOrg(orgParam string) string {
	return fmt.Sprintf(`(organisation_name, node_name, target_chef_version, evaluated_at) IN (
        SELECT organisation_name, node_name, target_chef_version, MAX(evaluated_at)
          FROM node_readiness
         WHERE organisation_name = %s
         GROUP BY organisation_name, node_name, target_chef_version
    )`, orgParam)
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetNodeReadiness returns the readiness record for the given organisation,
// node, and target Chef version. Returns (nil, nil) if no record exists.
func (db *DB) GetNodeReadiness(ctx context.Context, orgName, nodeName, targetChefVersion string) (*NodeReadiness, error) {
	return db.getNodeReadiness(ctx, db.q(), orgName, nodeName, targetChefVersion)
}

func (db *DB) getNodeReadiness(ctx context.Context, q queryable, orgName, nodeName, targetChefVersion string) (*NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_name = $1
		   AND node_name = $2
		   AND target_chef_version = $3
	`

	r, err := scanNodeReadiness(q.QueryRowContext(ctx, query, orgName, nodeName, targetChefVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting node readiness: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListNodeReadinessForSnapshot returns all readiness records for the given
// organisation and node, ordered by target_chef_version.
func (db *DB) ListNodeReadinessForSnapshot(ctx context.Context, orgName, nodeName string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_name = $1
		   AND node_name = $2
		 ORDER BY target_chef_version
	`
	return db.scanNodeReadinessRows(ctx, query, orgName, nodeName)
}

// ListNodeReadinessByNodeName returns the latest readiness records for the
// given node within the specified organisation. This queries by
// (organisation_name, node_name), making it resilient to snapshot ID changes
// across collection runs.
func (db *DB) ListNodeReadinessByNodeName(ctx context.Context, orgName, nodeName string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_name = $1
		   AND node_name = $2
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY target_chef_version
	`
	return db.scanNodeReadinessRows(ctx, query, orgName, nodeName)
}

// BulkListNodeReadinessByNodeNames returns the latest readiness records for
// multiple nodes within the specified organisation in a single query. This
// replaces the N+1 pattern of calling ListNodeReadinessByNodeName per node.
// Results are returned as a map keyed by node_name for O(1) lookup.
func (db *DB) BulkListNodeReadinessByNodeNames(ctx context.Context, orgName string, nodeNames []string) (map[string][]NodeReadiness, error) {
	if len(nodeNames) == 0 {
		return nil, nil
	}

	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_name = $1
		   AND node_name = ANY($2)
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY node_name, target_chef_version
	`
	rows, err := db.pool.QueryContext(ctx, query, orgName, pq.Array(nodeNames))
	if err != nil {
		return nil, fmt.Errorf("datastore: bulk listing node readiness: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]NodeReadiness, len(nodeNames))
	for rows.Next() {
		var r NodeReadiness
		var sufficientDisk sql.NullBool
		var availableDisk, requiredDisk sql.NullInt64
		var blockingCookbooks []byte

		if err := rows.Scan(
			&r.OrganisationName,
			&r.NodeName,
			&r.TargetChefVersion,
			&r.IsReady,
			&r.AllCookbooksCompatible,
			&sufficientDisk,
			&blockingCookbooks,
			&availableDisk,
			&requiredDisk,
			&r.StaleData,
			&r.CookstyleStatus,
			&r.KitchenStatus,
			&r.EvaluatedAt,
			&r.CreatedAt,
			&r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning bulk node readiness row: %w", err)
		}

		if sufficientDisk.Valid {
			v := sufficientDisk.Bool
			r.SufficientDiskSpace = &v
		}
		if availableDisk.Valid {
			v := int(availableDisk.Int64)
			r.AvailableDiskMB = &v
		}
		if requiredDisk.Valid {
			v := int(requiredDisk.Int64)
			r.RequiredDiskMB = &v
		}
		r.BlockingCookbooks = jsonFromNullBytes(blockingCookbooks)

		result[r.NodeName] = append(result[r.NodeName], r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating bulk node readiness rows: %w", err)
	}
	return result, nil
}

// ListNodeReadinessForOrganisation returns all readiness records for the
// given organisation from the latest completed collection run, ordered by
// node_name then target_chef_version.
func (db *DB) ListNodeReadinessForOrganisation(ctx context.Context, orgName string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_name = $1
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY node_name, target_chef_version
	`
	return db.scanNodeReadinessRows(ctx, query, orgName)
}

// ListNodeReadinessForOrganisationAndTarget returns all readiness records
// for the given organisation and target Chef version from the latest
// completed collection run, ordered by node name.
func (db *DB) ListNodeReadinessForOrganisationAndTarget(ctx context.Context, orgName, targetChefVersion string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_name = $1
		   AND target_chef_version = $2
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY node_name
	`
	return db.scanNodeReadinessRows(ctx, query, orgName, targetChefVersion)
}

// ListReadyNodes returns all readiness records where is_ready = TRUE for
// the given organisation and target Chef version, scoped to the latest
// completed collection run.
func (db *DB) ListReadyNodes(ctx context.Context, orgName, targetChefVersion string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_name = $1
		   AND target_chef_version = $2
		   AND is_ready = TRUE
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY node_name
	`
	return db.scanNodeReadinessRows(ctx, query, orgName, targetChefVersion)
}

// ListBlockedNodes returns all readiness records where is_ready = FALSE for
// the given organisation and target Chef version, scoped to the latest
// completed collection run.
func (db *DB) ListBlockedNodes(ctx context.Context, orgName, targetChefVersion string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_name = $1
		   AND target_chef_version = $2
		   AND is_ready = FALSE
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY node_name
	`
	return db.scanNodeReadinessRows(ctx, query, orgName, targetChefVersion)
}

// ListStaleNodeReadiness returns all readiness records where stale_data = TRUE
// for the given organisation from the latest completed collection run,
// ordered by node name.
func (db *DB) ListStaleNodeReadiness(ctx context.Context, orgName string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_name = $1
		   AND stale_data = TRUE
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY node_name, target_chef_version
	`
	return db.scanNodeReadinessRows(ctx, query, orgName)
}

// ---------------------------------------------------------------------------
// Count
// ---------------------------------------------------------------------------

// CountNodeReadiness returns the total, ready, and blocked counts for the
// given organisation and target Chef version, scoped to the latest completed
// collection run. Without this scoping, every historical collection cycle's
// readiness rows would be counted, inflating the totals.
func (db *DB) CountNodeReadiness(ctx context.Context, orgName, targetChefVersion string) (total, ready, blocked int, err error) {
	query := `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE is_ready = TRUE),
			COUNT(*) FILTER (WHERE is_ready = FALSE)
		  FROM node_readiness
		 WHERE organisation_name = $1
		   AND target_chef_version = $2
		   AND ` + latestReadinessForOrg("$1") + `
	`
	err = db.pool.QueryRowContext(ctx, query, orgName, targetChefVersion).Scan(&total, &ready, &blocked)
	if err != nil {
		err = fmt.Errorf("datastore: counting node readiness: %w", err)
	}
	return
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertNodeReadiness inserts a new readiness record or updates the existing
// one for the same (organisation_name, node_name, target_chef_version)
// combination. Returns the resulting row.
func (db *DB) UpsertNodeReadiness(ctx context.Context, p UpsertNodeReadinessParams) (*NodeReadiness, error) {
	return db.upsertNodeReadiness(ctx, db.q(), p)
}

// UpsertNodeReadinessTx is the transactional variant of UpsertNodeReadiness.
func (db *DB) UpsertNodeReadinessTx(ctx context.Context, tx *sql.Tx, p UpsertNodeReadinessParams) (*NodeReadiness, error) {
	return db.upsertNodeReadiness(ctx, tx, p)
}

func (db *DB) upsertNodeReadiness(ctx context.Context, q queryable, p UpsertNodeReadinessParams) (*NodeReadiness, error) {
	if p.OrganisationName == "" {
		return nil, fmt.Errorf("datastore: organisation_name is required")
	}
	if p.NodeName == "" {
		return nil, fmt.Errorf("datastore: node_name is required")
	}
	if p.TargetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target_chef_version is required")
	}

	query := `
		INSERT INTO node_readiness (
			organisation_name, node_name,
			target_chef_version, is_ready, all_cookbooks_compatible,
			sufficient_disk_space, blocking_cookbooks, available_disk_mb,
			required_disk_mb, stale_data, cookstyle_status, kitchen_status,
			evaluated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (organisation_name, node_name, target_chef_version)
		DO UPDATE SET
			is_ready                = EXCLUDED.is_ready,
			all_cookbooks_compatible = EXCLUDED.all_cookbooks_compatible,
			sufficient_disk_space   = EXCLUDED.sufficient_disk_space,
			blocking_cookbooks      = EXCLUDED.blocking_cookbooks,
			available_disk_mb       = EXCLUDED.available_disk_mb,
			required_disk_mb        = EXCLUDED.required_disk_mb,
			stale_data              = EXCLUDED.stale_data,
			cookstyle_status        = EXCLUDED.cookstyle_status,
			kitchen_status          = EXCLUDED.kitchen_status,
			evaluated_at            = EXCLUDED.evaluated_at,
			updated_at              = now()
		RETURNING ` + nrColumns + `
	`

	r, err := scanNodeReadiness(q.QueryRowContext(ctx, query,
		p.OrganisationName,
		p.NodeName,
		p.TargetChefVersion,
		p.IsReady,
		p.AllCookbooksCompatible,
		nullBoolPtr(p.SufficientDiskSpace),
		nullJSON(p.BlockingCookbooks),
		nullIntPtr(p.AvailableDiskMB),
		nullIntPtr(p.RequiredDiskMB),
		p.StaleData,
		p.CookstyleStatus,
		p.KitchenStatus,
		p.EvaluatedAt,
	))
	if err != nil {
		return nil, fmt.Errorf("datastore: upserting node readiness: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteNodeReadinessForSnapshot removes all readiness records for the given
// organisation and node. Called when a new collection run replaces the snapshot.
func (db *DB) DeleteNodeReadinessForSnapshot(ctx context.Context, orgName, nodeName string) error {
	const query = `DELETE FROM node_readiness WHERE organisation_name = $1 AND node_name = $2`
	_, err := db.pool.ExecContext(ctx, query, orgName, nodeName)
	if err != nil {
		return fmt.Errorf("datastore: deleting node readiness for %s/%s: %w", orgName, nodeName, err)
	}
	return nil
}

// DeleteNodeReadinessForOrganisation removes all readiness records for the
// given organisation. Forces a full re-evaluation on the next cycle.
func (db *DB) DeleteNodeReadinessForOrganisation(ctx context.Context, orgName string) error {
	const query = `DELETE FROM node_readiness WHERE organisation_name = $1`
	_, err := db.pool.ExecContext(ctx, query, orgName)
	if err != nil {
		return fmt.Errorf("datastore: deleting node readiness for organisation %s: %w", orgName, err)
	}
	return nil
}

// DeleteNodeReadinessForOrganisationAndTarget removes all readiness records
// for the given organisation and target Chef version.
func (db *DB) DeleteNodeReadinessForOrganisationAndTarget(ctx context.Context, orgName, targetChefVersion string) error {
	const query = `DELETE FROM node_readiness WHERE organisation_name = $1 AND target_chef_version = $2`
	_, err := db.pool.ExecContext(ctx, query, orgName, targetChefVersion)
	if err != nil {
		return fmt.Errorf("datastore: deleting node readiness for organisation %s version %s: %w", orgName, targetChefVersion, err)
	}
	return nil
}

// DeleteNodeReadiness removes a single readiness record by its natural key.
func (db *DB) DeleteNodeReadiness(ctx context.Context, orgName, nodeName, targetChefVersion string) error {
	const query = `DELETE FROM node_readiness WHERE organisation_name = $1 AND node_name = $2 AND target_chef_version = $3`
	res, err := db.pool.ExecContext(ctx, query, orgName, nodeName, targetChefVersion)
	if err != nil {
		return fmt.Errorf("datastore: deleting node readiness %s/%s@%s: %w", orgName, nodeName, targetChefVersion, err)
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

func scanNodeReadiness(row interface{ Scan(dest ...any) error }) (NodeReadiness, error) {
	var r NodeReadiness
	var sufficientDisk sql.NullBool
	var availableDisk, requiredDisk sql.NullInt64
	var blockingCookbooks []byte

	err := row.Scan(
		&r.OrganisationName,
		&r.NodeName,
		&r.TargetChefVersion,
		&r.IsReady,
		&r.AllCookbooksCompatible,
		&sufficientDisk,
		&blockingCookbooks,
		&availableDisk,
		&requiredDisk,
		&r.StaleData,
		&r.CookstyleStatus,
		&r.KitchenStatus,
		&r.EvaluatedAt,
		&r.CreatedAt,
		&r.UpdatedAt,
	)
	if err != nil {
		return NodeReadiness{}, err
	}

	if sufficientDisk.Valid {
		v := sufficientDisk.Bool
		r.SufficientDiskSpace = &v
	}
	if availableDisk.Valid {
		v := int(availableDisk.Int64)
		r.AvailableDiskMB = &v
	}
	if requiredDisk.Valid {
		v := int(requiredDisk.Int64)
		r.RequiredDiskMB = &v
	}
	r.BlockingCookbooks = jsonFromNullBytes(blockingCookbooks)

	return r, nil
}

func (db *DB) scanNodeReadinessRows(ctx context.Context, query string, args ...any) ([]NodeReadiness, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing node readiness: %w", err)
	}
	defer rows.Close()

	var results []NodeReadiness
	for rows.Next() {
		var r NodeReadiness
		var sufficientDisk sql.NullBool
		var availableDisk, requiredDisk sql.NullInt64
		var blockingCookbooks []byte

		if err := rows.Scan(
			&r.OrganisationName,
			&r.NodeName,
			&r.TargetChefVersion,
			&r.IsReady,
			&r.AllCookbooksCompatible,
			&sufficientDisk,
			&blockingCookbooks,
			&availableDisk,
			&requiredDisk,
			&r.StaleData,
			&r.CookstyleStatus,
			&r.KitchenStatus,
			&r.EvaluatedAt,
			&r.CreatedAt,
			&r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning node readiness row: %w", err)
		}

		if sufficientDisk.Valid {
			v := sufficientDisk.Bool
			r.SufficientDiskSpace = &v
		}
		if availableDisk.Valid {
			v := int(availableDisk.Int64)
			r.AvailableDiskMB = &v
		}
		if requiredDisk.Valid {
			v := int(requiredDisk.Int64)
			r.RequiredDiskMB = &v
		}
		r.BlockingCookbooks = jsonFromNullBytes(blockingCookbooks)

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating node readiness rows: %w", err)
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Nullable pointer helpers
// ---------------------------------------------------------------------------

// nullBoolPtr converts a *bool to sql.NullBool. A nil pointer is treated as
// NULL.
func nullBoolPtr(b *bool) sql.NullBool {
	if b == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *b, Valid: true}
}

// nullIntPtr converts a *int to sql.NullInt64. A nil pointer is treated as
// NULL.
func nullIntPtr(i *int) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*i), Valid: true}
}
