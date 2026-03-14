// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GitRepoAutocorrectPreview represents a row in the
// git_repo_autocorrect_previews table.
type GitRepoAutocorrectPreview struct {
	ID                  string
	GitRepoID           string
	CookstyleResultID   string
	TotalOffenses       int
	CorrectableOffenses int
	RemainingOffenses   int
	FilesModified       int
	DiffOutput          string
	GeneratedAt         time.Time
	CreatedAt           time.Time
}

// UpsertGitRepoAutocorrectPreviewParams contains the fields needed to insert
// or update a git_repo_autocorrect_previews row. The unique constraint is
// (cookstyle_result_id).
type UpsertGitRepoAutocorrectPreviewParams struct {
	GitRepoID           string
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

const grAcpColumns = `id, git_repo_id, cookstyle_result_id,
       total_offenses, correctable_offenses, remaining_offenses,
       files_modified, diff_output, generated_at, created_at`

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetGitRepoAutocorrectPreview returns the autocorrect preview for the given
// cookstyle result ID. Returns (nil, nil) if no preview exists.
func (db *DB) GetGitRepoAutocorrectPreview(ctx context.Context, cookstyleResultID string) (*GitRepoAutocorrectPreview, error) {
	return db.getGitRepoAutocorrectPreview(ctx, db.q(), cookstyleResultID)
}

func (db *DB) getGitRepoAutocorrectPreview(ctx context.Context, q queryable, cookstyleResultID string) (*GitRepoAutocorrectPreview, error) {
	query := `
		SELECT ` + grAcpColumns + `
		  FROM git_repo_autocorrect_previews
		 WHERE cookstyle_result_id = $1
	`

	r, err := scanGitRepoAutocorrectPreview(q.QueryRowContext(ctx, query, cookstyleResultID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting git repo autocorrect preview: %w", err)
	}
	return &r, nil
}

// GetGitRepoAutocorrectPreviewByID returns a single autocorrect preview by
// its primary key. Returns ErrNotFound if no preview exists.
func (db *DB) GetGitRepoAutocorrectPreviewByID(ctx context.Context, id string) (*GitRepoAutocorrectPreview, error) {
	query := `
		SELECT ` + grAcpColumns + `
		  FROM git_repo_autocorrect_previews
		 WHERE id = $1
	`

	r, err := scanGitRepoAutocorrectPreview(db.q().QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting git repo autocorrect preview by id: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListGitRepoAutocorrectPreviewsByRepo returns all autocorrect previews for
// the given git repo ID, ordered by generated_at descending.
func (db *DB) ListGitRepoAutocorrectPreviewsByRepo(ctx context.Context, gitRepoID string) ([]GitRepoAutocorrectPreview, error) {
	query := `
		SELECT ` + grAcpColumns + `
		  FROM git_repo_autocorrect_previews
		 WHERE git_repo_id = $1
		 ORDER BY generated_at DESC
	`
	return db.scanGitRepoAutocorrectPreviews(ctx, query, gitRepoID)
}

// ListGitRepoAutocorrectPreviewsByName returns all autocorrect previews
// for git repos with the given name, ordered by generated_at descending.
func (db *DB) ListGitRepoAutocorrectPreviewsByName(ctx context.Context, name string) ([]GitRepoAutocorrectPreview, error) {
	query := `
		SELECT ap.id, ap.git_repo_id, ap.cookstyle_result_id,
		       ap.total_offenses, ap.correctable_offenses, ap.remaining_offenses,
		       ap.files_modified, ap.diff_output, ap.generated_at, ap.created_at
		  FROM git_repo_autocorrect_previews ap
		  JOIN git_repos gr ON gr.id = ap.git_repo_id
		 WHERE gr.name = $1
		 ORDER BY ap.generated_at DESC
	`
	return db.scanGitRepoAutocorrectPreviews(ctx, query, name)
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertGitRepoAutocorrectPreview inserts a new autocorrect preview or
// updates the existing one for the same cookstyle_result_id. Returns the
// resulting row.
func (db *DB) UpsertGitRepoAutocorrectPreview(ctx context.Context, p UpsertGitRepoAutocorrectPreviewParams) (*GitRepoAutocorrectPreview, error) {
	return db.upsertGitRepoAutocorrectPreview(ctx, db.q(), p)
}

func (db *DB) upsertGitRepoAutocorrectPreview(ctx context.Context, q queryable, p UpsertGitRepoAutocorrectPreviewParams) (*GitRepoAutocorrectPreview, error) {
	if p.GitRepoID == "" {
		return nil, fmt.Errorf("datastore: git_repo_id is required")
	}
	if p.CookstyleResultID == "" {
		return nil, fmt.Errorf("datastore: cookstyle_result_id is required")
	}

	query := `
		INSERT INTO git_repo_autocorrect_previews (
			git_repo_id, cookstyle_result_id,
			total_offenses, correctable_offenses, remaining_offenses,
			files_modified, diff_output, generated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (cookstyle_result_id)
		DO UPDATE SET
			git_repo_id         = EXCLUDED.git_repo_id,
			total_offenses      = EXCLUDED.total_offenses,
			correctable_offenses = EXCLUDED.correctable_offenses,
			remaining_offenses  = EXCLUDED.remaining_offenses,
			files_modified      = EXCLUDED.files_modified,
			diff_output         = EXCLUDED.diff_output,
			generated_at        = EXCLUDED.generated_at
		RETURNING ` + grAcpColumns + `
	`

	r, err := scanGitRepoAutocorrectPreview(q.QueryRowContext(ctx, query,
		p.GitRepoID,
		p.CookstyleResultID,
		p.TotalOffenses,
		p.CorrectableOffenses,
		p.RemainingOffenses,
		p.FilesModified,
		nullString(p.DiffOutput),
		p.GeneratedAt,
	))
	if err != nil {
		return nil, fmt.Errorf("datastore: upserting git repo autocorrect preview: %w", err)
	}
	return &r, nil
}

// UpsertGitRepoAutocorrectPreviewTx is the transactional variant of
// UpsertGitRepoAutocorrectPreview.
func (db *DB) UpsertGitRepoAutocorrectPreviewTx(ctx context.Context, tx *sql.Tx, p UpsertGitRepoAutocorrectPreviewParams) (*GitRepoAutocorrectPreview, error) {
	return db.upsertGitRepoAutocorrectPreview(ctx, tx, p)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteGitRepoAutocorrectPreviewsByRepo removes all autocorrect previews
// for the given git repo ID.
func (db *DB) DeleteGitRepoAutocorrectPreviewsByRepo(ctx context.Context, gitRepoID string) error {
	const query = `DELETE FROM git_repo_autocorrect_previews WHERE git_repo_id = $1`
	_, err := db.pool.ExecContext(ctx, query, gitRepoID)
	if err != nil {
		return fmt.Errorf("datastore: deleting git repo autocorrect previews for repo %s: %w", gitRepoID, err)
	}
	return nil
}

// DeleteAllGitRepoAutocorrectPreviews removes all git repo autocorrect
// preview records.
func (db *DB) DeleteAllGitRepoAutocorrectPreviews(ctx context.Context) error {
	const query = `DELETE FROM git_repo_autocorrect_previews`
	_, err := db.pool.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("datastore: deleting all git repo autocorrect previews: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanGitRepoAutocorrectPreview(row interface{ Scan(dest ...any) error }) (GitRepoAutocorrectPreview, error) {
	var r GitRepoAutocorrectPreview
	var diffOutput sql.NullString

	err := row.Scan(
		&r.ID,
		&r.GitRepoID,
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
		return GitRepoAutocorrectPreview{}, err
	}

	r.DiffOutput = stringFromNull(diffOutput)
	return r, nil
}

func (db *DB) scanGitRepoAutocorrectPreviews(ctx context.Context, query string, args ...any) ([]GitRepoAutocorrectPreview, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing git repo autocorrect previews: %w", err)
	}
	defer rows.Close()

	var results []GitRepoAutocorrectPreview
	for rows.Next() {
		var r GitRepoAutocorrectPreview
		var diffOutput sql.NullString

		if err := rows.Scan(
			&r.ID,
			&r.GitRepoID,
			&r.CookstyleResultID,
			&r.TotalOffenses,
			&r.CorrectableOffenses,
			&r.RemainingOffenses,
			&r.FilesModified,
			&diffOutput,
			&r.GeneratedAt,
			&r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning git repo autocorrect preview row: %w", err)
		}

		r.DiffOutput = stringFromNull(diffOutput)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating git repo autocorrect previews: %w", err)
	}
	return results, nil
}
