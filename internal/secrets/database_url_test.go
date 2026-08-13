// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"errors"
	"strings"
	"testing"
)

// Whether a connection names its database, checked where it is set up and where
// it is tested — see journeys/ownership-intake.md and
// journeys/ownership-connection.md.
//
// Unchecked, a database-less connection is accepted quietly and fails later, in
// front of the administrator setting up an import, who did not write it and
// cannot fix it. Checking it puts the refusal in front of the person who
// composed it, while they still have it open.

func TestDatabaseURL_AcceptsTheFormsTheImportScreenDocuments(t *testing.T) {
	accepted := []string{
		"postgres://user:pass@host:5432/cmdb",
		"postgres://user:pass@host:5432/cmdb?sslmode=require",
		"postgresql://user:pass@host:5432/cmdb",
		"sqlserver://user:pass@host:1433?database=cmdb",
		"sqlserver://user:pass@host:1433/instance?database=cmdb",
		"server=host;user id=svc;password=p;database=cmdb",
	}
	for _, dsn := range accepted {
		if err := ValidateDatabaseURL(dsn); err != nil {
			t.Errorf("rejected a connection the import screen tells people to use: %v", err)
		}
	}
}

// libpq's own spelling separates its pairs with spaces rather than with
// semicolons, and it names the database "dbname". Measured 2026-08-13: a
// connection in that form was refused as naming no database while naming one in
// plain sight — the refusal even printed "dbname=cmdb" back in its description
// of the shape. It is the form a PostgreSQL DBA hands over, and it is one of the
// two shapes journeys/ownership-connection.md says have to be recognised.
//
// The baseline is asserted first: the same connection with no dbname is still
// refused, so this cannot go green by the check being switched off.
func TestDatabaseURL_AcceptsPostgresKeywordSpelling(t *testing.T) {
	const named = "host=dbhost.example.com dbname=cmdb user=svc password=p sslmode=require"
	const unnamed = "host=dbhost.example.com user=svc password=p sslmode=require"

	if err := ValidateDatabaseURL(unnamed); err == nil {
		t.Fatal("the fixture proves nothing: a connection naming no database is accepted in " +
			"this spelling anyway, so accepting the one below measures nothing")
	}
	if err := ValidateDatabaseURL(named); err != nil {
		t.Errorf("refused a connection that names its database as libpq spells it: %v", err)
	}
}

func TestDatabaseURL_RejectsAConnectionThatNamesNoDatabase(t *testing.T) {
	rejected := []string{
		"postgres://user:pass@host:5432",
		"postgres://user:pass@host:5432/",
		"sqlserver://user:pass@host:1433",
		"server=host;user id=svc;password=p",
	}
	for _, dsn := range rejected {
		if err := ValidateDatabaseURL(dsn); err == nil {
			t.Errorf("accepted a connection that names no database: %s", dsn)
		}
	}
}

func TestDatabaseURL_RejectsSomethingThatIsNotAConnectionAtAll(t *testing.T) {
	for _, value := range []string{"hunter2", "/etc/passwd", "SELECT * FROM owners"} {
		if err := ValidateDatabaseURL(value); err == nil {
			t.Errorf("accepted %q as a database connection", value)
		}
	}
}

// A connection carries a password often enough — somebody pastes one in from
// another tool, and the refusal is the moment they are told not to. An error
// that quotes it is the shortest path from a password to a shared log, and this
// estate ships its logs to a Splunk a great many people can read.
func TestDatabaseURL_RefusalNeverQuotesTheValue(t *testing.T) {
	const secret = "hunter2"
	err := ValidateDatabaseURL("postgres://svc:" + secret + "@host:5432")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("the refusal quotes the connection string, password included: %v", err)
	}
}

// An unknown scheme is refused rather than accepted to fail later against a
// driver we do not have.
func TestDatabaseURL_RejectsADriverWeCannotUse(t *testing.T) {
	if err := ValidateDatabaseURL("mysql://user:pass@host:3306/cmdb"); err == nil {
		t.Error("accepted a connection for a driver the importer cannot open")
	}
}

// The shapes a real SQL Server estate hands over. Every one of these was
// refused by the first version and blocked a customer mid-session, so they are
// pinned by the exact text rather than by a paraphrase of it.
//
// Two separate faults. Go's url.Query() silently drops any parameter separated
// by a semicolon, which is how SQL Server connection strings routinely carry
// their options — so the database was there and unseen. And the keyword-value
// form was recognised only by "server=", while "Data Source=" is at least as
// common and is what their DBA supplied.
func TestDatabaseURL_AcceptsTheOptionsARealEstateCarries(t *testing.T) {
	accepted := []string{
		"sqlserver://svc:pw@host:1433?database=cmdb;ApplicationIntent=ReadOnly;MultiSubnetFailover=True",
		"sqlserver://svc:pw@host:1433?database=cmdb&ApplicationIntent=ReadOnly",
		"server=host;database=cmdb;ApplicationIntent=ReadOnly;MultiSubnetFailover=True",
		"Data Source=host;Initial Catalog=cmdb;ApplicationIntent=ReadOnly",
		"Data Source=host,1433;Initial Catalog=cmdb;Integrated Security=SSPI",
		"postgres://svc:pw@host:5432/cmdb?sslmode=require&application_name=cmm",
	}
	for _, dsn := range accepted {
		if err := ValidateDatabaseURL(dsn); err != nil {
			t.Errorf("refused a connection a customer actually uses: %v\n  shape: %s",
				err, redactForTest(dsn))
		}
	}
}

// Still refused, because the refusal is the point: a connection with no
// database named fails later, in front of somebody who did not write it.
func TestDatabaseURL_StillRefusesAConnectionWithNoDatabaseAmongItsOptions(t *testing.T) {
	rejected := []string{
		"sqlserver://svc:pw@host:1433?ApplicationIntent=ReadOnly;MultiSubnetFailover=True",
		"server=host;ApplicationIntent=ReadOnly;MultiSubnetFailover=True",
		"Data Source=host;Integrated Security=SSPI",
	}
	for _, dsn := range rejected {
		if err := ValidateDatabaseURL(dsn); err == nil {
			t.Errorf("accepted a connection naming no database: %s", redactForTest(dsn))
		}
	}
}

// The shapes URL parsing cannot represent, which a SQL Server estate uses as a
// matter of course. Every one was refused as "not a database connection for a
// supported driver" — naming the driver, when the scheme was right and the real
// cause was net/url declining to parse the string. That message sent a DBA
// looking in entirely the wrong place.
//
// Pinned by literal text rather than by a paraphrase, because a paraphrase is
// what let the previous version of this through.
func TestDatabaseURL_AcceptsTheShapesURLParsingCannotHandle(t *testing.T) {
	accepted := []string{
		// Options separated by semicolons with no "?" at all — the ADO habit.
		// net/url reads ";database=cmdb..." as part of the port.
		"sqlserver://svc:pw@host:1433;database=cmdb;ApplicationIntent=ReadOnly",
		// A named instance. A backslash cannot appear in a URL host, so this
		// shape is unrepresentable as a URL and must not be judged as one.
		"sqlserver://svc:pw@host\\SQLEXPRESS?database=cmdb",
		// A Windows-auth user. Same reason: "invalid userinfo".
		"sqlserver://DOMAIN\\svc:pw@host:1433?database=cmdb",
		// A bare % in a password is not a URL escape. Passwords are not URLs.
		"sqlserver://svc:pw%@host:1433?database=cmdb",
	}
	for _, dsn := range accepted {
		if err := ValidateDatabaseURL(dsn); err != nil {
			t.Errorf("refused a connection that names its database: %v\n  shape: %s",
				err, redactForTest(dsn))
		}
	}
}

// The refusals that earn their place must survive the change above. A string
// that cannot be parsed as a URL is not thereby a connection to anywhere, and
// widening the check must not turn "no database named" into silence.
func TestDatabaseURL_StillRefusesWhatItShould(t *testing.T) {
	cases := []struct {
		dsn  string
		why  string
		want error
	}{
		{"sqlserver://svc:pw@host\\SQLEXPRESS", "a named instance and no database", ErrDatabaseURLNamesNoDatabase},
		{"sqlserver://svc:pw@host:1433;ApplicationIntent=ReadOnly", "semicolon options and no database", ErrDatabaseURLNamesNoDatabase},
		{"mysql://user:pass@host:3306/cmdb", "a driver we cannot open", ErrNotADatabaseURL},
		{"hunter2", "a password pasted into the wrong box", ErrNotADatabaseURL},
	}
	for _, c := range cases {
		err := ValidateDatabaseURL(c.dsn)
		if err == nil {
			t.Errorf("accepted %s: %s", c.why, redactForTest(c.dsn))
			continue
		}
		if !errors.Is(err, c.want) {
			t.Errorf("refused %s for the wrong reason\n  got:  %v\n  want: %v", c.why, err, c.want)
		}
	}
}

// A keyword-value string may itself carry a URL as one of its option values.
// Splitting on "://" without looking must not turn that into a scheme.
func TestDatabaseURL_KeywordValueStringCarryingAURLIsStillKeywordValue(t *testing.T) {
	dsn := "Server=host;Database=cmdb;Failover Partner=host2;Callback=https://example.com/hook"
	if err := ValidateDatabaseURL(dsn); err != nil {
		t.Errorf("read an option value as the scheme: %v", err)
	}
}

// A refusal nobody can diagnose is barely better than no refusal.
//
// The value is encrypted at rest and never logged, both deliberately — this
// estate's logs are widely readable. The cost showed up the first time somebody
// was refused: neither they nor we could tell which rule had fired without
// decrypting the credential, and the customer is reachable only through a VDI
// and a screenshot. So the refusal says what SHAPE it saw, and no values at all.
func TestDatabaseURL_RefusalSaysWhatShapeItSaw(t *testing.T) {
	cases := []struct {
		dsn  string
		says string
	}{
		{"sqlserver://svc:pw@host:1433", "no database"},
		{"sqlserver://svc:pw@host:1433;ApplicationIntent=ReadOnly", "semicolon"},
		{"mysql://svc:pw@host:3306/cmdb", "scheme"},
		{"hunter2", "not recognisable"},
	}
	for _, c := range cases {
		err := ValidateDatabaseURL(c.dsn)
		if err == nil {
			t.Errorf("expected a refusal for %s", redactForTest(c.dsn))
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), c.says) {
			t.Errorf("the refusal does not describe the shape\n  looked for: %q\n  got: %v", c.says, err)
		}
	}
}

// The whole point is that this can be pasted into a ticket, so the refusal must
// carry the credential and nothing else. The host and the database are reported
// deliberately — they say which machine and which database were being reached,
// which is what makes a refusal diagnosable, and neither identifies a person.
func TestDatabaseURL_RefusalCarriesNoCredential(t *testing.T) {
	const (
		user = "svcaccount"
		pass = "hunter2"
	)
	dsn := "sqlserver://" + user + ":" + pass + "@dbserver01:1433;Extra=assetdb"

	err := ValidateDatabaseURL(dsn)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	for _, secret := range []string{user, pass} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("the refusal carries the credential (%q): %v", secret, err)
		}
	}
}

// Wrapping the sentinel with a shape must not stop callers recognising which
// refusal it is.
func TestDatabaseURL_RefusalsStayIdentifiableOnceDescribed(t *testing.T) {
	if err := ValidateDatabaseURL("sqlserver://svc:pw@host:1433"); !errors.Is(err, ErrDatabaseURLNamesNoDatabase) {
		t.Errorf("a missing database is no longer recognisable as one: %v", err)
	}
	if err := ValidateDatabaseURL("mysql://svc:pw@host:3306/cmdb"); !errors.Is(err, ErrNotADatabaseURL) {
		t.Errorf("an unusable driver is no longer recognisable as one: %v", err)
	}
}

// lib/pq requires TLS when no sslmode is given, and against a server without it
// the connection fails with an error that says nothing about the connection
// string. That cost an evening's debugging, so the shape reports it — and the
// shape is the only thing that reaches a log, since the value never does.
func TestConnectionShape_SaysWhenNoSSLModeIsSet(t *testing.T) {
	cases := []struct {
		dsn      string
		mentions bool
	}{
		{"postgres://svc:pw@host:5432/cmdb", true},
		{"postgres://svc:pw@host:5432/cmdb?sslmode=disable", false},
		{"postgres://svc:pw@host:5432/cmdb?sslmode=require", false},
		{"postgresql://svc:pw@host:5432/cmdb", true},
		// SQL Server does not use sslmode, so saying it is unset would be noise.
		{"sqlserver://svc:pw@host:1433?database=cmdb", false},
	}
	for _, c := range cases {
		got := strings.Contains(DescribeConnectionShape(c.dsn), "no sslmode set")
		if got != c.mentions {
			t.Errorf("shape warns about a missing sslmode = %v, want %v\n  for: %s\n  got: %s",
				got, c.mentions, c.dsn, DescribeConnectionShape(c.dsn))
		}
	}
}

// Checked on the exported function directly, not only through a refusal, because
// the logging path calls it on connections that passed validation — those are the
// ones that reach a driver and fail there.
func TestConnectionShape_NeverCarriesTheCredential(t *testing.T) {
	const (
		user = "svcaccount"
		pass = "hunter2"
	)
	for _, dsn := range []string{
		"postgres://" + user + ":" + pass + "@dbserver01:5432/assetdb",
		// A password containing the characters that delimit a URL. The userinfo
		// is removed whole rather than parsed, so these cannot survive it.
		"postgres://" + user + ":p@ss:w0rd/" + pass + "@dbserver01:5432/assetdb",
		"Server=dbserver01;Database=assetdb;User Id=" + user + ";Password=" + pass,
		"host=dbserver01 dbname=assetdb user=" + user + " password=" + pass,
	} {
		shape := DescribeConnectionShape(dsn)
		for _, secret := range []string{user, pass} {
			if strings.Contains(shape, secret) {
				t.Errorf("the description carries the credential (%q)\n  for: %s\n  got: %s", secret, dsn, shape)
			}
		}
	}
}

// When a connection is used, it should describe itself — everything except who
// is connecting and with what secret.
//
// The first version reported structure only, which said a connection was wrong
// without saying which machine or database it was pointed at. Those are the facts
// that turn a driver's complaint into a diagnosis, and neither is a credential.
// The user and the password are, so they are the only things withheld.
func TestConnectionShape_KeepsWhatDiagnosesAndDropsTheCredential(t *testing.T) {
	const (
		user = "svcaccount"
		pass = "hunter2"
	)
	cases := []struct {
		dsn   string
		keeps []string
	}{
		{
			"postgres://" + user + ":" + pass + "@dbserver01:5432/assetdb?sslmode=disable",
			[]string{"dbserver01", "5432", "assetdb", "sslmode=disable"},
		},
		{
			"sqlserver://" + user + ":" + pass + "@dbserver01:1433?database=assetdb&ApplicationIntent=ReadOnly",
			[]string{"dbserver01", "1433", "database=assetdb", "ApplicationIntent=ReadOnly"},
		},
		{
			"Server=dbserver01,1433;Database=assetdb;User Id=" + user + ";Password=" + pass,
			[]string{"dbserver01", "assetdb"},
		},
	}
	for _, c := range cases {
		got := DescribeConnectionShape(c.dsn)
		for _, keep := range c.keeps {
			if !strings.Contains(got, keep) {
				t.Errorf("description drops %q, which diagnoses rather than identifies\n  got: %s", keep, got)
			}
		}
		if strings.Contains(got, user) {
			t.Errorf("description carries the user\n  got: %s", got)
		}
		if strings.Contains(got, pass) {
			t.Errorf("description carries the password\n  got: %s", got)
		}
	}
}

// The description has to name the thing it is hiding.
//
// A customer connection was refused by the SQL Server driver as "invalid URL
// format". The description proved every visible part was fine — scheme, host,
// database, options — which narrowed the cause to the user or the password, and
// then stopped. Those are the parts that are redacted, so the reader was left
// where they started.
//
// Measured against the driver's own parser: %, a space, #, / and ? in a password,
// and a backslash in a username, each make a URL unparsable. ":" and "@" do not.
// So the characters are named, without the values around them.
func TestConnectionShape_NamesTheCharacterThatCannotBeInAURL(t *testing.T) {
	cases := []struct {
		dsn     string
		expects bool
		why     string
	}{
		{"sqlserver://svc:pw%rd@host?database=cmdb", true, "a bare % in the password"},
		{"sqlserver://svc:pw rd@host?database=cmdb", true, "a space in the password"},
		{"sqlserver://svc:pw#rd@host?database=cmdb", true, "a # in the password"},
		{"sqlserver://DOM\\svc:pw@host?database=cmdb", true, "a backslash in the username"},
		// Legal, and must not be reported: a needless warning is as bad as none.
		{"sqlserver://svc:pw@host?database=cmdb", false, "an ordinary password"},
		{"sqlserver://svc:p:w@host?database=cmdb", false, "a colon, which is allowed"},
		{"sqlserver://svc:pw%25rd@host?database=cmdb", false, "a properly encoded %"},
		// The options after "?" are not the userinfo, so a "?" there is fine.
		{"postgres://svc:pw@host:5432/cmdb?sslmode=disable", false, "query parameters"},
	}
	for _, c := range cases {
		got := strings.Contains(DescribeConnectionShape(c.dsn), "cannot appear in a URL")
		if got != c.expects {
			t.Errorf("reports an unusable character = %v, want %v — %s\n  got: %s",
				got, c.expects, c.why, DescribeConnectionShape(c.dsn))
		}
	}
}

// A backslash in the credential and a backslash in the host are different
// problems with the same symptom, so each is reported once and only the one that
// applies. Saying both would send somebody to change the wrong half.
func TestConnectionShape_TellsACredentialBackslashFromAnInstanceOne(t *testing.T) {
	inCredential := DescribeConnectionShape(`sqlserver://DOM\svc:pw@host?database=cmdb`)
	if !strings.Contains(inCredential, "user or password contains a backslash") {
		t.Errorf("did not report a backslash in the credential: %s", inCredential)
	}
	if strings.Contains(inCredential, "host contains a backslash") {
		t.Errorf("blamed the host for a backslash in the credential: %s", inCredential)
	}

	inHost := DescribeConnectionShape(`sqlserver://svc:pw@host\SQLEXPRESS?database=cmdb`)
	if !strings.Contains(inHost, "host contains a backslash") {
		t.Errorf("did not report a named instance: %s", inHost)
	}
	if strings.Contains(inHost, "user or password contains a backslash") {
		t.Errorf("blamed the credential for a backslash in the host: %s", inHost)
	}
}

// Naming the character must not turn into quoting the value around it.
func TestConnectionShape_NamingTheCharacterRevealsNothingElse(t *testing.T) {
	const (
		user = "svcaccount"
		pass = "hunter2"
	)
	shape := DescribeConnectionShape("sqlserver://" + user + ":" + pass + "%x@host?database=cmdb")
	if !strings.Contains(shape, "cannot appear in a URL") {
		t.Fatalf("did not report the unusable character: %s", shape)
	}
	for _, secret := range []string{user, pass} {
		if strings.Contains(shape, secret) {
			t.Errorf("naming the character carried the credential (%q): %s", secret, shape)
		}
	}
}

// redactForTest keeps a failure message from carrying a password, on the same
// reasoning as the validator itself. The fixtures here are fake, but a test
// message is copied into tickets and chat by whoever is debugging.
func redactForTest(dsn string) string {
	if i := strings.Index(dsn, "://"); i >= 0 {
		if at := strings.Index(dsn[i:], "@"); at >= 0 {
			return dsn[:i+3] + "***" + dsn[i+at:]
		}
	}
	return dsn
}
