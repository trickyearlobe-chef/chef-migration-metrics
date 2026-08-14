// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package ownershipsql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	mssql "github.com/microsoft/go-mssqldb"
)

// Which of the five it was, decided by asking real servers to refuse in each of
// the five ways.
//
// The classification reads driver text, which is only defensible if the text is
// re-measured rather than remembered. Every case below provokes a real failure
// from a real server; if a driver changes its wording, this goes red and the
// rules get corrected, instead of an outcome quietly naming the wrong team.

func mssqlHostPart(t *testing.T) string {
	t.Helper()
	visible := visibleConnection(t, "CMM_TEST_MSSQL_VISIBLE_URL")
	parts := strings.SplitN(strings.TrimPrefix(visible, "sqlserver://"), "@", 2)
	if len(parts) != 2 {
		t.Fatalf("cannot read the host out of %q", visible)
	}
	return parts[1]
}

func TestFunctional_MSSQL_TheConnectionTestSaysWhichOfTheFiveItWas(t *testing.T) {
	password := nastyPassword(t)
	host := mssqlHostPart(t)
	ctx := context.Background()
	marker := PasswordMarker

	cases := []struct {
		name       string
		connection string
		password   string
		want       Outcome
	}{
		{
			name:       "connected",
			connection: "sqlserver://cmmnasty:" + marker + "@" + host,
			password:   password,
			want:       OutcomeConnected,
		},
		{
			name:       "the account or its password is wrong",
			connection: "sqlserver://cmmnasty:" + marker + "@" + host,
			password:   "definitely-not-the-password",
			want:       OutcomeRefused,
		},
		{
			name:       "no such database",
			connection: "sqlserver://cmmnasty:" + marker + "@localhost:1433?database=nosuchdb",
			password:   password,
			want:       OutcomeNoDatabase,
		},
		{
			name:       "nothing is listening",
			connection: "sqlserver://cmmnasty:" + marker + "@localhost:14333?database=cmdb",
			password:   password,
			want:       OutcomeUnreachable,
		},
		{
			name:       "no such host",
			connection: "sqlserver://cmmnasty:" + marker + "@nosuchhost.invalid:1433?database=cmdb",
			password:   password,
			want:       OutcomeUnreachable,
		},
		{
			name:       "the account is not the database's to check",
			connection: `sqlserver://EXAMPLECORP\svc:` + marker + "@" + host,
			password:   password,
			want:       OutcomeUntrustedDomain,
		},
		{
			name:       "the connection never says where the password goes",
			connection: "sqlserver://cmmnasty@" + host,
			password:   password,
			want:       OutcomeMalformed,
		},
		{
			name:       "the connection names another database entirely",
			connection: "postgres://cmmnasty:" + marker + "@" + host,
			password:   password,
			want:       OutcomeMalformed,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := TestConnection(ctx, Config{
				Driver:     DriverSQLServer,
				Connection: c.connection,
				Password:   c.password,
			})
			if got.Outcome != c.want {
				t.Errorf("outcome = %q, want %q\n  detail: %s", got.Outcome, c.want, got.Detail)
			}
			if got.Outcome != OutcomeConnected && got.Detail == "" {
				t.Error("a failure with no detail — the words of whatever refused us are the " +
					"only thing in it worth having")
			}
			// Whatever it says, it must not say the password.
			for spelling, form := range spellingsOf(c.password) {
				if strings.TrimSpace(form) == "" {
					continue
				}
				if strings.Contains(got.Detail, form) || strings.Contains(got.Connection, form) {
					t.Errorf("the %s spelling of the password came back: %s | %s",
						spelling, got.Detail, got.Connection)
				}
			}
		})
	}
}

// Listing tables is not a connection test, which is the distinction the journey
// draws. Both must reach the same server, but only one of them is trying to
// find out why it failed.
func TestFunctional_MSSQL_TestingAConnectionIsNotListingTables(t *testing.T) {
	_ = nastyPassword(t) // skips unless the container fixtures are set
	host := mssqlHostPart(t)

	cfg := Config{
		Driver:     DriverSQLServer,
		Connection: "sqlserver://cmmnasty:" + PasswordMarker + "@" + host,
		Password:   "definitely-not-the-password",
	}

	result := TestConnection(context.Background(), cfg)
	if result.Succeeded() {
		t.Fatal("the wrong password connected, so there is nothing to compare")
	}
	if result.Outcome != OutcomeRefused {
		t.Errorf("outcome = %q, want %q", result.Outcome, OutcomeRefused)
	}

	// The same connection through the browsing path gives an error and no
	// answer to "which of the five", which is exactly the conflation the
	// journey rejects.
	if _, err := ListTables(context.Background(), cfg); err == nil {
		t.Fatal("listing tables succeeded with a wrong password")
	}
}

// A successful test reports the connection it actually sent, masked, so the
// thing on screen is the thing that worked.
func TestFunctional_MSSQL_ASuccessfulTestShowsWhatItSent(t *testing.T) {
	password := nastyPassword(t)
	visible := visibleConnection(t, "CMM_TEST_MSSQL_VISIBLE_URL")

	result := TestConnection(context.Background(), Config{
		Driver:     DriverSQLServer,
		Connection: visible,
		Password:   password,
	})
	if !result.Succeeded() {
		t.Fatalf("could not connect: %s", result.Detail)
	}
	if !strings.Contains(result.Connection, PasswordMask) {
		t.Errorf("the connection shown carries no mask: %s", result.Connection)
	}
	if strings.Contains(result.Connection, PasswordMarker) {
		t.Errorf("the connection shown is the template, not what was sent: %s", result.Connection)
	}
	if !strings.Contains(result.Connection, "cmmnasty") {
		t.Errorf("the account is not readable in what was sent: %s", result.Connection)
	}
}

// PostgreSQL says these things differently, so it is measured separately rather
// than assumed to match.
func TestFunctional_Postgres_TheConnectionTestSaysWhichOfTheFiveItWas(t *testing.T) {
	admin := postgresAdminURL(t)
	ctx := context.Background()
	const (
		user     = "cmm_outcome_probe"
		password = `pa%ss;wo rd#7Q!`
	)
	database := strings.TrimPrefix(admin.Path, "/")

	db, err := sql.Open(DriverPostgres, admin.String())
	if err != nil {
		t.Fatalf("opening the admin connection: %v", err)
	}
	defer func() { _ = db.Close() }()

	drop := fmt.Sprintf("DROP ROLE IF EXISTS %s", quoteRoleIdent(user))
	if _, err := db.Exec(drop); err != nil {
		t.Skipf("cannot manage roles on this database: %v", err)
	}
	if _, err := db.Exec(fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD %s",
		quoteRoleIdent(user), quoteRoleLiteral(password))); err != nil {
		t.Skipf("cannot create a login: %v", err)
	}
	defer func() {
		if _, err := db.Exec(drop); err != nil {
			t.Errorf("left the probe role behind: %v", err)
		}
	}()

	at := func(dbName string) string {
		return "postgres://" + user + ":" + PasswordMarker + "@" + admin.Host +
			"/" + dbName + "?sslmode=disable"
	}

	cases := []struct {
		name       string
		connection string
		password   string
		want       Outcome
	}{
		{"connected", at(database), password, OutcomeConnected},
		{"the account or its password is wrong", at(database), "wrong", OutcomeRefused},
		{"no such database", at("nosuchdb"), password, OutcomeNoDatabase},
		{"nothing is listening",
			"postgres://" + user + ":" + PasswordMarker + "@" + admin.Hostname() +
				":54329/" + database + "?sslmode=disable", password, OutcomeUnreachable},
		{"no such host",
			"postgres://" + user + ":" + PasswordMarker +
				"@nosuchhost.invalid:5432/" + database + "?sslmode=disable",
			password, OutcomeUnreachable},
		{"the connection never says where the password goes",
			"postgres://" + user + "@" + admin.Host + "/" + database + "?sslmode=disable",
			password, OutcomeMalformed},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := TestConnection(ctx, Config{
				Driver:     DriverPostgres,
				Connection: c.connection,
				Password:   c.password,
			})
			if got.Outcome != c.want {
				t.Errorf("outcome = %q, want %q\n  detail: %s", got.Outcome, c.want, got.Detail)
			}
			for spelling, form := range spellingsOf(c.password) {
				if strings.TrimSpace(form) == "" {
					continue
				}
				if strings.Contains(got.Detail, form) {
					t.Errorf("the %s spelling of the password came back: %s", spelling, got.Detail)
				}
			}
		})
	}
}

// What SQL Server said when it aborted the process, rather than six fixed words.
//
// Measured, and the reason this test exists: a customer read a table and got
// "SQL Server had internal error" and nothing else. That string is the driver's
// own rendering of an aborted process — severity 20 and up — and the real
// message is wrapped inside it, where nothing on a screen can reach it.
//
// A severity-20 error is what aborts a process, so that is what this raises.
func TestFunctional_MSSQL_AnAbortedProcessSaysWhySQLServerAbortedIt(t *testing.T) {
	visible := visibleConnection(t, "CMM_TEST_MSSQL_VISIBLE_URL")
	password := nastyPassword(t)

	_, err := Open(context.Background(), Config{
		Driver:     DriverSQLServer,
		Connection: visible,
		Password:   password,
		Query:      "RAISERROR('deliberate abort', 20, 1) WITH LOG",
	})
	if err == nil {
		t.Fatal("the server accepted a severity-20 error without aborting, so this measures " +
			"nothing")
	}

	// The baseline: the driver's own rendering really is the fixed string, so
	// what is asserted below is this code digging the message out rather than
	// the driver having started to say it.
	var hidden mssql.ServerError
	if !errors.As(err, &hidden) {
		t.Skip("the driver no longer wraps an aborted process in ServerError; if it now says " +
			"what happened by itself, delete this rather than leaving it to assert nothing")
	}
	if strings.Contains(hidden.Error(), "kill state") || strings.Contains(hidden.Error(), "596") {
		t.Fatal("the fixture proves nothing: the driver's wrapper now carries the message")
	}

	// What the person reading actually gets.
	for _, want := range []string{"SQL Server error", "severity"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure does not carry %q, so a screen still shows six words that "+
				"say nothing: %v", want, err)
		}
	}
	if !strings.Contains(err.Error(), "running the query") {
		t.Errorf("the failure no longer says what we were doing: %v", err)
	}
}
