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

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// KitchenAnalysisResult represents a row in the kitchen_analysis_results table.
type KitchenAnalysisResult struct {
	GitRepoName        string          `json:"git_repo_name"`
	GitRepoURL         string          `json:"git_repo_url"`
	AnalysedAt         time.Time       `json:"analysed_at"`
	HeadCommitSHA      string          `json:"head_commit_sha"`
	KitchenFiles       json.RawMessage `json:"kitchen_files"`
	HasLocalOverride   bool            `json:"has_local_override"`
	LocalOverrideKeys  json.RawMessage `json:"local_override_keys,omitempty"`
	DriverName         string          `json:"driver_name,omitempty"`
	ProvisionerName    string          `json:"provisioner_name,omitempty"`
	RequireChefOmnibus *bool           `json:"require_chef_omnibus,omitempty"`
	Platforms          json.RawMessage `json:"platforms"`
	Suites             json.RawMessage `json:"suites"`
	TransportType      string          `json:"transport_type,omitempty"`
	Extensions         json.RawMessage `json:"extensions,omitempty"`
	VariantFiles       json.RawMessage `json:"variant_files,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// KitchenDiscoveredPlatform represents a row in the kitchen_discovered_platforms table.
type KitchenDiscoveredPlatform struct {
	PlatformName     string          `json:"platform_name"`
	NormalisedName   string          `json:"normalised_name"`
	OSFamily         string          `json:"os_family"`
	OSVersion        string          `json:"os_version,omitempty"`
	CookbookCount    int             `json:"cookbook_count"`
	HasExtensions    bool            `json:"has_extensions"`
	CommonExtensions json.RawMessage `json:"common_extensions,omitempty"`
	TransportType    string          `json:"transport_type,omitempty"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// UpsertKitchenAnalysisResultParams contains the fields needed to insert or
// update a kitchen_analysis_results row. The unique constraint is
// (git_repo_name, git_repo_url).
type UpsertKitchenAnalysisResultParams struct {
	GitRepoName        string
	GitRepoURL         string
	AnalysedAt         time.Time
	HeadCommitSHA      string
	KitchenFiles       json.RawMessage
	HasLocalOverride   bool
	LocalOverrideKeys  json.RawMessage
	DriverName         string
	ProvisionerName    string
	RequireChefOmnibus *bool
	Platforms          json.RawMessage
	Suites             json.RawMessage
	TransportType      string
	Extensions         json.RawMessage
	VariantFiles       json.RawMessage
	ErrorMessage       string
}

// KitchenAnalysisSummary holds aggregate statistics across all analysed repos.
type KitchenAnalysisSummary struct {
	TotalScanned           int            `json:"total_scanned"`
	TotalWithoutKitchen    int            `json:"total_without_kitchen"`
	TotalWithLocalOverride int            `json:"total_with_local_override"`
	TotalWithConflicts     int            `json:"total_with_conflicts"`
	DriverCounts           map[string]int `json:"driver_counts"`
	TransportCounts        map[string]int `json:"transport_counts"`
	ProvisionerCounts      map[string]int `json:"provisioner_counts"`
	PlatformCount          int            `json:"platform_count"`
}

// ---------------------------------------------------------------------------
// Column lists
// ---------------------------------------------------------------------------

const karColumns = `git_repo_name, git_repo_url, analysed_at, head_commit_sha,
       kitchen_files, has_local_override, local_override_keys,
       driver_name, provisioner_name, require_chef_omnibus,
       platforms, suites, transport_type,
       extensions, variant_files, error_message,
       created_at, updated_at`

const kdpColumns = `platform_name, normalised_name, os_family, os_version,
       cookbook_count, has_extensions, common_extensions, transport_type,
       updated_at`

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func validateKitchenAnalysisParams(p UpsertKitchenAnalysisResultParams) error {
	if p.GitRepoName == "" {
		return fmt.Errorf("datastore: git_repo_name is required")
	}
	if p.GitRepoURL == "" {
		return fmt.Errorf("datastore: git_repo_url is required")
	}
	if p.HeadCommitSHA == "" {
		return fmt.Errorf("datastore: head_commit_sha is required")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertKitchenAnalysisResult inserts a new kitchen analysis result or updates
// the existing one for the same (git_repo_name, git_repo_url). Returns the
// resulting row.
func (db *DB) UpsertKitchenAnalysisResult(ctx context.Context, p UpsertKitchenAnalysisResultParams) (*KitchenAnalysisResult, error) {
	return db.upsertKitchenAnalysisResult(ctx, db.q(), p)
}

func (db *DB) upsertKitchenAnalysisResult(ctx context.Context, q queryable, p UpsertKitchenAnalysisResultParams) (*KitchenAnalysisResult, error) {
	if err := validateKitchenAnalysisParams(p); err != nil {
		return nil, err
	}

	if p.AnalysedAt.IsZero() {
		p.AnalysedAt = time.Now().UTC()
	}
	if len(p.KitchenFiles) == 0 {
		p.KitchenFiles = json.RawMessage(`[]`)
	}
	if len(p.Platforms) == 0 {
		p.Platforms = json.RawMessage(`[]`)
	}
	if len(p.Suites) == 0 {
		p.Suites = json.RawMessage(`[]`)
	}

	query := `
		INSERT INTO kitchen_analysis_results (
			git_repo_name, git_repo_url, analysed_at, head_commit_sha,
			kitchen_files, has_local_override, local_override_keys,
			driver_name, provisioner_name, require_chef_omnibus,
			platforms, suites, transport_type,
			extensions, variant_files, error_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (git_repo_name, git_repo_url)
		DO UPDATE SET
			analysed_at         = EXCLUDED.analysed_at,
			head_commit_sha     = EXCLUDED.head_commit_sha,
			kitchen_files       = EXCLUDED.kitchen_files,
			has_local_override  = EXCLUDED.has_local_override,
			local_override_keys = EXCLUDED.local_override_keys,
			driver_name         = EXCLUDED.driver_name,
			provisioner_name    = EXCLUDED.provisioner_name,
			require_chef_omnibus = EXCLUDED.require_chef_omnibus,
			platforms           = EXCLUDED.platforms,
			suites              = EXCLUDED.suites,
			transport_type      = EXCLUDED.transport_type,
			extensions          = EXCLUDED.extensions,
			variant_files       = EXCLUDED.variant_files,
			error_message       = EXCLUDED.error_message,
			updated_at          = now()
		RETURNING ` + karColumns

	r, err := scanKitchenAnalysisResult(q.QueryRowContext(ctx, query,
		p.GitRepoName,
		p.GitRepoURL,
		p.AnalysedAt,
		p.HeadCommitSHA,
		p.KitchenFiles,
		p.HasLocalOverride,
		nullableJSON(p.LocalOverrideKeys),
		nullString(p.DriverName),
		nullString(p.ProvisionerName),
		nullBoolPtr(p.RequireChefOmnibus),
		p.Platforms,
		p.Suites,
		nullString(p.TransportType),
		nullableJSON(p.Extensions),
		nullableJSON(p.VariantFiles),
		nullString(p.ErrorMessage),
	))
	if err != nil {
		return nil, fmt.Errorf("datastore: upserting kitchen analysis result: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetKitchenAnalysisResult returns the analysis result for the given
// (git_repo_name, git_repo_url). Returns (nil, nil) if not found.
func (db *DB) GetKitchenAnalysisResult(ctx context.Context, gitRepoName, gitRepoURL string) (*KitchenAnalysisResult, error) {
	query := `
		SELECT ` + karColumns + `
		  FROM kitchen_analysis_results
		 WHERE git_repo_name = $1
		   AND git_repo_url = $2
	`
	r, err := scanKitchenAnalysisResult(db.q().QueryRowContext(ctx, query, gitRepoName, gitRepoURL))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting kitchen analysis result: %w", err)
	}
	return &r, nil
}

// GetKitchenAnalysisResultByName returns the first analysis result matching
// the given git_repo_name. Returns (nil, nil) if not found.
func (db *DB) GetKitchenAnalysisResultByName(ctx context.Context, gitRepoName string) (*KitchenAnalysisResult, error) {
	query := `
		SELECT ` + karColumns + `
		  FROM kitchen_analysis_results
		 WHERE git_repo_name = $1
		 LIMIT 1
	`
	r, err := scanKitchenAnalysisResult(db.q().QueryRowContext(ctx, query, gitRepoName))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting kitchen analysis result by name: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListKitchenAnalysisResults returns all kitchen analysis results ordered by
// git_repo_name.
func (db *DB) ListKitchenAnalysisResults(ctx context.Context) ([]KitchenAnalysisResult, error) {
	query := `
		SELECT ` + karColumns + `
		  FROM kitchen_analysis_results
		 ORDER BY git_repo_name
	`
	return scanKitchenAnalysisResults(db.pool.QueryContext(ctx, query))
}

// ListKitchenAnalysisResultsFiltered returns kitchen analysis results with
// optional filters. Either or both filters may be nil (meaning "don't filter").
func (db *DB) ListKitchenAnalysisResultsFiltered(ctx context.Context, driverName string, hasLocalOverride *bool) ([]KitchenAnalysisResult, error) {
	query := `SELECT ` + karColumns + ` FROM kitchen_analysis_results WHERE 1=1`
	args := []any{}
	argIdx := 1

	if driverName != "" {
		query += fmt.Sprintf(" AND driver_name = $%d", argIdx)
		args = append(args, driverName)
		argIdx++
	}
	if hasLocalOverride != nil {
		query += fmt.Sprintf(" AND has_local_override = $%d", argIdx)
		args = append(args, *hasLocalOverride)
	}

	query += " ORDER BY git_repo_name"
	return scanKitchenAnalysisResults(db.pool.QueryContext(ctx, query, args...))
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteKitchenAnalysisResultsByRepo removes the analysis result for the
// given (git_repo_name, git_repo_url).
func (db *DB) DeleteKitchenAnalysisResultsByRepo(ctx context.Context, gitRepoName, gitRepoURL string) error {
	const query = `DELETE FROM kitchen_analysis_results WHERE git_repo_name = $1 AND git_repo_url = $2`
	_, err := db.pool.ExecContext(ctx, query, gitRepoName, gitRepoURL)
	if err != nil {
		return fmt.Errorf("datastore: deleting kitchen analysis results for %s (%s): %w", gitRepoName, gitRepoURL, err)
	}
	return nil
}

// DeleteAllKitchenAnalysisResults removes all kitchen analysis results.
func (db *DB) DeleteAllKitchenAnalysisResults(ctx context.Context) error {
	const query = `DELETE FROM kitchen_analysis_results`
	_, err := db.pool.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("datastore: deleting all kitchen analysis results: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Discovered platforms
// ---------------------------------------------------------------------------

// RebuildDiscoveredPlatforms deletes all rows from kitchen_discovered_platforms
// and rebuilds the table by aggregating platform data from
// kitchen_analysis_results using jsonb_array_elements.
func (db *DB) RebuildDiscoveredPlatforms(ctx context.Context) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM kitchen_discovered_platforms`); err != nil {
			return fmt.Errorf("datastore: truncating kitchen_discovered_platforms: %w", err)
		}

		const insertQuery = `
			INSERT INTO kitchen_discovered_platforms (
				platform_name, normalised_name, os_family, os_version,
				cookbook_count, has_extensions, common_extensions, transport_type
			)
			SELECT
				p->>'name'                                          AS platform_name,
				COALESCE(p->>'normalised_name', p->>'name')         AS normalised_name,
				COALESCE(p->>'os_family', 'other')                  AS os_family,
				COALESCE(p->>'os_version', '')                      AS os_version,
				COUNT(DISTINCT kar.git_repo_name || kar.git_repo_url) AS cookbook_count,
				BOOL_OR((p->'extensions') IS NOT NULL AND (p->'extensions')::text != '{}' AND (p->'extensions')::text != 'null') AS has_extensions,
				CASE
					WHEN BOOL_OR((p->'extensions') IS NOT NULL AND (p->'extensions')::text != '{}' AND (p->'extensions')::text != 'null')
					THEN (array_agg(p->'extensions' ORDER BY kar.analysed_at DESC) FILTER (WHERE (p->'extensions') IS NOT NULL AND (p->'extensions')::text != '{}' AND (p->'extensions')::text != 'null'))[1]
					ELSE NULL
				END                                                 AS common_extensions,
				COALESCE(
					(array_agg(COALESCE(p->>'transport_type', kar.transport_type) ORDER BY kar.analysed_at DESC) FILTER (WHERE COALESCE(p->>'transport_type', kar.transport_type) IS NOT NULL AND COALESCE(p->>'transport_type', kar.transport_type) != ''))[1],
					''
				)                                                   AS transport_type
			FROM kitchen_analysis_results kar,
			     jsonb_array_elements(kar.platforms) AS p
			GROUP BY p->>'name', COALESCE(p->>'normalised_name', p->>'name'),
			         COALESCE(p->>'os_family', 'other'), COALESCE(p->>'os_version', '')
			ON CONFLICT (platform_name) DO UPDATE SET
				normalised_name   = EXCLUDED.normalised_name,
				os_family         = EXCLUDED.os_family,
				os_version        = EXCLUDED.os_version,
				cookbook_count     = EXCLUDED.cookbook_count,
				has_extensions    = EXCLUDED.has_extensions,
				common_extensions = EXCLUDED.common_extensions,
				transport_type    = EXCLUDED.transport_type,
				updated_at        = now()
		`
		if _, err := tx.ExecContext(ctx, insertQuery); err != nil {
			return fmt.Errorf("datastore: rebuilding kitchen_discovered_platforms: %w", err)
		}
		return nil
	})
}

// ListDiscoveredPlatforms returns all discovered platforms ordered by
// cookbook_count DESC, platform_name.
func (db *DB) ListDiscoveredPlatforms(ctx context.Context) ([]KitchenDiscoveredPlatform, error) {
	query := `
		SELECT ` + kdpColumns + `
		  FROM kitchen_discovered_platforms
		 ORDER BY cookbook_count DESC, platform_name
	`
	return scanKitchenDiscoveredPlatforms(db.pool.QueryContext(ctx, query))
}

// ListDiscoveredPlatformsFiltered returns discovered platforms with optional
// filters. Empty osFamily means no OS family filter. minCount of 0 means no
// minimum count filter.
func (db *DB) ListDiscoveredPlatformsFiltered(ctx context.Context, osFamily string, minCount int) ([]KitchenDiscoveredPlatform, error) {
	query := `SELECT ` + kdpColumns + ` FROM kitchen_discovered_platforms WHERE 1=1`
	args := []any{}
	argIdx := 1

	if osFamily != "" {
		query += fmt.Sprintf(" AND os_family = $%d", argIdx)
		args = append(args, osFamily)
		argIdx++
	}
	if minCount > 0 {
		query += fmt.Sprintf(" AND cookbook_count >= $%d", argIdx)
		args = append(args, minCount)
	}

	query += " ORDER BY cookbook_count DESC, platform_name"
	return scanKitchenDiscoveredPlatforms(db.pool.QueryContext(ctx, query, args...))
}

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

// GetKitchenAnalysisSummary returns aggregate statistics across all analysed
// repos.
func (db *DB) GetKitchenAnalysisSummary(ctx context.Context) (*KitchenAnalysisSummary, error) {
	summary := &KitchenAnalysisSummary{
		DriverCounts:      make(map[string]int),
		TransportCounts:   make(map[string]int),
		ProvisionerCounts: make(map[string]int),
	}

	// Scalar counts.
	const scalarQuery = `
		SELECT
			COUNT(*)                                                           AS total_scanned,
			(SELECT COUNT(*) FROM git_repos)                                   AS total_repos,
			COUNT(*) FILTER (WHERE has_local_override = true)                  AS total_with_local_override,
			COUNT(*) FILTER (
				WHERE has_local_override = true
				  AND (local_override_keys::text LIKE '%driver%'
				    OR local_override_keys::text LIKE '%platform%')
			)                                                                  AS total_with_conflicts,
			(SELECT COUNT(*) FROM kitchen_discovered_platforms)                AS platform_count
		FROM kitchen_analysis_results
	`
	var totalScanned, totalRepos, withOverride, withConflicts, platformCount int
	err := db.q().QueryRowContext(ctx, scalarQuery).Scan(
		&totalScanned, &totalRepos, &withOverride, &withConflicts, &platformCount,
	)
	if err != nil {
		return nil, fmt.Errorf("datastore: getting kitchen analysis summary scalars: %w", err)
	}
	summary.TotalScanned = totalScanned
	summary.TotalWithoutKitchen = totalRepos - totalScanned
	summary.TotalWithLocalOverride = withOverride
	summary.TotalWithConflicts = withConflicts
	summary.PlatformCount = platformCount

	// Driver counts.
	if err := db.scanGroupCounts(ctx, `
		SELECT COALESCE(driver_name, '') AS name, COUNT(*) AS cnt
		  FROM kitchen_analysis_results
		 GROUP BY driver_name
	`, summary.DriverCounts); err != nil {
		return nil, fmt.Errorf("datastore: getting kitchen analysis driver counts: %w", err)
	}

	// Transport counts.
	if err := db.scanGroupCounts(ctx, `
		SELECT COALESCE(transport_type, '') AS name, COUNT(*) AS cnt
		  FROM kitchen_analysis_results
		 GROUP BY transport_type
	`, summary.TransportCounts); err != nil {
		return nil, fmt.Errorf("datastore: getting kitchen analysis transport counts: %w", err)
	}

	// Provisioner counts.
	if err := db.scanGroupCounts(ctx, `
		SELECT COALESCE(provisioner_name, '') AS name, COUNT(*) AS cnt
		  FROM kitchen_analysis_results
		 GROUP BY provisioner_name
	`, summary.ProvisionerCounts); err != nil {
		return nil, fmt.Errorf("datastore: getting kitchen analysis provisioner counts: %w", err)
	}

	return summary, nil
}

// scanGroupCounts runs a query that returns (name TEXT, cnt INT) rows and
// populates the given map.
func (db *DB) scanGroupCounts(ctx context.Context, query string, dest map[string]int) error {
	rows, err := db.pool.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var cnt int
		if err := rows.Scan(&name, &cnt); err != nil {
			return err
		}
		if name == "" {
			name = "unknown"
		}
		dest[name] = cnt
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanKitchenAnalysisResult(row *sql.Row) (KitchenAnalysisResult, error) {
	var r KitchenAnalysisResult
	var localOverrideKeys, extensions, variantFiles []byte
	var driverName, provisionerName, transportType, errorMessage sql.NullString
	var requireChefOmnibus sql.NullBool

	err := row.Scan(
		&r.GitRepoName,
		&r.GitRepoURL,
		&r.AnalysedAt,
		&r.HeadCommitSHA,
		&r.KitchenFiles,
		&r.HasLocalOverride,
		&localOverrideKeys,
		&driverName,
		&provisionerName,
		&requireChefOmnibus,
		&r.Platforms,
		&r.Suites,
		&transportType,
		&extensions,
		&variantFiles,
		&errorMessage,
		&r.CreatedAt,
		&r.UpdatedAt,
	)
	if err != nil {
		return KitchenAnalysisResult{}, err
	}

	r.LocalOverrideKeys = normaliseJSON(localOverrideKeys)
	r.DriverName = stringFromNull(driverName)
	r.ProvisionerName = stringFromNull(provisionerName)
	r.TransportType = stringFromNull(transportType)
	r.Extensions = normaliseJSON(extensions)
	r.VariantFiles = normaliseJSON(variantFiles)
	r.ErrorMessage = stringFromNull(errorMessage)
	if requireChefOmnibus.Valid {
		b := requireChefOmnibus.Bool
		r.RequireChefOmnibus = &b
	}

	return r, nil
}

func scanKitchenAnalysisResults(rows *sql.Rows, err error) ([]KitchenAnalysisResult, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: listing kitchen analysis results: %w", err)
	}
	defer rows.Close()

	var results []KitchenAnalysisResult
	for rows.Next() {
		var r KitchenAnalysisResult
		var localOverrideKeys, extensions, variantFiles []byte
		var driverName, provisionerName, transportType, errorMessage sql.NullString
		var requireChefOmnibus sql.NullBool

		if err := rows.Scan(
			&r.GitRepoName,
			&r.GitRepoURL,
			&r.AnalysedAt,
			&r.HeadCommitSHA,
			&r.KitchenFiles,
			&r.HasLocalOverride,
			&localOverrideKeys,
			&driverName,
			&provisionerName,
			&requireChefOmnibus,
			&r.Platforms,
			&r.Suites,
			&transportType,
			&extensions,
			&variantFiles,
			&errorMessage,
			&r.CreatedAt,
			&r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning kitchen analysis result row: %w", err)
		}

		r.LocalOverrideKeys = normaliseJSON(localOverrideKeys)
		r.DriverName = stringFromNull(driverName)
		r.ProvisionerName = stringFromNull(provisionerName)
		r.TransportType = stringFromNull(transportType)
		r.Extensions = normaliseJSON(extensions)
		r.VariantFiles = normaliseJSON(variantFiles)
		r.ErrorMessage = stringFromNull(errorMessage)
		if requireChefOmnibus.Valid {
			b := requireChefOmnibus.Bool
			r.RequireChefOmnibus = &b
		}

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating kitchen analysis results: %w", err)
	}
	return results, nil
}

func scanKitchenDiscoveredPlatforms(rows *sql.Rows, err error) ([]KitchenDiscoveredPlatform, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: listing kitchen discovered platforms: %w", err)
	}
	defer rows.Close()

	var results []KitchenDiscoveredPlatform
	for rows.Next() {
		var p KitchenDiscoveredPlatform
		var osVersion, transportType sql.NullString
		var commonExtensions []byte

		if err := rows.Scan(
			&p.PlatformName,
			&p.NormalisedName,
			&p.OSFamily,
			&osVersion,
			&p.CookbookCount,
			&p.HasExtensions,
			&commonExtensions,
			&transportType,
			&p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning kitchen discovered platform row: %w", err)
		}

		p.OSVersion = stringFromNull(osVersion)
		p.CommonExtensions = normaliseJSON(commonExtensions)
		p.TransportType = stringFromNull(transportType)

		results = append(results, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating kitchen discovered platforms: %w", err)
	}
	return results, nil
}
