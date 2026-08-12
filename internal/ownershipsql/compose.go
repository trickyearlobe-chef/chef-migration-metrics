// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"fmt"
	"strings"
)

// Composing a connection from a part somebody can read and a part they cannot.
//
// See journeys/ownership-connection.md. The administrator holds one connection
// string with the address, the database and the account in it, and the password
// is kept apart and put in here. Only the password is hidden, because it is the
// only value nobody can inspect; hiding the rest is what made a fortnight of
// failures unreadable.
//
// Every escaping rule below was MEASURED against a running server, not read from
// documentation — see compose_functional_test.go. Reasoning about them produced
// several confident wrong answers, and one of those wrong answers had already
// been written into the plan: brace-quoting an ADO password is accepted by the
// parser, arrives with the braces still attached, and comes back as "login
// failed" — a wrong password rather than a bad string.

// PasswordMask stands in for the password wherever a connection is shown. It is
// a fixed width, so it does not leak the length of the password.
const PasswordMask = "********"

// Form is the shape a connection string arrives in. The escaping rule differs
// by form, and applying one form's rule to another produces a string that reads
// correctly and is refused — so the form has to be recognised, not assumed.
type Form string

const (
	// FormURL is the "scheme://user:password@host/db?opts" spelling.
	FormURL Form = "url"
	// FormKeyword is "key=value" repeated: separated by ";" for SQL Server and
	// by spaces for PostgreSQL.
	FormKeyword Form = "keyword"
	// FormODBC is SQL Server's "odbc:" prefixed keyword spelling, which quotes
	// with braces rather than double quotes.
	FormODBC Form = "odbc"
)

// DetectForm reports which spelling a connection string is written in, by the
// same rules the driver applies when it reads one.
func DetectForm(connection string) Form {
	trimmed := strings.TrimSpace(connection)
	if len(trimmed) >= 5 && strings.EqualFold(trimmed[:5], "odbc:") {
		return FormODBC
	}
	if _, _, isURL := splitConnectionScheme(trimmed); isURL {
		return FormURL
	}
	return FormKeyword
}

// Composed is a connection with its password put in, in both the spelling that
// goes to the driver and the spelling that goes on a screen.
//
// The two are produced in one pass from the same parts, so the masked view
// cannot drift from what is actually sent. A masked view derived separately
// would eventually show a connection that was not the one being used, which is
// the failure this whole journey exists to end.
type Composed struct {
	// DSN is the real connection. It carries the password, so it is never
	// logged, never returned to a client and never put in an error.
	DSN string
	// Masked is the same connection with the password replaced by PasswordMask.
	// It is safe to show, screenshot and put in a support bundle.
	Masked string
	// Form is the spelling that was recognised, so a caller can say which
	// escaping rule was applied.
	Form Form
}

// Compose puts the password into a visible connection string, escaped for the
// form the string is written in and the driver it is going to.
//
// The visible connection must not already carry a password. Two passwords in
// one connection is not a thing to guess at: whichever won, the administrator
// would be reading one and sending the other.
func Compose(driver, visible, password string) (Composed, error) {
	if !IsSupportedDriver(driver) {
		return Composed{}, fmt.Errorf("ownershipsql: unsupported driver %q", driver)
	}
	trimmed := strings.TrimSpace(visible)
	if trimmed == "" {
		return Composed{}, fmt.Errorf("ownershipsql: the connection is empty")
	}

	form := DetectForm(trimmed)
	build, err := composerFor(driver, form, trimmed)
	if err != nil {
		return Composed{}, err
	}
	return Composed{
		DSN:    build(password),
		Masked: build(PasswordMask),
		Form:   form,
	}, nil
}

// composerFor validates the visible connection once and returns a function that
// writes a given secret into it. Returning a builder rather than a string is
// what guarantees the masked view and the real connection differ in exactly one
// place: they are the same code run twice.
func composerFor(driver string, form Form, visible string) (func(secret string) string, error) {
	switch form {
	case FormURL:
		return urlComposer(visible)
	case FormODBC:
		return keywordComposer(visible, ";", odbcQuote)
	default:
		if driver == DriverPostgres {
			return keywordComposer(visible, " ", postgresQuote)
		}
		return keywordComposer(visible, ";", adoQuote)
	}
}

// ---------------------------------------------------------------------------
// The URL spelling
// ---------------------------------------------------------------------------

func urlComposer(visible string) (func(string) string, error) {
	scheme, rest, isURL := splitConnectionScheme(visible)
	if !isURL {
		return nil, fmt.Errorf("ownershipsql: %q is not a connection URL", visible)
	}
	account := userinfoOfConnection(rest)
	if account == "" {
		return nil, fmt.Errorf(
			"ownershipsql: the connection names no account, so there is nowhere to put the password")
	}
	if strings.Contains(account, ":") {
		return nil, fmt.Errorf(
			"ownershipsql: the connection already carries a password; remove it — " +
				"the password is held separately and put in for you")
	}
	tail := rest[len(account):] // starts at the "@"

	// The account is percent-encoded because a Windows domain login contains a
	// backslash, which no URL can carry, and that is the customer's own account.
	// This is not a quiet rewrite: what arrives at the server is the account as
	// typed — measured through the driver's parser — and the composed string is
	// shown, so the encoding is on screen rather than behind them.
	encodedAccount := percentEncode(account, accountAllows)
	return func(secret string) string {
		return scheme + "://" + encodedAccount + ":" + percentEncode(secret, passwordAllows) + tail
	}, nil
}

// accountAllows reports whether a byte may sit in the user portion as it stands.
// It is the unreserved and sub-delimiter set of RFC 3986 — and unlike a
// password, ":" is excluded, because that is what separates the account from the
// password that is about to be appended.
func accountAllows(b byte) bool { return mayAppearInACredential(b) && b != ':' }

// passwordAllows is the same set with ":" permitted: everything after the first
// ":" is the password, so a colon inside one is unambiguous.
func passwordAllows(b byte) bool { return mayAppearInACredential(b) }

func percentEncode(s string, allowed func(byte) bool) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if allowed(c) {
			out.WriteByte(c)
			continue
		}
		out.WriteByte('%')
		out.WriteByte(hexDigits[c>>4])
		out.WriteByte(hexDigits[c&0x0F])
	}
	return out.String()
}

// ---------------------------------------------------------------------------
// The keyword spellings
// ---------------------------------------------------------------------------

// keywordComposer appends a password setting to a key=value connection.
//
// The separator and the quoting rule are the two things that differ between the
// three keyword dialects, so they are the two things passed in.
func keywordComposer(visible, separator string, quote func(string) string) (func(string) string, error) {
	if key := existingPasswordKey(visible, separator); key != "" {
		return nil, fmt.Errorf(
			"ownershipsql: the connection already sets %q; remove it — "+
				"the password is held separately and put in for you", key)
	}
	prefix := strings.TrimRight(visible, separator+" ")
	return func(secret string) string {
		return prefix + separator + "password=" + quote(secret)
	}, nil
}

// passwordKeys are the spellings each dialect accepts for the same setting. A
// connection that sets any of them already has a password in it.
var passwordKeys = map[string]bool{"password": true, "pwd": true}

func existingPasswordKey(visible, separator string) string {
	for _, part := range strings.Split(visible, separator) {
		name, _, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "odbc:")))
		if passwordKeys[name] {
			return name
		}
	}
	return ""
}

// adoQuote wraps a value for SQL Server's ";"-separated keyword spelling.
//
// Measured, and it is the one that was written down backwards. The driver
// strips one pair of surrounding double quotes and collapses each doubled
// double quote inside them; braces mean nothing to it, so a brace-quoted
// password arrives with its braces attached and the server answers "login
// failed for user" — a wrong credential rather than a bad string, which sends
// the search to the account instead of the tooling.
func adoQuote(v string) string { return `"` + strings.ReplaceAll(v, `"`, `""`) + `"` }

// odbcQuote wraps a value for the "odbc:" spelling, which does use braces, and
// doubles a closing brace inside one. This is why the rule cannot be chosen by
// driver alone: the same driver reads both, and they disagree.
func odbcQuote(v string) string { return "{" + strings.ReplaceAll(v, "}", "}}") + "}" }

// postgresQuote wraps a value for libpq's space-separated keyword spelling,
// which quotes with single quotes and escapes with a backslash.
func postgresQuote(v string) string {
	return `'` + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + `'`
}
