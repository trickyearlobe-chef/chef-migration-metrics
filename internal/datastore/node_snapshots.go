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

// NodeSnapshot represents a row in the node_snapshots table. Each snapshot
// captures the state of a single node at the time of a collection run.
type NodeSnapshot struct {
	ID              string          `json:"id"`
	CollectionRunID string          `json:"collection_run_id"`
	OrganisationID  string          `json:"organisation_id"`
	NodeName        string          `json:"node_name"`
	ChefEnvironment string          `json:"chef_environment,omitempty"`
	ChefVersion     string          `json:"chef_version,omitempty"`
	Platform        string          `json:"platform,omitempty"`
	PlatformVersion string          `json:"platform_version,omitempty"`
	PlatformFamily  string          `json:"platform_family,omitempty"`
	Filesystem      json.RawMessage `json:"filesystem,omitempty"`
	Cookbooks       json.RawMessage `json:"cookbooks,omitempty"`
	RunList         json.RawMessage `json:"run_list,omitempty"`
	Roles           json.RawMessage `json:"roles,omitempty"`
	PolicyName      string          `json:"policy_name,omitempty"`
	PolicyGroup     string          `json:"policy_group,omitempty"`
	OhaiTime        float64         `json:"ohai_time,omitempty"`
	IsStale         bool            `json:"is_stale"`
	CollectedAt     time.Time       `json:"collected_at"`
	CreatedAt       time.Time       `json:"created_at"`
}

// IsPolicyfileNode returns true if the node is managed by Policyfiles
// (both policy_name and policy_group are set).
func (ns NodeSnapshot) IsPolicyfileNode() bool {
	return ns.PolicyName != "" && ns.PolicyGroup != ""
}

// MarshalJSON implements json.Marshaler for NodeSnapshot.
func (ns NodeSnapshot) MarshalJSON() ([]byte, error) {
	type Alias NodeSnapshot
	return json.Marshal((Alias)(ns))
}

// ---------------------------------------------------------------------------
// Insert
// ---------------------------------------------------------------------------

// InsertNodeSnapshotParams holds the fields required to insert a single
// node snapshot.
type InsertNodeSnapshotParams struct {
	CollectionRunID string
	OrganisationID  string
	NodeName        string
	ChefEnvironment string
	ChefVersion     string
	Platform        string
	PlatformVersion string
	PlatformFamily  string
	Filesystem      json.RawMessage // raw JSON from Chef API
	Cookbooks       json.RawMessage // raw JSON from Chef API
	RunList         json.RawMessage // raw JSON from Chef API
	Roles           json.RawMessage // raw JSON from Chef API
	PolicyName      string
	PolicyGroup     string
	OhaiTime        float64
	IsStale         bool
	CollectedAt     time.Time
}

// InsertNodeSnapshot inserts a single node snapshot and returns the created
// row.
func (db *DB) InsertNodeSnapshot(ctx context.Context, p InsertNodeSnapshotParams) (NodeSnapshot, error) {
	return db.insertNodeSnapshot(ctx, db.q(), p)
}

func (db *DB) insertNodeSnapshot(ctx context.Context, q queryable, p InsertNodeSnapshotParams) (NodeSnapshot, error) {
	if p.CollectionRunID == "" {
		return NodeSnapshot{}, fmt.Errorf("datastore: collection run ID is required to insert a node snapshot")
	}
	if p.OrganisationID == "" {
		return NodeSnapshot{}, fmt.Errorf("datastore: organisation ID is required to insert a node snapshot")
	}
	if p.NodeName == "" {
		return NodeSnapshot{}, fmt.Errorf("datastore: node name is required to insert a node snapshot")
	}
	if p.CollectedAt.IsZero() {
		p.CollectedAt = time.Now().UTC()
	}

	const query = `
		INSERT INTO node_snapshots (
			collection_run_id, organisation_id, node_name,
			chef_environment, chef_version, platform, platform_version,
			platform_family, filesystem, cookbooks, run_list, roles,
			policy_name, policy_group, ohai_time, is_stale, collected_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17
		)
		RETURNING id, collection_run_id, organisation_id, node_name,
		          chef_environment, chef_version, platform, platform_version,
		          platform_family, filesystem, cookbooks, run_list, roles,
		          policy_name, policy_group, ohai_time, is_stale,
		          collected_at, created_at
	`

	return scanNodeSnapshot(q.QueryRowContext(ctx, query,
		p.CollectionRunID,
		p.OrganisationID,
		p.NodeName,
		nullString(p.ChefEnvironment),
		nullString(p.ChefVersion),
		nullString(p.Platform),
		nullString(p.PlatformVersion),
		nullString(p.PlatformFamily),
		nullJSON(p.Filesystem),
		nullJSON(p.Cookbooks),
		nullJSON(p.RunList),
		nullJSON(p.Roles),
		nullString(p.PolicyName),
		nullString(p.PolicyGroup),
		nullFloat(p.OhaiTime),
		p.IsStale,
		p.CollectedAt,
	))
}

// ---------------------------------------------------------------------------
// Bulk insert
// ---------------------------------------------------------------------------

// BulkInsertNodeSnapshots inserts multiple node snapshots within a single
// transaction for efficiency. It returns the count of rows inserted. If any
// insert fails, the entire batch is rolled back.
func (db *DB) BulkInsertNodeSnapshots(ctx context.Context, params []InsertNodeSnapshotParams) (int, error) {
	_, count, err := db.bulkInsertNodeSnapshots(ctx, params, false)
	return count, err
}

// BulkInsertNodeSnapshotsReturningIDs inserts multiple node snapshots within
// a single transaction and returns a map of node name → generated snapshot
// UUID alongside the inserted count. This is used by the collector to
// correlate node snapshots with their cookbook usage records without a
// separate lookup query.
//
// If a node name appears more than once in params, the map will contain the
// ID of the last inserted row for that name.
func (db *DB) BulkInsertNodeSnapshotsReturningIDs(ctx context.Context, params []InsertNodeSnapshotParams) (map[string]string, int, error) {
	return db.bulkInsertNodeSnapshots(ctx, params, true)
}

// bulkInsertNodeSnapshots is the shared implementation for both
// BulkInsertNodeSnapshots and BulkInsertNodeSnapshotsReturningIDs.
// When returnIDs is true, the INSERT uses RETURNING id and populates
// the returned map. When false, the map is nil.
func (db *DB) bulkInsertNodeSnapshots(ctx context.Context, params []InsertNodeSnapshotParams, returnIDs bool) (map[string]string, int, error) {
	if len(params) == 0 {
		return nil, 0, nil
	}

	var idMap map[string]string
	if returnIDs {
		idMap = make(map[string]string, len(params))
	}

	inserted := 0
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		var queryStr string
		if returnIDs {
			queryStr = `
				INSERT INTO node_snapshots (
					collection_run_id, organisation_id, node_name,
					chef_environment, chef_version, platform, platform_version,
					platform_family, filesystem, cookbooks, run_list, roles,
					policy_name, policy_group, ohai_time, is_stale, collected_at
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
					$13, $14, $15, $16, $17
				)
				RETURNING id
			`
		} else {
			queryStr = `
				INSERT INTO node_snapshots (
					collection_run_id, organisation_id, node_name,
					chef_environment, chef_version, platform, platform_version,
					platform_family, filesystem, cookbooks, run_list, roles,
					policy_name, policy_group, ohai_time, is_stale, collected_at
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
					$13, $14, $15, $16, $17
				)
			`
		}

		stmt, err := tx.PrepareContext(ctx, queryStr)
		if err != nil {
			return fmt.Errorf("datastore: preparing node snapshot insert: %w", err)
		}
		defer stmt.Close()

		for i, p := range params {
			if p.CollectionRunID == "" {
				return fmt.Errorf("datastore: collection run ID is required (row %d)", i)
			}
			if p.OrganisationID == "" {
				return fmt.Errorf("datastore: organisation ID is required (row %d)", i)
			}
			if p.NodeName == "" {
				return fmt.Errorf("datastore: node name is required (row %d)", i)
			}
			if p.CollectedAt.IsZero() {
				p.CollectedAt = time.Now().UTC()
			}

			args := []interface{}{
				p.CollectionRunID,
				p.OrganisationID,
				p.NodeName,
				nullString(p.ChefEnvironment),
				nullString(p.ChefVersion),
				nullString(p.Platform),
				nullString(p.PlatformVersion),
				nullString(p.PlatformFamily),
				nullJSON(p.Filesystem),
				nullJSON(p.Cookbooks),
				nullJSON(p.RunList),
				nullJSON(p.Roles),
				nullString(p.PolicyName),
				nullString(p.PolicyGroup),
				nullFloat(p.OhaiTime),
				p.IsStale,
				p.CollectedAt,
			}

			if returnIDs {
				var snapshotID string
				if err := stmt.QueryRowContext(ctx, args...).Scan(&snapshotID); err != nil {
					return fmt.Errorf("datastore: inserting node snapshot %q (row %d): %w", p.NodeName, i, err)
				}
				idMap[p.NodeName] = snapshotID
			} else {
				if _, err := stmt.ExecContext(ctx, args...); err != nil {
					return fmt.Errorf("datastore: inserting node snapshot %q (row %d): %w", p.NodeName, i, err)
				}
			}
			inserted++
		}

		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return idMap, inserted, nil
}

// ---------------------------------------------------------------------------
// Query methods
// ---------------------------------------------------------------------------

// GetNodeSnapshot returns the node snapshot with the given UUID. Returns
// ErrNotFound if no such snapshot exists.
func (db *DB) GetNodeSnapshot(ctx context.Context, id string) (NodeSnapshot, error) {
	return db.getNodeSnapshot(ctx, db.q(), id)
}

func (db *DB) getNodeSnapshot(ctx context.Context, q queryable, id string) (NodeSnapshot, error) {
	const query = `
		SELECT id, collection_run_id, organisation_id, node_name,
		       chef_environment, chef_version, platform, platform_version,
		       platform_family, filesystem, cookbooks, run_list, roles,
		       policy_name, policy_group, ohai_time, is_stale,
		       collected_at, created_at
		FROM node_snapshots
		WHERE id = $1
	`
	return scanNodeSnapshot(q.QueryRowContext(ctx, query, id))
}

// ListNodeSnapshotsByCollectionRun returns all node snapshots for the given
// collection run, ordered by node name.
func (db *DB) ListNodeSnapshotsByCollectionRun(ctx context.Context, collectionRunID string) ([]NodeSnapshot, error) {
	return db.listNodeSnapshotsByCollectionRun(ctx, db.q(), collectionRunID)
}

func (db *DB) listNodeSnapshotsByCollectionRun(ctx context.Context, q queryable, collectionRunID string) ([]NodeSnapshot, error) {
	const query = `
		SELECT id, collection_run_id, organisation_id, node_name,
		       chef_environment, chef_version, platform, platform_version,
		       platform_family, filesystem, cookbooks, run_list, roles,
		       policy_name, policy_group, ohai_time, is_stale,
		       collected_at, created_at
		FROM node_snapshots
		WHERE collection_run_id = $1
		ORDER BY node_name
	`
	return scanNodeSnapshots(q.QueryContext(ctx, query, collectionRunID))
}

// ListNodeSnapshotsByOrganisation returns all node snapshots for the given
// organisation from the most recent completed collection run. This gives the
// current picture of the fleet for that org. Returns an empty slice if no
// completed collection run exists.
func (db *DB) ListNodeSnapshotsByOrganisation(ctx context.Context, organisationID string) ([]NodeSnapshot, error) {
	return db.listNodeSnapshotsByOrganisation(ctx, db.q(), organisationID)
}

func (db *DB) listNodeSnapshotsByOrganisation(ctx context.Context, q queryable, organisationID string) ([]NodeSnapshot, error) {
	const query = `
		SELECT ns.id, ns.collection_run_id, ns.organisation_id, ns.node_name,
		       ns.chef_environment, ns.chef_version, ns.platform, ns.platform_version,
		       ns.platform_family, ns.filesystem, ns.cookbooks, ns.run_list, ns.roles,
		       ns.policy_name, ns.policy_group, ns.ohai_time, ns.is_stale,
		       ns.collected_at, ns.created_at
		FROM node_snapshots ns
		INNER JOIN collection_runs cr ON cr.id = ns.collection_run_id
		WHERE ns.organisation_id = $1
		  AND cr.status = 'completed'
		  AND cr.started_at = (
			SELECT MAX(cr2.started_at)
			FROM collection_runs cr2
			WHERE cr2.organisation_id = $1 AND cr2.status = 'completed'
		  )
		ORDER BY ns.node_name
	`
	return scanNodeSnapshots(q.QueryContext(ctx, query, organisationID))
}

// GetNodeSnapshotByName returns the most recent snapshot for a node with the
// given name in the given organisation. Returns ErrNotFound if no snapshot
// exists for that node.
func (db *DB) GetNodeSnapshotByName(ctx context.Context, organisationID, nodeName string) (NodeSnapshot, error) {
	return db.getNodeSnapshotByName(ctx, db.q(), organisationID, nodeName)
}

func (db *DB) getNodeSnapshotByName(ctx context.Context, q queryable, organisationID, nodeName string) (NodeSnapshot, error) {
	const query = `
		SELECT id, collection_run_id, organisation_id, node_name,
		       chef_environment, chef_version, platform, platform_version,
		       platform_family, filesystem, cookbooks, run_list, roles,
		       policy_name, policy_group, ohai_time, is_stale,
		       collected_at, created_at
		FROM node_snapshots
		WHERE organisation_id = $1 AND node_name = $2
		ORDER BY collected_at DESC
		LIMIT 1
	`
	return scanNodeSnapshot(q.QueryRowContext(ctx, query, organisationID, nodeName))
}

// CountNodeSnapshotsByCollectionRun returns the number of node snapshots
// associated with the given collection run.
func (db *DB) CountNodeSnapshotsByCollectionRun(ctx context.Context, collectionRunID string) (int, error) {
	return db.countNodeSnapshotsByCollectionRun(ctx, db.q(), collectionRunID)
}

func (db *DB) countNodeSnapshotsByCollectionRun(ctx context.Context, q queryable, collectionRunID string) (int, error) {
	var count int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM node_snapshots WHERE collection_run_id = $1`,
		collectionRunID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("datastore: counting node snapshots: %w", err)
	}
	return count, nil
}

// CountStaleNodesByCollectionRun returns the number of stale node snapshots
// in the given collection run.
func (db *DB) CountStaleNodesByCollectionRun(ctx context.Context, collectionRunID string) (int, error) {
	return db.countStaleNodesByCollectionRun(ctx, db.q(), collectionRunID)
}

func (db *DB) countStaleNodesByCollectionRun(ctx context.Context, q queryable, collectionRunID string) (int, error) {
	var count int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM node_snapshots WHERE collection_run_id = $1 AND is_stale = TRUE`,
		collectionRunID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("datastore: counting stale node snapshots: %w", err)
	}
	return count, nil
}

// DeleteNodeSnapshotsByCollectionRun removes all node snapshots for the
// given collection run. Returns the number of rows deleted.
func (db *DB) DeleteNodeSnapshotsByCollectionRun(ctx context.Context, collectionRunID string) (int, error) {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM node_snapshots WHERE collection_run_id = $1`,
		collectionRunID,
	)
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting node snapshots: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	return int(n), nil
}

// ---------------------------------------------------------------------------
// JSON helper
// ---------------------------------------------------------------------------

// nullJSON returns nil (SQL NULL) for empty or nil JSON, or the raw bytes
// otherwise. This prevents inserting empty strings as JSONB values.
func nullJSON(data json.RawMessage) interface{} {
	if len(data) == 0 {
		return nil
	}
	return []byte(data)
}

// jsonFromNullBytes converts a nullable byte slice from the database back
// to json.RawMessage. NULL becomes nil.
func jsonFromNullBytes(data []byte) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	return json.RawMessage(data)
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanNodeSnapshot(row *sql.Row) (NodeSnapshot, error) {
	var ns NodeSnapshot
	var chefEnv, chefVer, platform, platformVer, platformFam sql.NullString
	var policyName, policyGroup sql.NullString
	var ohaiTime sql.NullFloat64
	var filesystem, cookbooks, runList, roles []byte

	err := row.Scan(
		&ns.ID,
		&ns.CollectionRunID,
		&ns.OrganisationID,
		&ns.NodeName,
		&chefEnv,
		&chefVer,
		&platform,
		&platformVer,
		&platformFam,
		&filesystem,
		&cookbooks,
		&runList,
		&roles,
		&policyName,
		&policyGroup,
		&ohaiTime,
		&ns.IsStale,
		&ns.CollectedAt,
		&ns.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return NodeSnapshot{}, ErrNotFound
		}
		return NodeSnapshot{}, fmt.Errorf("datastore: scanning node snapshot: %w", err)
	}

	ns.ChefEnvironment = stringFromNull(chefEnv)
	ns.ChefVersion = stringFromNull(chefVer)
	ns.Platform = stringFromNull(platform)
	ns.PlatformVersion = stringFromNull(platformVer)
	ns.PlatformFamily = stringFromNull(platformFam)
	ns.PolicyName = stringFromNull(policyName)
	ns.PolicyGroup = stringFromNull(policyGroup)
	ns.OhaiTime = floatFromNull(ohaiTime)
	ns.Filesystem = jsonFromNullBytes(filesystem)
	ns.Cookbooks = jsonFromNullBytes(cookbooks)
	ns.RunList = jsonFromNullBytes(runList)
	ns.Roles = jsonFromNullBytes(roles)
	return ns, nil
}

func scanNodeSnapshots(rows *sql.Rows, err error) ([]NodeSnapshot, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying node snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []NodeSnapshot
	for rows.Next() {
		var ns NodeSnapshot
		var chefEnv, chefVer, platform, platformVer, platformFam sql.NullString
		var policyName, policyGroup sql.NullString
		var ohaiTime sql.NullFloat64
		var filesystem, cookbooks, runList, roles []byte

		if err := rows.Scan(
			&ns.ID,
			&ns.CollectionRunID,
			&ns.OrganisationID,
			&ns.NodeName,
			&chefEnv,
			&chefVer,
			&platform,
			&platformVer,
			&platformFam,
			&filesystem,
			&cookbooks,
			&runList,
			&roles,
			&policyName,
			&policyGroup,
			&ohaiTime,
			&ns.IsStale,
			&ns.CollectedAt,
			&ns.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning node snapshot row: %w", err)
		}

		ns.ChefEnvironment = stringFromNull(chefEnv)
		ns.ChefVersion = stringFromNull(chefVer)
		ns.Platform = stringFromNull(platform)
		ns.PlatformVersion = stringFromNull(platformVer)
		ns.PlatformFamily = stringFromNull(platformFam)
		ns.PolicyName = stringFromNull(policyName)
		ns.PolicyGroup = stringFromNull(policyGroup)
		ns.OhaiTime = floatFromNull(ohaiTime)
		ns.Filesystem = jsonFromNullBytes(filesystem)
		ns.Cookbooks = jsonFromNullBytes(cookbooks)
		ns.RunList = jsonFromNullBytes(runList)
		ns.Roles = jsonFromNullBytes(roles)
		snapshots = append(snapshots, ns)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating node snapshot rows: %w", err)
	}
	return snapshots, nil
}
