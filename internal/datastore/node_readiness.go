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

// NodeReadiness represents a row in the node_readiness table. Each record
// captures whether a specific node is ready for upgrade to a specific target
// Chef Client version.
type NodeReadiness struct {
	ID                     string          `json:"id"`
	NodeSnapshotID         string          `json:"node_snapshot_id"`
	OrganisationID         string          `json:"organisation_id"`
	NodeName               string          `json:"node_name"`
	TargetChefVersion      string          `json:"target_chef_version"`
	IsReady                bool            `json:"is_ready"`
	AllCookbooksCompatible bool            `json:"all_cookbooks_compatible"`
	SufficientDiskSpace    *bool           `json:"sufficient_disk_space"` // nil = unknown
	BlockingCookbooks      json.RawMessage `json:"blocking_cookbooks"`    // JSONB array
	AvailableDiskMB        *int            `json:"available_disk_mb"`     // nil = unknown
	RequiredDiskMB         *int            `json:"required_disk_mb"`      // nil = not set
	StaleData              bool            `json:"stale_data"`
	EvaluatedAt            time.Time       `json:"evaluated_at"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

// UpsertNodeReadinessParams contains the fields needed to insert or update
// a node_readiness row. The unique constraint is
// (node_snapshot_id, target_chef_version).
type UpsertNodeReadinessParams struct {
	NodeSnapshotID         string
	OrganisationID         string
	NodeName               string
	TargetChefVersion      string
	IsReady                bool
	AllCookbooksCompatible bool
	SufficientDiskSpace    *bool           // nil = unknown
	BlockingCookbooks      json.RawMessage // JSONB array
	AvailableDiskMB        *int            // nil = unknown
	RequiredDiskMB         *int            // nil = not set
	StaleData              bool
	EvaluatedAt            time.Time
}

// ---------------------------------------------------------------------------
// Column list — shared across all queries
// ---------------------------------------------------------------------------

const nrColumns = `id, node_snapshot_id, organisation_id, node_name,
       target_chef_version, is_ready, all_cookbooks_compatible,
       sufficient_disk_space, blocking_cookbooks, available_disk_mb,
       required_disk_mb, stale_data, evaluated_at, created_at, updated_at`

// latestReadinessForOrg returns a SQL fragment that restricts results to the
// single most recent node_readiness row for each (node_name, target_chef_version)
// combination within the specified organisation. The orgParam argument is the
// SQL parameter placeholder for the organisation_id (e.g. "$1").
//
// By scoping the inner DISTINCT ON to a single organisation, the query can
// satisfy the ORDER BY via the idx_node_readiness_latest covering index
// without scanning rows for other organisations.
func latestReadinessForOrg(orgParam string) string {
	return fmt.Sprintf(`id IN (
        SELECT DISTINCT ON (node_name, target_chef_version) id
          FROM node_readiness
         WHERE organisation_id = %s
         ORDER BY node_name, target_chef_version, evaluated_at DESC
    )`, orgParam)
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetNodeReadiness returns the readiness record for the given node snapshot
// and target Chef version. Returns (nil, nil) if no record exists.
func (db *DB) GetNodeReadiness(ctx context.Context, nodeSnapshotID, targetChefVersion string) (*NodeReadiness, error) {
	return db.getNodeReadiness(ctx, db.q(), nodeSnapshotID, targetChefVersion)
}

func (db *DB) getNodeReadiness(ctx context.Context, q queryable, nodeSnapshotID, targetChefVersion string) (*NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE node_snapshot_id = $1
		   AND target_chef_version = $2
	`

	r, err := scanNodeReadiness(q.QueryRowContext(ctx, query, nodeSnapshotID, targetChefVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting node readiness: %w", err)
	}
	return &r, nil
}

// GetNodeReadinessByID returns a single readiness record by its primary key.
// Returns ErrNotFound if no record exists.
func (db *DB) GetNodeReadinessByID(ctx context.Context, id string) (*NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE id = $1
	`

	r, err := scanNodeReadiness(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting node readiness by id: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListNodeReadinessForSnapshot returns all readiness records for the given
// node snapshot, ordered by target_chef_version.
func (db *DB) ListNodeReadinessForSnapshot(ctx context.Context, nodeSnapshotID string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE node_snapshot_id = $1
		 ORDER BY target_chef_version
	`
	return db.scanNodeReadinessRows(ctx, query, nodeSnapshotID)
}

// ListNodeReadinessForOrganisation returns all readiness records for the
// given organisation from the latest completed collection run, ordered by
// node_name then target_chef_version.
func (db *DB) ListNodeReadinessForOrganisation(ctx context.Context, organisationID string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_id = $1
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY node_name, target_chef_version
	`
	return db.scanNodeReadinessRows(ctx, query, organisationID)
}

// ListNodeReadinessForOrganisationAndTarget returns all readiness records
// for the given organisation and target Chef version from the latest
// completed collection run, ordered by node name.
func (db *DB) ListNodeReadinessForOrganisationAndTarget(ctx context.Context, organisationID, targetChefVersion string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_id = $1
		   AND target_chef_version = $2
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY node_name
	`
	return db.scanNodeReadinessRows(ctx, query, organisationID, targetChefVersion)
}

// ListReadyNodes returns all readiness records where is_ready = TRUE for
// the given organisation and target Chef version, scoped to the latest
// completed collection run.
func (db *DB) ListReadyNodes(ctx context.Context, organisationID, targetChefVersion string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_id = $1
		   AND target_chef_version = $2
		   AND is_ready = TRUE
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY node_name
	`
	return db.scanNodeReadinessRows(ctx, query, organisationID, targetChefVersion)
}

// ListBlockedNodes returns all readiness records where is_ready = FALSE for
// the given organisation and target Chef version, scoped to the latest
// completed collection run.
func (db *DB) ListBlockedNodes(ctx context.Context, organisationID, targetChefVersion string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_id = $1
		   AND target_chef_version = $2
		   AND is_ready = FALSE
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY node_name
	`
	return db.scanNodeReadinessRows(ctx, query, organisationID, targetChefVersion)
}

// ListStaleNodeReadiness returns all readiness records where stale_data = TRUE
// for the given organisation from the latest completed collection run,
// ordered by node name.
func (db *DB) ListStaleNodeReadiness(ctx context.Context, organisationID string) ([]NodeReadiness, error) {
	query := `
		SELECT ` + nrColumns + `
		  FROM node_readiness
		 WHERE organisation_id = $1
		   AND stale_data = TRUE
		   AND ` + latestReadinessForOrg("$1") + `
		 ORDER BY node_name, target_chef_version
	`
	return db.scanNodeReadinessRows(ctx, query, organisationID)
}

// ---------------------------------------------------------------------------
// Count
// ---------------------------------------------------------------------------

// CountNodeReadiness returns the total, ready, and blocked counts for the
// given organisation and target Chef version, scoped to the latest completed
// collection run. Without this scoping, every historical collection cycle's
// readiness rows would be counted, inflating the totals.
func (db *DB) CountNodeReadiness(ctx context.Context, organisationID, targetChefVersion string) (total, ready, blocked int, err error) {
	query := `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE is_ready = TRUE),
			COUNT(*) FILTER (WHERE is_ready = FALSE)
		  FROM node_readiness
		 WHERE organisation_id = $1
		   AND target_chef_version = $2
		   AND ` + latestReadinessForOrg("$1") + `
	`
	err = db.pool.QueryRowContext(ctx, query, organisationID, targetChefVersion).Scan(&total, &ready, &blocked)
	if err != nil {
		err = fmt.Errorf("datastore: counting node readiness: %w", err)
	}
	return
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertNodeReadiness inserts a new readiness record or updates the existing
// one for the same (node_snapshot_id, target_chef_version) combination.
// Returns the resulting row.
func (db *DB) UpsertNodeReadiness(ctx context.Context, p UpsertNodeReadinessParams) (*NodeReadiness, error) {
	return db.upsertNodeReadiness(ctx, db.q(), p)
}

// UpsertNodeReadinessTx is the transactional variant of UpsertNodeReadiness.
func (db *DB) UpsertNodeReadinessTx(ctx context.Context, tx *sql.Tx, p UpsertNodeReadinessParams) (*NodeReadiness, error) {
	return db.upsertNodeReadiness(ctx, tx, p)
}

func (db *DB) upsertNodeReadiness(ctx context.Context, q queryable, p UpsertNodeReadinessParams) (*NodeReadiness, error) {
	if p.NodeSnapshotID == "" {
		return nil, fmt.Errorf("datastore: node_snapshot_id is required")
	}
	if p.OrganisationID == "" {
		return nil, fmt.Errorf("datastore: organisation_id is required")
	}
	if p.NodeName == "" {
		return nil, fmt.Errorf("datastore: node_name is required")
	}
	if p.TargetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target_chef_version is required")
	}

	query := `
		INSERT INTO node_readiness (
			node_snapshot_id, organisation_id, node_name,
			target_chef_version, is_ready, all_cookbooks_compatible,
			sufficient_disk_space, blocking_cookbooks, available_disk_mb,
			required_disk_mb, stale_data, evaluated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (node_snapshot_id, target_chef_version)
		DO UPDATE SET
			organisation_id         = EXCLUDED.organisation_id,
			node_name               = EXCLUDED.node_name,
			is_ready                = EXCLUDED.is_ready,
			all_cookbooks_compatible = EXCLUDED.all_cookbooks_compatible,
			sufficient_disk_space   = EXCLUDED.sufficient_disk_space,
			blocking_cookbooks      = EXCLUDED.blocking_cookbooks,
			available_disk_mb       = EXCLUDED.available_disk_mb,
			required_disk_mb        = EXCLUDED.required_disk_mb,
			stale_data              = EXCLUDED.stale_data,
			evaluated_at            = EXCLUDED.evaluated_at,
			updated_at              = now()
		RETURNING ` + nrColumns + `
	`

	r, err := scanNodeReadiness(q.QueryRowContext(ctx, query,
		p.NodeSnapshotID,
		p.OrganisationID,
		p.NodeName,
		p.TargetChefVersion,
		p.IsReady,
		p.AllCookbooksCompatible,
		nullBoolPtr(p.SufficientDiskSpace),
		nullJSON(p.BlockingCookbooks),
		nullIntPtr(p.AvailableDiskMB),
		nullIntPtr(p.RequiredDiskMB),
		p.StaleData,
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
// node snapshot. Called when a new collection run replaces the snapshot.
func (db *DB) DeleteNodeReadinessForSnapshot(ctx context.Context, nodeSnapshotID string) error {
	const query = `DELETE FROM node_readiness WHERE node_snapshot_id = $1`
	_, err := db.pool.ExecContext(ctx, query, nodeSnapshotID)
	if err != nil {
		return fmt.Errorf("datastore: deleting node readiness for snapshot %s: %w", nodeSnapshotID, err)
	}
	return nil
}

// DeleteNodeReadinessForOrganisation removes all readiness records for the
// given organisation. Forces a full re-evaluation on the next cycle.
func (db *DB) DeleteNodeReadinessForOrganisation(ctx context.Context, organisationID string) error {
	const query = `DELETE FROM node_readiness WHERE organisation_id = $1`
	_, err := db.pool.ExecContext(ctx, query, organisationID)
	if err != nil {
		return fmt.Errorf("datastore: deleting node readiness for organisation %s: %w", organisationID, err)
	}
	return nil
}

// DeleteNodeReadinessForOrganisationAndTarget removes all readiness records
// for the given organisation and target Chef version.
func (db *DB) DeleteNodeReadinessForOrganisationAndTarget(ctx context.Context, organisationID, targetChefVersion string) error {
	const query = `DELETE FROM node_readiness WHERE organisation_id = $1 AND target_chef_version = $2`
	_, err := db.pool.ExecContext(ctx, query, organisationID, targetChefVersion)
	if err != nil {
		return fmt.Errorf("datastore: deleting node readiness for organisation %s version %s: %w", organisationID, targetChefVersion, err)
	}
	return nil
}

// DeleteNodeReadiness removes a single readiness record by ID.
// Returns ErrNotFound if no such record exists.
func (db *DB) DeleteNodeReadiness(ctx context.Context, id string) error {
	const query = `DELETE FROM node_readiness WHERE id = $1`
	res, err := db.pool.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting node readiness %s: %w", id, err)
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
		&r.ID,
		&r.NodeSnapshotID,
		&r.OrganisationID,
		&r.NodeName,
		&r.TargetChefVersion,
		&r.IsReady,
		&r.AllCookbooksCompatible,
		&sufficientDisk,
		&blockingCookbooks,
		&availableDisk,
		&requiredDisk,
		&r.StaleData,
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
			&r.ID,
			&r.NodeSnapshotID,
			&r.OrganisationID,
			&r.NodeName,
			&r.TargetChefVersion,
			&r.IsReady,
			&r.AllCookbooksCompatible,
			&sufficientDisk,
			&blockingCookbooks,
			&availableDisk,
			&requiredDisk,
			&r.StaleData,
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
