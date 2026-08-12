// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"strings"
	"testing"

	"github.com/microsoft/go-mssqldb/msdsn"
)

// What a composed connection has to survive. These are not decorative: the "%"
// and the ";" are from the customer's own password, the backslash is from their
// domain login, and the leading and trailing spaces are the ones a keyword
// parser silently trims.
var awkwardPasswords = []string{
	`plain`,
	`pa%ss;wo rd#7Q!`,
	`semi;colon`,
	`equals=sign`,
	`space bar`,
	`quote"double`,
	`quote'single`,
	`ends"quote`,
	`"`,
	`""`,
	`back\slash`,
	`brace{open}close`,
	`}close`,
	`trailing `,
	` leading`,
	`£pound`,
	`@at`,
	`colon:inside`,
}

// The account a SQL Server estate actually uses. The backslash is the character
// that made the customer's connection unparsable.
const domainAccount = `EXAMPLECORP\svcaccount`

// The password arrives at the driver as it was typed, in the URL spelling.
//
// The baseline is asserted first — the same password written in unescaped is
// refused or arrives wrong — so this cannot pass because the driver became
// tolerant of something for an unrelated reason.
func TestComposeURLFormReachesTheDriverIntact(t *testing.T) {
	for _, password := range awkwardPasswords {
		t.Run(password, func(t *testing.T) {
			visible := "sqlserver://" + domainAccount + "@dbhost.example.com:1433?database=Staging"

			// Baseline: written in by hand, without composing.
			naive := "sqlserver://" + domainAccount + ":" + password +
				"@dbhost.example.com:1433?database=Staging"
			if cfg, err := msdsn.Parse(naive); err == nil &&
				cfg.Password == password && cfg.User == domainAccount {
				t.Fatalf("the fixture proves nothing: %q needs no escaping in a URL, "+
					"so this case is not measuring composition", password)
			}

			composed, err := Compose(DriverSQLServer, visible, password)
			if err != nil {
				t.Fatalf("composing: %v", err)
			}
			cfg, err := msdsn.Parse(composed.DSN)
			if err != nil {
				t.Fatalf("the composed connection is unparsable: %v", err)
			}
			if cfg.Password != password {
				t.Errorf("password did not survive\n  typed:   %q\n  arrives: %q", password, cfg.Password)
			}
			if cfg.User != domainAccount {
				t.Errorf("account did not survive\n  typed:   %q\n  arrives: %q", domainAccount, cfg.User)
			}
			if cfg.Host != "dbhost.example.com" || cfg.Database != "Staging" {
				t.Errorf("the rest of the connection changed: host=%q database=%q", cfg.Host, cfg.Database)
			}
		})
	}
}

// The same, in SQL Server's keyword spelling — the one the recorded plan had
// backwards. Brace-quoting here is accepted by the parser and arrives wrong.
func TestComposeKeywordFormReachesTheDriverIntact(t *testing.T) {
	for _, password := range awkwardPasswords {
		t.Run(password, func(t *testing.T) {
			visible := "server=dbhost.example.com;port=1433;database=Staging;user id=" + domainAccount

			composed, err := Compose(DriverSQLServer, visible, password)
			if err != nil {
				t.Fatalf("composing: %v", err)
			}
			cfg, err := msdsn.Parse(composed.DSN)
			if err != nil {
				t.Fatalf("the composed connection is unparsable: %v", err)
			}
			if cfg.Password != password {
				t.Errorf("password did not survive\n  typed:   %q\n  arrives: %q\n  sent:    %s",
					password, cfg.Password, composed.Masked)
			}
			if cfg.User != domainAccount {
				t.Errorf("account did not survive\n  typed:   %q\n  arrives: %q", domainAccount, cfg.User)
			}
			if cfg.Host != "dbhost.example.com" || cfg.Database != "Staging" {
				t.Errorf("the rest of the connection changed: host=%q database=%q", cfg.Host, cfg.Database)
			}
		})
	}
}

// The baseline for the keyword spelling, asserted separately because most
// passwords need no quoting there and a per-case check would be false.
//
// These four are the ones the keyword parser gets wrong when a password is
// appended raw: a ";" ends the value early, and leading or trailing spaces are
// trimmed off. If this ever stops failing, the round-trip test above is
// measuring nothing and should be retired rather than trusted.
func TestTheKeywordSpellingNeedsQuotingAtAll(t *testing.T) {
	visible := "server=dbhost.example.com;port=1433;database=Staging;user id=" + domainAccount
	for _, password := range []string{`pa%ss;wo rd#7Q!`, `semi;colon`, `trailing `, ` leading`} {
		cfg, err := msdsn.Parse(visible + ";password=" + password)
		if err != nil {
			continue // refused outright is also "not silently wrong"
		}
		if cfg.Password == password {
			t.Errorf("appending %q raw now arrives intact, so quoting it is no longer "+
				"what makes the round-trip test pass", password)
		}
	}
}

// The "odbc:" spelling is a third shape the same driver reads, and it quotes
// differently again. This is the evidence for why the rule cannot be chosen by
// driver alone.
func TestComposeODBCFormReachesTheDriverIntact(t *testing.T) {
	for _, password := range awkwardPasswords {
		t.Run(password, func(t *testing.T) {
			visible := "odbc:server=dbhost.example.com;database=Staging;user id=" + domainAccount

			composed, err := Compose(DriverSQLServer, visible, password)
			if err != nil {
				t.Fatalf("composing: %v", err)
			}
			if composed.Form != FormODBC {
				t.Fatalf("form = %q, want %q", composed.Form, FormODBC)
			}
			cfg, err := msdsn.Parse(composed.DSN)
			if err != nil {
				t.Fatalf("the composed connection is unparsable: %v", err)
			}
			if cfg.Password != password {
				t.Errorf("password did not survive\n  typed:   %q\n  arrives: %q", password, cfg.Password)
			}
		})
	}
}

// Applying one form's rule to the other is the silent failure this journey
// exists to end, so it is pinned as a fact rather than left as a warning: the
// wrong rule produces a string that parses and carries a password nobody typed.
func TestTheWrongQuotingRuleIsAcceptedAndArrivesWrong(t *testing.T) {
	const password = `pa%ss;wo rd#7Q!`
	visible := "server=dbhost.example.com;database=Staging;user id=" + domainAccount

	cfg, err := msdsn.Parse(visible + ";password=" + odbcQuote(password))
	if err != nil {
		t.Skip("brace-quoting an ADO password is now refused outright, which is safer " +
			"than the behaviour this test was written against")
	}
	if cfg.Password == password {
		t.Fatal("brace-quoting an ADO password now works; the composer should be " +
			"reviewed and this test retired")
	}
	if !strings.Contains(cfg.Password, "{") {
		t.Errorf("expected the braces to arrive as part of the password, got %q", cfg.Password)
	}
}

// The masked view is the composed connection with the password replaced, and
// nothing else. It is produced by the same code, so it cannot drift from what
// is sent — which is the whole point of showing it.
func TestMaskedIsTheSameConnectionWithOnlyThePasswordReplaced(t *testing.T) {
	visibles := map[string]string{
		DriverSQLServer: "sqlserver://" + domainAccount + "@dbhost.example.com:1433?database=Staging",
		DriverPostgres:  "host=dbhost.example.com dbname=staging user=svcaccount sslmode=require",
	}
	for driver, visible := range visibles {
		t.Run(driver, func(t *testing.T) {
			const password = `pa%ss;wo rd#7Q!`
			composed, err := Compose(driver, visible, password)
			if err != nil {
				t.Fatalf("composing: %v", err)
			}

			// Composing the mask itself must give the masked view exactly. If
			// these ever differ, the view is being derived a second way.
			asIfMasked, err := Compose(driver, visible, PasswordMask)
			if err != nil {
				t.Fatalf("composing the mask: %v", err)
			}
			if asIfMasked.DSN != composed.Masked {
				t.Errorf("the masked view is not the composed connection\n  masked:   %s\n  composed: %s",
					composed.Masked, asIfMasked.DSN)
			}
			if composed.Masked == composed.DSN {
				t.Error("the masked view is identical to the real connection, so the password is in it")
			}
			if !strings.Contains(composed.Masked, PasswordMask) {
				t.Errorf("the masked view does not show a mask: %s", composed.Masked)
			}
		})
	}
}

// The password must not come back in the masked view in any spelling — the
// escaped form is the one that will be missed, because it no longer looks like
// the password that was stored.
func TestTheMaskedViewNeverCarriesThePassword(t *testing.T) {
	visibles := map[string]string{
		DriverSQLServer: "sqlserver://" + domainAccount + "@dbhost.example.com:1433?database=Staging",
		DriverPostgres:  "host=dbhost.example.com dbname=staging user=svcaccount sslmode=require",
	}
	for driver, visible := range visibles {
		for _, password := range awkwardPasswords {
			composed, err := Compose(driver, visible, password)
			if err != nil {
				t.Fatalf("composing %q: %v", password, err)
			}
			for spelling, form := range spellingsOf(password) {
				if strings.TrimSpace(form) == "" {
					continue
				}
				if strings.Contains(composed.Masked, form) {
					t.Errorf("%s: the %s spelling of %q is in the masked view: %s",
						driver, spelling, password, composed.Masked)
				}
			}
		}
	}
}

// spellingsOf is every shape a password can take on its way into a connection.
// A redaction that only knows the stored spelling misses the rest.
func spellingsOf(password string) map[string]string {
	return map[string]string{
		"stored":         password,
		"percent-encode": percentEncode(password, passwordAllows),
		"ado-quoted":     adoQuote(password),
		"odbc-quoted":    odbcQuote(password),
		"postgres-quote": postgresQuote(password),
	}
}

// Nothing the administrator can see is rewritten behind them: the account and
// whatever stands in front of it arrive at the server exactly as typed.
//
// The prefix before a backslash is not always a domain. On a Windows box the
// machine's own hostname doubles as one, and "." means the same thing — an
// account local to that machine rather than to a directory. None of that
// changes what has to happen here: the whole account is the administrator's to
// write, and it must arrive as written whatever it means.
func TestTheAccountArrivesExactlyAsTyped(t *testing.T) {
	accounts := []string{
		`svcaccount`,
		`EXAMPLECORP\svcaccount`,
		`WINBOX\svclocal`,    // the machine's hostname standing in for a domain
		`.\svclocal`,         // the same thing written the short way
		`WORKGROUP\svclocal`, // a workgroup rather than a domain
		`svc.account`,
		`svc-account@example.com`,
		`svcaccount@corp.example.com`, // the UPN spelling of a domain account
		`spaced account`,
		`WINBOX\spaced account`,
	}
	spellings := map[string]func(string) string{
		"url": func(a string) string {
			return "sqlserver://" + a + "@dbhost.example.com:1433?database=Staging"
		},
		"keyword": func(a string) string {
			return "server=dbhost.example.com;database=Staging;user id=" + a
		},
	}

	for _, account := range accounts {
		for spelling, visibleFor := range spellings {
			t.Run(spelling+"/"+account, func(t *testing.T) {
				composed, err := Compose(DriverSQLServer, visibleFor(account), "irrelevant")
				if err != nil {
					t.Fatalf("composing: %v", err)
				}
				cfg, err := msdsn.Parse(composed.DSN)
				if err != nil {
					t.Fatalf("unparsable: %v", err)
				}
				if cfg.User != account {
					t.Errorf("the account was rewritten\n  typed:   %q\n  arrives: %q",
						account, cfg.User)
				}
			})
		}
	}
}

func TestDetectForm(t *testing.T) {
	cases := map[string]Form{
		"sqlserver://sa@localhost:1433?database=cmdb":  FormURL,
		"postgres://svc@localhost:5432/cmdb":           FormURL,
		"server=localhost;database=cmdb;user id=sa":    FormKeyword,
		"host=localhost dbname=cmdb user=svc":          FormKeyword,
		"odbc:server=localhost;database=cmdb":          FormODBC,
		"ODBC:server=localhost;database=cmdb":          FormODBC,
		"  sqlserver://sa@localhost?database=cmdb    ": FormURL,
	}
	for connection, want := range cases {
		if got := DetectForm(connection); got != want {
			t.Errorf("DetectForm(%q) = %q, want %q", connection, got, want)
		}
	}
}

// A connection that already carries a password is refused rather than guessed
// at. Whichever password won, the administrator would be reading one and
// sending the other — which is this journey's failure in a new place.
func TestAConnectionThatAlreadyHasAPasswordIsRefused(t *testing.T) {
	cases := map[string]string{
		DriverSQLServer: "sqlserver://sa:alreadyhere@localhost:1433?database=cmdb",
		DriverPostgres:  "host=localhost dbname=cmdb user=svc password=alreadyhere",
	}
	for driver, visible := range cases {
		t.Run(driver, func(t *testing.T) {
			_, err := Compose(driver, visible, "newpassword")
			if err == nil {
				t.Fatal("a connection that already carries a password was accepted")
			}
			if strings.Contains(err.Error(), "alreadyhere") {
				t.Errorf("the refusal quotes the password it found: %v", err)
			}
		})
	}
}

// "pwd" is the same setting under another name, so it has to be recognised too.
func TestTheShortSpellingOfPasswordIsAlsoRefused(t *testing.T) {
	_, err := Compose(DriverSQLServer, "server=localhost;database=cmdb;pwd=alreadyhere", "new")
	if err == nil {
		t.Fatal("a connection setting pwd= was accepted as having no password")
	}
}

func TestComposeRefusesWhatItCannotPlaceAPasswordIn(t *testing.T) {
	cases := map[string]string{
		"empty":         "",
		"blank":         "   ",
		"url-no-accoun": "sqlserver://dbhost.example.com:1433?database=Staging",
	}
	for name, visible := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Compose(DriverSQLServer, visible, "pw"); err == nil {
				t.Fatal("accepted a connection with nowhere to put a password")
			}
		})
	}
}

func TestComposeRefusesAnUnsupportedDriver(t *testing.T) {
	if _, err := Compose("mysql", "mysql://svc@localhost/cmdb", "pw"); err == nil {
		t.Fatal("an unsupported driver was accepted")
	}
}
