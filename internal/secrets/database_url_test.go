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
