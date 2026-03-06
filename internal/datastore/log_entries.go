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

// LogEntry represents a row in the log_entries table. Each entry is a single
// structured log event produced by the application.
type LogEntry struct {
	ID                  string    `json:"id"`
	Timestamp           time.Time `json:"timestamp"`
	Severity            string    `json:"severity"`
	Scope               string    `json:"scope"`
	Message             string    `json:"message"`
	Organisation        string    `json:"organisation,omitempty"`
	CookbookName        string    `json:"cookbook_name,omitempty"`
	CookbookVersion     string    `json:"cookbook_version,omitempty"`
	CommitSHA           string    `json:"commit_sha,omitempty"`
	ChefClientVersion   string    `json:"chef_client_version,omitempty"`
	ProcessOutput       string    `json:"process_output,omitempty"`
	CollectionRunID     string    `json:"collection_run_id,omitempty"`
	NotificationChannel string    `json:"notification_channel,omitempty"`
	ExportJobID         string    `json:"export_job_id,omitempty"`
	TLSDomain           string    `json:"tls_domain,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

// MarshalJSON implements json.Marshaler for LogEntry.
func (le LogEntry) MarshalJSON() ([]byte, error) {
	type Alias LogEntry
	return json.Marshal((Alias)(le))
}

// ---------------------------------------------------------------------------
// Insert
// ---------------------------------------------------------------------------

// InsertLogEntryParams holds the fields required to insert a log entry.
type InsertLogEntryParams struct {
	Timestamp           time.Time
	Severity            string
	Scope               string
	Message             string
	Organisation        string
	CookbookName        string
	CookbookVersion     string
	CommitSHA           string
	ChefClientVersion   string
	ProcessOutput       string
	CollectionRunID     string
	NotificationChannel string
	ExportJobID         string
	TLSDomain           string
}

// validateLogEntryParams checks that all required fields are present and that
// severity is one of the allowed values.
func validateLogEntryParams(p InsertLogEntryParams) error {
	if p.Timestamp.IsZero() {
		return fmt.Errorf("datastore: timestamp is required for log entry")
	}
	switch p.Severity {
	case "DEBUG", "INFO", "WARN", "ERROR":
		// valid
	default:
		return fmt.Errorf("datastore: invalid severity %q for log entry (must be DEBUG, INFO, WARN, or ERROR)", p.Severity)
	}
	if p.Scope == "" {
		return fmt.Errorf("datastore: scope is required for log entry")
	}
	if p.Message == "" {
		return fmt.Errorf("datastore: message is required for log entry")
	}
	return nil
}

// InsertLogEntry inserts a single log entry and returns the created row.
func (db *DB) InsertLogEntry(ctx context.Context, p InsertLogEntryParams) (LogEntry, error) {
	return db.insertLogEntry(ctx, db.q(), p)
}

func (db *DB) insertLogEntry(ctx context.Context, q queryable, p InsertLogEntryParams) (LogEntry, error) {
	if err := validateLogEntryParams(p); err != nil {
		return LogEntry{}, err
	}

	const query = `
		INSERT INTO log_entries (
			timestamp, severity, scope, message,
			organisation, cookbook_name, cookbook_version,
			commit_sha, chef_client_version, process_output,
			collection_run_id, notification_channel, export_job_id,
			tls_domain
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10,
			$11, $12, $13,
			$14
		)
		RETURNING id, timestamp, severity, scope, message,
		          organisation, cookbook_name, cookbook_version,
		          commit_sha, chef_client_version, process_output,
		          collection_run_id, notification_channel, export_job_id,
		          tls_domain, created_at
	`

	return scanLogEntry(q.QueryRowContext(ctx, query,
		p.Timestamp,
		p.Severity,
		p.Scope,
		p.Message,
		nullString(p.Organisation),
		nullString(p.CookbookName),
		nullString(p.CookbookVersion),
		nullString(p.CommitSHA),
		nullString(p.ChefClientVersion),
		nullString(p.ProcessOutput),
		nullStringUUID(p.CollectionRunID),
		nullString(p.NotificationChannel),
		nullString(p.ExportJobID),
		nullString(p.TLSDomain),
	))
}

// BulkInsertLogEntries inserts multiple log entries within a single
// transaction for efficiency. Returns the number of rows inserted.
func (db *DB) BulkInsertLogEntries(ctx context.Context, entries []InsertLogEntryParams) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}

	var count int
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		const query = `
			INSERT INTO log_entries (
				timestamp, severity, scope, message,
				organisation, cookbook_name, cookbook_version,
				commit_sha, chef_client_version, process_output,
				collection_run_id, notification_channel, export_job_id,
				tls_domain
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7,
				$8, $9, $10,
				$11, $12, $13,
				$14
			)
		`

		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return fmt.Errorf("preparing log entry insert: %w", err)
		}
		defer stmt.Close()

		for i, p := range entries {
			if err := validateLogEntryParams(p); err != nil {
				return fmt.Errorf("entry %d: %w", i, err)
			}

			_, err := stmt.ExecContext(ctx,
				p.Timestamp,
				p.Severity,
				p.Scope,
				p.Message,
				nullString(p.Organisation),
				nullString(p.CookbookName),
				nullString(p.CookbookVersion),
				nullString(p.CommitSHA),
				nullString(p.ChefClientVersion),
				nullString(p.ProcessOutput),
				nullStringUUID(p.CollectionRunID),
				nullString(p.NotificationChannel),
				nullString(p.ExportJobID),
				nullString(p.TLSDomain),
			)
			if err != nil {
				return fmt.Errorf("inserting entry %d: %w", i, err)
			}
			count++
		}
		return nil
	})
	return count, err
}

// ---------------------------------------------------------------------------
// Query
// ---------------------------------------------------------------------------

// GetLogEntry retrieves a single log entry by ID.
func (db *DB) GetLogEntry(ctx context.Context, id string) (LogEntry, error) {
	if id == "" {
		return LogEntry{}, fmt.Errorf("datastore: log entry ID is required")
	}

	const query = `
		SELECT id, timestamp, severity, scope, message,
		       organisation, cookbook_name, cookbook_version,
		       commit_sha, chef_client_version, process_output,
		       collection_run_id, notification_channel, export_job_id,
		       tls_domain, created_at
		FROM log_entries
		WHERE id = $1
	`

	entry, err := scanLogEntry(db.q().QueryRowContext(ctx, query, id))
	if err != nil {
		return LogEntry{}, err
	}
	return entry, nil
}

// LogEntryFilter specifies criteria for querying log entries. All non-zero
// fields are combined with AND. An empty filter returns all entries.
type LogEntryFilter struct {
	// Scope filters by the log scope.
	Scope string

	// Severity filters by exact severity level.
	Severity string

	// MinSeverity filters entries at or above the given severity (DEBUG=0,
	// INFO=1, WARN=2, ERROR=3). This is mutually exclusive with Severity.
	MinSeverity string

	// Organisation filters by organisation name.
	Organisation string

	// CookbookName filters by cookbook name.
	CookbookName string

	// CollectionRunID filters by the associated collection run.
	CollectionRunID string

	// Since filters entries with timestamp >= Since.
	Since time.Time

	// Until filters entries with timestamp < Until.
	Until time.Time

	// Limit caps the number of returned entries. 0 means no limit.
	Limit int

	// Offset is the number of entries to skip (for pagination).
	Offset int
}

// severityOrdinal returns the numeric ordinal for a severity string, or -1
// if the severity is not recognised.
func severityOrdinal(s string) int {
	switch s {
	case "DEBUG":
		return 0
	case "INFO":
		return 1
	case "WARN":
		return 2
	case "ERROR":
		return 3
	default:
		return -1
	}
}

// minSeverityValues returns the list of severity values at or above the
// given minimum severity. Returns nil if the severity is not recognised.
func minSeverityValues(minSev string) []string {
	ord := severityOrdinal(minSev)
	if ord < 0 {
		return nil
	}
	all := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	return all[ord:]
}

// ListLogEntries retrieves log entries matching the given filter, ordered by
// timestamp descending (newest first).
func (db *DB) ListLogEntries(ctx context.Context, f LogEntryFilter) ([]LogEntry, error) {
	query := `
		SELECT id, timestamp, severity, scope, message,
		       organisation, cookbook_name, cookbook_version,
		       commit_sha, chef_client_version, process_output,
		       collection_run_id, notification_channel, export_job_id,
		       tls_domain, created_at
		FROM log_entries
		WHERE 1=1
	`
	args := []interface{}{}
	argN := 0

	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	if f.Scope != "" {
		query += " AND scope = " + nextArg()
		args = append(args, f.Scope)
	}
	if f.Severity != "" {
		query += " AND severity = " + nextArg()
		args = append(args, f.Severity)
	} else if f.MinSeverity != "" {
		values := minSeverityValues(f.MinSeverity)
		if len(values) > 0 {
			query += " AND severity = ANY(" + nextArg() + ")"
			args = append(args, stringSliceToArray(values))
		}
	}
	if f.Organisation != "" {
		query += " AND organisation = " + nextArg()
		args = append(args, f.Organisation)
	}
	if f.CookbookName != "" {
		query += " AND cookbook_name = " + nextArg()
		args = append(args, f.CookbookName)
	}
	if f.CollectionRunID != "" {
		query += " AND collection_run_id = " + nextArg()
		args = append(args, f.CollectionRunID)
	}
	if !f.Since.IsZero() {
		query += " AND timestamp >= " + nextArg()
		args = append(args, f.Since)
	}
	if !f.Until.IsZero() {
		query += " AND timestamp < " + nextArg()
		args = append(args, f.Until)
	}

	query += " ORDER BY timestamp DESC"

	if f.Limit > 0 {
		query += " LIMIT " + nextArg()
		args = append(args, f.Limit)
	}
	if f.Offset > 0 {
		query += " OFFSET " + nextArg()
		args = append(args, f.Offset)
	}

	rows, err := db.q().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing log entries: %w", err)
	}
	defer rows.Close()

	var entries []LogEntry
	for rows.Next() {
		entry, err := scanLogEntryRow(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating log entries: %w", err)
	}
	return entries, nil
}

// CountLogEntries returns the total number of log entries matching the given
// filter (ignoring Limit and Offset).
func (db *DB) CountLogEntries(ctx context.Context, f LogEntryFilter) (int, error) {
	query := `SELECT COUNT(*) FROM log_entries WHERE 1=1`
	args := []interface{}{}
	argN := 0

	nextArg := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	if f.Scope != "" {
		query += " AND scope = " + nextArg()
		args = append(args, f.Scope)
	}
	if f.Severity != "" {
		query += " AND severity = " + nextArg()
		args = append(args, f.Severity)
	} else if f.MinSeverity != "" {
		values := minSeverityValues(f.MinSeverity)
		if len(values) > 0 {
			query += " AND severity = ANY(" + nextArg() + ")"
			args = append(args, stringSliceToArray(values))
		}
	}
	if f.Organisation != "" {
		query += " AND organisation = " + nextArg()
		args = append(args, f.Organisation)
	}
	if f.CookbookName != "" {
		query += " AND cookbook_name = " + nextArg()
		args = append(args, f.CookbookName)
	}
	if f.CollectionRunID != "" {
		query += " AND collection_run_id = " + nextArg()
		args = append(args, f.CollectionRunID)
	}
	if !f.Since.IsZero() {
		query += " AND timestamp >= " + nextArg()
		args = append(args, f.Since)
	}
	if !f.Until.IsZero() {
		query += " AND timestamp < " + nextArg()
		args = append(args, f.Until)
	}

	var count int
	if err := db.q().QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("datastore: counting log entries: %w", err)
	}
	return count, nil
}

// ListLogEntriesByCollectionRun retrieves all log entries for the given
// collection run, ordered by timestamp ascending (chronological).
func (db *DB) ListLogEntriesByCollectionRun(ctx context.Context, collectionRunID string) ([]LogEntry, error) {
	if collectionRunID == "" {
		return nil, fmt.Errorf("datastore: collection run ID is required")
	}

	const query = `
		SELECT id, timestamp, severity, scope, message,
		       organisation, cookbook_name, cookbook_version,
		       commit_sha, chef_client_version, process_output,
		       collection_run_id, notification_channel, export_job_id,
		       tls_domain, created_at
		FROM log_entries
		WHERE collection_run_id = $1
		ORDER BY timestamp ASC
	`

	rows, err := db.q().QueryContext(ctx, query, collectionRunID)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing log entries by collection run: %w", err)
	}
	defer rows.Close()

	var entries []LogEntry
	for rows.Next() {
		entry, err := scanLogEntryRow(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating log entries: %w", err)
	}
	return entries, nil
}

// ---------------------------------------------------------------------------
// Delete / Retention
// ---------------------------------------------------------------------------

// PurgeLogEntriesBefore deletes all log entries with a timestamp older than
// the given cutoff time. Returns the number of rows deleted.
func (db *DB) PurgeLogEntriesBefore(ctx context.Context, before time.Time) (int64, error) {
	if before.IsZero() {
		return 0, fmt.Errorf("datastore: cutoff time is required for log purge")
	}

	const query = `DELETE FROM log_entries WHERE timestamp < $1`
	result, err := db.q().ExecContext(ctx, query, before)
	if err != nil {
		return 0, fmt.Errorf("datastore: purging log entries: %w", err)
	}
	return result.RowsAffected()
}

// PurgeLogEntriesOlderThanDays deletes all log entries older than the given
// number of days. Returns the number of rows deleted. A retentionDays value
// of 0 or negative deletes nothing and returns an error.
func (db *DB) PurgeLogEntriesOlderThanDays(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, fmt.Errorf("datastore: retention days must be > 0, got %d", retentionDays)
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	return db.PurgeLogEntriesBefore(ctx, cutoff)
}

// DeleteLogEntriesByCollectionRun deletes all log entries associated with
// the given collection run. Returns the number of rows deleted.
func (db *DB) DeleteLogEntriesByCollectionRun(ctx context.Context, collectionRunID string) (int64, error) {
	if collectionRunID == "" {
		return 0, fmt.Errorf("datastore: collection run ID is required")
	}

	const query = `DELETE FROM log_entries WHERE collection_run_id = $1`
	result, err := db.q().ExecContext(ctx, query, collectionRunID)
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting log entries by collection run: %w", err)
	}
	return result.RowsAffected()
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

// scanLogEntry scans a single log entry from a *sql.Row.
func scanLogEntry(row *sql.Row) (LogEntry, error) {
	var le LogEntry
	var (
		organisation        sql.NullString
		cookbookName        sql.NullString
		cookbookVersion     sql.NullString
		commitSHA           sql.NullString
		chefClientVersion   sql.NullString
		processOutput       sql.NullString
		collectionRunID     sql.NullString
		notificationChannel sql.NullString
		exportJobID         sql.NullString
		tlsDomain           sql.NullString
	)

	err := row.Scan(
		&le.ID,
		&le.Timestamp,
		&le.Severity,
		&le.Scope,
		&le.Message,
		&organisation,
		&cookbookName,
		&cookbookVersion,
		&commitSHA,
		&chefClientVersion,
		&processOutput,
		&collectionRunID,
		&notificationChannel,
		&exportJobID,
		&tlsDomain,
		&le.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return LogEntry{}, ErrNotFound
		}
		return LogEntry{}, fmt.Errorf("datastore: scanning log entry: %w", err)
	}

	le.Organisation = stringFromNull(organisation)
	le.CookbookName = stringFromNull(cookbookName)
	le.CookbookVersion = stringFromNull(cookbookVersion)
	le.CommitSHA = stringFromNull(commitSHA)
	le.ChefClientVersion = stringFromNull(chefClientVersion)
	le.ProcessOutput = stringFromNull(processOutput)
	le.CollectionRunID = stringFromNull(collectionRunID)
	le.NotificationChannel = stringFromNull(notificationChannel)
	le.ExportJobID = stringFromNull(exportJobID)
	le.TLSDomain = stringFromNull(tlsDomain)

	return le, nil
}

// scanLogEntryRow scans a single log entry from a *sql.Rows iterator.
func scanLogEntryRow(rows *sql.Rows) (LogEntry, error) {
	var le LogEntry
	var (
		organisation        sql.NullString
		cookbookName        sql.NullString
		cookbookVersion     sql.NullString
		commitSHA           sql.NullString
		chefClientVersion   sql.NullString
		processOutput       sql.NullString
		collectionRunID     sql.NullString
		notificationChannel sql.NullString
		exportJobID         sql.NullString
		tlsDomain           sql.NullString
	)

	err := rows.Scan(
		&le.ID,
		&le.Timestamp,
		&le.Severity,
		&le.Scope,
		&le.Message,
		&organisation,
		&cookbookName,
		&cookbookVersion,
		&commitSHA,
		&chefClientVersion,
		&processOutput,
		&collectionRunID,
		&notificationChannel,
		&exportJobID,
		&tlsDomain,
		&le.CreatedAt,
	)
	if err != nil {
		return LogEntry{}, fmt.Errorf("datastore: scanning log entry row: %w", err)
	}

	le.Organisation = stringFromNull(organisation)
	le.CookbookName = stringFromNull(cookbookName)
	le.CookbookVersion = stringFromNull(cookbookVersion)
	le.CommitSHA = stringFromNull(commitSHA)
	le.ChefClientVersion = stringFromNull(chefClientVersion)
	le.ProcessOutput = stringFromNull(processOutput)
	le.CollectionRunID = stringFromNull(collectionRunID)
	le.NotificationChannel = stringFromNull(notificationChannel)
	le.ExportJobID = stringFromNull(exportJobID)
	le.TLSDomain = stringFromNull(tlsDomain)

	return le, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nullStringUUID converts a string to sql.NullString, treating empty strings
// as NULL. This is used for UUID foreign key columns where the empty string
// is not a valid UUID.
func nullStringUUID(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// stringSliceToArray converts a []string to a pq-compatible array literal
// for use with ANY($N) queries.
type stringArray []string

func stringSliceToArray(s []string) stringArray {
	return stringArray(s)
}

// Value implements the driver.Valuer interface for stringArray, producing a
// PostgreSQL array literal.
func (a stringArray) Value() (interface{}, error) {
	if len(a) == 0 {
		return "{}", nil
	}
	return "{" + joinQuoted(a) + "}", nil
}

// joinQuoted joins strings with commas, quoting each element for use in a
// PostgreSQL array literal.
func joinQuoted(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ","
		}
		result += `"` + s + `"`
	}
	return result
}
