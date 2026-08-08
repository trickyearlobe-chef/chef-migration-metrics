// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"
)

// ClearedOwnership counts what a clear-down removed, so the screen can say what
// happened rather than "done".
type ClearedOwnership struct {
	Assignments int `json:"assignments"`
	Owners      int `json:"owners"`
}

// ClearImportedOwnership removes what imports brought in, and nothing else.
//
// It exists for judging a source: import it, look at what arrived, throw it
// away, adjust the query, import again. Without it the second import is judged
// against the residue of the first.
//
// **What survives, and why.** Hand-made assignments, owners nobody imported,
// aliases, duplicate dismissals and the failure register all cost somebody real
// effort and are not what a trial import dirtied. So does an imported owner who
// has since been given a hand-made assignment: the import brought them in, but
// somebody has since taken responsibility for them, and that is the work this
// must not silently discard.
//
// Owner provenance comes from the audit log rather than a column on `owners`,
// because there is no such column. That makes the audit log load-bearing here:
// an owner whose creation entry has been purged (365-day retention) reads as
// hand-made and is kept. Keeping too much is the right way to be wrong.
//
// One transaction, so a failure part way through cannot leave assignments gone
// and their owners still standing.
func (db *DB) ClearImportedOwnership(ctx context.Context) (ClearedOwnership, error) {
	var cleared ClearedOwnership

	tx, err := db.pool.BeginTx(ctx, nil)
	if err != nil {
		return cleared, fmt.Errorf("datastore: starting the clear-down: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM ownership_assignments WHERE assignment_source = 'import'`)
	if err != nil {
		return cleared, fmt.Errorf("datastore: removing imported assignments: %w", err)
	}
	assignments, err := res.RowsAffected()
	if err != nil {
		return cleared, fmt.Errorf("datastore: counting removed assignments: %w", err)
	}

	// Ordered after the assignment delete so "has nothing attached" is judged
	// on the state the clear-down leaves behind, not the one it started from.
	res, err = tx.ExecContext(ctx, `
		DELETE FROM owners o
		WHERE EXISTS (
			SELECT 1 FROM ownership_audit_log l
			WHERE l.owner_name = o.name
			  AND l.action = 'owner_created'
			  AND l.details ->> 'source' = 'import'
		)
		AND NOT EXISTS (
			SELECT 1 FROM ownership_assignments a WHERE a.owner_name = o.name
		)`)
	if err != nil {
		return cleared, fmt.Errorf("datastore: removing imported owners: %w", err)
	}
	owners, err := res.RowsAffected()
	if err != nil {
		return cleared, fmt.Errorf("datastore: counting removed owners: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return cleared, fmt.Errorf("datastore: committing the clear-down: %w", err)
	}

	cleared.Assignments = int(assignments)
	cleared.Owners = int(owners)
	return cleared, nil
}

// CountImportedOwnership reports what a clear-down would remove, so the
// confirmation can name a number instead of asking somebody to agree to
// "delete imported ownership" and find out afterwards.
//
// Deliberately a separate read rather than a dry-run flag on the delete: a
// confirm-then-act pair where the confirmation and the action share a code path
// is one edit away from the preview doing the deleting.
func (db *DB) CountImportedOwnership(ctx context.Context) (ClearedOwnership, error) {
	var cleared ClearedOwnership

	err := db.pool.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM ownership_assignments WHERE assignment_source = 'import'),
			(SELECT count(*) FROM owners o
			 WHERE EXISTS (
				SELECT 1 FROM ownership_audit_log l
				WHERE l.owner_name = o.name
				  AND l.action = 'owner_created'
				  AND l.details ->> 'source' = 'import'
			 )
			 AND NOT EXISTS (
				SELECT 1 FROM ownership_assignments a
				WHERE a.owner_name = o.name AND a.assignment_source <> 'import'
			 ))`).Scan(&cleared.Assignments, &cleared.Owners)
	if err != nil {
		return cleared, fmt.Errorf("datastore: counting imported ownership: %w", err)
	}
	return cleared, nil
}
