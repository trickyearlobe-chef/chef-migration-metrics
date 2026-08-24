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

// CollectionRun represents a row in the collection_runs table. Each run
// tracks a single data collection cycle for one organisation. The primary
// key is organisation_name (natural key).
type CollectionRun struct {
	OrganisationName string    `json:"organisation_name"`
	Status           string    `json:"status"` // "running", "completed", "failed", "interrupted"
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at,omitempty"`
	TotalNodes       int       `json:"total_nodes,omitempty"`
	NodesCollected   int       `json:"nodes_collected,omitempty"`
	CheckpointStart  int       `json:"checkpoint_start,omitempty"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// MarshalJSON implements json.Marshaler for CollectionRun.
func (cr CollectionRun) MarshalJSON() ([]byte, error) {
	type Alias CollectionRun
	return json.Marshal((Alias)(cr))
}

// IsTerminal returns true if the run is in a terminal state (completed,
// failed, or interrupted) and will not be updated further.
func (cr CollectionRun) IsTerminal() bool {
	return cr.Status == "completed" || cr.Status == "failed" || cr.Status == "interrupted"
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// CreateCollectionRunParams holds the fields required to start a new
// collection run.
type CreateCollectionRunParams struct {
	OrganisationName string
}

// CreateCollectionRun upserts a collection run for the given organisation,
// resetting it to "running" status with the current time as started_at.
// With the UNIQUE constraint on organisation_name, each
// organisation has at most one collection_runs row. Subsequent calls for
// the same organisation update the existing row in place — the row is
// stable across runs, which keeps foreign-key references (node_snapshots,
// cookbook_usage_analysis, metric_snapshots, log_entries) intact.
func (db *DB) CreateCollectionRun(ctx context.Context, p CreateCollectionRunParams) (CollectionRun, error) {
	return db.createCollectionRun(ctx, db.q(), p)
}

func (db *DB) createCollectionRun(ctx context.Context, q queryable, p CreateCollectionRunParams) (CollectionRun, error) {
	if p.OrganisationName == "" {
		return CollectionRun{}, fmt.Errorf("datastore: organisation name is required to create a collection run")
	}

	const query = `
		INSERT INTO collection_runs (organisation_name, status, started_at)
		VALUES ($1, 'running', now())
		ON CONFLICT (organisation_name)
		DO UPDATE SET
			status           = 'running',
			started_at       = now(),
			completed_at     = NULL,
			total_nodes      = NULL,
			nodes_collected  = NULL,
			checkpoint_start = NULL,
			error_message    = NULL,
			updated_at       = now()
		RETURNING organisation_name, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	return scanCollectionRun(q.QueryRowContext(ctx, query, p.OrganisationName))
}

// ---------------------------------------------------------------------------
// Update progress
// ---------------------------------------------------------------------------

// UpdateCollectionRunProgressParams holds the fields for updating the
// progress of a running collection.
type UpdateCollectionRunProgressParams struct {
	OrganisationName string
	TotalNodes       int
	NodesCollected   int
	CheckpointStart  int
}

// The four functions that change a collection run — progress, complete, fail
// and interrupt — all match their row by organisation name alone, and read the
// result with QueryRowContext, which returns the first row and discards the
// rest without reporting it. That is safe only because the primary key on
// organisation_name guarantees there is exactly one row per organisation.
//
// If that key is ever dropped so an organisation can hold more than one run,
// each of these rewrites every row that organisation has and still returns
// success. Change them to target a run id and to refuse to proceed when they
// would affect more than one row, in the same change that drops the key.

// UpdateCollectionRunProgress updates the node counts and checkpoint
// position of a running collection. This is called periodically during
// collection to support checkpoint/resume.
func (db *DB) UpdateCollectionRunProgress(ctx context.Context, p UpdateCollectionRunProgressParams) (CollectionRun, error) {
	return db.updateCollectionRunProgress(ctx, db.q(), p)
}

func (db *DB) updateCollectionRunProgress(ctx context.Context, q queryable, p UpdateCollectionRunProgressParams) (CollectionRun, error) {
	if p.OrganisationName == "" {
		return CollectionRun{}, fmt.Errorf("datastore: organisation name is required to update progress")
	}

	const query = `
		UPDATE collection_runs
		SET total_nodes      = $2,
		    nodes_collected  = $3,
		    checkpoint_start = $4,
		    updated_at       = now()
		WHERE organisation_name = $1
		RETURNING organisation_name, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	run, err := scanCollectionRun(q.QueryRowContext(ctx, query,
		p.OrganisationName, p.TotalNodes, p.NodesCollected, p.CheckpointStart,
	))
	if err != nil {
		return CollectionRun{}, fmt.Errorf("datastore: updating collection run progress: %w", err)
	}
	return run, nil
}

// ---------------------------------------------------------------------------
// Complete / Fail / Interrupt
// ---------------------------------------------------------------------------

// CompleteCollectionRun marks a collection run as "completed" with the final
// node counts and the current time as completed_at.
func (db *DB) CompleteCollectionRun(ctx context.Context, organisationName string, totalNodes, nodesCollected int) (CollectionRun, error) {
	return db.completeCollectionRun(ctx, db.q(), organisationName, totalNodes, nodesCollected)
}

func (db *DB) completeCollectionRun(ctx context.Context, q queryable, organisationName string, totalNodes, nodesCollected int) (CollectionRun, error) {
	if organisationName == "" {
		return CollectionRun{}, fmt.Errorf("datastore: organisation name is required to complete")
	}

	const query = `
		UPDATE collection_runs
		SET status          = 'completed',
		    total_nodes     = $2,
		    nodes_collected = $3,
		    completed_at    = now(),
		    updated_at      = now()
		WHERE organisation_name = $1
		RETURNING organisation_name, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	run, err := scanCollectionRun(q.QueryRowContext(ctx, query,
		organisationName, totalNodes, nodesCollected,
	))
	if err != nil {
		return CollectionRun{}, fmt.Errorf("datastore: completing collection run: %w", err)
	}
	return run, nil
}

// FailCollectionRun marks a collection run as "failed" with the given error
// message and the current time as completed_at.
func (db *DB) FailCollectionRun(ctx context.Context, organisationName string, errMsg string) (CollectionRun, error) {
	return db.failCollectionRun(ctx, db.q(), organisationName, errMsg)
}

func (db *DB) failCollectionRun(ctx context.Context, q queryable, organisationName string, errMsg string) (CollectionRun, error) {
	if organisationName == "" {
		return CollectionRun{}, fmt.Errorf("datastore: organisation name is required to fail")
	}

	const query = `
		UPDATE collection_runs
		SET status        = 'failed',
		    error_message = $2,
		    completed_at  = now(),
		    updated_at    = now()
		WHERE organisation_name = $1
		RETURNING organisation_name, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	run, err := scanCollectionRun(q.QueryRowContext(ctx, query, organisationName, errMsg))
	if err != nil {
		return CollectionRun{}, fmt.Errorf("datastore: failing collection run: %w", err)
	}
	return run, nil
}

// InterruptCollectionRun marks a collection run as "interrupted". This is
// used during graceful shutdown to record runs that were in progress when
// the application stopped. The checkpoint_start value is preserved so that
// the run can be resumed later.
func (db *DB) InterruptCollectionRun(ctx context.Context, organisationName string) (CollectionRun, error) {
	return db.interruptCollectionRun(ctx, db.q(), organisationName)
}

func (db *DB) interruptCollectionRun(ctx context.Context, q queryable, organisationName string) (CollectionRun, error) {
	if organisationName == "" {
		return CollectionRun{}, fmt.Errorf("datastore: organisation name is required to interrupt")
	}

	const query = `
		UPDATE collection_runs
		SET status       = 'interrupted',
		    completed_at = now(),
		    updated_at   = now()
		WHERE organisation_name = $1
		RETURNING organisation_name, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	run, err := scanCollectionRun(q.QueryRowContext(ctx, query, organisationName))
	if err != nil {
		return CollectionRun{}, fmt.Errorf("datastore: interrupting collection run: %w", err)
	}
	return run, nil
}

// ---------------------------------------------------------------------------
// Query methods
// ---------------------------------------------------------------------------

// GetInterruptedCollectionRuns returns all collection runs currently in
// "interrupted" status, across all organisations. This is used during startup
// to evaluate which interrupted runs should be resumed vs. abandoned.
func (db *DB) GetInterruptedCollectionRuns(ctx context.Context) ([]CollectionRun, error) {
	const query = `
		SELECT organisation_name, status, started_at, completed_at,
		       total_nodes, nodes_collected, checkpoint_start,
		       error_message, created_at, updated_at
		FROM collection_runs
		WHERE status = 'interrupted'
		ORDER BY started_at ASC
	`
	return scanCollectionRuns(db.q().QueryContext(ctx, query))
}

// AbandonCollectionRun marks an interrupted collection run as "failed" with
// an error message indicating it was abandoned due to age. This is used
// during startup recovery when an interrupted run is too old to resume.
func (db *DB) AbandonCollectionRun(ctx context.Context, organisationName string, reason string) (CollectionRun, error) {
	return db.abandonCollectionRun(ctx, db.q(), organisationName, reason)
}

func (db *DB) abandonCollectionRun(ctx context.Context, q queryable, organisationName string, reason string) (CollectionRun, error) {
	if organisationName == "" {
		return CollectionRun{}, fmt.Errorf("datastore: organisation name is required to abandon")
	}
	if reason == "" {
		reason = "abandoned: interrupted run too old to resume"
	}

	const query = `
		UPDATE collection_runs
		SET status        = 'failed',
		    error_message = $2,
		    completed_at  = now(),
		    updated_at    = now()
		WHERE organisation_name = $1 AND status = 'interrupted'
		RETURNING organisation_name, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	run, err := scanCollectionRun(q.QueryRowContext(ctx, query, organisationName, reason))
	if err != nil {
		return CollectionRun{}, fmt.Errorf("datastore: abandoning collection run: %w", err)
	}
	return run, nil
}

// ListCompletedRunsForOrganisation returns all completed collection run rows
// for the given organisation since the given time. This is used during
// checkpoint/resume to determine which organisations have already been
// collected within the scope of an interrupted run.
func (db *DB) ListCompletedRunsForOrganisation(ctx context.Context, organisationName string, since time.Time) ([]CollectionRun, error) {
	const query = `
		SELECT organisation_name, status, started_at, completed_at,
		       total_nodes, nodes_collected, checkpoint_start,
		       error_message, created_at, updated_at
		FROM collection_runs
		WHERE organisation_name = $1
		  AND status = 'completed'
		  AND started_at >= $2
		ORDER BY started_at DESC
	`
	return scanCollectionRuns(db.q().QueryContext(ctx, query, organisationName, since))
}

// GetLatestCollectionRun returns the most recent collection run for the
// given organisation (by started_at descending). Returns ErrNotFound if no
// runs exist for the organisation.
func (db *DB) GetLatestCollectionRun(ctx context.Context, organisationName string) (CollectionRun, error) {
	return db.getLatestCollectionRun(ctx, db.q(), organisationName)
}

func (db *DB) getLatestCollectionRun(ctx context.Context, q queryable, organisationName string) (CollectionRun, error) {
	const query = `
		SELECT organisation_name, status, started_at, completed_at,
		       total_nodes, nodes_collected, checkpoint_start,
		       error_message, created_at, updated_at
		FROM collection_runs
		WHERE organisation_name = $1
		ORDER BY started_at DESC
		LIMIT 1
	`
	return scanCollectionRun(q.QueryRowContext(ctx, query, organisationName))
}

// GetRunningCollectionRuns returns all collection runs currently in
// "running" status, across all organisations. This is used during startup
// to detect and mark interrupted runs from a previous process.
func (db *DB) GetRunningCollectionRuns(ctx context.Context) ([]CollectionRun, error) {
	const query = `
		SELECT organisation_name, status, started_at, completed_at,
		       total_nodes, nodes_collected, checkpoint_start,
		       error_message, created_at, updated_at
		FROM collection_runs
		WHERE status = 'running'
		ORDER BY started_at ASC
	`
	return scanCollectionRuns(db.q().QueryContext(ctx, query))
}

// PurgeOldCollectionRuns is a no-op retained for backward compatibility.
// With the upsert model, each organisation has at most one
// collection_runs row, so there are never stale rows to purge.
func (db *DB) PurgeOldCollectionRuns(ctx context.Context) (int, error) {
	return 0, nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanCollectionRun(row *sql.Row) (CollectionRun, error) {
	var cr CollectionRun
	var completedAt sql.NullTime
	var totalNodes, nodesCollected, checkpointStart sql.NullInt64
	var errorMessage sql.NullString

	err := row.Scan(
		&cr.OrganisationName,
		&cr.Status,
		&cr.StartedAt,
		&completedAt,
		&totalNodes,
		&nodesCollected,
		&checkpointStart,
		&errorMessage,
		&cr.CreatedAt,
		&cr.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return CollectionRun{}, ErrNotFound
		}
		return CollectionRun{}, fmt.Errorf("datastore: scanning collection run: %w", err)
	}

	cr.CompletedAt = timeFromNull(completedAt)
	cr.TotalNodes = intFromNull(totalNodes)
	cr.NodesCollected = intFromNull(nodesCollected)
	cr.CheckpointStart = intFromNull(checkpointStart)
	cr.ErrorMessage = stringFromNull(errorMessage)
	return cr, nil
}

// ---------------------------------------------------------------------------
// Filtered listing with SQL push-down
// ---------------------------------------------------------------------------

// CollectionRunFilter holds optional filter criteria for listing collection
// runs across all organisations with SQL WHERE clause push-down.
type CollectionRunFilter struct {
	// Organisation filters by exact organisation name (joined via organisations table).
	Organisation string

	// Status filters by exact collection run status.
	Status string

	// Limit caps the number of returned rows. 0 means no limit.
	Limit int

	// Offset is the number of rows to skip (for pagination).
	Offset int
}

// buildCollectionRunFilterQuery returns the WHERE clause (starting with
// " WHERE 1=1") and positional args for a CollectionRunFilter. The query
// assumes the collection_runs table is aliased as "cr" and the organisations
// table is aliased as "o".
func buildCollectionRunFilterQuery(f CollectionRunFilter) (where string, args []interface{}) {
	where = " WHERE 1=1"
	args = []interface{}{}
	argN := 0

	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	if f.Organisation != "" {
		where += " AND o.name = " + nextArg()
		args = append(args, f.Organisation)
	}
	if f.Status != "" {
		where += " AND cr.status = " + nextArg()
		args = append(args, f.Status)
	}

	return where, args
}

// collectionRunJoinColumns is the SELECT column list for filtered collection
// run queries that join with the organisations table.
const collectionRunJoinColumns = `cr.organisation_name, o.name, cr.status, cr.started_at, cr.completed_at,
       cr.total_nodes, cr.nodes_collected, cr.checkpoint_start,
       cr.error_message, cr.created_at, cr.updated_at`

// CollectionRunWithOrg is a CollectionRun enriched with the human-readable
// organisation name from the joined organisations table.
type CollectionRunWithOrg struct {
	OrganisationName string        `json:"organisation_name"`
	Run              CollectionRun `json:"run"`
}

// ListCollectionRunsFiltered returns collection runs across all organisations
// matching the given filter, ordered by started_at descending. Results are
// paginated via Limit/Offset.
func (db *DB) ListCollectionRunsFiltered(ctx context.Context, f CollectionRunFilter) ([]CollectionRunWithOrg, error) {
	where, args := buildCollectionRunFilterQuery(f)

	query := `SELECT ` + collectionRunJoinColumns + `
		FROM collection_runs cr
		INNER JOIN organisations o ON o.name = cr.organisation_name` + where +
		` ORDER BY cr.started_at DESC`

	argN := len(args)
	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	if f.Limit > 0 {
		query += " LIMIT " + nextArg()
		args = append(args, f.Limit)
	}
	if f.Offset > 0 {
		query += " OFFSET " + nextArg()
		args = append(args, f.Offset)
	}

	return db.scanCollectionRunsWithOrg(ctx, query, args...)
}

// CountCollectionRunsFiltered returns the total number of collection runs
// matching the given filter (ignoring Limit and Offset).
func (db *DB) CountCollectionRunsFiltered(ctx context.Context, f CollectionRunFilter) (int, error) {
	where, args := buildCollectionRunFilterQuery(f)

	query := `SELECT COUNT(*) FROM collection_runs cr
		INNER JOIN organisations o ON o.name = cr.organisation_name` + where

	var count int
	if err := db.q().QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("datastore: counting filtered collection runs: %w", err)
	}
	return count, nil
}

// scanCollectionRunsWithOrg executes a query that returns collection run
// rows joined with the organisation name column, and scans them into
// CollectionRunWithOrg structs.
func (db *DB) scanCollectionRunsWithOrg(ctx context.Context, query string, args ...interface{}) ([]CollectionRunWithOrg, error) {
	rows, err := db.q().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: querying filtered collection runs: %w", err)
	}
	defer rows.Close()

	var results []CollectionRunWithOrg
	for rows.Next() {
		var cr CollectionRun
		var orgName string
		var completedAt sql.NullTime
		var totalNodes, nodesCollected, checkpointStart sql.NullInt64
		var errorMessage sql.NullString

		if err := rows.Scan(
			&cr.OrganisationName,
			&orgName,
			&cr.Status,
			&cr.StartedAt,
			&completedAt,
			&totalNodes,
			&nodesCollected,
			&checkpointStart,
			&errorMessage,
			&cr.CreatedAt,
			&cr.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning filtered collection run row: %w", err)
		}

		cr.CompletedAt = timeFromNull(completedAt)
		cr.TotalNodes = intFromNull(totalNodes)
		cr.NodesCollected = intFromNull(nodesCollected)
		cr.CheckpointStart = intFromNull(checkpointStart)
		cr.ErrorMessage = stringFromNull(errorMessage)
		results = append(results, CollectionRunWithOrg{
			OrganisationName: orgName,
			Run:              cr,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating filtered collection run rows: %w", err)
	}
	return results, nil
}

func scanCollectionRuns(rows *sql.Rows, err error) ([]CollectionRun, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying collection runs: %w", err)
	}
	defer rows.Close()

	var runs []CollectionRun
	for rows.Next() {
		var cr CollectionRun
		var completedAt sql.NullTime
		var totalNodes, nodesCollected, checkpointStart sql.NullInt64
		var errorMessage sql.NullString

		if err := rows.Scan(
			&cr.OrganisationName,
			&cr.Status,
			&cr.StartedAt,
			&completedAt,
			&totalNodes,
			&nodesCollected,
			&checkpointStart,
			&errorMessage,
			&cr.CreatedAt,
			&cr.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning collection run row: %w", err)
		}

		cr.CompletedAt = timeFromNull(completedAt)
		cr.TotalNodes = intFromNull(totalNodes)
		cr.NodesCollected = intFromNull(nodesCollected)
		cr.CheckpointStart = intFromNull(checkpointStart)
		cr.ErrorMessage = stringFromNull(errorMessage)
		runs = append(runs, cr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating collection run rows: %w", err)
	}
	return runs, nil
}
