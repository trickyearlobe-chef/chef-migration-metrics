//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"testing"
)

// The journey suite for journeys/service-git-access.md — "Letting it reach our
// git servers". Run it with `make journey`.
//
// Nothing here is built. Red is the todo list; a skip means it cannot be
// answered from here yet, and says why.

// "To see the public keys this machine has, with their filenames."
func TestJourney_ICanSeeTheKeysThisMachineHas(t *testing.T) {
	if !reaches(t, http.MethodGet, "/api/v1/git-access/keys") {
		t.Error("there is no way to see the public keys on this machine, so the person who " +
			"has to paste one into GitLab cannot get at it without a shell they may not have")
	}
}

// "To generate one when I want one — because there is none, or because I want
// one for this tool specifically."
func TestJourney_ICanGenerateOneWhenIWantOne(t *testing.T) {
	if !reaches(t, http.MethodPost, "/api/v1/git-access/keys") {
		t.Error("nothing can make a key, so anybody wanting one — for an empty box, or one " +
			"meant for this tool — needs a terminal they may not have")
	}
}

// "It never replaces a key already there", and it appears "in the list under
// its own name".
func TestJourney_MakingOneNeverReplacesWhatIsThere(t *testing.T) {
	t.Skip("TODO: nothing makes a key, so nothing can overwrite one. A generated key is one " +
		"more in the list, under a name of its own — writing over the key their server " +
		"already trusts cuts this machine off from every repository at once.")
}

// "The first connection made so their host key is accepted, after I have seen
// the fingerprint and said yes."
func TestJourney_ISeeTheFingerprintBeforeAnythingIsTrusted(t *testing.T) {
	if !reaches(t, http.MethodPost, "/api/v1/git-access/host-keys") {
		t.Error("nothing offers to trust a git server, so accepting its key is still done " +
			"at a shell — and whoever does it there is not shown a fingerprint to check")
	}
}

// "Trusting a host key is a decision, so it is shown and confirmed."
func TestJourney_NothingIsTrustedWithoutSomebodySayingSo(t *testing.T) {
	t.Skip("TODO: nothing trusts a host key yet. Looking and trusting have to be two acts: " +
		"the first answers with the fingerprint and changes nothing, the second names the " +
		"fingerprint it was shown and refuses if the server now answers with another.")
}

// "Nothing names a key when it connects."
func TestJourney_NothingNamesAKeyWhenItConnects(t *testing.T) {
	t.Skip("TODO: true today by omission rather than by decision, and nothing would notice " +
		"somebody adding a picker — which a screen listing keys invites.")
}

// "To be told which half is missing when a clone fails."
func TestJourney_AFailedCloneSaysWhichHalfIsMissing(t *testing.T) {
	t.Skip("TODO: a failed clone reports what git said and nothing else, so an untrusted key " +
		"and an untrusted server read the same and send somebody to the wrong team.")
}

// "All of it on the screen where I add the git addresses."
func TestJourney_ItIsWhereTheGitAddressesAre(t *testing.T) {
	t.Skip("TODO: nothing to place yet. It belongs where the git addresses are added, not on " +
		"a settings page somebody has to know exists.")
}

// "Nothing here can prove the key was actually accepted at the other end."
func TestJourney_TheKeyWasAcceptedAtTheOtherEnd(t *testing.T) {
	t.Skip("Not answerable from this product. The key is pasted into GitLab by hand; all " +
		"this can show is what was offered.")
}

// "Nothing here can prove the fingerprint shown is the right one."
func TestJourney_TheFingerprintIsTheOneTheyPublished(t *testing.T) {
	t.Skip("Not answerable from this product. What is shown is what answered; only a person " +
		"can compare it with what their administrators published.")
}

// "Nothing here can prove which key SSH will choose."
func TestJourney_TheRightKeyIsTheOneSSHChooses(t *testing.T) {
	t.Skip("Not answerable from this product, and deliberately so: which key is offered " +
		"depends on the machine's own configuration.")
}
