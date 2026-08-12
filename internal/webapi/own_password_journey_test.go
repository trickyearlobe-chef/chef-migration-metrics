//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The journey suite for journeys/own-password.md. Run it with `make journey`.
//
// One test per thing the journey says has to be in place. Green means built,
// red means still to do — so running this recomputes the todo list rather than
// asking anybody to keep one true.
//
// Most of it is red, which is correct: nobody can change their own password on
// this service today. Outside the gating suite on purpose, so that stays a
// backlog and never a broken build.

// ownPasswordCandidates are the conventional places a person's own password
// would be changed. A set rather than one address, because which one it lands
// on is the implementer's choice and this suite must not pre-decide it: any of
// these turns the test green with no edit here.
var ownPasswordCandidates = []string{
	"/api/v1/auth/me/password",
	"/api/v1/auth/me/change-password",
	"/api/v1/me/password",
	"/api/v1/users/me/password",
}

// ownPasswordServed reports the first candidate address the service routes,
// asked by method because a wrong method on a real address answers 405 and
// that still means the address exists.
func ownPasswordServed(t *testing.T, method string) (string, bool) {
	t.Helper()
	for _, path := range ownPasswordCandidates {
		req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		agentJourneyRouter().ServeHTTP(w, withAdminSession(req))
		if w.Code != http.StatusNotFound {
			return path, true
		}
	}
	return "", false
}

// "To change it myself, in the same place I see everything else about my
// account. Not a request for somebody to action — the change itself."
func TestJourney_ICanChangeMyOwnPassword(t *testing.T) {
	if _, ok := ownPasswordServed(t, http.MethodPut); !ok {
		if _, ok := ownPasswordServed(t, http.MethodPost); !ok {
			t.Error("there is no way for somebody to change their own password; the only " +
				"way one changes is an administrator setting it, so every change means a " +
				"ticket and somebody else knowing what it is")
		}
	}
}

// "To prove it is me before it changes. My current password, typed again."
func TestJourney_ChangingItNeedsTheCurrentPassword(t *testing.T) {
	path, ok := ownPasswordServed(t, http.MethodPut)
	if !ok {
		t.Skip("nobody can change their own password yet; " +
			"TestJourney_ICanChangeMyOwnPassword is the gap")
	}

	// A change that offers only a new password must be refused. Accepting it
	// would mean an unlocked screen is the whole of the security.
	req := httptest.NewRequest(http.MethodPut, path,
		strings.NewReader(`{"new_password":"Sufficiently-Long-1"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	agentJourneyRouter().ServeHTTP(w, withAdminSession(req))

	if w.Code < 400 {
		t.Errorf("a password was changed without the current one being given (status %d), "+
			"so anybody who finds an unlocked screen owns the account", w.Code)
	}
}

// "To be told what will be accepted before I am told I got it wrong."
func TestJourney_IAmToldTheRulesBeforeIAmToldIAmWrong(t *testing.T) {
	// The sign-in description is where a screen would read this from: it is
	// already fetched before anybody is authenticated, and it already says
	// which ways of signing in exist.
	w := httptest.NewRecorder()
	agentJourneyRouter().ServeHTTP(w,
		withAdminSession(httptest.NewRequest(http.MethodGet, "/api/v1/auth/info", nil)))

	var info map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("reading what the service says about signing in: %v", err)
	}
	for key := range info {
		if strings.Contains(key, "password") || strings.Contains(key, "min_length") {
			return
		}
	}
	t.Error("nothing tells a person what a password has to look like before they type one, " +
		"so the minimum length is discovered one rejection at a time")
}

// "Not to be offered this at all if I sign in through the company identity
// provider. I have no password here — it lives with them."
//
// The suite can only ask whether the service says which kind of account it is
// talking to. Whether a screen then hides the field is a judgement made by
// looking at it.
func TestJourney_TheServiceSaysWhetherThisAccountHasAPasswordAtAll(t *testing.T) {
	// A router with authentication actually wired. The bare journey router
	// answers this address with "not implemented", so asking it would report
	// the service as silent about the provider when it is not.
	router, secret := credentialScopeFixture(t, "admin", false)
	w := credentialRequest(t, router, secret, http.MethodGet, "/api/v1/auth/me", "")

	var me map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &me); err != nil {
		t.Fatalf("reading who the caller is (status %d): %v", w.Code, err)
	}
	if _, ok := me["provider"]; !ok {
		t.Error("what the caller is told about themselves does not say how they signed in, " +
			"so a screen cannot tell somebody with no password here from somebody with one")
	}
}

// "Nothing here should be able to reach anybody else's password."
//
// Green from the start, and here to stay that way rather than to report
// progress. An administrator setting somebody's password is a different
// address and a different act; this asks that no OTHER way in appears beside
// it as the self-service one is built.
func TestJourney_ThereIsNoAddressForSomebodyElsesPassword(t *testing.T) {
	for _, path := range []string{
		"/api/v1/auth/me/password/somebody",
		"/api/v1/users/somebody/password",
		"/api/v1/auth/users/somebody/password",
	} {
		req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		agentJourneyRouter().ServeHTTP(w, withAdminSession(req))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s is served (status %d) — setting another person's password belongs "+
				"to the administrator's own address and nowhere else", path, w.Code)
		}
	}
}

// "nothing I hand to a tool should be able to change one at all."
//
// Weak while the address does not exist, because an unrouted path refuses
// everything. It is written against the real scope rule rather than against a
// 404 so that it keeps its meaning on the day the address arrives: a credential
// may write to the failure register and nowhere else, and this is nowhere else.
func TestJourney_ACredentialCannotChangeAPassword(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", true)

	for _, path := range append(ownPasswordCandidates,
		"/api/v1/admin/users/somebody/password") {
		w := credentialRequest(t, router, secret, http.MethodPut, path,
			`{"current_password":"x","new_password":"Sufficiently-Long-1"}`)
		if w.Code >= 200 && w.Code < 300 {
			t.Errorf("a credential changed a password at %s (status %d) — handing one to an "+
				"editor assistant would hand over the account itself", path, w.Code)
		}
	}
}

// "An administrator setting somebody's password stays exactly as it is."
//
// The journey says this must not be replaced by the self-service path. Nothing
// tests the endpoint's behaviour — that gap is named in the journey — so this
// asks only that the address is still served and still administrator-only.
func TestJourney_AnAdministratorCanStillSetSomebodysPassword(t *testing.T) {
	const path = "/api/v1/admin/users/somebody/password"

	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	agentJourneyRouter().ServeHTTP(w, withAdminSession(req))
	if w.Code == http.StatusNotFound {
		t.Fatal("an administrator can no longer set somebody's password, which is the only " +
			"way one changes today and the only way back in for a locked-out person")
	}

	// Read the role off the route table rather than the per-handler overrides:
	// this address is guarded at registration, so that is where its level is
	// recorded, and an override map entry would be the wrong place to look.
	var found bool
	for _, rt := range agentJourneyRouter().Routes() {
		if rt.Pattern != "/api/v1/admin/users/" {
			continue
		}
		found = true
		if rt.Role != RoleAdmin {
			t.Errorf("setting somebody else's password is served at the %q level, want %q — "+
				"it is the way back in for a locked-out person and belongs to an "+
				"administrator", rt.Role, RoleAdmin)
		}
	}
	if !found {
		t.Error("the administrator's user-management addresses are no longer registered")
	}
}
