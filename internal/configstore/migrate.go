// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// LegacyDB abstracts the database operations needed to read from the legacy
// runtime_settings table during migration. This interface is satisfied by
// *sql.DB and test fakes.
type LegacyDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
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
	RuntimeSettingsMigrated int
	Skipped                 bool
	SkipReason              string
}

// MigrateFromLegacy migrates data from the legacy runtime_settings table into
// the unified config_store table. The migration is idempotent — if config_store
// already has entries, migration is skipped.
func MigrateFromLegacy(ctx context.Context, legacyDB LegacyDB, store *Store) (*MigrateResult, error) {
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

	// Migrate runtime_settings.
	settingsCount, err := migrateRuntimeSettings(ctx, legacyDB, store)
	if err != nil {
		return nil, fmt.Errorf("migrate: runtime_settings: %w", err)
	}
	result.RuntimeSettingsMigrated = settingsCount

	return result, nil
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

// queryLegacyRuntimeSettings reads all rows from the legacy runtime_settings
// table. Returns an empty slice (not an error) when the table no longer exists,
// which happens after migration 0025 drops it.
func queryLegacyRuntimeSettings(ctx context.Context, db LegacyDB) ([]legacyRuntimeSettingRow, error) {
	query := `
		SELECT key, value, updated_at, updated_by
		FROM runtime_settings
		ORDER BY key`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		// pg error 42P01 = undefined_table. The table was dropped by migration
		// 0025 — treat this as "no rows to migrate" rather than a fatal error.
		if isUndefinedTable(err) {
			return nil, nil
		}
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

// isUndefinedTable reports whether err is a PostgreSQL "undefined_table"
// error (SQLSTATE 42P01).
func isUndefinedTable(err error) bool {
	type sqlstater interface{ SQLState() string }
	var se sqlstater
	if errors.As(err, &se) {
		return se.SQLState() == "42P01"
	}
	return false
}
