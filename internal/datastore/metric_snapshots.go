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

// MetricSnapshot represents a row in the metric_snapshots table.
// Pre-aggregated data is stored in the Data field as JSON so that
// dashboard trend views avoid scanning the large node_snapshots table.
type MetricSnapshot struct {
	ID                string          `json:"id"`
	CollectionRunID   string          `json:"collection_run_id,omitempty"`
	OrganisationID    string          `json:"organisation_id"`
	SnapshotType      string          `json:"snapshot_type"`
	TargetChefVersion string          `json:"target_chef_version,omitempty"`
	Data              json.RawMessage `json:"data"`
	SnapshotAt        time.Time       `json:"snapshot_at"`
	CreatedAt         time.Time       `json:"created_at"`
}

// InsertMetricSnapshotParams holds the fields required to insert a metric snapshot.
type InsertMetricSnapshotParams struct {
	CollectionRunID   string
	OrganisationID    string
	SnapshotType      string
	TargetChefVersion string
	Data              json.RawMessage
	SnapshotAt        time.Time
}

// ---------------------------------------------------------------------------
// Insert
// ---------------------------------------------------------------------------

// InsertMetricSnapshot inserts a new metric snapshot and returns the created row.
func (db *DB) InsertMetricSnapshot(ctx context.Context, p InsertMetricSnapshotParams) (MetricSnapshot, error) {
	return db.insertMetricSnapshot(ctx, db.q(), p)
}

func (db *DB) insertMetricSnapshot(ctx context.Context, q queryable, p InsertMetricSnapshotParams) (MetricSnapshot, error) {
	if p.OrganisationID == "" {
		return MetricSnapshot{}, fmt.Errorf("datastore: organisation ID is required to insert a metric snapshot")
	}
	if p.SnapshotType == "" {
		return MetricSnapshot{}, fmt.Errorf("datastore: snapshot type is required to insert a metric snapshot")
	}
	if len(p.Data) == 0 {
		return MetricSnapshot{}, fmt.Errorf("datastore: data is required to insert a metric snapshot")
	}
	if p.SnapshotAt.IsZero() {
		p.SnapshotAt = time.Now().UTC()
	}

	const query = `
		INSERT INTO metric_snapshots (
			collection_run_id, organisation_id, snapshot_type,
			target_chef_version, data, snapshot_at
		) VALUES (
			$1, $2, $3, $4, $5, $6
		)
		RETURNING id, collection_run_id, organisation_id, snapshot_type,
		          target_chef_version, data, snapshot_at, created_at
	`

	return scanMetricSnapshot(q.QueryRowContext(ctx, query,
		nullString(p.CollectionRunID),
		p.OrganisationID,
		p.SnapshotType,
		nullString(p.TargetChefVersion),
		[]byte(p.Data),
		p.SnapshotAt,
	))
}

// ---------------------------------------------------------------------------
// Query methods
// ---------------------------------------------------------------------------

// ListMetricSnapshotsByOrganisation returns metric snapshots for the given
// organisation filtered by snapshot_type, ordered by snapshot_at DESC.
// If limit > 0, the result set is capped to that many rows. This is used
// for trend charts.
func (db *DB) ListMetricSnapshotsByOrganisation(ctx context.Context, organisationID, snapshotType string, limit int) ([]MetricSnapshot, error) {
	return db.listMetricSnapshotsByOrganisation(ctx, db.q(), organisationID, snapshotType, limit)
}

func (db *DB) listMetricSnapshotsByOrganisation(ctx context.Context, q queryable, organisationID, snapshotType string, limit int) ([]MetricSnapshot, error) {
	query := `
		SELECT id, collection_run_id, organisation_id, snapshot_type,
		       target_chef_version, data, snapshot_at, created_at
		FROM metric_snapshots
		WHERE organisation_id = $1
		  AND snapshot_type = $2
		ORDER BY snapshot_at DESC
	`
	args := []any{organisationID, snapshotType}

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	return scanMetricSnapshots(q.QueryContext(ctx, query, args...))
}

// ListMetricSnapshotsByOrganisationAndVersion returns metric snapshots for
// the given organisation filtered by snapshot_type and target_chef_version,
// ordered by snapshot_at DESC. If limit > 0, the result set is capped.
func (db *DB) ListMetricSnapshotsByOrganisationAndVersion(ctx context.Context, organisationID, snapshotType, targetChefVersion string, limit int) ([]MetricSnapshot, error) {
	return db.listMetricSnapshotsByOrganisationAndVersion(ctx, db.q(), organisationID, snapshotType, targetChefVersion, limit)
}

func (db *DB) listMetricSnapshotsByOrganisationAndVersion(ctx context.Context, q queryable, organisationID, snapshotType, targetChefVersion string, limit int) ([]MetricSnapshot, error) {
	query := `
		SELECT id, collection_run_id, organisation_id, snapshot_type,
		       target_chef_version, data, snapshot_at, created_at
		FROM metric_snapshots
		WHERE organisation_id = $1
		  AND snapshot_type = $2
		  AND target_chef_version = $3
		ORDER BY snapshot_at DESC
	`
	args := []any{organisationID, snapshotType, targetChefVersion}

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	return scanMetricSnapshots(q.QueryContext(ctx, query, args...))
}

// ---------------------------------------------------------------------------
// Retention
// ---------------------------------------------------------------------------

// PurgeMetricSnapshotsOlderThan deletes metric snapshots with snapshot_at
// before the given cutoff time. Returns the number of rows deleted.
func (db *DB) PurgeMetricSnapshotsOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	const query = `DELETE FROM metric_snapshots WHERE snapshot_at < $1`
	res, err := db.pool.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("datastore: purging metric snapshots older than %s: %w", cutoff.Format(time.RFC3339), err)
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

func scanMetricSnapshot(row *sql.Row) (MetricSnapshot, error) {
	var ms MetricSnapshot
	var collectionRunID, targetChefVersion sql.NullString
	var data []byte

	err := row.Scan(
		&ms.ID,
		&collectionRunID,
		&ms.OrganisationID,
		&ms.SnapshotType,
		&targetChefVersion,
		&data,
		&ms.SnapshotAt,
		&ms.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return MetricSnapshot{}, ErrNotFound
		}
		return MetricSnapshot{}, fmt.Errorf("datastore: scanning metric snapshot: %w", err)
	}

	ms.CollectionRunID = stringFromNull(collectionRunID)
	ms.TargetChefVersion = stringFromNull(targetChefVersion)
	ms.Data = jsonFromNullBytes(data)
	return ms, nil
}

func scanMetricSnapshots(rows *sql.Rows, err error) ([]MetricSnapshot, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying metric snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []MetricSnapshot
	for rows.Next() {
		var ms MetricSnapshot
		var collectionRunID, targetChefVersion sql.NullString
		var data []byte

		if err := rows.Scan(
			&ms.ID,
			&collectionRunID,
			&ms.OrganisationID,
			&ms.SnapshotType,
			&targetChefVersion,
			&data,
			&ms.SnapshotAt,
			&ms.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning metric snapshot row: %w", err)
		}

		ms.CollectionRunID = stringFromNull(collectionRunID)
		ms.TargetChefVersion = stringFromNull(targetChefVersion)
		ms.Data = jsonFromNullBytes(data)
		snapshots = append(snapshots, ms)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating metric snapshot rows: %w", err)
	}
	return snapshots, nil
}
