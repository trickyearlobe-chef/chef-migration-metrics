// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package ownershipsql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// Whether a composed connection is escaped correctly is not answerable by
// argument. It was answered by argument twice, wrongly and confidently, and one
// of those wrong answers reached the plan: brace-quoting a password in SQL
// Server's keyword spelling parses cleanly, arrives with the braces attached,
// and is refused as a bad login. Only a server that really admits or really
// refuses the connection can tell the two apart.
//
// So these tests connect. Start the database and seed it first:
//
//	make mssql-up && make seed-mssql && make test-mssql
//
// The awkward login is created by deploy/docker-compose/seed-mssql.sql.

func nastyPassword(t *testing.T) string {
	t.Helper()
	p := os.Getenv("CMM_TEST_MSSQL_NASTY_PW")
	if p == "" {
		t.Skip("CMM_TEST_MSSQL_NASTY_PW is not set; run this through `make test-mssql`")
	}
	return p
}

func visibleConnection(t *testing.T, envVar string) string {
	t.Helper()
	v := os.Getenv(envVar)
	if v == "" {
		t.Skipf("%s is not set; run this through `make test-mssql`", envVar)
	}
	return v
}

func canConnect(ctx context.Context, driver, dsn string) error {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	pingCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return db.PingContext(pingCtx)
}

// The URL spelling, with the password put in and percent-encoded.
func TestFunctional_MSSQL_ComposedURLConnects(t *testing.T) {
	password := nastyPassword(t)
	visible := visibleConnection(t, "CMM_TEST_MSSQL_VISIBLE_URL")
	ctx := context.Background()

	composed, err := Compose(DriverSQLServer, visible, password)
	if err != nil {
		t.Fatalf("composing: %v", err)
	}
	if composed.Form != FormURL {
		t.Fatalf("form = %q, want %q — the fixture is not the URL spelling", composed.Form, FormURL)
	}

	// Baseline: written in by hand it is refused, so connecting below is the
	// composition working and not the password being harmless.
	naive := strings.ReplaceAll(visible, PasswordMarker, password)
	if err := canConnect(ctx, DriverSQLServer, naive); err == nil {
		t.Fatal("the awkward password connects unescaped, so this test proves nothing")
	}

	if err := canConnect(ctx, DriverSQLServer, composed.DSN); err != nil {
		t.Fatalf("the composed connection was refused: %v\n  sent: %s", err, composed.Masked)
	}
}

// The keyword spelling. This is the one the plan had backwards.
func TestFunctional_MSSQL_ComposedKeywordConnects(t *testing.T) {
	password := nastyPassword(t)
	visible := visibleConnection(t, "CMM_TEST_MSSQL_VISIBLE_KEYWORD")
	ctx := context.Background()

	composed, err := Compose(DriverSQLServer, visible, password)
	if err != nil {
		t.Fatalf("composing: %v", err)
	}
	if composed.Form != FormKeyword {
		t.Fatalf("form = %q, want %q — the fixture is not the keyword spelling",
			composed.Form, FormKeyword)
	}

	if err := canConnect(ctx, DriverSQLServer, composed.DSN); err != nil {
		t.Fatalf("the composed connection was refused: %v\n  sent: %s", err, composed.Masked)
	}
}

// The two wrong ways to write the same thing, held as facts so nobody
// re-derives them. Both are accepted by the parser and refused by the server,
// which is the failure mode this journey exists to end: a string that reads
// correctly and does not work.
func TestFunctional_MSSQL_TheWrongKeywordQuotingIsRefusedByTheServer(t *testing.T) {
	password := nastyPassword(t)
	visible := visibleConnection(t, "CMM_TEST_MSSQL_VISIBLE_KEYWORD")
	ctx := context.Background()

	wrong := map[string]string{
		"raw":           strings.ReplaceAll(visible, PasswordMarker, password),
		"brace-quoted":  strings.ReplaceAll(visible, PasswordMarker, odbcQuote(password)),
		"single-quoted": strings.ReplaceAll(visible, PasswordMarker, "'"+password+"'"),
	}
	for name, dsn := range wrong {
		t.Run(name, func(t *testing.T) {
			if err := canConnect(ctx, DriverSQLServer, dsn); err == nil {
				t.Errorf("%s quoting now connects; the composer's rule should be "+
					"re-measured and this expectation retired", name)
			}
		})
	}
}

// The "odbc:" spelling, which the same driver reads with brace rules.
func TestFunctional_MSSQL_ComposedODBCConnects(t *testing.T) {
	password := nastyPassword(t)
	visible := visibleConnection(t, "CMM_TEST_MSSQL_VISIBLE_KEYWORD")
	ctx := context.Background()

	composed, err := Compose(DriverSQLServer, "odbc:"+visible, password)
	if err != nil {
		t.Fatalf("composing: %v", err)
	}
	if composed.Form != FormODBC {
		t.Fatalf("form = %q, want %q", composed.Form, FormODBC)
	}
	if err := canConnect(ctx, DriverSQLServer, composed.DSN); err != nil {
		t.Fatalf("the composed odbc connection was refused: %v\n  sent: %s", err, composed.Masked)
	}
}

// A visible connection and its password reach the real entry point, not only
// the helper. A repair applied in one place and forgotten in another leaves the
// screen still broken, which has happened here before.
//
// This is also what caught the composed connection being escaped a second time
// on its way through: it connected when handed straight to the driver and was
// refused as a bad login through ListTables, because every "%" had become
// "%25". That is the failure this journey is about, produced by our own code.
func TestFunctional_MSSQL_AVisibleConnectionAndItsPasswordListTables(t *testing.T) {
	password := nastyPassword(t)
	visible := visibleConnection(t, "CMM_TEST_MSSQL_VISIBLE_URL")

	tables, err := ListTables(context.Background(), Config{
		Driver:     DriverSQLServer,
		Connection: visible,
		Password:   password,
	})
	if err != nil {
		t.Fatalf("listing tables over a composed connection: %v", err)
	}
	if len(tables) == 0 {
		t.Error("connected but saw no tables, so this proves less than it appears to")
	}
}

// The same through Open, which is the path a real import takes.
func TestFunctional_MSSQL_AVisibleConnectionAndItsPasswordReadRows(t *testing.T) {
	password := nastyPassword(t)
	visible := visibleConnection(t, "CMM_TEST_MSSQL_VISIBLE_KEYWORD")

	src, err := Open(context.Background(), Config{
		Driver:     DriverSQLServer,
		Connection: visible,
		Password:   password,
		Query:      "SELECT 1 AS one",
	})
	if err != nil {
		t.Fatalf("opening a composed connection: %v", err)
	}
	defer func() { _ = src.Close() }()
	if !src.Next() {
		t.Fatalf("read no rows: %v", src.Err())
	}
}

// The account reaches the server exactly as typed — proved by the server
// itself, which quotes it back when it refuses the login.
//
// This closes a gap the parser tests cannot: they show the driver reads the
// account back out of the string unchanged, not that the string reaching the
// server carries it. A refusal naming the account is the only evidence of the
// second thing available without an account that actually works.
func TestFunctional_MSSQL_TheServerQuotesTheAccountBackAsTyped(t *testing.T) {
	visible := visibleConnection(t, "CMM_TEST_MSSQL_VISIBLE_URL")
	host := strings.SplitN(strings.TrimPrefix(visible, "sqlserver://"), "@", 2)
	if len(host) != 2 {
		t.Fatalf("cannot read the host out of %q", visible)
	}
	ctx := context.Background()

	// Accounts SQL Server authenticates itself. A backslash means something
	// else entirely — see the test below.
	accounts := []string{
		`nosuchuser`,
		`svcaccount@corp.example.com`,
		`spaced account`,
	}
	for _, account := range accounts {
		t.Run(account, func(t *testing.T) {
			composed, err := Compose(DriverSQLServer,
				"sqlserver://"+account+":"+PasswordMarker+"@"+host[1],
				"definitely-not-the-password")
			if err != nil {
				t.Fatalf("composing: %v", err)
			}
			err = canConnect(ctx, DriverSQLServer, composed.DSN)
			if err == nil {
				t.Fatal("this account connected, so the refusal cannot be read")
			}
			if !strings.Contains(err.Error(), "Login failed for user") {
				t.Fatalf("refused for a reason that says nothing about the account: %v", err)
			}
			if !strings.Contains(err.Error(), account) {
				t.Errorf("the server was told a different account from the one typed\n"+
					"  typed:  %q\n  server: %v", account, err)
			}
		})
	}
}

// A backslash in the account is not a SQL login at all.
//
// Measured, and it is worth knowing before anybody tries to test one: whatever
// stands before the backslash — a domain, a workgroup, the machine's own
// hostname, or "." — the driver switches to integrated authentication and the
// password stops being a SQL password. A server that is not in that domain
// refuses with "the login is from an untrusted domain", and unlike every other
// refusal it does NOT name the account back.
//
// So an account of this shape cannot be proven to work anywhere we have. The
// container is Linux and joined to nothing, and SQL Server will not create a
// password login whose name contains a backslash — "not a valid name because it
// contains invalid characters". The customer's own connection is this shape, so
// what happens when their firewall opens is a path nothing here has exercised.
func TestFunctional_MSSQL_ABackslashAccountAsksForIntegratedAuthentication(t *testing.T) {
	visible := visibleConnection(t, "CMM_TEST_MSSQL_VISIBLE_URL")
	host := strings.SplitN(strings.TrimPrefix(visible, "sqlserver://"), "@", 2)
	if len(host) != 2 {
		t.Fatalf("cannot read the host out of %q", visible)
	}
	ctx := context.Background()

	// A real domain, a workgroup, and the two spellings of "this machine".
	for _, account := range []string{
		`EXAMPLECORP\svcaccount`,
		`WORKGROUP\svclocal`,
		`WINBOX\svclocal`,
		`.\svclocal`,
	} {
		t.Run(account, func(t *testing.T) {
			composed, err := Compose(DriverSQLServer,
				"sqlserver://"+account+":"+PasswordMarker+"@"+host[1],
				"definitely-not-the-password")
			if err != nil {
				t.Fatalf("composing: %v", err)
			}
			err = canConnect(ctx, DriverSQLServer, composed.DSN)
			if err == nil {
				t.Fatal("a backslash account connected; this expectation should be re-measured")
			}
			if !strings.Contains(err.Error(), "Integrated authentication") {
				t.Errorf("a backslash account no longer routes to integrated authentication, "+
					"which changes what the customer's connection will do: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PostgreSQL
//
// libpq validates nothing when a connection is opened, so its parser cannot be
// measured without a server at all — every malformed string looks fine until it
// is used. This creates the awkward login it needs and removes it afterwards,
// so it depends on nothing having been seeded by hand.
// ---------------------------------------------------------------------------

func postgresAdminURL(t *testing.T) *url.URL {
	t.Helper()
	raw := os.Getenv("CMM_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("CMM_TEST_DATABASE_URL is not set — skipping the PostgreSQL composition tests")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("CMM_TEST_DATABASE_URL is not a URL: %v", err)
	}
	return u
}

// withAwkwardPostgresLogin creates a role whose password needs escaping, runs
// the test body, and drops it again.
func withAwkwardPostgresLogin(t *testing.T, body func(admin *url.URL, user, password string)) {
	t.Helper()
	admin := postgresAdminURL(t)
	const (
		user     = "cmm_compose_probe"
		password = `pa%ss;wo rd#7Q!`
	)

	db, err := sql.Open(DriverPostgres, admin.String())
	if err != nil {
		t.Fatalf("opening the admin connection: %v", err)
	}
	defer func() { _ = db.Close() }()

	drop := fmt.Sprintf("DROP ROLE IF EXISTS %s", quoteRoleIdent(user))
	if _, err := db.Exec(drop); err != nil {
		t.Skipf("cannot manage roles on this database, so composition cannot be measured: %v", err)
	}
	create := fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD %s",
		quoteRoleIdent(user), quoteRoleLiteral(password))
	if _, err := db.Exec(create); err != nil {
		t.Skipf("cannot create a login on this database: %v", err)
	}
	defer func() {
		if _, err := db.Exec(drop); err != nil {
			t.Errorf("left the probe role behind: %v", err)
		}
	}()

	body(admin, user, password)
}

func quoteRoleIdent(s string) string   { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func quoteRoleLiteral(s string) string { return `'` + strings.ReplaceAll(s, `'`, `''`) + `'` }

func TestFunctional_Postgres_ComposedConnectionsConnect(t *testing.T) {
	withAwkwardPostgresLogin(t, func(admin *url.URL, user, password string) {
		ctx := context.Background()
		database := strings.TrimPrefix(admin.Path, "/")

		visibles := map[string]string{
			"url": "postgres://" + user + ":" + PasswordMarker + "@" + admin.Host +
				"/" + database + "?sslmode=disable",
			"keyword": "host=" + admin.Hostname() + " port=" + portOf(admin) +
				" dbname=" + database + " user=" + user +
				" password=" + PasswordMarker + " sslmode=disable",
		}
		for name, visible := range visibles {
			t.Run(name, func(t *testing.T) {
				composed, err := Compose(DriverPostgres, visible, password)
				if err != nil {
					t.Fatalf("composing: %v", err)
				}
				if err := canConnect(ctx, DriverPostgres, composed.DSN); err != nil {
					t.Fatalf("the composed connection was refused: %v\n  sent: %s",
						err, composed.Masked)
				}
			})
		}
	})
}

// The baseline for PostgreSQL: appended raw, the same password fails. Without
// this the test above could pass because the password was harmless.
func TestFunctional_Postgres_TheAwkwardPasswordNeedsEscaping(t *testing.T) {
	withAwkwardPostgresLogin(t, func(admin *url.URL, user, password string) {
		ctx := context.Background()
		database := strings.TrimPrefix(admin.Path, "/")

		naive := map[string]string{
			"url": "postgres://" + user + ":" + password + "@" + admin.Host + "/" +
				database + "?sslmode=disable",
			"keyword": "host=" + admin.Hostname() + " port=" + portOf(admin) +
				" dbname=" + database + " user=" + user + " password=" + password +
				" sslmode=disable",
		}
		for name, dsn := range naive {
			t.Run(name, func(t *testing.T) {
				if err := canConnect(ctx, DriverPostgres, dsn); err == nil {
					t.Error("the awkward password connects unescaped, so the composition " +
						"tests are not measuring escaping")
				}
			})
		}
	})
}

func portOf(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	return "5432"
}
