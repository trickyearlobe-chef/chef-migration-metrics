// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
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
