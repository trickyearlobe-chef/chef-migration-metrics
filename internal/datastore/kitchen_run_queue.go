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
// Status constants
// ---------------------------------------------------------------------------

const (
	QueueStatusQueued      = "queued"
	QueueStatusRunning     = "running"
	QueueStatusCompleted   = "completed"
	QueueStatusFailed      = "failed"
	QueueStatusCancelled   = "cancelled"
	QueueStatusInterrupted = "interrupted"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// KitchenQueueItem represents a row in kitchen_run_queue.
type KitchenQueueItem struct {
	ID                 string     `json:"id"`
	RunType            string     `json:"run_type"`
	GitRepoName        string     `json:"git_repo_name,omitempty"`
	GitRepoURL         string     `json:"git_repo_url,omitempty"`
	SuiteName          string     `json:"suite_name,omitempty"`
	PlatformName       string     `json:"platform_name,omitempty"`
	InstanceName       string     `json:"instance_name,omitempty"`
	TargetChefVersion  string     `json:"target_chef_version"`
	HeadCommitSHA      string     `json:"head_commit_sha,omitempty"`
	NodeName           string     `json:"node_name,omitempty"`
	OrganisationName   string     `json:"organisation_name,omitempty"`
	CookbookSource     string     `json:"cookbook_source,omitempty"`
	BatchID            string     `json:"batch_id,omitempty"`
	Priority           int        `json:"priority"`
	Status             string     `json:"status"`
	EnqueuedAt         time.Time  `json:"enqueued_at"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	ErrorMessage       string     `json:"error_message,omitempty"`
	Output             string     `json:"output,omitempty"`
	RetryOf            string     `json:"retry_of,omitempty"`
}

// EnqueueKitchenRunParams holds the fields for enqueuing a kitchen run.
type EnqueueKitchenRunParams struct {
	RunType           string
	GitRepoName       string
	GitRepoURL        string
	SuiteName         string
	PlatformName      string
	InstanceName      string
	TargetChefVersion string
	HeadCommitSHA     string
	NodeName          string
	OrganisationName  string
	CookbookSource    string
	BatchID           string
	Priority          int
	RetryOf           string
}

// ---------------------------------------------------------------------------
// Enqueue
// ---------------------------------------------------------------------------

// EnqueueKitchenRun inserts a new item into the queue. Returns the created
// item. If a duplicate exists (same instance already queued/running), returns
// ErrAlreadyExists.
func (db *DB) EnqueueKitchenRun(ctx context.Context, p EnqueueKitchenRunParams) (*KitchenQueueItem, error) {
	if p.RunType == "" {
		return nil, fmt.Errorf("datastore: run_type is required")
	}
	if p.TargetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target_chef_version is required")
	}
	if p.Priority == 0 {
		p.Priority = 10
	}

	var item KitchenQueueItem
	err := db.pool.QueryRowContext(ctx, `
		INSERT INTO kitchen_run_queue (
			run_type, git_repo_name, git_repo_url, suite_name, platform_name,
			instance_name, target_chef_version, head_commit_sha, node_name,
			organisation_name, cookbook_source, batch_id, priority, retry_of
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			NULLIF($12, '')::UUID, $13, NULLIF($14, '')::UUID)
		RETURNING id, run_type, COALESCE(git_repo_name, ''), COALESCE(git_repo_url, ''),
			COALESCE(suite_name, ''), COALESCE(platform_name, ''), COALESCE(instance_name, ''),
			target_chef_version, COALESCE(head_commit_sha, ''), COALESCE(node_name, ''),
			COALESCE(organisation_name, ''), COALESCE(cookbook_source, ''),
			COALESCE(batch_id::TEXT, ''), priority, status, enqueued_at`,
		p.RunType, nullEmpty(p.GitRepoName), nullEmpty(p.GitRepoURL),
		nullEmpty(p.SuiteName), nullEmpty(p.PlatformName), nullEmpty(p.InstanceName),
		p.TargetChefVersion, nullEmpty(p.HeadCommitSHA), nullEmpty(p.NodeName),
		nullEmpty(p.OrganisationName), nullEmpty(p.CookbookSource),
		p.BatchID, p.Priority, p.RetryOf,
	).Scan(
		&item.ID, &item.RunType, &item.GitRepoName, &item.GitRepoURL,
		&item.SuiteName, &item.PlatformName, &item.InstanceName,
		&item.TargetChefVersion, &item.HeadCommitSHA, &item.NodeName,
		&item.OrganisationName, &item.CookbookSource,
		&item.BatchID, &item.Priority, &item.Status, &item.EnqueuedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyExists
		}
		return nil, fmt.Errorf("datastore: enqueue kitchen run: %w", err)
	}
	return &item, nil
}

// ---------------------------------------------------------------------------
// Claim (dequeue)
// ---------------------------------------------------------------------------

// ClaimNextKitchenRun atomically claims the highest-priority oldest queued item
// by transitioning it to 'running'. Returns nil if the queue is empty.
func (db *DB) ClaimNextKitchenRun(ctx context.Context) (*KitchenQueueItem, error) {
	var item KitchenQueueItem
	var startedAt time.Time

	err := db.pool.QueryRowContext(ctx, `
		UPDATE kitchen_run_queue
		SET status = 'running', started_at = now()
		WHERE id = (
			SELECT id FROM kitchen_run_queue
			WHERE status = 'queued'
			ORDER BY priority DESC, enqueued_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, run_type, COALESCE(git_repo_name, ''), COALESCE(git_repo_url, ''),
			COALESCE(suite_name, ''), COALESCE(platform_name, ''), COALESCE(instance_name, ''),
			target_chef_version, COALESCE(head_commit_sha, ''), COALESCE(node_name, ''),
			COALESCE(organisation_name, ''), COALESCE(cookbook_source, ''),
			COALESCE(batch_id::TEXT, ''), priority, status, enqueued_at, started_at`).Scan(
		&item.ID, &item.RunType, &item.GitRepoName, &item.GitRepoURL,
		&item.SuiteName, &item.PlatformName, &item.InstanceName,
		&item.TargetChefVersion, &item.HeadCommitSHA, &item.NodeName,
		&item.OrganisationName, &item.CookbookSource,
		&item.BatchID, &item.Priority, &item.Status, &item.EnqueuedAt, &startedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: claim kitchen run: %w", err)
	}
	item.StartedAt = &startedAt
	return &item, nil
}

// ---------------------------------------------------------------------------
// Complete / Fail
// ---------------------------------------------------------------------------

// CompleteKitchenRun marks a queue item as completed with optional output.
func (db *DB) CompleteKitchenRun(ctx context.Context, id string, output string) error {
	result, err := db.pool.ExecContext(ctx, `
		UPDATE kitchen_run_queue
		SET status = 'completed', completed_at = now(), output = NULLIF($2, '')
		WHERE id = $1 AND status = 'running'`, id, output)
	if err != nil {
		return fmt.Errorf("datastore: complete kitchen run: %w", err)
	}
	return checkRowsAffected(result, id)
}

// FailKitchenRun marks a queue item as failed with an error message and output.
func (db *DB) FailKitchenRun(ctx context.Context, id string, errMsg string, output string) error {
	result, err := db.pool.ExecContext(ctx, `
		UPDATE kitchen_run_queue
		SET status = 'failed', completed_at = now(), error_message = $2, output = NULLIF($3, '')
		WHERE id = $1 AND status = 'running'`, id, errMsg, output)
	if err != nil {
		return fmt.Errorf("datastore: fail kitchen run: %w", err)
	}
	return checkRowsAffected(result, id)
}

// ---------------------------------------------------------------------------
// Cancel
// ---------------------------------------------------------------------------

// CancelKitchenRun transitions a queued or running item to cancelled.
func (db *DB) CancelKitchenRun(ctx context.Context, id string) error {
	result, err := db.pool.ExecContext(ctx, `
		UPDATE kitchen_run_queue
		SET status = 'cancelled', completed_at = now()
		WHERE id = $1 AND status IN ('queued', 'running')`, id)
	if err != nil {
		return fmt.Errorf("datastore: cancel kitchen run: %w", err)
	}
	return checkRowsAffected(result, id)
}

// ---------------------------------------------------------------------------
// Retry
// ---------------------------------------------------------------------------

// RetryKitchenRun creates a new queue item with the same parameters as the
// given item, linking it via retry_of. Only works for failed/interrupted/cancelled items.
func (db *DB) RetryKitchenRun(ctx context.Context, id string) (*KitchenQueueItem, error) {
	var item KitchenQueueItem
	err := db.pool.QueryRowContext(ctx, `
		INSERT INTO kitchen_run_queue (
			run_type, git_repo_name, git_repo_url, suite_name, platform_name,
			instance_name, target_chef_version, head_commit_sha, node_name,
			organisation_name, cookbook_source, batch_id, priority, retry_of
		)
		SELECT run_type, git_repo_name, git_repo_url, suite_name, platform_name,
			instance_name, target_chef_version, head_commit_sha, node_name,
			organisation_name, cookbook_source, batch_id, priority, $1::UUID
		FROM kitchen_run_queue
		WHERE id = $1 AND status IN ('failed', 'interrupted', 'cancelled')
		RETURNING id, run_type, COALESCE(git_repo_name, ''), COALESCE(git_repo_url, ''),
			COALESCE(suite_name, ''), COALESCE(platform_name, ''), COALESCE(instance_name, ''),
			target_chef_version, COALESCE(head_commit_sha, ''), COALESCE(node_name, ''),
			COALESCE(organisation_name, ''), COALESCE(cookbook_source, ''),
			COALESCE(batch_id::TEXT, ''), priority, status, enqueued_at`, id).Scan(
		&item.ID, &item.RunType, &item.GitRepoName, &item.GitRepoURL,
		&item.SuiteName, &item.PlatformName, &item.InstanceName,
		&item.TargetChefVersion, &item.HeadCommitSHA, &item.NodeName,
		&item.OrganisationName, &item.CookbookSource,
		&item.BatchID, &item.Priority, &item.Status, &item.EnqueuedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrAlreadyExists
		}
		return nil, fmt.Errorf("datastore: retry kitchen run: %w", err)
	}
	return &item, nil
}

// ---------------------------------------------------------------------------
// Query
// ---------------------------------------------------------------------------

// KitchenQueueFilter controls which items are returned by ListKitchenQueue.
type KitchenQueueFilter struct {
	RepoName string
	RunType  string
	Statuses []string
	Limit    int
}

// ListKitchenQueue returns queue items matching the filter, ordered by
// priority DESC then enqueued_at DESC (most recent first for display).
func (db *DB) ListKitchenQueue(ctx context.Context, f KitchenQueueFilter) ([]KitchenQueueItem, error) {
	query := `
		SELECT id, run_type, COALESCE(git_repo_name, ''), COALESCE(git_repo_url, ''),
			COALESCE(suite_name, ''), COALESCE(platform_name, ''), COALESCE(instance_name, ''),
			target_chef_version, COALESCE(head_commit_sha, ''), COALESCE(node_name, ''),
			COALESCE(organisation_name, ''), COALESCE(cookbook_source, ''),
			COALESCE(batch_id::TEXT, ''), priority, status, enqueued_at,
			started_at, completed_at, COALESCE(error_message, ''),
			COALESCE(retry_of::TEXT, '')
		FROM kitchen_run_queue WHERE 1=1`

	args := []any{}
	argN := 0

	if f.RepoName != "" {
		argN++
		query += fmt.Sprintf(" AND git_repo_name = $%d", argN)
		args = append(args, f.RepoName)
	}
	if f.RunType != "" {
		argN++
		query += fmt.Sprintf(" AND run_type = $%d", argN)
		args = append(args, f.RunType)
	}
	if len(f.Statuses) > 0 {
		argN++
		query += fmt.Sprintf(" AND status = ANY($%d::TEXT[])", argN)
		args = append(args, f.Statuses)
	}

	query += " ORDER BY priority DESC, enqueued_at DESC"

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	argN++
	query += fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: list kitchen queue: %w", err)
	}
	defer rows.Close()

	var items []KitchenQueueItem
	for rows.Next() {
		var item KitchenQueueItem
		err := rows.Scan(
			&item.ID, &item.RunType, &item.GitRepoName, &item.GitRepoURL,
			&item.SuiteName, &item.PlatformName, &item.InstanceName,
			&item.TargetChefVersion, &item.HeadCommitSHA, &item.NodeName,
			&item.OrganisationName, &item.CookbookSource,
			&item.BatchID, &item.Priority, &item.Status, &item.EnqueuedAt,
			&item.StartedAt, &item.CompletedAt, &item.ErrorMessage,
			&item.RetryOf,
		)
		if err != nil {
			return nil, fmt.Errorf("datastore: scan kitchen queue item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetKitchenQueueItem returns a single queue item by ID.
func (db *DB) GetKitchenQueueItem(ctx context.Context, id string) (*KitchenQueueItem, error) {
	var item KitchenQueueItem
	err := db.pool.QueryRowContext(ctx, `
		SELECT id, run_type, COALESCE(git_repo_name, ''), COALESCE(git_repo_url, ''),
			COALESCE(suite_name, ''), COALESCE(platform_name, ''), COALESCE(instance_name, ''),
			target_chef_version, COALESCE(head_commit_sha, ''), COALESCE(node_name, ''),
			COALESCE(organisation_name, ''), COALESCE(cookbook_source, ''),
			COALESCE(batch_id::TEXT, ''), priority, status, enqueued_at,
			started_at, completed_at, COALESCE(error_message, ''),
			COALESCE(output, ''), COALESCE(retry_of::TEXT, '')
		FROM kitchen_run_queue WHERE id = $1`, id).Scan(
		&item.ID, &item.RunType, &item.GitRepoName, &item.GitRepoURL,
		&item.SuiteName, &item.PlatformName, &item.InstanceName,
		&item.TargetChefVersion, &item.HeadCommitSHA, &item.NodeName,
		&item.OrganisationName, &item.CookbookSource,
		&item.BatchID, &item.Priority, &item.Status, &item.EnqueuedAt,
		&item.StartedAt, &item.CompletedAt, &item.ErrorMessage,
		&item.Output, &item.RetryOf,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: get kitchen queue item: %w", err)
	}
	return &item, nil
}

// KitchenQueueStats holds summary counts for the queue.
type KitchenQueueStats struct {
	Queued  int `json:"queued"`
	Running int `json:"running"`
}

// GetKitchenQueueStats returns counts of queued and running items.
func (db *DB) GetKitchenQueueStats(ctx context.Context) (*KitchenQueueStats, error) {
	var stats KitchenQueueStats
	err := db.pool.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'queued'),
			COUNT(*) FILTER (WHERE status = 'running')
		FROM kitchen_run_queue`).Scan(&stats.Queued, &stats.Running)
	if err != nil {
		return nil, fmt.Errorf("datastore: kitchen queue stats: %w", err)
	}
	return &stats, nil
}

// ---------------------------------------------------------------------------
// Startup recovery
// ---------------------------------------------------------------------------

// MarkInterruptedKitchenRuns transitions all running items to interrupted.
// Called on startup to handle items that were in-flight during a crash.
// Returns the number of items marked.
func (db *DB) MarkInterruptedKitchenRuns(ctx context.Context) (int64, error) {
	result, err := db.pool.ExecContext(ctx, `
		UPDATE kitchen_run_queue
		SET status = 'interrupted', completed_at = now(),
		    error_message = 'interrupted: application restarted while running'
		WHERE status = 'running'`)
	if err != nil {
		return 0, fmt.Errorf("datastore: mark interrupted kitchen runs: %w", err)
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func nullEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func checkRowsAffected(result sql.Result, id string) error {
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: queue item %s", ErrNotFound, id)
	}
	return nil
}
