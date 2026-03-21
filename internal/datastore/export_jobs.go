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

// ExportJob represents a row in the export_jobs table. Each record tracks
// a data export request through its lifecycle: pending → processing →
// completed/failed/expired.
type ExportJob struct {
	ID            string          `json:"id"`
	ExportType    string          `json:"export_type"` // ready_nodes, blocked_nodes, cookbook_remediation
	Format        string          `json:"format"`      // csv, json, chef_search_query
	Filters       json.RawMessage `json:"filters"`     // JSONB — caller-specified filters
	Status        string          `json:"status"`      // pending, processing, completed, failed, expired
	RowCount      int             `json:"row_count"`
	FilePath      string          `json:"file_path"`
	FileSizeBytes int64           `json:"file_size_bytes"`
	ErrorMessage  string          `json:"error_message,omitempty"`
	RequestedBy   string          `json:"requested_by,omitempty"`
	RequestedAt   time.Time       `json:"requested_at"`
	CompletedAt   time.Time       `json:"completed_at,omitempty"`
	ExpiresAt     time.Time       `json:"expires_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Export job status constants.
const (
	ExportStatusPending    = "pending"
	ExportStatusProcessing = "processing"
	ExportStatusCompleted  = "completed"
	ExportStatusFailed     = "failed"
	ExportStatusExpired    = "expired"
)

// Export type constants.
const (
	ExportTypeReadyNodes          = "ready_nodes"
	ExportTypeBlockedNodes        = "blocked_nodes"
	ExportTypeCookbookRemediation = "cookbook_remediation"
)

// Export format constants.
const (
	ExportFormatCSV             = "csv"
	ExportFormatJSON            = "json"
	ExportFormatChefSearchQuery = "chef_search_query"
)

// InsertExportJobParams contains the fields needed to insert a new pending
// export job.
type InsertExportJobParams struct {
	ExportType  string
	Format      string
	Filters     json.RawMessage
	RequestedBy string
	ExpiresAt   time.Time
}

// ---------------------------------------------------------------------------
// Column list — shared across all queries
// ---------------------------------------------------------------------------

const ejColumns = `id, export_type, format, filters, status,
       row_count, file_path, file_size_bytes, error_message,
       requested_by, requested_at, completed_at, expires_at, created_at`

// ---------------------------------------------------------------------------
// Insert
// ---------------------------------------------------------------------------

// InsertExportJob creates a new export job in pending status and returns it.
func (db *DB) InsertExportJob(ctx context.Context, p InsertExportJobParams) (*ExportJob, error) {
	if p.ExportType == "" {
		return nil, fmt.Errorf("datastore: export_type is required")
	}
	if p.Format == "" {
		return nil, fmt.Errorf("datastore: format is required")
	}

	query := `
		INSERT INTO export_jobs (
			export_type, format, filters, status,
			requested_by, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + ejColumns + `
	`

	filters := nullJSON(p.Filters)

	r, err := scanExportJob(db.q().QueryRowContext(ctx, query,
		p.ExportType,
		p.Format,
		filters,
		ExportStatusPending,
		nullString(p.RequestedBy),
		nullTime(p.ExpiresAt),
	))
	if err != nil {
		return nil, fmt.Errorf("datastore: inserting export job: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetExportJob returns a single export job by its primary key UUID.
// Returns ErrNotFound if no such job exists.
func (db *DB) GetExportJob(ctx context.Context, id string) (*ExportJob, error) {
	if id == "" {
		return nil, fmt.Errorf("datastore: export job id is required")
	}

	query := `
		SELECT ` + ejColumns + `
		  FROM export_jobs
		 WHERE id = $1
	`

	r, err := scanExportJob(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting export job: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Update status transitions
// ---------------------------------------------------------------------------

// UpdateExportJobStatus updates a job's status and associated result fields.
// This is used for processing → completed or processing → failed transitions.
func (db *DB) UpdateExportJobStatus(ctx context.Context, id, status string, rowCount int, filePath string, fileSizeBytes int64, errorMessage string) error {
	if id == "" {
		return fmt.Errorf("datastore: export job id is required")
	}
	if status == "" {
		return fmt.Errorf("datastore: status is required")
	}

	var completedAt sql.NullTime
	if status == ExportStatusCompleted || status == ExportStatusFailed {
		completedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	}

	query := `
		UPDATE export_jobs
		   SET status          = $1,
		       row_count       = $2,
		       file_path       = $3,
		       file_size_bytes = $4,
		       error_message   = $5,
		       completed_at    = $6
		 WHERE id = $7
	`

	res, err := db.pool.ExecContext(ctx, query,
		status,
		rowCount,
		nullString(filePath),
		fileSizeBytes,
		nullString(errorMessage),
		completedAt,
		id,
	)
	if err != nil {
		return fmt.Errorf("datastore: updating export job status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateExportJobExpired marks a completed export job as expired. This is
// called by the cleanup worker after the file has been deleted.
func (db *DB) UpdateExportJobExpired(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("datastore: export job id is required")
	}

	query := `
		UPDATE export_jobs
		   SET status = $1
		 WHERE id = $2
		   AND status = $3
	`

	res, err := db.pool.ExecContext(ctx, query, ExportStatusExpired, id, ExportStatusCompleted)
	if err != nil {
		return fmt.Errorf("datastore: marking export job expired: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListExportJobsByStatus returns all export jobs with the given status,
// ordered by requested_at descending.
func (db *DB) ListExportJobsByStatus(ctx context.Context, status string) ([]ExportJob, error) {
	query := `
		SELECT ` + ejColumns + `
		  FROM export_jobs
		 WHERE status = $1
		 ORDER BY requested_at DESC
	`
	return db.scanExportJobRows(ctx, query, status)
}

// ListExpiredExportJobs returns all completed export jobs whose expires_at
// is before the given time. These are candidates for file cleanup.
func (db *DB) ListExpiredExportJobs(ctx context.Context, now time.Time) ([]ExportJob, error) {
	query := `
		SELECT ` + ejColumns + `
		  FROM export_jobs
		 WHERE status = $1
		   AND expires_at IS NOT NULL
		   AND expires_at < $2
		 ORDER BY expires_at ASC
	`
	return db.scanExportJobRows(ctx, query, ExportStatusCompleted, now)
}

// ListExportJobsByRequester returns recent export jobs for the given
// requester, ordered by requested_at descending, limited to the given count.
func (db *DB) ListExportJobsByRequester(ctx context.Context, requestedBy string, limit int) ([]ExportJob, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT ` + ejColumns + `
		  FROM export_jobs
		 WHERE requested_by = $1
		 ORDER BY requested_at DESC
		 LIMIT $2
	`
	return db.scanExportJobRows(ctx, query, requestedBy, limit)
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanExportJob(row interface{ Scan(dest ...any) error }) (ExportJob, error) {
	var j ExportJob
	var filters []byte
	var filePath, errorMessage, requestedBy sql.NullString
	var completedAt, expiresAt sql.NullTime
	var fileSizeBytes sql.NullInt64

	err := row.Scan(
		&j.ID,
		&j.ExportType,
		&j.Format,
		&filters,
		&j.Status,
		&j.RowCount,
		&filePath,
		&fileSizeBytes,
		&errorMessage,
		&requestedBy,
		&j.RequestedAt,
		&completedAt,
		&expiresAt,
		&j.CreatedAt,
	)
	if err != nil {
		return ExportJob{}, err
	}

	j.Filters = jsonFromNullBytes(filters)
	j.FilePath = stringFromNull(filePath)
	j.FileSizeBytes = fileSizeBytes.Int64
	j.ErrorMessage = stringFromNull(errorMessage)
	j.RequestedBy = stringFromNull(requestedBy)
	j.CompletedAt = timeFromNull(completedAt)
	j.ExpiresAt = timeFromNull(expiresAt)

	return j, nil
}

func (db *DB) scanExportJobRows(ctx context.Context, query string, args ...any) ([]ExportJob, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing export jobs: %w", err)
	}
	defer rows.Close()

	var results []ExportJob
	for rows.Next() {
		var j ExportJob
		var filters []byte
		var filePath, errorMessage, requestedBy sql.NullString
		var completedAt, expiresAt sql.NullTime
		var fileSizeBytes sql.NullInt64

		if err := rows.Scan(
			&j.ID,
			&j.ExportType,
			&j.Format,
			&filters,
			&j.Status,
			&j.RowCount,
			&filePath,
			&fileSizeBytes,
			&errorMessage,
			&requestedBy,
			&j.RequestedAt,
			&completedAt,
			&expiresAt,
			&j.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning export job row: %w", err)
		}

		j.Filters = jsonFromNullBytes(filters)
		j.FilePath = stringFromNull(filePath)
		j.FileSizeBytes = fileSizeBytes.Int64
		j.ErrorMessage = stringFromNull(errorMessage)
		j.RequestedBy = stringFromNull(requestedBy)
		j.CompletedAt = timeFromNull(completedAt)
		j.ExpiresAt = timeFromNull(expiresAt)

		results = append(results, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating export job rows: %w", err)
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Purge
// ---------------------------------------------------------------------------

// DeleteExpiredExportJobRows removes export_jobs rows that have been marked
// as 'expired' (files already cleaned up from disk) and whose expires_at
// timestamp is older than the given cutoff. This prevents indefinite row
// accumulation while retaining recent expired rows for audit visibility.
// Returns the count of deleted rows.
func (db *DB) DeleteExpiredExportJobRows(ctx context.Context, olderThan time.Time) (int, error) {
	const query = `DELETE FROM export_jobs WHERE status = 'expired' AND expires_at < $1`
	res, err := db.pool.ExecContext(ctx, query, olderThan)
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting expired export job rows: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	return int(n), nil
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

// ValidExportType returns true if the given string is a recognised export type.
func ValidExportType(t string) bool {
	switch t {
	case ExportTypeReadyNodes, ExportTypeBlockedNodes, ExportTypeCookbookRemediation:
		return true
	}
	return false
}

// ValidExportFormat returns true if the given string is a recognised export format.
func ValidExportFormat(f string) bool {
	switch f {
	case ExportFormatCSV, ExportFormatJSON, ExportFormatChefSearchQuery:
		return true
	}
	return false
}
