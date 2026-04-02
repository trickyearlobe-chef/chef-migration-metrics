// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// LegacyDB abstracts the database operations needed to read from the legacy
// credentials and runtime_settings tables during migration. This interface
// is satisfied by *sql.DB and test fakes.
type LegacyDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// legacyCredentialRow holds a single row from the legacy credentials table.
type legacyCredentialRow struct {
	Name           string
	CredentialType string
	EncryptedValue string // hex-encoded nonce:ciphertext format
	Metadata       []byte // JSONB, may be nil
	LastRotatedAt  sql.NullTime
	CreatedBy      string
	UpdatedBy      sql.NullString
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// legacyRuntimeSettingRow holds a single row from the legacy runtime_settings table.
type legacyRuntimeSettingRow struct {
	Key       string
	Value     json.RawMessage
	UpdatedAt time.Time
	UpdatedBy string
}

// MigrateResult records the outcome of a legacy data migration.
type MigrateResult struct {
	CredentialsMigrated     int
	RuntimeSettingsMigrated int
	Skipped                 bool
	SkipReason              string
}

// MigrateFromLegacy migrates data from the legacy credentials and
// runtime_settings tables into the unified config_store table. Each legacy
// credential is re-encrypted under the new AAD scheme (key name instead of
// credential_type:name).
//
// The migration is idempotent — if config_store already contains entries
// with the credentials/ prefix, credential migration is skipped. Similarly,
// runtime_settings migration is skipped if matching keys already exist.
//
// The legacy tables are NOT dropped — that happens in a future migration
// after the release is validated.
//
// The oldEncryptor is used to decrypt values from the legacy credentials
// table (which uses the hex nonce:ciphertext format with credential_type:name
// AAD). The store's own encryption is used to re-encrypt into the new format.
func MigrateFromLegacy(ctx context.Context, legacyDB LegacyDB, store *Store, oldEncryptor *secrets.Encryptor) (*MigrateResult, error) {
	result := &MigrateResult{}

	// Check if config_store already has entries — if so, migration was
	// already performed.
	empty, err := store.IsEmpty(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: check config_store empty: %w", err)
	}
	if !empty {
		result.Skipped = true
		result.SkipReason = "config_store already has entries"
		return result, nil
	}

	// Migrate credentials.
	credCount, err := migrateCredentials(ctx, legacyDB, store, oldEncryptor)
	if err != nil {
		return nil, fmt.Errorf("migrate: credentials: %w", err)
	}
	result.CredentialsMigrated = credCount

	// Migrate runtime_settings.
	settingsCount, err := migrateRuntimeSettings(ctx, legacyDB, store)
	if err != nil {
		return nil, fmt.Errorf("migrate: runtime_settings: %w", err)
	}
	result.RuntimeSettingsMigrated = settingsCount

	return result, nil
}

// migrateCredentials reads all rows from the legacy credentials table,
// decrypts each using the old AAD scheme, and re-encrypts into config_store
// with key "credentials/<name>" and secret=true.
func migrateCredentials(ctx context.Context, legacyDB LegacyDB, store *Store, oldEncryptor *secrets.Encryptor) (int, error) {
	if oldEncryptor == nil {
		// No encryptor means no credentials can be decrypted. Check if
		// the table has any rows — if empty, that's fine.
		count, err := countLegacyCredentials(ctx, legacyDB)
		if err != nil {
			return 0, fmt.Errorf("count legacy credentials: %w", err)
		}
		if count > 0 {
			return 0, fmt.Errorf("legacy credentials table has %d rows but no encryption key is available to decrypt them", count)
		}
		return 0, nil
	}

	rows, err := queryLegacyCredentials(ctx, legacyDB)
	if err != nil {
		return 0, err
	}

	migrated := 0
	for _, row := range rows {
		if err := migrateOneCredential(ctx, store, oldEncryptor, row); err != nil {
			return migrated, fmt.Errorf("credential %q: %w", row.Name, err)
		}
		migrated++
	}

	return migrated, nil
}

// migrateOneCredential decrypts a single legacy credential row using the
// old AAD scheme and re-encrypts it into config_store.
func migrateOneCredential(ctx context.Context, store *Store, oldEncryptor *secrets.Encryptor, row legacyCredentialRow) error {
	// Build old-style AAD: "credential_type:name"
	oldAAD, err := secrets.BuildAAD(row.CredentialType, row.Name)
	if err != nil {
		return fmt.Errorf("build old AAD: %w", err)
	}

	// Decrypt using the old hex nonce:ciphertext format.
	plaintext, err := oldEncryptor.Decrypt(row.EncryptedValue, oldAAD)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}
	defer secrets.ZeroBytes(plaintext)

	// Parse metadata from JSONB.
	var metadata map[string]any
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			// Non-fatal — store with nil metadata.
			metadata = nil
		}
	}

	// Build the credential envelope that will be stored in config_store.
	env := credentialEnvelope{
		CredentialType: row.CredentialType,
		Value:          plaintext,
		Metadata:       metadata,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt,
	}
	if row.LastRotatedAt.Valid {
		t := row.LastRotatedAt.Time
		env.LastRotatedAt = &t
	}
	if row.UpdatedBy.Valid {
		env.UpdatedBy = row.UpdatedBy.String
	}

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	// Store in config_store with key "credentials/<name>", secret=true.
	key := credentialKeyPrefix + row.Name
	updatedBy := "migration"
	if row.UpdatedBy.Valid && row.UpdatedBy.String != "" {
		updatedBy = row.UpdatedBy.String
	}

	if err := store.Set(ctx, key, json.RawMessage(data), true, updatedBy); err != nil {
		return fmt.Errorf("store set: %w", err)
	}

	return nil
}

// migrateRuntimeSettings reads all rows from the legacy runtime_settings
// table and encrypts them into config_store with secret=false.
func migrateRuntimeSettings(ctx context.Context, legacyDB LegacyDB, store *Store) (int, error) {
	rows, err := queryLegacyRuntimeSettings(ctx, legacyDB)
	if err != nil {
		return 0, err
	}

	migrated := 0
	for _, row := range rows {
		if err := store.Set(ctx, row.Key, row.Value, false, row.UpdatedBy); err != nil {
			return migrated, fmt.Errorf("runtime setting %q: %w", row.Key, err)
		}
		migrated++
	}

	return migrated, nil
}

// queryLegacyCredentials reads all rows from the legacy credentials table.
func queryLegacyCredentials(ctx context.Context, db LegacyDB) ([]legacyCredentialRow, error) {
	query := `
		SELECT name, credential_type, encrypted_value, metadata,
		       last_rotated_at, created_by, updated_by, created_at, updated_at
		FROM credentials
		ORDER BY name`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query legacy credentials: %w", err)
	}
	defer rows.Close()

	var result []legacyCredentialRow
	for rows.Next() {
		var r legacyCredentialRow
		if err := rows.Scan(
			&r.Name, &r.CredentialType, &r.EncryptedValue, &r.Metadata,
			&r.LastRotatedAt, &r.CreatedBy, &r.UpdatedBy, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan legacy credential: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate legacy credentials: %w", err)
	}

	return result, nil
}

// queryLegacyRuntimeSettings reads all rows from the legacy runtime_settings table.
func queryLegacyRuntimeSettings(ctx context.Context, db LegacyDB) ([]legacyRuntimeSettingRow, error) {
	query := `
		SELECT key, value, updated_at, updated_by
		FROM runtime_settings
		ORDER BY key`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query legacy runtime_settings: %w", err)
	}
	defer rows.Close()

	var result []legacyRuntimeSettingRow
	for rows.Next() {
		var r legacyRuntimeSettingRow
		if err := rows.Scan(&r.Key, &r.Value, &r.UpdatedAt, &r.UpdatedBy); err != nil {
			return nil, fmt.Errorf("scan legacy runtime setting: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate legacy runtime_settings: %w", err)
	}

	return result, nil
}

// countLegacyCredentials returns the number of rows in the legacy credentials
// table. Used to determine whether an encryption key is required.
func countLegacyCredentials(ctx context.Context, db LegacyDB) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT COUNT(*) FROM credentials`)
	if err != nil {
		return 0, fmt.Errorf("count legacy credentials: %w", err)
	}
	defer rows.Close()

	var count int
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, fmt.Errorf("scan legacy credential count: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate legacy credential count: %w", err)
	}

	return count, nil
}
