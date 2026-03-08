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

// Organisation represents a row in the organisations table.
type Organisation struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	ChefServerURL         string    `json:"chef_server_url"`
	OrgName               string    `json:"org_name"`
	ClientName            string    `json:"client_name"`
	ClientKeyCredentialID string    `json:"client_key_credential_id,omitempty"`
	Source                string    `json:"source"` // "config" or "api"
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// MarshalJSON implements json.Marshaler so that Organisation can be serialised
// without pulling in encoding/json at call sites. This is a convenience — the
// struct tags above do the real work.
func (o Organisation) MarshalJSON() ([]byte, error) {
	type Alias Organisation
	return json.Marshal((Alias)(o))
}

// ---------------------------------------------------------------------------
// UpsertOrganisationFromConfig inserts or updates an organisation that
// originates from the configuration file (source = 'config'). The upsert
// key is the organisation name — if a row with the same name already exists
// it is updated; otherwise a new row is created.
//
// This is called during application startup to synchronise the organisations
// table with the current configuration.
// ---------------------------------------------------------------------------

// UpsertOrganisationParams holds the fields required to upsert an
// organisation from configuration.
type UpsertOrganisationParams struct {
	Name                  string
	ChefServerURL         string
	OrgName               string
	ClientName            string
	ClientKeyCredentialID string // optional — empty string means NULL
}

func (db *DB) UpsertOrganisationFromConfig(ctx context.Context, p UpsertOrganisationParams) (Organisation, error) {
	return db.upsertOrganisationFromConfig(ctx, db.q(), p)
}

func (db *DB) upsertOrganisationFromConfig(ctx context.Context, q queryable, p UpsertOrganisationParams) (Organisation, error) {
	if p.Name == "" {
		return Organisation{}, fmt.Errorf("datastore: organisation name is required")
	}
	if p.ChefServerURL == "" {
		return Organisation{}, fmt.Errorf("datastore: chef server URL is required for organisation %q", p.Name)
	}
	if p.OrgName == "" {
		return Organisation{}, fmt.Errorf("datastore: org name is required for organisation %q", p.Name)
	}
	if p.ClientName == "" {
		return Organisation{}, fmt.Errorf("datastore: client name is required for organisation %q", p.Name)
	}

	const query = `
		INSERT INTO organisations (name, chef_server_url, org_name, client_name, client_key_credential_id, source)
		VALUES ($1, $2, $3, $4, $5, 'config')
		ON CONFLICT (name) DO UPDATE SET
			chef_server_url          = EXCLUDED.chef_server_url,
			org_name                 = EXCLUDED.org_name,
			client_name              = EXCLUDED.client_name,
			client_key_credential_id = EXCLUDED.client_key_credential_id,
			source                   = 'config',
			updated_at               = now()
		RETURNING id, name, chef_server_url, org_name, client_name,
		          client_key_credential_id, source, created_at, updated_at
	`

	var credID sql.NullString
	if p.ClientKeyCredentialID != "" {
		credID = sql.NullString{String: p.ClientKeyCredentialID, Valid: true}
	}

	var org Organisation
	var scanCredID sql.NullString
	err := q.QueryRowContext(ctx, query,
		p.Name,
		p.ChefServerURL,
		p.OrgName,
		p.ClientName,
		credID,
	).Scan(
		&org.ID,
		&org.Name,
		&org.ChefServerURL,
		&org.OrgName,
		&org.ClientName,
		&scanCredID,
		&org.Source,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		return Organisation{}, fmt.Errorf("datastore: upserting organisation %q: %w", p.Name, err)
	}
	org.ClientKeyCredentialID = stringFromNull(scanCredID)
	return org, nil
}

// ---------------------------------------------------------------------------
// SyncOrganisationsFromConfig upserts all organisations from the given
// params and removes any config-sourced organisations that are no longer
// present in the configuration. API-sourced organisations are never removed
// by this function.
//
// Returns the full list of organisations after sync.
// ---------------------------------------------------------------------------

func (db *DB) SyncOrganisationsFromConfig(ctx context.Context, params []UpsertOrganisationParams) ([]Organisation, error) {
	var result []Organisation

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		// Upsert each configured organisation.
		configNames := make(map[string]bool, len(params))
		for _, p := range params {
			org, err := db.upsertOrganisationFromConfig(ctx, tx, p)
			if err != nil {
				return err
			}
			result = append(result, org)
			configNames[p.Name] = true
		}

		// Remove config-sourced organisations that are no longer in the config.
		existing, err := db.listOrganisationsBySource(ctx, tx, "config")
		if err != nil {
			return err
		}
		for _, org := range existing {
			if !configNames[org.Name] {
				if _, err := tx.ExecContext(ctx,
					`DELETE FROM organisations WHERE id = $1`, org.ID,
				); err != nil {
					return fmt.Errorf("datastore: removing stale organisation %q: %w", org.Name, err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Query methods
// ---------------------------------------------------------------------------

// GetOrganisation returns the organisation with the given UUID. Returns
// ErrNotFound if no such organisation exists.
func (db *DB) GetOrganisation(ctx context.Context, id string) (Organisation, error) {
	return db.getOrganisation(ctx, db.q(), id)
}

func (db *DB) getOrganisation(ctx context.Context, q queryable, id string) (Organisation, error) {
	const query = `
		SELECT id, name, chef_server_url, org_name, client_name,
		       client_key_credential_id, source, created_at, updated_at
		FROM organisations
		WHERE id = $1
	`
	return scanOrganisation(q.QueryRowContext(ctx, query, id))
}

// GetOrganisationByName returns the organisation with the given name.
// Returns ErrNotFound if no such organisation exists.
func (db *DB) GetOrganisationByName(ctx context.Context, name string) (Organisation, error) {
	return db.getOrganisationByName(ctx, db.q(), name)
}

func (db *DB) getOrganisationByName(ctx context.Context, q queryable, name string) (Organisation, error) {
	const query = `
		SELECT id, name, chef_server_url, org_name, client_name,
		       client_key_credential_id, source, created_at, updated_at
		FROM organisations
		WHERE name = $1
	`
	return scanOrganisation(q.QueryRowContext(ctx, query, name))
}

// ListOrganisations returns all organisations ordered by name.
func (db *DB) ListOrganisations(ctx context.Context) ([]Organisation, error) {
	return db.listOrganisations(ctx, db.q())
}

func (db *DB) listOrganisations(ctx context.Context, q queryable) ([]Organisation, error) {
	const query = `
		SELECT id, name, chef_server_url, org_name, client_name,
		       client_key_credential_id, source, created_at, updated_at
		FROM organisations
		ORDER BY name
	`
	return scanOrganisations(q.QueryContext(ctx, query))
}

// listOrganisationsBySource returns all organisations with the given source
// value. Used internally by SyncOrganisationsFromConfig.
func (db *DB) listOrganisationsBySource(ctx context.Context, q queryable, source string) ([]Organisation, error) {
	const query = `
		SELECT id, name, chef_server_url, org_name, client_name,
		       client_key_credential_id, source, created_at, updated_at
		FROM organisations
		WHERE source = $1
		ORDER BY name
	`
	return scanOrganisations(q.QueryContext(ctx, query, source))
}

// DeleteOrganisation removes the organisation with the given UUID. Returns
// ErrNotFound if no such organisation exists. Cascading deletes will remove
// associated collection runs, node snapshots, cookbooks, and other dependent
// data.
func (db *DB) DeleteOrganisation(ctx context.Context, id string) error {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM organisations WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("datastore: deleting organisation: %w", err)
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

// scanOrganisation scans a single row into an Organisation. Returns
// ErrNotFound when the row contains sql.ErrNoRows.
func scanOrganisation(row *sql.Row) (Organisation, error) {
	var org Organisation
	var credID sql.NullString
	err := row.Scan(
		&org.ID,
		&org.Name,
		&org.ChefServerURL,
		&org.OrgName,
		&org.ClientName,
		&credID,
		&org.Source,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Organisation{}, ErrNotFound
		}
		return Organisation{}, fmt.Errorf("datastore: scanning organisation: %w", err)
	}
	org.ClientKeyCredentialID = stringFromNull(credID)
	return org, nil
}

// scanOrganisations scans multiple rows into a slice of Organisation.
func scanOrganisations(rows *sql.Rows, err error) ([]Organisation, error) {
	if err != nil {
		return nil, fmt.Errorf("datastore: querying organisations: %w", err)
	}
	defer rows.Close()

	var orgs []Organisation
	for rows.Next() {
		var org Organisation
		var credID sql.NullString
		if err := rows.Scan(
			&org.ID,
			&org.Name,
			&org.ChefServerURL,
			&org.OrgName,
			&org.ClientName,
			&credID,
			&org.Source,
			&org.CreatedAt,
			&org.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("datastore: scanning organisation row: %w", err)
		}
		org.ClientKeyCredentialID = stringFromNull(credID)
		orgs = append(orgs, org)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating organisation rows: %w", err)
	}
	return orgs, nil
}
