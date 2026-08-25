// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipsql"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// The endpoints an administrator sets a connection up through. See
// journeys/ownership-connection.md.
//
// The order these support: somebody stores the password, somebody composes the
// connection round a marker, somebody tests it, and only then is it stored and
// used. So testing works on a connection that has never been saved, and saving
// does not require a passing test — the server may be unreachable from here, and
// a save that demanded a green test could not record the connection at all.

const (
	testConnectionPath = "/api/v1/ownership/import/test-connection"
	showConnectionPath = "/api/v1/ownership/import/show-connection"
	connectionsPath    = "/api/v1/ownership/import/connections"
)

// newTestRouterForConnections builds a router with somewhere to keep
// connections and somewhere to keep the password beside them.
func newTestRouterForConnections(t *testing.T) *Router {
	t.Helper()
	creds := newMockCredentialStore(testCredentialEncryptor(t))
	mustCreateCredential(t, creds, "cmdb-password", secrets.CredentialTypeGeneric, "s3cr3t!p@ss")
	return newTestRouterForAdminConfig(nil, newTestConfigStore(t), nil,
		WithCredentialStore(creds))
}

func postAsAdmin(t *testing.T, r *Router, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, withAdminSession(jsonRequest(http.MethodPost, path, body)))
	return w
}

const savedConnectionBody = `{"name":"asset-database","driver":"sqlserver",` +
	`"connection":"` + `sqlserver://EXAMPLECORP\\svcaccount:` + ownershipsql.PasswordMarker +
	`@dbhost.example.com:1433?database=cmdb","password_credential":"cmdb-password"}`

// ---------------------------------------------------------------------------
// Storing one, and reading it back
// ---------------------------------------------------------------------------

func TestConnections_SavedOneReadsBackWithoutItsPassword(t *testing.T) {
	r := newTestRouterForConnections(t)

	// Baseline: nothing is stored, so what is read below was put there by this
	// call rather than by the fixture.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, withAdminSession(jsonRequest(http.MethodGet, connectionsPath, "")))
	if strings.Contains(w.Body.String(), "asset-database") {
		t.Fatal("the fixture proves nothing: a connection is listed before one was stored")
	}

	if w := postAsAdmin(t, r, connectionsPath, savedConnectionBody); w.Code != http.StatusOK {
		t.Fatalf("storing a connection answered %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, withAdminSession(jsonRequest(http.MethodGet, connectionsPath, "")))
	if w.Code != http.StatusOK {
		t.Fatalf("listing connections answered %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// Everything the administrator has to be able to check when it fails.
	for _, visible := range []string{
		"dbhost.example.com", "1433", "database=cmdb", "EXAMPLECORP", "svcaccount",
		ownershipsql.PasswordMarker, "cmdb-password",
	} {
		if !strings.Contains(body, visible) {
			t.Errorf("%q is not in what comes back, so it cannot be checked: %s", visible, body)
		}
	}
	// The one thing that must never come back.
	if strings.Contains(body, "s3cr3t") {
		t.Errorf("the password came back with the connection: %s", body)
	}
}

func TestConnections_OneThatDoesNotSayWhereThePasswordGoesIsRefused(t *testing.T) {
	r := newTestRouterForConnections(t)

	// Baseline: with the marker it is accepted, so the refusal below is about
	// the marker and not something else in the connection.
	if w := postAsAdmin(t, r, connectionsPath, savedConnectionBody); w.Code != http.StatusOK {
		t.Fatalf("the fixture proves nothing: even with the marker this is refused: %s", w.Body.String())
	}

	inline := `{"name":"password-written-in","driver":"sqlserver",` +
		`"connection":"sqlserver://svcaccount:hunter2@dbhost.example.com:1433?database=cmdb",` +
		`"password_credential":"cmdb-password"}`
	w := postAsAdmin(t, r, connectionsPath, inline)
	if w.Code == http.StatusOK {
		t.Fatal("a connection with the password written into it was stored as readable " +
			"configuration, where anybody can read it")
	}
	if !strings.Contains(w.Body.String(), ownershipsql.PasswordMarker) {
		t.Errorf("the refusal does not say how to mark the position: %s", w.Body.String())
	}
}

func TestConnections_NamingACredentialThatIsNotThereIsRefused(t *testing.T) {
	r := newTestRouterForConnections(t)

	body := strings.Replace(savedConnectionBody, "cmdb-password", "no-such-credential", 1)
	w := postAsAdmin(t, r, connectionsPath, body)
	if w.Code == http.StatusOK {
		t.Fatal("stored a connection whose password is nowhere, which fails at the moment " +
			"somebody tries to use it instead of now")
	}
	if !strings.Contains(w.Body.String(), "no-such-credential") {
		t.Errorf("the refusal does not name the credential it could not find: %s", w.Body.String())
	}
}

func TestConnections_ABodyCarryingSomethingWeCannotReadIsRefused(t *testing.T) {
	r := newTestRouterForConnections(t)

	body := `{"name":"asset-database","connection":"sqlserver://svc:` +
		ownershipsql.PasswordMarker + `@host:1433?database=cmdb",` +
		`"password_credential":"cmdb-password","passwrod":"typed-here-by-mistake"}`
	w := postAsAdmin(t, r, connectionsPath, body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d — a body with a field nobody reads is one where the "+
			"caller cannot tell what was acted on", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "passwrod") {
		t.Errorf("the refusal does not name the field: %s", w.Body.String())
	}
}

func TestConnections_OneCanBeReadAndDeletedByName(t *testing.T) {
	r := newTestRouterForConnections(t)
	if w := postAsAdmin(t, r, connectionsPath, savedConnectionBody); w.Code != http.StatusOK {
		t.Fatalf("storing: %s", w.Body.String())
	}
	const item = connectionsPath + "/asset-database"

	w := httptest.NewRecorder()
	r.ServeHTTP(w, withAdminSession(jsonRequest(http.MethodGet, item, "")))
	if w.Code != http.StatusOK {
		t.Fatalf("reading one back answered %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, withAdminSession(jsonRequest(http.MethodDelete, item, "")))
	if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
		t.Fatalf("deleting answered %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, withAdminSession(jsonRequest(http.MethodGet, item, "")))
	if w.Code != http.StatusNotFound {
		t.Errorf("after deleting, reading it answered %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestConnections_OneThatWasNeverStoredIsNotFound(t *testing.T) {
	r := newTestRouterForConnections(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, withAdminSession(jsonRequest(http.MethodGet, connectionsPath+"/no-such", "")))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// Being shown what will actually be sent
// ---------------------------------------------------------------------------

// "To be shown what will actually be sent, with the password masked."
//
// It answers without reading the password at all: the mask is substituted
// through the same escaping the real password goes through, so what comes back
// is the shape of the real connection and never the secret in it.
func TestConnections_ShowsWhatWillActuallyBeSent(t *testing.T) {
	r := newTestRouterForConnections(t)

	body := `{"driver":"sqlserver","connection":"` +
		`sqlserver://EXAMPLECORP\\svcaccount:` + ownershipsql.PasswordMarker +
		`@dbhost.example.com:1433?database=cmdb"}`
	w := postAsAdmin(t, r, showConnectionPath, body)
	if w.Code != http.StatusOK {
		t.Fatalf("showing the composed connection answered %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		Driver     string `json:"driver"`
		Connection string `json:"connection"`
		Form       string `json:"form"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding: %v\n  body: %s", err, w.Body.String())
	}
	if strings.Contains(got.Connection, ownershipsql.PasswordMarker) {
		t.Errorf("what I am shown still has the marker in it, so it is not what will be "+
			"sent: %s", got.Connection)
	}
	if !strings.Contains(got.Connection, ownershipsql.PasswordMask) {
		t.Errorf("the password is not masked in what I am shown: %s", got.Connection)
	}
	// The parts worth reading survive, including the encoding of the account —
	// which is on screen rather than behind the administrator.
	for _, visible := range []string{"dbhost.example.com", "1433", "database=cmdb", "EXAMPLECORP"} {
		if !strings.Contains(got.Connection, visible) {
			t.Errorf("%q is missing from what I am shown: %s", visible, got.Connection)
		}
	}
	if got.Form != string(ownershipsql.FormURL) {
		t.Errorf("form = %q, want %q — which escaping rule was applied is part of the answer",
			got.Form, ownershipsql.FormURL)
	}
}

// Showing a stored connection does not read its password.
//
// The one above cannot prove this: it composes a connection that has no stored
// password behind it, so nothing was there to leak. This one has a real
// password in the credential store and asserts it does not come back — which
// fails if the composing ever reaches for the real value to answer.
func TestConnections_ShowingAStoredOneNeverReadsThePassword(t *testing.T) {
	r := newTestRouterForConnections(t)
	if w := postAsAdmin(t, r, connectionsPath, savedConnectionBody); w.Code != http.StatusOK {
		t.Fatalf("storing: %s", w.Body.String())
	}

	// Baseline: the password really is retrievable, so its absence below is
	// this endpoint not fetching it rather than there being nothing to fetch.
	cred, err := r.credentialStore.Get(t.Context(), "cmdb-password")
	if err != nil {
		t.Fatalf("the fixture proves nothing: the password is not stored: %v", err)
	}
	password := string(cred.Plaintext)
	if password == "" {
		t.Fatal("the fixture proves nothing: the stored password is empty")
	}

	w := postAsAdmin(t, r, showConnectionPath, `{"name":"asset-database"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("showing a stored connection answered %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), password) {
		t.Errorf("the password is in what I am shown: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), ownershipsql.PasswordMask) {
		t.Errorf("nothing stands in for the password, so what I am shown is not what will "+
			"be sent: %s", w.Body.String())
	}
}

func TestConnections_ShowingOneWithNoMarkerSaysHowToMarkIt(t *testing.T) {
	r := newTestRouterForConnections(t)

	body := `{"driver":"sqlserver","connection":"sqlserver://svc@dbhost.example.com:1433?database=cmdb"}`
	w := postAsAdmin(t, r, showConnectionPath, body)
	if w.Code == http.StatusOK {
		t.Fatal("a connection that never says where the password goes was composed anyway")
	}
	if !strings.Contains(w.Body.String(), ownershipsql.PasswordMarker) {
		t.Errorf("the refusal does not tell me what to write: %s", w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Testing one
// ---------------------------------------------------------------------------

// "To test it before going any further" — before it is stored, because the
// order is compose, test, then keep.
func TestConnections_TestsOneThatWasNeverStored(t *testing.T) {
	if listening(t, "127.0.0.1:1") {
		t.Skip("something is listening on port 1, so this proves nothing")
	}
	r := newTestRouterForConnections(t)

	body := `{"driver":"sqlserver","connection":"sqlserver://svc:` +
		ownershipsql.PasswordMarker + `@127.0.0.1:1?database=cmdb",` +
		`"password_credential":"cmdb-password"}`
	w := postAsAdmin(t, r, testConnectionPath, body)
	if w.Code != http.StatusOK {
		t.Fatalf("testing a connection answered %d: %s", w.Code, w.Body.String())
	}

	var got ownershipsql.Result
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding: %v\n  body: %s", err, w.Body.String())
	}
	// Which of the five it was, so the answer names a person to go and talk to.
	if got.Outcome != ownershipsql.OutcomeUnreachable {
		t.Errorf("outcome = %q, want %q\n  detail: %s",
			got.Outcome, ownershipsql.OutcomeUnreachable, got.Detail)
	}
	if got.Detail == "" {
		t.Error("the refusal was tidied into nothing, which throws away the only thing in it " +
			"worth having")
	}
	// What was actually sent, which is the thing worth reading when it failed.
	if !strings.Contains(got.Connection, ownershipsql.PasswordMask) {
		t.Errorf("the test does not say what it sent: %q", got.Connection)
	}
	if strings.Contains(w.Body.String(), "s3cr3t") {
		t.Errorf("the password came back in the answer: %s", w.Body.String())
	}
}

// A stored connection is tested by name, and it is the stored one that is sent
// rather than anything the caller supplies alongside.
func TestConnections_TestsAStoredOneByName(t *testing.T) {
	if listening(t, "127.0.0.1:1") {
		t.Skip("something is listening on port 1, so this proves nothing")
	}
	r := newTestRouterForConnections(t)

	stored := strings.Replace(savedConnectionBody, "dbhost.example.com:1433", "127.0.0.1:1", 1)
	if w := postAsAdmin(t, r, connectionsPath, stored); w.Code != http.StatusOK {
		t.Fatalf("storing: %s", w.Body.String())
	}

	w := postAsAdmin(t, r, testConnectionPath, `{"name":"asset-database"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("testing a stored connection answered %d: %s", w.Code, w.Body.String())
	}
	var got ownershipsql.Result
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding: %v\n  body: %s", err, w.Body.String())
	}
	if !strings.Contains(got.Connection, "127.0.0.1") {
		t.Errorf("the stored connection was not the one sent: %q", got.Connection)
	}
	if got.Outcome != ownershipsql.OutcomeUnreachable {
		t.Errorf("outcome = %q, want %q\n  detail: %s",
			got.Outcome, ownershipsql.OutcomeUnreachable, got.Detail)
	}
}

// The password is read at the moment of testing, so a connection naming a
// credential that is not there is refused as that, and nothing is dialled.
func TestConnections_TestingOneWhosePasswordIsNowhereSaysSo(t *testing.T) {
	r := newTestRouterForConnections(t)

	body := `{"driver":"sqlserver","connection":"sqlserver://svc:` +
		ownershipsql.PasswordMarker + `@127.0.0.1:1?database=cmdb",` +
		`"password_credential":"no-such-credential"}`
	w := postAsAdmin(t, r, testConnectionPath, body)
	if w.Code == http.StatusOK {
		t.Fatal("tested a connection with no password, which reports as a refused login and " +
			"sends somebody to argue with the account owner")
	}
	if !strings.Contains(w.Body.String(), "no-such-credential") {
		t.Errorf("the refusal does not name the credential it could not find: %s", w.Body.String())
	}
}

// listening reports whether anything answers at addr, so a test that depends on
// nothing being there says so rather than failing for the wrong reason.
func listening(t *testing.T, addr string) bool {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
