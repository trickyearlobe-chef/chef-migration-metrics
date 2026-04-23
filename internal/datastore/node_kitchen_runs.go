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

// ---------------------------------------------------------------------------
// Cookbook source constants
// ---------------------------------------------------------------------------

const (
	CookbookSourceServer = "server"
	CookbookSourceGit    = "git"
	CookbookSourceHybrid = "hybrid"
)

var validCookbookSources = map[string]bool{
	CookbookSourceServer: true,
	CookbookSourceGit:    true,
	CookbookSourceHybrid: true,
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// NodeKitchenRun represents a row in the node_kitchen_runs table.
type NodeKitchenRun struct {
	ID                string          `json:"id"`
	NodeName          string          `json:"node_name"`
	OrganisationName  string          `json:"organisation_name"`
	TargetChefVersion string          `json:"target_chef_version"`
	CookbookSource    string          `json:"cookbook_source"`
	PlatformName      string          `json:"platform_name"`
	TemplateUsed      string          `json:"template_used,omitempty"`
	RunList           json.RawMessage `json:"run_list"`
	CookbookVersions  json.RawMessage `json:"cookbook_versions"`
	ConvergePassed    *bool           `json:"converge_passed"`
	VerifyPassed      *bool           `json:"verify_passed"`
	ConvergeOutput    string          `json:"converge_output,omitempty"`
	VerifyOutput      string          `json:"verify_output,omitempty"`
	DestroyOutput     string          `json:"destroy_output,omitempty"`
	DurationSeconds   *int            `json:"duration_seconds"`
	ErrorMessage      string          `json:"error_message,omitempty"`
	StartedAt         *time.Time      `json:"started_at"`
	CompletedAt       *time.Time      `json:"completed_at"`
	VMTrackingID      string          `json:"vm_tracking_id,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

// UpsertNodeKitchenRunParams holds the fields needed to insert or update a
// node kitchen run.
type UpsertNodeKitchenRunParams struct {
	NodeName          string
	OrganisationName  string
	TargetChefVersion string
	CookbookSource    string
	PlatformName      string
	TemplateUsed      string
	RunList           json.RawMessage
	CookbookVersions  json.RawMessage
	ConvergePassed    *bool
	VerifyPassed      *bool
	ConvergeOutput    string
	VerifyOutput      string
	DestroyOutput     string
	DurationSeconds   *int
	ErrorMessage      string
	StartedAt         *time.Time
	CompletedAt       *time.Time
	VMTrackingID      string
}

// UpdateNodeKitchenRunResultParams holds the fields updated after a kitchen
// run completes.
type UpdateNodeKitchenRunResultParams struct {
	ConvergePassed  *bool
	VerifyPassed    *bool
	ConvergeOutput  string
	VerifyOutput    string
	DestroyOutput   string
	DurationSeconds *int
	ErrorMessage    string
	CompletedAt     *time.Time
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func validateUpsertNodeKitchenRunParams(p UpsertNodeKitchenRunParams) error {
	if p.NodeName == "" {
		return fmt.Errorf("datastore: node_name is required")
	}
	if p.OrganisationName == "" {
		return fmt.Errorf("datastore: organisation_name is required")
	}
	if p.TargetChefVersion == "" {
		return fmt.Errorf("datastore: target_chef_version is required")
	}
	if p.PlatformName == "" {
		return fmt.Errorf("datastore: platform_name is required")
	}
	if !validCookbookSources[p.CookbookSource] {
		return fmt.Errorf("datastore: invalid cookbook_source %q", p.CookbookSource)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Column list
// ---------------------------------------------------------------------------

const nodeKitchenRunColumns = `id, node_name, organisation_name, target_chef_version,
    cookbook_source, platform_name, template_used, run_list, cookbook_versions,
    converge_passed, verify_passed, converge_output, verify_output,
    destroy_output, duration_seconds, error_message, started_at, completed_at,
    vm_tracking_id, created_at`

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertNodeKitchenRun inserts a node kitchen run or updates the existing row
// for the same (node_name, organisation_name, target_chef_version,
// cookbook_source) combination. Returns the resulting row.
func (db *DB) UpsertNodeKitchenRun(ctx context.Context, p UpsertNodeKitchenRunParams) (NodeKitchenRun, error) {
	return db.upsertNodeKitchenRun(ctx, db.q(), p)
}

func (db *DB) upsertNodeKitchenRun(ctx context.Context, q queryable, p UpsertNodeKitchenRunParams) (NodeKitchenRun, error) {
	if err := validateUpsertNodeKitchenRunParams(p); err != nil {
		return NodeKitchenRun{}, err
	}

	runList := jsonWithDefault(p.RunList, "[]")
	cookbookVersions := jsonWithDefault(p.CookbookVersions, "{}")

	const query = `
		INSERT INTO node_kitchen_runs (
			node_name, organisation_name, target_chef_version, cookbook_source,
			platform_name, template_used, run_list, cookbook_versions,
			converge_passed, verify_passed, converge_output, verify_output,
			destroy_output, duration_seconds, error_message, started_at,
			completed_at, vm_tracking_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18
		)
		ON CONFLICT (node_name, organisation_name, target_chef_version, cookbook_source) DO UPDATE SET
			platform_name    = EXCLUDED.platform_name,
			template_used    = EXCLUDED.template_used,
			run_list         = EXCLUDED.run_list,
			cookbook_versions = EXCLUDED.cookbook_versions,
			converge_passed  = EXCLUDED.converge_passed,
			verify_passed    = EXCLUDED.verify_passed,
			converge_output  = EXCLUDED.converge_output,
			verify_output    = EXCLUDED.verify_output,
			destroy_output   = EXCLUDED.destroy_output,
			duration_seconds = EXCLUDED.duration_seconds,
			error_message    = EXCLUDED.error_message,
			started_at       = EXCLUDED.started_at,
			completed_at     = EXCLUDED.completed_at,
			vm_tracking_id   = EXCLUDED.vm_tracking_id
		RETURNING ` + nodeKitchenRunColumns

	r, err := scanNodeKitchenRun(q.QueryRowContext(ctx, query,
		p.NodeName,
		p.OrganisationName,
		p.TargetChefVersion,
		p.CookbookSource,
		p.PlatformName,
		nullString(p.TemplateUsed),
		runList,
		cookbookVersions,
		nullBoolPtr(p.ConvergePassed),
		nullBoolPtr(p.VerifyPassed),
		nullString(p.ConvergeOutput),
		nullString(p.VerifyOutput),
		nullString(p.DestroyOutput),
		nullIntPtr(p.DurationSeconds),
		nullString(p.ErrorMessage),
		nullTimePtr(p.StartedAt),
		nullTimePtr(p.CompletedAt),
		nullString(p.VMTrackingID),
	))
	if err != nil {
		return NodeKitchenRun{}, fmt.Errorf("datastore: upserting node kitchen run: %w", err)
	}
	return r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListNodeKitchenRuns returns all kitchen runs for the given organisation,
// ordered by created_at DESC.
func (db *DB) ListNodeKitchenRuns(ctx context.Context, orgName string) ([]NodeKitchenRun, error) {
	if orgName == "" {
		return nil, fmt.Errorf("datastore: organisation_name is required")
	}

	const query = `SELECT ` + nodeKitchenRunColumns + `
		FROM node_kitchen_runs
		WHERE organisation_name = $1
		ORDER BY created_at DESC`

	return scanNodeKitchenRuns(db.q().QueryContext(ctx, query, orgName))
}

// ListNodeKitchenRunsByNode returns all kitchen runs for the given
// organisation and node, ordered by created_at DESC.
func (db *DB) ListNodeKitchenRunsByNode(ctx context.Context, orgName, nodeName string) ([]NodeKitchenRun, error) {
	if orgName == "" {
		return nil, fmt.Errorf("datastore: organisation_name is required")
	}
	if nodeName == "" {
		return nil, fmt.Errorf("datastore: node_name is required")
	}

	const query = `SELECT ` + nodeKitchenRunColumns + `
		FROM node_kitchen_runs
		WHERE organisation_name = $1 AND node_name = $2
		ORDER BY created_at DESC`

	return scanNodeKitchenRuns(db.q().QueryContext(ctx, query, orgName, nodeName))
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetNodeKitchenRun returns a single node kitchen run by its primary key
// UUID. Returns (nil, nil) if not found.
func (db *DB) GetNodeKitchenRun(ctx context.Context, id string) (*NodeKitchenRun, error) {
	if id == "" {
		return nil, fmt.Errorf("datastore: id is required")
	}

	const query = `SELECT ` + nodeKitchenRunColumns + `
		FROM node_kitchen_runs WHERE id = $1`

	r, err := scanNodeKitchenRun(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting node kitchen run: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Update result
// ---------------------------------------------------------------------------

// UpdateNodeKitchenRunResult updates a kitchen run after it completes.
// Returns ErrNotFound if the id does not exist.
func (db *DB) UpdateNodeKitchenRunResult(ctx context.Context, id string, p UpdateNodeKitchenRunResultParams) (NodeKitchenRun, error) {
	if id == "" {
		return NodeKitchenRun{}, fmt.Errorf("datastore: id is required")
	}

	const query = `
		UPDATE node_kitchen_runs SET
			converge_passed  = $2,
			verify_passed    = $3,
			converge_output  = $4,
			verify_output    = $5,
			destroy_output   = $6,
			duration_seconds = $7,
			error_message    = $8,
			completed_at     = $9
		WHERE id = $1
		RETURNING ` + nodeKitchenRunColumns

	r, err := scanNodeKitchenRun(db.q().QueryRowContext(ctx, query,
		id,
		nullBoolPtr(p.ConvergePassed),
		nullBoolPtr(p.VerifyPassed),
		nullString(p.ConvergeOutput),
		nullString(p.VerifyOutput),
		nullString(p.DestroyOutput),
		nullIntPtr(p.DurationSeconds),
		nullString(p.ErrorMessage),
		nullTimePtr(p.CompletedAt),
	))
	if err == sql.ErrNoRows {
		return NodeKitchenRun{}, ErrNotFound
	}
	if err != nil {
		return NodeKitchenRun{}, fmt.Errorf("datastore: updating node kitchen run result: %w", err)
	}
	return r, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteNodeKitchenRun removes a node kitchen run by id.
// Returns ErrNotFound if no row was deleted.
func (db *DB) DeleteNodeKitchenRun(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("datastore: id is required")
	}

	const query = `DELETE FROM node_kitchen_runs WHERE id = $1`

	res, err := db.q().ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting node kitchen run: %w", err)
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

func scanNodeKitchenRun(row *sql.Row) (NodeKitchenRun, error) {
	var r NodeKitchenRun
	var templateUsed, convergeOutput, verifyOutput, destroyOutput sql.NullString
	var errorMessage, vmTrackingID sql.NullString
	var convergePassed, verifyPassed sql.NullBool
	var durationSeconds sql.NullInt64
	var startedAt, completedAt sql.NullTime
	var runList, cookbookVersions []byte

	err := row.Scan(
		&r.ID,
		&r.NodeName,
		&r.OrganisationName,
		&r.TargetChefVersion,
		&r.CookbookSource,
		&r.PlatformName,
		&templateUsed,
		&runList,
		&cookbookVersions,
		&convergePassed,
		&verifyPassed,
		&convergeOutput,
		&verifyOutput,
		&destroyOutput,
		&durationSeconds,
		&errorMessage,
		&startedAt,
		&completedAt,
		&vmTrackingID,
		&r.CreatedAt,
	)
	if err != nil {
		return NodeKitchenRun{}, err
	}

	r.TemplateUsed = stringFromNull(templateUsed)
	r.RunList = jsonFromNullBytes(runList)
	r.CookbookVersions = jsonFromNullBytes(cookbookVersions)
	r.ConvergePassed = boolPtrFromNull(convergePassed)
	r.VerifyPassed = boolPtrFromNull(verifyPassed)
	r.ConvergeOutput = stringFromNull(convergeOutput)
	r.VerifyOutput = stringFromNull(verifyOutput)
	r.DestroyOutput = stringFromNull(destroyOutput)
	r.DurationSeconds = intPtrFromNull(durationSeconds)
	r.ErrorMessage = stringFromNull(errorMessage)
	r.StartedAt = timePtrFromNull(startedAt)
	r.CompletedAt = timePtrFromNull(completedAt)
	r.VMTrackingID = stringFromNull(vmTrackingID)

	return r, nil
}

func scanNodeKitchenRuns(rows *sql.Rows, err error) ([]NodeKitchenRun, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying node kitchen runs: %w", err)
	}
	defer rows.Close()

	var results []NodeKitchenRun
	for rows.Next() {
		var r NodeKitchenRun
		var templateUsed, convergeOutput, verifyOutput, destroyOutput sql.NullString
		var errorMessage, vmTrackingID sql.NullString
		var convergePassed, verifyPassed sql.NullBool
		var durationSeconds sql.NullInt64
		var startedAt, completedAt sql.NullTime
		var runList, cookbookVersions []byte

		if err := rows.Scan(
			&r.ID,
			&r.NodeName,
			&r.OrganisationName,
			&r.TargetChefVersion,
			&r.CookbookSource,
			&r.PlatformName,
			&templateUsed,
			&runList,
			&cookbookVersions,
			&convergePassed,
			&verifyPassed,
			&convergeOutput,
			&verifyOutput,
			&destroyOutput,
			&durationSeconds,
			&errorMessage,
			&startedAt,
			&completedAt,
			&vmTrackingID,
			&r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning node kitchen run row: %w", err)
		}

		r.TemplateUsed = stringFromNull(templateUsed)
		r.RunList = jsonFromNullBytes(runList)
		r.CookbookVersions = jsonFromNullBytes(cookbookVersions)
		r.ConvergePassed = boolPtrFromNull(convergePassed)
		r.VerifyPassed = boolPtrFromNull(verifyPassed)
		r.ConvergeOutput = stringFromNull(convergeOutput)
		r.VerifyOutput = stringFromNull(verifyOutput)
		r.DestroyOutput = stringFromNull(destroyOutput)
		r.DurationSeconds = intPtrFromNull(durationSeconds)
		r.ErrorMessage = stringFromNull(errorMessage)
		r.StartedAt = timePtrFromNull(startedAt)
		r.CompletedAt = timePtrFromNull(completedAt)
		r.VMTrackingID = stringFromNull(vmTrackingID)

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating node kitchen run rows: %w", err)
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Nullable pointer helpers
// ---------------------------------------------------------------------------

// jsonWithDefault returns the JSON bytes or a default value when empty.
// Use for NOT NULL JSONB columns that have a database default.
func jsonWithDefault(data json.RawMessage, def string) []byte {
	if len(data) == 0 {
		return []byte(def)
	}
	return []byte(data)
}

func boolPtrFromNull(nb sql.NullBool) *bool {
	if !nb.Valid {
		return nil
	}
	v := nb.Bool
	return &v
}

func intPtrFromNull(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	v := int(ni.Int64)
	return &v
}

func nullTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func timePtrFromNull(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	v := nt.Time
	return &v
}
