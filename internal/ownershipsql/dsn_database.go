// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"errors"
	"net/url"
	"strings"
)

// A connection must name the database it reads. See journeys/ownership-intake.md.
//
// The alternative — accepting a connection with no database and offering a list
// of everything the account can reach — needs an account permitted to enumerate
// databases on the server. That is a broader grant than reading one table
// requires, and one more thing for somebody to have to defend. Naming the
// database says the same thing more narrowly, and whoever issued the credential
// already knew which one they meant. Two databases means two connections.
//
// This is not a new rule so much as an enforced one: the connection examples the
// import screen shows have always included the database.

// ErrDSNNamesNoDatabase is returned when a connection string does not say which
// database to read. The message is written for whoever is pasting the string in,
// who is usually not the person who composed it.
var ErrDSNNamesNoDatabase = errors.New(
	"the connection does not name a database — add it, as in " +
		"postgres://user:pass@host:5432/DATABASE or " +
		"sqlserver://user:pass@host:1433?database=DATABASE")

// validateDSNNamesDatabase reports whether the connection string says which
// database to read.
//
// It never quotes the connection string in an error: the string carries the
// password, and an error is the shortest path from a credential to a shared log.
func validateDSNNamesDatabase(driver, dsn string) error {
	if named(driver, dsn) {
		return nil
	}
	return ErrDSNNamesNoDatabase
}

// named does the parsing. Both spellings a DBA might hand over are accepted: a
// URL, and SQL Server's semicolon-separated ADO form.
func named(driver, dsn string) bool {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return false
	}

	// ADO / keyword-value form: server=host;database=cmdb;...
	// Checked first because it is not a URL and parsing it as one succeeds in a
	// way that tells us nothing.
	if !strings.Contains(trimmed, "://") {
		return keywordValueNamesDatabase(trimmed)
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		// Unparseable is somebody else's error to report; it is not our business
		// to reject it here, and rejecting it as "no database" would send the
		// reader looking for the wrong problem.
		return true
	}

	// A database given as a query parameter, which is SQL Server's URL spelling.
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

	// A database given as the path, which is Postgres's spelling. SQL Server
	// uses the path for a named instance, so a path alone is not enough there.
	if driver == DriverSQLServer {
		return false
	}
	return strings.TrimSpace(strings.Trim(parsed.Path, "/")) != ""
}

// keywordValueNamesDatabase looks for a non-empty database (or its documented
// aliases) among semicolon-separated key=value pairs.
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
