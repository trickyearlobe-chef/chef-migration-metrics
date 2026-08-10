// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"strings"
	"testing"
)

// The connection names its database — see journeys/ownership-intake.md.
//
// The alternative was to let the connection omit it and offer a list of every
// database the account can reach. That needs an account permitted to enumerate
// them, which is a broader grant than importing one table needs and one more
// thing for somebody to defend. Naming one database says the same thing more
// narrowly, and whoever set the credential up already knew which one they meant.
//
// The connection examples the screen shows have always included the database.
// This makes the shape we promise the shape we accept.

func TestDSNMustNameADatabase_AcceptsTheFormsWeDocument(t *testing.T) {
	accepted := []struct {
		driver string
		dsn    string
	}{
		{DriverPostgres, "postgres://user:pass@host:5432/cmdb"},
		{DriverPostgres, "postgres://user:pass@host:5432/cmdb?sslmode=require"},
		{DriverSQLServer, "sqlserver://user:pass@host:1433?database=cmdb"},
		{DriverSQLServer, "sqlserver://user:pass@host:1433/instance?database=cmdb"},
		// The ADO spelling, which a DBA is as likely to hand over as a URL.
		{DriverSQLServer, "server=host;user id=svc;password=p;database=cmdb"},
	}
	for _, c := range accepted {
		if err := validateDSNNamesDatabase(c.driver, c.dsn); err != nil {
			t.Errorf("%s: rejected a connection that does name its database: %v\n  %s",
				c.driver, err, c.dsn)
		}
	}
}

func TestDSNMustNameADatabase_RejectsAConnectionWithoutOne(t *testing.T) {
	rejected := []struct {
		driver string
		dsn    string
	}{
		{DriverPostgres, "postgres://user:pass@host:5432"},
		{DriverPostgres, "postgres://user:pass@host:5432/"},
		{DriverPostgres, "postgres://user:pass@host:5432/?sslmode=require"},
		{DriverSQLServer, "sqlserver://user:pass@host:1433"},
		{DriverSQLServer, "sqlserver://user:pass@host:1433?connection+timeout=30"},
		{DriverSQLServer, "server=host;user id=svc;password=p"},
	}
	for _, c := range rejected {
		err := validateDSNNamesDatabase(c.driver, c.dsn)
		if err == nil {
			t.Errorf("%s: accepted a connection that names no database:\n  %s", c.driver, c.dsn)
			continue
		}
		// The person reading this is usually not the person who wrote the
		// connection string, so the message has to say what to add.
		if !strings.Contains(strings.ToLower(err.Error()), "database") {
			t.Errorf("%s: refusal does not say what is missing: %v", c.driver, err)
		}
	}
}

// The refusal must never quote the connection string back: it carries the
// password. This is the mistake that puts credentials in a shared log.
func TestDSNMustNameADatabase_RefusalDoesNotLeakTheCredential(t *testing.T) {
	const secret = "hunter2"
	err := validateDSNNamesDatabase(DriverPostgres, "postgres://svc:"+secret+"@host:5432")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("the refusal quotes the connection string, password included: %v", err)
	}
}

// The check has to sit at the entry points, not only in the helper, or a future
// call site reaches the database without it. Listing tables is where an
// administrator starts, so it is where the missing database must be reported —
// not several screens later when they try to read rows.
func TestEntryPointsRefuseAConnectionWithoutADatabase(t *testing.T) {
	cfg := Config{Driver: DriverPostgres, DSN: "postgres://user:pass@host:5432", Query: "SELECT 1"}

	if _, err := ListTables(t.Context(), cfg); err == nil {
		t.Error("listing tables accepted a connection that names no database")
	}
	if _, err := Open(t.Context(), cfg); err == nil {
		t.Error("opening a source accepted a connection that names no database")
	}
}
