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

// CookbookUsageAnalysis represents a row in the cookbook_usage_analysis table.
// Each row is a snapshot of an analysis run for a single organisation tied to
// a specific collection run.
type CookbookUsageAnalysis struct {
	ID              string    `json:"id"`
	OrganisationID  string    `json:"organisation_id"`
	CollectionRunID string    `json:"collection_run_id"`
	TotalCookbooks  int       `json:"total_cookbooks"`
	ActiveCookbooks int       `json:"active_cookbooks"`
	UnusedCookbooks int       `json:"unused_cookbooks"`
	TotalNodes      int       `json:"total_nodes"`
	AnalysedAt      time.Time `json:"analysed_at"`
	CreatedAt       time.Time `json:"created_at"`
}

// MarshalJSON implements json.Marshaler for CookbookUsageAnalysis.
func (a CookbookUsageAnalysis) MarshalJSON() ([]byte, error) {
	type Alias CookbookUsageAnalysis
	return json.Marshal((Alias)(a))
}

// CookbookUsageDetail represents a row in the cookbook_usage_detail table.
// Each row stores per-cookbook-version usage statistics within a single
// analysis run.
type CookbookUsageDetail struct {
	ID                   string          `json:"id"`
	AnalysisID           string          `json:"analysis_id"`
	OrganisationID       string          `json:"organisation_id"`
	CookbookName         string          `json:"cookbook_name"`
	CookbookVersion      string          `json:"cookbook_version"`
	NodeCount            int             `json:"node_count"`
	IsActive             bool            `json:"is_active"`
	Roles                json.RawMessage `json:"roles,omitempty"`
	PolicyNames          json.RawMessage `json:"policy_names,omitempty"`
	PolicyGroups         json.RawMessage `json:"policy_groups,omitempty"`
	PlatformCounts       json.RawMessage `json:"platform_counts,omitempty"`
	PlatformFamilyCounts json.RawMessage `json:"platform_family_counts,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
}

// MarshalJSON implements json.Marshaler for CookbookUsageDetail.
func (d CookbookUsageDetail) MarshalJSON() ([]byte, error) {
	type Alias CookbookUsageDetail
	return json.Marshal((Alias)(d))
}

// ---------------------------------------------------------------------------
// Insert analysis header
// ---------------------------------------------------------------------------

// InsertCookbookUsageAnalysisParams holds the fields required to insert a
// cookbook usage analysis snapshot.
type InsertCookbookUsageAnalysisParams struct {
	OrganisationID  string
	CollectionRunID string
	TotalCookbooks  int
	ActiveCookbooks int
	UnusedCookbooks int
	TotalNodes      int
	AnalysedAt      time.Time
}

// InsertCookbookUsageAnalysis inserts a single analysis header row and
// returns the created row including the generated ID.
func (db *DB) InsertCookbookUsageAnalysis(ctx context.Context, p InsertCookbookUsageAnalysisParams) (CookbookUsageAnalysis, error) {
	return db.insertCookbookUsageAnalysis(ctx, db.q(), p)
}

func (db *DB) insertCookbookUsageAnalysis(ctx context.Context, q queryable, p InsertCookbookUsageAnalysisParams) (CookbookUsageAnalysis, error) {
	if err := validateAnalysisParams(p); err != nil {
		return CookbookUsageAnalysis{}, err
	}

	const query = `
		INSERT INTO cookbook_usage_analysis
			(organisation_id, collection_run_id, total_cookbooks, active_cookbooks,
			 unused_cookbooks, total_nodes, analysed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, organisation_id, collection_run_id, total_cookbooks,
				  active_cookbooks, unused_cookbooks, total_nodes, analysed_at, created_at
	`

	return scanCookbookUsageAnalysis(q.QueryRowContext(ctx, query,
		p.OrganisationID,
		p.CollectionRunID,
		p.TotalCookbooks,
		p.ActiveCookbooks,
		p.UnusedCookbooks,
		p.TotalNodes,
		p.AnalysedAt,
	))
}

// InsertCookbookUsageAnalysisTx inserts a single analysis header row within
// the given transaction.
func (db *DB) InsertCookbookUsageAnalysisTx(ctx context.Context, tx *sql.Tx, p InsertCookbookUsageAnalysisParams) (CookbookUsageAnalysis, error) {
	return db.insertCookbookUsageAnalysis(ctx, tx, p)
}

// ---------------------------------------------------------------------------
// Insert detail rows
// ---------------------------------------------------------------------------

// InsertCookbookUsageDetailParams holds the fields required to insert a
// single cookbook usage detail row.
type InsertCookbookUsageDetailParams struct {
	AnalysisID           string
	OrganisationID       string
	CookbookName         string
	CookbookVersion      string
	NodeCount            int
	IsActive             bool
	Roles                json.RawMessage
	PolicyNames          json.RawMessage
	PolicyGroups         json.RawMessage
	PlatformCounts       json.RawMessage
	PlatformFamilyCounts json.RawMessage
}

// InsertCookbookUsageDetail inserts a single detail row and returns the
// created row.
func (db *DB) InsertCookbookUsageDetail(ctx context.Context, p InsertCookbookUsageDetailParams) (CookbookUsageDetail, error) {
	return db.insertCookbookUsageDetail(ctx, db.q(), p)
}

func (db *DB) insertCookbookUsageDetail(ctx context.Context, q queryable, p InsertCookbookUsageDetailParams) (CookbookUsageDetail, error) {
	if err := validateDetailParams(p); err != nil {
		return CookbookUsageDetail{}, err
	}

	const query = `
		INSERT INTO cookbook_usage_detail
			(analysis_id, organisation_id, cookbook_name, cookbook_version,
			 node_count, is_active, roles, policy_names,
			 policy_groups, platform_counts, platform_family_counts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, analysis_id, organisation_id, cookbook_name, cookbook_version,
				  node_count, is_active, roles, policy_names,
				  policy_groups, platform_counts, platform_family_counts, created_at
	`

	return scanCookbookUsageDetail(q.QueryRowContext(ctx, query,
		p.AnalysisID,
		p.OrganisationID,
		p.CookbookName,
		p.CookbookVersion,
		p.NodeCount,
		p.IsActive,
		nullableJSON(p.Roles),
		nullableJSON(p.PolicyNames),
		nullableJSON(p.PolicyGroups),
		nullableJSON(p.PlatformCounts),
		nullableJSON(p.PlatformFamilyCounts),
	))
}

// InsertCookbookUsageDetailTx inserts a single detail row within the given
// transaction.
func (db *DB) InsertCookbookUsageDetailTx(ctx context.Context, tx *sql.Tx, p InsertCookbookUsageDetailParams) (CookbookUsageDetail, error) {
	return db.insertCookbookUsageDetail(ctx, tx, p)
}

// BulkInsertCookbookUsageDetails inserts multiple detail rows in a single
// transaction. Returns the count of rows inserted. If any insert fails, the
// entire batch is rolled back.
func (db *DB) BulkInsertCookbookUsageDetails(ctx context.Context, params []InsertCookbookUsageDetailParams) (int, error) {
	if len(params) == 0 {
		return 0, nil
	}

	inserted := 0
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		const query = `
			INSERT INTO cookbook_usage_detail
				(analysis_id, organisation_id, cookbook_name, cookbook_version,
				 node_count, is_active, roles, policy_names,
				 policy_groups, platform_counts, platform_family_counts)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`

		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return fmt.Errorf("datastore: preparing cookbook usage detail insert: %w", err)
		}
		defer stmt.Close()

		for i, p := range params {
			if err := validateDetailParams(p); err != nil {
				return fmt.Errorf("row %d: %w", i, err)
			}

			_, err := stmt.ExecContext(ctx,
				p.AnalysisID,
				p.OrganisationID,
				p.CookbookName,
				p.CookbookVersion,
				p.NodeCount,
				p.IsActive,
				nullableJSON(p.Roles),
				nullableJSON(p.PolicyNames),
				nullableJSON(p.PolicyGroups),
				nullableJSON(p.PlatformCounts),
				nullableJSON(p.PlatformFamilyCounts),
			)
			if err != nil {
				return fmt.Errorf("datastore: inserting cookbook usage detail (row %d): %w", i, err)
			}
			inserted++
		}

		return nil
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

// BulkInsertCookbookUsageDetailsTx inserts multiple detail rows within the
// provided transaction. Returns the count of rows inserted.
func (db *DB) BulkInsertCookbookUsageDetailsTx(ctx context.Context, tx *sql.Tx, params []InsertCookbookUsageDetailParams) (int, error) {
	if len(params) == 0 {
		return 0, nil
	}

	const query = `
		INSERT INTO cookbook_usage_detail
			(analysis_id, organisation_id, cookbook_name, cookbook_version,
			 node_count, is_active, roles, policy_names,
			 policy_groups, platform_counts, platform_family_counts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("datastore: preparing cookbook usage detail insert: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for i, p := range params {
		if err := validateDetailParams(p); err != nil {
			return inserted, fmt.Errorf("row %d: %w", i, err)
		}

		_, err := stmt.ExecContext(ctx,
			p.AnalysisID,
			p.OrganisationID,
			p.CookbookName,
			p.CookbookVersion,
			p.NodeCount,
			p.IsActive,
			nullableJSON(p.Roles),
			nullableJSON(p.PolicyNames),
			nullableJSON(p.PolicyGroups),
			nullableJSON(p.PlatformCounts),
			nullableJSON(p.PlatformFamilyCounts),
		)
		if err != nil {
			return inserted, fmt.Errorf("datastore: inserting cookbook usage detail (row %d): %w", i, err)
		}
		inserted++
	}

	return inserted, nil
}

// ---------------------------------------------------------------------------
// Query — Analysis headers
// ---------------------------------------------------------------------------

// GetCookbookUsageAnalysis returns a single analysis row by ID.
func (db *DB) GetCookbookUsageAnalysis(ctx context.Context, id string) (CookbookUsageAnalysis, error) {
	const query = `
		SELECT id, organisation_id, collection_run_id, total_cookbooks,
			   active_cookbooks, unused_cookbooks, total_nodes, analysed_at, created_at
		FROM cookbook_usage_analysis
		WHERE id = $1
	`
	return scanCookbookUsageAnalysis(db.pool.QueryRowContext(ctx, query, id))
}

// GetLatestCookbookUsageAnalysis returns the most recent analysis row for
// the given organisation, or ErrNotFound if none exists.
func (db *DB) GetLatestCookbookUsageAnalysis(ctx context.Context, organisationID string) (CookbookUsageAnalysis, error) {
	const query = `
		SELECT id, organisation_id, collection_run_id, total_cookbooks,
			   active_cookbooks, unused_cookbooks, total_nodes, analysed_at, created_at
		FROM cookbook_usage_analysis
		WHERE organisation_id = $1
		ORDER BY analysed_at DESC
		LIMIT 1
	`
	return scanCookbookUsageAnalysis(db.pool.QueryRowContext(ctx, query, organisationID))
}

// ListCookbookUsageAnalyses returns all analysis snapshots for the given
// organisation, ordered by analysed_at descending (most recent first).
func (db *DB) ListCookbookUsageAnalyses(ctx context.Context, organisationID string) ([]CookbookUsageAnalysis, error) {
	const query = `
		SELECT id, organisation_id, collection_run_id, total_cookbooks,
			   active_cookbooks, unused_cookbooks, total_nodes, analysed_at, created_at
		FROM cookbook_usage_analysis
		WHERE organisation_id = $1
		ORDER BY analysed_at DESC
	`
	return scanCookbookUsageAnalyses(db.pool.QueryContext(ctx, query, organisationID))
}

// GetCookbookUsageAnalysisByCollectionRun returns the analysis row for the
// given collection run, or ErrNotFound if none exists.
func (db *DB) GetCookbookUsageAnalysisByCollectionRun(ctx context.Context, collectionRunID string) (CookbookUsageAnalysis, error) {
	const query = `
		SELECT id, organisation_id, collection_run_id, total_cookbooks,
			   active_cookbooks, unused_cookbooks, total_nodes, analysed_at, created_at
		FROM cookbook_usage_analysis
		WHERE collection_run_id = $1
	`
	return scanCookbookUsageAnalysis(db.pool.QueryRowContext(ctx, query, collectionRunID))
}

// ---------------------------------------------------------------------------
// Query — Detail rows
// ---------------------------------------------------------------------------

// ListCookbookUsageDetails returns all detail rows for the given analysis
// run, ordered by node_count descending then cookbook_name, cookbook_version.
func (db *DB) ListCookbookUsageDetails(ctx context.Context, analysisID string) ([]CookbookUsageDetail, error) {
	const query = `
		SELECT id, analysis_id, organisation_id, cookbook_name, cookbook_version,
			   node_count, is_active, roles, policy_names,
			   policy_groups, platform_counts, platform_family_counts, created_at
		FROM cookbook_usage_detail
		WHERE analysis_id = $1
		ORDER BY node_count DESC, cookbook_name, cookbook_version
	`
	return scanCookbookUsageDetails(db.pool.QueryContext(ctx, query, analysisID))
}

// ListCookbookUsageDetailsByCookbook returns all detail rows for a specific
// cookbook name within the given analysis run, ordered by cookbook_version.
func (db *DB) ListCookbookUsageDetailsByCookbook(ctx context.Context, analysisID, cookbookName string) ([]CookbookUsageDetail, error) {
	const query = `
		SELECT id, analysis_id, organisation_id, cookbook_name, cookbook_version,
			   node_count, is_active, roles, policy_names,
			   policy_groups, platform_counts, platform_family_counts, created_at
		FROM cookbook_usage_detail
		WHERE analysis_id = $1 AND cookbook_name = $2
		ORDER BY cookbook_version
	`
	return scanCookbookUsageDetails(db.pool.QueryContext(ctx, query, analysisID, cookbookName))
}

// ListActiveCookbookUsageDetails returns all detail rows flagged as active
// for the given analysis run, ordered by node_count descending.
func (db *DB) ListActiveCookbookUsageDetails(ctx context.Context, analysisID string) ([]CookbookUsageDetail, error) {
	const query = `
		SELECT id, analysis_id, organisation_id, cookbook_name, cookbook_version,
			   node_count, is_active, roles, policy_names,
			   policy_groups, platform_counts, platform_family_counts, created_at
		FROM cookbook_usage_detail
		WHERE analysis_id = $1 AND is_active = TRUE
		ORDER BY node_count DESC, cookbook_name, cookbook_version
	`
	return scanCookbookUsageDetails(db.pool.QueryContext(ctx, query, analysisID))
}

// ListUnusedCookbookUsageDetails returns all detail rows flagged as inactive
// (unused) for the given analysis run, ordered by cookbook_name,
// cookbook_version.
func (db *DB) ListUnusedCookbookUsageDetails(ctx context.Context, analysisID string) ([]CookbookUsageDetail, error) {
	const query = `
		SELECT id, analysis_id, organisation_id, cookbook_name, cookbook_version,
			   node_count, is_active, roles, policy_names,
			   policy_groups, platform_counts, platform_family_counts, created_at
		FROM cookbook_usage_detail
		WHERE analysis_id = $1 AND is_active = FALSE
		ORDER BY cookbook_name, cookbook_version
	`
	return scanCookbookUsageDetails(db.pool.QueryContext(ctx, query, analysisID))
}

// ---------------------------------------------------------------------------
// Lightweight summary query (for blast radius / internal use)
// ---------------------------------------------------------------------------

// CookbookUsageSummary is a lightweight projection of cookbook_usage_detail
// containing only the fields needed for blast radius computation. This avoids
// fetching the large JSONB columns (roles, policy_groups, platform_counts,
// platform_family_counts) that are not needed by the caller.
type CookbookUsageSummary struct {
	CookbookName    string          `json:"cookbook_name"`
	CookbookVersion string          `json:"cookbook_version"`
	NodeCount       int             `json:"node_count"`
	PolicyNames     json.RawMessage `json:"policy_names,omitempty"`
}

// ListCookbookUsageSummaries returns a lightweight summary of each
// cookbook_usage_detail row for the given analysis ID. Only cookbook_name,
// cookbook_version, node_count, and policy_names are fetched — the other
// JSONB columns are skipped entirely at the SQL level.
func (db *DB) ListCookbookUsageSummaries(ctx context.Context, analysisID string) ([]CookbookUsageSummary, error) {
	const query = `
		SELECT cookbook_name, cookbook_version, node_count, policy_names
		FROM cookbook_usage_detail
		WHERE analysis_id = $1
		ORDER BY node_count DESC, cookbook_name, cookbook_version
	`

	rows, err := db.pool.QueryContext(ctx, query, analysisID)
	if err != nil {
		return nil, fmt.Errorf("datastore: querying cookbook usage summaries: %w", err)
	}
	defer rows.Close()

	var summaries []CookbookUsageSummary
	for rows.Next() {
		var s CookbookUsageSummary
		var policyNames sql.NullString

		if err := rows.Scan(&s.CookbookName, &s.CookbookVersion, &s.NodeCount, &policyNames); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookbook usage summary row: %w", err)
		}
		if policyNames.Valid {
			s.PolicyNames = json.RawMessage(policyNames.String)
		}
		summaries = append(summaries, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookbook usage summary rows: %w", err)
	}
	return summaries, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteCookbookUsageAnalysis deletes an analysis run and all associated
// detail rows (via CASCADE). Returns the number of detail rows deleted.
func (db *DB) DeleteCookbookUsageAnalysis(ctx context.Context, id string) error {
	_, err := db.pool.ExecContext(ctx,
		`DELETE FROM cookbook_usage_analysis WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("datastore: deleting cookbook usage analysis: %w", err)
	}
	return nil
}

// DeleteCookbookUsageAnalysesByOrg deletes all analysis runs (and cascaded
// details) for the given organisation. Returns the number of analysis rows
// deleted.
func (db *DB) DeleteCookbookUsageAnalysesByOrg(ctx context.Context, organisationID string) (int, error) {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM cookbook_usage_analysis WHERE organisation_id = $1`,
		organisationID,
	)
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting cookbook usage analyses by org: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: checking rows affected: %w", err)
	}
	return int(n), nil
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

func validateAnalysisParams(p InsertCookbookUsageAnalysisParams) error {
	if p.OrganisationID == "" {
		return fmt.Errorf("datastore: organisation ID is required for cookbook usage analysis")
	}
	if p.CollectionRunID == "" {
		return fmt.Errorf("datastore: collection run ID is required for cookbook usage analysis")
	}
	if p.AnalysedAt.IsZero() {
		return fmt.Errorf("datastore: analysed_at timestamp is required for cookbook usage analysis")
	}
	return nil
}

func validateDetailParams(p InsertCookbookUsageDetailParams) error {
	if p.AnalysisID == "" {
		return fmt.Errorf("datastore: analysis ID is required for cookbook usage detail")
	}
	if p.OrganisationID == "" {
		return fmt.Errorf("datastore: organisation ID is required for cookbook usage detail")
	}
	if p.CookbookName == "" {
		return fmt.Errorf("datastore: cookbook name is required for cookbook usage detail")
	}
	if p.CookbookVersion == "" {
		return fmt.Errorf("datastore: cookbook version is required for cookbook usage detail")
	}
	return nil
}

// ---------------------------------------------------------------------------
// JSON helper
// ---------------------------------------------------------------------------

// nullableJSON returns a sql.NullString wrapping JSON bytes. If the slice is
// nil or empty, it returns a NULL. This avoids writing empty strings into
// JSONB columns.
func nullableJSON(data json.RawMessage) sql.NullString {
	if len(data) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: string(data), Valid: true}
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanCookbookUsageAnalysis(row *sql.Row) (CookbookUsageAnalysis, error) {
	var a CookbookUsageAnalysis
	err := row.Scan(
		&a.ID,
		&a.OrganisationID,
		&a.CollectionRunID,
		&a.TotalCookbooks,
		&a.ActiveCookbooks,
		&a.UnusedCookbooks,
		&a.TotalNodes,
		&a.AnalysedAt,
		&a.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return CookbookUsageAnalysis{}, ErrNotFound
		}
		return CookbookUsageAnalysis{}, fmt.Errorf("datastore: scanning cookbook usage analysis: %w", err)
	}
	return a, nil
}

func scanCookbookUsageAnalyses(rows *sql.Rows, err error) ([]CookbookUsageAnalysis, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying cookbook usage analyses: %w", err)
	}
	defer rows.Close()

	var analyses []CookbookUsageAnalysis
	for rows.Next() {
		var a CookbookUsageAnalysis
		if err := rows.Scan(
			&a.ID,
			&a.OrganisationID,
			&a.CollectionRunID,
			&a.TotalCookbooks,
			&a.ActiveCookbooks,
			&a.UnusedCookbooks,
			&a.TotalNodes,
			&a.AnalysedAt,
			&a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookbook usage analysis row: %w", err)
		}
		analyses = append(analyses, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookbook usage analysis rows: %w", err)
	}
	return analyses, nil
}

func scanCookbookUsageDetail(row *sql.Row) (CookbookUsageDetail, error) {
	var d CookbookUsageDetail
	var roles, policyNames, policyGroups, platformCounts, platformFamilyCounts sql.NullString

	err := row.Scan(
		&d.ID,
		&d.AnalysisID,
		&d.OrganisationID,
		&d.CookbookName,
		&d.CookbookVersion,
		&d.NodeCount,
		&d.IsActive,
		&roles,
		&policyNames,
		&policyGroups,
		&platformCounts,
		&platformFamilyCounts,
		&d.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return CookbookUsageDetail{}, ErrNotFound
		}
		return CookbookUsageDetail{}, fmt.Errorf("datastore: scanning cookbook usage detail: %w", err)
	}

	if roles.Valid {
		d.Roles = json.RawMessage(roles.String)
	}
	if policyNames.Valid {
		d.PolicyNames = json.RawMessage(policyNames.String)
	}
	if policyGroups.Valid {
		d.PolicyGroups = json.RawMessage(policyGroups.String)
	}
	if platformCounts.Valid {
		d.PlatformCounts = json.RawMessage(platformCounts.String)
	}
	if platformFamilyCounts.Valid {
		d.PlatformFamilyCounts = json.RawMessage(platformFamilyCounts.String)
	}

	return d, nil
}

func scanCookbookUsageDetails(rows *sql.Rows, err error) ([]CookbookUsageDetail, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying cookbook usage details: %w", err)
	}
	defer rows.Close()

	var details []CookbookUsageDetail
	for rows.Next() {
		var d CookbookUsageDetail
		var roles, policyNames, policyGroups, platformCounts, platformFamilyCounts sql.NullString

		if err := rows.Scan(
			&d.ID,
			&d.AnalysisID,
			&d.OrganisationID,
			&d.CookbookName,
			&d.CookbookVersion,
			&d.NodeCount,
			&d.IsActive,
			&roles,
			&policyNames,
			&policyGroups,
			&platformCounts,
			&platformFamilyCounts,
			&d.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookbook usage detail row: %w", err)
		}

		if roles.Valid {
			d.Roles = json.RawMessage(roles.String)
		}
		if policyNames.Valid {
			d.PolicyNames = json.RawMessage(policyNames.String)
		}
		if policyGroups.Valid {
			d.PolicyGroups = json.RawMessage(policyGroups.String)
		}
		if platformCounts.Valid {
			d.PlatformCounts = json.RawMessage(platformCounts.String)
		}
		if platformFamilyCounts.Valid {
			d.PlatformFamilyCounts = json.RawMessage(platformFamilyCounts.String)
		}

		details = append(details, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookbook usage detail rows: %w", err)
	}
	return details, nil
}
