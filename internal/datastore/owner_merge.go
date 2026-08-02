// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// MergeOwnersResult reports what folding one owner into another moved.
type MergeOwnersResult struct {
	FromOwner string `json:"from_owner"`
	IntoOwner string `json:"into_owner"`

	// Reassigned and Skipped count assignments moved and assignments the
	// target already held (which are removed from the source rather than
	// duplicated onto the target).
	Reassigned int `json:"reassigned"`
	Skipped    int `json:"skipped"`

	// AliasesMoved counts identities the source was known by that now
	// resolve to the target. AliasesDropped counts any the target already
	// held under the same type and value.
	AliasesMoved   int `json:"aliases_moved"`
	AliasesDropped int `json:"aliases_dropped"`

	// SourceNameAliased records whether the source owner's own name was
	// added to the target as a custom alias.
	SourceNameAliased bool `json:"source_name_aliased"`
}

// MergeOwners folds one owner into another: the work moves, every identity the
// source was known by moves with it, and the source owner is removed.
//
// Moving the aliases is what makes the correction durable. Reassignment alone
// leaves the source's aliases pointing at an emptied owner, so the raw string
// from the original export still resolves there and the next ingest puts the
// work straight back.
//
// The source owner's own name is seeded onto the target as a custom alias for
// the same reason: once the owner row is gone, a source naming it would
// otherwise create the person again.
func (db *DB) MergeOwners(ctx context.Context, fromOwnerName, intoOwnerName string) (MergeOwnersResult, error) {
	result := MergeOwnersResult{FromOwner: fromOwnerName, IntoOwner: intoOwnerName}

	if fromOwnerName == "" || intoOwnerName == "" {
		return result, errors.New("datastore: both owner names are required to merge")
	}
	if fromOwnerName == intoOwnerName {
		return result, errors.New("datastore: an owner cannot be merged into itself")
	}

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		// Lock both rows before anything moves, so a concurrent merge or
		// delete cannot empty one side underneath this one.
		for _, name := range []string{fromOwnerName, intoOwnerName} {
			var found string
			err := tx.QueryRowContext(ctx, `SELECT name FROM owners WHERE name = $1 FOR UPDATE`, name).Scan(&found)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return fmt.Errorf("locking owner %q: %w", name, err)
			}
		}

		reassigned, skipped, err := reassignOwnershipTx(ctx, tx, fromOwnerName, intoOwnerName, "", "")
		if err != nil {
			return err
		}
		result.Reassigned = reassigned
		result.Skipped = skipped

		// Move every alias the target does not already hold. The uniqueness
		// constraint on (alias_type, alias_value) is global, so a collision
		// means the target already answers to that identity.
		moved, err := tx.ExecContext(ctx, `
			UPDATE owner_aliases a
			SET owner_name = $2
			WHERE a.owner_name = $1
			  AND NOT EXISTS (
			      SELECT 1 FROM owner_aliases b
			      WHERE b.owner_name = $2
			        AND b.alias_type = a.alias_type
			        AND b.alias_value = a.alias_value
			  )
		`, fromOwnerName, intoOwnerName)
		if err != nil {
			return fmt.Errorf("moving aliases from %q: %w", fromOwnerName, err)
		}
		movedCount, err := moved.RowsAffected()
		if err != nil {
			return fmt.Errorf("counting moved aliases: %w", err)
		}
		result.AliasesMoved = int(movedCount)

		dropped, err := tx.ExecContext(ctx, `DELETE FROM owner_aliases WHERE owner_name = $1`, fromOwnerName)
		if err != nil {
			return fmt.Errorf("removing the source owner's remaining aliases: %w", err)
		}
		droppedCount, err := dropped.RowsAffected()
		if err != nil {
			return fmt.Errorf("counting dropped aliases: %w", err)
		}
		result.AliasesDropped = int(droppedCount)

		// Keep the source owner's name reachable. ON CONFLICT covers the case
		// where somebody has already recorded it against the target.
		seeded, err := tx.ExecContext(ctx, `
			INSERT INTO owner_aliases (owner_name, alias_type, alias_value, source)
			VALUES ($1, 'custom', $2, 'merge')
			ON CONFLICT (alias_type, alias_value) DO NOTHING
		`, intoOwnerName, fromOwnerName)
		if err != nil {
			return fmt.Errorf("seeding the source owner's name as an alias: %w", err)
		}
		seededCount, err := seeded.RowsAffected()
		if err != nil {
			return fmt.Errorf("counting the seeded alias: %w", err)
		}
		result.SourceNameAliased = seededCount > 0

		if _, err := tx.ExecContext(ctx, `DELETE FROM owners WHERE name = $1`, fromOwnerName); err != nil {
			return fmt.Errorf("deleting the merged owner %q: %w", fromOwnerName, err)
		}
		return nil
	})
	if err != nil {
		return MergeOwnersResult{FromOwner: fromOwnerName, IntoOwner: intoOwnerName}, err
	}
	return result, nil
}
