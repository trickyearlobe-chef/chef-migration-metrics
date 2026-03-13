// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AutocorrectPreview represents a row in the autocorrect_previews table.
type AutocorrectPreview struct {
	ID                  string
	CookbookID          string
	CookstyleResultID   string
	TotalOffenses       int
	CorrectableOffenses int
	RemainingOffenses   int
	FilesModified       int
	DiffOutput          string
	GeneratedAt         time.Time
	CreatedAt           time.Time
}

// UpsertAutocorrectPreviewParams contains the fields needed to insert or
// update an autocorrect_previews row. The unique constraint is
// (cookstyle_result_id).
type UpsertAutocorrectPreviewParams struct {
	CookbookID          string
	CookstyleResultID   string
	TotalOffenses       int
	CorrectableOffenses int
	RemainingOffenses   int
	FilesModified       int
	DiffOutput          string
	GeneratedAt         time.Time
}

// ---------------------------------------------------------------------------
// Column list — shared across all queries
// ---------------------------------------------------------------------------

const acpColumns = `id, cookbook_id, cookstyle_result_id,
       total_offenses, correctable_offenses, remaining_offenses,
       files_modified, diff_output, generated_at, created_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetAutocorrectPreview returns the autocorrect preview for the given
// cookstyle result ID. Returns (nil, nil) if no preview exists.
func (db *DB) GetAutocorrectPreview(ctx context.Context, cookstyleResultID string) (*AutocorrectPreview, error) {
	return db.getAutocorrectPreview(ctx, db.q(), cookstyleResultID)
}

func (db *DB) getAutocorrectPreview(ctx context.Context, q queryable, cookstyleResultID string) (*AutocorrectPreview, error) {
	query := `
		SELECT ` + acpColumns + `
		  FROM autocorrect_previews
		 WHERE cookstyle_result_id = $1
	`

	r, err := scanAutocorrectPreview(q.QueryRowContext(ctx, query, cookstyleResultID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting autocorrect preview: %w", err)
	}
	return &r, nil
}

// GetAutocorrectPreviewByID returns a single autocorrect preview by its
// primary key. Returns ErrNotFound if no preview exists.
func (db *DB) GetAutocorrectPreviewByID(ctx context.Context, id string) (*AutocorrectPreview, error) {
	query := `
		SELECT ` + acpColumns + `
		  FROM autocorrect_previews
		 WHERE id = $1
	`

	r, err := scanAutocorrectPreview(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting autocorrect preview by id: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListAutocorrectPreviewsForCookbook returns all autocorrect previews for
// the given cookbook ID, ordered by generated_at descending.
func (db *DB) ListAutocorrectPreviewsForCookbook(ctx context.Context, cookbookID string) ([]AutocorrectPreview, error) {
	query := `
		SELECT ` + acpColumns + `
		  FROM autocorrect_previews
		 WHERE cookbook_id = $1
		 ORDER BY generated_at DESC
	`
	return db.scanAutocorrectPreviews(ctx, query, cookbookID)
}

// ListAutocorrectPreviewsForOrganisation returns all autocorrect previews
// for cookbooks belonging to the given organisation, ordered by cookbook
// name, version, and generated_at descending.
func (db *DB) ListAutocorrectPreviewsForOrganisation(ctx context.Context, organisationID string) ([]AutocorrectPreview, error) {
	query := `
		SELECT ap.id, ap.cookbook_id, ap.cookstyle_result_id,
		       ap.total_offenses, ap.correctable_offenses, ap.remaining_offenses,
		       ap.files_modified, ap.diff_output, ap.generated_at, ap.created_at
		  FROM autocorrect_previews ap
		  JOIN cookbooks c ON c.id = ap.cookbook_id
		 WHERE c.organisation_id = $1
		 ORDER BY c.name, c.version, ap.generated_at DESC
	`
	return db.scanAutocorrectPreviews(ctx, query, organisationID)
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertAutocorrectPreview inserts a new autocorrect preview or updates the
// existing one for the same cookstyle_result_id. Returns the resulting row.
func (db *DB) UpsertAutocorrectPreview(ctx context.Context, p UpsertAutocorrectPreviewParams) (*AutocorrectPreview, error) {
	return db.upsertAutocorrectPreview(ctx, db.q(), p)
}

func (db *DB) upsertAutocorrectPreview(ctx context.Context, q queryable, p UpsertAutocorrectPreviewParams) (*AutocorrectPreview, error) {
	if p.CookbookID == "" {
		return nil, fmt.Errorf("datastore: cookbook_id is required")
	}
	if p.CookstyleResultID == "" {
		return nil, fmt.Errorf("datastore: cookstyle_result_id is required")
	}

	query := `
		INSERT INTO autocorrect_previews (
			cookbook_id, cookstyle_result_id,
			total_offenses, correctable_offenses, remaining_offenses,
			files_modified, diff_output, generated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (cookstyle_result_id)
		DO UPDATE SET
			cookbook_id          = EXCLUDED.cookbook_id,
			total_offenses      = EXCLUDED.total_offenses,
			correctable_offenses = EXCLUDED.correctable_offenses,
			remaining_offenses  = EXCLUDED.remaining_offenses,
			files_modified      = EXCLUDED.files_modified,
			diff_output         = EXCLUDED.diff_output,
			generated_at        = EXCLUDED.generated_at
		RETURNING ` + acpColumns + `
	`

	r, err := scanAutocorrectPreview(q.QueryRowContext(ctx, query,
		p.CookbookID,
		p.CookstyleResultID,
		p.TotalOffenses,
		p.CorrectableOffenses,
		p.RemainingOffenses,
		p.FilesModified,
		nullString(p.DiffOutput),
		p.GeneratedAt,
	))
	if err != nil {
		return nil, fmt.Errorf("datastore: upserting autocorrect preview: %w", err)
	}
	return &r, nil
}

// UpsertAutocorrectPreviewTx is the transactional variant of
// UpsertAutocorrectPreview.
func (db *DB) UpsertAutocorrectPreviewTx(ctx context.Context, tx *sql.Tx, p UpsertAutocorrectPreviewParams) (*AutocorrectPreview, error) {
	return db.upsertAutocorrectPreview(ctx, tx, p)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteAutocorrectPreviewsForCookbook removes all autocorrect previews for
// the given cookbook ID.
func (db *DB) DeleteAutocorrectPreviewsForCookbook(ctx context.Context, cookbookID string) error {
	const query = `DELETE FROM autocorrect_previews WHERE cookbook_id = $1`
	_, err := db.pool.ExecContext(ctx, query, cookbookID)
	if err != nil {
		return fmt.Errorf("datastore: deleting autocorrect previews for cookbook %s: %w", cookbookID, err)
	}
	return nil
}

// DeleteAutocorrectPreviewsForOrganisation removes all autocorrect previews
// for cookbooks belonging to the given organisation.
func (db *DB) DeleteAutocorrectPreviewsForOrganisation(ctx context.Context, organisationID string) error {
	const query = `
		DELETE FROM autocorrect_previews
		 WHERE cookbook_id IN (
			SELECT id FROM cookbooks WHERE organisation_id = $1
		 )
	`
	_, err := db.pool.ExecContext(ctx, query, organisationID)
	if err != nil {
		return fmt.Errorf("datastore: deleting autocorrect previews for organisation %s: %w", organisationID, err)
	}
	return nil
}

// DeleteAutocorrectPreview removes a single autocorrect preview by ID.
// Returns ErrNotFound if no such preview exists.
func (db *DB) DeleteAutocorrectPreview(ctx context.Context, id string) error {
	const query = `DELETE FROM autocorrect_previews WHERE id = $1`
	res, err := db.pool.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("datastore: deleting autocorrect preview %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAutocorrectPreviewForCookstyleResult removes the autocorrect
// preview associated with the given cookstyle result ID.
func (db *DB) DeleteAutocorrectPreviewForCookstyleResult(ctx context.Context, cookstyleResultID string) error {
	const query = `DELETE FROM autocorrect_previews WHERE cookstyle_result_id = $1`
	_, err := db.pool.ExecContext(ctx, query, cookstyleResultID)
	if err != nil {
		return fmt.Errorf("datastore: deleting autocorrect preview for cookstyle result %s: %w", cookstyleResultID, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanAutocorrectPreview(row interface{ Scan(dest ...any) error }) (AutocorrectPreview, error) {
	var r AutocorrectPreview
	var diffOutput sql.NullString

	err := row.Scan(
		&r.ID,
		&r.CookbookID,
		&r.CookstyleResultID,
		&r.TotalOffenses,
		&r.CorrectableOffenses,
		&r.RemainingOffenses,
		&r.FilesModified,
		&diffOutput,
		&r.GeneratedAt,
		&r.CreatedAt,
	)
	if err != nil {
		return AutocorrectPreview{}, err
	}

	r.DiffOutput = stringFromNull(diffOutput)
	return r, nil
}

func (db *DB) scanAutocorrectPreviews(ctx context.Context, query string, args ...any) ([]AutocorrectPreview, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing autocorrect previews: %w", err)
	}
	defer rows.Close()

	var results []AutocorrectPreview
	for rows.Next() {
		var r AutocorrectPreview
		var diffOutput sql.NullString

		if err := rows.Scan(
			&r.ID,
			&r.CookbookID,
			&r.CookstyleResultID,
			&r.TotalOffenses,
			&r.CorrectableOffenses,
			&r.RemainingOffenses,
			&r.FilesModified,
			&diffOutput,
			&r.GeneratedAt,
			&r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning autocorrect preview row: %w", err)
		}

		r.DiffOutput = stringFromNull(diffOutput)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating autocorrect previews: %w", err)
	}
	return results, nil
}

// DeleteAllAutocorrectPreviews removes all autocorrect preview records.
func (db *DB) DeleteAllAutocorrectPreviews(ctx context.Context) error {
	const query = `DELETE FROM autocorrect_previews`
	_, err := db.pool.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("datastore: deleting all autocorrect previews: %w", err)
	}
	return nil
}
