// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ConfigEntry represents a single row in the config_store table. Values are
// stored as AES-256-GCM encrypted bytes with a per-row nonce. Encryption and
// decryption are handled by the caller (internal/configstore package) — this
// layer deals only with raw storage.
type ConfigEntry struct {
	// Key is the dot-notation config key (primary key). Examples:
	// "organisations", "collection", "credentials/my-key".
	Key string

	// EncryptedValue is the AES-256-GCM ciphertext (includes GCM auth tag).
	EncryptedValue []byte

	// Nonce is the 12-byte GCM initialisation vector. Unique per write.
	Nonce []byte

	// Secret controls API read behaviour. When true, the decrypted value
	// is never returned via the API (write-only). Used for credentials.
	Secret bool

	// UpdatedAt is the last modification time.
	UpdatedAt time.Time

	// UpdatedBy is the username of the admin who last modified this entry.
	UpdatedBy string
}

// GetConfigEntry retrieves a single config_store row by primary key.
// Returns nil, nil when the key does not exist.
func (db *DB) GetConfigEntry(ctx context.Context, key string) (*ConfigEntry, error) {
	const query = `
		SELECT key, encrypted_value, nonce, secret, updated_at, updated_by
		FROM config_store
		WHERE key = $1`

	var e ConfigEntry
	err := db.pool.QueryRowContext(ctx, query, key).Scan(
		&e.Key,
		&e.EncryptedValue,
		&e.Nonce,
		&e.Secret,
		&e.UpdatedAt,
		&e.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("datastore: get config entry %q: %w", key, err)
	}
	return &e, nil
}

// SetConfigEntry creates or updates a config_store row. On conflict the
// encrypted value, nonce, secret flag, timestamp, and user are replaced.
func (db *DB) SetConfigEntry(ctx context.Context, e *ConfigEntry) error {
	const query = `
		INSERT INTO config_store (key, encrypted_value, nonce, secret, updated_at, updated_by)
		VALUES ($1, $2, $3, $4, NOW(), $5)
		ON CONFLICT (key) DO UPDATE SET
			encrypted_value = EXCLUDED.encrypted_value,
			nonce           = EXCLUDED.nonce,
			secret          = EXCLUDED.secret,
			updated_at      = EXCLUDED.updated_at,
			updated_by      = EXCLUDED.updated_by`

	_, err := db.pool.ExecContext(ctx, query,
		e.Key,
		e.EncryptedValue,
		e.Nonce,
		e.Secret,
		e.UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("datastore: set config entry %q: %w", e.Key, err)
	}
	return nil
}

// DeleteConfigEntry removes a config_store row by key. Returns nil even if
// the key did not exist (idempotent).
func (db *DB) DeleteConfigEntry(ctx context.Context, key string) error {
	const query = `DELETE FROM config_store WHERE key = $1`

	_, err := db.pool.ExecContext(ctx, query, key)
	if err != nil {
		return fmt.Errorf("datastore: delete config entry %q: %w", key, err)
	}
	return nil
}

// ListConfigEntries returns all config_store rows ordered by key.
func (db *DB) ListConfigEntries(ctx context.Context) ([]ConfigEntry, error) {
	const query = `
		SELECT key, encrypted_value, nonce, secret, updated_at, updated_by
		FROM config_store
		ORDER BY key`

	return db.scanConfigEntries(ctx, query)
}

// ListConfigEntriesByPrefix returns config_store rows whose key starts with
// the given prefix, ordered by key. The prefix is matched using LIKE with a
// trailing wildcard. The caller must not include the '%' — it is appended
// automatically.
func (db *DB) ListConfigEntriesByPrefix(ctx context.Context, prefix string) ([]ConfigEntry, error) {
	const query = `
		SELECT key, encrypted_value, nonce, secret, updated_at, updated_by
		FROM config_store
		WHERE key LIKE $1
		ORDER BY key`

	return db.scanConfigEntries(ctx, query, prefix+"%")
}

// CountConfigEntries returns the total number of rows in config_store.
func (db *DB) CountConfigEntries(ctx context.Context) (int, error) {
	const query = `SELECT COUNT(*) FROM config_store`

	var count int
	if err := db.pool.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("datastore: count config entries: %w", err)
	}
	return count, nil
}

// ConfigStoreIsEmpty returns true when config_store contains zero rows. This
// is a convenience wrapper around CountConfigEntries for startup checks.
func (db *DB) ConfigStoreIsEmpty(ctx context.Context) (bool, error) {
	// EXISTS is faster than COUNT(*) for a simple emptiness check.
	const query = `SELECT EXISTS(SELECT 1 FROM config_store LIMIT 1)`

	var exists bool
	if err := db.pool.QueryRowContext(ctx, query).Scan(&exists); err != nil {
		return false, fmt.Errorf("datastore: config store is empty check: %w", err)
	}
	return !exists, nil
}

// scanConfigEntries is an internal helper that executes a query and scans
// the result set into a slice of ConfigEntry. The query must select the
// columns: key, encrypted_value, nonce, secret, updated_at, updated_by.
func (db *DB) scanConfigEntries(ctx context.Context, query string, args ...any) ([]ConfigEntry, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: list config entries: %w", err)
	}
	defer rows.Close()

	var entries []ConfigEntry
	for rows.Next() {
		var e ConfigEntry
		if err := rows.Scan(
			&e.Key,
			&e.EncryptedValue,
			&e.Nonce,
			&e.Secret,
			&e.UpdatedAt,
			&e.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("datastore: scan config entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterate config entries: %w", err)
	}
	return entries, nil
}
