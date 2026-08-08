// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"
	"time"
)

// maxStoredRejections bounds what one import keeps.
//
// A rejection list is read by a person who is going to go and fix the rows, so
// past a certain size it stops being a work list and becomes a second copy of
// the source. The cap is reported alongside the rows rather than applied
// silently — "1000 of 40000 shown" and "1000 rejections" are very different
// statements about a source's quality.
const maxStoredRejections = 1000

// ImportRejection is one row an import could not use.
//
// The most direct statement of source data quality there is: this row names a
// person who is not in the staff table, that one has no asset name at all.
type ImportRejection struct {
	ImportLabel string    `json:"import_label"`
	RunAt       time.Time `json:"run_at"`
	SourceRow   int       `json:"source_row"`
	Reason      string    `json:"reason"`
	OwnerRaw    string    `json:"owner_raw"`
	EntityType  string    `json:"entity_type"`
	EntityKey   string    `json:"entity_key"`
}

// ReplaceImportRejections swaps the stored rejections for one import.
//
// Replace rather than append: a rejection is a statement about the source as it
// stands, not a history of it. Once a row is fixed at source it stops being a
// problem, and a table that accumulated every run would report fixed rows
// forever alongside real ones — which is how a work list stops being worked.
//
// One transaction, so a failure part way through cannot leave the previous
// run's findings deleted and the new ones missing, which would read as a source
// that had suddenly become clean.
//
// Returns the number stored, which is less than len(rejections) when the cap
// bites. The caller reports the difference; silently keeping the first thousand
// would make a bad source look mediocre.
func (db *DB) ReplaceImportRejections(ctx context.Context, label string, mappingID *int64, rejections []ImportRejection) (int, error) {
	if label == "" {
		return 0, fmt.Errorf("datastore: an import label is required to store rejections")
	}

	tx, err := db.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("datastore: starting the rejection swap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM ownership_import_rejections WHERE import_label = $1`, label); err != nil {
		return 0, fmt.Errorf("datastore: clearing previous rejections for %q: %w", label, err)
	}

	stored := rejections
	if len(stored) > maxStoredRejections {
		stored = stored[:maxStoredRejections]
	}

	for _, r := range stored {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ownership_import_rejections
				(mapping_id, import_label, source_row, reason, owner_raw, entity_type, entity_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			mappingID, label, r.SourceRow, r.Reason, r.OwnerRaw, r.EntityType, r.EntityKey); err != nil {
			return 0, fmt.Errorf("datastore: storing a rejection for %q: %w", label, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("datastore: committing the rejection swap: %w", err)
	}
	return len(stored), nil
}

// ListImportRejections returns stored rejections a page at a time, in source
// order within each import, so the file reads against the source.
func (db *DB) ListImportRejections(ctx context.Context, limit, offset int) ([]ImportRejection, error) {
	const query = `
		SELECT import_label, run_at, source_row, reason, owner_raw, entity_type, entity_key
		FROM ownership_import_rejections
		ORDER BY import_label, source_row
		LIMIT $1 OFFSET $2`

	rows, err := db.pool.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing import rejections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ImportRejection
	for rows.Next() {
		var r ImportRejection
		if err := rows.Scan(&r.ImportLabel, &r.RunAt, &r.SourceRow, &r.Reason,
			&r.OwnerRaw, &r.EntityType, &r.EntityKey); err != nil {
			return nil, fmt.Errorf("datastore: scanning an import rejection: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
