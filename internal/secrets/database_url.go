// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"errors"
	"net/url"
	"strings"
)

// A database connection is a credential with a shape, so it is checked when it
// is stored — the same treatment a Chef client key gets, and for the same
// reason. See journeys/ownership-intake.md.
//
// Stored as a generic secret it is just bytes: a connection missing its
// database, or pointing at a driver we cannot open, is accepted quietly and
// fails much later, in front of the administrator setting up an import. That
// person did not compose the string and often cannot fix it. Checking here puts
// the refusal in front of whoever wrote it, while they still have it open.
//
// The two things checked are the two the importer cannot recover from: a driver
// it has no way to open, and a connection that does not say which database to
// read. What the connection can actually reach is not knowable until it is used,
// and is deliberately not guessed at here.

// databaseURLSchemes are the drivers the importer can open. Kept as literals
// rather than imported from the SQL package: this is the lowest layer in the
// tree and must not grow a dependency on a domain package to validate bytes.
// internal/ownershipsql's supported-driver list is the authority; if it gains a
// driver, this list gains the scheme.
var databaseURLSchemes = map[string]bool{
	"postgres":   true,
	"postgresql": true,
	"sqlserver":  true,
}

// ErrNotADatabaseURL is returned when the value is not a connection string for
// a driver the importer can open.
var ErrNotADatabaseURL = errors.New(
	"secrets: value is not a database connection for a supported driver " +
		"(postgres, sqlserver)")

// ErrDatabaseURLNamesNoDatabase is returned when the connection does not say
// which database to read. The message shows the shape rather than the value,
// because the value carries the password.
var ErrDatabaseURLNamesNoDatabase = errors.New(
	"secrets: the connection does not name a database — add it, as in " +
		"postgres://user:pass@host:5432/DATABASE or " +
		"sqlserver://user:pass@host:1433?database=DATABASE")

// validateDatabaseURL checks a stored database connection string.
//
// No error it returns ever includes the value. The value is a password, and an
// error message is the shortest path from a credential into a log that a great
// many people can read.
func validateDatabaseURL(value []byte) ValidationResult {
	dsn := strings.TrimSpace(string(value))

	// SQL Server's ADO spelling: server=host;database=cmdb;...
	if !strings.Contains(dsn, "://") {
		if !strings.Contains(strings.ToLower(dsn), "server=") {
			return ValidationResult{Valid: false, Error: ErrNotADatabaseURL}
		}
		if !keywordValueNamesDatabase(dsn) {
			return ValidationResult{Valid: false, Error: ErrDatabaseURLNamesNoDatabase}
		}
		return ValidationResult{Valid: true, Metadata: map[string]any{"driver": "sqlserver"}}
	}

	parsed, err := url.Parse(dsn)
	if err != nil || !databaseURLSchemes[strings.ToLower(parsed.Scheme)] {
		return ValidationResult{Valid: false, Error: ErrNotADatabaseURL}
	}

	scheme := strings.ToLower(parsed.Scheme)
	if !urlNamesDatabase(parsed, scheme) {
		return ValidationResult{Valid: false, Error: ErrDatabaseURLNamesNoDatabase}
	}
	return ValidationResult{Valid: true, Metadata: map[string]any{"driver": scheme}}
}

// urlNamesDatabase reports whether the connection says which database to read.
// Postgres puts it in the path; SQL Server puts it in a query parameter and uses
// the path for a named instance, so a path alone does not count there.
func urlNamesDatabase(parsed *url.URL, scheme string) bool {
	for key, values := range parsed.Query() {
		if !strings.EqualFold(key, "database") {
			continue
		}
		for _, v := range values {
			if strings.TrimSpace(v) != "" {
				return true
			}
		}
	}
	if scheme == "sqlserver" {
		return false
	}
	return strings.TrimSpace(strings.Trim(parsed.Path, "/")) != ""
}

// keywordValueNamesDatabase looks for a non-empty database among
// semicolon-separated key=value pairs, including the spellings SQL Server
// tooling emits.
func keywordValueNamesDatabase(dsn string) bool {
	for _, pair := range strings.Split(dsn, ";") {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "database", "initial catalog", "dbname":
			if strings.TrimSpace(value) != "" {
				return true
			}
		}
	}
	return false
}
