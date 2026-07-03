// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CustomCopDefinition represents an operator-defined pattern matcher for
// issues not covered by cookstyle. Scanned during analysis alongside cookstyle.
type CustomCopDefinition struct {
	ID                   string    `json:"id"`
	CopName              string    `json:"cop_name"`
	Description          string    `json:"description"`
	PatternType          string    `json:"pattern_type"` // regex, literal
	Pattern              string    `json:"pattern"`
	FileGlob             string    `json:"file_glob"`
	TargetChefVersionMin string    `json:"target_chef_version_min,omitempty"`
	RemovedIn            string    `json:"removed_in,omitempty"`
	Classification       string    `json:"classification"` // blocker, review, noise
	Enabled              bool      `json:"enabled"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// ListCustomCopDefinitions returns all custom cop definitions.
func (db *DB) ListCustomCopDefinitions(ctx context.Context) ([]CustomCopDefinition, error) {
	const query = `
		SELECT id, cop_name, description, pattern_type, pattern, file_glob,
			COALESCE(target_chef_version_min, ''), COALESCE(removed_in, ''),
			classification, enabled, created_at, updated_at
		FROM custom_cop_definitions
		ORDER BY cop_name`

	rows, err := db.pool.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CustomCopDefinition
	for rows.Next() {
		var d CustomCopDefinition
		if err := rows.Scan(
			&d.ID, &d.CopName, &d.Description, &d.PatternType, &d.Pattern, &d.FileGlob,
			&d.TargetChefVersionMin, &d.RemovedIn, &d.Classification, &d.Enabled, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// ListEnabledCustomCopDefinitions returns only enabled custom cop definitions.
func (db *DB) ListEnabledCustomCopDefinitions(ctx context.Context) ([]CustomCopDefinition, error) {
	const query = `
		SELECT id, cop_name, description, pattern_type, pattern, file_glob,
			COALESCE(target_chef_version_min, ''), COALESCE(removed_in, ''),
			classification, enabled, created_at, updated_at
		FROM custom_cop_definitions
		WHERE enabled = true
		ORDER BY cop_name`

	rows, err := db.pool.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CustomCopDefinition
	for rows.Next() {
		var d CustomCopDefinition
		if err := rows.Scan(
			&d.ID, &d.CopName, &d.Description, &d.PatternType, &d.Pattern, &d.FileGlob,
			&d.TargetChefVersionMin, &d.RemovedIn, &d.Classification, &d.Enabled, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// GetCustomCopDefinition returns a custom cop definition by cop_name.
func (db *DB) GetCustomCopDefinition(ctx context.Context, copName string) (*CustomCopDefinition, error) {
	const query = `
		SELECT id, cop_name, description, pattern_type, pattern, file_glob,
			COALESCE(target_chef_version_min, ''), COALESCE(removed_in, ''),
			classification, enabled, created_at, updated_at
		FROM custom_cop_definitions
		WHERE cop_name = $1`

	var d CustomCopDefinition
	err := db.pool.QueryRowContext(ctx, query, copName).Scan(
		&d.ID, &d.CopName, &d.Description, &d.PatternType, &d.Pattern, &d.FileGlob,
		&d.TargetChefVersionMin, &d.RemovedIn, &d.Classification, &d.Enabled, &d.CreatedAt, &d.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// CreateCustomCopDefinition inserts a new custom cop definition and returns
// the generated UUID.
func (db *DB) CreateCustomCopDefinition(ctx context.Context, d CustomCopDefinition) (string, error) {
	const query = `
		INSERT INTO custom_cop_definitions (cop_name, description, pattern_type, pattern, file_glob, target_chef_version_min, removed_in, classification, enabled)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), $8, $9)
		RETURNING id`

	var id string
	err := db.pool.QueryRowContext(ctx, query,
		d.CopName, d.Description, d.PatternType, d.Pattern, d.FileGlob,
		d.TargetChefVersionMin, d.RemovedIn, d.Classification, d.Enabled,
	).Scan(&id)
	return id, err
}

// UpdateCustomCopDefinition updates an existing custom cop definition by cop_name.
func (db *DB) UpdateCustomCopDefinition(ctx context.Context, d *CustomCopDefinition) error {
	const query = `
		UPDATE custom_cop_definitions SET
			description = $2,
			pattern_type = $3,
			pattern = $4,
			file_glob = $5,
			target_chef_version_min = NULLIF($6, ''),
			removed_in = NULLIF($7, ''),
			classification = $8,
			enabled = $9,
			updated_at = now()
		WHERE cop_name = $1`

	_, err := db.pool.ExecContext(ctx, query,
		d.CopName, d.Description, d.PatternType, d.Pattern, d.FileGlob,
		d.TargetChefVersionMin, d.RemovedIn, d.Classification, d.Enabled,
	)
	return err
}

// DeleteCustomCopDefinition removes a custom cop definition by cop_name.
func (db *DB) DeleteCustomCopDefinition(ctx context.Context, copName string) error {
	const query = `DELETE FROM custom_cop_definitions WHERE cop_name = $1`
	_, err := db.pool.ExecContext(ctx, query, copName)
	return err
}
