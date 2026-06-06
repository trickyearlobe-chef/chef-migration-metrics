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

	"github.com/lib/pq"
)

// NodeSnapshot represents a row in the node_snapshots table. Each snapshot
// captures the state of a single node at the time of a collection run.
// The primary key is (organisation_name, node_name).
type NodeSnapshot struct {
	CollectionRunOrg string          `json:"collection_run_org,omitempty"`
	OrganisationName string          `json:"organisation_name"`
	NodeName         string          `json:"node_name"`
	ChefEnvironment  string          `json:"chef_environment,omitempty"`
	ChefVersion      string          `json:"chef_version,omitempty"`
	Platform         string          `json:"platform,omitempty"`
	PlatformVersion  string          `json:"platform_version,omitempty"`
	PlatformFamily   string          `json:"platform_family,omitempty"`
	PlatformCaption  string          `json:"platform_caption,omitempty"`
	Filesystem       json.RawMessage `json:"filesystem,omitempty"`
	Cookbooks        json.RawMessage `json:"cookbooks,omitempty"`
	RunList          json.RawMessage `json:"run_list,omitempty"`
	Roles            json.RawMessage `json:"roles,omitempty"`
	PolicyName       string          `json:"policy_name,omitempty"`
	PolicyGroup      string          `json:"policy_group,omitempty"`
	OhaiTime         float64         `json:"ohai_time,omitempty"`
	CustomAttributes json.RawMessage `json:"custom_attributes,omitempty"`
	IsStale          bool            `json:"is_stale"`
	CollectedAt      time.Time       `json:"collected_at"`
	CreatedAt        time.Time       `json:"created_at"`

	// Parallel deployment tracking (nullable — absent when migration cookbook not deployed)
	MigrationState       string `json:"migration_state,omitempty"`
	ActiveChefVersion    string `json:"active_chef_version,omitempty"`
	DormantInstalled     *bool  `json:"dormant_installed,omitempty"`
	DormantChefVersion   string `json:"dormant_chef_version,omitempty"`
	TargetVersion        string `json:"target_version,omitempty"`
	TargetExecutionTime  string `json:"target_execution_time,omitempty"`
	TargetConvergeStatus string `json:"target_converge_status,omitempty"`
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
	CollectionRunOrg string
	OrganisationName string
	NodeName         string
	ChefEnvironment  string
	ChefVersion      string
	Platform         string
	PlatformVersion  string
	PlatformFamily   string
	PlatformCaption  string
	Filesystem       json.RawMessage // raw JSON from Chef API
	Cookbooks        json.RawMessage // raw JSON from Chef API
	RunList          json.RawMessage // raw JSON from Chef API
	Roles            json.RawMessage // raw JSON from Chef API
	PolicyName       string
	PolicyGroup      string
	OhaiTime         float64
	CustomAttributes json.RawMessage // raw JSON — flat map keyed by dot-separated attribute path
	IsStale          bool
	CollectedAt      time.Time

	// Parallel deployment tracking
	MigrationState       string
	ActiveChefVersion    string
	DormantInstalled     *bool
	DormantChefVersion   string
	TargetVersion        string
	TargetExecutionTime  string
	TargetConvergeStatus string
}

// UpsertNodeSnapshot inserts a node snapshot or updates the existing row for
// the same (organisation_name, node_name) combination. Returns the resulting row.
func (db *DB) UpsertNodeSnapshot(ctx context.Context, p InsertNodeSnapshotParams) (NodeSnapshot, error) {
	return db.upsertNodeSnapshot(ctx, db.q(), p)
}

func (db *DB) upsertNodeSnapshot(ctx context.Context, q queryable, p InsertNodeSnapshotParams) (NodeSnapshot, error) {
	if p.CollectionRunOrg == "" {
		return NodeSnapshot{}, fmt.Errorf("datastore: collection run org is required to insert a node snapshot")
	}
	if p.OrganisationName == "" {
		return NodeSnapshot{}, fmt.Errorf("datastore: organisation name is required to insert a node snapshot")
	}
	if p.NodeName == "" {
		return NodeSnapshot{}, fmt.Errorf("datastore: node name is required to insert a node snapshot")
	}
	if p.CollectedAt.IsZero() {
		p.CollectedAt = time.Now().UTC()
	}

	const query = `
		INSERT INTO node_snapshots (
			collection_run_org, organisation_name, node_name,
			chef_environment, chef_version, platform, platform_version,
			platform_family, platform_caption, filesystem, cookbooks, run_list, roles,
			policy_name, policy_group, ohai_time, custom_attributes,
			is_stale, collected_at,
			migration_state, active_chef_version, dormant_installed,
			dormant_chef_version, target_version, target_execution_time,
			target_converge_status
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18, $19,
			$20, $21, $22, $23, $24, $25, $26
		)
		ON CONFLICT (organisation_name, node_name) DO UPDATE SET
			collection_run_org = EXCLUDED.collection_run_org,
			chef_environment   = EXCLUDED.chef_environment,
			chef_version       = EXCLUDED.chef_version,
			platform           = EXCLUDED.platform,
			platform_version   = EXCLUDED.platform_version,
			platform_family    = EXCLUDED.platform_family,
			platform_caption   = EXCLUDED.platform_caption,
			filesystem         = EXCLUDED.filesystem,
			cookbooks          = EXCLUDED.cookbooks,
			run_list           = EXCLUDED.run_list,
			roles              = EXCLUDED.roles,
			policy_name        = EXCLUDED.policy_name,
			policy_group       = EXCLUDED.policy_group,
			ohai_time          = EXCLUDED.ohai_time,
			custom_attributes  = EXCLUDED.custom_attributes,
			is_stale           = EXCLUDED.is_stale,
			collected_at       = EXCLUDED.collected_at,
			migration_state       = EXCLUDED.migration_state,
			active_chef_version   = EXCLUDED.active_chef_version,
			dormant_installed     = EXCLUDED.dormant_installed,
			dormant_chef_version  = EXCLUDED.dormant_chef_version,
			target_version        = EXCLUDED.target_version,
			target_execution_time = EXCLUDED.target_execution_time,
			target_converge_status = EXCLUDED.target_converge_status
		RETURNING collection_run_org, organisation_name, node_name,
		          chef_environment, chef_version, platform, platform_version,
		          platform_family, platform_caption, filesystem, cookbooks, run_list, roles,
		          policy_name, policy_group, ohai_time, custom_attributes,
		          is_stale, collected_at, created_at,
		          migration_state, active_chef_version, dormant_installed,
		          dormant_chef_version, target_version, target_execution_time,
		          target_converge_status
	`

	return scanNodeSnapshot(q.QueryRowContext(ctx, query,
		p.CollectionRunOrg,
		p.OrganisationName,
		p.NodeName,
		nullString(p.ChefEnvironment),
		nullString(p.ChefVersion),
		nullString(p.Platform),
		nullString(p.PlatformVersion),
		nullString(p.PlatformFamily),
		nullString(p.PlatformCaption),
		nullJSON(p.Filesystem),
		nullJSON(p.Cookbooks),
		nullJSON(p.RunList),
		nullJSON(p.Roles),
		nullString(p.PolicyName),
		nullString(p.PolicyGroup),
		nullFloat(p.OhaiTime),
		nullJSON(p.CustomAttributes),
		p.IsStale,
		p.CollectedAt,
		nullString(p.MigrationState),
		nullString(p.ActiveChefVersion),
		nullBool(p.DormantInstalled),
		nullString(p.DormantChefVersion),
		nullString(p.TargetVersion),
		nullString(p.TargetExecutionTime),
		nullString(p.TargetConvergeStatus),
	))
}

// ---------------------------------------------------------------------------
// Bulk insert
// ---------------------------------------------------------------------------

// BulkUpsertNodeSnapshots upserts multiple node snapshots within a single
// transaction. Existing rows for the same (organisation_name, node_name) are
// updated in place. Returns the count of rows affected (inserted or updated).
// If any upsert fails, the entire batch is rolled back.
func (db *DB) BulkUpsertNodeSnapshots(ctx context.Context, params []InsertNodeSnapshotParams) (int, error) {
	_, count, err := db.bulkUpsertNodeSnapshots(ctx, params, false)
	return count, err
}

// bulkUpsertNodeSnapshots is the implementation for BulkUpsertNodeSnapshots.
// The returnKeys parameter controls whether the query uses RETURNING to
// populate the returned map. When false (the normal path), the map is nil.
func (db *DB) bulkUpsertNodeSnapshots(ctx context.Context, params []InsertNodeSnapshotParams, returnKeys bool) (map[string]string, int, error) {
	if len(params) == 0 {
		return nil, 0, nil
	}

	var keyMap map[string]string
	if returnKeys {
		keyMap = make(map[string]string, len(params))
	}

	const batchSize = 500
	const numCols = 26
	inserted := 0

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		for start := 0; start < len(params); start += batchSize {
			end := start + batchSize
			if end > len(params) {
				end = len(params)
			}
			batch := params[start:end]

			// Validate the batch.
			for i, p := range batch {
				idx := start + i
				if p.CollectionRunOrg == "" {
					return fmt.Errorf("datastore: collection run org is required (row %d)", idx)
				}
				if p.OrganisationName == "" {
					return fmt.Errorf("datastore: organisation name is required (row %d)", idx)
				}
				if p.NodeName == "" {
					return fmt.Errorf("datastore: node name is required (row %d)", idx)
				}
			}

			// Build multi-row VALUES clause.
			var sb strings.Builder
			sb.WriteString(`
				INSERT INTO node_snapshots (
					collection_run_org, organisation_name, node_name,
					chef_environment, chef_version, platform, platform_version,
					platform_family, platform_caption, filesystem, cookbooks, run_list, roles,
					policy_name, policy_group, ohai_time, custom_attributes,
					is_stale, collected_at,
					migration_state, active_chef_version, dormant_installed,
					dormant_chef_version, target_version, target_execution_time,
					target_converge_status
				) VALUES `)

			// ON CONFLICT clause will be appended after the VALUES rows.

			args := make([]interface{}, 0, len(batch)*numCols)
			for i, p := range batch {
				if i > 0 {
					sb.WriteString(", ")
				}
				offset := i * numCols
				fmt.Fprintf(&sb,
					"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
					offset+1, offset+2, offset+3, offset+4, offset+5, offset+6,
					offset+7, offset+8, offset+9, offset+10, offset+11, offset+12,
					offset+13, offset+14, offset+15, offset+16, offset+17, offset+18,
					offset+19, offset+20, offset+21, offset+22, offset+23, offset+24,
					offset+25, offset+26,
				)

				if p.CollectedAt.IsZero() {
					p.CollectedAt = time.Now().UTC()
				}

				args = append(args,
					p.CollectionRunOrg,
					p.OrganisationName,
					p.NodeName,
					nullString(p.ChefEnvironment),
					nullString(p.ChefVersion),
					nullString(p.Platform),
					nullString(p.PlatformVersion),
					nullString(p.PlatformFamily),
					nullString(p.PlatformCaption),
					nullJSON(p.Filesystem),
					nullJSON(p.Cookbooks),
					nullJSON(p.RunList),
					nullJSON(p.Roles),
					nullString(p.PolicyName),
					nullString(p.PolicyGroup),
					nullFloat(p.OhaiTime),
					nullJSON(p.CustomAttributes),
					p.IsStale,
					p.CollectedAt,
					nullString(p.MigrationState),
					nullString(p.ActiveChefVersion),
					nullBool(p.DormantInstalled),
					nullString(p.DormantChefVersion),
					nullString(p.TargetVersion),
					nullString(p.TargetExecutionTime),
					nullString(p.TargetConvergeStatus),
				)
			}

			// Append the ON CONFLICT ... DO UPDATE clause.
			sb.WriteString(`
				ON CONFLICT (organisation_name, node_name) DO UPDATE SET
					collection_run_org = EXCLUDED.collection_run_org,
					chef_environment   = EXCLUDED.chef_environment,
					chef_version       = EXCLUDED.chef_version,
					platform           = EXCLUDED.platform,
					platform_version   = EXCLUDED.platform_version,
					platform_family    = EXCLUDED.platform_family,
					platform_caption   = EXCLUDED.platform_caption,
					filesystem         = EXCLUDED.filesystem,
					cookbooks          = EXCLUDED.cookbooks,
					run_list           = EXCLUDED.run_list,
					roles              = EXCLUDED.roles,
					policy_name        = EXCLUDED.policy_name,
					policy_group       = EXCLUDED.policy_group,
					ohai_time          = EXCLUDED.ohai_time,
					custom_attributes  = EXCLUDED.custom_attributes,
					is_stale           = EXCLUDED.is_stale,
					collected_at       = EXCLUDED.collected_at,
					migration_state       = EXCLUDED.migration_state,
					active_chef_version   = EXCLUDED.active_chef_version,
					dormant_installed     = EXCLUDED.dormant_installed,
					dormant_chef_version  = EXCLUDED.dormant_chef_version,
					target_version        = EXCLUDED.target_version,
					target_execution_time = EXCLUDED.target_execution_time,
					target_converge_status = EXCLUDED.target_converge_status
			`)

			if returnKeys {
				sb.WriteString(" RETURNING organisation_name, node_name")
				rows, err := tx.QueryContext(ctx, sb.String(), args...)
				if err != nil {
					return fmt.Errorf("datastore: batch upserting node snapshots (rows %d-%d): %w", start, end-1, err)
				}
				for rows.Next() {
					var orgName, nodeName string
					if err := rows.Scan(&orgName, &nodeName); err != nil {
						rows.Close()
						return fmt.Errorf("datastore: scanning batch node snapshot key: %w", err)
					}
					keyMap[nodeName] = orgName
					inserted++
				}
				rows.Close()
				if err := rows.Err(); err != nil {
					return fmt.Errorf("datastore: iterating batch node snapshot keys: %w", err)
				}
			} else {
				result, err := tx.ExecContext(ctx, sb.String(), args...)
				if err != nil {
					return fmt.Errorf("datastore: batch upserting node snapshots (rows %d-%d): %w", start, end-1, err)
				}
				n, _ := result.RowsAffected()
				inserted += int(n)
			}
		}
		return nil
	})

	if err != nil {
		return nil, 0, err
	}
	return keyMap, inserted, nil
}

// ---------------------------------------------------------------------------
// Query methods
// ---------------------------------------------------------------------------

// ListNodeSnapshotsByCollectionRun returns all node snapshots for the given
// collection run (identified by organisation name), ordered by node name.
func (db *DB) ListNodeSnapshotsByCollectionRun(ctx context.Context, orgName string) ([]NodeSnapshot, error) {
	return db.listNodeSnapshotsByCollectionRun(ctx, db.q(), orgName)
}

func (db *DB) listNodeSnapshotsByCollectionRun(ctx context.Context, q queryable, orgName string) ([]NodeSnapshot, error) {
	const query = `
		SELECT collection_run_org, organisation_name, node_name,
		       chef_environment, chef_version, platform, platform_version,
		       platform_family, platform_caption, filesystem, cookbooks, run_list, roles,
		       policy_name, policy_group, ohai_time, custom_attributes,
		       is_stale, collected_at, created_at,
		       migration_state, active_chef_version, dormant_installed,
		       dormant_chef_version, target_version, target_execution_time,
		       target_converge_status
		FROM node_snapshots
		WHERE collection_run_org = $1
		ORDER BY node_name
	`
	return scanNodeSnapshots(q.QueryContext(ctx, query, orgName))
}

// ListNodeSnapshotsByOrganisation returns all node snapshots for the given
// organisation from the most recent completed collection run. This gives the
// current picture of the fleet for that org. Returns an empty slice if no
// completed collection run exists.
func (db *DB) ListNodeSnapshotsByOrganisation(ctx context.Context, organisationName string) ([]NodeSnapshot, error) {
	return db.listNodeSnapshotsByOrganisation(ctx, db.q(), organisationName)
}

func (db *DB) listNodeSnapshotsByOrganisation(ctx context.Context, q queryable, organisationName string) ([]NodeSnapshot, error) {
	// Do not gate on collection_runs.status. Node snapshots are upserted in
	// place and are valid once written, even if the collection run later fails.
	const query = `
		SELECT ns.collection_run_org, ns.organisation_name, ns.node_name,
		       ns.chef_environment, ns.chef_version, ns.platform, ns.platform_version,
		       ns.platform_family, ns.platform_caption, ns.filesystem, ns.cookbooks, ns.run_list, ns.roles,
		       ns.policy_name, ns.policy_group, ns.ohai_time, ns.custom_attributes,
		       ns.is_stale, ns.collected_at, ns.created_at,
		       ns.migration_state, ns.active_chef_version, ns.dormant_installed,
		       ns.dormant_chef_version, ns.target_version, ns.target_execution_time,
		       ns.target_converge_status
		FROM node_snapshots ns
		WHERE ns.organisation_name = $1
		ORDER BY ns.node_name
	`
	return scanNodeSnapshots(q.QueryContext(ctx, query, organisationName))
}

// GetNodeSnapshotByName returns the most recent snapshot for a node with the
// given name in the given organisation. Returns ErrNotFound if no snapshot
// exists for that node.
func (db *DB) GetNodeSnapshotByName(ctx context.Context, organisationName, nodeName string) (NodeSnapshot, error) {
	return db.getNodeSnapshotByName(ctx, db.q(), organisationName, nodeName)
}

func (db *DB) getNodeSnapshotByName(ctx context.Context, q queryable, organisationName, nodeName string) (NodeSnapshot, error) {
	const query = `
		SELECT collection_run_org, organisation_name, node_name,
		       chef_environment, chef_version, platform, platform_version,
		       platform_family, platform_caption, filesystem, cookbooks, run_list, roles,
		       policy_name, policy_group, ohai_time, custom_attributes,
		       is_stale, collected_at, created_at,
		       migration_state, active_chef_version, dormant_installed,
		       dormant_chef_version, target_version, target_execution_time,
		       target_converge_status
		FROM node_snapshots
		WHERE organisation_name = $1 AND node_name = $2
		ORDER BY collected_at DESC
		LIMIT 1
	`
	return scanNodeSnapshot(q.QueryRowContext(ctx, query, organisationName, nodeName))
}

// CountNodeSnapshotsByCollectionRun returns the number of node snapshots
// associated with the given collection run (identified by organisation name).
func (db *DB) CountNodeSnapshotsByCollectionRun(ctx context.Context, orgName string) (int, error) {
	return db.countNodeSnapshotsByCollectionRun(ctx, db.q(), orgName)
}

func (db *DB) countNodeSnapshotsByCollectionRun(ctx context.Context, q queryable, orgName string) (int, error) {
	var count int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM node_snapshots WHERE collection_run_org = $1`,
		orgName,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("datastore: counting node snapshots: %w", err)
	}
	return count, nil
}

// CountStaleNodesByCollectionRun returns the number of stale node snapshots
// in the given collection run (identified by organisation name).
func (db *DB) CountStaleNodesByCollectionRun(ctx context.Context, orgName string) (int, error) {
	return db.countStaleNodesByCollectionRun(ctx, db.q(), orgName)
}

func (db *DB) countStaleNodesByCollectionRun(ctx context.Context, q queryable, orgName string) (int, error) {
	var count int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM node_snapshots WHERE collection_run_org = $1 AND is_stale = TRUE`,
		orgName,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("datastore: counting stale node snapshots: %w", err)
	}
	return count, nil
}

// DeleteNodeSnapshotsByCollectionRun removes all node snapshots for the
// given collection run (identified by organisation name). Returns the number
// of rows deleted.
func (db *DB) DeleteNodeSnapshotsByCollectionRun(ctx context.Context, orgName string) (int, error) {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM node_snapshots WHERE collection_run_org = $1`,
		orgName,
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

// DeleteOrphanedNodeSnapshots removes node_snapshots rows for the given
// organisation whose node_name is not in the provided set of active names.
// This cleans up rows for nodes that have been decommissioned from the Chef
// Server and no longer appear in collection results. Dependent rows in
// node_readiness are removed automatically via ON DELETE CASCADE.
//
// Returns the count of deleted rows. If activeNodeNames is empty, no rows
// are deleted (safety guard against accidental purge during transient
// collection failures).
func (db *DB) DeleteOrphanedNodeSnapshots(ctx context.Context, organisationName string, activeNodeNames []string) (int, error) {
	if len(activeNodeNames) == 0 {
		return 0, nil
	}

	const query = `
		DELETE FROM node_snapshots
		WHERE organisation_name = $1
		  AND node_name != ALL($2::text[])
	`
	res, err := db.pool.ExecContext(ctx, query, organisationName, pq.Array(activeNodeNames))
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting orphaned node snapshots for org %s: %w", organisationName, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	return int(n), nil
}

// CountStaleFreshByCollectionRun returns total, stale, and fresh node counts
// for the given collection run (identified by organisation name) without
// loading full node snapshot rows.
func (db *DB) CountStaleFreshByCollectionRun(ctx context.Context, orgName string) (total, stale, fresh int, err error) {
	const query = `
		SELECT COUNT(*) AS total,
		       COUNT(*) FILTER (WHERE is_stale = TRUE) AS stale,
		       COUNT(*) FILTER (WHERE is_stale = FALSE) AS fresh
		  FROM node_snapshots
		 WHERE collection_run_org = $1
	`
	err = db.pool.QueryRowContext(ctx, query, orgName).Scan(&total, &stale, &fresh)
	if err != nil {
		err = fmt.Errorf("datastore: counting stale/fresh by collection run: %w", err)
	}
	return
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
	var collectionRunOrg sql.NullString
	var chefEnv, chefVer, platform, platformVer, platformFam sql.NullString
	var platformCaption sql.NullString
	var policyName, policyGroup sql.NullString
	var ohaiTime sql.NullFloat64
	var filesystem, cookbooks, runList, roles, customAttributes []byte
	var migrationState, activeChefVer, dormantChefVer sql.NullString
	var dormantInstalled sql.NullBool
	var targetVer, targetExecTime, targetConvergeStatus sql.NullString

	err := row.Scan(
		&collectionRunOrg,
		&ns.OrganisationName,
		&ns.NodeName,
		&chefEnv,
		&chefVer,
		&platform,
		&platformVer,
		&platformFam,
		&platformCaption,
		&filesystem,
		&cookbooks,
		&runList,
		&roles,
		&policyName,
		&policyGroup,
		&ohaiTime,
		&customAttributes,
		&ns.IsStale,
		&ns.CollectedAt,
		&ns.CreatedAt,
		&migrationState,
		&activeChefVer,
		&dormantInstalled,
		&dormantChefVer,
		&targetVer,
		&targetExecTime,
		&targetConvergeStatus,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return NodeSnapshot{}, ErrNotFound
		}
		return NodeSnapshot{}, fmt.Errorf("datastore: scanning node snapshot: %w", err)
	}

	ns.CollectionRunOrg = stringFromNull(collectionRunOrg)
	ns.ChefEnvironment = stringFromNull(chefEnv)
	ns.ChefVersion = stringFromNull(chefVer)
	ns.Platform = stringFromNull(platform)
	ns.PlatformVersion = stringFromNull(platformVer)
	ns.PlatformFamily = stringFromNull(platformFam)
	ns.PlatformCaption = stringFromNull(platformCaption)
	ns.PolicyName = stringFromNull(policyName)
	ns.PolicyGroup = stringFromNull(policyGroup)
	ns.OhaiTime = floatFromNull(ohaiTime)
	ns.Filesystem = jsonFromNullBytes(filesystem)
	ns.Cookbooks = jsonFromNullBytes(cookbooks)
	ns.RunList = jsonFromNullBytes(runList)
	ns.Roles = jsonFromNullBytes(roles)
	ns.CustomAttributes = jsonFromNullBytes(customAttributes)
	ns.MigrationState = stringFromNull(migrationState)
	ns.ActiveChefVersion = stringFromNull(activeChefVer)
	ns.DormantInstalled = boolFromNull(dormantInstalled)
	ns.DormantChefVersion = stringFromNull(dormantChefVer)
	ns.TargetVersion = stringFromNull(targetVer)
	ns.TargetExecutionTime = stringFromNull(targetExecTime)
	ns.TargetConvergeStatus = stringFromNull(targetConvergeStatus)
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
		var collectionRunOrg sql.NullString
		var chefEnv, chefVer, platform, platformVer, platformFam sql.NullString
		var platformCaption sql.NullString
		var policyName, policyGroup sql.NullString
		var ohaiTime sql.NullFloat64
		var filesystem, cookbooks, runList, roles, customAttributes []byte
		var migrationState, activeChefVer, dormantChefVer sql.NullString
		var dormantInstalled sql.NullBool
		var targetVer, targetExecTime, targetConvergeStatus sql.NullString

		if err := rows.Scan(
			&collectionRunOrg,
			&ns.OrganisationName,
			&ns.NodeName,
			&chefEnv,
			&chefVer,
			&platform,
			&platformVer,
			&platformFam,
			&platformCaption,
			&filesystem,
			&cookbooks,
			&runList,
			&roles,
			&policyName,
			&policyGroup,
			&ohaiTime,
			&customAttributes,
			&ns.IsStale,
			&ns.CollectedAt,
			&ns.CreatedAt,
			&migrationState,
			&activeChefVer,
			&dormantInstalled,
			&dormantChefVer,
			&targetVer,
			&targetExecTime,
			&targetConvergeStatus,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning node snapshot row: %w", err)
		}

		ns.CollectionRunOrg = stringFromNull(collectionRunOrg)
		ns.ChefEnvironment = stringFromNull(chefEnv)
		ns.ChefVersion = stringFromNull(chefVer)
		ns.Platform = stringFromNull(platform)
		ns.PlatformVersion = stringFromNull(platformVer)
		ns.PlatformFamily = stringFromNull(platformFam)
		ns.PlatformCaption = stringFromNull(platformCaption)
		ns.PolicyName = stringFromNull(policyName)
		ns.PolicyGroup = stringFromNull(policyGroup)
		ns.OhaiTime = floatFromNull(ohaiTime)
		ns.Filesystem = jsonFromNullBytes(filesystem)
		ns.Cookbooks = jsonFromNullBytes(cookbooks)
		ns.RunList = jsonFromNullBytes(runList)
		ns.Roles = jsonFromNullBytes(roles)
		ns.CustomAttributes = jsonFromNullBytes(customAttributes)
		ns.MigrationState = stringFromNull(migrationState)
		ns.ActiveChefVersion = stringFromNull(activeChefVer)
		ns.DormantInstalled = boolFromNull(dormantInstalled)
		ns.DormantChefVersion = stringFromNull(dormantChefVer)
		ns.TargetVersion = stringFromNull(targetVer)
		ns.TargetExecutionTime = stringFromNull(targetExecTime)
		ns.TargetConvergeStatus = stringFromNull(targetConvergeStatus)
		snapshots = append(snapshots, ns)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating node snapshot rows: %w", err)
	}
	return snapshots, nil
}
