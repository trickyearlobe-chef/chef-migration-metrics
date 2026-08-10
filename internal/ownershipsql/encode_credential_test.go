// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"net/url"
	"strings"
	"testing"
)

// A password is not a URL, and the person who typed it should not have to know
// that it is about to be treated as one.
//
// A customer's connection was refused by the driver as "invalid URL format".
// Everything visible in it was legal — proved against the driver's own parser —
// so the cause was a character in the user or password that a URL cannot carry
// unescaped. The password is encrypted and nobody at hand knows it, so it cannot
// be retyped in another form. The only fix available is to encode it on the way
// to the driver.
//
// Only the userinfo is touched. It is a bounded region with a known meaning; the
// host, the database and the vendor options are left exactly as written.

func TestEncodeCredential_LeavesAWorkingConnectionAlone(t *testing.T) {
	for _, dsn := range []string{
		"sqlserver://svc:pw@host:1433?database=cmdb",
		"sqlserver://svc:pw@host:1433?database=cmdb&ApplicationIntent=ReadOnly",
		// Already encoded, and must not be encoded twice.
		"sqlserver://svc:pa%25ss@host:1433?database=cmdb",
		// No credential at all.
		"sqlserver://host:1433?database=cmdb",
		// Not a URL, so there is no userinfo to find.
		"Server=host;Database=cmdb;User Id=svc;Password=pw",
	} {
		got := encodeCredentialForURL(dsn)
		if got != dsn {
			t.Errorf("changed a connection that did not need it\n  before: %s\n  after:  %s", dsn, got)
		}
	}
}

// The characters measured to break a URL, each of which the driver refuses.
func TestEncodeCredential_EncodesWhatAURLCannotCarry(t *testing.T) {
	cases := []struct{ dsn, wantUserinfo string }{
		{"sqlserver://svc:pw%rd@host:1433?database=cmdb", "svc:pw%25rd"},
		{"sqlserver://svc:pw rd@host:1433?database=cmdb", "svc:pw%20rd"},
		{"sqlserver://svc:pw#rd@host:1433?database=cmdb", "svc:pw%23rd"},
		{"sqlserver://svc:pw/rd@host:1433?database=cmdb", "svc:pw%2Frd"},
		{`sqlserver://DOM\svc:pw@host:1433?database=cmdb`, "DOM%5Csvc:pw"},
		// Several at once, including one already encoded, which stays as it is.
		{"sqlserver://svc:a b%c%25d@host:1433?database=cmdb", "svc:a%20b%25c%25d"},
	}
	for _, c := range cases {
		got := encodeCredentialForURL(c.dsn)
		if !strings.Contains(got, c.wantUserinfo+"@") {
			t.Errorf("wrong encoding\n  from: %s\n  got:  %s\n  want userinfo: %s", c.dsn, got, c.wantUserinfo)
		}
		// The rest of the connection must be untouched.
		if !strings.HasSuffix(got, "@host:1433?database=cmdb") {
			t.Errorf("changed something outside the credential\n  from: %s\n  got:  %s", c.dsn, got)
		}
	}
}

// The result has to be a URL, which is the entire point. net/url is the judge
// here — it is the thing the driver uses.
func TestEncodeCredential_ResultParsesAsAURL(t *testing.T) {
	for _, dsn := range []string{
		"sqlserver://svc:pw%rd@host:1433?database=cmdb",
		"sqlserver://svc:pw rd@host:1433?database=cmdb",
		"sqlserver://svc:pw#rd@host:1433?database=cmdb",
		"sqlserver://svc:pw/rd@host:1433?database=cmdb",
		`sqlserver://DOM\svc:pw@host:1433?database=cmdb`,
		"sqlserver://svc:p@ss%wo:rd@host:1433?database=cmdb",
	} {
		encoded := encodeCredentialForURL(dsn)
		if _, err := url.Parse(encoded); err != nil {
			t.Errorf("still not a URL after encoding\n  from: %s\n  err:  %v", dsn, err)
		}
	}
}

// The password must survive intact, or this turns a loud parse failure into a
// quiet "wrong password" — which is worse, because it looks like the credential
// is wrong rather than the tooling.
func TestEncodeCredential_PasswordDecodesBackToWhatWasTyped(t *testing.T) {
	for _, password := range []string{
		"pw%rd", "pw rd", "pw#rd", "pw/rd", "p@ssw0rd", "p:ssw0rd",
		"pa%ss;wo rd#7Q!", "%", "%%", "%25", "already%20encoded",
	} {
		dsn := "sqlserver://svc:" + password + "@host:1433?database=cmdb"
		parsed, err := url.Parse(encodeCredentialForURL(dsn))
		if err != nil {
			t.Errorf("%q: not a URL after encoding: %v", password, err)
			continue
		}
		got, _ := parsed.User.Password()
		// "%25" and "already%20encoded" are left alone deliberately: a valid
		// escape is assumed to be one, since that is what anybody writing a
		// connection string means by it. Everything else round-trips exactly.
		if isAlreadyEncoded(password) {
			continue
		}
		if got != password {
			t.Errorf("password did not survive\n  typed:  %q\n  became: %q", password, got)
		}
	}
}

func isAlreadyEncoded(password string) bool {
	return password == "%25" || password == "already%20encoded"
}
