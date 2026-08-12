// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// These run through the real authentication middleware with a real credential
// in the header, because the rule under test is about what happens between the
// header and the handler. A session pushed straight into the context would skip
// the whole of it.

// credentialScopeFixture returns a router with real auth wired, plus a secret
// for a credential belonging to an account at the given role.
func credentialScopeFixture(t *testing.T, role string, canWrite bool) (*Router, string) {
	t.Helper()

	store := newMemCredentialStore().withUser("engineer", role)
	creds := auth.NewCredentialManager(store)
	_, secret, err := creds.Issue(context.Background(), "engineer", "editor", canWrite)
	if err != nil {
		t.Fatalf("issuing a credential: %v", err)
	}

	hub := NewEventHub()
	go hub.Run()
	sessions := auth.NewSessionManager(mockSessionStore{}, 8*time.Hour)
	mw := auth.NewMiddleware(sessions, auth.WithCredentials(creds))
	localAuth := auth.NewLocalAuthenticator(mockLocalAuthStore{}, 5)

	router := NewRouter(&mockStore{}, testConfigWithTargetVersions("19.0"), hub,
		WithAuth(localAuth, sessions, mw, nil),
		WithCredentialManager(creds),
	)
	return router, secret
}

// credentialRequest issues a request carrying the credential in the header,
// exactly as an editor assistant would.
func credentialRequest(t *testing.T, router *Router, secret, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// "A credential is another way into the same account. My level of access,
// exactly what I see on screen." Reading is not narrowed.
func TestCredentialReadsAsTheAccountItBelongsTo(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	w := credentialRequest(t, router, secret, http.MethodGet, "/api/v1/auth/me", "")
	if w.Code != http.StatusOK {
		t.Fatalf("a credential could not read as its own account: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "engineer") {
		t.Errorf("the service does not report the credential's account as the caller: %s",
			w.Body.String())
	}
}

// An account's own role still applies. A credential cannot reach past it.
func TestCredentialCannotReachAboveItsAccountsRole(t *testing.T) {
	router, secret := credentialScopeFixture(t, "viewer", true)

	w := credentialRequest(t, router, secret, http.MethodGet, "/api/v1/admin/users", "")
	if w.Code != http.StatusForbidden {
		t.Errorf("a viewer's credential reached an administrator's address: %d %s",
			w.Code, w.Body.String())
	}
}

// "Writing means the register of failures, and nothing else."
func TestAWritingCredentialCanRecordAFinding(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", true)

	body := `{"subject_name":"example-cookbook","subject_type":"git_repo",` +
		`"cookbook_name":"example-cookbook","verdict":"broken","reason":"it fails"}`
	w := credentialRequest(t, router, secret, http.MethodPost, "/api/v1/failure-register", body)
	if w.Code == http.StatusForbidden {
		t.Fatalf("a credential made to record findings cannot record one: %s", w.Body.String())
	}
}

// The other half of the same sentence: everything that is not the register.
func TestAWritingCredentialCannotReachAnythingElse(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", true)

	// Addresses this account could reach at a screen, spread across roles and
	// verbs, so the rule is shown to be about the address rather than about
	// permission.
	elsewhere := []struct{ method, path, body string }{
		{http.MethodDelete, "/api/v1/admin/users/somebody", ""},
		{http.MethodPost, "/api/v1/ownership/merge", `{}`},
		{http.MethodPut, "/api/v1/admin/config/collection", `{}`},
		{http.MethodPost, "/api/v1/cookstyle/custom-cops", `{}`},
		// Not even making another credential: a leaked one must not be able to
		// mint its own replacements.
		{http.MethodPost, "/api/v1/auth/me/tokens", `{"name":"another"}`},
		// Not revising or resolving somebody's entry either. Both are further
		// judgements about a finding, and neither records that a tool made the
		// change — a revision stores no author at all.
		{http.MethodPatch, "/api/v1/failure-register/some-id", `{"plan":"do it"}`},
		{http.MethodPost, "/api/v1/failure-register/some-id/resolve", `{"note":"fixed"}`},
	}
	for _, e := range elsewhere {
		w := credentialRequest(t, router, secret, e.method, e.path, e.body)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s answered %d for a credential that may only write to the failure "+
				"register — a credential that can write is not an unlocked service",
				e.method, e.path, w.Code)
		}
	}
}

// "read only if they do not choose" — and read-only has to mean it.
func TestAReadOnlyCredentialCannotWriteAnywhere(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	body := `{"subject_name":"example-cookbook","subject_type":"git_repo",` +
		`"cookbook_name":"example-cookbook","verdict":"broken","reason":"it fails"}`
	w := credentialRequest(t, router, secret, http.MethodPost, "/api/v1/failure-register", body)
	if w.Code != http.StatusForbidden {
		t.Errorf("a read-only credential recorded a finding (%d): %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "read") {
		t.Errorf("the refusal does not say the credential is read-only, so whoever hits it "+
			"cannot tell what to do about it: %s", w.Body.String())
	}
}

// A read-only credential still reads everything its account can.
func TestAReadOnlyCredentialStillReads(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", false)

	w := credentialRequest(t, router, secret, http.MethodGet, "/api/v1/failure-register", "")
	if w.Code == http.StatusForbidden {
		t.Errorf("a read-only credential cannot read: %s", w.Body.String())
	}
}

// "An entry it wrote must be visibly not mine."
func TestAnEntryACredentialWroteIsMarkedAsSuch(t *testing.T) {
	store := newMemCredentialStore().withUser("engineer", "admin")
	creds := auth.NewCredentialManager(store)
	_, secret, err := creds.Issue(context.Background(), "engineer", "editor", true)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	var recorded datastore.RecordFailureVerdictParams
	mock := &mockStore{
		RecordFailureVerdictFn: func(_ context.Context, p datastore.RecordFailureVerdictParams) (
			datastore.FailureRegisterEntry, error) {
			recorded = p
			return datastore.FailureRegisterEntry{}, nil
		},
	}

	hub := NewEventHub()
	go hub.Run()
	sessions := auth.NewSessionManager(mockSessionStore{}, 8*time.Hour)
	mw := auth.NewMiddleware(sessions, auth.WithCredentials(creds))
	router := NewRouter(mock, testConfigWithTargetVersions("19.0"), hub,
		WithAuth(auth.NewLocalAuthenticator(mockLocalAuthStore{}, 5), sessions, mw, nil),
		WithCredentialManager(creds),
	)

	body := `{"subject_name":"example-cookbook","subject_type":"git_repo",` +
		`"cookbook_name":"example-cookbook","verdict":"broken","reason":"it fails"}`
	credentialRequest(t, router, secret, http.MethodPost, "/api/v1/failure-register", body)

	if recorded.RaisedOrigin != datastore.OriginCredential {
		t.Errorf("an entry a tool wrote was recorded as origin %q, so it reads as its "+
			"owner's own judgement", recorded.RaisedOrigin)
	}
	if recorded.RaisedOriginName != "editor" {
		t.Errorf("the entry does not name the credential that wrote it (%q), so one tool "+
			"cannot be told from another", recorded.RaisedOriginName)
	}
	if recorded.RaisedBy != "engineer" {
		t.Errorf("the entry is attributed to %q rather than the credential's owner", recorded.RaisedBy)
	}
}

// A person at a screen is recorded as one. If everything came back "screen"
// the field would look right and mean nothing.
func TestAnEntryMadeAtAScreenIsNotMarkedAsAMachine(t *testing.T) {
	var recorded datastore.RecordFailureVerdictParams
	mock := &mockStore{
		RecordFailureVerdictFn: func(_ context.Context, p datastore.RecordFailureVerdictParams) (
			datastore.FailureRegisterEntry, error) {
			recorded = p
			return datastore.FailureRegisterEntry{}, nil
		},
	}

	body := `{"subject_name":"example-cookbook","subject_type":"git_repo",` +
		`"cookbook_name":"example-cookbook","verdict":"broken","reason":"it fails"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/failure-register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newTestRouterWithMockAndConfig(mock, testConfigWithTargetVersions("19.0")).
		ServeHTTP(w, withAdminSession(req))

	if recorded.RaisedOrigin != datastore.OriginScreen {
		t.Errorf("an entry typed at a screen was recorded as origin %q", recorded.RaisedOrigin)
	}
	if recorded.RaisedOriginName != "" {
		t.Errorf("an entry typed at a screen names a credential (%q)", recorded.RaisedOriginName)
	}
}

// "the service attaches that, never the caller." A body that tries to claim an
// origin must not be believed.
func TestACallerCannotClaimHowItGotIn(t *testing.T) {
	var recorded datastore.RecordFailureVerdictParams
	mock := &mockStore{
		RecordFailureVerdictFn: func(_ context.Context, p datastore.RecordFailureVerdictParams) (
			datastore.FailureRegisterEntry, error) {
			recorded = p
			return datastore.FailureRegisterEntry{}, nil
		},
	}

	// Every spelling a caller might reach for, sent at once.
	body := `{"subject_name":"example-cookbook","subject_type":"git_repo",` +
		`"cookbook_name":"example-cookbook","verdict":"broken","reason":"it fails",` +
		`"raised_origin":"screen","raised_origin_name":"","origin":"screen",` +
		`"access_method":"screen","raised_by":"somebody-else"}`

	store := newMemCredentialStore().withUser("engineer", "admin")
	creds := auth.NewCredentialManager(store)
	_, secret, err := creds.Issue(context.Background(), "engineer", "editor", true)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}
	hub := NewEventHub()
	go hub.Run()
	sessions := auth.NewSessionManager(mockSessionStore{}, 8*time.Hour)
	mw := auth.NewMiddleware(sessions, auth.WithCredentials(creds))
	router := NewRouter(mock, testConfigWithTargetVersions("19.0"), hub,
		WithAuth(auth.NewLocalAuthenticator(mockLocalAuthStore{}, 5), sessions, mw, nil),
		WithCredentialManager(creds),
	)

	credentialRequest(t, router, secret, http.MethodPost, "/api/v1/failure-register", body)

	if recorded.RaisedOrigin != datastore.OriginCredential {
		t.Errorf("a caller talked the service out of recording that a tool wrote this "+
			"(origin %q) — an entry a caller can sign as a person's own judgement is worse "+
			"than no attribution at all", recorded.RaisedOrigin)
	}
	if recorded.RaisedBy != "engineer" {
		t.Errorf("a caller set who the entry is attributed to (%q)", recorded.RaisedBy)
	}
}

// A destroyed credential stops working through the front door too, not only in
// the manager that destroyed it.
func TestADestroyedCredentialIsRefusedAtTheDoor(t *testing.T) {
	store := newMemCredentialStore().withUser("engineer", "admin")
	creds := auth.NewCredentialManager(store)
	tok, secret, err := creds.Issue(context.Background(), "engineer", "editor", false)
	if err != nil {
		t.Fatalf("issuing: %v", err)
	}

	hub := NewEventHub()
	go hub.Run()
	sessions := auth.NewSessionManager(mockSessionStore{}, 8*time.Hour)
	mw := auth.NewMiddleware(sessions, auth.WithCredentials(creds))
	router := NewRouter(&mockStore{}, testConfigWithTargetVersions("19.0"), hub,
		WithAuth(auth.NewLocalAuthenticator(mockLocalAuthStore{}, 5), sessions, mw, nil),
		WithCredentialManager(creds),
	)

	if w := credentialRequest(t, router, secret, http.MethodGet, "/api/v1/auth/me", ""); w.Code != http.StatusOK {
		t.Fatalf("the credential did not work before being destroyed: %d", w.Code)
	}
	if err := creds.Destroy(context.Background(), "engineer", tok.ID); err != nil {
		t.Fatalf("destroying: %v", err)
	}
	if w := credentialRequest(t, router, secret, http.MethodGet, "/api/v1/auth/me", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("a destroyed credential still gets in (%d), so destroying one is not a "+
			"remedy for it having leaked", w.Code)
	}
}

// An unattended job gets its own account rather than borrowing somebody's, so
// nothing here outlives the person who set it up. The account is what carries
// the access, and a credential issued from a machine's own record carries that
// account's level and no more.
func TestAnUnattendedJobsCredentialCarriesItsOwnAccountsLevel(t *testing.T) {
	router, secret := credentialScopeFixture(t, "viewer", false)

	// The machine account is a viewer, whatever the person who set it up is.
	if w := credentialRequest(t, router, secret, http.MethodGet, "/api/v1/auth/me", ""); w.Code != http.StatusOK {
		t.Fatalf("a machine account's credential cannot read: %d", w.Code)
	}
	if w := credentialRequest(t, router, secret, http.MethodGet,
		"/api/v1/admin/users", ""); w.Code != http.StatusForbidden {
		t.Errorf("a viewer machine account's credential reached an administrator's address "+
			"(%d), so the job's access is not its own account's", w.Code)
	}
}
