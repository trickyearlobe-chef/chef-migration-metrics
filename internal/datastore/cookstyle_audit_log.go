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

// CookstyleAuditEntry is a row in the cookstyle_audit_log table — a record of a
// criteria change (cop reclassification or custom-cop change) that triggered the
// re-evaluation propagation closure.
type CookstyleAuditEntry struct {
	ID                string          `json:"id"`
	Timestamp         time.Time       `json:"timestamp"`
	Action            string          `json:"action"`
	Actor             string          `json:"actor"`
	CopName           string          `json:"cop_name"`
	TargetChefVersion string          `json:"target_chef_version,omitempty"`
	Details           json.RawMessage `json:"details,omitempty"`
}

// InsertCookstyleAuditParams holds the fields for a new cookstyle audit entry.
type InsertCookstyleAuditParams struct {
	Action            string
	Actor             string
	CopName           string
	TargetChefVersion string // empty for target-agnostic changes (e.g. custom cops)
	Details           json.RawMessage
}

// InsertCookstyleAuditEntry records a cookstyle criteria-change audit entry.
func (db *DB) InsertCookstyleAuditEntry(ctx context.Context, p InsertCookstyleAuditParams) error {
	const query = `
		INSERT INTO cookstyle_audit_log
			(action, actor, cop_name, target_chef_version, details)
		VALUES ($1, $2, $3, $4, $5)`

	var details []byte
	if p.Details != nil {
		details = p.Details
	}

	if _, err := db.q().ExecContext(ctx, query,
		p.Action, p.Actor, p.CopName, nullString(p.TargetChefVersion), details,
	); err != nil {
		return fmt.Errorf("datastore: inserting cookstyle audit entry: %w", err)
	}
	return nil
}

// ListCookstyleAuditLog returns cookstyle audit entries in reverse chronological
// order, optionally filtered by cop name. limit <= 0 defaults to 50.
func (db *DB) ListCookstyleAuditLog(ctx context.Context, copName string, limit int) ([]CookstyleAuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	where := ""
	args := []any{}
	if copName != "" {
		where = "WHERE cop_name = $1"
		args = append(args, copName)
	}
	query := fmt.Sprintf(`
		SELECT id, timestamp, action, actor, cop_name,
		       COALESCE(target_chef_version, ''), details
		FROM cookstyle_audit_log
		%s
		ORDER BY timestamp DESC
		LIMIT $%d`, where, len(args)+1)
	args = append(args, limit)

	rows, err := db.q().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing cookstyle audit log: %w", err)
	}
	defer rows.Close()

	var entries []CookstyleAuditEntry
	for rows.Next() {
		var e CookstyleAuditEntry
		var details []byte
		var tcv sql.NullString
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Action, &e.Actor, &e.CopName, &tcv, &details); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookstyle audit row: %w", err)
		}
		e.TargetChefVersion = tcv.String
		if details != nil {
			e.Details = json.RawMessage(details)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookstyle audit rows: %w", err)
	}
	return entries, nil
}
