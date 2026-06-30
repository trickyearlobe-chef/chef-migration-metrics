// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"
)

// Completion markers for one-time, idempotent Go-side data backfills (table
// schema_backfills, migration 0043). A named backfill writes its marker once it
// has run; the boot routine checks the marker and skips the work thereafter.
// The backfills themselves are idempotent (re-deriving precise data is a no-op),
// so the marker is purely a cheap-skip gate, not a correctness guard.

// BackfillCompleted reports whether the named backfill has already run.
func (db *DB) BackfillCompleted(ctx context.Context, name string) (bool, error) {
	const query = `SELECT EXISTS (SELECT 1 FROM schema_backfills WHERE name = $1)`
	var exists bool
	if err := db.q().QueryRowContext(ctx, query, name).Scan(&exists); err != nil {
		return false, fmt.Errorf("datastore: checking backfill marker %q: %w", name, err)
	}
	return exists, nil
}

// MarkBackfillCompleted records that the named backfill has run. It is safe to
// call more than once — a duplicate marker is a no-op.
func (db *DB) MarkBackfillCompleted(ctx context.Context, name string) error {
	const query = `INSERT INTO schema_backfills (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`
	if _, err := db.q().ExecContext(ctx, query, name); err != nil {
		return fmt.Errorf("datastore: marking backfill %q complete: %w", name, err)
	}
	return nil
}
