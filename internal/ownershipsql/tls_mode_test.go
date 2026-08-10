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

// SQL Server spells this differently ("encrypt="), and the two vocabularies do
// not map onto each other cleanly enough to guess. Refusing is honest; the
// option can still be set in the connection string, where it works.
func TestApplyTLSMode_RefusesAnOverrideForSQLServer(t *testing.T) {
	_, err := applyTLSMode(DriverSQLServer, "sqlserver://svc:pw@host:1433?database=cmdb", "disable")
	if err == nil {
		t.Error("accepted a TLS mode for SQL Server, which does not use sslmode")
	}
	if err != nil && !strings.Contains(err.Error(), "encrypt") {
		t.Errorf("the refusal does not say what to use instead: %v", err)
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
