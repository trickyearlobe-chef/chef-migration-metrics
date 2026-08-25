//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth/jit"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth/samlsp"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// The journey suite for journeys/service-access.md. Run it with `make journey`.
//
// One test per thing the journey says has to be in place. A green one is built;
// a red one is not yet. Deliberately outside the gating suite — a red here is
// the normal state and must never block a build.

// serviceAccessProvisionStore captures what a sign-in would write, so a test can
// ask whether somebody was coined rather than recognised.
type serviceAccessProvisionStore struct {
	existing  map[string]datastore.User
	created   []string
	upsertErr error
}

func (s *serviceAccessProvisionStore) UpsertSAMLUser(_ context.Context, p datastore.InsertUserParams) (datastore.User, bool, error) {
	if s.upsertErr != nil {
		return datastore.User{}, false, s.upsertErr
	}
	if _, known := s.existing[p.Username]; known {
		return datastore.User{Username: p.Username, Role: p.Role}, false, nil
	}
	s.created = append(s.created, p.Username)
	return datastore.User{Username: p.Username, Role: p.Role}, true, nil
}

func (s *serviceAccessProvisionStore) RecordLoginSuccess(context.Context, string) error { return nil }

// ---------------------------------------------------------------------------
// What I need
// ---------------------------------------------------------------------------

// "People signing in with their existing company credentials, not a separate
// password to manage."
func TestJourney_SigningInWithCompanyCredentials(t *testing.T) {
	if !reaches(t, http.MethodGet, "/api/v1/auth/saml/login") &&
		!reaches(t, http.MethodPost, "/api/v1/auth/saml/acs") {
		t.Error("there is no way to sign in through an identity provider, so everybody needs a " +
			"second password kept here")
	}
}

// "Local accounts as well, because the identity provider is exactly what is
// unavailable on the day I most need to get in."
func TestJourney_LocalAccountsExistToo(t *testing.T) {
	if !reaches(t, http.MethodPost, "/api/v1/auth/login") {
		t.Error("there is no local sign-in, so there is no way in when the identity provider " +
			"is the broken thing — and refusing a nameless sign-in would then strand everybody")
	}
}

// "Somebody who has never signed in before to become a user on their first
// successful sign-in, without me provisioning them by hand."
func TestJourney_AFirstSignInMakesTheUser(t *testing.T) {
	store := &serviceAccessProvisionStore{existing: map[string]datastore.User{}}
	_, created, err := jit.New(store, nil).Provision(context.Background(), &samlsp.UserInfo{
		SAMLSubject: "subject-1", Username: "ada", Role: "viewer",
	})
	if err != nil {
		t.Fatalf("a first sign-in failed outright: %v", err)
	}
	if !created {
		t.Error("somebody signing in for the first time was not made a user, so an administrator " +
			"has to create every account by hand before anybody can work")
	}
}

// "Their level of access decided by the identity provider, so that when somebody
// changes role or leaves, that is handled where it is already handled."
func TestJourney_TheProviderDecidesTheLevelOfAccess(t *testing.T) {
	store := &serviceAccessProvisionStore{
		existing: map[string]datastore.User{"ada": {Username: "ada", Role: "viewer"}},
	}
	u, _, err := jit.New(store, nil).Provision(context.Background(), &samlsp.UserInfo{
		SAMLSubject: "subject-1", Username: "ada", Role: "admin",
	})
	if err != nil {
		t.Fatalf("provisioning an existing person failed: %v", err)
	}
	if u.Role != "admin" {
		t.Errorf("role = %q, want admin — the provider raised this person's access and it did "+
			"not follow, so leaving or changing role is handled in a second list here", u.Role)
	}
}

// "Anybody arriving without an explicit level of access gets the lowest one.
// Never the highest, never nothing, and not a failure."
func TestJourney_NoStatedAccessMeansTheLowest(t *testing.T) {
	store := &serviceAccessProvisionStore{existing: map[string]datastore.User{}}
	u, _, err := jit.New(store, nil).Provision(context.Background(), &samlsp.UserInfo{
		SAMLSubject: "subject-2", Username: "grace", Role: "",
	})
	if err != nil {
		t.Fatalf("arriving with no level of access failed the sign-in: %v", err)
	}
	if u.Role != "viewer" {
		t.Errorf("role = %q, want viewer — somebody arriving with nothing stated got something "+
			"other than the safe default", u.Role)
	}
}

// "A first administrator on a brand-new installation, before there is anybody to
// grant anything."
func TestJourney_AFirstAdministratorOnAnEmptyInstallation(t *testing.T) {
	t.Skip("Answerable only at startup, not from here: the first administrator is seeded from " +
		"the environment before any request is served, so nothing in the request path can " +
		"observe it.")
}

// "Somebody who guesses passwords to be shut out, without that becoming a way to
// lock a real administrator out on purpose."
func TestJourney_GuessingIsShutOutWithoutBecomingAWeapon(t *testing.T) {
	t.Skip("Half of this is assertable and half is not: that repeated failures stop being " +
		"accepted is held beside the local authenticator, but that a stranger cannot use it to " +
		"lock a named administrator out is a property of the whole sign-in path under load.")
}

// ---------------------------------------------------------------------------
// The decisions behind it
// ---------------------------------------------------------------------------

// "A person is anchored on their company username, not on whatever token the
// sign-in produced."
func TestJourney_AnchoredOnTheNameNotTheToken(t *testing.T) {
	store := &serviceAccessProvisionStore{
		existing: map[string]datastore.User{"ada": {Username: "ada", Role: "viewer"}},
	}
	_, created, err := jit.New(store, nil).Provision(context.Background(), &samlsp.UserInfo{
		SAMLSubject: "a-different-token-every-time", Username: "ada", Role: "viewer",
	})
	if err != nil {
		t.Fatalf("a returning person failed to sign in: %v", err)
	}
	if created {
		t.Error("a person who already exists was made again because the sign-in produced a " +
			"different token, so every sign-in would leave a new empty account behind")
	}
}

// "A sign-in must carry a name to sign in as, or it is refused. ... Refusing is
// the whole of the fix: it removes the possibility rather than detecting it
// afterwards."
//
// The baseline is TestJourney_LocalAccountsExistToo: refusing is only safe while
// there is another way in, so that test is what makes this one safe to want.
func TestJourney_NobodyIsCoinedOutOfAnOpaqueToken(t *testing.T) {
	store := &serviceAccessProvisionStore{existing: map[string]datastore.User{}}
	_, created, err := jit.New(store, nil).Provision(context.Background(), &samlsp.UserInfo{
		SAMLSubject: "https://idp.example.com:User@Example.COM", Username: "", Role: "viewer",
	})
	if err == nil && created {
		t.Errorf("a first sign-in carrying no name made a person anyway, named %q — their work "+
			"will hang off a string that is not a name, and nothing later can tell it apart "+
			"from one somebody chose", store.created)
	}
}

// "A misconfiguration must degrade the service, never prevent it starting."
func TestJourney_AMisconfigurationDegradesRatherThanStops(t *testing.T) {
	t.Skip("Not answerable from the request path: the ladder runs at startup, and what it " +
		"proves is that the service came up at all. The journey names this as the thing " +
		"nothing proves end to end.")
}

// ---------------------------------------------------------------------------
// What nothing can prove
// ---------------------------------------------------------------------------

// "Nothing proves a real identity provider works."
func TestJourney_ARealIdentityProviderWorks(t *testing.T) {
	t.Skip("Assertions are parsed from fixtures. Every provider differs in what it releases " +
		"and under which names, and only a real one can settle it.")
}

// "The load-bearing assumption: that the identity provider does not reissue a
// username to a different human being."
func TestJourney_AUsernameIsNeverReissuedToSomebodyElse(t *testing.T) {
	t.Skip("Not answerable from this product at all — it is a property of the directory. " +
		"Nothing here could detect a violation; it would simply hand one person another " +
		"person's work.")
}
