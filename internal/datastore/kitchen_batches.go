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
// Batch status constants
// ---------------------------------------------------------------------------

const (
	BatchStatusDraft      = "draft"
	BatchStatusPreparing  = "preparing"
	BatchStatusPreviewing = "previewing"
	BatchStatusRunning    = "running"
	BatchStatusCompleted  = "completed"
	BatchStatusCancelled  = "cancelled"
	BatchStatusFailed     = "failed"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// BatchFilters represents the JSONB filter criteria for a kitchen batch.
type BatchFilters struct {
	CookbookNames      []string `json:"cookbook_names,omitempty"`
	Platforms          []string `json:"platforms,omitempty"`
	ExcludeCookbooks   []string `json:"exclude_cookbooks,omitempty"`
	HasTestSuite       *bool    `json:"has_test_suite,omitempty"`
	PreviousStatus     string   `json:"previous_status,omitempty"`
	TargetChefVersions []string `json:"target_chef_versions,omitempty"`
	IncludeExcluded    bool     `json:"include_excluded,omitempty"`
}

// KitchenBatch represents a row in the kitchen_batches table.
type KitchenBatch struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Filters          BatchFilters `json:"filters"`
	MaxCount         *int         `json:"max_count"`
	MaxConcurrentVMs *int         `json:"max_concurrent_vms"`
	DryRun           bool         `json:"dry_run"`
	Status           string       `json:"status"`
	CreatedBy        string       `json:"created_by,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
	StartedAt        *time.Time   `json:"started_at,omitempty"`
	CompletedAt      *time.Time   `json:"completed_at,omitempty"`
}

// CreateKitchenBatchParams holds the fields required to create a kitchen batch.
type CreateKitchenBatchParams struct {
	Name             string
	Filters          BatchFilters
	MaxCount         *int
	MaxConcurrentVMs *int
	DryRun           bool
	CreatedBy        string
}

// UpdateKitchenBatchParams holds the fields that can be updated on a draft batch.
type UpdateKitchenBatchParams struct {
	Name             string
	Filters          BatchFilters
	MaxCount         *int
	MaxConcurrentVMs *int
	DryRun           bool
}

// ---------------------------------------------------------------------------
// Column list
// ---------------------------------------------------------------------------

const kitchenBatchColumns = `
	id, name, filters, max_count, max_concurrent_vms,
	dry_run, status, created_by, created_at, started_at, completed_at
`

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// CreateKitchenBatch inserts a new kitchen batch and returns the resulting row.
func (db *DB) CreateKitchenBatch(ctx context.Context, p CreateKitchenBatchParams) (KitchenBatch, error) {
	return db.createKitchenBatch(ctx, db.q(), p)
}

func (db *DB) createKitchenBatch(ctx context.Context, q queryable, p CreateKitchenBatchParams) (KitchenBatch, error) {
	if p.Name == "" {
		return KitchenBatch{}, fmt.Errorf("datastore: batch name is required")
	}

	filtersJSON, err := json.Marshal(p.Filters)
	if err != nil {
		return KitchenBatch{}, fmt.Errorf("datastore: marshalling batch filters: %w", err)
	}

	var maxCount sql.NullInt64
	if p.MaxCount != nil {
		maxCount = sql.NullInt64{Int64: int64(*p.MaxCount), Valid: true}
	}
	var maxConcurrentVMs sql.NullInt64
	if p.MaxConcurrentVMs != nil {
		maxConcurrentVMs = sql.NullInt64{Int64: int64(*p.MaxConcurrentVMs), Valid: true}
	}

	const query = `
		INSERT INTO kitchen_batches (
			name, filters, max_count, max_concurrent_vms, dry_run, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6
		)
		RETURNING ` + kitchenBatchColumns

	return scanKitchenBatch(q.QueryRowContext(ctx, query,
		p.Name,
		filtersJSON,
		maxCount,
		maxConcurrentVMs,
		p.DryRun,
		nullString(p.CreatedBy),
	))
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetKitchenBatch returns a kitchen batch by its UUID primary key.
// Returns ErrNotFound if no such batch exists.
func (db *DB) GetKitchenBatch(ctx context.Context, id string) (KitchenBatch, error) {
	return db.getKitchenBatch(ctx, db.q(), id)
}

func (db *DB) getKitchenBatch(ctx context.Context, q queryable, id string) (KitchenBatch, error) {
	query := `SELECT ` + kitchenBatchColumns + `
		FROM kitchen_batches
		WHERE id = $1`
	return scanKitchenBatch(q.QueryRowContext(ctx, query, id))
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListKitchenBatches returns all kitchen batches, ordered by created_at DESC.
func (db *DB) ListKitchenBatches(ctx context.Context) ([]KitchenBatch, error) {
	return db.listKitchenBatches(ctx, db.q())
}

func (db *DB) listKitchenBatches(ctx context.Context, q queryable) ([]KitchenBatch, error) {
	query := `SELECT ` + kitchenBatchColumns + `
		FROM kitchen_batches
		ORDER BY created_at DESC`
	return scanKitchenBatches(q.QueryContext(ctx, query))
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// UpdateKitchenBatch updates a draft kitchen batch. Only batches with
// status = 'draft' can be updated. Returns ErrNotFound if the batch does
// not exist or is not in draft status.
func (db *DB) UpdateKitchenBatch(ctx context.Context, id string, p UpdateKitchenBatchParams) (KitchenBatch, error) {
	return db.updateKitchenBatch(ctx, db.q(), id, p)
}

func (db *DB) updateKitchenBatch(ctx context.Context, q queryable, id string, p UpdateKitchenBatchParams) (KitchenBatch, error) {
	if p.Name == "" {
		return KitchenBatch{}, fmt.Errorf("datastore: batch name is required")
	}

	filtersJSON, err := json.Marshal(p.Filters)
	if err != nil {
		return KitchenBatch{}, fmt.Errorf("datastore: marshalling batch filters: %w", err)
	}

	var maxCount sql.NullInt64
	if p.MaxCount != nil {
		maxCount = sql.NullInt64{Int64: int64(*p.MaxCount), Valid: true}
	}
	var maxConcurrentVMs sql.NullInt64
	if p.MaxConcurrentVMs != nil {
		maxConcurrentVMs = sql.NullInt64{Int64: int64(*p.MaxConcurrentVMs), Valid: true}
	}

	const query = `
		UPDATE kitchen_batches SET
			name = $2,
			filters = $3,
			max_count = $4,
			max_concurrent_vms = $5,
			dry_run = $6
		WHERE id = $1 AND status = 'draft'
		RETURNING ` + kitchenBatchColumns

	return scanKitchenBatch(q.QueryRowContext(ctx, query,
		id,
		p.Name,
		filtersJSON,
		maxCount,
		maxConcurrentVMs,
		p.DryRun,
	))
}

// ---------------------------------------------------------------------------
// Update status
// ---------------------------------------------------------------------------

// UpdateKitchenBatchStatus transitions a batch to a new status. When the
// new status is "running" or "previewing", started_at is set. When the new
// status is "completed" or "cancelled", completed_at is set. Returns
// ErrNotFound if the batch does not exist.
func (db *DB) UpdateKitchenBatchStatus(ctx context.Context, id string, status string, now time.Time) (KitchenBatch, error) {
	return db.updateKitchenBatchStatus(ctx, db.q(), id, status, now)
}

func (db *DB) updateKitchenBatchStatus(ctx context.Context, q queryable, id string, status string, now time.Time) (KitchenBatch, error) {
	const query = `
		UPDATE kitchen_batches SET
			status = $2,
			started_at = CASE WHEN $2 IN ('running', 'previewing') THEN $3 ELSE started_at END,
			completed_at = CASE WHEN $2 IN ('completed', 'cancelled') THEN $3 ELSE completed_at END
		WHERE id = $1
		RETURNING ` + kitchenBatchColumns

	return scanKitchenBatch(q.QueryRowContext(ctx, query, id, status, now))
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteKitchenBatch removes a kitchen batch. Only batches in draft,
// completed, or cancelled status can be deleted. Returns ErrNotFound if the
// batch does not exist or is not in a deletable status.
func (db *DB) DeleteKitchenBatch(ctx context.Context, id string) error {
	return db.deleteKitchenBatch(ctx, db.q(), id)
}

func (db *DB) deleteKitchenBatch(ctx context.Context, q queryable, id string) error {
	const query = `DELETE FROM kitchen_batches
		WHERE id = $1 AND status IN ('draft', 'completed', 'cancelled')`

	res, err := q.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting kitchen batch: %w", err)
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
// Git repo kitchen exclusion
// ---------------------------------------------------------------------------

// SetGitRepoKitchenExclusion marks a git repo as excluded from kitchen
// testing. Returns ErrNotFound if no git repo with that name exists.
func (db *DB) SetGitRepoKitchenExclusion(ctx context.Context, name string, reason string, excludedBy string) error {
	if err := db.setGitRepoKitchenExclusion(ctx, db.q(), name, reason, excludedBy); err != nil {
		return err
	}
	// Exclusion changes which results are "active" → recompute TK status.
	return db.RecomputeGitRepoTKStatusByName(ctx, name)
}

func (db *DB) setGitRepoKitchenExclusion(ctx context.Context, q queryable, name string, reason string, excludedBy string) error {
	const query = `
		UPDATE git_repos SET
			kitchen_excluded = true,
			kitchen_exclude_reason = $2,
			kitchen_excluded_by = $3,
			kitchen_excluded_at = now()
		WHERE name = $1`

	res, err := q.ExecContext(ctx, query, name, nullString(reason), nullString(excludedBy))
	if err != nil {
		return fmt.Errorf("datastore: setting git repo kitchen exclusion: %w", err)
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

// ClearGitRepoKitchenExclusion removes the kitchen exclusion flag from a
// git repo. Returns ErrNotFound if no git repo with that name exists.
func (db *DB) ClearGitRepoKitchenExclusion(ctx context.Context, name string) error {
	if err := db.clearGitRepoKitchenExclusion(ctx, db.q(), name); err != nil {
		return err
	}
	// Exclusion changes which results are "active" → recompute TK status.
	return db.RecomputeGitRepoTKStatusByName(ctx, name)
}

func (db *DB) clearGitRepoKitchenExclusion(ctx context.Context, q queryable, name string) error {
	const query = `
		UPDATE git_repos SET
			kitchen_excluded = false,
			kitchen_exclude_reason = NULL,
			kitchen_excluded_by = NULL,
			kitchen_excluded_at = NULL
		WHERE name = $1`

	res, err := q.ExecContext(ctx, query, name)
	if err != nil {
		return fmt.Errorf("datastore: clearing git repo kitchen exclusion: %w", err)
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

// ListExcludedGitRepos returns all git repos that have been excluded from
// kitchen testing, ordered by name.
func (db *DB) ListExcludedGitRepos(ctx context.Context) ([]GitRepo, error) {
	return db.listExcludedGitRepos(ctx, db.q())
}

func (db *DB) listExcludedGitRepos(ctx context.Context, q queryable) ([]GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + `
		FROM git_repos
		WHERE kitchen_excluded = true
		ORDER BY name`
	return scanGitRepos(q.QueryContext(ctx, query))
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanKitchenBatch(row *sql.Row) (KitchenBatch, error) {
	var kb KitchenBatch
	var filtersJSON []byte
	var maxCount, maxConcurrentVMs sql.NullInt64
	var createdBy sql.NullString
	var startedAt, completedAt sql.NullTime

	err := row.Scan(
		&kb.ID,
		&kb.Name,
		&filtersJSON,
		&maxCount,
		&maxConcurrentVMs,
		&kb.DryRun,
		&kb.Status,
		&createdBy,
		&kb.CreatedAt,
		&startedAt,
		&completedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return KitchenBatch{}, ErrNotFound
		}
		return KitchenBatch{}, fmt.Errorf("datastore: scanning kitchen batch: %w", err)
	}

	if err := json.Unmarshal(filtersJSON, &kb.Filters); err != nil {
		return KitchenBatch{}, fmt.Errorf("datastore: unmarshalling batch filters: %w", err)
	}

	kb.MaxCount = intPtrFromNull(maxCount)
	kb.MaxConcurrentVMs = intPtrFromNull(maxConcurrentVMs)
	kb.CreatedBy = stringFromNull(createdBy)
	kb.StartedAt = timePtrFromNull(startedAt)
	kb.CompletedAt = timePtrFromNull(completedAt)

	return kb, nil
}

func scanKitchenBatches(rows *sql.Rows, err error) ([]KitchenBatch, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying kitchen batches: %w", err)
	}
	defer rows.Close()

	var batches []KitchenBatch
	for rows.Next() {
		var kb KitchenBatch
		var filtersJSON []byte
		var maxCount, maxConcurrentVMs sql.NullInt64
		var createdBy sql.NullString
		var startedAt, completedAt sql.NullTime

		if err := rows.Scan(
			&kb.ID,
			&kb.Name,
			&filtersJSON,
			&maxCount,
			&maxConcurrentVMs,
			&kb.DryRun,
			&kb.Status,
			&createdBy,
			&kb.CreatedAt,
			&startedAt,
			&completedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning kitchen batch row: %w", err)
		}

		if err := json.Unmarshal(filtersJSON, &kb.Filters); err != nil {
			return nil, fmt.Errorf("datastore: unmarshalling batch filters: %w", err)
		}

		kb.MaxCount = intPtrFromNull(maxCount)
		kb.MaxConcurrentVMs = intPtrFromNull(maxConcurrentVMs)
		kb.CreatedBy = stringFromNull(createdBy)
		kb.StartedAt = timePtrFromNull(startedAt)
		kb.CompletedAt = timePtrFromNull(completedAt)

		batches = append(batches, kb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating kitchen batch rows: %w", err)
	}
	return batches, nil
}

// CancelStaleBatches transitions any batches in "running" or "preparing"
// status to "cancelled" with a completed_at timestamp. This is called at
// startup to clean up batches that were interrupted by a process restart.
// Returns the number of batches cancelled.
func (db *DB) CancelStaleBatches(ctx context.Context, now time.Time) (int, error) {
	const query = `
		UPDATE kitchen_batches
		SET status = 'cancelled', completed_at = $1
		WHERE status IN ('running', 'preparing')`

	res, err := db.q().ExecContext(ctx, query, now)
	if err != nil {
		return 0, fmt.Errorf("datastore: cancelling stale batches: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: rows affected: %w", err)
	}
	return int(n), nil
}
