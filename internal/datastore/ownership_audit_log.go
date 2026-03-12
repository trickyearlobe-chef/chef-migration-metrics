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

// OwnershipAuditEntry represents a row in the ownership_audit_log table.
type OwnershipAuditEntry struct {
	ID           string          `json:"id"`
	Timestamp    time.Time       `json:"timestamp"`
	Action       string          `json:"action"`
	Actor        string          `json:"actor"`
	OwnerName    string          `json:"owner_name"`
	EntityType   string          `json:"entity_type,omitempty"`
	EntityKey    string          `json:"entity_key,omitempty"`
	Organisation string          `json:"organisation,omitempty"`
	Details      json.RawMessage `json:"details,omitempty"`
}

// InsertAuditEntryParams holds the fields for a new audit log entry.
type InsertAuditEntryParams struct {
	Action       string
	Actor        string
	OwnerName    string
	EntityType   string // empty for owner-level actions
	EntityKey    string
	Organisation string
	Details      json.RawMessage
}

// AuditLogFilter holds query parameters for listing audit log entries.
type AuditLogFilter struct {
	Action     string
	Actor      string
	OwnerName  string
	EntityType string
	EntityKey  string
	Since      time.Time
	Until      time.Time
	Limit      int
	Offset     int
}

// InsertAuditEntry creates a new audit log entry.
func (db *DB) InsertAuditEntry(ctx context.Context, p InsertAuditEntryParams) error {
	return db.insertAuditEntry(ctx, db.q(), p)
}

// InsertAuditEntryTx creates a new audit log entry within an existing
// transaction.
func (db *DB) InsertAuditEntryTx(ctx context.Context, tx *sql.Tx, p InsertAuditEntryParams) error {
	return db.insertAuditEntry(ctx, tx, p)
}

func (db *DB) insertAuditEntry(ctx context.Context, q queryable, p InsertAuditEntryParams) error {
	const query = `
		INSERT INTO ownership_audit_log
			(action, actor, owner_name, entity_type, entity_key, organisation, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	var detailsBytes []byte
	if p.Details != nil {
		detailsBytes = p.Details
	}

	_, err := q.ExecContext(ctx, query,
		p.Action,
		p.Actor,
		p.OwnerName,
		nullString(p.EntityType),
		nullString(p.EntityKey),
		nullString(p.Organisation),
		detailsBytes,
	)
	if err != nil {
		return fmt.Errorf("datastore: inserting audit entry: %w", err)
	}
	return nil
}

// ListAuditLog returns audit log entries matching the given filter, in
// reverse chronological order.
func (db *DB) ListAuditLog(ctx context.Context, f AuditLogFilter) ([]OwnershipAuditEntry, int, error) {
	return db.listAuditLog(ctx, db.q(), f)
}

func (db *DB) listAuditLog(ctx context.Context, q queryable, f AuditLogFilter) ([]OwnershipAuditEntry, int, error) {
	where := "WHERE 1=1"
	args := []any{}
	argN := 1

	if f.Action != "" {
		where += fmt.Sprintf(" AND action = $%d", argN)
		args = append(args, f.Action)
		argN++
	}
	if f.Actor != "" {
		where += fmt.Sprintf(" AND actor = $%d", argN)
		args = append(args, f.Actor)
		argN++
	}
	if f.OwnerName != "" {
		where += fmt.Sprintf(" AND owner_name = $%d", argN)
		args = append(args, f.OwnerName)
		argN++
	}
	if f.EntityType != "" {
		where += fmt.Sprintf(" AND entity_type = $%d", argN)
		args = append(args, f.EntityType)
		argN++
	}
	if f.EntityKey != "" {
		where += fmt.Sprintf(" AND entity_key = $%d", argN)
		args = append(args, f.EntityKey)
		argN++
	}
	if !f.Since.IsZero() {
		where += fmt.Sprintf(" AND timestamp >= $%d", argN)
		args = append(args, f.Since)
		argN++
	}
	if !f.Until.IsZero() {
		where += fmt.Sprintf(" AND timestamp <= $%d", argN)
		args = append(args, f.Until)
		argN++
	}

	// Count.
	countQuery := "SELECT COUNT(*) FROM ownership_audit_log " + where
	var total int
	if err := q.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("datastore: counting audit log entries: %w", err)
	}

	// Fetch page.
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	dataQuery := fmt.Sprintf(`
		SELECT id, timestamp, action, actor, owner_name,
		       entity_type, entity_key, organisation, details
		FROM ownership_audit_log
		%s
		ORDER BY timestamp DESC
		LIMIT $%d OFFSET $%d
	`, where, argN, argN+1)
	args = append(args, limit, offset)

	rows, err := q.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: listing audit log: %w", err)
	}
	defer rows.Close()

	var entries []OwnershipAuditEntry
	for rows.Next() {
		var e OwnershipAuditEntry
		var entityType, entityKey, organisation sql.NullString
		var details []byte

		if err := rows.Scan(
			&e.ID,
			&e.Timestamp,
			&e.Action,
			&e.Actor,
			&e.OwnerName,
			&entityType,
			&entityKey,
			&organisation,
			&details,
		); err != nil {
			return nil, 0, fmt.Errorf("datastore: scanning audit log row: %w", err)
		}

		e.EntityType = stringFromNull(entityType)
		e.EntityKey = stringFromNull(entityKey)
		e.Organisation = stringFromNull(organisation)
		if details != nil {
			e.Details = json.RawMessage(details)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("datastore: iterating audit log rows: %w", err)
	}
	return entries, total, nil
}

// PurgeAuditLog deletes audit log entries older than the given cutoff time.
// Returns the number of deleted rows.
func (db *DB) PurgeAuditLog(ctx context.Context, olderThan time.Time) (int, error) {
	return db.purgeAuditLog(ctx, db.q(), olderThan)
}

func (db *DB) purgeAuditLog(ctx context.Context, q queryable, olderThan time.Time) (int, error) {
	res, err := q.ExecContext(ctx,
		`DELETE FROM ownership_audit_log WHERE timestamp < $1`,
		olderThan,
	)
	if err != nil {
		return 0, fmt.Errorf("datastore: purging audit log: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	return int(n), nil
}
