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
	OrganisationName    string
	CookbookName        string
	CookbookVersion     string
	TargetChefVersion   string
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
// constraint is (organisation_name, cookbook_name, cookbook_version, target_chef_version).
type UpsertServerCookbookAutocorrectPreviewParams struct {
	OrganisationName    string
	CookbookName        string
	CookbookVersion     string
	TargetChefVersion   string
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

const scacpColumns = `organisation_name, cookbook_name, cookbook_version, target_chef_version,
       total_offenses, correctable_offenses, remaining_offenses,
       files_modified, diff_output, generated_at, created_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetServerCookbookAutocorrectPreview returns the autocorrect preview for the
// given organisation, cookbook, and target Chef version. Returns (nil, nil) if
// no preview exists.
func (db *DB) GetServerCookbookAutocorrectPreview(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*ServerCookbookAutocorrectPreview, error) {
	return db.getServerCookbookAutocorrectPreview(ctx, db.q(), orgName, cookbookName, cookbookVersion, targetChefVersion)
}

func (db *DB) getServerCookbookAutocorrectPreview(ctx context.Context, q queryable, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*ServerCookbookAutocorrectPreview, error) {
	query := `
		SELECT ` + scacpColumns + `
		  FROM server_cookbook_autocorrect_previews
		 WHERE organisation_name = $1
		   AND cookbook_name = $2
		   AND cookbook_version = $3
		   AND target_chef_version = $4
	`

	r, err := scanServerCookbookAutocorrectPreview(q.QueryRowContext(ctx, query, orgName, cookbookName, cookbookVersion, targetChefVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting server cookbook autocorrect preview: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListServerCookbookAutocorrectPreviewsByCookbook returns all autocorrect
// previews for the given organisation, cookbook name, and cookbook version,
// ordered by generated_at descending.
func (db *DB) ListServerCookbookAutocorrectPreviewsByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]ServerCookbookAutocorrectPreview, error) {
	query := `
		SELECT ` + scacpColumns + `
		  FROM server_cookbook_autocorrect_previews
		 WHERE organisation_name = $1
		   AND cookbook_name = $2
		   AND cookbook_version = $3
		 ORDER BY generated_at DESC
	`
	return db.scanServerCookbookAutocorrectPreviews(ctx, query, orgName, cookbookName, cookbookVersion)
}

// ListServerCookbookAutocorrectPreviewsByOrganisation returns all autocorrect
// previews for server cookbooks belonging to the given organisation, ordered
// by cookbook name, version, and generated_at descending.
func (db *DB) ListServerCookbookAutocorrectPreviewsByOrganisation(ctx context.Context, orgName string) ([]ServerCookbookAutocorrectPreview, error) {
	query := `
		SELECT ` + scacpColumns + `
		  FROM server_cookbook_autocorrect_previews
		 WHERE organisation_name = $1
		 ORDER BY cookbook_name, cookbook_version, generated_at DESC
	`
	return db.scanServerCookbookAutocorrectPreviews(ctx, query, orgName)
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertServerCookbookAutocorrectPreview inserts a new autocorrect preview or
// updates the existing one for the same (organisation_name, cookbook_name,
// cookbook_version, target_chef_version). Returns the resulting row.
func (db *DB) UpsertServerCookbookAutocorrectPreview(ctx context.Context, p UpsertServerCookbookAutocorrectPreviewParams) (*ServerCookbookAutocorrectPreview, error) {
	return db.upsertServerCookbookAutocorrectPreview(ctx, db.q(), p)
}

func (db *DB) upsertServerCookbookAutocorrectPreview(ctx context.Context, q queryable, p UpsertServerCookbookAutocorrectPreviewParams) (*ServerCookbookAutocorrectPreview, error) {
	if p.OrganisationName == "" {
		return nil, fmt.Errorf("datastore: organisation_name is required")
	}
	if p.CookbookName == "" {
		return nil, fmt.Errorf("datastore: cookbook_name is required")
	}
	if p.TargetChefVersion == "" {
		return nil, fmt.Errorf("datastore: target_chef_version is required")
	}

	query := `
		INSERT INTO server_cookbook_autocorrect_previews (
			organisation_name, cookbook_name, cookbook_version, target_chef_version,
			total_offenses, correctable_offenses, remaining_offenses,
			files_modified, diff_output, generated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (organisation_name, cookbook_name, cookbook_version, target_chef_version)
		DO UPDATE SET
			total_offenses       = EXCLUDED.total_offenses,
			correctable_offenses = EXCLUDED.correctable_offenses,
			remaining_offenses   = EXCLUDED.remaining_offenses,
			files_modified       = EXCLUDED.files_modified,
			diff_output          = EXCLUDED.diff_output,
			generated_at         = EXCLUDED.generated_at
		RETURNING ` + scacpColumns + `
	`

	r, err := scanServerCookbookAutocorrectPreview(q.QueryRowContext(ctx, query,
		p.OrganisationName,
		p.CookbookName,
		p.CookbookVersion,
		p.TargetChefVersion,
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
// previews for the given organisation, cookbook name, and cookbook version.
func (db *DB) DeleteServerCookbookAutocorrectPreviewsByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) error {
	const query = `DELETE FROM server_cookbook_autocorrect_previews WHERE organisation_name = $1 AND cookbook_name = $2 AND cookbook_version = $3`
	_, err := db.pool.ExecContext(ctx, query, orgName, cookbookName, cookbookVersion)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook autocorrect previews for cookbook %s/%s@%s: %w", orgName, cookbookName, cookbookVersion, err)
	}
	return nil
}

// DeleteServerCookbookAutocorrectPreviewsByOrganisation removes all
// autocorrect previews for server cookbooks belonging to the given
// organisation.
func (db *DB) DeleteServerCookbookAutocorrectPreviewsByOrganisation(ctx context.Context, orgName string) error {
	const query = `DELETE FROM server_cookbook_autocorrect_previews WHERE organisation_name = $1`
	_, err := db.pool.ExecContext(ctx, query, orgName)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook autocorrect previews for organisation %s: %w", orgName, err)
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
		&r.OrganisationName,
		&r.CookbookName,
		&r.CookbookVersion,
		&r.TargetChefVersion,
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
			&r.OrganisationName,
			&r.CookbookName,
			&r.CookbookVersion,
			&r.TargetChefVersion,
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
