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

// GitRepo represents a row in the git_repos table. Each row is a unique
// cookbook name + git URL combination. Git repos are not org-scoped — they
// are matched by name across organisations.
type GitRepo struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	GitRepoURL    string    `json:"git_repo_url"`
	HeadCommitSHA string    `json:"head_commit_sha,omitempty"`
	DefaultBranch string    `json:"default_branch,omitempty"`
	HasTestSuite  bool      `json:"has_test_suite"`
	LastFetchedAt time.Time `json:"last_fetched_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// MarshalJSON implements json.Marshaler for GitRepo.
func (gr GitRepo) MarshalJSON() ([]byte, error) {
	type Alias GitRepo
	return json.Marshal((Alias)(gr))
}

// gitRepoColumns is the column list used by all SELECT queries against
// git_repos, kept in one place for consistency.
const gitRepoColumns = `
	id, name, git_repo_url, head_commit_sha, default_branch,
	has_test_suite, last_fetched_at, created_at, updated_at
`

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertGitRepoParams holds the fields required to upsert a git repo.
// The upsert key is (name, git_repo_url).
type UpsertGitRepoParams struct {
	Name          string
	GitRepoURL    string
	HeadCommitSHA string
	DefaultBranch string
	HasTestSuite  bool
	LastFetchedAt time.Time
}

// UpsertGitRepo inserts or updates a git repo row.
func (db *DB) UpsertGitRepo(ctx context.Context, p UpsertGitRepoParams) (GitRepo, error) {
	return db.upsertGitRepo(ctx, db.q(), p)
}

func (db *DB) upsertGitRepo(ctx context.Context, q queryable, p UpsertGitRepoParams) (GitRepo, error) {
	if p.Name == "" {
		return GitRepo{}, fmt.Errorf("datastore: git repo name is required")
	}
	if p.GitRepoURL == "" {
		return GitRepo{}, fmt.Errorf("datastore: git repo URL is required")
	}
	if p.LastFetchedAt.IsZero() {
		p.LastFetchedAt = time.Now().UTC()
	}

	const query = `
		INSERT INTO git_repos (
			name, git_repo_url, head_commit_sha, default_branch,
			has_test_suite, last_fetched_at
		) VALUES (
			$1, $2, $3, $4, $5, $6
		)
		ON CONFLICT (name, git_repo_url)
		DO UPDATE SET
			head_commit_sha = EXCLUDED.head_commit_sha,
			default_branch  = EXCLUDED.default_branch,
			has_test_suite  = EXCLUDED.has_test_suite,
			last_fetched_at = EXCLUDED.last_fetched_at,
			updated_at      = now()
		RETURNING ` + gitRepoColumns

	return scanGitRepo(q.QueryRowContext(ctx, query,
		p.Name,
		p.GitRepoURL,
		nullString(p.HeadCommitSHA),
		nullString(p.DefaultBranch),
		p.HasTestSuite,
		p.LastFetchedAt,
	))
}

// ---------------------------------------------------------------------------
// Query methods
// ---------------------------------------------------------------------------

// GetGitRepo returns a git repo by UUID. Returns ErrNotFound if no such
// git repo exists.
func (db *DB) GetGitRepo(ctx context.Context, id string) (GitRepo, error) {
	return db.getGitRepo(ctx, db.q(), id)
}

func (db *DB) getGitRepo(ctx context.Context, q queryable, id string) (GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + ` FROM git_repos WHERE id = $1`
	return scanGitRepo(q.QueryRowContext(ctx, query, id))
}

// GetGitRepoByKey returns a git repo by its natural key (name, git_repo_url).
// Returns ErrNotFound if no match exists.
func (db *DB) GetGitRepoByKey(ctx context.Context, name, gitRepoURL string) (GitRepo, error) {
	return db.getGitRepoByKey(ctx, db.q(), name, gitRepoURL)
}

func (db *DB) getGitRepoByKey(ctx context.Context, q queryable, name, gitRepoURL string) (GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + `
		FROM git_repos
		WHERE name = $1 AND git_repo_url = $2`
	return scanGitRepo(q.QueryRowContext(ctx, query, name, gitRepoURL))
}

// GetGitRepoByName returns the most recently fetched git repo with the
// given name. When multiple git_repo_url entries exist for the same name,
// the one with the latest last_fetched_at wins (matching the previous
// DISTINCT ON behaviour for the old cookbooks table). Returns ErrNotFound
// if no match exists.
func (db *DB) GetGitRepoByName(ctx context.Context, name string) (GitRepo, error) {
	return db.getGitRepoByName(ctx, db.q(), name)
}

func (db *DB) getGitRepoByName(ctx context.Context, q queryable, name string) (GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + `
		FROM git_repos
		WHERE name = $1
		ORDER BY last_fetched_at DESC NULLS LAST
		LIMIT 1`
	return scanGitRepo(q.QueryRowContext(ctx, query, name))
}

// ListGitRepos returns all git repos, ordered by name. When multiple rows
// exist for the same cookbook name (different git_repo_url values), only the
// most recently fetched row per name is returned (DISTINCT ON).
func (db *DB) ListGitRepos(ctx context.Context) ([]GitRepo, error) {
	return db.listGitRepos(ctx, db.q())
}

func (db *DB) listGitRepos(ctx context.Context, q queryable) ([]GitRepo, error) {
	query := `SELECT DISTINCT ON (name) ` + gitRepoColumns + `
		FROM git_repos
		ORDER BY name, last_fetched_at DESC NULLS LAST`
	return scanGitRepos(q.QueryContext(ctx, query))
}

// ListAllGitRepos returns every git repo row without deduplication, ordered
// by name then git_repo_url. This is useful for operations that need to
// process all URLs (e.g. git clone/pull, committer extraction).
func (db *DB) ListAllGitRepos(ctx context.Context) ([]GitRepo, error) {
	return db.listAllGitRepos(ctx, db.q())
}

func (db *DB) listAllGitRepos(ctx context.Context, q queryable) ([]GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + `
		FROM git_repos
		ORDER BY name, git_repo_url`
	return scanGitRepos(q.QueryContext(ctx, query))
}

// ListGitReposByName returns all git repo rows with the given cookbook name,
// ordered by last_fetched_at DESC. There may be multiple rows if the same
// cookbook is available at different git URLs.
func (db *DB) ListGitReposByName(ctx context.Context, name string) ([]GitRepo, error) {
	return db.listGitReposByName(ctx, db.q(), name)
}

func (db *DB) listGitReposByName(ctx context.Context, q queryable, name string) ([]GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + `
		FROM git_repos
		WHERE name = $1
		ORDER BY last_fetched_at DESC NULLS LAST`
	return scanGitRepos(q.QueryContext(ctx, query, name))
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteGitRepoResult holds the outcome of a DeleteGitReposByName
// operation, including how many repo and committer rows were removed and
// which git repo URLs were cleaned up.
type DeleteGitRepoResult struct {
	ReposDeleted      int
	CommittersDeleted int
	RepoURLs          []string
}

// DeleteGitReposByName removes all git repo rows for the given cookbook name
// and deletes associated committer data from git_repo_committers. There may
// be multiple rows for the same cookbook name with different git_repo_url
// values (stale data from URL changes); this method cleans up all of them
// in a single transaction.
//
// Cascading foreign-key deletes handle cookstyle results, test kitchen
// results, autocorrect previews, and complexity records automatically.
//
// Returns ErrNotFound if no git repo with that name exists.
func (db *DB) DeleteGitReposByName(ctx context.Context, name string) (DeleteGitRepoResult, error) {
	var result DeleteGitRepoResult

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		// Collect all git_repo_url values for this name so we can clean up
		// committer data after deleting the repo rows.
		rows, err := tx.QueryContext(ctx,
			`SELECT git_repo_url FROM git_repos WHERE name = $1`,
			name,
		)
		if err != nil {
			return fmt.Errorf("datastore: selecting git repo URLs for %q: %w", name, err)
		}
		defer rows.Close()

		for rows.Next() {
			var url string
			if err := rows.Scan(&url); err != nil {
				return fmt.Errorf("datastore: scanning git repo URL: %w", err)
			}
			result.RepoURLs = append(result.RepoURLs, url)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("datastore: iterating git repo URLs: %w", err)
		}

		// Delete all git repo rows for this name. Cascading FK deletes
		// remove cookstyle results, test kitchen results, autocorrect
		// previews, and complexity records.
		res, err := tx.ExecContext(ctx,
			`DELETE FROM git_repos WHERE name = $1`,
			name,
		)
		if err != nil {
			return fmt.Errorf("datastore: deleting git repos for %q: %w", name, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("datastore: checking rows affected: %w", err)
		}
		if n == 0 {
			return ErrNotFound
		}
		result.ReposDeleted = int(n)

		// Delete committer data for all collected repo URLs.
		if len(result.RepoURLs) > 0 {
			res, err := tx.ExecContext(ctx,
				`DELETE FROM git_repo_committers WHERE git_repo_url = ANY($1)`,
				stringSliceToArray(result.RepoURLs),
			)
			if err != nil {
				return fmt.Errorf("datastore: deleting committers for git repo %q: %w", name, err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("datastore: checking committer rows affected: %w", err)
			}
			result.CommittersDeleted = int(n)
		}

		return nil
	})
	if err != nil {
		return DeleteGitRepoResult{}, err
	}
	return result, nil
}

// DeleteGitRepo removes a single git repo by UUID. Returns ErrNotFound if
// no such git repo exists. Cascading deletes handle dependent rows.
func (db *DB) DeleteGitRepo(ctx context.Context, id string) error {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM git_repos WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("datastore: deleting git repo: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanGitRepo(row *sql.Row) (GitRepo, error) {
	var gr GitRepo
	var commitSHA, branch sql.NullString
	var lastFetched sql.NullTime

	err := row.Scan(
		&gr.ID,
		&gr.Name,
		&gr.GitRepoURL,
		&commitSHA,
		&branch,
		&gr.HasTestSuite,
		&lastFetched,
		&gr.CreatedAt,
		&gr.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return GitRepo{}, ErrNotFound
		}
		return GitRepo{}, fmt.Errorf("datastore: scanning git repo: %w", err)
	}

	gr.HeadCommitSHA = stringFromNull(commitSHA)
	gr.DefaultBranch = stringFromNull(branch)
	gr.LastFetchedAt = timeFromNull(lastFetched)
	return gr, nil
}

func scanGitRepos(rows *sql.Rows, err error) ([]GitRepo, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying git repos: %w", err)
	}
	defer rows.Close()

	var repos []GitRepo
	for rows.Next() {
		var gr GitRepo
		var commitSHA, branch sql.NullString
		var lastFetched sql.NullTime

		if err := rows.Scan(
			&gr.ID,
			&gr.Name,
			&gr.GitRepoURL,
			&commitSHA,
			&branch,
			&gr.HasTestSuite,
			&lastFetched,
			&gr.CreatedAt,
			&gr.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning git repo row: %w", err)
		}

		gr.HeadCommitSHA = stringFromNull(commitSHA)
		gr.DefaultBranch = stringFromNull(branch)
		gr.LastFetchedAt = timeFromNull(lastFetched)
		repos = append(repos, gr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating git repo rows: %w", err)
	}
	return repos, nil
}
