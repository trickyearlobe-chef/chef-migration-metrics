// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package ownershipsql

import (
	"context"
	"os"
	"testing"
)

// The source is driver-agnostic: it reads whatever a SELECT returns. This
// exercises it against PostgreSQL, which needs no extra driver, so the reading
// and conversion logic is covered without a SQL Server to hand. The SQL Server
// path differs only in the driver name and connection string.
func TestFunctional_SQLSource_ReadsQueryRows(t *testing.T) {
	dsn := os.Getenv("CMM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("CMM_TEST_DATABASE_URL must be set: this test must not silently skip")
	}
	ctx := context.Background()

	src, err := Open(ctx, Config{
		Driver: DriverPostgres,
		DSN:    dsn,
		Query: `SELECT * FROM (VALUES
			('alice.brown', 'git_repo', 'apt'),
			('bob.jones',   'node',     'web-01'),
			(NULL,          'cookbook', 'nginx')
		) AS t(owner_name, entity_type, entity_key)`,
	})
	if err != nil {
		t.Fatalf("opening the source: %v", err)
	}
	defer func() { _ = src.Close() }()

	if got, want := src.Columns(), []string{"owner_name", "entity_type", "entity_key"}; len(got) != len(want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}

	var rows []map[string]string
	for src.Next() {
		r := src.Row()
		if r.Number != len(rows)+1 {
			t.Errorf("row number = %d, want %d", r.Number, len(rows)+1)
		}
		rows = append(rows, r.Values)
	}
	if err := src.Err(); err != nil {
		t.Fatalf("iterating: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("read %d rows, want 3", len(rows))
	}
	if rows[0]["owner_name"] != "alice.brown" || rows[1]["entity_key"] != "web-01" {
		t.Errorf("rows read back wrong: %v", rows)
	}
	// NULL reads as empty, the same as an absent cell in a file: a missing
	// value is reported downstream, never a reason to reject the row.
	if rows[2]["owner_name"] != "" {
		t.Errorf("NULL owner = %q, want empty", rows[2]["owner_name"])
	}
}

// An unreadable source must say so. Returning an empty result would read as
// "this database has no owners in it", which is a different and wrong answer.
func TestFunctional_SQLSource_ReportsAConnectionFailure(t *testing.T) {
	_, err := Open(context.Background(), Config{
		Driver: DriverPostgres,
		DSN:    "postgres://nobody:nothing@127.0.0.1:1/none?sslmode=disable",
		Query:  "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected a connection failure to be reported, got none")
	}
}

func TestFunctional_SQLSource_RejectsAnUnknownDriver(t *testing.T) {
	if _, err := Open(context.Background(), Config{
		Driver: "oracle", DSN: "x", Query: "SELECT 1",
	}); err == nil {
		t.Fatal("expected an unsupported driver to be rejected")
	}
}
