//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipsql"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

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

	// Baseline: what an import connection has to be today.
	whole := secrets.ValidateCredentialValue(
		secrets.CredentialTypeDatabaseURL, []byte(password))
	if whole.Valid {
		t.Fatal("the fixture proves nothing: a bare password is already accepted as an " +
			"import connection, so nothing here is measuring what is encrypted")
	}

	// What the journey asks for: the secret is the password, and the rest of
	// the connection is configuration somebody can read.
	if !secrets.ValidateCredentialValue(secrets.CredentialTypeGeneric, []byte(password)).Valid {
		t.Error("a password on its own cannot be stored as the secret for an import, so the " +
			"whole connection goes on being encrypted and a failure cannot be read")
	}
	t.Skip("The half above passes; the half that matters does not exist. Nothing yet holds " +
		"the connection itself as readable configuration beside that password — the import " +
		"still takes one encrypted string and nothing else. Remove this skip when a " +
		"connection can be read back without its password.")
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
	visible := `sqlserver://EXAMPLECORP\svcaccount@dbhost.example.com:1433?database=Staging`

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
	visible := "sqlserver://" + account + "@dbhost.example.com:1433?database=Staging"

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
			`sqlserver://EXAMPLECORP\svc@dbhost.example.com:1433?database=Staging`,
			ownershipsql.FormURL,
		},
		"keyword": {
			`server=dbhost.example.com;database=Staging;user id=EXAMPLECORP\svc`,
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
	visible := `sqlserver://EXAMPLECORP\svc@dbhost.example.com:1433?database=Staging`

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

	t.Skip("The composing and masking exist and are measured; nothing SHOWS them. No " +
		"endpoint returns a masked connection and no screen displays one, so the " +
		"administrator still cannot read what was sent — which is the requirement. " +
		"Remove this skip when the import screen shows the composed connection.")
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
	t.Skip("TODO: a stored credential can be tested, but that checks the shape of what was " +
		"stored rather than whether the server answers, and the import screen does not " +
		"offer it. Listing tables is the first thing that really connects, which is exactly " +
		"the conflation the journey rejects.")
}

// "A failure that tells me which of the four it was, in the words of whatever
// refused me."
func TestJourney_AFailedConnectionSaysWhichOfTheFourThingsFailed(t *testing.T) {
	t.Skip("TODO: depends on the test above existing at all. The requirement is that the " +
		"refusal is passed through rather than tidied into 'could not connect' — a wrong " +
		"account, a closed network, a wrong database name and a malformed string are four " +
		"different people to go and talk to.")
}

// "except for my password, which must never come back to me in a message."
func TestJourney_NoMessageEverCarriesThePasswordBack(t *testing.T) {
	t.Skip("TODO, and it is the one on this list that fails dangerously rather than " +
		"annoyingly. The thing that reads the string and the server both routinely quote " +
		"what they were handed, and neither knows which part of it was a secret — so " +
		"passing their words through unfiltered puts the password on a screen, in a support " +
		"bundle and in logs the whole organisation can read. It has to be taken out wherever " +
		"and however it appears, including escaped, which is the case that will be missed.")
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
