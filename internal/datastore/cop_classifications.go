// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CopClassification represents an operator-assigned migration impact level
// for a specific cop at a specific target Chef version.
type CopClassification struct {
	ID                string    `json:"id"`
	CopName           string    `json:"cop_name"`
	TargetChefVersion string    `json:"target_chef_version"`
	Classification    string    `json:"classification"` // blocker, review, noise
	Reason            string    `json:"reason,omitempty"`
	CreatedBy         string    `json:"created_by,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ListCopClassifications returns all classifications for a given target version.
func (db *DB) ListCopClassifications(ctx context.Context, targetChefVersion string) ([]CopClassification, error) {
	const query = `
		SELECT id, cop_name, target_chef_version, classification, COALESCE(reason, ''), COALESCE(created_by, ''), created_at, updated_at
		FROM cop_classifications
		WHERE target_chef_version = $1
		ORDER BY cop_name`

	rows, err := db.pool.QueryContext(ctx, query, targetChefVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CopClassification
	for rows.Next() {
		var c CopClassification
		if err := rows.Scan(&c.ID, &c.CopName, &c.TargetChefVersion, &c.Classification, &c.Reason, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// GetCopClassification returns the classification for a specific cop and target version.
// Returns nil, nil if no classification exists.
func (db *DB) GetCopClassification(ctx context.Context, copName, targetChefVersion string) (*CopClassification, error) {
	const query = `
		SELECT id, cop_name, target_chef_version, classification, COALESCE(reason, ''), COALESCE(created_by, ''), created_at, updated_at
		FROM cop_classifications
		WHERE cop_name = $1 AND target_chef_version = $2`

	var c CopClassification
	err := db.pool.QueryRowContext(ctx, query, copName, targetChefVersion).Scan(
		&c.ID, &c.CopName, &c.TargetChefVersion, &c.Classification, &c.Reason, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpsertCopClassification creates or updates the classification for a cop at a target version.
func (db *DB) UpsertCopClassification(ctx context.Context, copName, targetChefVersion, classification, reason, createdBy string) error {
	const query = `
		INSERT INTO cop_classifications (cop_name, target_chef_version, classification, reason, created_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (cop_name, target_chef_version) DO UPDATE SET
			classification = EXCLUDED.classification,
			reason = EXCLUDED.reason,
			created_by = EXCLUDED.created_by,
			updated_at = now()`

	_, err := db.pool.ExecContext(ctx, query, copName, targetChefVersion, classification, reason, createdBy)
	return err
}

// DeleteCopClassification removes the operator override for a cop at a target version.
func (db *DB) DeleteCopClassification(ctx context.Context, copName, targetChefVersion string) error {
	const query = `DELETE FROM cop_classifications WHERE cop_name = $1 AND target_chef_version = $2`
	_, err := db.pool.ExecContext(ctx, query, copName, targetChefVersion)
	return err
}
