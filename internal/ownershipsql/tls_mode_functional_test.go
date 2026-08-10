// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package ownershipsql

import (
	"strings"
	"testing"
)

// What each TLS mode does against a real SQL Server, because parsing cannot tell
// you and reasoning got it wrong.
//
// The mapping from a mode to connection-string options was first written by
// analogy with PostgreSQL and was wrong twice over. "encrypt=false" looked like
// the equivalent of "prefer" — it is not; it still demands TLS for the login and
// still verifies the certificate. And "encrypt=false" parses to the same value as
// saying nothing at all, then behaves differently, because that value is also
// Go's zero value, so a parse-only check cannot distinguish them.
//
// This connects. It is the only thing that can settle it.
//
//	make mssql-up && make seed-mssql
//	CMM_TEST_MSSQL_DSN="sqlserver://sa:...@localhost:1433?database=cmdb" \
//	  go test -tags functional -run TestFunctional_MSSQL_TLSMode ./internal/ownershipsql/
func TestFunctional_MSSQL_TLSMode_DisableAndRequireConnect(t *testing.T) {
	base := mssqlDSN(t)

	// The container presents SQL Server's self-signed fallback certificate,
	// which is what an unprepared server in an estate looks like.
	for _, mode := range []string{"disable", "require"} {
		dsn, err := applyTLSMode(DriverSQLServer, base, mode)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if err := pingWith(t, dsn); err != nil {
			t.Errorf("%s: did not connect, so the options it writes are wrong: %v", mode, err)
		}
	}
}

// "verify" must actually verify. Against a self-signed certificate that means
// refusing to connect — if this ever passes, the mode is not doing its job and
// somebody is encrypting to a server they have not authenticated.
func TestFunctional_MSSQL_TLSMode_VerifyRefusesASelfSignedCertificate(t *testing.T) {
	base := mssqlDSN(t)

	dsn, err := applyTLSMode(DriverSQLServer, base, "verify")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	err = pingWith(t, dsn)
	if err == nil {
		t.Skip("this server presents a certificate the system trusts, so there is " +
			"nothing for a self-signed refusal to prove here")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Errorf("verify failed for a reason other than the certificate, so this "+
			"proves nothing about verification: %v", err)
	}
}

// The whole point of the override is that it changes the outcome. Without it,
// this server is reachable; "strict" asks for TDS 8.0, which it cannot do — so a
// mode that was silently ignored would show up here as a connection that
// wrongly succeeded.
func TestFunctional_MSSQL_TLSMode_IsNotSilentlyIgnored(t *testing.T) {
	base := mssqlDSN(t)

	if err := pingWith(t, base); err != nil {
		t.Fatalf("the server is not reachable without an override, so this test "+
			"cannot tell whether the override was applied: %v", err)
	}

	dsn, err := applyTLSMode(DriverSQLServer, base, "strict")
	if err != nil {
		t.Fatalf("strict: %v", err)
	}
	if err := pingWith(t, dsn); err == nil {
		t.Error("connected with strict TLS to a server that cannot do it, so the " +
			"mode was not applied to the connection")
	}
}
