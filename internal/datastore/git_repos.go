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

// GitRepo represents a row in the git_repos table. The composite natural
// primary key is (Name, GitRepoURL). Git repos are not org-scoped — they
// are matched by name across organisations.
type GitRepo struct {
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

	KitchenExcluded      bool       `json:"kitchen_excluded"`
	KitchenExcludeReason string     `json:"kitchen_exclude_reason,omitempty"`
	KitchenExcludedBy    string     `json:"kitchen_excluded_by,omitempty"`
	KitchenExcludedAt    *time.Time `json:"kitchen_excluded_at,omitempty"`

	// Materialised status columns — pre-computed for the active target version.
	CompatibilityStatus string `json:"compatibility_status"`
	TKStatus            string `json:"tk_status"`
	TKPassed            int    `json:"tk_passed"`
	TKTotal             int    `json:"tk_total"`
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
	name, git_repo_url, head_commit_sha, default_branch,
	has_test_suite, clone_status, clone_error,
	last_fetched_at, created_at, updated_at,
	kitchen_excluded, kitchen_exclude_reason, kitchen_excluded_by, kitchen_excluded_at,
	compatibility_status, tk_status, tk_passed, tk_total
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

// UpsertGitRepo inserts or updates a git repo row. If a row for this
// (name, git_repo_url) already exists, its HEAD, branch, test suite flag,
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
		ON CONFLICT (name, git_repo_url)
		DO UPDATE SET
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

// GetGitRepoByKey returns a git repo by its composite primary key
// (name, git_repo_url). Returns ErrNotFound if no such git repo exists.
func (db *DB) GetGitRepoByKey(ctx context.Context, name, gitRepoURL string) (GitRepo, error) {
	return db.getGitRepoByKey(ctx, db.q(), name, gitRepoURL)
}

func (db *DB) getGitRepoByKey(ctx context.Context, q queryable, name, gitRepoURL string) (GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + `
		FROM git_repos
		WHERE name = $1 AND git_repo_url = $2`
	return scanGitRepo(q.QueryRowContext(ctx, query, name, gitRepoURL))
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
func (db *DB) ListGitRepos(ctx context.Context) ([]GitRepo, error) {
	return db.listGitRepos(ctx, db.q())
}

func (db *DB) listGitRepos(ctx context.Context, q queryable) ([]GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + `
		FROM git_repos
		ORDER BY name`
	return scanGitRepos(q.QueryContext(ctx, query))
}

// ListGitReposByName returns the git repo rows for the given cookbook name.
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
func (db *DB) MarkGitRepoCloneOK(ctx context.Context, name, gitRepoURL string) (GitRepo, error) {
	const query = `
		UPDATE git_repos
		   SET clone_status = 'ok',
		       clone_error  = NULL,
		       updated_at   = now()
		 WHERE name = $1 AND git_repo_url = $2
		RETURNING ` + gitRepoColumns
	return scanGitRepo(db.pool.QueryRowContext(ctx, query, name, gitRepoURL))
}

// MarkGitRepoCloneFailed marks a git repo clone as failed with the given
// error detail.
func (db *DB) MarkGitRepoCloneFailed(ctx context.Context, name, gitRepoURL, cloneError string) (GitRepo, error) {
	var ce sql.NullString
	if cloneError != "" {
		ce = sql.NullString{String: cloneError, Valid: true}
	}
	const query = `
		UPDATE git_repos
		   SET clone_status = 'failed',
		       clone_error  = $3,
		       updated_at   = now()
		 WHERE name = $1 AND git_repo_url = $2
		RETURNING ` + gitRepoColumns
	return scanGitRepo(db.pool.QueryRowContext(ctx, query, name, gitRepoURL, ce))
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
		ON CONFLICT (name, git_repo_url)
		DO UPDATE SET
			clone_status = 'failed',
			clone_error  = EXCLUDED.clone_error,
			updated_at   = now()
		RETURNING ` + gitRepoColumns

	return scanGitRepo(db.pool.QueryRowContext(ctx, query, name, gitRepoURL, ce))
}

// ResetGitRepoCloneStatus resets the clone_status to 'pending' and clears
// the clone_error. This forces a fresh clone attempt on the next run.
func (db *DB) ResetGitRepoCloneStatus(ctx context.Context, name, gitRepoURL string) (GitRepo, error) {
	const query = `
		UPDATE git_repos
		   SET clone_status = 'pending',
		       clone_error  = NULL,
		       updated_at   = now()
		 WHERE name = $1 AND git_repo_url = $2
		RETURNING ` + gitRepoColumns
	return scanGitRepo(db.pool.QueryRowContext(ctx, query, name, gitRepoURL))
}

// ListClonedGitRepos returns only repos with clone_status = 'ok'.
// Use this when feeding repos to scanners that need a local clone.
func (db *DB) ListClonedGitRepos(ctx context.Context) ([]GitRepo, error) {
	query := `SELECT ` + gitRepoColumns + `
		FROM git_repos
		WHERE clone_status = 'ok'
		ORDER BY name`
	return scanGitRepos(db.pool.QueryContext(ctx, query))
}

// DeleteStaleGitRepos removes git_repos rows for a cookbook name where the
// URL differs from keepURL. This cleans up stale rows left behind when a
// cookbook migrates between git orgs. FK cascades handle cookstyle,
// complexity, autocorrect, and kitchen analysis rows. Committers are
// cleaned up explicitly since they have no FK to git_repos.
func (db *DB) DeleteStaleGitRepos(ctx context.Context, name, keepURL string) (int64, error) {
	return db.deleteStaleGitRepos(ctx, name, keepURL)
}

func (db *DB) deleteStaleGitRepos(ctx context.Context, name, keepURL string) (int64, error) {
	var deleted int64
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		// Find URLs we are about to delete so we can clean up committers.
		rows, err := tx.QueryContext(ctx,
			`SELECT git_repo_url FROM git_repos WHERE name = $1 AND git_repo_url != $2`,
			name, keepURL)
		if err != nil {
			return fmt.Errorf("listing stale URLs: %w", err)
		}
		var staleURLs []string
		for rows.Next() {
			var u string
			if err := rows.Scan(&u); err != nil {
				rows.Close()
				return fmt.Errorf("scanning stale URL: %w", err)
			}
			staleURLs = append(staleURLs, u)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterating stale URLs: %w", err)
		}

		if len(staleURLs) == 0 {
			return nil
		}

		// Delete committers for stale URLs (no FK cascade).
		for _, u := range staleURLs {
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM git_repo_committers WHERE git_repo_url = $1`, u); err != nil {
				return fmt.Errorf("deleting committers for %s: %w", u, err)
			}
		}

		// Delete the stale git_repos rows (FK cascades handle the rest).
		res, err := tx.ExecContext(ctx,
			`DELETE FROM git_repos WHERE name = $1 AND git_repo_url != $2`,
			name, keepURL)
		if err != nil {
			return fmt.Errorf("deleting stale git repos: %w", err)
		}
		deleted, err = res.RowsAffected()
		if err != nil {
			return fmt.Errorf("checking rows affected: %w", err)
		}
		return nil
	})
	return deleted, err
}

// DeleteGitRepo removes a single git repo by its composite primary key
// (name, git_repo_url). Returns ErrNotFound if no such git repo exists.
// Cascading deletes handle dependent rows.
func (db *DB) DeleteGitRepo(ctx context.Context, name, gitRepoURL string) error {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM git_repos WHERE name = $1 AND git_repo_url = $2`, name, gitRepoURL,
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
	var excludeReason, excludedBy sql.NullString
	var excludedAt sql.NullTime

	err := row.Scan(
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
		&gr.KitchenExcluded,
		&excludeReason,
		&excludedBy,
		&excludedAt,
		&gr.CompatibilityStatus,
		&gr.TKStatus,
		&gr.TKPassed,
		&gr.TKTotal,
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
	gr.KitchenExcludeReason = stringFromNull(excludeReason)
	gr.KitchenExcludedBy = stringFromNull(excludedBy)
	gr.KitchenExcludedAt = timePtrFromNull(excludedAt)
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
		var excludeReason, excludedBy sql.NullString
		var excludedAt sql.NullTime

		if err := rows.Scan(
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
			&gr.KitchenExcluded,
			&excludeReason,
			&excludedBy,
			&excludedAt,
			&gr.CompatibilityStatus,
			&gr.TKStatus,
			&gr.TKPassed,
			&gr.TKTotal,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning git repo row: %w", err)
		}

		gr.HeadCommitSHA = stringFromNull(commitSHA)
		gr.DefaultBranch = stringFromNull(branch)
		gr.CloneError = stringFromNull(cloneErr)
		gr.LastFetchedAt = timeFromNull(lastFetched)
		gr.KitchenExcludeReason = stringFromNull(excludeReason)
		gr.KitchenExcludedBy = stringFromNull(excludedBy)
		gr.KitchenExcludedAt = timePtrFromNull(excludedAt)
		repos = append(repos, gr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating git repo rows: %w", err)
	}
	return repos, nil
}
