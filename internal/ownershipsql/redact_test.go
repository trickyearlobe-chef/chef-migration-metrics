// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	mssql "github.com/microsoft/go-mssqldb"
)

// serverErrorCarrying stands in for what the driver returns when SQL Server
// aborts the process: a fixed string on the outside, the real message wrapped
// inside. The driver's own type cannot be built from here — the field holding
// the message is unexported — so this reproduces its shape, which is what the
// code under test reads. The real thing is exercised against a running server
// by TestFunctional_MSSQL_AnAbortedProcessSaysWhySQLServerAbortedIt.
type serverErrorShape struct{ inner error }

func (e serverErrorShape) Error() string { return "SQL Server had internal error" }
func (e serverErrorShape) Unwrap() error { return e.inner }

func serverErrorCarrying(message string, number int32, severity uint8, line int32) error {
	return serverErrorShape{inner: mssql.Error{
		Message: message, Number: number, Class: severity, LineNo: line,
	}}
}

// The password must not come back in anything anybody reads.
//
// This is the one on the list that fails dangerously rather than annoyingly.
// Both the thing that reads the connection and the server routinely quote what
// they were handed, and neither knows which part of it was a secret — so an
// unfiltered refusal puts the password on a screen, in a support bundle, and in
// logs a great many people can read.
//
// The escaped spelling is the case that gets missed, because by the time it is
// in a message it no longer looks like the password that was stored.
func TestRedactRemovesEverySpellingOfThePassword(t *testing.T) {
	for _, password := range awkwardPasswords {
		if strings.TrimSpace(password) == "" {
			continue
		}
		t.Run(password, func(t *testing.T) {
			// Every way the password can appear on its way into a connection.
			for name, spelling := range spellingsOf(password) {
				if strings.TrimSpace(spelling) == "" {
					continue
				}
				// A driver quoting the whole connection back at us.
				message := fmt.Sprintf(
					`unable to open connection "sqlserver://svc:%s@dbhost:1433?database=cmdb": refused`,
					spelling)

				// Baseline: it really is in there before redaction, or this
				// case proves nothing.
				if !strings.Contains(message, spelling) {
					t.Fatalf("the fixture proves nothing: the %s spelling is not in the message", name)
				}

				cleaned := RedactPassword(message, password)
				if strings.Contains(cleaned, spelling) {
					t.Errorf("the %s spelling survived redaction\n  password: %q\n  spelling: %q\n  left:     %s",
						name, password, spelling, cleaned)
				}
				if strings.Contains(cleaned, password) {
					t.Errorf("the password itself survived redaction of its %s spelling: %s",
						name, cleaned)
				}
			}
		})
	}
}

// Redaction must not eat the parts of the message worth reading. A refusal
// tidied into nothing is the other failure this journey names.
func TestRedactLeavesTheRestOfTheMessageAlone(t *testing.T) {
	const password = `pa%ss;wo rd#7Q!`
	message := `mssql: login error: Login failed for user 'EXAMPLECORP\svcaccount'. ` +
		`connection "sqlserver://EXAMPLECORP%5Csvcaccount:pa%25ss;wo%20rd%237Q!@dbhost:1433?database=cmdb"`

	cleaned := RedactPassword(message, password)
	for _, keep := range []string{
		"Login failed for user",
		`EXAMPLECORP\svcaccount`,
		"dbhost:1433",
		"database=cmdb",
	} {
		if !strings.Contains(cleaned, keep) {
			t.Errorf("redaction removed something worth reading (%q): %s", keep, cleaned)
		}
	}
	if strings.Contains(cleaned, "pa%25ss") || strings.Contains(cleaned, password) {
		t.Errorf("the password is still there: %s", cleaned)
	}
}

// An empty or whitespace password must not turn the whole message into masks.
func TestRedactWithNoPasswordChangesNothing(t *testing.T) {
	const message = "mssql: login error: Login failed for user 'svc'."
	for _, password := range []string{"", " ", "\t"} {
		if got := RedactPassword(message, password); got != message {
			t.Errorf("redacting with %q changed the message: %s", password, got)
		}
	}
}

// A short password that happens to be a substring of something innocent is
// still redacted — losing a word is the right trade against leaking a secret,
// and the alternative is deciding which occurrences "look like" the password.
func TestRedactIsWholesale(t *testing.T) {
	got := RedactPassword("connecting to host db.example.com", "db")
	if strings.Contains(got, "db.example.com") {
		t.Errorf("a password that appears inside other text was left: %s", got)
	}
}

// The error path is what actually matters: an error leaving this package with a
// password in it is the leak, whatever the helper does in isolation.
func TestRedactedErrorKeepsTheCauseChainable(t *testing.T) {
	sentinel := errors.New("refused by the server")
	wrapped := fmt.Errorf("connecting: %w (dsn=sqlserver://svc:hunter2@h)", sentinel)

	redacted := redactErr(wrapped, "hunter2")
	if strings.Contains(redacted.Error(), "hunter2") {
		t.Errorf("the password survived: %v", redacted)
	}
	if !errors.Is(redacted, sentinel) {
		t.Error("redacting broke the error chain, so callers can no longer tell what failed")
	}
}

func TestRedactErrNilStaysNil(t *testing.T) {
	if err := redactErr(nil, "hunter2"); err != nil {
		t.Errorf("a nil error became %v", err)
	}
}

// The driver hides its own message behind a fixed string, and we printed the
// fixed string.
//
// Measured against a running server 2026-08-14, after a customer read a table
// and got "SQL Server had internal error" and nothing else: when SQL Server
// aborts the process — severity 20 and up — go-mssqldb returns a ServerError
// whose Error() is that constant, with the real message wrapped inside it. The
// customer's whole diagnosis was one unwrap away and unreachable from a screen.
//
// The baseline is asserted first: the fixed string really is all the driver's
// own rendering gives, so this cannot pass because the wrapper started saying
// something useful of its own accord.
func TestADriverThatHidesItsMessageIsMadeToSayIt(t *testing.T) {
	hidden := mssql.ServerError{}
	if strings.Contains(hidden.Error(), "kill state") {
		t.Fatal("the fixture proves nothing: the driver's wrapper now carries the message " +
			"itself, so there is nothing left to dig out")
	}

	wrapped := fmt.Errorf("ownershipsql: running the query: %w", serverErrorCarrying(
		"Cannot continue the execution because the session is in the kill state.", 596, 21, 4))

	explained := explainDriverError(wrapped)
	for _, want := range []string{"kill state", "596", "severity 21", "line 4"} {
		if !strings.Contains(explained.Error(), want) {
			t.Errorf("the explanation does not carry %q, so the reader is no better off "+
				"than with the fixed string: %v", want, explained)
		}
	}
	// What was already there is kept: the reader needs to know which of our
	// steps it was, not only what the server said.
	if !strings.Contains(explained.Error(), "running the query") {
		t.Errorf("the explanation lost what we were doing: %v", explained)
	}
}

// Anything else is left exactly as it is. Most driver errors say what they mean
// and rewriting them is how the useful ones get lost.
func TestAnErrorThatSaysWhatItMeansIsLeftAlone(t *testing.T) {
	plain := fmt.Errorf("ownershipsql: connecting: %w",
		errors.New("unable to open tcp connection with host 'dbhost.example.com:1433'"))
	if explainDriverError(plain).Error() != plain.Error() {
		t.Errorf("an error that already reads well was rewritten:\n  was:  %v\n  now:  %v",
			plain, explainDriverError(plain))
	}
}

// The password is taken out of what the driver hid, exactly as it is taken out
// of everything else it says. A message dug out of a wrapper is still a message
// somebody will screenshot.
func TestTheHiddenMessageIsRedactedToo(t *testing.T) {
	const password = `pa%ss;wo rd#7Q!`
	wrapped := fmt.Errorf("ownershipsql: running the query: %w", serverErrorCarrying(
		"Login failed for user using password "+password, 18456, 14, 1))

	cleaned := redactErr(explainDriverError(wrapped), password)
	if strings.Contains(cleaned.Error(), password) {
		t.Errorf("the password came back inside the message that was dug out: %v", cleaned)
	}
	if !strings.Contains(cleaned.Error(), "Login failed") {
		t.Errorf("redaction took away the message it was meant to keep: %v", cleaned)
	}
}
