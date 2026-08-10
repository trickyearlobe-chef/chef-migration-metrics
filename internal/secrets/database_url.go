// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"errors"
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
//
// "mssql" is here because it is a widely used alias for the same driver, and
// refusing it teaches nothing.
var databaseURLSchemes = map[string]bool{
	"postgres":   true,
	"postgresql": true,
	"sqlserver":  true,
	"mssql":      true,
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

// ValidateDatabaseURL reports whether a connection string names a database and
// a driver the importer can open. Exported so the SQL package can apply exactly
// this check at the point of use without a second copy of the parsing — the
// first version had two, and a customer connection was refused by one of them.
//
// It errs towards accepting. The purpose is an early, helpful refusal for the
// obvious mistake of omitting the database; a connection string is a bag of
// vendor options this code has no business adjudicating, and refusing a valid
// one blocks somebody with no way round it.
func ValidateDatabaseURL(dsn string) error {
	result := validateDatabaseURL([]byte(dsn))
	if result.Valid {
		return nil
	}
	return result.Error
}

// validateDatabaseURL checks a stored database connection string.
//
// No error it returns ever includes the value. The value is a password, and an
// error message is the shortest path from a credential into a log that a great
// many people can read.
func validateDatabaseURL(value []byte) ValidationResult {
	dsn := strings.TrimSpace(string(value))
	if dsn == "" {
		return ValidationResult{Valid: false, Error: ErrNotADatabaseURL}
	}

	// Whether this is a URL is decided textually, and net/url is never asked to
	// parse it. A connection string is routinely something net/url cannot
	// represent: a named instance (`host\INSTANCE`), a Windows-auth user
	// (`DOMAIN\svc`), a bare `%` in a password, or options separated by
	// semicolons with no `?` at all. Every one of those was refused as an
	// unsupported driver — naming the driver, when the scheme was right and the
	// real cause was a parse failure. Nothing here needs a parsed URL: the only
	// question is whether a database is named, and that is answered on the text.
	if scheme, rest, isURL := splitDatabaseURLScheme(dsn); isURL {
		if !databaseURLSchemes[scheme] {
			return ValidationResult{Valid: false, Error: ErrNotADatabaseURL}
		}
		if namesDatabase(dsn) {
			return ValidationResult{Valid: true, Metadata: map[string]any{"driver": scheme}}
		}
		// Postgres names the database in the path. SQL Server uses the path for
		// a named instance, so a path alone does not count there.
		if scheme != "sqlserver" && scheme != "mssql" && pathNamesDatabase(rest) {
			return ValidationResult{Valid: true, Metadata: map[string]any{"driver": scheme}}
		}
		return ValidationResult{Valid: false, Error: ErrDatabaseURLNamesNoDatabase}
	}

	// The keyword-value spelling a DBA is as likely to hand over as a URL.
	if !looksLikeKeywordValue(dsn) {
		return ValidationResult{Valid: false, Error: ErrNotADatabaseURL}
	}
	if !namesDatabase(dsn) {
		return ValidationResult{Valid: false, Error: ErrDatabaseURLNamesNoDatabase}
	}
	return ValidationResult{Valid: true, Metadata: map[string]any{"driver": "sqlserver"}}
}

// splitDatabaseURLScheme reports whether the string is written in the URL
// spelling, and if so returns the scheme folded and whatever follows "://".
//
// A "jdbc:" prefix is stripped: `jdbc:sqlserver://` names the same driver, and
// it is what an application's own configuration hands over.
//
// The text before "://" is only a scheme if it looks like one. A keyword-value
// string can carry a URL as an option value — a callback, a metadata address —
// and splitting on "://" without looking would read all of it as the scheme, so
// anything containing a separator or a space means this is not URL spelling.
func splitDatabaseURLScheme(dsn string) (scheme, rest string, isURL bool) {
	before, after, found := strings.Cut(dsn, "://")
	if !found {
		return "", "", false
	}
	candidate := strings.ToLower(strings.TrimSpace(before))
	candidate = strings.TrimPrefix(candidate, "jdbc:")
	if candidate == "" || strings.ContainsAny(candidate, ";=& \t/@") {
		return "", "", false
	}
	return candidate, after, true
}

// pathNamesDatabase reports whether the part after the host names a database in
// the path, as Postgres does. Read textually for the same reason as everything
// else here: the string may be one net/url cannot parse.
func pathNamesDatabase(rest string) bool {
	end := strings.IndexAny(rest, "?;")
	if end >= 0 {
		rest = rest[:end]
	}
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return false
	}
	return strings.TrimSpace(strings.Trim(rest[slash:], "/")) != ""
}

// namesDatabase looks for a database keyword with a value, across every
// separator these strings use. A connection string is a bag of key=value pairs
// whichever spelling it arrives in, so they are all treated the same way.
func namesDatabase(dsn string) bool {
	fields := strings.FieldsFunc(dsn, func(r rune) bool {
		return r == ';' || r == '&' || r == '?'
	})
	for _, field := range fields {
		key, value, found := strings.Cut(field, "=")
		if !found {
			continue
		}
		switch normaliseConnectionKey(key) {
		// "databaseName" is the JDBC spelling and arrives with the jdbc: prefix.
		case "database", "databasename", "initialcatalog", "dbname":
			if strings.TrimSpace(value) != "" {
				return true
			}
		}
	}
	return false
}

// looksLikeKeywordValue reports whether this is a connection string at all,
// rather than a password somebody pasted into the wrong box. Deliberately
// generous: the alternative to accepting an odd one is refusing a valid one.
func looksLikeKeywordValue(dsn string) bool {
	for field := range strings.SplitSeq(dsn, ";") {
		key, _, found := strings.Cut(field, "=")
		if !found {
			continue
		}
		switch normaliseConnectionKey(key) {
		case "server", "datasource", "addr", "address", "host", "database",
			"initialcatalog", "dbname":
			return true
		}
	}
	return false
}

// normaliseConnectionKey folds the spellings the same keyword arrives in:
// "Initial Catalog", "initial catalog" and "InitialCatalog" are one key.
func normaliseConnectionKey(key string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), " ", ""))
}
