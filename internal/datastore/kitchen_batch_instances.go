// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Batch instance status constants
// ---------------------------------------------------------------------------

const (
	BatchInstancePending        = "pending"
	BatchInstanceRunning        = "running"
	BatchInstancePassed         = "passed"
	BatchInstanceFailed         = "failed"
	BatchInstanceErrored        = "errored"
	BatchInstanceTimedOut       = "timed_out"
	BatchInstanceNetworkTimeout = "network_timeout"
	BatchInstanceCancelled      = "cancelled"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// KitchenBatchInstance represents a row in the kitchen_batch_instances table.
type KitchenBatchInstance struct {
	ID               string     `json:"id"`
	BatchID          string     `json:"batch_id"`
	GitRepoName      string     `json:"git_repo_name"`
	GitRepoURL       string     `json:"git_repo_url"`
	InstanceName     string     `json:"instance_name"`
	PlatformName     string     `json:"platform_name"`
	SuiteName        string     `json:"suite_name"`
	TargetChefVersion string    `json:"target_chef_version"`
	Status           string     `json:"status"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// CreateBatchInstanceParams holds the fields required to create a batch instance.
type CreateBatchInstanceParams struct {
	BatchID           string
	GitRepoName       string
	GitRepoURL        string
	InstanceName      string
	PlatformName      string
	SuiteName         string
	TargetChefVersion string
}

// ---------------------------------------------------------------------------
// Column list
// ---------------------------------------------------------------------------

const batchInstanceColumns = `
	id, batch_id, git_repo_name, git_repo_url, instance_name,
	platform_name, suite_name, target_chef_version, status,
	error_message, started_at, completed_at, created_at
`

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// CreateBatchInstance inserts a new batch instance and returns the resulting row.
func (db *DB) CreateBatchInstance(ctx context.Context, p CreateBatchInstanceParams) (KitchenBatchInstance, error) {
	return db.createBatchInstance(ctx, db.q(), p)
}

func (db *DB) createBatchInstance(ctx context.Context, q queryable, p CreateBatchInstanceParams) (KitchenBatchInstance, error) {
	const query = `
		INSERT INTO kitchen_batch_instances (
			batch_id, git_repo_name, git_repo_url, instance_name,
			platform_name, suite_name, target_chef_version
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
		RETURNING ` + batchInstanceColumns

	return scanBatchInstance(q.QueryRowContext(ctx, query,
		p.BatchID,
		p.GitRepoName,
		p.GitRepoURL,
		p.InstanceName,
		p.PlatformName,
		p.SuiteName,
		p.TargetChefVersion,
	))
}

// ---------------------------------------------------------------------------
// Bulk create
// ---------------------------------------------------------------------------

// CreateBatchInstances inserts multiple batch instances within a transaction
// and returns the resulting rows.
func (db *DB) CreateBatchInstances(ctx context.Context, params []CreateBatchInstanceParams) ([]KitchenBatchInstance, error) {
	var instances []KitchenBatchInstance

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		for _, p := range params {
			inst, err := db.createBatchInstance(ctx, tx, p)
			if err != nil {
				return err
			}
			instances = append(instances, inst)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return instances, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListBatchInstances returns all batch instances for a given batch ID,
// ordered by git_repo_name, instance_name.
func (db *DB) ListBatchInstances(ctx context.Context, batchID string) ([]KitchenBatchInstance, error) {
	return db.listBatchInstances(ctx, db.q(), batchID)
}

func (db *DB) listBatchInstances(ctx context.Context, q queryable, batchID string) ([]KitchenBatchInstance, error) {
	query := `SELECT ` + batchInstanceColumns + `
		FROM kitchen_batch_instances
		WHERE batch_id = $1
		ORDER BY git_repo_name, instance_name`
	return scanBatchInstances(q.QueryContext(ctx, query, batchID))
}

// ---------------------------------------------------------------------------
// Update status
// ---------------------------------------------------------------------------

// UpdateBatchInstanceStatus transitions a batch instance to a new status.
// When the status is "running", started_at is set. When the status is
// terminal (passed, failed, errored, timed_out, network_timeout, cancelled),
// completed_at is set.
func (db *DB) UpdateBatchInstanceStatus(ctx context.Context, id string, status string, errorMessage string, now time.Time) error {
	return db.updateBatchInstanceStatus(ctx, db.q(), id, status, errorMessage, now)
}

func (db *DB) updateBatchInstanceStatus(ctx context.Context, q queryable, id string, status string, errorMessage string, now time.Time) error {
	const query = `
		UPDATE kitchen_batch_instances SET
			status = $2,
			error_message = $3,
			started_at = CASE WHEN $2 = 'running' THEN $4 ELSE started_at END,
			completed_at = CASE WHEN $2 IN ('passed', 'failed', 'errored', 'timed_out', 'network_timeout', 'cancelled') THEN $4 ELSE completed_at END
		WHERE id = $1`

	res, err := q.ExecContext(ctx, query, id, status, nullString(errorMessage), now)
	if err != nil {
		return fmt.Errorf("datastore: updating batch instance status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Count by status
// ---------------------------------------------------------------------------

// CountBatchInstancesByStatus returns a map of status to count for all
// instances belonging to the given batch.
func (db *DB) CountBatchInstancesByStatus(ctx context.Context, batchID string) (map[string]int, error) {
	return db.countBatchInstancesByStatus(ctx, db.q(), batchID)
}

func (db *DB) countBatchInstancesByStatus(ctx context.Context, q queryable, batchID string) (map[string]int, error) {
	const query = `
		SELECT status, count(*)
		FROM kitchen_batch_instances
		WHERE batch_id = $1
		GROUP BY status`

	rows, err := q.QueryContext(ctx, query, batchID)
	if err != nil {
		return nil, fmt.Errorf("datastore: counting batch instances by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("datastore: scanning batch instance count row: %w", err)
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating batch instance count rows: %w", err)
	}
	return counts, nil
}

// ---------------------------------------------------------------------------
// Cancel pending
// ---------------------------------------------------------------------------

// CancelPendingBatchInstances sets all pending instances for the given batch
// to cancelled. Returns the number of rows affected.
func (db *DB) CancelPendingBatchInstances(ctx context.Context, batchID string) (int, error) {
	return db.cancelPendingBatchInstances(ctx, db.q(), batchID)
}

func (db *DB) cancelPendingBatchInstances(ctx context.Context, q queryable, batchID string) (int, error) {
	const query = `
		UPDATE kitchen_batch_instances
		SET status = 'cancelled'
		WHERE batch_id = $1 AND status = 'pending'`

	res, err := q.ExecContext(ctx, query, batchID)
	if err != nil {
		return 0, fmt.Errorf("datastore: cancelling pending batch instances: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	return int(n), nil
}

// ---------------------------------------------------------------------------
// CAS-style batch status update
// ---------------------------------------------------------------------------

// UpdateKitchenBatchStatusIfCurrent performs a compare-and-swap update on a
// kitchen batch status. It updates the batch only if its current status
// matches expectedStatus. When the new status is "running" or "previewing",
// started_at is set. When the new status is "completed" or "cancelled",
// completed_at is set. Returns ErrNotFound if the batch does not exist or
// its status does not match expectedStatus.
func (db *DB) UpdateKitchenBatchStatusIfCurrent(ctx context.Context, id string, expectedStatus string, newStatus string, now time.Time) (KitchenBatch, error) {
	return db.updateKitchenBatchStatusIfCurrent(ctx, db.q(), id, expectedStatus, newStatus, now)
}

func (db *DB) updateKitchenBatchStatusIfCurrent(ctx context.Context, q queryable, id string, expectedStatus string, newStatus string, now time.Time) (KitchenBatch, error) {
	const query = `
		UPDATE kitchen_batches SET
			status = $3,
			started_at = CASE WHEN $3 IN ('running', 'previewing') THEN $4 ELSE started_at END,
			completed_at = CASE WHEN $3 IN ('completed', 'cancelled') THEN $4 ELSE completed_at END
		WHERE id = $1 AND status = $2
		RETURNING ` + kitchenBatchColumns

	return scanKitchenBatch(q.QueryRowContext(ctx, query, id, expectedStatus, newStatus, now))
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanBatchInstance(row *sql.Row) (KitchenBatchInstance, error) {
	var bi KitchenBatchInstance
	var errorMessage sql.NullString
	var startedAt, completedAt sql.NullTime

	err := row.Scan(
		&bi.ID,
		&bi.BatchID,
		&bi.GitRepoName,
		&bi.GitRepoURL,
		&bi.InstanceName,
		&bi.PlatformName,
		&bi.SuiteName,
		&bi.TargetChefVersion,
		&bi.Status,
		&errorMessage,
		&startedAt,
		&completedAt,
		&bi.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return KitchenBatchInstance{}, ErrNotFound
		}
		return KitchenBatchInstance{}, fmt.Errorf("datastore: scanning batch instance: %w", err)
	}

	bi.ErrorMessage = stringFromNull(errorMessage)
	bi.StartedAt = timePtrFromNull(startedAt)
	bi.CompletedAt = timePtrFromNull(completedAt)

	return bi, nil
}

func scanBatchInstances(rows *sql.Rows, err error) ([]KitchenBatchInstance, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying batch instances: %w", err)
	}
	defer rows.Close()

	var instances []KitchenBatchInstance
	for rows.Next() {
		var bi KitchenBatchInstance
		var errorMessage sql.NullString
		var startedAt, completedAt sql.NullTime

		if err := rows.Scan(
			&bi.ID,
			&bi.BatchID,
			&bi.GitRepoName,
			&bi.GitRepoURL,
			&bi.InstanceName,
			&bi.PlatformName,
			&bi.SuiteName,
			&bi.TargetChefVersion,
			&bi.Status,
			&errorMessage,
			&startedAt,
			&completedAt,
			&bi.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning batch instance row: %w", err)
		}

		bi.ErrorMessage = stringFromNull(errorMessage)
		bi.StartedAt = timePtrFromNull(startedAt)
		bi.CompletedAt = timePtrFromNull(completedAt)

		instances = append(instances, bi)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating batch instance rows: %w", err)
	}
	return instances, nil
}
