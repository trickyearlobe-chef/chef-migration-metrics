//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipconn"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipsql"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// journeyConfig is the config store with the database and the encryption taken
// out. What is being asked here is whether a connection can be read back at
// all, which is a question about the shape of what is stored rather than about
// where it is kept.
type journeyConfig struct{ values map[string]json.RawMessage }

func newJourneyConfig() *journeyConfig {
	return &journeyConfig{values: map[string]json.RawMessage{}}
}

func (c *journeyConfig) Get(_ context.Context, key string) (json.RawMessage, error) {
	v, ok := c.values[key]
	if !ok {
		return nil, configstore.ErrNotFound
	}
	return v, nil
}

func (c *journeyConfig) Set(_ context.Context, key string, value json.RawMessage, _ bool, _ string) error {
	c.values[key] = value
	return nil
}

// The journey suite for journeys/ownership-connection.md — "Connecting to a
// database that is not mine". Run it with `make journey`.
//
// Nothing in this journey is built, so this is a todo list rather than an
// inventory, and every test here is skipped rather than red. That is not a
// lower standard, it is the honest state: the shape is settled — one visible
// connection with the password substituted into it — but there is nothing yet
// to compose, so there is nothing to assert against.
//
// The escaping ones in particular must be MEASURED against a real SQL Server
// rather than asserted from argument. That is the whole finding this journey
// came from, and writing them any other way would reproduce it.

// "Only the password out of sight. The address, the database, the account and
// the domain in plain view, and editable."
//
// Today the secret IS the whole connection: an import connection is stored as a
// connection string, which is encrypted entire, so the host and the account are
// hidden along with the password. The baseline is asserted first — a bare
// password is refused as an import connection — so this cannot go green because
// validation was loosened for some unrelated reason.
func TestJourney_OnlyThePasswordIsOutOfSight(t *testing.T) {
	const password = "s3cr3t!p@ss/w0rd"

	// The secret is the password and nothing else, so it is stored as what it
	// is: bytes with no shape. A password refused for its shape is a password
	// somebody's database really has and this tool will not hold.
	if !secrets.ValidateCredentialValue(secrets.CredentialTypeGeneric, []byte(password)).Valid {
		t.Error("a password on its own cannot be stored as the secret for an import, so the " +
			"whole connection goes on being encrypted and a failure cannot be read")
	}

	// And the other half: the connection itself is held as configuration
	// beside that password, and reads back with the address, the database, the
	// account and the domain all legible.
	config := newJourneyConfig()
	stored := ownershipconn.NewStore(config)
	connection := `sqlserver://EXAMPLECORP\svcaccount:` + ownershipsql.PasswordMarker +
		`@dbhost.example.com:1433?database=cmdb`

	// Baseline: nothing is stored yet, so what is read below was put there by
	// this code rather than sitting in the fixture.
	if _, err := stored.Get(context.Background(), "asset-database"); err == nil {
		t.Fatal("the fixture proves nothing: a connection is readable before one was stored")
	}

	if err := stored.Save(context.Background(), ownershipconn.Connection{
		Name:               "asset-database",
		Driver:             ownershipsql.DriverSQLServer,
		Connection:         connection,
		PasswordCredential: "asset-database-password",
	}, "admin"); err != nil {
		t.Fatalf("a connection could not be stored as configuration: %v", err)
	}

	readBack, err := stored.Get(context.Background(), "asset-database")
	if err != nil {
		t.Fatalf("a stored connection could not be read back: %v", err)
	}
	for _, visible := range []string{
		"dbhost.example.com", "1433", "database=cmdb", "EXAMPLECORP", "svcaccount",
	} {
		if !strings.Contains(readBack.Connection, visible) {
			t.Errorf("%q cannot be read back, so it cannot be checked when the connection "+
				"fails: %s", visible, readBack.Connection)
		}
	}
	if readBack.PasswordCredential == "" {
		t.Error("nothing says which credential holds the password, so the connection cannot " +
			"be composed without somebody typing one in")
	}
}

// "The password, and only the password, put in for me — correctly."
//
// The composing half is built and is measured rather than argued: the password
// arrives at the driver exactly as typed, for every awkward character, in each
// spelling a connection comes in. Those measurements are
// TestCompose*ReachesTheDriverIntact in internal/ownershipsql, and the ones that
// connect to a running server are TestFunctional_MSSQL_Composed* and
// TestFunctional_Postgres_ComposedConnectionsConnect.
//
// What is asserted here is the product-level promise: the administrator hands
// over a connection with no password in it, and what goes to the database has
// one.
func TestJourney_ThePasswordIsPutInForMeAndEscapedCorrectly(t *testing.T) {
	const password = `pa%ss;wo rd/w0rd!`
	visible := `sqlserver://EXAMPLECORP\svcaccount:` + ownershipsql.PasswordMarker +
		`@dbhost.example.com:1433?database=Staging`

	// Baseline: the visible connection really does not carry the password, so
	// finding it below is this code putting it there.
	if strings.Contains(visible, password) {
		t.Fatal("the fixture proves nothing: the password is already in the visible connection")
	}

	composed, err := ownershipsql.Compose(ownershipsql.DriverSQLServer, visible, password)
	if err != nil {
		t.Fatalf("composing the connection: %v", err)
	}
	if composed.DSN == visible {
		t.Fatal("the composed connection is the one that was typed, so no password was put in")
	}
	// The percent sign and the space are escaped; the semicolon is not, because
	// it is legal where it lands and a server measured it as such. Asserting on
	// the ones that are actually escaped rather than the ones that look
	// dangerous is the difference between a test and a guess.
	for _, escaped := range []string{"%25", "%20"} {
		if !strings.Contains(composed.DSN, escaped) {
			t.Errorf("the password went in unescaped, which is the failure this journey is "+
				"about: %s", composed.Masked)
		}
	}
}

// "I say where it goes, and the screen tells me how to say it."
//
// Half of this is built: the position is marked rather than guessed, and a
// connection that does not say where the password goes is refused by name
// instead of being repaired. The other half is a screen that says so.
func TestJourney_IAmToldHowToMarkWhereThePasswordGoes(t *testing.T) {
	const password = "s3cr3t"
	withMarker := `sqlserver://svc:` + ownershipsql.PasswordMarker +
		`@dbhost.example.com:1433?database=Staging`
	without := `sqlserver://svc@dbhost.example.com:1433?database=Staging`

	// Baseline: with the marker it composes, so the refusal below is the
	// missing marker and not something else wrong with the connection.
	if _, err := ownershipsql.Compose(ownershipsql.DriverSQLServer, withMarker, password); err != nil {
		t.Fatalf("the fixture proves nothing: even with the marker this will not compose: %v", err)
	}

	_, err := ownershipsql.Compose(ownershipsql.DriverSQLServer, without, password)
	if err == nil {
		t.Fatal("a connection that never says where the password goes was accepted, so " +
			"something I did not write is being sent")
	}
	if !strings.Contains(err.Error(), ownershipsql.PasswordMarker) {
		t.Errorf("the refusal does not tell me what to write: %v", err)
	}

	t.Skip("The refusal names the marker; no screen does. Nothing shows the administrator " +
		"how to mark the position before they get it wrong, which is the half of this " +
		"requirement that is about being told rather than being refused.")
}

// "It must not quietly rewrite anything the administrator can see: typing an
// account one way and sending it another is the same unreadable failure in a
// new place."
//
// The account is percent-encoded on the way out, because a domain login carries
// a backslash that no URL can hold — but what the server is told is the account
// as typed. That it survives the driver's own parser unchanged is measured by
// TestTheAccountArrivesExactlyAsTyped and TestDomainUserSurvivesToTheDriver in
// internal/ownershipsql. What is asserted here is that the rewriting is not
// quiet: it is there to be read in the masked view.
func TestJourney_NothingIVisiblyTypedIsRewrittenBehindMe(t *testing.T) {
	const account = `EXAMPLECORP\svcaccount`
	visible := "sqlserver://" + account + ":" + ownershipsql.PasswordMarker +
		"@dbhost.example.com:1433?database=Staging"

	composed, err := ownershipsql.Compose(ownershipsql.DriverSQLServer, visible, "irrelevant")
	if err != nil {
		t.Fatalf("composing the connection: %v", err)
	}
	for _, visiblePart := range []string{"dbhost.example.com", "1433", "database=Staging"} {
		if !strings.Contains(composed.Masked, visiblePart) {
			t.Errorf("%q is not in what I am shown, so I cannot check it: %s",
				visiblePart, composed.Masked)
		}
	}
	if !strings.Contains(composed.Masked, "EXAMPLECORP") {
		t.Errorf("the domain was not left readable: %s", composed.Masked)
	}
}

// "How to escape depends on the form of the string, and getting that backwards
// fails silently."
//
// The dangerous one, and the plan had it backwards: brace-quoting a password in
// SQL Server's keyword spelling parses cleanly, arrives with the braces still
// attached and comes back as a refused login. Measured against a running server
// by TestFunctional_MSSQL_TheWrongKeywordQuotingIsRefusedByTheServer.
func TestJourney_TheEscapingMatchesTheFormOfTheStringItWasGiven(t *testing.T) {
	const password = `pa%ss;wo rd!`
	shapes := map[string]struct {
		visible string
		form    ownershipsql.Form
	}{
		"url": {
			`sqlserver://EXAMPLECORP\svc:` + ownershipsql.PasswordMarker +
				`@dbhost.example.com:1433?database=Staging`,
			ownershipsql.FormURL,
		},
		"keyword": {
			`server=dbhost.example.com;database=Staging;user id=EXAMPLECORP\svc;password=` +
				ownershipsql.PasswordMarker,
			ownershipsql.FormKeyword,
		},
	}

	sent := map[string]string{}
	for name, shape := range shapes {
		composed, err := ownershipsql.Compose(ownershipsql.DriverSQLServer, shape.visible, password)
		if err != nil {
			t.Fatalf("%s: composing: %v", name, err)
		}
		if composed.Form != shape.form {
			t.Errorf("%s: the form was read as %q, so the wrong escaping rule was applied",
				name, composed.Form)
		}
		sent[name] = composed.DSN
	}

	// The same password, escaped two different ways, because the two shapes want
	// different treatment. If these ever match, one form is being escaped by the
	// other's rule — which is the silent failure this requirement names.
	urlTail := strings.SplitN(sent["url"], "@", 2)[0]
	if strings.Contains(sent["keyword"], urlTail[strings.LastIndex(urlTail, ":")+1:]) {
		t.Error("both shapes escaped the password identically, so the form is not being " +
			"recognised — the rule that reads correctly and is refused")
	}
}

// "To be shown what will actually be sent, with the password masked."
func TestJourney_IAmShownWhatWillActuallyBeSent(t *testing.T) {
	const password = `pa%ss;wo rd!`
	visible := `sqlserver://EXAMPLECORP\svc:` + ownershipsql.PasswordMarker +
		`@dbhost.example.com:1433?database=Staging`

	composed, err := ownershipsql.Compose(ownershipsql.DriverSQLServer, visible, password)
	if err != nil {
		t.Fatalf("composing the connection: %v", err)
	}
	// The masked view is the real connection run through the same code with the
	// password swapped, so it cannot drift from what is sent.
	if composed.Masked == composed.DSN {
		t.Fatal("what I am shown is the real connection, password and all")
	}
	if strings.Contains(composed.Masked, password) {
		t.Errorf("the password is in what I am shown: %s", composed.Masked)
	}

	// And it is reachable: an address answers with the composed connection,
	// masked, for a connection that has not been stored yet.
	if !reaches(t, "POST", "/api/v1/ownership/import/show-connection") {
		t.Error("nothing will say what a connection composes to, so the administrator cannot " +
			"read what was sent")
	}

	t.Skip("The composing and masking exist, are measured, and an endpoint now answers with " +
		"them — but no screen displays one, so the administrator still cannot read what was " +
		"sent without asking the API themselves. Remove this skip when the import screen " +
		"shows the composed connection.")
}

// "Show me one that would work for the kind of database I picked, filled in
// with what I have already told you — then let me change any of it."
func TestJourney_AConnectionIsProposedAndICanOverrideIt(t *testing.T) {
	t.Skip("TODO: nothing proposes a connection. The shape is settled — one visible string " +
		"with the password substituted in — so what is missing is a starting example per " +
		"kind of database, filled in from what has already been said and freely editable " +
		"afterwards.")
}

// "To test it before going any further. Asking for the list of tables is not a
// connection test."
func TestJourney_ICanTestTheConnectionBeforeBrowsingIt(t *testing.T) {
	// Testing is its own act now: it dials the server, runs no query and reads
	// no rows, so when it fails the failure is about connecting and nothing
	// else. Proved against running servers by
	// TestFunctional_MSSQL_TheConnectionTestSaysWhichOfTheFiveItWas and its
	// PostgreSQL twin, and by TestFunctional_MSSQL_TestingAConnectionIsNotListingTables.
	//
	// Asserted here without a server: a connection that cannot be composed is
	// answered as a connection test rather than as an error from browsing.
	result := ownershipsql.TestConnection(context.Background(), ownershipsql.Config{
		Driver:     ownershipsql.DriverSQLServer,
		Connection: `sqlserver://svc@dbhost.example.com:1433?database=Staging`,
		Password:   "s3cr3t",
	})
	if result.Succeeded() {
		t.Fatal("a connection with nowhere to put the password reported success")
	}
	if result.Outcome != ownershipsql.OutcomeMalformed {
		t.Errorf("outcome = %q, want %q", result.Outcome, ownershipsql.OutcomeMalformed)
	}
	if result.Detail == "" {
		t.Error("the test says it failed and not why, which is what asking for the table " +
			"list already did")
	}

	// And it is its own address, answerable before anything is stored — the
	// order this is actually done in: compose, test, then keep.
	if !reaches(t, "POST", "/api/v1/ownership/import/test-connection") {
		t.Error("there is no way to ask whether a connection works except by trying to use it")
	}

	t.Skip("Testing a connection is built, measured, and has an address of its own — but the " +
		"import screen does not offer it. Browsing tables is still the first thing that " +
		"really connects there, which is the conflation this requirement rejects. Remove " +
		"this skip when the screen has a test of its own.")
}

// "A failure that tells me which of the five it was, in the words of whatever
// refused me."
func TestJourney_AFailedConnectionSaysWhichOfTheFiveThingsFailed(t *testing.T) {
	// Each of the five is a different person to go and talk to, so each is a
	// distinct answer rather than a shade of "could not connect". Which words
	// mean which was MEASURED against running servers, and is re-measured by
	// the functional suites — text matching that is only remembered goes stale
	// and then names the wrong team.
	for _, outcome := range []ownershipsql.Outcome{
		ownershipsql.OutcomeMalformed,
		ownershipsql.OutcomeUnreachable,
		ownershipsql.OutcomeRefused,
		ownershipsql.OutcomeNoDatabase,
		ownershipsql.OutcomeUntrustedDomain,
	} {
		if outcome == "" {
			t.Error("an outcome with no name cannot send anybody anywhere")
		}
	}

	// A refusal nothing recognises says so, rather than being filed under
	// whichever outcome happens to be the fallback.
	if ownershipsql.OutcomeUnknown == ownershipsql.OutcomeRefused {
		t.Error("an unrecognised failure is reported as a refused login, which names a team " +
			"on no evidence")
	}

	// And the words of whatever refused are passed through rather than tidied
	// away — with the password taken out.
	result := ownershipsql.TestConnection(context.Background(), ownershipsql.Config{
		Driver:     ownershipsql.DriverSQLServer,
		Connection: `sqlserver://svc:` + ownershipsql.PasswordMarker + `@127.0.0.1:1?database=Staging`,
		Password:   "s3cr3t",
	})
	if result.Succeeded() {
		t.Skip("something is listening on port 1, so this proves nothing")
	}
	if result.Outcome != ownershipsql.OutcomeUnreachable {
		t.Errorf("nothing listening reported as %q, want %q\n  detail: %s",
			result.Outcome, ownershipsql.OutcomeUnreachable, result.Detail)
	}
	if result.Detail == "" {
		t.Error("the refusal was tidied into nothing, which throws away the only thing in " +
			"it worth having")
	}
	if strings.Contains(result.Detail, "s3cr3t") {
		t.Errorf("the password came back in the refusal: %s", result.Detail)
	}
}

// "To be told when the account is not the database's to check."
func TestJourney_IAmToldWhenTheAccountIsNotTheDatabasesToCheck(t *testing.T) {
	// It is one of the answers a connection test can give, rather than being
	// filed under a refused login — which would send somebody to argue with
	// the owner of an account that was never the database's to check.
	if ownershipsql.OutcomeUntrustedDomain == ownershipsql.OutcomeRefused {
		t.Error("an account handed to a directory reports as a refused login, which names " +
			"the wrong team")
	}
	if !reaches(t, "POST", "/api/v1/ownership/import/test-connection") {
		t.Error("nothing tests a connection, so nothing can report this")
	}

	t.Skip("The reporting exists and the behaviour underneath is measured — " +
		"TestFunctional_MSSQL_ABackslashAccountAsksForIntegratedAuthentication in " +
		"internal/ownershipsql shows that anything before a backslash, whether a domain, a " +
		"machine name, a workgroup or a dot, hands the login to a directory rather than to " +
		"the database. What cannot be answered here is whether the right words come back " +
		"from a server that really is in a domain: ours belongs to nobody's list of people " +
		"and will not accept an account that looks as though it does. This refusal is also " +
		"the only one that does not name the account back, so it is the one where the " +
		"composed connection is all the administrator has.")
}

// "except for my password, which must never come back to me in a message."
func TestJourney_NoMessageEverCarriesThePasswordBack(t *testing.T) {
	const password = `pa%ss;wo rd#7Q!`

	// Whatever refused me, quoting the whole connection back — which is what
	// both the string reader and the server actually do.
	quoted := `unable to open connection ` +
		`"sqlserver://EXAMPLECORP%5Csvc:pa%25ss;wo%20rd%237Q!@dbhost:1433?database=cmdb": refused`

	// Baseline: the escaped spelling really is in there, and it does not look
	// like the password that was stored. That is the case that gets missed.
	if strings.Contains(quoted, password) {
		t.Fatal("the fixture proves nothing: the message carries the password as typed, " +
			"so it is not testing the escaped spelling")
	}

	cleaned := ownershipsql.RedactPassword(quoted, password)
	if strings.Contains(cleaned, "pa%25ss") {
		t.Errorf("the escaped password came back in a message: %s", cleaned)
	}
	if strings.Contains(cleaned, password) {
		t.Errorf("the password came back in a message: %s", cleaned)
	}

	// And the message is still worth reading afterwards — a refusal tidied
	// into nothing is the other failure this journey names.
	for _, keep := range []string{"dbhost:1433", "database=cmdb", "EXAMPLECORP"} {
		if !strings.Contains(cleaned, keep) {
			t.Errorf("redaction took away something I need (%q): %s", keep, cleaned)
		}
	}
}

// "Nothing proves a proposed connection is a good starting point."
func TestJourney_NothingProvesTheProposedConnectionHelps(t *testing.T) {
	t.Skip("Recorded, not planned: a suggestion that is usually wrong is worse than none, " +
		"because it turns setting up a connection into correcting one. Nothing can tell " +
		"the two apart except somebody using it.")
}

// "The load-bearing assumption: that the account can reach the server from
// where this tool runs at all."
func TestJourney_TheServerIsReachableFromHere(t *testing.T) {
	t.Skip("Not answerable from this product. Their database answers on several addresses " +
		"across subnets; from here one refuses and another times out. Until that is opened " +
		"every failure in this journey looks like a firewall, and no amount of getting the " +
		"string right will change it.")
}
