// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package ownershipsql

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The whole point is that a connection which the driver refuses becomes one it
// accepts, with the same credential. Nothing short of connecting proves that.
//
// Needs a login whose password contains the characters that break a URL. Create
// one on the container used by the other SQL Server tests:
//
//	CREATE LOGIN cmmnasty WITH PASSWORD = 'pa%ss;wo rd#7Q!', CHECK_POLICY = OFF;
//	ALTER SERVER ROLE sysadmin ADD MEMBER cmmnasty;
//
// then set CMM_TEST_MSSQL_NASTY_DSN to a URL-form connection using it, exactly as
// somebody would have typed it — unencoded.
func TestFunctional_MSSQL_EncodesACredentialTheDriverWouldRefuse(t *testing.T) {
	dsn := os.Getenv("CMM_TEST_MSSQL_NASTY_DSN")
	if dsn == "" {
		t.Skip("CMM_TEST_MSSQL_NASTY_DSN is not set; see the comment above")
	}

	// As typed, this must be refused — otherwise the test proves nothing, because
	// there was nothing to repair.
	if err := pingWith(t, dsn); err == nil {
		t.Skip("this connection is already usable, so there is no repair to prove")
	} else if !strings.Contains(err.Error(), "invalid URL format") {
		t.Fatalf("refused for a reason this does not repair: %v", err)
	}

	// And through the path the importer actually uses.
	cfg := Config{Driver: DriverSQLServer, DSN: dsn}
	resolved, err := cfg.resolveDSN()
	if err != nil {
		t.Fatalf("resolving the connection: %v", err)
	}
	if err := pingWith(t, resolved); err != nil {
		t.Fatalf("still cannot connect after encoding the credential: %v", err)
	}
}

// And it must reach the real entry point, not only the helper — a repair applied
// in resolveDSN and forgotten in ListTables would leave the screen still broken.
func TestFunctional_MSSQL_ListingTablesRepairsTheCredential(t *testing.T) {
	dsn := os.Getenv("CMM_TEST_MSSQL_NASTY_DSN")
	if dsn == "" {
		t.Skip("CMM_TEST_MSSQL_NASTY_DSN is not set; see the comment above")
	}

	tables, err := ListTables(context.Background(), Config{Driver: DriverSQLServer, DSN: dsn})
	if err != nil {
		t.Fatalf("listing tables with a credential needing encoding: %v", err)
	}
	if len(tables) == 0 {
		t.Error("connected but saw no tables, so this proves less than it appears to")
	}
}
