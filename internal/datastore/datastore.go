// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package datastore provides the database access layer for Chef Migration
// Metrics. It manages the PostgreSQL connection pool, runs schema migrations,
// and exposes repository methods for all persisted data.
//
// Other packages must not import database/sql directly — all database access
// is centralised here.
package datastore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors returned by repository methods. Callers should check with
// errors.Is() rather than comparing directly.
var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrInUse         = errors.New("resource is in use")
)

// DB wraps a *sql.DB connection pool and provides repository methods for all
// application tables. Create one with Open() and close it with Close().
type DB struct {
	pool *sql.DB
}

// Open connects to the PostgreSQL database at the given URL, verifies
// connectivity, and returns a ready-to-use DB handle.
//
// The url parameter is a PostgreSQL connection string, e.g.:
//
//	postgres://user:pass@localhost:5432/chef_migration_metrics?sslmode=disable
func Open(url string) (*DB, error) {
	if url == "" {
		return nil, fmt.Errorf("datastore: database URL is empty")
	}

	pool, err := sql.Open("postgres", url)
	if err != nil {
		return nil, fmt.Errorf("datastore: opening database: %w", err)
	}

	// Sensible pool defaults — callers can override via Configure().
	pool.SetMaxOpenConns(25)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(5 * time.Minute)
	pool.SetConnMaxIdleTime(1 * time.Minute)

	// Verify we can actually reach the database.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pool.PingContext(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("datastore: pinging database: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Close releases the connection pool. Call this on application shutdown.
func (db *DB) Close() error {
	if db.pool != nil {
		return db.pool.Close()
	}
	return nil
}

// Pool returns the underlying *sql.DB for use by packages that absolutely
// need it (e.g. the secrets package's DBCredentialStore). Prefer using the
// repository methods on DB whenever possible.
func (db *DB) Pool() *sql.DB {
	return db.pool
}

// Configure adjusts connection pool settings. Call immediately after Open()
// if the defaults are not suitable.
func (db *DB) Configure(maxOpen, maxIdle int, connMaxLifetime, connMaxIdleTime time.Duration) {
	db.pool.SetMaxOpenConns(maxOpen)
	db.pool.SetMaxIdleConns(maxIdle)
	db.pool.SetConnMaxLifetime(connMaxLifetime)
	db.pool.SetConnMaxIdleTime(connMaxIdleTime)
}

// Ping verifies that the database is still reachable. Useful for health
// checks.
func (db *DB) Ping(ctx context.Context) error {
	return db.pool.PingContext(ctx)
}

// ---------------------------------------------------------------------------
// Migrations
// ---------------------------------------------------------------------------

// MigrateUp reads SQL migration files from the given directory on disk and
// applies any that have not yet been run. This is the legacy entry point
// retained for backward compatibility — callers that have an fs.FS (e.g.
// embedded migrations) should use MigrateUpFS instead.
//
// Migration files must follow the naming convention:
//
//	NNNN_short_description.up.sql
//
// Only .up.sql files are applied by this function.
func (db *DB) MigrateUp(ctx context.Context, migrationsDir string) (applied int, err error) {
	return db.MigrateUpFS(ctx, os.DirFS(migrationsDir))
}

// MigrateUpFS reads SQL migration files from the given fs.FS and applies any
// that have not yet been run. Migrations are applied in order of their
// numeric prefix (e.g. 0001, 0002, ...).
//
// The schema_migrations table is created automatically if it does not exist.
// Each migration is applied within a transaction — if a migration fails, it
// is rolled back and the error is returned. Subsequent migrations are not
// attempted.
//
// The fsys parameter can be an embed.FS (for self-contained binaries),
// os.DirFS (for disk-based migrations), or any other fs.FS implementation.
//
// Migration files must follow the naming convention:
//
//	NNNN_short_description.up.sql
//
// Only .up.sql files are applied by this function.
func (db *DB) MigrateUpFS(ctx context.Context, fsys fs.FS) (applied int, err error) {
	if err := db.ensureMigrationsTable(ctx); err != nil {
		return 0, fmt.Errorf("datastore: creating schema_migrations table: %w", err)
	}

	migrations, err := discoverMigrationsFS(fsys)
	if err != nil {
		return 0, fmt.Errorf("datastore: discovering migrations: %w", err)
	}

	if len(migrations) == 0 {
		return 0, nil
	}

	currentVersion, err := db.currentMigrationVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("datastore: reading current migration version: %w", err)
	}

	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}

		if err := db.applyMigrationFS(ctx, fsys, m); err != nil {
			return applied, fmt.Errorf("datastore: applying migration %04d (%s): %w", m.Version, m.Name, err)
		}
		applied++
	}

	return applied, nil
}

// migration represents a single migration file discovered from an fs.FS.
type migration struct {
	Version  int
	Name     string
	Filename string // bare filename (e.g. "0001_initial_schema.up.sql")
}

// discoverMigrationsFS scans the given fs.FS for *.up.sql files and returns
// them sorted by version number. The fs.FS is read at its root — files are
// expected to be at the top level (e.g. "0001_initial_schema.up.sql").
func discoverMigrationsFS(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("reading migrations filesystem: %w", err)
	}

	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		m, err := parseMigrationFilename(name)
		if err != nil {
			// Skip files that don't match the expected naming convention.
			continue
		}
		migrations = append(migrations, m)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	// Check for duplicate version numbers.
	for i := 1; i < len(migrations); i++ {
		if migrations[i].Version == migrations[i-1].Version {
			return nil, fmt.Errorf("duplicate migration version %04d: %s and %s",
				migrations[i].Version, migrations[i-1].Name, migrations[i].Name)
		}
	}

	return migrations, nil
}

// discoverMigrations scans the given directory on disk for *.up.sql files
// and returns them sorted by version number. This is a convenience wrapper
// around discoverMigrationsFS for callers that have a directory path.
//
// It checks that the directory exists before delegating so that the error
// message includes the original path for operator diagnostics.
func discoverMigrations(dir string) ([]migration, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("migrations directory does not exist: %s", dir)
		}
		return nil, fmt.Errorf("checking migrations directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("migrations path is not a directory: %s", dir)
	}
	return discoverMigrationsFS(os.DirFS(dir))
}

// parseMigrationFilename extracts the version number and descriptive name
// from a migration filename like "0001_initial_schema.up.sql".
func parseMigrationFilename(filename string) (migration, error) {
	// Strip the .up.sql suffix.
	base := strings.TrimSuffix(filename, ".up.sql")

	// Split on the first underscore: "0001" + "initial_schema"
	idx := strings.Index(base, "_")
	if idx < 0 {
		return migration{}, fmt.Errorf("migration filename missing underscore separator: %s", filename)
	}

	versionStr := base[:idx]
	name := base[idx+1:]

	version, err := strconv.Atoi(versionStr)
	if err != nil {
		return migration{}, fmt.Errorf("migration filename has non-numeric version prefix: %s", filename)
	}
	if version <= 0 {
		return migration{}, fmt.Errorf("migration version must be positive: %s", filename)
	}

	return migration{
		Version:  version,
		Name:     name,
		Filename: filename,
	}, nil
}

// ensureMigrationsTable creates the schema_migrations table if it does not
// already exist.
func (db *DB) ensureMigrationsTable(ctx context.Context) error {
	_, err := db.pool.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INTEGER     PRIMARY KEY,
			name        TEXT        NOT NULL,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

// currentMigrationVersion returns the highest migration version that has been
// applied, or 0 if no migrations have been run.
func (db *DB) currentMigrationVersion(ctx context.Context) (int, error) {
	var version sql.NullInt64
	err := db.pool.QueryRowContext(ctx,
		`SELECT MAX(version) FROM schema_migrations`,
	).Scan(&version)
	if err != nil {
		return 0, err
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}

// applyMigrationFS reads a migration SQL file from the given fs.FS and
// executes it within a transaction. On success, a row is inserted into
// schema_migrations to record the migration. On failure, the transaction
// is rolled back.
func (db *DB) applyMigrationFS(ctx context.Context, fsys fs.FS, m migration) error {
	sqlBytes, err := fs.ReadFile(fsys, m.Filename)
	if err != nil {
		return fmt.Errorf("reading migration file %s: %w", m.Filename, err)
	}

	sqlStr := string(sqlBytes)
	if strings.TrimSpace(sqlStr) == "" {
		return fmt.Errorf("migration file is empty: %s", m.Filename)
	}

	tx, err := db.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	if _, err := tx.ExecContext(ctx, sqlStr); err != nil {
		return fmt.Errorf("executing SQL: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
		m.Version, m.Name,
	); err != nil {
		return fmt.Errorf("recording migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// MigrationVersion returns the current (highest applied) migration version,
// or 0 if no migrations have been applied. Returns an error if the
// schema_migrations table does not exist.
func (db *DB) MigrationVersion(ctx context.Context) (int, error) {
	return db.currentMigrationVersion(ctx)
}

// ---------------------------------------------------------------------------
// Transaction helper
// ---------------------------------------------------------------------------

// Tx executes fn within a database transaction. If fn returns an error, the
// transaction is rolled back. Otherwise it is committed. The *sql.Tx is
// passed to fn for executing queries within the transaction scope.
func (db *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("datastore: beginning transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("datastore: committing transaction: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Query helpers
// ---------------------------------------------------------------------------

// queryable is satisfied by both *sql.DB and *sql.Tx, allowing repository
// methods to work within or outside a transaction.
type queryable interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// q returns the default queryable (the connection pool).
func (db *DB) q() queryable {
	return db.pool
}

// nullString converts a Go string to sql.NullString. An empty string is
// treated as NULL.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullFloat converts a Go float64 to sql.NullFloat64. A zero value is
// treated as NULL.
func nullFloat(f float64) sql.NullFloat64 {
	if f == 0 {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: f, Valid: true}
}

// nullTime converts a Go time.Time to sql.NullTime. A zero time is treated
// as NULL.
func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// nullInt converts a Go int to sql.NullInt64. A zero value is treated as
// NULL.
func nullInt(i int) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(i), Valid: true}
}

// stringFromNull converts a sql.NullString to a Go string. NULL becomes "".
func stringFromNull(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// floatFromNull converts a sql.NullFloat64 to a Go float64. NULL becomes 0.
func floatFromNull(nf sql.NullFloat64) float64 {
	if nf.Valid {
		return nf.Float64
	}
	return 0
}

// timeFromNull converts a sql.NullTime to a Go time.Time. NULL becomes the
// zero value.
func timeFromNull(nt sql.NullTime) time.Time {
	if nt.Valid {
		return nt.Time
	}
	return time.Time{}
}

// intFromNull converts a sql.NullInt64 to a Go int. NULL becomes 0.
func intFromNull(ni sql.NullInt64) int {
	if ni.Valid {
		return int(ni.Int64)
	}
	return 0
}

// normaliseJSON converts a potentially nil byte slice from a JSONB column
// scan into a json.RawMessage, returning nil when the value is SQL NULL.
func normaliseJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}
