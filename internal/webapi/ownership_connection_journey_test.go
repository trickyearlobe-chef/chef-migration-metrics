//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"testing"

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
func TestJourney_ThePasswordIsPutInForMeAndEscapedCorrectly(t *testing.T) {
	t.Skip("TODO, and it must be MEASURED not reasoned about. The case is a password with " +
		"punctuation in it, and the only honest test sends one to a SQL Server that really " +
		"refuses or really admits it — the container and its sample data already exist. " +
		"Working out what a driver accepts by argument is what produced several confident " +
		"wrong answers in one evening; this test exists so that is not done again.")
}

// "It must not quietly rewrite anything the administrator can see: typing an
// account one way and sending it another is the same unreadable failure in a
// new place."
func TestJourney_NothingIVisiblyTypedIsRewrittenBehindMe(t *testing.T) {
	t.Skip("TODO: the account and the domain in front of it stay exactly as written, because " +
		"they are visible and therefore mine to correct. Pinning this needs the composed " +
		"connection to exist to compare against what was typed.")
}

// "How to escape depends on the form of the string, and getting that backwards
// fails silently."
func TestJourney_TheEscapingMatchesTheFormOfTheStringItWasGiven(t *testing.T) {
	t.Skip("TODO, and the dangerous one. The two shapes a connection arrives in want " +
		"different treatment for the same punctuation, so applying one form's rule to the " +
		"other yields a string that reads correctly and is refused. Both forms have to be " +
		"measured against a real server; neither can be settled by reading a driver's " +
		"documentation, which is how this was got wrong before.")
}

// "To be shown what will actually be sent, with the password masked."
func TestJourney_IAmShownWhatWillActuallyBeSent(t *testing.T) {
	t.Skip("TODO: nothing composes a connection, so nothing can show one. This is the " +
		"requirement that ends the guessing — a masked view of the composed string answers " +
		"in one glance the question that has been costing days.")
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
