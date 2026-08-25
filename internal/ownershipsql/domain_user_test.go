// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"testing"

	"github.com/microsoft/go-mssqldb/msdsn"
)

// A Windows domain login, which is what a SQL Server estate actually uses.
//
// Such an account is written user="DOMAIN\" + username. Written into a URL, the
// backslash makes the whole string unparsable — net/url calls it "invalid
// userinfo" — and the driver reports "unable to parse connection string:
// invalid URL format", which names neither the character nor the field.
//
// Checked through the driver's own parser rather than ours, because what matters
// is not that we produce a URL but that the driver reads the domain user back
// out of it unchanged. An encoding that parsed but arrived as a different
// username would authenticate as nobody and look like a wrong password.
func TestDomainUserSurvivesToTheDriver(t *testing.T) {
	const (
		user     = `EXAMPLECORP\svcaccount`
		password = "s0mep%ss"
		host     = "dbhost.example.com"
		database = "Staging"
	)
	typed := "sqlserver://" + user + ":" + password + "@" + host +
		"?database=" + database + "&ApplicationIntent=ReadOnly&MultiSubnetFailover=True"

	// As typed it is refused, which is the whole reason this exists. If this ever
	// stops being true the test below proves nothing, so it is asserted.
	if _, err := msdsn.Parse(typed); err == nil {
		t.Fatal("the driver now accepts a backslash in a URL username; this test no longer has a subject")
	}

	cfg, err := msdsn.Parse(encodeCredentialForURL(typed))
	if err != nil {
		t.Fatalf("still refused after encoding the credential: %v", err)
	}

	if cfg.User != user {
		t.Errorf("the domain user did not survive\n  typed:   %q\n  arrives: %q", user, cfg.User)
	}
	if cfg.Password != password {
		t.Errorf("the password did not survive\n  typed:   %q\n  arrives: %q", password, cfg.Password)
	}
	if cfg.Host != host {
		t.Errorf("host changed: %q", cfg.Host)
	}
	if cfg.Database != database {
		t.Errorf("database changed: %q", cfg.Database)
	}
}
