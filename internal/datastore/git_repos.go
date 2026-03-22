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

// Clone status constants mirror the download status constants used by
// server_cookbooks, providing a three-state lifecycle for git repos.
const (
	CloneStatusOK      = "ok"      // Successfully cloned or pulled
	CloneStatusFailed  = "failed"  // Clone/pull attempted but failed
	CloneStatusPending = "pending" // Not yet attempted
)

// GitRepo represents a row in the git_repos table. Each row is a unique
// cookbook name. The git_repo_url records which URL the repo was cloned from.
// Git repos are not org-scoped — they are matched by name across organisations.
type GitRepo struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	GitRepoURL    string    `json:"git_repo_url"`
	HeadCommitSHA string    `json:"head_commit_sha,omitempty"`
	DefaultBranch string    `json:"default_branch,omitempty"`
	HasTestSuite  bool      `json:"has_test_suite"`
	CloneStatus   string    `json:"clone_status"`
	CloneError    string    `json:"clone_error,omitempty"`
	LastFetchedAt time.Time `json:"last_fetched_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// IsCloned returns true when the git repo has been successfully cloned.
func (gr GitRepo) IsCloned() bool {
	return gr.CloneStatus == CloneStatusOK
}

// NeedsClone returns true when the git repo needs a clone attempt
// (either never attempted or previously failed).
func (gr GitRepo) NeedsClone() bool {
	return gr.CloneStatus == CloneStatusPending || gr.CloneStatus == CloneStatusFailed
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
	has_test_suite, clone_status, clone_error,
	last_fetched_at, created_at, updated_at
`

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertGitRepoParams holds the fields required to upsert a git repo.
// The upsert key is (name).
type UpsertGitRepoParams struct {
	Name          string
	GitRepoURL    string
	HeadCommitSHA string
	DefaultBranch string
	HasTestSuite  bool
	LastFetchedAt time.Time
}

// UpsertGitRepo inserts or updates a git repo row. If a row for this
// cookbook name already exists, its URL, HEAD, branch, test suite flag,
// and clone status are updated.
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
			has_test_suite, clone_status, clone_error, last_fetched_at
		) VALUES (
			$1, $2, $3, $4, $5, 'ok', NULL, $6
		)
		ON CONFLICT (name)
		DO UPDATE SET
			git_repo_url    = EXCLUDED.git_repo_url,
			head_commit_sha = EXCLUDED.head_commit_sha,
			default_branch  = EXCLUDED.default_branch,
			has_test_suite  = EXCLUDED.has_test_suite,
			clone_status    = 'ok',
			clone_error     = NULL,
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

// GetGitRepoByName returns the git repo with the given cookbook name.
// Returns ErrNotFound if no match exists.
func (db *DB) GetGitRepoByName(ctx context.Context, name string) (GitRepo, error) {
	return db.getGitRepoByName(ctx, db.q(), name)
}

func (db *DB) getGitRepoByName(ctx context.Context, q queryable, name string) (GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + `
		FROM git_repos
		WHERE name = $1`
	return scanGitRepo(q.QueryRowContext(ctx, query, name))
}

// ListGitRepos returns all git repos, ordered by name.
// There is exactly one row per cookbook name.
func (db *DB) ListGitRepos(ctx context.Context) ([]GitRepo, error) {
	return db.listGitRepos(ctx, db.q())
}

func (db *DB) listGitRepos(ctx context.Context, q queryable) ([]GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + `
		FROM git_repos
		ORDER BY name`
	return scanGitRepos(q.QueryContext(ctx, query))
}

// ListGitReposByName returns the git repo row for the given cookbook name.
// The result is returned as a slice for API compatibility, but will contain
// at most one element since name is unique.
func (db *DB) ListGitReposByName(ctx context.Context, name string) ([]GitRepo, error) {
	return db.listGitReposByName(ctx, db.q(), name)
}

func (db *DB) listGitReposByName(ctx context.Context, q queryable, name string) ([]GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + `
		FROM git_repos
		WHERE name = $1`
	return scanGitRepos(q.QueryContext(ctx, query, name))
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteGitRepoResult holds the outcome of a DeleteGitReposByName
// operation, including how many repo and committer rows were removed and
// which git repo URL was cleaned up.
type DeleteGitRepoResult struct {
	ReposDeleted      int
	CommittersDeleted int
	RepoURLs          []string
}

// DeleteGitReposByName removes the git repo row for the given cookbook name
// and deletes associated committer data from git_repo_committers.
//
// Cascading foreign-key deletes handle cookstyle results, test kitchen
// results, autocorrect previews, and complexity records automatically.
//
// Returns ErrNotFound if no git repo with that name exists.
func (db *DB) DeleteGitReposByName(ctx context.Context, name string) (DeleteGitRepoResult, error) {
	var result DeleteGitRepoResult

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		// Collect the git_repo_url so we can clean up committer data.
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

		// Delete the git repo row. Cascading FK deletes remove cookstyle
		// results, test kitchen results, autocorrect previews, and
		// complexity records.
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

		// Delete committer data for the repo URL.
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

// ---------------------------------------------------------------------------
// Clone status management
// ---------------------------------------------------------------------------

// MarkGitRepoCloneOK marks a git repo as successfully cloned/pulled and
// clears any previous clone error.
func (db *DB) MarkGitRepoCloneOK(ctx context.Context, id string) (GitRepo, error) {
	const query = `
		UPDATE git_repos
		   SET clone_status = 'ok',
		       clone_error  = NULL,
		       updated_at   = now()
		 WHERE id = $1
		RETURNING ` + gitRepoColumns
	return scanGitRepo(db.pool.QueryRowContext(ctx, query, id))
}

// MarkGitRepoCloneFailed marks a git repo clone as failed with the given
// error detail.
func (db *DB) MarkGitRepoCloneFailed(ctx context.Context, id, cloneError string) (GitRepo, error) {
	var ce sql.NullString
	if cloneError != "" {
		ce = sql.NullString{String: cloneError, Valid: true}
	}
	const query = `
		UPDATE git_repos
		   SET clone_status = 'failed',
		       clone_error  = $2,
		       updated_at   = now()
		 WHERE id = $1
		RETURNING ` + gitRepoColumns
	return scanGitRepo(db.pool.QueryRowContext(ctx, query, id, ce))
}

// UpsertGitRepoFailed inserts or updates a git repo row in the 'failed'
// state. This is used when a clone fails from all configured base URLs —
// the repo row is created (or updated) so it appears in the UI as missing,
// rather than being silently absent.
func (db *DB) UpsertGitRepoFailed(ctx context.Context, name, gitRepoURL, cloneError string) (GitRepo, error) {
	if name == "" {
		return GitRepo{}, fmt.Errorf("datastore: git repo name is required")
	}
	if gitRepoURL == "" {
		return GitRepo{}, fmt.Errorf("datastore: git repo URL is required")
	}
	var ce sql.NullString
	if cloneError != "" {
		ce = sql.NullString{String: cloneError, Valid: true}
	}
	const query = `
		INSERT INTO git_repos (
			name, git_repo_url, clone_status, clone_error
		) VALUES (
			$1, $2, 'failed', $3
		)
		ON CONFLICT (name)
		DO UPDATE SET
			git_repo_url = EXCLUDED.git_repo_url,
			clone_status = 'failed',
			clone_error  = EXCLUDED.clone_error,
			updated_at   = now()
		RETURNING ` + gitRepoColumns

	return scanGitRepo(db.pool.QueryRowContext(ctx, query, name, gitRepoURL, ce))
}

// ResetGitRepoCloneStatus resets the clone_status to 'pending' and clears
// the clone_error. This forces a fresh clone attempt on the next run.
func (db *DB) ResetGitRepoCloneStatus(ctx context.Context, id string) (GitRepo, error) {
	const query = `
		UPDATE git_repos
		   SET clone_status = 'pending',
		       clone_error  = NULL,
		       updated_at   = now()
		 WHERE id = $1
		RETURNING ` + gitRepoColumns
	return scanGitRepo(db.pool.QueryRowContext(ctx, query, id))
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
	var commitSHA, branch, cloneErr sql.NullString
	var lastFetched sql.NullTime

	err := row.Scan(
		&gr.ID,
		&gr.Name,
		&gr.GitRepoURL,
		&commitSHA,
		&branch,
		&gr.HasTestSuite,
		&gr.CloneStatus,
		&cloneErr,
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
	gr.CloneError = stringFromNull(cloneErr)
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
		var commitSHA, branch, cloneErr sql.NullString
		var lastFetched sql.NullTime

		if err := rows.Scan(
			&gr.ID,
			&gr.Name,
			&gr.GitRepoURL,
			&commitSHA,
			&branch,
			&gr.HasTestSuite,
			&gr.CloneStatus,
			&cloneErr,
			&lastFetched,
			&gr.CreatedAt,
			&gr.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning git repo row: %w", err)
		}

		gr.HeadCommitSHA = stringFromNull(commitSHA)
		gr.DefaultBranch = stringFromNull(branch)
		gr.CloneError = stringFromNull(cloneErr)
		gr.LastFetchedAt = timeFromNull(lastFetched)
		repos = append(repos, gr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating git repo rows: %w", err)
	}
	return repos, nil
}
