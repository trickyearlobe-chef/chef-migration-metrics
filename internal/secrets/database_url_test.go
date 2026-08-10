// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"errors"
	"strings"
	"testing"
)

// A database connection is its own kind of secret, checked when it is stored —
// see journeys/ownership-intake.md.
//
// Stored as a generic secret, a malformed or database-less connection string is
// accepted quietly and fails later, in front of the administrator setting up an
// import, who did not write it and cannot fix it. Checking it here puts the
// refusal in front of the person who composed it, while they still have it open.

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
		result := ValidateCredentialValue(CredentialTypeDatabaseURL, []byte(dsn))
		if !result.Valid {
			t.Errorf("rejected a connection the import screen tells people to use: %v", result.Error)
		}
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
		result := ValidateCredentialValue(CredentialTypeDatabaseURL, []byte(dsn))
		if result.Valid {
			t.Errorf("stored a connection that names no database: %s", dsn)
		}
	}
}

func TestDatabaseURL_RejectsSomethingThatIsNotAConnectionAtAll(t *testing.T) {
	for _, value := range []string{"hunter2", "/etc/passwd", "SELECT * FROM owners"} {
		result := ValidateCredentialValue(CredentialTypeDatabaseURL, []byte(value))
		if result.Valid {
			t.Errorf("stored %q as a database connection", value)
		}
	}
}

// The value IS the password. An error that quotes it is the shortest path from
// a credential to a shared log — and this estate ships its logs to a Splunk a
// great many people can read.
func TestDatabaseURL_RefusalNeverQuotesTheValue(t *testing.T) {
	const secret = "hunter2"
	result := ValidateCredentialValue(CredentialTypeDatabaseURL, []byte("postgres://svc:"+secret+"@host:5432"))
	if result.Valid {
		t.Fatal("expected a refusal")
	}
	if strings.Contains(result.Error.Error(), secret) {
		t.Errorf("the refusal quotes the connection string, password included: %v", result.Error)
	}
}

// An unknown scheme is refused rather than stored to fail later against a
// driver we do not have.
func TestDatabaseURL_RejectsADriverWeCannotUse(t *testing.T) {
	result := ValidateCredentialValue(CredentialTypeDatabaseURL, []byte("mysql://user:pass@host:3306/cmdb"))
	if result.Valid {
		t.Error("stored a connection for a driver the importer cannot open")
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

// The whole point is that this can be pasted into a ticket. Every part of the
// value is checked against the message, not just the password, because a host
// name in a shared log is somebody's infrastructure.
func TestDatabaseURL_ShapeSummaryLeaksNothingFromTheValue(t *testing.T) {
	const (
		user = "svcaccount"
		pass = "hunter2"
		host = "dbserver01"
		db   = "assetdb"
	)
	dsn := "sqlserver://" + user + ":" + pass + "@" + host + ":1433;Extra=" + db

	err := ValidateDatabaseURL(dsn)
	if err == nil {
		t.Fatal("expected a refusal")
	}
	for _, secret := range []string{user, pass, host, db} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("the refusal carries part of the value (%q): %v", secret, err)
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
