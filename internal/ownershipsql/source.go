// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package ownershipsql reads ownership rows from a database, for administrators
// whose owner list lives in a system of record rather than a file.
//
// It exists as its own package because internal/ownershipimport is deliberately
// pure — no database access — so the mapping and reporting logic stays testable
// without one. This package supplies an ownershipimport.RowSource and nothing
// else; everything above the source abstraction (the row cap, the value filter,
// the distinct-value cap, report truncation) applies to a query result exactly
// as it applies to a file, with no change.
//
// The source is a streaming cursor. It is single-pass and cannot be rewound,
// which is why profile, preview and commit each open their own.
package ownershipsql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	// Both are first-class sources an administrator can choose, not one real
	// option and one for testing. The package registers both rather than
	// relying on whatever the binary happens to import.
	_ "github.com/lib/pq"
	_ "github.com/microsoft/go-mssqldb"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipimport"
)

// The database types an ownership query can be read from, as they appear in the
// API and the UI. Both are supported for real use.
const (
	DriverSQLServer = "sqlserver"
	DriverPostgres  = "postgres"
)

// SupportedDrivers lists what may be chosen, so the API can reject anything
// else rather than handing an arbitrary string to database/sql.
var SupportedDrivers = []string{DriverSQLServer, DriverPostgres}

// IsSupportedDriver reports whether the named driver may be opened.
func IsSupportedDriver(name string) bool {
	for _, d := range SupportedDrivers {
		if d == name {
			return true
		}
	}
	return false
}

// Config describes one ownership query.
type Config struct {
	// Driver is one of SupportedDrivers.
	Driver string
	// DSN is the connection string, including credentials. It is never logged
	// and never returned to a client.
	DSN string
	// Query is the SELECT to read rows from. It is supplied by an
	// administrator, and is run as-is under whatever the connection's own
	// permissions allow.
	Query string
	// ConnectTimeout bounds establishing the connection. Zero means 10s.
	ConnectTimeout time.Duration
}

// sqlSource adapts a query result to ownershipimport.RowSource.
type sqlSource struct {
	db      *sql.DB
	rows    *sql.Rows
	columns []string
	scan    []any
	ptrs    []any
	row     ownershipimport.Row
	number  int
	err     error
}

// Open connects, runs the query and returns a source positioned before the
// first row. The caller must Close it.
//
// The connection is verified before the query runs, so a bad host or a refused
// login is reported as such rather than as an empty result — an unreadable
// source and an empty one read the same on screen and mean opposite things.
func Open(ctx context.Context, cfg Config) (ownershipimport.RowSource, error) {
	if !IsSupportedDriver(cfg.Driver) {
		return nil, fmt.Errorf("ownershipsql: unsupported driver %q", cfg.Driver)
	}
	if strings.TrimSpace(cfg.Query) == "" {
		return nil, fmt.Errorf("ownershipsql: a query is required")
	}
	if err := validateDSNNamesDatabase(cfg.Driver, cfg.DSN); err != nil {
		return nil, err
	}

	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("ownershipsql: opening %s connection: %w", cfg.Driver, err)
	}
	// One connection: this is a single streaming read, not a pool of work.
	db.SetMaxOpenConns(1)

	timeout := cfg.ConnectTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ownershipsql: connecting to the %s database: %w", cfg.Driver, err)
	}

	rows, err := db.QueryContext(ctx, cfg.Query)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ownershipsql: running the query: %w", err)
	}

	columns, err := rows.Columns()
	if err != nil {
		_ = rows.Close()
		_ = db.Close()
		return nil, fmt.Errorf("ownershipsql: reading the result columns: %w", err)
	}

	s := &sqlSource{db: db, rows: rows, columns: columns}
	s.scan = make([]any, len(columns))
	s.ptrs = make([]any, len(columns))
	for i := range s.scan {
		s.ptrs[i] = &s.scan[i]
	}
	return s, nil
}

func (s *sqlSource) Columns() []string { return s.columns }

func (s *sqlSource) Next() bool {
	if s.err != nil {
		return false
	}
	if !s.rows.Next() {
		s.err = s.rows.Err()
		return false
	}
	if err := s.rows.Scan(s.ptrs...); err != nil {
		s.err = fmt.Errorf("ownershipsql: reading row %d: %w", s.number+1, err)
		return false
	}
	s.number++
	values := make(map[string]string, len(s.columns))
	for i, name := range s.columns {
		values[name] = asText(s.scan[i])
	}
	// A query result is rectangular by construction, so there is no ragged
	// row to report — the Malformed path exists for delimited text.
	s.row = ownershipimport.Row{Number: s.number, Values: values}
	return true
}

func (s *sqlSource) Row() ownershipimport.Row { return s.row }

func (s *sqlSource) Err() error { return s.err }

func (s *sqlSource) Close() error {
	rowsErr := s.rows.Close()
	dbErr := s.db.Close()
	if rowsErr != nil {
		return rowsErr
	}
	return dbErr
}

// asText renders one cell as the string the mapper works in. NULL becomes the
// empty string, which is what an absent cell means everywhere else in the
// intake — a row is never rejected for a missing value, it is reported.
func asText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case bool:
		return strconv.FormatBool(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case time.Time:
		// RFC3339 so a date read from a database and a date typed into a
		// spreadsheet compare the same way downstream.
		return t.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// ---------------------------------------------------------------------------
// Browsing what is there
//
// An administrator setting this up may not know the database — ours certainly
// does not know the customer's. Listing the tables lets them point at one
// rather than write SQL against a schema nobody present can see.
// ---------------------------------------------------------------------------

// Table is one table or view the connection can see.
type Table struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	// Kind is "table" or "view". A view is often exactly what an operations
	// team has already built for reporting, so it is offered too.
	Kind string `json:"kind"`
}

// QualifiedName is the table as it should appear in a query, quoted for its
// database so a name with a space or a reserved word still works.
func (t Table) QualifiedName(driver string) string {
	if driver == DriverSQLServer {
		return "[" + t.Schema + "].[" + t.Name + "]"
	}
	return `"` + t.Schema + `"."` + t.Name + `"`
}

// listTablesQuery is ANSI INFORMATION_SCHEMA, which both databases implement.
// The system schemas differ, so each excludes its own.
func listTablesQuery(driver string) string {
	const base = `SELECT table_schema, table_name, table_type
		FROM information_schema.tables
		WHERE table_type IN ('BASE TABLE', 'VIEW')`
	if driver == DriverSQLServer {
		return base + ` AND table_schema NOT IN ('sys', 'INFORMATION_SCHEMA')
			ORDER BY table_schema, table_name`
	}
	return base + ` AND table_schema NOT IN ('pg_catalog', 'information_schema')
		ORDER BY table_schema, table_name`
}

// ListTables returns the tables and views the connection can see, so an
// administrator can choose one instead of writing a query blind.
func ListTables(ctx context.Context, cfg Config) ([]Table, error) {
	if !IsSupportedDriver(cfg.Driver) {
		return nil, fmt.Errorf("ownershipsql: unsupported driver %q", cfg.Driver)
	}
	// Checked here as well as in Open: listing tables is the first thing an
	// administrator does, so it is where the missing database should be
	// reported — not several screens later when they try to read rows.
	if err := validateDSNNamesDatabase(cfg.Driver, cfg.DSN); err != nil {
		return nil, err
	}

	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("ownershipsql: opening %s connection: %w", cfg.Driver, err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	timeout := cfg.ConnectTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("ownershipsql: connecting to the %s database: %w", cfg.Driver, err)
	}

	rows, err := db.QueryContext(ctx, listTablesQuery(cfg.Driver))
	if err != nil {
		return nil, fmt.Errorf("ownershipsql: listing tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Table
	for rows.Next() {
		var schema, name, tableType string
		if err := rows.Scan(&schema, &name, &tableType); err != nil {
			return nil, fmt.Errorf("ownershipsql: reading the table list: %w", err)
		}
		kind := "table"
		if tableType == "VIEW" {
			kind = "view"
		}
		out = append(out, Table{Schema: schema, Name: name, Kind: kind})
	}
	return out, rows.Err()
}
