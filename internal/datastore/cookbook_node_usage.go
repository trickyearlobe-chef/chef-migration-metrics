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

// CookbookNodeUsage represents a row in the cookbook_node_usage table. Each
// row records that a specific node snapshot was running a specific server
// cookbook at a specific version.
type CookbookNodeUsage struct {
	ID               string    `json:"id"`
	ServerCookbookID string    `json:"server_cookbook_id"`
	NodeSnapshotID   string    `json:"node_snapshot_id"`
	CookbookVersion  string    `json:"cookbook_version"`
	CreatedAt        time.Time `json:"created_at"`
}

// MarshalJSON implements json.Marshaler for CookbookNodeUsage.
func (u CookbookNodeUsage) MarshalJSON() ([]byte, error) {
	type Alias CookbookNodeUsage
	return json.Marshal((Alias)(u))
}

// ---------------------------------------------------------------------------
// Insert
// ---------------------------------------------------------------------------

// InsertCookbookNodeUsageParams holds the fields required to insert a single
// cookbook-node usage record.
type InsertCookbookNodeUsageParams struct {
	ServerCookbookID string
	NodeSnapshotID   string
	CookbookVersion  string
}

// InsertCookbookNodeUsage inserts a single cookbook-node usage record and
// returns the created row.
func (db *DB) InsertCookbookNodeUsage(ctx context.Context, p InsertCookbookNodeUsageParams) (CookbookNodeUsage, error) {
	return db.insertCookbookNodeUsage(ctx, db.q(), p)
}

func (db *DB) insertCookbookNodeUsage(ctx context.Context, q queryable, p InsertCookbookNodeUsageParams) (CookbookNodeUsage, error) {
	if err := validateUsageParams(p); err != nil {
		return CookbookNodeUsage{}, err
	}

	const query = `
		INSERT INTO cookbook_node_usage (server_cookbook_id, node_snapshot_id, cookbook_version)
		VALUES ($1, $2, $3)
		RETURNING id, server_cookbook_id, node_snapshot_id, cookbook_version, created_at
	`

	return scanCookbookNodeUsage(q.QueryRowContext(ctx, query,
		p.ServerCookbookID,
		p.NodeSnapshotID,
		p.CookbookVersion,
	))
}

// ---------------------------------------------------------------------------
// Bulk insert
// ---------------------------------------------------------------------------

// BulkInsertCookbookNodeUsage inserts multiple cookbook-node usage records
// within a single transaction for efficiency using multi-row INSERT
// statements. Returns the count of rows inserted. If any insert fails, the
// entire batch is rolled back.
func (db *DB) BulkInsertCookbookNodeUsage(ctx context.Context, params []InsertCookbookNodeUsageParams) (int, error) {
	if len(params) == 0 {
		return 0, nil
	}

	const batchSize = 2000 // 3 columns × 2000 = 6000 params, well under 65535
	const numCols = 3
	inserted := 0

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		for start := 0; start < len(params); start += batchSize {
			end := start + batchSize
			if end > len(params) {
				end = len(params)
			}
			batch := params[start:end]

			// Validate batch.
			for i, p := range batch {
				if err := validateUsageParams(p); err != nil {
					return fmt.Errorf("row %d: %w", start+i, err)
				}
			}

			// Build multi-row VALUES clause.
			var sb strings.Builder
			sb.WriteString(`INSERT INTO cookbook_node_usage (server_cookbook_id, node_snapshot_id, cookbook_version) VALUES `)

			args := make([]interface{}, 0, len(batch)*numCols)
			for i, p := range batch {
				if i > 0 {
					sb.WriteString(", ")
				}
				offset := i * numCols
				fmt.Fprintf(&sb, "($%d, $%d, $%d)", offset+1, offset+2, offset+3)
				args = append(args, p.ServerCookbookID, p.NodeSnapshotID, p.CookbookVersion)
			}

			result, err := tx.ExecContext(ctx, sb.String(), args...)
			if err != nil {
				return fmt.Errorf("datastore: batch inserting cookbook node usage (rows %d-%d): %w", start, end-1, err)
			}
			n, _ := result.RowsAffected()
			inserted += int(n)
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

// DeleteCookbookNodeUsageByCollectionRun removes all cookbook-node usage
// records associated with node snapshots from the given collection run.
// Returns the number of rows deleted.
func (db *DB) DeleteCookbookNodeUsageByCollectionRun(ctx context.Context, collectionRunID string) (int, error) {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM cookbook_node_usage
		 WHERE node_snapshot_id IN (
			SELECT id FROM node_snapshots WHERE collection_run_id = $1
		 )`,
		collectionRunID,
	)
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting cookbook node usage by collection run: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	return int(n), nil
}

// DeleteCookbookNodeUsageByServerCookbook removes all usage records for the
// given server cookbook. Returns the number of rows deleted.
func (db *DB) DeleteCookbookNodeUsageByServerCookbook(ctx context.Context, serverCookbookID string) (int, error) {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM cookbook_node_usage WHERE server_cookbook_id = $1`,
		serverCookbookID,
	)
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting cookbook node usage by server cookbook: %w", err)
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

// ListCookbookNodeUsageByServerCookbook returns all usage records for the
// given server cookbook, ordered by cookbook_version then node_snapshot_id.
func (db *DB) ListCookbookNodeUsageByServerCookbook(ctx context.Context, serverCookbookID string) ([]CookbookNodeUsage, error) {
	return db.listCookbookNodeUsageByServerCookbook(ctx, db.q(), serverCookbookID)
}

func (db *DB) listCookbookNodeUsageByServerCookbook(ctx context.Context, q queryable, serverCookbookID string) ([]CookbookNodeUsage, error) {
	const query = `
		SELECT id, server_cookbook_id, node_snapshot_id, cookbook_version, created_at
		FROM cookbook_node_usage
		WHERE server_cookbook_id = $1
		ORDER BY cookbook_version, node_snapshot_id
	`
	return scanCookbookNodeUsages(q.QueryContext(ctx, query, serverCookbookID))
}

// ListCookbookNodeUsageByNodeSnapshot returns all usage records for the
// given node snapshot, ordered by cookbook_version.
func (db *DB) ListCookbookNodeUsageByNodeSnapshot(ctx context.Context, nodeSnapshotID string) ([]CookbookNodeUsage, error) {
	return db.listCookbookNodeUsageByNodeSnapshot(ctx, db.q(), nodeSnapshotID)
}

func (db *DB) listCookbookNodeUsageByNodeSnapshot(ctx context.Context, q queryable, nodeSnapshotID string) ([]CookbookNodeUsage, error) {
	const query = `
		SELECT id, server_cookbook_id, node_snapshot_id, cookbook_version, created_at
		FROM cookbook_node_usage
		WHERE node_snapshot_id = $1
		ORDER BY cookbook_version
	`
	return scanCookbookNodeUsages(q.QueryContext(ctx, query, nodeSnapshotID))
}

// ListCookbookNodeUsageByCollectionRun returns all usage records for node
// snapshots from the given collection run, ordered by cookbook_id then
// node_snapshot_id.
func (db *DB) ListCookbookNodeUsageByCollectionRun(ctx context.Context, collectionRunID string) ([]CookbookNodeUsage, error) {
	return db.listCookbookNodeUsageByCollectionRun(ctx, db.q(), collectionRunID)
}

func (db *DB) listCookbookNodeUsageByCollectionRun(ctx context.Context, q queryable, collectionRunID string) ([]CookbookNodeUsage, error) {
	const query = `
		SELECT u.id, u.server_cookbook_id, u.node_snapshot_id, u.cookbook_version, u.created_at
		FROM cookbook_node_usage u
		INNER JOIN node_snapshots ns ON ns.id = u.node_snapshot_id
		WHERE ns.collection_run_id = $1
		ORDER BY u.server_cookbook_id, u.node_snapshot_id
	`
	return scanCookbookNodeUsages(q.QueryContext(ctx, query, collectionRunID))
}

// ---------------------------------------------------------------------------
// Aggregation queries
// ---------------------------------------------------------------------------

// CookbookUsageCount holds the result of a count-by-cookbook aggregation.
type CookbookUsageCount struct {
	ServerCookbookID string `json:"server_cookbook_id"`
	CookbookVersion  string `json:"cookbook_version"`
	NodeCount        int    `json:"node_count"`
}

// CountNodesByCookbook returns the number of distinct node snapshots using
// each cookbook version within the given collection run. Results are ordered
// by node count descending.
func (db *DB) CountNodesByCookbook(ctx context.Context, collectionRunID string) ([]CookbookUsageCount, error) {
	return db.countNodesByCookbook(ctx, db.q(), collectionRunID)
}

func (db *DB) countNodesByCookbook(ctx context.Context, q queryable, collectionRunID string) ([]CookbookUsageCount, error) {
	const query = `
		SELECT u.server_cookbook_id, u.cookbook_version, COUNT(DISTINCT u.node_snapshot_id) AS node_count
		FROM cookbook_node_usage u
		INNER JOIN node_snapshots ns ON ns.id = u.node_snapshot_id
		WHERE ns.collection_run_id = $1
		GROUP BY u.server_cookbook_id, u.cookbook_version
		ORDER BY node_count DESC, u.server_cookbook_id, u.cookbook_version
	`

	rows, err := q.QueryContext(ctx, query, collectionRunID)
	if err != nil {
		return nil, fmt.Errorf("datastore: counting nodes by cookbook: %w", err)
	}
	defer rows.Close()

	var counts []CookbookUsageCount
	for rows.Next() {
		var c CookbookUsageCount
		if err := rows.Scan(&c.ServerCookbookID, &c.CookbookVersion, &c.NodeCount); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookbook usage count: %w", err)
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookbook usage counts: %w", err)
	}
	return counts, nil
}

// CountNodesByCookbookName returns the number of distinct node snapshots
// using each version of a specific cookbook (by name) within the given
// organisation's most recent completed collection run.
func (db *DB) CountNodesByCookbookName(ctx context.Context, organisationID, cookbookName string) ([]CookbookUsageCount, error) {
	return db.countNodesByCookbookName(ctx, db.q(), organisationID, cookbookName)
}

func (db *DB) countNodesByCookbookName(ctx context.Context, q queryable, organisationID, cookbookName string) ([]CookbookUsageCount, error) {
	const query = `
		SELECT u.server_cookbook_id, u.cookbook_version, COUNT(DISTINCT u.node_snapshot_id) AS node_count
		FROM cookbook_node_usage u
		INNER JOIN node_snapshots ns ON ns.id = u.node_snapshot_id
		INNER JOIN collection_runs cr ON cr.id = ns.collection_run_id
		INNER JOIN server_cookbooks sc ON sc.id = u.server_cookbook_id
		WHERE cr.organisation_id = $1
		  AND cr.status = 'completed'
		  AND sc.name = $2
		  AND cr.started_at = (
			SELECT MAX(cr2.started_at)
			FROM collection_runs cr2
			WHERE cr2.organisation_id = $1 AND cr2.status = 'completed'
		  )
		GROUP BY u.server_cookbook_id, u.cookbook_version
		ORDER BY node_count DESC
	`

	rows, err := q.QueryContext(ctx, query, organisationID, cookbookName)
	if err != nil {
		return nil, fmt.Errorf("datastore: counting nodes by cookbook name: %w", err)
	}
	defer rows.Close()

	var counts []CookbookUsageCount
	for rows.Next() {
		var c CookbookUsageCount
		if err := rows.Scan(&c.ServerCookbookID, &c.CookbookVersion, &c.NodeCount); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookbook usage count: %w", err)
		}
		counts = append(counts, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookbook usage counts: %w", err)
	}
	return counts, nil
}

// ---------------------------------------------------------------------------
// Validation helper
// ---------------------------------------------------------------------------

func validateUsageParams(p InsertCookbookNodeUsageParams) error {
	if p.ServerCookbookID == "" {
		return fmt.Errorf("datastore: server cookbook ID is required for cookbook node usage")
	}
	if p.NodeSnapshotID == "" {
		return fmt.Errorf("datastore: node snapshot ID is required for cookbook node usage")
	}
	if p.CookbookVersion == "" {
		return fmt.Errorf("datastore: cookbook version is required for cookbook node usage")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanCookbookNodeUsage(row *sql.Row) (CookbookNodeUsage, error) {
	var u CookbookNodeUsage
	err := row.Scan(
		&u.ID,
		&u.ServerCookbookID,
		&u.NodeSnapshotID,
		&u.CookbookVersion,
		&u.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return CookbookNodeUsage{}, ErrNotFound
		}
		return CookbookNodeUsage{}, fmt.Errorf("datastore: scanning cookbook node usage: %w", err)
	}
	return u, nil
}

func scanCookbookNodeUsages(rows *sql.Rows, err error) ([]CookbookNodeUsage, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying cookbook node usage: %w", err)
	}
	defer rows.Close()

	var usages []CookbookNodeUsage
	for rows.Next() {
		var u CookbookNodeUsage
		if err := rows.Scan(
			&u.ID,
			&u.ServerCookbookID,
			&u.NodeSnapshotID,
			&u.CookbookVersion,
			&u.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookbook node usage row: %w", err)
		}
		usages = append(usages, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookbook node usage rows: %w", err)
	}
	return usages, nil
}
