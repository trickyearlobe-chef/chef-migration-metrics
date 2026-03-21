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
// tracks a single data collection cycle for one organisation.
type CollectionRun struct {
	ID              string    `json:"id"`
	OrganisationID  string    `json:"organisation_id"`
	Status          string    `json:"status"` // "running", "completed", "failed", "interrupted"
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
	TotalNodes      int       `json:"total_nodes,omitempty"`
	NodesCollected  int       `json:"nodes_collected,omitempty"`
	CheckpointStart int       `json:"checkpoint_start,omitempty"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
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
	OrganisationID string
}

// CreateCollectionRun upserts a collection run for the given organisation,
// resetting it to "running" status with the current time as started_at.
// With the UNIQUE constraint on organisation_id (migration 0006), each
// organisation has at most one collection_runs row. Subsequent calls for
// the same organisation update the existing row in place — the row ID is
// stable across runs, which keeps foreign-key references (node_snapshots,
// cookbook_usage_analysis, metric_snapshots, log_entries) intact.
func (db *DB) CreateCollectionRun(ctx context.Context, p CreateCollectionRunParams) (CollectionRun, error) {
	return db.createCollectionRun(ctx, db.q(), p)
}

func (db *DB) createCollectionRun(ctx context.Context, q queryable, p CreateCollectionRunParams) (CollectionRun, error) {
	if p.OrganisationID == "" {
		return CollectionRun{}, fmt.Errorf("datastore: organisation ID is required to create a collection run")
	}

	const query = `
		INSERT INTO collection_runs (organisation_id, status, started_at)
		VALUES ($1, 'running', now())
		ON CONFLICT (organisation_id)
		DO UPDATE SET
			status           = 'running',
			started_at       = now(),
			completed_at     = NULL,
			total_nodes      = NULL,
			nodes_collected  = NULL,
			checkpoint_start = NULL,
			error_message    = NULL,
			updated_at       = now()
		RETURNING id, organisation_id, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	return scanCollectionRun(q.QueryRowContext(ctx, query, p.OrganisationID))
}

// ---------------------------------------------------------------------------
// Update progress
// ---------------------------------------------------------------------------

// UpdateCollectionRunProgressParams holds the fields for updating the
// progress of a running collection.
type UpdateCollectionRunProgressParams struct {
	ID              string
	TotalNodes      int
	NodesCollected  int
	CheckpointStart int
}

// UpdateCollectionRunProgress updates the node counts and checkpoint
// position of a running collection. This is called periodically during
// collection to support checkpoint/resume.
func (db *DB) UpdateCollectionRunProgress(ctx context.Context, p UpdateCollectionRunProgressParams) (CollectionRun, error) {
	return db.updateCollectionRunProgress(ctx, db.q(), p)
}

func (db *DB) updateCollectionRunProgress(ctx context.Context, q queryable, p UpdateCollectionRunProgressParams) (CollectionRun, error) {
	if p.ID == "" {
		return CollectionRun{}, fmt.Errorf("datastore: collection run ID is required to update progress")
	}

	const query = `
		UPDATE collection_runs
		SET total_nodes      = $2,
		    nodes_collected  = $3,
		    checkpoint_start = $4,
		    updated_at       = now()
		WHERE id = $1
		RETURNING id, organisation_id, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	run, err := scanCollectionRun(q.QueryRowContext(ctx, query,
		p.ID, p.TotalNodes, p.NodesCollected, p.CheckpointStart,
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
func (db *DB) CompleteCollectionRun(ctx context.Context, id string, totalNodes, nodesCollected int) (CollectionRun, error) {
	return db.completeCollectionRun(ctx, db.q(), id, totalNodes, nodesCollected)
}

func (db *DB) completeCollectionRun(ctx context.Context, q queryable, id string, totalNodes, nodesCollected int) (CollectionRun, error) {
	if id == "" {
		return CollectionRun{}, fmt.Errorf("datastore: collection run ID is required to complete")
	}

	const query = `
		UPDATE collection_runs
		SET status          = 'completed',
		    total_nodes     = $2,
		    nodes_collected = $3,
		    completed_at    = now(),
		    updated_at      = now()
		WHERE id = $1
		RETURNING id, organisation_id, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	run, err := scanCollectionRun(q.QueryRowContext(ctx, query,
		id, totalNodes, nodesCollected,
	))
	if err != nil {
		return CollectionRun{}, fmt.Errorf("datastore: completing collection run: %w", err)
	}
	return run, nil
}

// FailCollectionRun marks a collection run as "failed" with the given error
// message and the current time as completed_at.
func (db *DB) FailCollectionRun(ctx context.Context, id string, errMsg string) (CollectionRun, error) {
	return db.failCollectionRun(ctx, db.q(), id, errMsg)
}

func (db *DB) failCollectionRun(ctx context.Context, q queryable, id string, errMsg string) (CollectionRun, error) {
	if id == "" {
		return CollectionRun{}, fmt.Errorf("datastore: collection run ID is required to fail")
	}

	const query = `
		UPDATE collection_runs
		SET status        = 'failed',
		    error_message = $2,
		    completed_at  = now(),
		    updated_at    = now()
		WHERE id = $1
		RETURNING id, organisation_id, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	run, err := scanCollectionRun(q.QueryRowContext(ctx, query, id, errMsg))
	if err != nil {
		return CollectionRun{}, fmt.Errorf("datastore: failing collection run: %w", err)
	}
	return run, nil
}

// InterruptCollectionRun marks a collection run as "interrupted". This is
// used during graceful shutdown to record runs that were in progress when
// the application stopped. The checkpoint_start value is preserved so that
// the run can be resumed later.
func (db *DB) InterruptCollectionRun(ctx context.Context, id string) (CollectionRun, error) {
	return db.interruptCollectionRun(ctx, db.q(), id)
}

func (db *DB) interruptCollectionRun(ctx context.Context, q queryable, id string) (CollectionRun, error) {
	if id == "" {
		return CollectionRun{}, fmt.Errorf("datastore: collection run ID is required to interrupt")
	}

	const query = `
		UPDATE collection_runs
		SET status       = 'interrupted',
		    completed_at = now(),
		    updated_at   = now()
		WHERE id = $1
		RETURNING id, organisation_id, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	run, err := scanCollectionRun(q.QueryRowContext(ctx, query, id))
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
		SELECT id, organisation_id, status, started_at, completed_at,
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
func (db *DB) AbandonCollectionRun(ctx context.Context, id string, reason string) (CollectionRun, error) {
	return db.abandonCollectionRun(ctx, db.q(), id, reason)
}

func (db *DB) abandonCollectionRun(ctx context.Context, q queryable, id string, reason string) (CollectionRun, error) {
	if id == "" {
		return CollectionRun{}, fmt.Errorf("datastore: collection run ID is required to abandon")
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
		WHERE id = $1 AND status = 'interrupted'
		RETURNING id, organisation_id, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	run, err := scanCollectionRun(q.QueryRowContext(ctx, query, id, reason))
	if err != nil {
		return CollectionRun{}, fmt.Errorf("datastore: abandoning collection run: %w", err)
	}
	return run, nil
}

// ResumeCollectionRun resets an interrupted collection run back to "running"
// status so that the collector can continue from the checkpoint. The
// checkpoint_start value is preserved.
func (db *DB) ResumeCollectionRun(ctx context.Context, id string) (CollectionRun, error) {
	return db.resumeCollectionRun(ctx, db.q(), id)
}

func (db *DB) resumeCollectionRun(ctx context.Context, q queryable, id string) (CollectionRun, error) {
	if id == "" {
		return CollectionRun{}, fmt.Errorf("datastore: collection run ID is required to resume")
	}

	const query = `
		UPDATE collection_runs
		SET status     = 'running',
		    updated_at = now()
		WHERE id = $1 AND status = 'interrupted'
		RETURNING id, organisation_id, status, started_at, completed_at,
		          total_nodes, nodes_collected, checkpoint_start,
		          error_message, created_at, updated_at
	`

	run, err := scanCollectionRun(q.QueryRowContext(ctx, query, id))
	if err != nil {
		return CollectionRun{}, fmt.Errorf("datastore: resuming collection run: %w", err)
	}
	return run, nil
}

// ListCompletedRunsForOrganisation returns all completed collection run IDs
// for the given organisation since the given time. This is used during
// checkpoint/resume to determine which organisations have already been
// collected within the scope of an interrupted run.
func (db *DB) ListCompletedRunsForOrganisation(ctx context.Context, organisationID string, since time.Time) ([]CollectionRun, error) {
	const query = `
		SELECT id, organisation_id, status, started_at, completed_at,
		       total_nodes, nodes_collected, checkpoint_start,
		       error_message, created_at, updated_at
		FROM collection_runs
		WHERE organisation_id = $1
		  AND status = 'completed'
		  AND started_at >= $2
		ORDER BY started_at DESC
	`
	return scanCollectionRuns(db.q().QueryContext(ctx, query, organisationID, since))
}

// GetCollectionRun returns the collection run with the given UUID. Returns
// ErrNotFound if no such run exists.
func (db *DB) GetCollectionRun(ctx context.Context, id string) (CollectionRun, error) {
	return db.getCollectionRun(ctx, db.q(), id)
}

func (db *DB) getCollectionRun(ctx context.Context, q queryable, id string) (CollectionRun, error) {
	const query = `
		SELECT id, organisation_id, status, started_at, completed_at,
		       total_nodes, nodes_collected, checkpoint_start,
		       error_message, created_at, updated_at
		FROM collection_runs
		WHERE id = $1
	`
	return scanCollectionRun(q.QueryRowContext(ctx, query, id))
}

// GetLatestCollectionRun returns the most recent collection run for the
// given organisation (by started_at descending). Returns ErrNotFound if no
// runs exist for the organisation.
func (db *DB) GetLatestCollectionRun(ctx context.Context, organisationID string) (CollectionRun, error) {
	return db.getLatestCollectionRun(ctx, db.q(), organisationID)
}

func (db *DB) getLatestCollectionRun(ctx context.Context, q queryable, organisationID string) (CollectionRun, error) {
	const query = `
		SELECT id, organisation_id, status, started_at, completed_at,
		       total_nodes, nodes_collected, checkpoint_start,
		       error_message, created_at, updated_at
		FROM collection_runs
		WHERE organisation_id = $1
		ORDER BY started_at DESC
		LIMIT 1
	`
	return scanCollectionRun(q.QueryRowContext(ctx, query, organisationID))
}

// GetLatestCompletedCollectionRun returns the most recent completed
// collection run for the given organisation. Returns ErrNotFound if no
// completed runs exist.
func (db *DB) GetLatestCompletedCollectionRun(ctx context.Context, organisationID string) (CollectionRun, error) {
	return db.getLatestCompletedCollectionRun(ctx, db.q(), organisationID)
}

func (db *DB) getLatestCompletedCollectionRun(ctx context.Context, q queryable, organisationID string) (CollectionRun, error) {
	const query = `
		SELECT id, organisation_id, status, started_at, completed_at,
		       total_nodes, nodes_collected, checkpoint_start,
		       error_message, created_at, updated_at
		FROM collection_runs
		WHERE organisation_id = $1 AND status = 'completed'
		ORDER BY started_at DESC
		LIMIT 1
	`
	return scanCollectionRun(q.QueryRowContext(ctx, query, organisationID))
}

// ListCollectionRuns returns all collection runs for the given organisation,
// ordered by started_at descending (most recent first). If limit is > 0,
// at most limit rows are returned.
func (db *DB) ListCollectionRuns(ctx context.Context, organisationID string, limit int) ([]CollectionRun, error) {
	return db.listCollectionRuns(ctx, db.q(), organisationID, limit)
}

func (db *DB) listCollectionRuns(ctx context.Context, q queryable, organisationID string, limit int) ([]CollectionRun, error) {
	var query string
	var args []any

	if limit > 0 {
		query = `
			SELECT id, organisation_id, status, started_at, completed_at,
			       total_nodes, nodes_collected, checkpoint_start,
			       error_message, created_at, updated_at
			FROM collection_runs
			WHERE organisation_id = $1
			ORDER BY started_at DESC
			LIMIT $2
		`
		args = []any{organisationID, limit}
	} else {
		query = `
			SELECT id, organisation_id, status, started_at, completed_at,
			       total_nodes, nodes_collected, checkpoint_start,
			       error_message, created_at, updated_at
			FROM collection_runs
			WHERE organisation_id = $1
			ORDER BY started_at DESC
		`
		args = []any{organisationID}
	}

	return scanCollectionRuns(q.QueryContext(ctx, query, args...))
}

// GetRunningCollectionRuns returns all collection runs currently in
// "running" status, across all organisations. This is used during startup
// to detect and mark interrupted runs from a previous process.
func (db *DB) GetRunningCollectionRuns(ctx context.Context) ([]CollectionRun, error) {
	const query = `
		SELECT id, organisation_id, status, started_at, completed_at,
		       total_nodes, nodes_collected, checkpoint_start,
		       error_message, created_at, updated_at
		FROM collection_runs
		WHERE status = 'running'
		ORDER BY started_at ASC
	`
	return scanCollectionRuns(db.q().QueryContext(ctx, query))
}

// PurgeOldCollectionRuns is a no-op retained for backward compatibility.
// With the upsert model (migration 0006), each organisation has at most one
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
		&cr.ID,
		&cr.OrganisationID,
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
			&cr.ID,
			&cr.OrganisationID,
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
