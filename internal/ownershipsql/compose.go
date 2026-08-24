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
// string with the address, the database and the account in it, and marks where
// the password goes. Only the password is hidden, because it is the only value
// nobody can inspect; hiding the rest is what made a fortnight of failures
// unreadable.
//
// The position is marked rather than worked out. A connection can want its
// password somewhere this code would not have guessed, and guessing produces a
// string that reads correctly and is refused — which is the failure this whole
// journey exists to end. So the marker is required, and a connection without
// one is refused rather than repaired: sending the literal marker to a server
// would authenticate as nobody and read as a wrong password.
//
// Every escaping rule below was MEASURED against a running server, not read
// from documentation — see compose_functional_test.go. Reasoning about them
// does not work: brace-quoting an ADO password is accepted by the parser,
// arrives with the braces still attached, and comes back as "login failed" — a
// wrong password rather than a bad string.

// PasswordMarker is where the password goes. The administrator positions it;
// the proposed starting connection already contains one, so most people never
// have to think about it.
//
// Chosen to survive being passed around, which is not a small thing: these
// connections get pasted into scripts, Makefiles and shells on the way to us.
// Measured rather than assumed, because these spellings do not survive:
//
//   - "${password}" is expanded by every shell, silently, to nothing.
//   - "[password]" is a glob character class — it became "d" in both bash and
//     zsh purely because a file named "d" was in the directory.
//   - "<password>" is redirection, "#password#" is a comment, and "%password%"
//     collides with percent-encoding in the URL spelling.
//
// Letters and underscores have no meaning to a shell, to make, to a URL or to a
// keyword connection string. It also says what it is, which is the one part of
// "tell me how to mark it" that survives being pasted somewhere with no screen
// attached.
const PasswordMarker = "PASSWORD_GOES_HERE"

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

// ErrNoPasswordMarker is returned when a connection does not say where the
// password goes. It is its own error so a screen can name the marker rather
// than repeating a sentence.
var ErrNoPasswordMarker = fmt.Errorf(
	"the connection does not say where the password goes: put %s where it belongs", PasswordMarker)

// Compose puts the password where the connection says it goes, escaped for the
// form the connection is written in and the driver it is going to.
func Compose(driver, connection, password string) (Composed, error) {
	if !IsSupportedDriver(driver) {
		return Composed{}, fmt.Errorf("ownershipsql: unsupported driver %q", driver)
	}
	trimmed := strings.TrimSpace(connection)
	if trimmed == "" {
		return Composed{}, fmt.Errorf("ownershipsql: the connection is empty")
	}
	if !strings.Contains(trimmed, PasswordMarker) {
		return Composed{}, fmt.Errorf("ownershipsql: %w", ErrNoPasswordMarker)
	}

	form := DetectForm(trimmed)
	if err := schemeAgreesWithDriver(form, driver, trimmed); err != nil {
		return Composed{}, err
	}
	prepared, err := prepareVisibleParts(form, trimmed)
	if err != nil {
		return Composed{}, err
	}

	// Whether the administrator already wrote the quotes around the marker
	// decides whether this adds its own. Quoting an already-quoted value gives
	// the driver two wrappers, and it strips one — so the password arrives with
	// stray quotation marks and the login is refused.
	quoted := markerIsAlreadyQuoted(prepared, form, driver)
	escape := escaperFor(form, driver, quoted)

	build := func(secret string) string {
		return strings.ReplaceAll(prepared, PasswordMarker, escape(secret))
	}
	return Composed{DSN: build(password), Masked: build(PasswordMask), Form: form}, nil
}

// ---------------------------------------------------------------------------
// The scheme names the database
// ---------------------------------------------------------------------------

// DriverNamedByScheme reports which database a URL scheme names, so a screen
// can derive the database from the connection instead of asking twice.
//
// A URL-shaped connection already says which database it is for. Asking again
// alongside it is not merely redundant — the two can disagree.
func DriverNamedByScheme(scheme string) (string, bool) {
	switch strings.ToLower(scheme) {
	case "sqlserver":
		return DriverSQLServer, true
	case "postgres", "postgresql":
		return DriverPostgres, true
	}
	return "", false
}

// schemeAgreesWithDriver refuses a connection whose scheme names a different
// database from the one chosen, and one whose scheme names nothing either
// driver understands.
//
// Worth refusing rather than passing on, because neither driver says the
// database is the wrong kind. Given a "postgres://" connection the SQL Server
// driver reads the string as keyword pairs, finds no account, and the server
// answers "Login failed for user ''", which reads as a broken credential.
// libpq handed a "sqlserver://" connection gets as far as "SSL is not enabled
// on the server", which reads as a TLS problem.
func schemeAgreesWithDriver(form Form, driver, connection string) error {
	if form != FormURL {
		// The keyword spellings carry no scheme, so there is nothing to check
		// and the database has to be said some other way.
		return nil
	}
	scheme, _, isURL := splitConnectionScheme(connection)
	if !isURL {
		return nil
	}
	named, known := DriverNamedByScheme(scheme)
	if !known {
		return fmt.Errorf(
			"ownershipsql: %q is not a database this reads; the connection should begin "+
				"sqlserver:// or postgres://", scheme+"://")
	}
	if named != driver {
		return fmt.Errorf(
			"ownershipsql: this connection begins %s://, which is %s, but %s was chosen",
			scheme, named, driver)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Preparing the parts the administrator can see
// ---------------------------------------------------------------------------

// prepareVisibleParts escapes the things that cannot travel as written.
//
// Only the account, and only in a URL: a Windows domain login carries a
// backslash that no URL can hold. This is not a quiet rewrite — what arrives at
// the server is the account as typed, and the composed connection is shown, so
// the encoding is on screen rather than behind them.
func prepareVisibleParts(form Form, connection string) (string, error) {
	if form != FormURL {
		// The keyword spellings need nothing encoded: what the administrator
		// wrote is what the parser reads.
		return connection, nil
	}

	scheme, rest, isURL := splitConnectionScheme(connection)
	if !isURL {
		return "", fmt.Errorf("ownershipsql: %q is not a connection URL", connection)
	}
	userinfo := userinfoOfConnection(rest)
	if userinfo == "" {
		// No account, so nothing to encode. The marker is elsewhere in the
		// string and that is the administrator's business.
		return connection, nil
	}

	// The account is what precedes the first ":", which is what separates it
	// from the password. If the marker is in that position there is no account
	// to encode.
	account, remainder, hasSeparator := strings.Cut(userinfo, ":")
	if strings.Contains(account, PasswordMarker) {
		return connection, nil
	}
	encoded := percentEncode(account, accountAllows)
	if !hasSeparator {
		return scheme + "://" + encoded + rest[len(userinfo):], nil
	}
	return scheme + "://" + encoded + ":" + remainder + rest[len(userinfo):], nil
}

// accountAllows reports whether a byte may sit in the user portion as it
// stands. It is the unreserved and sub-delimiter set of RFC 3986 — and unlike a
// password, ":" is excluded, because that is what separates the account from
// the password.
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
// Escaping, by form
// ---------------------------------------------------------------------------

// quoteCharsFor is the pair the administrator may have written around the
// marker themselves. A URL has no such pair.
func quoteCharsFor(form Form, driver string) (open, close string) {
	switch {
	case form == FormODBC:
		return "{", "}"
	case form == FormKeyword && driver == DriverPostgres:
		return "'", "'"
	case form == FormKeyword:
		return `"`, `"`
	default:
		return "", ""
	}
}

// markerIsAlreadyQuoted reports whether every marker is written inside the
// quotes its form uses, so this does not add a second pair.
func markerIsAlreadyQuoted(connection string, form Form, driver string) bool {
	open, close := quoteCharsFor(form, driver)
	if open == "" {
		return false
	}
	found := false
	for i := 0; ; {
		at := strings.Index(connection[i:], PasswordMarker)
		if at < 0 {
			break
		}
		at += i
		before := at > 0 && string(connection[at-1]) == open
		afterAt := at + len(PasswordMarker)
		after := afterAt < len(connection) && string(connection[afterAt]) == close
		if !before || !after {
			return false
		}
		found = true
		i = afterAt
	}
	return found
}

// escaperFor returns how a secret is written for this form. When the marker is
// already wrapped in the form's quotes, only the inside is escaped — the
// wrapper is the administrator's and is left alone.
func escaperFor(form Form, driver string, alreadyQuoted bool) func(string) string {
	switch {
	case form == FormURL:
		return func(s string) string { return percentEncode(s, passwordAllows) }
	case form == FormODBC:
		if alreadyQuoted {
			return odbcEscapeInner
		}
		return odbcQuote
	case driver == DriverPostgres:
		if alreadyQuoted {
			return postgresEscapeInner
		}
		return postgresQuote
	default:
		if alreadyQuoted {
			return adoEscapeInner
		}
		return adoQuote
	}
}

// adoEscapeInner doubles a double quote, which is how SQL Server's ";"-separated
// keyword spelling carries one inside a quoted value.
//
// Measured: the driver strips one pair of surrounding double quotes and
// collapses each doubled double quote inside them. Braces mean nothing to it,
// so a brace-quoted password arrives with its braces attached and the server
// answers "login failed for user" — a wrong credential rather than a bad
// string, which sends the search to the account instead of the tooling.
func adoEscapeInner(v string) string { return strings.ReplaceAll(v, `"`, `""`) }

func adoQuote(v string) string { return `"` + adoEscapeInner(v) + `"` }

// odbcEscapeInner doubles a closing brace, which is how the "odbc:" spelling
// carries one. This is why the rule cannot be chosen by driver alone: the same
// driver reads both spellings, and they disagree.
func odbcEscapeInner(v string) string { return strings.ReplaceAll(v, "}", "}}") }

func odbcQuote(v string) string { return "{" + odbcEscapeInner(v) + "}" }

// postgresEscapeInner backslash-escapes what libpq's space-separated keyword
// spelling treats as special inside single quotes.
func postgresEscapeInner(v string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v)
}

func postgresQuote(v string) string { return `'` + postgresEscapeInner(v) + `'` }
