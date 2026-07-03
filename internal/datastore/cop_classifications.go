// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CopClassification represents an operator-assigned migration impact level for
// a cop. There is a single active target Chef version, so classifications are
// keyed by cop_name only.
type CopClassification struct {
	ID             string    `json:"id"`
	CopName        string    `json:"cop_name"`
	Classification string    `json:"classification"` // blocker, review, noise
	Reason         string    `json:"reason,omitempty"`
	CreatedBy      string    `json:"created_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ListCopClassifications returns all operator classification overrides.
func (db *DB) ListCopClassifications(ctx context.Context) ([]CopClassification, error) {
	const query = `
		SELECT id, cop_name, classification, COALESCE(reason, ''), COALESCE(created_by, ''), created_at, updated_at
		FROM cop_classifications
		ORDER BY cop_name`

	rows, err := db.pool.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CopClassification
	for rows.Next() {
		var c CopClassification
		if err := rows.Scan(&c.ID, &c.CopName, &c.Classification, &c.Reason, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// GetCopClassification returns the classification for a specific cop.
// Returns nil, nil if no classification exists.
func (db *DB) GetCopClassification(ctx context.Context, copName string) (*CopClassification, error) {
	const query = `
		SELECT id, cop_name, classification, COALESCE(reason, ''), COALESCE(created_by, ''), created_at, updated_at
		FROM cop_classifications
		WHERE cop_name = $1`

	var c CopClassification
	err := db.pool.QueryRowContext(ctx, query, copName).Scan(
		&c.ID, &c.CopName, &c.Classification, &c.Reason, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpsertCopClassification creates or updates the classification for a cop.
func (db *DB) UpsertCopClassification(ctx context.Context, copName, classification, reason, createdBy string) error {
	const query = `
		INSERT INTO cop_classifications (cop_name, classification, reason, created_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (cop_name) DO UPDATE SET
			classification = EXCLUDED.classification,
			reason = EXCLUDED.reason,
			created_by = EXCLUDED.created_by,
			updated_at = now()`

	_, err := db.pool.ExecContext(ctx, query, copName, classification, reason, createdBy)
	return err
}

// DeleteCopClassification removes the operator override for a cop.
func (db *DB) DeleteCopClassification(ctx context.Context, copName string) error {
	const query = `DELETE FROM cop_classifications WHERE cop_name = $1`
	_, err := db.pool.ExecContext(ctx, query, copName)
	return err
}
