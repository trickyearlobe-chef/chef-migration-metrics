// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RuntimeSetting represents a single runtime configuration override.
type RuntimeSetting struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	UpdatedAt time.Time       `json:"updated_at"`
	UpdatedBy string          `json:"updated_by"`
}

// GetRuntimeSetting retrieves a runtime setting by key. Returns nil, nil when
// the key does not exist.
func (db *DB) GetRuntimeSetting(ctx context.Context, key string) (*RuntimeSetting, error) {
	const query = `SELECT key, value, updated_at, updated_by FROM runtime_settings WHERE key = $1`

	var s RuntimeSetting
	err := db.pool.QueryRowContext(ctx, query, key).Scan(&s.Key, &s.Value, &s.UpdatedAt, &s.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: get runtime setting %q: %w", key, err)
	}
	return &s, nil
}

// SetRuntimeSetting creates or updates a runtime setting. The value must be
// valid JSON.
func (db *DB) SetRuntimeSetting(ctx context.Context, key string, value json.RawMessage, updatedBy string) error {
	const query = `
		INSERT INTO runtime_settings (key, value, updated_at, updated_by)
		VALUES ($1, $2, NOW(), $3)
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			updated_at = EXCLUDED.updated_at,
			updated_by = EXCLUDED.updated_by`

	_, err := db.pool.ExecContext(ctx, query, key, value, updatedBy)
	if err != nil {
		return fmt.Errorf("datastore: set runtime setting %q: %w", key, err)
	}
	return nil
}

// DeleteRuntimeSetting removes a runtime setting by key. Returns nil even if
// the key did not exist (idempotent).
func (db *DB) DeleteRuntimeSetting(ctx context.Context, key string) error {
	const query = `DELETE FROM runtime_settings WHERE key = $1`

	_, err := db.pool.ExecContext(ctx, query, key)
	if err != nil {
		return fmt.Errorf("datastore: delete runtime setting %q: %w", key, err)
	}
	return nil
}
