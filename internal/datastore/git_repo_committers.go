// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GitRepoCommitter represents a row in the git_repo_committers table.
type GitRepoCommitter struct {
	ID            string    `json:"id"`
	GitRepoURL    string    `json:"git_repo_url"`
	AuthorName    string    `json:"author_name"`
	AuthorEmail   string    `json:"author_email"`
	CommitCount   int       `json:"commit_count"`
	FirstCommitAt time.Time `json:"first_commit_at"`
	LastCommitAt  time.Time `json:"last_commit_at"`
	CollectedAt   time.Time `json:"collected_at"`
}

// CommitterListFilter holds query parameters for listing committers.
type CommitterListFilter struct {
	GitRepoURL string    // required — filter by git repo URL
	Since      time.Time // only include committers with last_commit_at after this date
	Sort       string    // last_commit_at, commit_count, author_name
	Order      string    // asc, desc
	Limit      int
	Offset     int
}

// ListCommittersByRepo returns committers for the given git repo URL, with
// sorting, pagination, and an optional since filter on last_commit_at.
// Returns the matching rows and the total count for pagination.
func (db *DB) ListCommittersByRepo(ctx context.Context, f CommitterListFilter) ([]GitRepoCommitter, int, error) {
	return db.listCommittersByRepo(ctx, db.q(), f)
}

func (db *DB) listCommittersByRepo(ctx context.Context, q queryable, f CommitterListFilter) ([]GitRepoCommitter, int, error) {
	where := "WHERE git_repo_url = $1"
	args := []any{f.GitRepoURL}
	argN := 2

	if !f.Since.IsZero() {
		where += fmt.Sprintf(" AND last_commit_at >= $%d", argN)
		args = append(args, f.Since)
		argN++
	}

	// Count total.
	countQuery := "SELECT COUNT(*) FROM git_repo_committers " + where
	var total int
	if err := q.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("datastore: counting committers: %w", err)
	}

	// Determine sort column.
	sortCol := "last_commit_at"
	switch f.Sort {
	case "commit_count":
		sortCol = "commit_count"
	case "author_name":
		sortCol = "author_name"
	case "last_commit_at":
		sortCol = "last_commit_at"
	}

	// Determine sort order.
	order := "DESC"
	switch f.Order {
	case "asc":
		order = "ASC"
	case "desc":
		order = "DESC"
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
		SELECT id, git_repo_url, author_name, author_email,
		       commit_count, first_commit_at, last_commit_at, collected_at
		FROM git_repo_committers
		%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, where, sortCol, order, argN, argN+1)
	args = append(args, limit, offset)

	rows, err := q.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: listing committers: %w", err)
	}
	defer rows.Close()

	committers, err := scanCommitters(rows)
	if err != nil {
		return nil, 0, err
	}
	return committers, total, nil
}

// ReplaceCommittersForRepo deletes all existing committers for the given repo
// URL and inserts the provided committers, all within a single transaction.
// This is used by the collection pipeline to refresh committer data.
func (db *DB) ReplaceCommittersForRepo(ctx context.Context, gitRepoURL string, committers []GitRepoCommitter) error {
	return db.replaceCommittersForRepo(ctx, gitRepoURL, committers)
}

func (db *DB) replaceCommittersForRepo(ctx context.Context, gitRepoURL string, committers []GitRepoCommitter) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		// Delete all existing committers for this repo.
		_, err := tx.ExecContext(ctx,
			`DELETE FROM git_repo_committers WHERE git_repo_url = $1`,
			gitRepoURL,
		)
		if err != nil {
			return fmt.Errorf("datastore: deleting committers for repo: %w", err)
		}

		// Insert the new committers.
		const insertQuery = `
			INSERT INTO git_repo_committers
				(git_repo_url, author_name, author_email, commit_count,
				 first_commit_at, last_commit_at, collected_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`

		for _, c := range committers {
			collectedAt := c.CollectedAt
			if collectedAt.IsZero() {
				collectedAt = time.Now()
			}
			_, err := tx.ExecContext(ctx, insertQuery,
				gitRepoURL,
				c.AuthorName,
				c.AuthorEmail,
				c.CommitCount,
				c.FirstCommitAt,
				c.LastCommitAt,
				collectedAt,
			)
			if err != nil {
				return fmt.Errorf("datastore: inserting committer %s: %w", c.AuthorEmail, err)
			}
		}

		return nil
	})
}

// GetGitRepoURLForCookbook looks up the git_repo_url from the cookbooks table
// for a cookbook with source = 'git'. Returns ErrNotFound if no git-sourced
// cookbook exists with that name.
func (db *DB) GetGitRepoURLForCookbook(ctx context.Context, cookbookName string) (string, error) {
	return db.getGitRepoURLForCookbook(ctx, db.q(), cookbookName)
}

func (db *DB) getGitRepoURLForCookbook(ctx context.Context, q queryable, cookbookName string) (string, error) {
	const query = `
		SELECT git_repo_url
		FROM cookbooks
		WHERE name = $1 AND source = 'git'
		LIMIT 1
	`

	var repoURL sql.NullString
	err := q.QueryRowContext(ctx, query, cookbookName).Scan(&repoURL)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("datastore: git cookbook %q: %w", cookbookName, ErrNotFound)
		}
		return "", fmt.Errorf("datastore: looking up git repo URL for cookbook %q: %w", cookbookName, err)
	}

	if !repoURL.Valid || repoURL.String == "" {
		return "", fmt.Errorf("datastore: git cookbook %q has no repo URL: %w", cookbookName, ErrNotFound)
	}

	return repoURL.String, nil
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

func scanCommitter(row *sql.Row) (GitRepoCommitter, error) {
	var c GitRepoCommitter
	err := row.Scan(
		&c.ID,
		&c.GitRepoURL,
		&c.AuthorName,
		&c.AuthorEmail,
		&c.CommitCount,
		&c.FirstCommitAt,
		&c.LastCommitAt,
		&c.CollectedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return GitRepoCommitter{}, fmt.Errorf("datastore: committer: %w", ErrNotFound)
		}
		return GitRepoCommitter{}, fmt.Errorf("datastore: scanning committer: %w", err)
	}
	return c, nil
}

func scanCommitters(rows *sql.Rows) ([]GitRepoCommitter, error) {
	var committers []GitRepoCommitter
	for rows.Next() {
		var c GitRepoCommitter
		if err := rows.Scan(
			&c.ID,
			&c.GitRepoURL,
			&c.AuthorName,
			&c.AuthorEmail,
			&c.CommitCount,
			&c.FirstCommitAt,
			&c.LastCommitAt,
			&c.CollectedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning committer row: %w", err)
		}
		committers = append(committers, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating committer rows: %w", err)
	}
	return committers, nil
}
