// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"strings"
	"testing"
)

// The TLS mode has to be settable without editing the connection string.
//
// lib/pq requires TLS when the connection says nothing — stricter than psql —
// and against a server without it the import fails with an error that never
// mentions the connection string. The connection is a stored credential, so the
// only way to change it was to retype the whole thing, password included, which
// is exactly the situation the credential store exists to avoid.
//
// It is an override, so a mode given here replaces one already in the string.
// Saying nothing changes nothing.

func TestApplyTLSMode_LeavesTheConnectionAloneWhenNoModeIsGiven(t *testing.T) {
	for _, dsn := range []string{
		"postgres://svc:pw@host:5432/cmdb",
		"postgres://svc:pw@host:5432/cmdb?sslmode=require",
		"host=host dbname=cmdb user=svc",
	} {
		got, err := applyTLSMode(DriverPostgres, dsn, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != dsn {
			t.Errorf("changed a connection when no mode was asked for\n  before: %s\n  after:  %s", dsn, got)
		}
	}
}

func TestApplyTLSMode_SetsTheModeWhenTheConnectionSaysNothing(t *testing.T) {
	cases := []struct{ dsn, mode, want string }{
		{
			"postgres://svc:pw@host:5432/cmdb", "disable",
			"postgres://svc:pw@host:5432/cmdb?sslmode=disable",
		},
		{
			// An existing query gains one more parameter, it does not lose any.
			"postgres://svc:pw@host:5432/cmdb?application_name=cmm", "prefer",
			"postgres://svc:pw@host:5432/cmdb?application_name=cmm&sslmode=prefer",
		},
		{
			// The keyword-value spelling uses spaces, not semicolons.
			"host=host dbname=cmdb user=svc", "disable",
			"host=host dbname=cmdb user=svc sslmode=disable",
		},
	}
	for _, c := range cases {
		got, err := applyTLSMode(DriverPostgres, c.dsn, c.mode)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != c.want {
			t.Errorf("wrong result\n  from: %s\n  got:  %s\n  want: %s", c.dsn, got, c.want)
		}
	}
}

// It is an override. A mode already in the connection is replaced, not added to,
// because two sslmode parameters is a connection nobody can reason about.
func TestApplyTLSMode_ReplacesAModeAlreadyInTheConnection(t *testing.T) {
	cases := []struct{ dsn, mode, want string }{
		{
			"postgres://svc:pw@host:5432/cmdb?sslmode=require", "disable",
			"postgres://svc:pw@host:5432/cmdb?sslmode=disable",
		},
		{
			"postgres://svc:pw@host:5432/cmdb?sslmode=require&application_name=cmm", "disable",
			"postgres://svc:pw@host:5432/cmdb?sslmode=disable&application_name=cmm",
		},
		{
			"host=host sslmode=require dbname=cmdb", "disable",
			"host=host sslmode=disable dbname=cmdb",
		},
	}
	for _, c := range cases {
		got, err := applyTLSMode(DriverPostgres, c.dsn, c.mode)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != c.want {
			t.Errorf("did not replace the existing mode\n  from: %s\n  got:  %s\n  want: %s", c.dsn, got, c.want)
		}
		if strings.Count(got, "sslmode=") != 1 {
			t.Errorf("left more than one sslmode in the connection: %s", got)
		}
	}
}

// Only the modes Postgres itself defines. A typo must be refused here rather
// than reaching the driver as an unexplained connection failure.
func TestApplyTLSMode_RefusesAModePostgresDoesNotHave(t *testing.T) {
	for _, mode := range []string{"off", "yes", "true", "disabled", "SSL"} {
		if _, err := applyTLSMode(DriverPostgres, "postgres://svc:pw@host:5432/cmdb", mode); err == nil {
			t.Errorf("accepted %q as a TLS mode", mode)
		}
	}
	for _, mode := range PostgresTLSModes {
		if _, err := applyTLSMode(DriverPostgres, "postgres://svc:pw@host:5432/cmdb", mode); err != nil {
			t.Errorf("refused %q, which Postgres does define: %v", mode, err)
		}
	}
}

// SQL Server has its own vocabulary, and each mode is written as the options
// MEASURED to produce that behaviour against a real server. See tls_mode.go for
// the measurements; the live half is in the functional test.
//
// The mapping is not guessable. "require" has to disable certificate checking to
// mean what Postgres means by it, and "verify" has to turn that off again.
func TestApplyTLSMode_WritesTheMeasuredOptionsForSQLServer(t *testing.T) {
	const dsn = "sqlserver://svc:pw@host:1433?database=cmdb"
	cases := []struct {
		mode  string
		wants []string
	}{
		{"disable", []string{"encrypt=disable"}},
		{"require", []string{"encrypt=true", "TrustServerCertificate=true"}},
		{"verify", []string{"encrypt=true", "TrustServerCertificate=false"}},
		{"strict", []string{"encrypt=strict"}},
	}
	for _, c := range cases {
		got, err := applyTLSMode(DriverSQLServer, dsn, c.mode)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.mode, err)
		}
		for _, want := range c.wants {
			if !strings.Contains(got, want) {
				t.Errorf("%s: does not set %s\n  got: %s", c.mode, want, got)
			}
		}
		if !strings.Contains(got, "database=cmdb") {
			t.Errorf("%s: lost the database from the connection\n  got: %s", c.mode, got)
		}
	}
}

// Postgres's "prefer" and "allow" are deliberately not offered for SQL Server:
// it has no "encrypt if offered, otherwise do not", and the nearest spelling
// still demands TLS and still verifies the certificate.
func TestApplyTLSMode_DoesNotPretendSQLServerHasPrefer(t *testing.T) {
	for _, mode := range []string{"prefer", "allow", "verify-ca", "verify-full"} {
		if _, err := applyTLSMode(DriverSQLServer, "sqlserver://svc:pw@host:1433?database=cmdb", mode); err == nil {
			t.Errorf("offered %q for SQL Server, which has no such setting", mode)
		}
	}
}

// And a mode set twice is replaced, not repeated, on the SQL Server side too.
func TestApplyTLSMode_ReplacesSQLServerOptionsAlreadyPresent(t *testing.T) {
	got, err := applyTLSMode(DriverSQLServer,
		"sqlserver://svc:pw@host:1433?database=cmdb&encrypt=disable", "require")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(got, "encrypt=") != 1 {
		t.Errorf("left more than one encrypt in the connection: %s", got)
	}
	if strings.Contains(got, "encrypt=disable") {
		t.Errorf("did not replace the existing encrypt: %s", got)
	}
}

// The refusals must not quote the connection: it carries the password.
func TestApplyTLSMode_RefusalNeverQuotesTheConnection(t *testing.T) {
	const secret = "hunter2"
	dsn := "postgres://svc:" + secret + "@host:5432/cmdb"
	if _, err := applyTLSMode(DriverPostgres, dsn, "nonsense"); err != nil {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("the refusal quotes the connection: %v", err)
		}
	}
}
