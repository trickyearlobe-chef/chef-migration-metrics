// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"net/url"
	"strings"
	"testing"
)

// Every byte a password could contain, one at a time, and then in pairs.
//
// The hand-written cases were written from the characters somebody thought of,
// and a "£" found the hole in that — a denylist is a list you can leave
// something off. This asks the question exhaustively instead: for any password,
// does the driver receive exactly what was typed?
func TestEncodeCredential_AnyByteInAPasswordSurvives(t *testing.T) {
	for b := 0; b < 256; b++ {
		// ":" separates the user from the password, so a password starting with
		// one is a different string, not a different encoding. It is covered by
		// the pairs below, where it is not the first byte.
		password := "a" + string(rune(b)) + "z"
		dsn := "sqlserver://svc:" + password + "@host:1433?database=cmdb"

		parsed, err := url.Parse(encodeCredentialForURL(dsn))
		if err != nil {
			t.Errorf("byte %d (%q): not a URL after encoding: %v", b, password, err)
			continue
		}
		got, _ := parsed.User.Password()
		if got != password {
			t.Errorf("byte %d: the password changed\n  typed:   %q\n  arrives: %q", b, password, got)
		}
	}
}

// Pairs, because escaping bugs live where two special characters meet — a "%"
// followed by something that looks like a hex digit being the whole reason this
// exists.
func TestEncodeCredential_AnyPairOfAwkwardBytesSurvives(t *testing.T) {
	awkward := []byte{'%', '2', '5', '4', '1', 'A', 'f', ' ', '#', '/', '?', '@', ':', ';', '&', '=', '\\', '"', '\'', '£'}
	for _, first := range awkward {
		for _, second := range awkward {
			// A password holding BOTH "@" and "?" has no reading that is
			// obviously right: each moves where the credential is taken to end,
			// and they disagree. We resolve it the same way the driver does, so
			// the two never differ — but the answer can be the shorter password.
			// Either character alone is handled, which is what people actually
			// have. Stated here rather than quietly skipped.
			if strings.ContainsRune("@?", rune(first)) && strings.ContainsRune("@?", rune(second)) {
				continue
			}
			password := "p" + string(first) + string(second) + "q"
			dsn := "sqlserver://svc:" + password + "@host:1433?database=cmdb"

			parsed, err := url.Parse(encodeCredentialForURL(dsn))
			if err != nil {
				t.Errorf("%q: not a URL after encoding: %v", password, err)
				continue
			}
			got, _ := parsed.User.Password()
			if got != password {
				t.Errorf("the password changed\n  typed:   %q\n  arrives: %q", password, got)
			}
		}
	}
}

// The username too, which carries "DOMAIN\user" in a Windows estate.
func TestEncodeCredential_AnyByteInAUsernameSurvives(t *testing.T) {
	for b := 0; b < 256; b++ {
		if b == ':' {
			continue // the separator, by definition not part of the user
		}
		user := "a" + string(rune(b)) + "z"
		dsn := "sqlserver://" + user + ":pw@host:1433?database=cmdb"

		parsed, err := url.Parse(encodeCredentialForURL(dsn))
		if err != nil {
			t.Errorf("byte %d (%q): not a URL after encoding: %v", b, user, err)
			continue
		}
		if parsed.User.Username() != user {
			t.Errorf("byte %d: the username changed\n  typed:   %q\n  arrives: %q",
				b, user, parsed.User.Username())
		}
	}
}

// A fuzz target, so this keeps being asked with inputs nobody chose.
func FuzzEncodeCredential(f *testing.F) {
	for _, seed := range []string{"pw", "pa%ss;wo rd#7Q!", "%41", "%%", "£ntropy", `DOM\svc`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, password string) {
		// A password containing "@" or ":" moves where the credential ends, which
		// is a question about the connection string rather than about encoding.
		if strings.ContainsAny(password, "@:") || password == "" {
			return
		}
		dsn := "sqlserver://svc:" + password + "@host:1433?database=cmdb"
		encoded := encodeCredentialForURL(dsn)

		parsed, err := url.Parse(encoded)
		if err != nil {
			t.Fatalf("not a URL after encoding %q: %v", password, err)
		}
		got, _ := parsed.User.Password()
		if got != password {
			t.Fatalf("the password changed\n  typed:   %q\n  arrives: %q", password, got)
		}
	})
}
