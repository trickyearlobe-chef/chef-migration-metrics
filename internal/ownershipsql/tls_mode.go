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

// SQLServerTLSModes are the choices SQL Server actually offers, in its own
// vocabulary rather than Postgres's.
//
// The two do not translate. Every value below was measured against a real server
// (a container with SQL Server's self-signed fallback certificate), because
// parsing alone cannot tell you this — "encrypt=false" and saying nothing about
// encryption parse to the same value and then behave differently, since that
// value is also Go's zero value.
//
//	disable   encrypt=disable                          connects, no TLS
//	require   encrypt=true&TrustServerCertificate=true  connects, encrypted, certificate not checked
//	verify    encrypt=true                             certificate checked, so a self-signed one is refused
//	strict    encrypt=strict                           TDS 8.0, refused by a server that cannot do it
//
// There is deliberately no equivalent of Postgres's "prefer" or "allow". SQL
// Server has no "encrypt if the server offers it, otherwise do not" — the
// closest spelling, encrypt=false, still demands TLS for the login and still
// verifies the certificate, so offering it under that name would be a lie.
var SQLServerTLSModes = []string{"disable", "require", "verify", "strict"}

// sqlServerTLSOptions is what each mode is written as in the connection string.
var sqlServerTLSOptions = map[string]map[string]string{
	"disable": {"encrypt": "disable"},
	"require": {"encrypt": "true", "TrustServerCertificate": "true"},
	"verify":  {"encrypt": "true", "TrustServerCertificate": "false"},
	"strict":  {"encrypt": "strict"},
}

// TLSModesFor returns the modes a driver offers, so a screen cannot present one
// the driver has never heard of.
func TLSModesFor(driver string) []string {
	if driver == DriverSQLServer {
		return SQLServerTLSModes
	}
	return PostgresTLSModes
}

// ErrTLSModeNotSupported is returned for a driver with no TLS vocabulary.
var ErrTLSModeNotSupported = errors.New(
	"ownershipsql: this database does not take a TLS mode")

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
	if !isTLSMode(driver, mode) {
		return "", fmt.Errorf("%w: %q — one of %s",
			ErrUnknownTLSMode, mode, strings.Join(TLSModesFor(driver), ", "))
	}

	if driver == DriverSQLServer {
		out := dsn
		for key, value := range sqlServerTLSOptions[mode] {
			out = setConnectionOption(out, key, value)
		}
		return out, nil
	}
	return setConnectionOption(dsn, "sslmode", mode), nil
}

// setConnectionOption sets a parameter, replacing one already there.
//
// Replacing in place keeps the parameter where its author put it, so comparing
// the two strings shows only a value changing. Two of the same parameter is a
// connection nobody can reason about, which is why this never simply appends.
func setConnectionOption(dsn, key, value string) string {
	if replaced, ok := replaceKeywordValue(dsn, key, value); ok {
		return replaced
	}
	// The keyword-value spellings differ: Postgres separates pairs with spaces,
	// SQL Server with semicolons. A URL uses "?" then "&".
	if !strings.Contains(dsn, "://") {
		if strings.Contains(dsn, ";") {
			return strings.TrimSuffix(dsn, ";") + ";" + key + "=" + value
		}
		return dsn + " " + key + "=" + value
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + key + "=" + value
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

func isTLSMode(driver, mode string) bool {
	for _, valid := range TLSModesFor(driver) {
		if mode == valid {
			return true
		}
	}
	return false
}
