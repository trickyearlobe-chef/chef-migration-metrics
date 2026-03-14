// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ServerCookbookAutocorrectPreview represents a row in the
// server_cookbook_autocorrect_previews table.
type ServerCookbookAutocorrectPreview struct {
	ID                  string
	ServerCookbookID    string
	CookstyleResultID   string
	TotalOffenses       int
	CorrectableOffenses int
	RemainingOffenses   int
	FilesModified       int
	DiffOutput          string
	GeneratedAt         time.Time
	CreatedAt           time.Time
}

// UpsertServerCookbookAutocorrectPreviewParams contains the fields needed to
// insert or update a server_cookbook_autocorrect_previews row. The unique
// constraint is (cookstyle_result_id).
type UpsertServerCookbookAutocorrectPreviewParams struct {
	ServerCookbookID    string
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

const scacpColumns = `id, server_cookbook_id, cookstyle_result_id,
       total_offenses, correctable_offenses, remaining_offenses,
       files_modified, diff_output, generated_at, created_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetServerCookbookAutocorrectPreview returns the autocorrect preview for the
// given cookstyle result ID. Returns (nil, nil) if no preview exists.
func (db *DB) GetServerCookbookAutocorrectPreview(ctx context.Context, cookstyleResultID string) (*ServerCookbookAutocorrectPreview, error) {
	return db.getServerCookbookAutocorrectPreview(ctx, db.q(), cookstyleResultID)
}

func (db *DB) getServerCookbookAutocorrectPreview(ctx context.Context, q queryable, cookstyleResultID string) (*ServerCookbookAutocorrectPreview, error) {
	query := `
		SELECT ` + scacpColumns + `
		  FROM server_cookbook_autocorrect_previews
		 WHERE cookstyle_result_id = $1
	`

	r, err := scanServerCookbookAutocorrectPreview(q.QueryRowContext(ctx, query, cookstyleResultID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting server cookbook autocorrect preview: %w", err)
	}
	return &r, nil
}

// GetServerCookbookAutocorrectPreviewByID returns a single autocorrect
// preview by its primary key. Returns ErrNotFound if no preview exists.
func (db *DB) GetServerCookbookAutocorrectPreviewByID(ctx context.Context, id string) (*ServerCookbookAutocorrectPreview, error) {
	query := `
		SELECT ` + scacpColumns + `
		  FROM server_cookbook_autocorrect_previews
		 WHERE id = $1
	`

	r, err := scanServerCookbookAutocorrectPreview(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting server cookbook autocorrect preview by id: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListServerCookbookAutocorrectPreviewsByCookbook returns all autocorrect
// previews for the given server cookbook ID, ordered by generated_at
// descending.
func (db *DB) ListServerCookbookAutocorrectPreviewsByCookbook(ctx context.Context, serverCookbookID string) ([]ServerCookbookAutocorrectPreview, error) {
	query := `
		SELECT ` + scacpColumns + `
		  FROM server_cookbook_autocorrect_previews
		 WHERE server_cookbook_id = $1
		 ORDER BY generated_at DESC
	`
	return db.scanServerCookbookAutocorrectPreviews(ctx, query, serverCookbookID)
}

// ListServerCookbookAutocorrectPreviewsByOrganisation returns all autocorrect
// previews for server cookbooks belonging to the given organisation, ordered
// by cookbook name, version, and generated_at descending.
func (db *DB) ListServerCookbookAutocorrectPreviewsByOrganisation(ctx context.Context, organisationID string) ([]ServerCookbookAutocorrectPreview, error) {
	query := `
		SELECT ap.id, ap.server_cookbook_id, ap.cookstyle_result_id,
		       ap.total_offenses, ap.correctable_offenses, ap.remaining_offenses,
		       ap.files_modified, ap.diff_output, ap.generated_at, ap.created_at
		  FROM server_cookbook_autocorrect_previews ap
		  JOIN server_cookbooks sc ON sc.id = ap.server_cookbook_id
		 WHERE sc.organisation_id = $1
		 ORDER BY sc.name, sc.version, ap.generated_at DESC
	`
	return db.scanServerCookbookAutocorrectPreviews(ctx, query, organisationID)
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertServerCookbookAutocorrectPreview inserts a new autocorrect preview or
// updates the existing one for the same cookstyle_result_id. Returns the
// resulting row.
func (db *DB) UpsertServerCookbookAutocorrectPreview(ctx context.Context, p UpsertServerCookbookAutocorrectPreviewParams) (*ServerCookbookAutocorrectPreview, error) {
	return db.upsertServerCookbookAutocorrectPreview(ctx, db.q(), p)
}

func (db *DB) upsertServerCookbookAutocorrectPreview(ctx context.Context, q queryable, p UpsertServerCookbookAutocorrectPreviewParams) (*ServerCookbookAutocorrectPreview, error) {
	if p.ServerCookbookID == "" {
		return nil, fmt.Errorf("datastore: server_cookbook_id is required")
	}
	if p.CookstyleResultID == "" {
		return nil, fmt.Errorf("datastore: cookstyle_result_id is required")
	}

	query := `
		INSERT INTO server_cookbook_autocorrect_previews (
			server_cookbook_id, cookstyle_result_id,
			total_offenses, correctable_offenses, remaining_offenses,
			files_modified, diff_output, generated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (cookstyle_result_id)
		DO UPDATE SET
			server_cookbook_id    = EXCLUDED.server_cookbook_id,
			total_offenses       = EXCLUDED.total_offenses,
			correctable_offenses = EXCLUDED.correctable_offenses,
			remaining_offenses   = EXCLUDED.remaining_offenses,
			files_modified       = EXCLUDED.files_modified,
			diff_output          = EXCLUDED.diff_output,
			generated_at         = EXCLUDED.generated_at
		RETURNING ` + scacpColumns + `
	`

	r, err := scanServerCookbookAutocorrectPreview(q.QueryRowContext(ctx, query,
		p.ServerCookbookID,
		p.CookstyleResultID,
		p.TotalOffenses,
		p.CorrectableOffenses,
		p.RemainingOffenses,
		p.FilesModified,
		nullString(p.DiffOutput),
		p.GeneratedAt,
	))
	if err != nil {
		return nil, fmt.Errorf("datastore: upserting server cookbook autocorrect preview: %w", err)
	}
	return &r, nil
}

// UpsertServerCookbookAutocorrectPreviewTx is the transactional variant of
// UpsertServerCookbookAutocorrectPreview.
func (db *DB) UpsertServerCookbookAutocorrectPreviewTx(ctx context.Context, tx *sql.Tx, p UpsertServerCookbookAutocorrectPreviewParams) (*ServerCookbookAutocorrectPreview, error) {
	return db.upsertServerCookbookAutocorrectPreview(ctx, tx, p)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteServerCookbookAutocorrectPreviewsByCookbook removes all autocorrect
// previews for the given server cookbook ID.
func (db *DB) DeleteServerCookbookAutocorrectPreviewsByCookbook(ctx context.Context, serverCookbookID string) error {
	const query = `DELETE FROM server_cookbook_autocorrect_previews WHERE server_cookbook_id = $1`
	_, err := db.pool.ExecContext(ctx, query, serverCookbookID)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook autocorrect previews for cookbook %s: %w", serverCookbookID, err)
	}
	return nil
}

// DeleteServerCookbookAutocorrectPreviewsByOrganisation removes all
// autocorrect previews for server cookbooks belonging to the given
// organisation.
func (db *DB) DeleteServerCookbookAutocorrectPreviewsByOrganisation(ctx context.Context, organisationID string) error {
	const query = `
		DELETE FROM server_cookbook_autocorrect_previews
		 WHERE server_cookbook_id IN (
			SELECT id FROM server_cookbooks WHERE organisation_id = $1
		 )
	`
	_, err := db.pool.ExecContext(ctx, query, organisationID)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook autocorrect previews for organisation %s: %w", organisationID, err)
	}
	return nil
}

// DeleteAllServerCookbookAutocorrectPreviews removes all server cookbook
// autocorrect preview records.
func (db *DB) DeleteAllServerCookbookAutocorrectPreviews(ctx context.Context) error {
	const query = `DELETE FROM server_cookbook_autocorrect_previews`
	_, err := db.pool.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("datastore: deleting all server cookbook autocorrect previews: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanServerCookbookAutocorrectPreview(row interface{ Scan(dest ...any) error }) (ServerCookbookAutocorrectPreview, error) {
	var r ServerCookbookAutocorrectPreview
	var diffOutput sql.NullString

	err := row.Scan(
		&r.ID,
		&r.ServerCookbookID,
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
		return ServerCookbookAutocorrectPreview{}, err
	}

	r.DiffOutput = stringFromNull(diffOutput)
	return r, nil
}

func (db *DB) scanServerCookbookAutocorrectPreviews(ctx context.Context, query string, args ...any) ([]ServerCookbookAutocorrectPreview, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing server cookbook autocorrect previews: %w", err)
	}
	defer rows.Close()

	var results []ServerCookbookAutocorrectPreview
	for rows.Next() {
		var r ServerCookbookAutocorrectPreview
		var diffOutput sql.NullString

		if err := rows.Scan(
			&r.ID,
			&r.ServerCookbookID,
			&r.CookstyleResultID,
			&r.TotalOffenses,
			&r.CorrectableOffenses,
			&r.RemainingOffenses,
			&r.FilesModified,
			&diffOutput,
			&r.GeneratedAt,
			&r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning server cookbook autocorrect preview row: %w", err)
		}

		r.DiffOutput = stringFromNull(diffOutput)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating server cookbook autocorrect previews: %w", err)
	}
	return results, nil
}
