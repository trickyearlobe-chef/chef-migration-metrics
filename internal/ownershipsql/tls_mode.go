// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"errors"
	"fmt"
	"strings"
)

// The TLS mode is settable without retyping the connection string.
//
// lib/pq requires TLS when the connection says nothing about it, which is
// stricter than psql, so a correct connection to a server without TLS fails —
// and the error names TLS without naming the connection string. The connection
// is a stored credential, so changing it meant retyping the whole thing,
// password included, which is what the credential store exists to avoid.

// PostgresTLSModes are the modes Postgres defines, in the order the strictness
// increases. Offered as a list so the screen and the check cannot disagree.
var PostgresTLSModes = []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"}

// ErrTLSModeNotSupported is returned for a driver that does not use sslmode.
var ErrTLSModeNotSupported = errors.New(
	"ownershipsql: only a PostgreSQL connection takes a TLS mode; SQL Server " +
		"spells this \"encrypt=\" and it can be set in the connection string")

// ErrUnknownTLSMode is returned for a mode Postgres does not define. It never
// quotes the connection, which carries the password.
var ErrUnknownTLSMode = errors.New("ownershipsql: not a PostgreSQL TLS mode")

// applyTLSMode returns the connection with sslmode set to mode.
//
// An empty mode changes nothing, so a connection that already says what it wants
// is left exactly as it was. A mode that is given wins over one already in the
// string — it is an override, and two sslmode parameters is a connection nobody
// can reason about.
//
// Done textually, never through net/url, for the same reason as everywhere else
// here: these strings routinely hold what a URL cannot represent, and a rewrite
// that reparses would refuse connections that work.
func applyTLSMode(driver, dsn, mode string) (string, error) {
	if mode == "" {
		return dsn, nil
	}
	if driver != DriverPostgres {
		return "", ErrTLSModeNotSupported
	}
	if !isPostgresTLSMode(mode) {
		return "", fmt.Errorf("%w: %q — one of %s",
			ErrUnknownTLSMode, mode, strings.Join(PostgresTLSModes, ", "))
	}

	// The keyword-value spelling separates pairs with spaces; the URL spelling
	// uses "?" then "&". Replacing in place keeps the parameter where the author
	// put it, so a diff of the two strings shows only the value changing.
	if replaced, ok := replaceKeywordValue(dsn, "sslmode", mode); ok {
		return replaced, nil
	}
	if !strings.Contains(dsn, "://") {
		return dsn + " sslmode=" + mode, nil
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + "sslmode=" + mode, nil
}

// replaceKeywordValue rewrites the value of a parameter already present,
// reporting whether it found one. Separators are preserved as they were.
func replaceKeywordValue(dsn, key, value string) (string, bool) {
	var out strings.Builder
	found := false
	field := strings.Builder{}

	flush := func(separator string) {
		text := field.String()
		field.Reset()
		if k, _, hasValue := strings.Cut(text, "="); hasValue &&
			strings.EqualFold(strings.TrimSpace(k), key) {
			out.WriteString(strings.TrimSpace(k) + "=" + value)
			found = true
		} else {
			out.WriteString(text)
		}
		out.WriteString(separator)
	}

	for _, r := range dsn {
		switch r {
		case '?', '&', ';', ' ':
			flush(string(r))
		default:
			field.WriteRune(r)
		}
	}
	flush("")
	return out.String(), found
}

func isPostgresTLSMode(mode string) bool {
	for _, valid := range PostgresTLSModes {
		if mode == valid {
			return true
		}
	}
	return false
}
