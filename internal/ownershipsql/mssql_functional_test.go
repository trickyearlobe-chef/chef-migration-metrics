// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package ownershipsql

import (
	"context"
	"os"
	"testing"
)

// SQL Server for real, not by analogy with PostgreSQL: the driver, the
// connection string, NVARCHAR, BIT, DATE, and how NULL comes back are exactly
// the things a PostgreSQL test cannot tell you about.
//
// Start the database with:
//
//	docker compose -f deploy/docker-compose/docker-compose.yml --profile mssql up -d mssql
//	make seed-mssql
//
// then set CMM_TEST_MSSQL_DSN. See deploy/docker-compose/seed-mssql.sql.
func mssqlDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("CMM_TEST_MSSQL_DSN")
	if dsn == "" {
		t.Skip("CMM_TEST_MSSQL_DSN is not set; start the container and seed it (see the comment above)")
	}
	return dsn
}

// The query an administrator would actually write: their system of record keeps
// people and assets in separate tables, so the ingest has to accept a join
// rather than demanding a table shaped like our import format.
const ownerQuery = `
	SELECT
		s.email        AS owner_name,
		a.asset_kind   AS entity_type,
		a.asset_name   AS entity_key,
		s.full_name    AS display_name,
		s.team         AS team,
		a.recorded_on  AS recorded_on
	FROM asset_owner a
	LEFT JOIN staff s ON s.staff_id = a.staff_id
	WHERE s.left_company = 0 OR s.left_company IS NULL
	ORDER BY a.asset_id`

func TestFunctional_MSSQL_ReadsAJoinedOwnerQuery(t *testing.T) {
	dsn := mssqlDSN(t)
	ctx := context.Background()

	src, err := Open(ctx, Config{Driver: DriverSQLServer, DSN: dsn, Query: ownerQuery})
	if err != nil {
		t.Fatalf("opening the SQL Server source: %v", err)
	}
	defer func() { _ = src.Close() }()

	wantCols := []string{"owner_name", "entity_type", "entity_key", "display_name", "team", "recorded_on"}
	if got := src.Columns(); len(got) != len(wantCols) {
		t.Fatalf("columns = %v, want %v", got, wantCols)
	}

	var rows []map[string]string
	for src.Next() {
		rows = append(rows, src.Row().Values)
	}
	if err := src.Err(); err != nil {
		t.Fatalf("iterating: %v", err)
	}

	// The person who left the company is filtered out by the query itself, so
	// the one asset against them does not appear: 11 assets, minus theirs.
	// That is the administrator's decision to make in SQL, and the ingest must
	// not second-guess it.
	if len(rows) != 10 {
		t.Fatalf("read %d rows, want 10 (the leaver's asset is excluded by the query)", len(rows))
	}

	first := rows[0]
	if first["owner_name"] != "priya.raman@example-corp.com" {
		t.Errorf("owner_name = %q, want the email from the joined staff row", first["owner_name"])
	}
	if first["entity_type"] != "node" || first["entity_key"] != "node1.example.com" {
		t.Errorf("first row = %v, want the first node", first)
	}
	// NVARCHAR must come back as text, not as bytes rendered with %v.
	if first["display_name"] != "Priya Raman" {
		t.Errorf("display_name = %q, want %q", first["display_name"], "Priya Raman")
	}
	// DATE must be readable, not a Go struct printed into a cell.
	if got := first["recorded_on"]; got != "2026-01-15T00:00:00Z" {
		t.Errorf("recorded_on = %q, want an RFC3339 timestamp", got)
	}
}

// A row whose owner cannot be resolved must still arrive. The intake's whole
// design is to report what a row would do rather than reject it, and an asset
// with nobody against it is precisely what the unowned question is for.
func TestFunctional_MSSQL_NullsArriveAsEmptyNotAsDropped(t *testing.T) {
	dsn := mssqlDSN(t)
	ctx := context.Background()

	src, err := Open(ctx, Config{Driver: DriverSQLServer, DSN: dsn, Query: ownerQuery})
	if err != nil {
		t.Fatalf("opening the SQL Server source: %v", err)
	}
	defer func() { _ = src.Close() }()

	var orphan, unnamed map[string]string
	for src.Next() {
		r := src.Row().Values
		switch r["entity_key"] {
		case "orphan-host-01":
			orphan = r
		case "unmatched-host-99":
			unnamed = r
		}
	}
	if err := src.Err(); err != nil {
		t.Fatalf("iterating: %v", err)
	}

	if orphan == nil {
		t.Fatal("the asset with no staff row was dropped; it must arrive with an empty owner")
	}
	if orphan["owner_name"] != "" {
		t.Errorf("owner_name = %q, want empty for an asset nobody owns", orphan["owner_name"])
	}
	if unnamed == nil {
		t.Fatal("the asset owned by a person with no email was dropped")
	}
	if unnamed["owner_name"] != "" || unnamed["display_name"] != "Unnamed Owner" {
		t.Errorf("unmatched row = %v, want an empty owner_name and the person's name", unnamed)
	}
}

// A wrong password must say so. Reporting an empty result would read as "this
// database holds no owners", which is a different and wrong answer.
func TestFunctional_MSSQL_ReportsARefusedLogin(t *testing.T) {
	mssqlDSN(t) // only run where a server is available
	_, err := Open(context.Background(), Config{
		Driver: DriverSQLServer,
		DSN:    "sqlserver://sa:definitely-not-the-password@localhost:1433?database=cmdb",
		Query:  "SELECT 1",
	})
	if err == nil {
		t.Fatal("expected a refused login to be reported, got none")
	}
}

// A query the server rejects must fail as a query error, not as no rows.
func TestFunctional_MSSQL_ReportsABadQuery(t *testing.T) {
	dsn := mssqlDSN(t)
	_, err := Open(context.Background(), Config{
		Driver: DriverSQLServer, DSN: dsn,
		Query: "SELECT * FROM table_that_does_not_exist",
	})
	if err == nil {
		t.Fatal("expected a bad query to be reported, got none")
	}
}

// Browsing beats writing SQL blind, which is the position anyone setting this
// up against somebody else's database is in.
func TestFunctional_MSSQL_ListsTablesAndViews(t *testing.T) {
	dsn := mssqlDSN(t)

	tables, err := ListTables(context.Background(), Config{Driver: DriverSQLServer, DSN: dsn})
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}

	found := map[string]Table{}
	for _, tbl := range tables {
		found[tbl.Name] = tbl
	}
	for _, want := range []string{"staff", "asset_owner"} {
		if _, ok := found[want]; !ok {
			t.Errorf("table %q missing from the list: %v", want, found)
		}
	}
	if got := found["staff"].Schema; got != "dbo" {
		t.Errorf("staff schema = %q, want dbo", got)
	}
	if got := found["staff"].Kind; got != "table" {
		t.Errorf("staff kind = %q, want table", got)
	}

	// SQL Server's own catalogue must not be offered as a source of owners.
	for _, tbl := range tables {
		if tbl.Schema == "sys" || tbl.Schema == "INFORMATION_SCHEMA" {
			t.Errorf("system table offered: %s.%s", tbl.Schema, tbl.Name)
		}
	}

	// The generated name is quoted the way SQL Server expects, so a table with
	// an awkward name still produces a query that runs.
	if got := found["staff"].QualifiedName(DriverSQLServer); got != "[dbo].[staff]" {
		t.Errorf("qualified name = %q, want [dbo].[staff]", got)
	}
}

// Choosing a table has to produce a query that actually runs — that is the
// whole point of offering the list.
func TestFunctional_MSSQL_AChosenTableProducesAWorkingQuery(t *testing.T) {
	dsn := mssqlDSN(t)
	ctx := context.Background()

	tables, err := ListTables(ctx, Config{Driver: DriverSQLServer, DSN: dsn})
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	var staff Table
	for _, tbl := range tables {
		if tbl.Name == "staff" {
			staff = tbl
		}
	}
	if staff.Name == "" {
		t.Fatal("staff table not found")
	}

	src, err := Open(ctx, Config{
		Driver: DriverSQLServer, DSN: dsn,
		Query: "SELECT * FROM " + staff.QualifiedName(DriverSQLServer),
	})
	if err != nil {
		t.Fatalf("opening the generated query: %v", err)
	}
	defer func() { _ = src.Close() }()

	n := 0
	for src.Next() {
		n++
	}
	if err := src.Err(); err != nil {
		t.Fatalf("iterating: %v", err)
	}
	if n != 5 {
		t.Errorf("read %d staff rows, want 5", n)
	}
}
