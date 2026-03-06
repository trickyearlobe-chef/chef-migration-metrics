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

// Cookbook represents a row in the cookbooks table. A cookbook can be sourced
// from a Chef server (has organisation_id, name, version) or from a git
// repository (has name, git_repo_url, head_commit_sha).
// Download status constants for the cookbooks table.
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
// Upsert — Chef Server cookbooks
// ---------------------------------------------------------------------------

// UpsertServerCookbookParams holds the fields required to upsert a cookbook
// sourced from a Chef server. The upsert key is (organisation_id, name,
// version) — Chef server cookbook versions are immutable, so an existing row
// is only updated with metadata changes (active status, stale flag, etc.).
type UpsertServerCookbookParams struct {
	OrganisationID  string
	Name            string
	Version         string
	HasTestSuite    bool
	IsActive        bool
	IsStaleCookbook bool
	FirstSeenAt     time.Time // set on first insert, not overwritten on update
	LastFetchedAt   time.Time
}

// UpsertServerCookbook inserts or updates a Chef-server-sourced cookbook.
func (db *DB) UpsertServerCookbook(ctx context.Context, p UpsertServerCookbookParams) (Cookbook, error) {
	return db.upsertServerCookbook(ctx, db.q(), p)
}

func (db *DB) upsertServerCookbook(ctx context.Context, q queryable, p UpsertServerCookbookParams) (Cookbook, error) {
	if p.OrganisationID == "" {
		return Cookbook{}, fmt.Errorf("datastore: organisation ID is required for server cookbook")
	}
	if p.Name == "" {
		return Cookbook{}, fmt.Errorf("datastore: cookbook name is required")
	}
	if p.Version == "" {
		return Cookbook{}, fmt.Errorf("datastore: cookbook version is required for server cookbook")
	}
	if p.LastFetchedAt.IsZero() {
		p.LastFetchedAt = time.Now().UTC()
	}
	if p.FirstSeenAt.IsZero() {
		p.FirstSeenAt = time.Now().UTC()
	}

	// The partial unique index uq_cookbooks_server covers (organisation_id, name, version)
	// WHERE source = 'chef_server'. We use a CTE to perform a conditional upsert
	// that works with the partial index.
	const query = `
		INSERT INTO cookbooks (
			organisation_id, name, version, source,
			has_test_suite, is_active, is_stale_cookbook,
			first_seen_at, last_fetched_at
		) VALUES (
			$1, $2, $3, 'chef_server',
			$4, $5, $6, $7, $8
		)
		ON CONFLICT (organisation_id, name, version) WHERE source = 'chef_server'
		DO UPDATE SET
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
		p.OrganisationID,
		p.Name,
		p.Version,
		p.HasTestSuite,
		p.IsActive,
		p.IsStaleCookbook,
		p.FirstSeenAt,
		p.LastFetchedAt,
	))
}

// ---------------------------------------------------------------------------
// Upsert — Git cookbooks
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
			download_status,
			first_seen_at, last_fetched_at
		) VALUES (
			$1, 'git', $2, $3, $4,
			$5, $6, $7, 'ok',
			$8, $9
		)
		ON CONFLICT (name, git_repo_url) WHERE source = 'git'
		DO UPDATE SET
			head_commit_sha   = EXCLUDED.head_commit_sha,
			default_branch    = EXCLUDED.default_branch,
			has_test_suite    = EXCLUDED.has_test_suite,
			is_active         = EXCLUDED.is_active,
			is_stale_cookbook  = EXCLUDED.is_stale_cookbook,
			download_status   = EXCLUDED.download_status,
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
// Bulk upsert
// ---------------------------------------------------------------------------

// BulkUpsertServerCookbooks upserts multiple Chef-server-sourced cookbooks
// within a single transaction for efficiency. Returns the count of rows
// upserted. If any upsert fails, the entire batch is rolled back.
func (db *DB) BulkUpsertServerCookbooks(ctx context.Context, params []UpsertServerCookbookParams) (int, error) {
	if len(params) == 0 {
		return 0, nil
	}

	upserted := 0
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		for i, p := range params {
			_, err := db.upsertServerCookbook(ctx, tx, p)
			if err != nil {
				return fmt.Errorf("row %d: %w", i, err)
			}
			upserted++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return upserted, nil
}

// ---------------------------------------------------------------------------
// Mark active/inactive
// ---------------------------------------------------------------------------

// MarkCookbooksActiveForOrg sets is_active = true for the named cookbooks
// (by name) within the given organisation, and is_active = false for all
// others. This is called after a collection run to reflect which cookbooks
// are actually in use by at least one node.
func (db *DB) MarkCookbooksActiveForOrg(ctx context.Context, organisationID string, activeNames []string) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		// Deactivate all server cookbooks for this org.
		_, err := tx.ExecContext(ctx,
			`UPDATE cookbooks SET is_active = FALSE, updated_at = now()
			 WHERE organisation_id = $1 AND source = 'chef_server'`,
			organisationID,
		)
		if err != nil {
			return fmt.Errorf("datastore: deactivating cookbooks: %w", err)
		}

		if len(activeNames) == 0 {
			return nil
		}

		// Activate the ones that are in use. We use ANY($2) to match a
		// PostgreSQL text array parameter.
		_, err = tx.ExecContext(ctx,
			`UPDATE cookbooks SET is_active = TRUE, updated_at = now()
			 WHERE organisation_id = $1 AND source = 'chef_server'
			   AND name = ANY($2)`,
			organisationID,
			activeNames,
		)
		if err != nil {
			return fmt.Errorf("datastore: activating cookbooks: %w", err)
		}

		return nil
	})
}

// MarkStaleCookbooksForOrg updates the is_stale_cookbook flag for all server
// cookbooks belonging to the given organisation. A cookbook is marked stale
// if its first_seen_at is before the cutoff time, and not stale otherwise.
// Returns the number of cookbooks marked as stale.
func (db *DB) MarkStaleCookbooksForOrg(ctx context.Context, organisationID string, cutoff time.Time) (int, error) {
	if organisationID == "" {
		return 0, fmt.Errorf("datastore: organisation ID is required to mark stale cookbooks")
	}

	var staleCount int
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		// Clear stale flag for all cookbooks in the org.
		if _, err := tx.ExecContext(ctx,
			`UPDATE cookbooks SET is_stale_cookbook = FALSE, updated_at = now()
			 WHERE organisation_id = $1 AND source = 'chef_server'`,
			organisationID,
		); err != nil {
			return fmt.Errorf("datastore: clearing stale cookbook flags: %w", err)
		}

		// Set stale flag where first_seen_at is before the cutoff.
		res, err := tx.ExecContext(ctx,
			`UPDATE cookbooks SET is_stale_cookbook = TRUE, updated_at = now()
			 WHERE organisation_id = $1 AND source = 'chef_server'
			   AND first_seen_at < $2`,
			organisationID, cutoff,
		)
		if err != nil {
			return fmt.Errorf("datastore: marking stale cookbooks: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("datastore: checking rows affected: %w", err)
		}
		staleCount = int(n)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return staleCount, nil
}

// ---------------------------------------------------------------------------
// Batch lookup
// ---------------------------------------------------------------------------

// GetServerCookbookIDMap returns a nested map of cookbook name → version → ID
// for all Chef-server-sourced cookbooks belonging to the given organisation.
// This is used during collection to efficiently resolve cookbook IDs when
// building cookbook-node usage records, avoiding N+1 queries.
func (db *DB) GetServerCookbookIDMap(ctx context.Context, organisationID string) (map[string]map[string]string, error) {
	return db.getServerCookbookIDMap(ctx, db.q(), organisationID)
}

func (db *DB) getServerCookbookIDMap(ctx context.Context, q queryable, organisationID string) (map[string]map[string]string, error) {
	const query = `
		SELECT id, name, version
		FROM cookbooks
		WHERE organisation_id = $1 AND source = 'chef_server'
	`

	rows, err := q.QueryContext(ctx, query, organisationID)
	if err != nil {
		return nil, fmt.Errorf("datastore: querying server cookbook ID map: %w", err)
	}
	defer rows.Close()

	result := make(map[string]map[string]string)
	for rows.Next() {
		var id, name, version string
		if err := rows.Scan(&id, &name, &version); err != nil {
			return nil, fmt.Errorf("datastore: scanning server cookbook ID map row: %w", err)
		}
		if result[name] == nil {
			result[name] = make(map[string]string)
		}
		result[name][version] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating server cookbook ID map rows: %w", err)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Query methods
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

// GetServerCookbook returns a Chef-server-sourced cookbook by organisation,
// name, and version. Returns ErrNotFound if no match exists.
func (db *DB) GetServerCookbook(ctx context.Context, organisationID, name, version string) (Cookbook, error) {
	return db.getServerCookbook(ctx, db.q(), organisationID, name, version)
}

func (db *DB) getServerCookbook(ctx context.Context, q queryable, organisationID, name, version string) (Cookbook, error) {
	const query = `
		SELECT id, organisation_id, name, version, source,
		       git_repo_url, head_commit_sha, default_branch,
		       has_test_suite, is_active, is_stale_cookbook,
		       download_status, download_error,
		       first_seen_at, last_fetched_at, created_at, updated_at
		FROM cookbooks
		WHERE organisation_id = $1 AND name = $2 AND version = $3
		  AND source = 'chef_server'
	`
	return scanCookbook(q.QueryRowContext(ctx, query, organisationID, name, version))
}

// GetGitCookbook returns a git-sourced cookbook by name and repo URL.
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

// ListCookbooksByOrganisation returns all cookbooks for the given
// organisation (Chef-server-sourced only), ordered by name then version.
func (db *DB) ListCookbooksByOrganisation(ctx context.Context, organisationID string) ([]Cookbook, error) {
	return db.listCookbooksByOrganisation(ctx, db.q(), organisationID)
}

func (db *DB) listCookbooksByOrganisation(ctx context.Context, q queryable, organisationID string) ([]Cookbook, error) {
	const query = `
		SELECT id, organisation_id, name, version, source,
		       git_repo_url, head_commit_sha, default_branch,
		       has_test_suite, is_active, is_stale_cookbook,
		       download_status, download_error,
		       first_seen_at, last_fetched_at, created_at, updated_at
		FROM cookbooks
		WHERE organisation_id = $1 AND source = 'chef_server'
		ORDER BY name, version
	`
	return scanCookbooks(q.QueryContext(ctx, query, organisationID))
}

// ListCookbooksByName returns all cookbook rows with the given name,
// regardless of source or organisation. Ordered by source, organisation_id,
// version.
func (db *DB) ListCookbooksByName(ctx context.Context, name string) ([]Cookbook, error) {
	return db.listCookbooksByName(ctx, db.q(), name)
}

func (db *DB) listCookbooksByName(ctx context.Context, q queryable, name string) ([]Cookbook, error) {
	const query = `
		SELECT id, organisation_id, name, version, source,
		       git_repo_url, head_commit_sha, default_branch,
		       has_test_suite, is_active, is_stale_cookbook,
		       download_status, download_error,
		       first_seen_at, last_fetched_at, created_at, updated_at
		FROM cookbooks
		WHERE name = $1
		ORDER BY source, organisation_id, version
	`
	return scanCookbooks(q.QueryContext(ctx, query, name))
}

// ListGitCookbooks returns all git-sourced cookbooks, ordered by name.
func (db *DB) ListGitCookbooks(ctx context.Context) ([]Cookbook, error) {
	return db.listGitCookbooks(ctx, db.q())
}

func (db *DB) listGitCookbooks(ctx context.Context, q queryable) ([]Cookbook, error) {
	const query = `
		SELECT id, organisation_id, name, version, source,
		       git_repo_url, head_commit_sha, default_branch,
		       has_test_suite, is_active, is_stale_cookbook,
		       download_status, download_error,
		       first_seen_at, last_fetched_at, created_at, updated_at
		FROM cookbooks
		WHERE source = 'git'
		ORDER BY name
	`
	return scanCookbooks(q.QueryContext(ctx, query))
}

// ListActiveCookbooksByOrganisation returns only active cookbooks for the
// given organisation, ordered by name then version.
func (db *DB) ListActiveCookbooksByOrganisation(ctx context.Context, organisationID string) ([]Cookbook, error) {
	return db.listActiveCookbooksByOrganisation(ctx, db.q(), organisationID)
}

func (db *DB) listActiveCookbooksByOrganisation(ctx context.Context, q queryable, organisationID string) ([]Cookbook, error) {
	const query = `
		SELECT id, organisation_id, name, version, source,
		       git_repo_url, head_commit_sha, default_branch,
		       has_test_suite, is_active, is_stale_cookbook,
		       download_status, download_error,
		       first_seen_at, last_fetched_at, created_at, updated_at
		FROM cookbooks
		WHERE organisation_id = $1 AND source = 'chef_server' AND is_active = TRUE
		ORDER BY name, version
	`
	return scanCookbooks(q.QueryContext(ctx, query, organisationID))
}

// ListStaleCookbooksByOrganisation returns cookbooks flagged as stale for
// the given organisation, ordered by name then version.
func (db *DB) ListStaleCookbooksByOrganisation(ctx context.Context, organisationID string) ([]Cookbook, error) {
	return db.listStaleCookbooksByOrganisation(ctx, db.q(), organisationID)
}

func (db *DB) listStaleCookbooksByOrganisation(ctx context.Context, q queryable, organisationID string) ([]Cookbook, error) {
	const query = `
		SELECT id, organisation_id, name, version, source,
		       git_repo_url, head_commit_sha, default_branch,
		       has_test_suite, is_active, is_stale_cookbook,
		       download_status, download_error,
		       first_seen_at, last_fetched_at, created_at, updated_at
		FROM cookbooks
		WHERE organisation_id = $1 AND source = 'chef_server' AND is_stale_cookbook = TRUE
		ORDER BY name, version
	`
	return scanCookbooks(q.QueryContext(ctx, query, organisationID))
}

// ServerCookbookExists checks whether a Chef-server-sourced cookbook with
// the given organisation, name, and version already exists in the database.
// This is used by the collection process to skip downloading cookbook
// versions that are already stored (immutability optimisation).
func (db *DB) ServerCookbookExists(ctx context.Context, organisationID, name, version string) (bool, error) {
	return db.serverCookbookExists(ctx, db.q(), organisationID, name, version)
}

func (db *DB) serverCookbookExists(ctx context.Context, q queryable, organisationID, name, version string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM cookbooks
			WHERE organisation_id = $1 AND name = $2 AND version = $3
			  AND source = 'chef_server'
			  AND download_status = 'ok'
		)`,
		organisationID, name, version,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("datastore: checking cookbook existence: %w", err)
	}
	return exists, nil
}

// DeleteCookbook removes the cookbook with the given UUID. Returns
// ErrNotFound if no such cookbook exists. Cascading deletes will remove
// associated test results, cookstyle results, complexity records, and
// usage records.
func (db *DB) DeleteCookbook(ctx context.Context, id string) error {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM cookbooks WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("datastore: deleting cookbook: %w", err)
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

// ---------------------------------------------------------------------------
// Download status management
// ---------------------------------------------------------------------------

// UpdateCookbookDownloadStatusParams holds the fields required to update a
// cookbook's download status.
type UpdateCookbookDownloadStatusParams struct {
	ID             string
	DownloadStatus string // "ok", "failed", or "pending"
	DownloadError  string // Error detail (only set when status = "failed")
}

// UpdateCookbookDownloadStatus updates the download_status and download_error
// for a single cookbook. Returns the updated cookbook. Returns ErrNotFound if
// no such cookbook exists.
func (db *DB) UpdateCookbookDownloadStatus(ctx context.Context, p UpdateCookbookDownloadStatusParams) (Cookbook, error) {
	return db.updateCookbookDownloadStatus(ctx, db.q(), p)
}

func (db *DB) updateCookbookDownloadStatus(ctx context.Context, q queryable, p UpdateCookbookDownloadStatusParams) (Cookbook, error) {
	if p.ID == "" {
		return Cookbook{}, fmt.Errorf("datastore: cookbook ID is required to update download status")
	}
	if p.DownloadStatus != DownloadStatusOK && p.DownloadStatus != DownloadStatusFailed && p.DownloadStatus != DownloadStatusPending {
		return Cookbook{}, fmt.Errorf("datastore: invalid download status: %q", p.DownloadStatus)
	}

	// Clear download_error when status is not 'failed'.
	var dlError sql.NullString
	if p.DownloadStatus == DownloadStatusFailed && p.DownloadError != "" {
		dlError = sql.NullString{String: p.DownloadError, Valid: true}
	}

	const query = `
		UPDATE cookbooks
		SET download_status = $2,
		    download_error  = $3,
		    updated_at      = now()
		WHERE id = $1
		RETURNING id, organisation_id, name, version, source,
		          git_repo_url, head_commit_sha, default_branch,
		          has_test_suite, is_active, is_stale_cookbook,
		          download_status, download_error,
		          first_seen_at, last_fetched_at, created_at, updated_at
	`

	return scanCookbook(q.QueryRowContext(ctx, query, p.ID, p.DownloadStatus, dlError))
}

// MarkCookbookDownloadOK is a convenience wrapper that marks a cookbook as
// successfully downloaded.
func (db *DB) MarkCookbookDownloadOK(ctx context.Context, id string) (Cookbook, error) {
	return db.UpdateCookbookDownloadStatus(ctx, UpdateCookbookDownloadStatusParams{
		ID:             id,
		DownloadStatus: DownloadStatusOK,
	})
}

// MarkCookbookDownloadFailed is a convenience wrapper that marks a cookbook
// download as failed with the given error detail.
func (db *DB) MarkCookbookDownloadFailed(ctx context.Context, id, downloadError string) (Cookbook, error) {
	return db.UpdateCookbookDownloadStatus(ctx, UpdateCookbookDownloadStatusParams{
		ID:             id,
		DownloadStatus: DownloadStatusFailed,
		DownloadError:  downloadError,
	})
}

// ListCookbooksNeedingDownload returns all Chef-server-sourced cookbooks for
// the given organisation that have a download_status of 'pending' or 'failed'.
// These are the cookbook versions that should be (re-)downloaded on the next
// collection run. Results are ordered by name then version.
func (db *DB) ListCookbooksNeedingDownload(ctx context.Context, organisationID string) ([]Cookbook, error) {
	return db.listCookbooksNeedingDownload(ctx, db.q(), organisationID)
}

func (db *DB) listCookbooksNeedingDownload(ctx context.Context, q queryable, organisationID string) ([]Cookbook, error) {
	const query = `
		SELECT id, organisation_id, name, version, source,
		       git_repo_url, head_commit_sha, default_branch,
		       has_test_suite, is_active, is_stale_cookbook,
		       download_status, download_error,
		       first_seen_at, last_fetched_at, created_at, updated_at
		FROM cookbooks
		WHERE organisation_id = $1
		  AND source = 'chef_server'
		  AND download_status IN ('pending', 'failed')
		ORDER BY name, version
	`
	return scanCookbooks(q.QueryContext(ctx, query, organisationID))
}

// ListActiveCookbooksNeedingDownload returns active Chef-server-sourced
// cookbooks for the given organisation that need downloading. Only active
// cookbooks (applied to at least one node) are returned, as per the spec:
// unused cookbooks are flagged but do not need to be fetched for analysis.
func (db *DB) ListActiveCookbooksNeedingDownload(ctx context.Context, organisationID string) ([]Cookbook, error) {
	return db.listActiveCookbooksNeedingDownload(ctx, db.q(), organisationID)
}

func (db *DB) listActiveCookbooksNeedingDownload(ctx context.Context, q queryable, organisationID string) ([]Cookbook, error) {
	const query = `
		SELECT id, organisation_id, name, version, source,
		       git_repo_url, head_commit_sha, default_branch,
		       has_test_suite, is_active, is_stale_cookbook,
		       download_status, download_error,
		       first_seen_at, last_fetched_at, created_at, updated_at
		FROM cookbooks
		WHERE organisation_id = $1
		  AND source = 'chef_server'
		  AND is_active = TRUE
		  AND download_status IN ('pending', 'failed')
		ORDER BY name, version
	`
	return scanCookbooks(q.QueryContext(ctx, query, organisationID))
}

// ResetCookbookDownloadStatus resets the download_status to 'pending' and
// clears the download_error for a specific cookbook. This is the "manual
// rescan" operation that forces a fresh download attempt on the next run.
// Returns ErrNotFound if no such cookbook exists.
func (db *DB) ResetCookbookDownloadStatus(ctx context.Context, id string) (Cookbook, error) {
	return db.UpdateCookbookDownloadStatus(ctx, UpdateCookbookDownloadStatusParams{
		ID:             id,
		DownloadStatus: DownloadStatusPending,
	})
}
