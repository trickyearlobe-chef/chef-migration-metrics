// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ServerCookbookCookstyleResult represents a row in the
// server_cookbook_cookstyle_results table. Server cookbooks are immutable
// snapshots fetched from a Chef Server, so there is no CommitSHA field.
type ServerCookbookCookstyleResult struct {
	ID                  string    `json:"id"`
	ServerCookbookID    string    `json:"server_cookbook_id"`
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
	ScannedAt           time.Time `json:"scanned_at"`
	CreatedAt           time.Time `json:"created_at"`
}

// UpsertServerCookbookCookstyleResultParams contains the fields needed to
// insert or update a server_cookbook_cookstyle_results row. The unique
// constraint is (server_cookbook_id, target_chef_version).
type UpsertServerCookbookCookstyleResultParams struct {
	ServerCookbookID    string
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
	ScannedAt           time.Time
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

// UpsertServerCookbookCookstyleResult inserts a new cookstyle result or
// updates the existing one for the same (server_cookbook_id, target_chef_version)
// combination. Returns the resulting row.
func (db *DB) UpsertServerCookbookCookstyleResult(ctx context.Context, p UpsertServerCookbookCookstyleResultParams) (*ServerCookbookCookstyleResult, error) {
	return db.upsertServerCookbookCookstyleResult(ctx, db.q(), p)
}

func (db *DB) upsertServerCookbookCookstyleResult(ctx context.Context, q queryable, p UpsertServerCookbookCookstyleResultParams) (*ServerCookbookCookstyleResult, error) {
	const query = `
		INSERT INTO server_cookbook_cookstyle_results (
			server_cookbook_id, target_chef_version, passed,
			offence_count, deprecation_count, correctness_count,
			deprecation_warnings, offences,
			process_stdout, process_stderr, duration_seconds,
			scanned_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (server_cookbook_id, target_chef_version)
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
			scanned_at          = EXCLUDED.scanned_at
		RETURNING id, server_cookbook_id, target_chef_version, passed,
		          offence_count, deprecation_count, correctness_count,
		          deprecation_warnings, offences,
		          process_stdout, process_stderr, duration_seconds,
		          scanned_at, created_at
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

	err := q.QueryRowContext(ctx, query,
		p.ServerCookbookID,
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
		p.ScannedAt,
	).Scan(
		&r.ID,
		&r.ServerCookbookID,
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

	return r, nil
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// GetServerCookbookCookstyleResult returns the cookstyle result for the given
// server cookbook ID and target Chef version. Returns (nil, nil) if no result
// exists.
func (db *DB) GetServerCookbookCookstyleResult(ctx context.Context, serverCookbookID, targetChefVersion string) (*ServerCookbookCookstyleResult, error) {
	return db.getServerCookbookCookstyleResult(ctx, db.q(), serverCookbookID, targetChefVersion)
}

func (db *DB) getServerCookbookCookstyleResult(ctx context.Context, q queryable, serverCookbookID, targetChefVersion string) (*ServerCookbookCookstyleResult, error) {
	const query = `
		SELECT id, server_cookbook_id, target_chef_version, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM server_cookbook_cookstyle_results
		 WHERE server_cookbook_id = $1
		   AND (target_chef_version = $2 OR ($2 = '' AND target_chef_version IS NULL))
	`

	var targetVersion sql.NullString
	if targetChefVersion != "" {
		targetVersion = sql.NullString{String: targetChefVersion, Valid: true}
	}

	r, err := scanServerCookbookCookstyleResult(q.QueryRowContext(ctx, query, serverCookbookID, targetVersion))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting server cookbook cookstyle result: %w", err)
	}
	return &r, nil
}

// GetServerCookbookCookstyleResultByID returns a single cookstyle result by
// its primary key. Returns ErrNotFound if no such result exists.
func (db *DB) GetServerCookbookCookstyleResultByID(ctx context.Context, id string) (*ServerCookbookCookstyleResult, error) {
	return db.getServerCookbookCookstyleResultByID(ctx, db.q(), id)
}

func (db *DB) getServerCookbookCookstyleResultByID(ctx context.Context, q queryable, id string) (*ServerCookbookCookstyleResult, error) {
	const query = `
		SELECT id, server_cookbook_id, target_chef_version, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM server_cookbook_cookstyle_results
		 WHERE id = $1
	`

	r, err := scanServerCookbookCookstyleResult(q.QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: getting server cookbook cookstyle result by id: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListServerCookbookCookstyleResults returns all cookstyle results for the
// given server cookbook ID, ordered by target_chef_version.
func (db *DB) ListServerCookbookCookstyleResults(ctx context.Context, serverCookbookID string) ([]ServerCookbookCookstyleResult, error) {
	return db.listServerCookbookCookstyleResults(ctx, db.q(), serverCookbookID)
}

func (db *DB) listServerCookbookCookstyleResults(ctx context.Context, q queryable, serverCookbookID string) ([]ServerCookbookCookstyleResult, error) {
	const query = `
		SELECT id, server_cookbook_id, target_chef_version, passed,
		       offence_count, deprecation_count, correctness_count,
		       deprecation_warnings, offences,
		       process_stdout, process_stderr, duration_seconds,
		       scanned_at, created_at
		  FROM server_cookbook_cookstyle_results
		 WHERE server_cookbook_id = $1
		 ORDER BY target_chef_version NULLS FIRST
	`
	return scanServerCookbookCookstyleResults(q.QueryContext(ctx, query, serverCookbookID))
}

// ListServerCookbookCookstyleResultsByOrganisation returns all cookstyle
// results for server cookbooks belonging to the given organisation.
func (db *DB) ListServerCookbookCookstyleResultsByOrganisation(ctx context.Context, organisationID string) ([]ServerCookbookCookstyleResult, error) {
	return db.listServerCookbookCookstyleResultsByOrganisation(ctx, db.q(), organisationID)
}

func (db *DB) listServerCookbookCookstyleResultsByOrganisation(ctx context.Context, q queryable, organisationID string) ([]ServerCookbookCookstyleResult, error) {
	const query = `
		SELECT r.id, r.server_cookbook_id, r.target_chef_version, r.passed,
		       r.offence_count, r.deprecation_count, r.correctness_count,
		       r.deprecation_warnings, r.offences,
		       r.process_stdout, r.process_stderr, r.duration_seconds,
		       r.scanned_at, r.created_at
		  FROM server_cookbook_cookstyle_results r
		  JOIN server_cookbooks sc ON sc.id = r.server_cookbook_id
		 WHERE sc.organisation_id = $1
		 ORDER BY sc.name, sc.version, r.target_chef_version NULLS FIRST
	`
	return scanServerCookbookCookstyleResults(q.QueryContext(ctx, query, organisationID))
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

// DeleteServerCookbookCookstyleResultsByCookbook removes all cookstyle
// results for the given server cookbook ID.
func (db *DB) DeleteServerCookbookCookstyleResultsByCookbook(ctx context.Context, serverCookbookID string) error {
	return db.deleteServerCookbookCookstyleResultsByCookbook(ctx, db.q(), serverCookbookID)
}

func (db *DB) deleteServerCookbookCookstyleResultsByCookbook(ctx context.Context, q queryable, serverCookbookID string) error {
	const query = `DELETE FROM server_cookbook_cookstyle_results WHERE server_cookbook_id = $1`
	_, err := q.ExecContext(ctx, query, serverCookbookID)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook cookstyle results for cookbook %s: %w", serverCookbookID, err)
	}
	return nil
}

// DeleteServerCookbookCookstyleResultsByOrganisation removes all cookstyle
// results for server cookbooks belonging to the given organisation.
func (db *DB) DeleteServerCookbookCookstyleResultsByOrganisation(ctx context.Context, organisationID string) error {
	return db.deleteServerCookbookCookstyleResultsByOrganisation(ctx, db.q(), organisationID)
}

func (db *DB) deleteServerCookbookCookstyleResultsByOrganisation(ctx context.Context, q queryable, organisationID string) error {
	const query = `
		DELETE FROM server_cookbook_cookstyle_results
		 WHERE server_cookbook_id IN (
			SELECT id FROM server_cookbooks WHERE organisation_id = $1
		 )
	`
	_, err := q.ExecContext(ctx, query, organisationID)
	if err != nil {
		return fmt.Errorf("datastore: deleting server cookbook cookstyle results for organisation %s: %w", organisationID, err)
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

	err := row.Scan(
		&r.ID,
		&r.ServerCookbookID,
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

		if err := rows.Scan(
			&r.ID,
			&r.ServerCookbookID,
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

		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating server cookbook cookstyle results: %w", err)
	}
	return results, nil
}
