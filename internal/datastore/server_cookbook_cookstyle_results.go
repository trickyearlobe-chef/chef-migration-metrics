// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ServerCookbookCookstyleResult represents a row in the
// server_cookbook_cookstyle_results table. Server cookbooks are immutable
// snapshots fetched from a Chef Server, so there is no CommitSHA field.
type ServerCookbookCookstyleResult struct {
	OrganisationName    string    `json:"organisation_name"`
	CookbookName        string    `json:"cookbook_name"`
	CookbookVersion     string    `json:"cookbook_version"`
	TargetChefVersion   string    `json:"target_chef_version"`
	Passed              bool      `json:"passed"`
	OffenceCount        int       `json:"offence_count"`
	DeprecationCount    int       `json:"deprecation_count"`
	CorrectnessCount    int       `json:"correctness_count"`
	DeprecationWarnings []byte    `json:"deprecation_warnings,omitempty"` // JSONB
	Offences            []byte    `json:"offences,omitempty"`             // JSONB
	ProcessStdout       string    `json:"process_stdout,omitempty"`
	ProcessStderr       string    `json:"process_stderr,omitempty"`
	DurationSeconds     int       `json:"duration_seconds"`
	ErrorMessage        string    `json:"error_message,omitempty"`
	ScannedAt           time.Time `json:"scanned_at"`
	CreatedAt           time.Time `json:"created_at"`
}

// UpsertServerCookbookCookstyleResultParams contains the fields needed to
// insert or update a server_cookbook_cookstyle_results row. The unique
// constraint is (organisation_name, cookbook_name, cookbook_version, target_chef_version).
type UpsertServerCookbookCookstyleResultParams struct {
	OrganisationName    string
	CookbookName        string
	CookbookVersion     string
	TargetChefVersion   string
	Passed              bool
	OffenceCount        int
	DeprecationCount    int
	CorrectnessCount    int
	DeprecationWarnings []byte // JSONB
	Offences            []byte // JSONB
	ProcessStdout       string
	ProcessStderr       string
	DurationSeconds     int
	ErrorMessage        string
	ScannedAt           time.Time
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertServerCookbookCookstyleResult inserts a new cookstyle result or
// updates the existing one for the same (organisation_name, cookbook_name,
// cookbook_version, target_chef_version) combination. Returns the resulting row.
func (db *DB) UpsertServerCookbookCookstyleResult(ctx context.Context, p UpsertServerCookbookCookstyleResultParams) (*ServerCookbookCookstyleResult, error) {
	return db.upsertServerCookbookCookstyleResult(ctx, db.q(), p)
}

func (db *DB) upsertServerCookbookCookstyleResult(ctx context.Context, q queryable, p UpsertServerCookbookCookstyleResultParams) (*ServerCookbookCookstyleResult, error) {
	const query = `
		INSERT INTO server_cookbook_cookstyle_results (
			organisation_name, cookbook_name, cookbook_version,
			target_chef_version, passed,
			offence_count, deprecation_count, correctness_count,
			deprecation_warnings, offences,
			process_stdout, process_stderr, duration_seconds,
			error_message, scanned_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (organisation_name, cookbook_name, cookbook_version, target_chef_version)
		DO UPDATE SET
			passed              = EXCLUDED.passed,
			offence_count       = EXCLUDED.offence_count,
			deprecation_count   = EXCLUDED.deprecation_count,
			correctness_count   = EXCLUDED.correctness_count,
			deprecation_warnings = EXCLUDED.deprecation_warnings,
			offences            = EXCLUDED.offences,
			process_stdout      = EXCLUDED.process_stdout,
			process_stderr      = EXCLUDED.process_stderr,
			duration_seconds    = EXCLUDED.duration_seconds,
			error_message       = EXCLUDED.error_message,
			scanned_at          = EXCLUDED.scanned_at
		RETURNING organisation_name, cookbook_name, cookbook_version,
		          target_chef_version, passed,
		          offence_count, deprecation_count, correctness_count,
		          deprecation_warnings, offences,
		          process_stdout, process_stderr, duration_seconds,
		          error_message, scanned_at, created_at
	`

	var targetVersion sql.NullString
	if p.TargetChefVersion != "" {
		targetVersion = sql.NullString{String: p.TargetChefVersion, Valid: true}
	}

	r := &ServerCookbookCookstyleResult{}
	var tvOut sql.NullString
	var deprecationWarnings, offences []byte
	var stdout, stderr sql.NullString
	var duration sql.NullInt64
	var errorMessage sql.NullString

	err := q.QueryRowContext(ctx, query,
		p.OrganisationName,
		p.CookbookName,
		p.CookbookVersion,
		targetVersion,
		p.Passed,
		p.OffenceCount,
		p.DeprecationCount,
		p.CorrectnessCount,
		p.DeprecationWarnings,
		p.Offences,
		nullString(p.ProcessStdout),
		nullString(p.ProcessStderr),
		nullInt(p.DurationSeconds),
		nullString(p.ErrorMessage),
		p.ScannedAt,
	).Scan(
		&r.OrganisationName,
		&r.CookbookName,
		&r.CookbookVersion,
		&tvOut,
		&r.Passed,
		&r.OffenceCount,
		&r.DeprecationCount,
		&r.CorrectnessCount,
		&deprecationWarnings,
		&offences,
		&stdout,
		&stderr,
		&duration,
		&errorMessage,
		&r.ScannedAt,
		&r.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("datastore: upserting server cookbook cookstyle result: %w", err)
	}

	r.TargetChefVersion = stringFromNull(tvOut)
	r.DeprecationWarnings = deprecationWarnings
	r.Offences = offences
	r.ProcessStdout = stringFromNull(stdout)
	r.ProcessStderr = stringFromNull(stderr)
	r.DurationSeconds = intFromNull(duration)
	r.ErrorMessage = stringFromNull(errorMessage)

	return r, nil
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetServerCookbookCookstyleResult returns the cookstyle result for the given
// organisation, cookbook, and target Chef version. Returns (nil, nil) if no
// result exists.
func (db *DB) GetServerCookbookCookstyleResult(ctx context.Context, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*ServerCookbookCookstyleResult, error) {
	return db.getServerCookbookCookstyleResult(ctx, db.q(), orgName, cookbookName, cookbookVersion, targetChefVersion)
}

func (db *DB) getServerCookbookCookstyleResult(ctx context.Context, q queryable, orgName, cookbookName, cookbookVersion, targetChefVersion string) (*ServerCookbookCookstyleResult, error) {
	const query = `
		SELECT organisation_name, cookbook_name, cookbook_version,
		       target_chef_version, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       error_message, scanned_at, created_at
		  FROM server_cookbook_cookstyle_results
		 WHERE organisation_name = $1
		   AND cookbook_name = $2
		   AND cookbook_version = $3
		   AND (target_chef_version = $4 OR ($4 = '' AND target_chef_version IS NULL))
	`

	var targetVersion sql.NullString
	if targetChefVersion != "" {
		targetVersion = sql.NullString{String: targetChefVersion, Valid: true}
	}

	r, err := scanServerCookbookCookstyleResult(q.QueryRowContext(ctx, query, orgName, cookbookName, cookbookVersion, targetVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting server cookbook cookstyle result: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListServerCookbookCookstyleResults returns all cookstyle results for the
// given organisation, cookbook name, and cookbook version, ordered by
// target_chef_version.
func (db *DB) ListServerCookbookCookstyleResults(ctx context.Context, orgName, cookbookName, cookbookVersion string) ([]ServerCookbookCookstyleResult, error) {
	return db.listServerCookbookCookstyleResults(ctx, db.q(), orgName, cookbookName, cookbookVersion)
}

func (db *DB) listServerCookbookCookstyleResults(ctx context.Context, q queryable, orgName, cookbookName, cookbookVersion string) ([]ServerCookbookCookstyleResult, error) {
	const query = `
		SELECT organisation_name, cookbook_name, cookbook_version,
		       target_chef_version, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       error_message, scanned_at, created_at
		  FROM server_cookbook_cookstyle_results
		 WHERE organisation_name = $1
		   AND cookbook_name = $2
		   AND cookbook_version = $3
		 ORDER BY target_chef_version NULLS FIRST
	`
	return scanServerCookbookCookstyleResults(q.QueryContext(ctx, query, orgName, cookbookName, cookbookVersion))
}

// ListServerCookbookCookstyleResultsByOrganisationAndVersions returns all
// cookstyle results for server cookbooks belonging to the given organisation,
// filtered by the specified target Chef versions. Rows where
// target_chef_version IS NULL are always included (some cookbooks are scanned
// without a target version profile). If targetChefVersions is empty, only
// NULL-version rows are returned.
func (db *DB) ListServerCookbookCookstyleResultsByOrganisationAndVersions(ctx context.Context, orgName string, targetChefVersions []string) ([]ServerCookbookCookstyleResult, error) {
	return db.listServerCookbookCookstyleResultsByOrganisationAndVersions(ctx, db.q(), orgName, targetChefVersions)
}

func (db *DB) listServerCookbookCookstyleResultsByOrganisationAndVersions(ctx context.Context, q queryable, orgName string, targetChefVersions []string) ([]ServerCookbookCookstyleResult, error) {
	args := []any{orgName}

	var versionClause string
	if len(targetChefVersions) > 0 {
		placeholders := make([]string, len(targetChefVersions))
		for i, v := range targetChefVersions {
			args = append(args, v)
			placeholders[i] = "$" + strconv.Itoa(i+2)
		}
		versionClause = "AND (target_chef_version IN (" + strings.Join(placeholders, ", ") + ") OR target_chef_version IS NULL)"
	} else {
		versionClause = "AND target_chef_version IS NULL"
	}

	query := `
		SELECT organisation_name, cookbook_name, cookbook_version,
		       target_chef_version, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       error_message, scanned_at, created_at
		  FROM server_cookbook_cookstyle_results
		 WHERE organisation_name = $1
		   ` + versionClause + `
	`
	return scanServerCookbookCookstyleResults(q.QueryContext(ctx, query, args...))
}

// ListServerCookbookCookstyleResultsByOrganisation returns all cookstyle
// results for server cookbooks belonging to the given organisation.
func (db *DB) ListServerCookbookCookstyleResultsByOrganisation(ctx context.Context, orgName string) ([]ServerCookbookCookstyleResult, error) {
	return db.listServerCookbookCookstyleResultsByOrganisation(ctx, db.q(), orgName)
}

func (db *DB) listServerCookbookCookstyleResultsByOrganisation(ctx context.Context, q queryable, orgName string) ([]ServerCookbookCookstyleResult, error) {
	const query = `
		SELECT organisation_name, cookbook_name, cookbook_version,
		       target_chef_version, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       error_message, scanned_at, created_at
		  FROM server_cookbook_cookstyle_results
		 WHERE organisation_name = $1
		 ORDER BY cookbook_name, cookbook_version, target_chef_version NULLS FIRST
	`
	return scanServerCookbookCookstyleResults(q.QueryContext(ctx, query, orgName))
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteServerCookbookCookstyleResultsByCookbook removes all cookstyle
// results for the given organisation, cookbook name, and cookbook version.
func (db *DB) DeleteServerCookbookCookstyleResultsByCookbook(ctx context.Context, orgName, cookbookName, cookbookVersion string) error {
	return db.deleteServerCookbookCookstyleResultsByCookbook(ctx, db.q(), orgName, cookbookName, cookbookVersion)
}

func (db *DB) deleteServerCookbookCookstyleResultsByCookbook(ctx context.Context, q queryable, orgName, cookbookName, cookbookVersion string) error {
	const query = `DELETE FROM server_cookbook_cookstyle_results WHERE organisation_name = $1 AND cookbook_name = $2 AND cookbook_version = $3`
	_, err := q.ExecContext(ctx, query, orgName, cookbookName, cookbookVersion)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook cookstyle results for cookbook %s/%s@%s: %w", orgName, cookbookName, cookbookVersion, err)
	}
	return nil
}

// DeleteServerCookbookCookstyleResultsByOrganisation removes all cookstyle
// results for server cookbooks belonging to the given organisation.
func (db *DB) DeleteServerCookbookCookstyleResultsByOrganisation(ctx context.Context, orgName string) error {
	return db.deleteServerCookbookCookstyleResultsByOrganisation(ctx, db.q(), orgName)
}

func (db *DB) deleteServerCookbookCookstyleResultsByOrganisation(ctx context.Context, q queryable, orgName string) error {
	const query = `DELETE FROM server_cookbook_cookstyle_results WHERE organisation_name = $1`
	_, err := q.ExecContext(ctx, query, orgName)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook cookstyle results for organisation %s: %w", orgName, err)
	}
	return nil
}

// DeleteAllServerCookbookCookstyleResults removes all server cookbook
// cookstyle results. This forces a full rescan on the next collection cycle.
func (db *DB) DeleteAllServerCookbookCookstyleResults(ctx context.Context) error {
	return db.deleteAllServerCookbookCookstyleResults(ctx, db.q())
}

func (db *DB) deleteAllServerCookbookCookstyleResults(ctx context.Context, q queryable) error {
	const query = `DELETE FROM server_cookbook_cookstyle_results`
	_, err := q.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("datastore: deleting all server cookbook cookstyle results: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Row scanning helpers
// ---------------------------------------------------------------------------

func scanServerCookbookCookstyleResult(row *sql.Row) (ServerCookbookCookstyleResult, error) {
	var r ServerCookbookCookstyleResult
	var tvOut sql.NullString
	var deprecationWarnings, offences []byte
	var stdout, stderr sql.NullString
	var duration sql.NullInt64
	var errorMessage sql.NullString

	err := row.Scan(
		&r.OrganisationName,
		&r.CookbookName,
		&r.CookbookVersion,
		&tvOut,
		&r.Passed,
		&r.OffenceCount,
		&r.DeprecationCount,
		&r.CorrectnessCount,
		&deprecationWarnings,
		&offences,
		&stdout,
		&stderr,
		&duration,
		&errorMessage,
		&r.ScannedAt,
		&r.CreatedAt,
	)
	if err != nil {
		return ServerCookbookCookstyleResult{}, err
	}

	r.TargetChefVersion = stringFromNull(tvOut)
	r.DeprecationWarnings = deprecationWarnings
	r.Offences = offences
	r.ProcessStdout = stringFromNull(stdout)
	r.ProcessStderr = stringFromNull(stderr)
	r.DurationSeconds = intFromNull(duration)
	r.ErrorMessage = stringFromNull(errorMessage)

	return r, nil
}

func scanServerCookbookCookstyleResults(rows *sql.Rows, err error) ([]ServerCookbookCookstyleResult, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying server cookbook cookstyle results: %w", err)
	}
	defer rows.Close()

	var results []ServerCookbookCookstyleResult
	for rows.Next() {
		var r ServerCookbookCookstyleResult
		var tvOut sql.NullString
		var deprecationWarnings, offences []byte
		var stdout, stderr sql.NullString
		var duration sql.NullInt64
		var errorMessage sql.NullString

		if err := rows.Scan(
			&r.OrganisationName,
			&r.CookbookName,
			&r.CookbookVersion,
			&tvOut,
			&r.Passed,
			&r.OffenceCount,
			&r.DeprecationCount,
			&r.CorrectnessCount,
			&deprecationWarnings,
			&offences,
			&stdout,
			&stderr,
			&duration,
			&errorMessage,
			&r.ScannedAt,
			&r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning server cookbook cookstyle result row: %w", err)
		}

		r.TargetChefVersion = stringFromNull(tvOut)
		r.DeprecationWarnings = deprecationWarnings
		r.Offences = offences
		r.ProcessStdout = stringFromNull(stdout)
		r.ProcessStderr = stringFromNull(stderr)
		r.DurationSeconds = intFromNull(duration)
		r.ErrorMessage = stringFromNull(errorMessage)

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating server cookbook cookstyle results: %w", err)
	}
	return results, nil
}
