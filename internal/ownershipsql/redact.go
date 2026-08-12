// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"sort"
	"strings"
)

// Taking the password back out of whatever comes back.
//
// See journeys/ownership-connection.md: "except for my password, which must
// never come back to me in a message". The thing that reads the connection and
// the server both routinely quote what they were handed, and neither knows
// which part of it was a secret. This estate ships its logs to a Splunk a great
// many people can read, and screenshots and support bundles carry the same
// text, so an unfiltered refusal is a disclosure.
//
// The escaped spellings are the ones that get missed. By the time a password is
// inside a driver's complaint it has been percent-encoded, or wrapped in quotes
// with its own quotes doubled, and it no longer looks like the password that
// was stored — so redacting only what was typed leaves the leak intact.

// RedactPassword removes every spelling of the password from text, replacing
// each with the mask.
//
// Wholesale, not clever: every occurrence goes, even where it happens to sit
// inside something innocent. Losing a word out of a message is the right trade
// against leaking a credential, and the alternative is a rule that decides which
// occurrences "look like" the password — which is the kind of judgement that is
// wrong once and then wrong in production.
func RedactPassword(text, password string) string {
	if strings.TrimSpace(password) == "" {
		// Nothing to hide, and redacting an empty string would mask the whole
		// message one character at a time.
		return text
	}
	replacements := make([]string, 0, 2*len(passwordSpellings(password)))
	for _, spelling := range passwordSpellings(password) {
		replacements = append(replacements, spelling, PasswordMask)
	}
	return strings.NewReplacer(replacements...).Replace(text)
}

// passwordSpellings is every shape the password can take between being stored
// and appearing in somebody's error message, longest first.
//
// Longest first matters: the quoted spellings contain the escaped one, and
// replacing the short form first would leave the wrapper behind as a fragment
// that still shows the shape of what was there.
func passwordSpellings(password string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if strings.TrimSpace(s) == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}

	add(password)
	add(percentEncode(password, passwordAllows))
	add(percentEncode(password, accountAllows))
	add(adoQuote(password))
	add(adoEscapeInner(password))
	add(odbcQuote(password))
	add(odbcEscapeInner(password))
	add(postgresQuote(password))
	add(postgresEscapeInner(password))

	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// redactErr returns err with every spelling of the password taken out, while
// keeping it chainable — callers still need errors.Is to work, because which
// of the five things failed is decided by the cause and not by the words.
func redactErr(err error, password string) error {
	if err == nil {
		return nil
	}
	cleaned := RedactPassword(err.Error(), password)
	if cleaned == err.Error() {
		return err
	}
	return redactedError{message: cleaned, cause: err}
}

// redactedError carries the tidied message and keeps the original reachable for
// errors.Is and errors.As — but never for printing.
type redactedError struct {
	message string
	cause   error
}

func (e redactedError) Error() string { return e.message }

// Unwrap exposes the cause for matching. Anything that prints the result of
// Unwrap rather than the error itself puts the password back, which is why
// nothing in this package does.
func (e redactedError) Unwrap() error { return e.cause }

var _ error = redactedError{}
