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

// Cookbook represents a row in the legacy cookbooks table. This type is
// retained temporarily during the refactor to support callers that have not
// yet been migrated to ServerCookbook / GitRepo. It will be deleted once all
// consumers are updated.
//
// Download status constants are shared with ServerCookbook.
const (
	DownloadStatusOK      = "ok"      // Content fetched successfully
	DownloadStatusFailed  = "failed"  // Download attempted but failed
	DownloadStatusPending = "pending" // Not yet downloaded
)

type Cookbook struct {
	ID              string    `json:"id"`
	OrganisationID  string    `json:"organisation_id,omitempty"`
	Name            string    `json:"name"`
	Version         string    `json:"version,omitempty"`
	Source          string    `json:"source"` // "git" or "chef_server"
	GitRepoURL      string    `json:"git_repo_url,omitempty"`
	HeadCommitSHA   string    `json:"head_commit_sha,omitempty"`
	DefaultBranch   string    `json:"default_branch,omitempty"`
	HasTestSuite    bool      `json:"has_test_suite"`
	IsActive        bool      `json:"is_active"`
	IsStaleCookbook bool      `json:"is_stale_cookbook"`
	DownloadStatus  string    `json:"download_status"`          // "ok", "failed", or "pending"
	DownloadError   string    `json:"download_error,omitempty"` // Error detail when status = "failed"
	FirstSeenAt     time.Time `json:"first_seen_at,omitempty"`
	LastFetchedAt   time.Time `json:"last_fetched_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// IsGit returns true if the cookbook is sourced from a git repository.
func (c Cookbook) IsGit() bool {
	return c.Source == "git"
}

// IsChefServer returns true if the cookbook is sourced from a Chef server.
func (c Cookbook) IsChefServer() bool {
	return c.Source == "chef_server"
}

// IsDownloaded returns true if the cookbook content has been successfully
// fetched (download_status = 'ok').
func (c Cookbook) IsDownloaded() bool {
	return c.DownloadStatus == DownloadStatusOK
}

// NeedsDownload returns true if the cookbook has a pending or failed download
// status and should be (re-)downloaded on the next collection run.
func (c Cookbook) NeedsDownload() bool {
	return c.DownloadStatus == DownloadStatusPending || c.DownloadStatus == DownloadStatusFailed
}

// MarshalJSON implements json.Marshaler for Cookbook.
func (c Cookbook) MarshalJSON() ([]byte, error) {
	type Alias Cookbook
	return json.Marshal((Alias)(c))
}

// ---------------------------------------------------------------------------
// Upsert — Git cookbooks (retained until Commit 4 introduces git_repos.go)
// ---------------------------------------------------------------------------

// UpsertGitCookbookParams holds the fields required to upsert a cookbook
// sourced from a git repository. The upsert key is (name, git_repo_url).
type UpsertGitCookbookParams struct {
	Name            string
	GitRepoURL      string
	HeadCommitSHA   string
	DefaultBranch   string
	HasTestSuite    bool
	IsActive        bool
	IsStaleCookbook bool
	FirstSeenAt     time.Time
	LastFetchedAt   time.Time
}

// UpsertGitCookbook inserts or updates a git-sourced cookbook.
func (db *DB) UpsertGitCookbook(ctx context.Context, p UpsertGitCookbookParams) (Cookbook, error) {
	return db.upsertGitCookbook(ctx, db.q(), p)
}

func (db *DB) upsertGitCookbook(ctx context.Context, q queryable, p UpsertGitCookbookParams) (Cookbook, error) {
	if p.Name == "" {
		return Cookbook{}, fmt.Errorf("datastore: cookbook name is required")
	}
	if p.GitRepoURL == "" {
		return Cookbook{}, fmt.Errorf("datastore: git repo URL is required for git cookbook")
	}
	if p.LastFetchedAt.IsZero() {
		p.LastFetchedAt = time.Now().UTC()
	}
	if p.FirstSeenAt.IsZero() {
		p.FirstSeenAt = time.Now().UTC()
	}

	const query = `
		INSERT INTO cookbooks (
			name, source, git_repo_url, head_commit_sha, default_branch,
			has_test_suite, is_active, is_stale_cookbook,
			first_seen_at, last_fetched_at
		) VALUES (
			$1, 'git', $2, $3, $4,
			$5, $6, $7,
			$8, $9
		)
		ON CONFLICT (name, git_repo_url) WHERE source = 'git'
		DO UPDATE SET
			head_commit_sha   = EXCLUDED.head_commit_sha,
			default_branch    = EXCLUDED.default_branch,
			has_test_suite    = EXCLUDED.has_test_suite,
			is_active         = EXCLUDED.is_active,
			is_stale_cookbook  = EXCLUDED.is_stale_cookbook,
			last_fetched_at   = EXCLUDED.last_fetched_at,
			updated_at        = now()
		RETURNING id, organisation_id, name, version, source,
		          git_repo_url, head_commit_sha, default_branch,
		          has_test_suite, is_active, is_stale_cookbook,
		          download_status, download_error,
		          first_seen_at, last_fetched_at, created_at, updated_at
	`

	return scanCookbook(q.QueryRowContext(ctx, query,
		p.Name,
		p.GitRepoURL,
		nullString(p.HeadCommitSHA),
		nullString(p.DefaultBranch),
		p.HasTestSuite,
		p.IsActive,
		p.IsStaleCookbook,
		p.FirstSeenAt,
		p.LastFetchedAt,
	))
}

// ---------------------------------------------------------------------------
// Query methods (retained until callers are migrated)
// ---------------------------------------------------------------------------

// GetCookbook returns the cookbook with the given UUID. Returns ErrNotFound
// if no such cookbook exists.
func (db *DB) GetCookbook(ctx context.Context, id string) (Cookbook, error) {
	return db.getCookbook(ctx, db.q(), id)
}

func (db *DB) getCookbook(ctx context.Context, q queryable, id string) (Cookbook, error) {
	const query = `
		SELECT id, organisation_id, name, version, source,
		       git_repo_url, head_commit_sha, default_branch,
		       has_test_suite, is_active, is_stale_cookbook,
		       download_status, download_error,
		       first_seen_at, last_fetched_at, created_at, updated_at
		FROM cookbooks
		WHERE id = $1
	`
	return scanCookbook(q.QueryRowContext(ctx, query, id))
}

// GetGitCookbook returns a git-sourced cookbook by name and git repo URL.
// Returns ErrNotFound if no match exists.
func (db *DB) GetGitCookbook(ctx context.Context, name, gitRepoURL string) (Cookbook, error) {
	return db.getGitCookbook(ctx, db.q(), name, gitRepoURL)
}

func (db *DB) getGitCookbook(ctx context.Context, q queryable, name, gitRepoURL string) (Cookbook, error) {
	const query = `
		SELECT id, organisation_id, name, version, source,
		       git_repo_url, head_commit_sha, default_branch,
		       has_test_suite, is_active, is_stale_cookbook,
		       download_status, download_error,
		       first_seen_at, last_fetched_at, created_at, updated_at
		FROM cookbooks
		WHERE name = $1 AND git_repo_url = $2 AND source = 'git'
	`
	return scanCookbook(q.QueryRowContext(ctx, query, name, gitRepoURL))
}

// ListGitCookbooks returns all git-sourced cookbooks, ordered by name.
func (db *DB) ListGitCookbooks(ctx context.Context) ([]Cookbook, error) {
	return db.listGitCookbooks(ctx, db.q())
}

func (db *DB) listGitCookbooks(ctx context.Context, q queryable) ([]Cookbook, error) {
	const query = `
		SELECT DISTINCT ON (name)
		       id, organisation_id, name, version, source,
		       git_repo_url, head_commit_sha, default_branch,
		       has_test_suite, is_active, is_stale_cookbook,
		       download_status, download_error,
		       first_seen_at, last_fetched_at, created_at, updated_at
		FROM cookbooks
		WHERE source = 'git'
		ORDER BY name, last_fetched_at DESC NULLS LAST
	`
	return scanCookbooks(q.QueryContext(ctx, query))
}

// ---------------------------------------------------------------------------
// Delete (git cookbooks)
// ---------------------------------------------------------------------------

// DeleteGitCookbookResult holds the outcome of a DeleteGitCookbooksByName
// operation, including how many cookbook and committer rows were removed and
// which git repo URLs were cleaned up.
type DeleteGitCookbookResult struct {
	CookbooksDeleted  int
	CommittersDeleted int
	RepoURLs          []string
}

// DeleteGitCookbooksByName removes all git-sourced cookbook rows for the given
// cookbook name and deletes associated committer data from
// git_repo_committers. There may be multiple rows for the same cookbook name
// with different git_repo_url values (stale data from URL changes); this
// method cleans up all of them in a single transaction.
//
// Cascading foreign-key deletes handle cookstyle results, complexity records,
// and other dependent rows automatically.
//
// Returns ErrNotFound if no git-sourced cookbook with that name exists.
func (db *DB) DeleteGitCookbooksByName(ctx context.Context, cookbookName string) (DeleteGitCookbookResult, error) {
	var result DeleteGitCookbookResult

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		// Collect all git_repo_url values for this cookbook name so we can
		// clean up committer data after deleting the cookbook rows.
		rows, err := tx.QueryContext(ctx,
			`SELECT git_repo_url FROM cookbooks
			 WHERE name = $1 AND source = 'git' AND git_repo_url IS NOT NULL`,
			cookbookName,
		)
		if err != nil {
			return fmt.Errorf("datastore: selecting git repo URLs for cookbook %q: %w", cookbookName, err)
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

		// Delete all git-sourced cookbook rows for this name. Cascading FK
		// deletes remove cookstyle results, complexity, test results, etc.
		res, err := tx.ExecContext(ctx,
			`DELETE FROM cookbooks WHERE name = $1 AND source = 'git'`,
			cookbookName,
		)
		if err != nil {
			return fmt.Errorf("datastore: deleting git cookbooks for %q: %w", cookbookName, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("datastore: checking rows affected: %w", err)
		}
		if n == 0 {
			return ErrNotFound
		}
		result.CookbooksDeleted = int(n)

		// Delete committer data for all collected repo URLs.
		if len(result.RepoURLs) > 0 {
			res, err := tx.ExecContext(ctx,
				`DELETE FROM git_repo_committers WHERE git_repo_url = ANY($1)`,
				stringSliceToArray(result.RepoURLs),
			)
			if err != nil {
				return fmt.Errorf("datastore: deleting committers for cookbook %q: %w", cookbookName, err)
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
		return DeleteGitCookbookResult{}, err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers (legacy Cookbook type)
// ---------------------------------------------------------------------------

func scanCookbook(row *sql.Row) (Cookbook, error) {
	var cb Cookbook
	var orgID, version, gitURL, commitSHA, branch, dlError sql.NullString
	var firstSeen, lastFetched sql.NullTime

	err := row.Scan(
		&cb.ID,
		&orgID,
		&cb.Name,
		&version,
		&cb.Source,
		&gitURL,
		&commitSHA,
		&branch,
		&cb.HasTestSuite,
		&cb.IsActive,
		&cb.IsStaleCookbook,
		&cb.DownloadStatus,
		&dlError,
		&firstSeen,
		&lastFetched,
		&cb.CreatedAt,
		&cb.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Cookbook{}, ErrNotFound
		}
		return Cookbook{}, fmt.Errorf("datastore: scanning cookbook: %w", err)
	}

	cb.OrganisationID = stringFromNull(orgID)
	cb.Version = stringFromNull(version)
	cb.GitRepoURL = stringFromNull(gitURL)
	cb.HeadCommitSHA = stringFromNull(commitSHA)
	cb.DefaultBranch = stringFromNull(branch)
	cb.DownloadError = stringFromNull(dlError)
	cb.FirstSeenAt = timeFromNull(firstSeen)
	cb.LastFetchedAt = timeFromNull(lastFetched)
	return cb, nil
}

func scanCookbooks(rows *sql.Rows, err error) ([]Cookbook, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying cookbooks: %w", err)
	}
	defer rows.Close()

	var cookbooks []Cookbook
	for rows.Next() {
		var cb Cookbook
		var orgID, version, gitURL, commitSHA, branch, dlError sql.NullString
		var firstSeen, lastFetched sql.NullTime

		if err := rows.Scan(
			&cb.ID,
			&orgID,
			&cb.Name,
			&version,
			&cb.Source,
			&gitURL,
			&commitSHA,
			&branch,
			&cb.HasTestSuite,
			&cb.IsActive,
			&cb.IsStaleCookbook,
			&cb.DownloadStatus,
			&dlError,
			&firstSeen,
			&lastFetched,
			&cb.CreatedAt,
			&cb.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookbook row: %w", err)
		}

		cb.OrganisationID = stringFromNull(orgID)
		cb.Version = stringFromNull(version)
		cb.GitRepoURL = stringFromNull(gitURL)
		cb.HeadCommitSHA = stringFromNull(commitSHA)
		cb.DefaultBranch = stringFromNull(branch)
		cb.DownloadError = stringFromNull(dlError)
		cb.FirstSeenAt = timeFromNull(firstSeen)
		cb.LastFetchedAt = timeFromNull(lastFetched)
		cookbooks = append(cookbooks, cb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookbook rows: %w", err)
	}
	return cookbooks, nil
}
