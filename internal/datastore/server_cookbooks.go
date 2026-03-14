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

// ServerCookbook represents a row in the server_cookbooks table. Each row is
// a single cookbook name+version pair fetched from a Chef Infra Server,
// scoped to an organisation.
type ServerCookbook struct {
	ID              string          `json:"id"`
	OrganisationID  string          `json:"organisation_id"`
	Name            string          `json:"name"`
	Version         string          `json:"version"`
	IsActive        bool            `json:"is_active"`
	IsStaleCookbook bool            `json:"is_stale_cookbook"`
	IsFrozen        bool            `json:"is_frozen"`
	DownloadStatus  string          `json:"download_status"`
	DownloadError   string          `json:"download_error,omitempty"`
	Maintainer      string          `json:"maintainer,omitempty"`
	Description     string          `json:"description,omitempty"`
	LongDescription string          `json:"long_description,omitempty"`
	License         string          `json:"license,omitempty"`
	Platforms       json.RawMessage `json:"platforms,omitempty"`
	Dependencies    json.RawMessage `json:"dependencies,omitempty"`
	FirstSeenAt     time.Time       `json:"first_seen_at,omitempty"`
	LastFetchedAt   time.Time       `json:"last_fetched_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// IsDownloaded returns true if the server cookbook content has been
// successfully fetched (download_status = 'ok').
func (sc ServerCookbook) IsDownloaded() bool {
	return sc.DownloadStatus == DownloadStatusOK
}

// NeedsDownload returns true if the server cookbook has a pending or failed
// download status and should be (re-)downloaded on the next collection run.
func (sc ServerCookbook) NeedsDownload() bool {
	return sc.DownloadStatus == DownloadStatusPending || sc.DownloadStatus == DownloadStatusFailed
}

// MarshalJSON implements json.Marshaler for ServerCookbook.
func (sc ServerCookbook) MarshalJSON() ([]byte, error) {
	type Alias ServerCookbook
	return json.Marshal((Alias)(sc))
}

// serverCookbookColumns is the column list used by all SELECT queries
// against server_cookbooks, kept in one place for consistency.
const serverCookbookColumns = `
	id, organisation_id, name, version,
	is_active, is_stale_cookbook, is_frozen,
	download_status, download_error,
	maintainer, description, long_description, license,
	platforms, dependencies,
	first_seen_at, last_fetched_at, created_at, updated_at
`

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertServerCookbookParams holds the fields required to upsert a server
// cookbook. The upsert key is (organisation_id, name, version).
type UpsertServerCookbookParams struct {
	OrganisationID  string
	Name            string
	Version         string
	IsActive        bool
	IsStaleCookbook bool
	FirstSeenAt     time.Time // set on first insert, not overwritten on update
	LastFetchedAt   time.Time
}

// UpsertServerCookbook inserts or updates a server cookbook row.
func (db *DB) UpsertServerCookbook(ctx context.Context, p UpsertServerCookbookParams) (ServerCookbook, error) {
	return db.upsertServerCookbook(ctx, db.q(), p)
}

func (db *DB) upsertServerCookbook(ctx context.Context, q queryable, p UpsertServerCookbookParams) (ServerCookbook, error) {
	if p.OrganisationID == "" {
		return ServerCookbook{}, fmt.Errorf("datastore: organisation ID is required for server cookbook")
	}
	if p.Name == "" {
		return ServerCookbook{}, fmt.Errorf("datastore: cookbook name is required")
	}
	if p.Version == "" {
		return ServerCookbook{}, fmt.Errorf("datastore: cookbook version is required for server cookbook")
	}
	if p.LastFetchedAt.IsZero() {
		p.LastFetchedAt = time.Now().UTC()
	}
	if p.FirstSeenAt.IsZero() {
		p.FirstSeenAt = time.Now().UTC()
	}

	const query = `
		INSERT INTO server_cookbooks (
			organisation_id, name, version,
			is_active, is_stale_cookbook,
			first_seen_at, last_fetched_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
		ON CONFLICT (organisation_id, name, version)
		DO UPDATE SET
			is_active         = EXCLUDED.is_active,
			is_stale_cookbook  = EXCLUDED.is_stale_cookbook,
			last_fetched_at   = EXCLUDED.last_fetched_at,
			updated_at        = now()
		RETURNING ` + serverCookbookColumns

	return scanServerCookbook(q.QueryRowContext(ctx, query,
		p.OrganisationID,
		p.Name,
		p.Version,
		p.IsActive,
		p.IsStaleCookbook,
		p.FirstSeenAt,
		p.LastFetchedAt,
	))
}

// BulkUpsertServerCookbooks upserts multiple server cookbooks within a
// single transaction for efficiency. Returns the count of rows upserted.
// If any upsert fails, the entire batch is rolled back.
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
// Mark active/stale
// ---------------------------------------------------------------------------

// MarkServerCookbooksActiveForOrg sets is_active = true for the named
// cookbooks (by name) within the given organisation, and is_active = false
// for all others. Called after a collection run to reflect which cookbooks
// are actually in use by at least one node.
func (db *DB) MarkServerCookbooksActiveForOrg(ctx context.Context, organisationID string, activeNames []string) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		// Deactivate all server cookbooks for this org.
		_, err := tx.ExecContext(ctx,
			`UPDATE server_cookbooks SET is_active = FALSE, updated_at = now()
			 WHERE organisation_id = $1`,
			organisationID,
		)
		if err != nil {
			return fmt.Errorf("datastore: deactivating server cookbooks: %w", err)
		}

		if len(activeNames) == 0 {
			return nil
		}

		// Activate the ones that are in use.
		_, err = tx.ExecContext(ctx,
			`UPDATE server_cookbooks SET is_active = TRUE, updated_at = now()
			 WHERE organisation_id = $1 AND name = ANY($2)`,
			organisationID,
			stringSliceToArray(activeNames),
		)
		if err != nil {
			return fmt.Errorf("datastore: activating server cookbooks: %w", err)
		}

		return nil
	})
}

// MarkStaleServerCookbooksForOrg updates the is_stale_cookbook flag for all
// server cookbooks belonging to the given organisation. A cookbook is marked
// stale if its first_seen_at is before the cutoff time. Returns the number
// of cookbooks marked as stale.
func (db *DB) MarkStaleServerCookbooksForOrg(ctx context.Context, organisationID string, cutoff time.Time) (int, error) {
	if organisationID == "" {
		return 0, fmt.Errorf("datastore: organisation ID is required to mark stale server cookbooks")
	}

	var staleCount int
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		// Clear stale flag for all cookbooks in the org.
		if _, err := tx.ExecContext(ctx,
			`UPDATE server_cookbooks SET is_stale_cookbook = FALSE, updated_at = now()
			 WHERE organisation_id = $1`,
			organisationID,
		); err != nil {
			return fmt.Errorf("datastore: clearing stale server cookbook flags: %w", err)
		}

		// Set stale flag where first_seen_at is before the cutoff.
		res, err := tx.ExecContext(ctx,
			`UPDATE server_cookbooks SET is_stale_cookbook = TRUE, updated_at = now()
			 WHERE organisation_id = $1 AND first_seen_at < $2`,
			organisationID, cutoff,
		)
		if err != nil {
			return fmt.Errorf("datastore: marking stale server cookbooks: %w", err)
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
// for all server cookbooks belonging to the given organisation. This is used
// during collection to efficiently resolve cookbook IDs when building
// cookbook-node usage records, avoiding N+1 queries.
func (db *DB) GetServerCookbookIDMap(ctx context.Context, organisationID string) (map[string]map[string]string, error) {
	return db.getServerCookbookIDMap(ctx, db.q(), organisationID)
}

func (db *DB) getServerCookbookIDMap(ctx context.Context, q queryable, organisationID string) (map[string]map[string]string, error) {
	const query = `
		SELECT id, name, version
		FROM server_cookbooks
		WHERE organisation_id = $1
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

// GetServerCookbook returns a server cookbook by UUID. Returns ErrNotFound
// if no such server cookbook exists.
func (db *DB) GetServerCookbook(ctx context.Context, id string) (ServerCookbook, error) {
	return db.getServerCookbook(ctx, db.q(), id)
}

func (db *DB) getServerCookbook(ctx context.Context, q queryable, id string) (ServerCookbook, error) {
	query := `SELECT ` + serverCookbookColumns + ` FROM server_cookbooks WHERE id = $1`
	return scanServerCookbook(q.QueryRowContext(ctx, query, id))
}

// GetServerCookbookByKey returns a server cookbook by its natural key
// (organisation_id, name, version). Returns ErrNotFound if no match exists.
func (db *DB) GetServerCookbookByKey(ctx context.Context, organisationID, name, version string) (ServerCookbook, error) {
	return db.getServerCookbookByKey(ctx, db.q(), organisationID, name, version)
}

func (db *DB) getServerCookbookByKey(ctx context.Context, q queryable, organisationID, name, version string) (ServerCookbook, error) {
	query := `SELECT ` + serverCookbookColumns + `
		FROM server_cookbooks
		WHERE organisation_id = $1 AND name = $2 AND version = $3`
	return scanServerCookbook(q.QueryRowContext(ctx, query, organisationID, name, version))
}

// ListServerCookbooksByOrganisation returns all server cookbooks for the
// given organisation, ordered by name then version.
func (db *DB) ListServerCookbooksByOrganisation(ctx context.Context, organisationID string) ([]ServerCookbook, error) {
	return db.listServerCookbooksByOrganisation(ctx, db.q(), organisationID)
}

func (db *DB) listServerCookbooksByOrganisation(ctx context.Context, q queryable, organisationID string) ([]ServerCookbook, error) {
	query := `SELECT ` + serverCookbookColumns + `
		FROM server_cookbooks
		WHERE organisation_id = $1
		ORDER BY name, version`
	return scanServerCookbooks(q.QueryContext(ctx, query, organisationID))
}

// ListServerCookbooksByName returns all server cookbook rows with the given
// name across all organisations, ordered by organisation_id then version.
func (db *DB) ListServerCookbooksByName(ctx context.Context, name string) ([]ServerCookbook, error) {
	return db.listServerCookbooksByName(ctx, db.q(), name)
}

func (db *DB) listServerCookbooksByName(ctx context.Context, q queryable, name string) ([]ServerCookbook, error) {
	query := `SELECT ` + serverCookbookColumns + `
		FROM server_cookbooks
		WHERE name = $1
		ORDER BY organisation_id, version`
	return scanServerCookbooks(q.QueryContext(ctx, query, name))
}

// ListActiveServerCookbooksByOrganisation returns only active server
// cookbooks for the given organisation, ordered by name then version.
func (db *DB) ListActiveServerCookbooksByOrganisation(ctx context.Context, organisationID string) ([]ServerCookbook, error) {
	return db.listActiveServerCookbooksByOrganisation(ctx, db.q(), organisationID)
}

func (db *DB) listActiveServerCookbooksByOrganisation(ctx context.Context, q queryable, organisationID string) ([]ServerCookbook, error) {
	query := `SELECT ` + serverCookbookColumns + `
		FROM server_cookbooks
		WHERE organisation_id = $1 AND is_active = TRUE
		ORDER BY name, version`
	return scanServerCookbooks(q.QueryContext(ctx, query, organisationID))
}

// ListStaleServerCookbooksByOrganisation returns server cookbooks flagged
// as stale for the given organisation, ordered by name then version.
func (db *DB) ListStaleServerCookbooksByOrganisation(ctx context.Context, organisationID string) ([]ServerCookbook, error) {
	return db.listStaleServerCookbooksByOrganisation(ctx, db.q(), organisationID)
}

func (db *DB) listStaleServerCookbooksByOrganisation(ctx context.Context, q queryable, organisationID string) ([]ServerCookbook, error) {
	query := `SELECT ` + serverCookbookColumns + `
		FROM server_cookbooks
		WHERE organisation_id = $1 AND is_stale_cookbook = TRUE
		ORDER BY name, version`
	return scanServerCookbooks(q.QueryContext(ctx, query, organisationID))
}

// ServerCookbookExists checks whether a server cookbook with the given
// organisation, name, and version already exists with download_status = 'ok'.
// Used by the collection process to skip downloading immutable cookbook
// versions that are already stored.
func (db *DB) ServerCookbookExists(ctx context.Context, organisationID, name, version string) (bool, error) {
	return db.serverCookbookExists(ctx, db.q(), organisationID, name, version)
}

func (db *DB) serverCookbookExists(ctx context.Context, q queryable, organisationID, name, version string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM server_cookbooks
			WHERE organisation_id = $1 AND name = $2 AND version = $3
			  AND download_status = 'ok'
		)`,
		organisationID, name, version,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("datastore: checking server cookbook existence: %w", err)
	}
	return exists, nil
}

// ---------------------------------------------------------------------------
// Download status management
// ---------------------------------------------------------------------------

// UpdateServerCookbookDownloadStatusParams holds the fields required to
// update a server cookbook's download status.
type UpdateServerCookbookDownloadStatusParams struct {
	ID             string
	DownloadStatus string // "ok", "failed", or "pending"
	DownloadError  string // Error detail (only set when status = "failed")
}

// UpdateServerCookbookDownloadStatus updates the download_status and
// download_error for a single server cookbook. Returns the updated row.
// Returns ErrNotFound if no such server cookbook exists.
func (db *DB) UpdateServerCookbookDownloadStatus(ctx context.Context, p UpdateServerCookbookDownloadStatusParams) (ServerCookbook, error) {
	return db.updateServerCookbookDownloadStatus(ctx, db.q(), p)
}

func (db *DB) updateServerCookbookDownloadStatus(ctx context.Context, q queryable, p UpdateServerCookbookDownloadStatusParams) (ServerCookbook, error) {
	if p.ID == "" {
		return ServerCookbook{}, fmt.Errorf("datastore: server cookbook ID is required to update download status")
	}
	if p.DownloadStatus != DownloadStatusOK && p.DownloadStatus != DownloadStatusFailed && p.DownloadStatus != DownloadStatusPending {
		return ServerCookbook{}, fmt.Errorf("datastore: invalid download status: %q", p.DownloadStatus)
	}

	// Clear download_error when status is not 'failed'.
	var dlError sql.NullString
	if p.DownloadStatus == DownloadStatusFailed && p.DownloadError != "" {
		dlError = sql.NullString{String: p.DownloadError, Valid: true}
	}

	query := `
		UPDATE server_cookbooks
		SET download_status = $2,
		    download_error  = $3,
		    updated_at      = now()
		WHERE id = $1
		RETURNING ` + serverCookbookColumns

	return scanServerCookbook(q.QueryRowContext(ctx, query, p.ID, p.DownloadStatus, dlError))
}

// MarkServerCookbookDownloadOK is a convenience wrapper that marks a server
// cookbook as successfully downloaded.
func (db *DB) MarkServerCookbookDownloadOK(ctx context.Context, id string) (ServerCookbook, error) {
	return db.UpdateServerCookbookDownloadStatus(ctx, UpdateServerCookbookDownloadStatusParams{
		ID:             id,
		DownloadStatus: DownloadStatusOK,
	})
}

// MarkServerCookbookDownloadFailed is a convenience wrapper that marks a
// server cookbook download as failed with the given error detail.
func (db *DB) MarkServerCookbookDownloadFailed(ctx context.Context, id, downloadError string) (ServerCookbook, error) {
	return db.UpdateServerCookbookDownloadStatus(ctx, UpdateServerCookbookDownloadStatusParams{
		ID:             id,
		DownloadStatus: DownloadStatusFailed,
		DownloadError:  downloadError,
	})
}

// ListServerCookbooksNeedingDownload returns all server cookbooks for the
// given organisation that have a download_status of 'pending' or 'failed'.
// These are cookbook versions that should be (re-)downloaded on the next
// collection run. Results are ordered by name then version.
func (db *DB) ListServerCookbooksNeedingDownload(ctx context.Context, organisationID string) ([]ServerCookbook, error) {
	return db.listServerCookbooksNeedingDownload(ctx, db.q(), organisationID)
}

func (db *DB) listServerCookbooksNeedingDownload(ctx context.Context, q queryable, organisationID string) ([]ServerCookbook, error) {
	query := `SELECT ` + serverCookbookColumns + `
		FROM server_cookbooks
		WHERE organisation_id = $1
		  AND download_status IN ('pending', 'failed')
		ORDER BY name, version`
	return scanServerCookbooks(q.QueryContext(ctx, query, organisationID))
}

// ListActiveServerCookbooksNeedingDownload returns active server cookbooks
// for the given organisation that need downloading. Only active cookbooks
// (applied to at least one node) are returned — unused cookbooks are flagged
// but do not need to be fetched for analysis.
func (db *DB) ListActiveServerCookbooksNeedingDownload(ctx context.Context, organisationID string) ([]ServerCookbook, error) {
	return db.listActiveServerCookbooksNeedingDownload(ctx, db.q(), organisationID)
}

func (db *DB) listActiveServerCookbooksNeedingDownload(ctx context.Context, q queryable, organisationID string) ([]ServerCookbook, error) {
	query := `SELECT ` + serverCookbookColumns + `
		FROM server_cookbooks
		WHERE organisation_id = $1
		  AND is_active = TRUE
		  AND download_status IN ('pending', 'failed')
		ORDER BY name, version`
	return scanServerCookbooks(q.QueryContext(ctx, query, organisationID))
}

// ResetServerCookbookDownloadStatus resets the download_status to 'pending'
// and clears the download_error for a specific server cookbook. This is the
// "manual rescan" operation that forces a fresh download on the next run.
func (db *DB) ResetServerCookbookDownloadStatus(ctx context.Context, id string) (ServerCookbook, error) {
	return db.UpdateServerCookbookDownloadStatus(ctx, UpdateServerCookbookDownloadStatusParams{
		ID:             id,
		DownloadStatus: DownloadStatusPending,
	})
}

// ResetAllServerCookbookDownloadStatuses resets download_status to 'pending'
// and clears download_error for ALL server cookbooks that currently have
// status 'ok'. Used by the admin "rescan all" endpoint. Returns the number
// of rows updated.
func (db *DB) ResetAllServerCookbookDownloadStatuses(ctx context.Context) (int, error) {
	const query = `
		UPDATE server_cookbooks
		   SET download_status = 'pending',
		       download_error  = NULL,
		       updated_at      = now()
		 WHERE download_status = 'ok'
	`
	result, err := db.pool.ExecContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("datastore: resetting all server cookbook download statuses: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// ---------------------------------------------------------------------------
// Metadata update
// ---------------------------------------------------------------------------

// UpdateServerCookbookMetadataParams holds the metadata fields to populate
// on a server cookbook after its manifest has been fetched from the Chef API.
type UpdateServerCookbookMetadataParams struct {
	ID              string
	IsFrozen        bool
	Maintainer      string
	Description     string
	LongDescription string
	License         string
	Platforms       json.RawMessage // JSON object, e.g. {"ubuntu": ">= 18.04"}
	Dependencies    json.RawMessage // JSON object, e.g. {"apt": ">= 0.0.0"}
}

// UpdateServerCookbookMetadata populates the metadata fields on a server
// cookbook row. This is called from the streaming pipeline after the cookbook
// version manifest is fetched (Step 7b). Returns the updated row. Returns
// ErrNotFound if no such server cookbook exists.
func (db *DB) UpdateServerCookbookMetadata(ctx context.Context, p UpdateServerCookbookMetadataParams) (ServerCookbook, error) {
	return db.updateServerCookbookMetadata(ctx, db.q(), p)
}

func (db *DB) updateServerCookbookMetadata(ctx context.Context, q queryable, p UpdateServerCookbookMetadataParams) (ServerCookbook, error) {
	if p.ID == "" {
		return ServerCookbook{}, fmt.Errorf("datastore: server cookbook ID is required to update metadata")
	}

	// Normalise nil JSON to null for database storage.
	platforms := nullJSON(p.Platforms)
	dependencies := nullJSON(p.Dependencies)

	query := `
		UPDATE server_cookbooks
		SET is_frozen         = $2,
		    maintainer        = $3,
		    description       = $4,
		    long_description  = $5,
		    license           = $6,
		    platforms         = $7,
		    dependencies      = $8,
		    updated_at        = now()
		WHERE id = $1
		RETURNING ` + serverCookbookColumns

	return scanServerCookbook(q.QueryRowContext(ctx, query,
		p.ID,
		p.IsFrozen,
		nullString(p.Maintainer),
		nullString(p.Description),
		nullString(p.LongDescription),
		nullString(p.License),
		platforms,
		dependencies,
	))
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteServerCookbook removes the server cookbook with the given UUID.
// Returns ErrNotFound if no such server cookbook exists. Cascading deletes
// will remove associated cookstyle results, autocorrect previews, complexity
// records, and node usage records.
func (db *DB) DeleteServerCookbook(ctx context.Context, id string) error {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM server_cookbooks WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook: %w", err)
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

func scanServerCookbook(row *sql.Row) (ServerCookbook, error) {
	var sc ServerCookbook
	var dlError, maintainer, description, longDesc, license sql.NullString
	var platforms, dependencies []byte
	var firstSeen, lastFetched sql.NullTime

	err := row.Scan(
		&sc.ID,
		&sc.OrganisationID,
		&sc.Name,
		&sc.Version,
		&sc.IsActive,
		&sc.IsStaleCookbook,
		&sc.IsFrozen,
		&sc.DownloadStatus,
		&dlError,
		&maintainer,
		&description,
		&longDesc,
		&license,
		&platforms,
		&dependencies,
		&firstSeen,
		&lastFetched,
		&sc.CreatedAt,
		&sc.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return ServerCookbook{}, ErrNotFound
		}
		return ServerCookbook{}, fmt.Errorf("datastore: scanning server cookbook: %w", err)
	}

	sc.DownloadError = stringFromNull(dlError)
	sc.Maintainer = stringFromNull(maintainer)
	sc.Description = stringFromNull(description)
	sc.LongDescription = stringFromNull(longDesc)
	sc.License = stringFromNull(license)
	sc.Platforms = normaliseJSON(platforms)
	sc.Dependencies = normaliseJSON(dependencies)
	sc.FirstSeenAt = timeFromNull(firstSeen)
	sc.LastFetchedAt = timeFromNull(lastFetched)
	return sc, nil
}

func scanServerCookbooks(rows *sql.Rows, err error) ([]ServerCookbook, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying server cookbooks: %w", err)
	}
	defer rows.Close()

	var cookbooks []ServerCookbook
	for rows.Next() {
		var sc ServerCookbook
		var dlError, maintainer, description, longDesc, license sql.NullString
		var platforms, dependencies []byte
		var firstSeen, lastFetched sql.NullTime

		if err := rows.Scan(
			&sc.ID,
			&sc.OrganisationID,
			&sc.Name,
			&sc.Version,
			&sc.IsActive,
			&sc.IsStaleCookbook,
			&sc.IsFrozen,
			&sc.DownloadStatus,
			&dlError,
			&maintainer,
			&description,
			&longDesc,
			&license,
			&platforms,
			&dependencies,
			&firstSeen,
			&lastFetched,
			&sc.CreatedAt,
			&sc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning server cookbook row: %w", err)
		}

		sc.DownloadError = stringFromNull(dlError)
		sc.Maintainer = stringFromNull(maintainer)
		sc.Description = stringFromNull(description)
		sc.LongDescription = stringFromNull(longDesc)
		sc.License = stringFromNull(license)
		sc.Platforms = normaliseJSON(platforms)
		sc.Dependencies = normaliseJSON(dependencies)
		sc.FirstSeenAt = timeFromNull(firstSeen)
		sc.LastFetchedAt = timeFromNull(lastFetched)
		cookbooks = append(cookbooks, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating server cookbook rows: %w", err)
	}
	return cookbooks, nil
}
