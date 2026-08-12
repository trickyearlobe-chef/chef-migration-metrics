//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// The journey suite for journeys/agent-access.md. Run it with `make journey`.
//
// One test per thing the journey says has to be in place. Green means built,
// red means still to do — so running this recomputes the todo list rather than
// asking anybody to keep one true.
//
// Outside the gating suite on purpose. Most of this journey is unbuilt, so red
// is its normal state and must never block a build.

// agentJourneyRouter builds a router with a mock store. The mock cannot satisfy
// a real request, so these tests ask whether a capability is wired, not whether
// it returns correct data.
//
// The exception is credentials, which are wired to a real in-memory store: what
// this journey asks about them — that a destroyed one stops working, that the
// secret never comes back — cannot be answered by a router that only routes.
func agentJourneyRouter() *Router {
	return newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0"),
		WithCredentialManager(auth.NewCredentialManager(
			newMemCredentialStore().withUser("admin", "admin"))))
}

// agentJourneySend issues an authenticated request with a body and returns the
// recorder. The counterpart to agentJourneyGet, for the handful of things here
// that are not reads.
func agentJourneySend(t *testing.T, router *Router, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, withAdminSession(req))
	return w
}

// agentJourneyGet issues an authenticated GET and returns the recorder.
//
// Only paths under the API prefix are meaningful here: everything else falls
// through to the single-page application, which answers 200 for paths that
// serve nothing, and would read as a capability that exists.
func agentJourneyGet(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	if !strings.HasPrefix(path, "/api/") {
		t.Fatalf("probe %q is outside the API prefix; anything else is answered by the "+
			"frontend fallback and proves nothing", path)
	}
	w := httptest.NewRecorder()
	agentJourneyRouter().ServeHTTP(w, withAdminSession(httptest.NewRequest(http.MethodGet, path, nil)))
	return w
}

// agentJourneyServes reports whether any of the candidate paths is routed. A
// set rather than one path, because which address a capability lands on is the
// implementer's choice and this suite must not pre-decide it: any of these
// turns the test green with no edit here.
func agentJourneyServes(t *testing.T, paths ...string) (string, bool) {
	t.Helper()
	for _, p := range paths {
		if agentJourneyGet(t, p).Code != http.StatusNotFound {
			return p, true
		}
	}
	return "", false
}

// apiDescriptionCandidates are the conventional places a served OpenAPI
// document would live.
var apiDescriptionCandidates = []string{
	"/api/v1/openapi.json",
	"/api/v1/openapi",
	"/api/openapi.json",
	"/api/v1/swagger.json",
}

// agentJourneyDescription returns the served API description, or nil.
func agentJourneyDescription(t *testing.T) map[string]any {
	t.Helper()
	path, ok := agentJourneyServes(t, apiDescriptionCandidates...)
	if !ok {
		return nil
	}
	var doc map[string]any
	if err := json.Unmarshal(agentJourneyGet(t, path).Body.Bytes(), &doc); err != nil {
		return nil
	}
	return doc
}

// "Everything the API can answer, described in a form that is standard and
// current."
func TestJourney_TheAPIDescribesItself(t *testing.T) {
	doc := agentJourneyDescription(t)
	if doc == nil {
		t.Error("the service serves no machine-readable description of its API, so nobody " +
			"outside this project can say what it can be asked for")
		return
	}
	if doc["openapi"] == nil && doc["swagger"] == nil {
		t.Error("what is served is not an OpenAPI document, so standard tooling cannot read it")
	}
	if paths, _ := doc["paths"].(map[string]any); len(paths) == 0 {
		t.Error("the description names no paths, so it describes nothing")
	}
}

// "A description I cannot trust is worse for me than none" — every path the
// description claims must actually be served.
func TestJourney_EveryDescribedPathIsReallyServed(t *testing.T) {
	doc := agentJourneyDescription(t)
	if doc == nil {
		t.Skip("no API description is served yet; TestJourney_TheAPIDescribesItself is the gap")
	}
	paths, _ := doc["paths"].(map[string]any)
	router := agentJourneyRouter()
	for path := range paths {
		// Ask where the address lands rather than serving it. Serving it runs
		// the real handler against a store that holds nothing, so "no such
		// cookbook" and "no such endpoint" come back identical — and the two
		// mean opposite things to somebody reading the description. It also
		// runs write handlers, which a probe has no business doing.
		concrete := agentJourneyTemplateToPath(path)
		if _, matched := router.mux.Handler(
			httptest.NewRequest(http.MethodGet, concrete, nil)); matched == "/" {
			t.Errorf("the description promises %q, which nothing serves — it is answered by "+
				"the frontend fallback", path)
		}
	}
}

// "the day somebody renames something, I find out because a build went red" —
// the other direction: every route registered must appear in the description.
func TestJourney_EveryServedRouteIsDescribed(t *testing.T) {
	doc := agentJourneyDescription(t)
	if doc == nil {
		t.Skip("no API description is served yet; TestJourney_TheAPIDescribesItself is the gap")
	}
	described, _ := doc["paths"].(map[string]any)

	for _, rt := range agentJourneyRouter().Routes() {
		for _, addr := range describableAddresses(rt) {
			if _, ok := described[addr.path]; !ok {
				t.Errorf("%s is served and undescribed, so an assistant working from the "+
					"description does not know it can be asked", addr.path)
			}
		}
	}
}

// "And to tell what it can ask for without being told. What we expose has to
// name its own capabilities well enough that an assistant picks the right one
// from a long list."
//
// An assistant sees a flat list of capabilities and their descriptions, and
// nothing else. A capability with no description is one it will skip or misuse
// — the failure the field reports on our other tools describe: the right tool
// present, not found, and an agent settling for a worse one.
func TestJourney_EveryCapabilityDescribesItself(t *testing.T) {
	doc := agentJourneyDescription(t)
	if doc == nil {
		t.Skip("nothing describes the API yet; TestJourney_TheAPIDescribesItself is the gap")
	}
	paths, _ := doc["paths"].(map[string]any)
	for path, item := range paths {
		ops, _ := item.(map[string]any)
		for method, op := range ops {
			fields, _ := op.(map[string]any)
			if fields == nil {
				continue
			}
			summary, _ := fields["summary"].(string)
			description, _ := fields["description"].(string)
			if strings.TrimSpace(summary) == "" && strings.TrimSpace(description) == "" {
				t.Errorf("%s %s says nothing about what it is for, so an assistant scanning "+
					"the list cannot tell whether it is the right one", method, path)
			}
		}
	}
}

// "A credential I can get for myself, from my own record, without raising a
// ticket."
func TestJourney_ICanIssueMyselfACredential(t *testing.T) {
	if _, ok := agentJourneyServes(t,
		"/api/v1/auth/me/tokens",
		"/api/v1/auth/me/api-tokens",
		"/api/v1/me/tokens",
		"/api/v1/users/me/tokens",
	); !ok {
		t.Error("there is no way for a person to get an API credential from their own record; " +
			"an integration can only hold that person's login")
	}
}

// "I want to be able to throw it away and get a new one the moment I am unsure
// about it" — and "I want to see that I have one and roughly when it was last
// used."
//
// Both halves are exercised against a real credential, because a listing that
// is empty answers the question by having nothing in it.
func TestJourney_ICanSeeAndDestroyMyCredential(t *testing.T) {
	path, ok := agentJourneyServes(t,
		"/api/v1/auth/me/tokens",
		"/api/v1/auth/me/api-tokens",
		"/api/v1/me/tokens",
		"/api/v1/users/me/tokens",
	)
	if !ok {
		t.Skip("credentials cannot be issued yet; TestJourney_ICanIssueMyselfACredential is the gap")
	}

	router := agentJourneyRouter()
	if w := agentJourneySend(t, router, http.MethodPost, path,
		`{"name":"editor"}`); w.Code != http.StatusCreated {
		t.Fatalf("creating a credential answered %d: %s", w.Code, w.Body.String())
	}

	listing := agentJourneyListTokens(t, router, path)
	if len(listing) != 1 {
		t.Fatalf("a person made one credential and their record shows %d, so they cannot "+
			"tell what exists in their name", len(listing))
	}
	id, _ := listing[0]["id"].(string)
	if id == "" {
		t.Fatal("a credential in the listing has no id, so there is no way to say which " +
			"one to destroy")
	}
	if _, ok := listing[0]["last_used_at"]; !ok {
		// Absent is how "never used" is reported, which is fine — what would
		// not be fine is the field never appearing once it has been used. That
		// is checked in internal/auth, where the clock is.
		t.Log("last_used_at is absent for a credential that has never been used")
	}

	w := agentJourneySend(t, router, http.MethodDelete, path+"/"+id, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("destroying a credential answered %d: %s — the one thing somebody does "+
			"when they think it has leaked is the thing that has to work", w.Code, w.Body.String())
	}
	if after := agentJourneyListTokens(t, router, path); len(after) != 0 {
		t.Errorf("a destroyed credential is still in its owner's record (%d left)", len(after))
	}
}

// "Shown once, destroyable instantly. No recovering an old one."
//
// The secret comes back from creation and from nowhere else. If it could be
// read a second time then destroying a credential would not be the remedy for
// believing somebody else has it — everyone who can read the listing has it.
func TestJourney_TheSecretIsShownOnceAndNeverAgain(t *testing.T) {
	path, ok := agentJourneyServes(t,
		"/api/v1/auth/me/tokens",
		"/api/v1/auth/me/api-tokens",
		"/api/v1/me/tokens",
		"/api/v1/users/me/tokens",
	)
	if !ok {
		t.Skip("credentials cannot be issued yet; TestJourney_ICanIssueMyselfACredential is the gap")
	}

	router := agentJourneyRouter()
	created := agentJourneySend(t, router, http.MethodPost, path, `{"name":"editor"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("creating a credential answered %d: %s", created.Code, created.Body.String())
	}

	var minted struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &minted); err != nil {
		t.Fatalf("reading what creating a credential returned: %v", err)
	}
	if strings.TrimSpace(minted.Secret) == "" {
		t.Fatal("creating a credential returned no secret, so there is nothing to put in " +
			"an editor and the credential is useless")
	}

	// The whole of the listing, as text: whatever it is called and however it
	// is nested, the secret must not be in it.
	again := agentJourneyGet(t, path)
	if strings.Contains(again.Body.String(), minted.Secret) {
		t.Error("the secret can be read back from the listing, so it is not shown once — " +
			"and destroying a credential is no longer a remedy for it having leaked")
	}
}

// agentJourneyListTokens reads the credential listing as plain maps, so a test
// can ask what fields are present without fixing their names here.
func agentJourneyListTokens(t *testing.T, router *Router, path string) []map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, withAdminSession(httptest.NewRequest(http.MethodGet, path, nil)))

	var listing struct {
		Tokens []map[string]any `json:"tokens"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil {
		t.Fatalf("a person cannot see which credentials exist in their name (status %d): %v",
			w.Code, err)
	}
	return listing.Tokens
}

// "The assistant-facing surface is part of the service. Not a companion
// process, not a sidecar."
func TestJourney_TheAssistantSurfaceIsBuiltIn(t *testing.T) {
	if _, ok := agentJourneyServes(t,
		"/api/v1/mcp",
		"/api/mcp",
		"/api/v1/mcp/sse",
	); !ok {
		t.Error("the service hosts nothing an editor assistant can connect to, so using it " +
			"means deploying a second thing next to it — which is exactly what cannot be " +
			"done inside a customer estate")
	}
}

// "the error and the trace under it, which cookbook was running at the time,
// and which machines it happened on."
//
// Run events are gated off by default and the gate answers as though the
// address does not exist, so this probe turns the feature on first. Otherwise
// the test would report a capability as missing when it is only switched off,
// which is the kind of red that teaches people to ignore the list.
func TestJourney_FailureDetailIsReachable(t *testing.T) {
	on := true
	cfg := testConfigWithTargetVersions("19.0")
	cfg.Ingest.ShowRunEvents = &on
	w := httptest.NewRecorder()
	newTestRouterWithMockAndConfig(&mockStore{}, cfg).ServeHTTP(
		w, withAdminSession(httptest.NewRequest(http.MethodGet, "/api/v1/run-events/runs", nil)))
	if w.Code == http.StatusNotFound {
		t.Error("what came back from a failed run is not reachable, so an assistant can " +
			"read the verdict but not the failure")
	}
}

// "the contents of the file it points at, out of the repository we already
// hold."
func TestJourney_SourceFileContentIsReachable(t *testing.T) {
	if _, ok := agentJourneyServes(t,
		"/api/v1/git-repos/example-cookbook/files",
		"/api/v1/git-repos/example-cookbook/files/content",
	); !ok {
		t.Error("the source of a cookbook we already hold is not reachable, so an assistant " +
			"has to guess at the recipe from its name")
	}
}

// "specifically our reclassified version of that, where we have decided which
// of those findings actually block the upgrade and which are tidying."
func TestJourney_OurClassificationOfTheStaticCheckIsReachable(t *testing.T) {
	if _, ok := agentJourneyServes(t, "/api/v1/cookstyle/cops"); !ok {
		t.Error("our judgement about which static findings block the upgrade is not reachable, " +
			"so an assistant reads the raw tool's opinion and ranks the wrong things first")
	}
}

// "what happened when we last ran the cookbook on a real machine, which is the
// only evidence that outranks the static check."
func TestJourney_RealMachineRunResultsAreReachable(t *testing.T) {
	if _, ok := agentJourneyServes(t,
		"/api/v1/kitchen/analysis/cookbooks",
		"/api/v1/kitchen/node-runs",
	); !ok {
		t.Error("what happened when the cookbook was last run on a real machine is not " +
			"reachable, so an assistant has only the static check to go on")
	}
}

// "no answer may ever be unbounded by default" — and "a caller asking for more
// than it gets less, not more."
func TestJourney_NoAnswerIsUnboundedByDefault(t *testing.T) {
	asked := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)
	if pg := ParsePagination(asked); pg.Limit() <= 0 {
		t.Error("a caller that asks for a list with no size gets everything, which fills an " +
			"assistant's whole working room in one answer")
	}
	greedy := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?per_page=1000000", nil)
	if ParsePagination(greedy).Limit() > 500 {
		t.Error("a caller can ask for an unbounded answer and get one; the ceiling has to be " +
			"ours, not the caller's")
	}
}

// "to fetch a page at a time" — a caller must be told how much it has not seen,
// or it cannot know to ask again.
func TestJourney_AListSaysHowMuchWasNotReturned(t *testing.T) {
	pg := ParsePagination(httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil))
	resp := NewPaginationResponse(pg, 12345)
	if resp.TotalItems != 12345 || resp.TotalPages <= 1 {
		t.Errorf("a page does not say how much was left behind (%+v), so an assistant cannot "+
			"tell a complete answer from the first fiftieth of one", resp)
	}
}

// "the same narrowing I do on the screen has to be available to it."
func TestJourney_TheSameNarrowingIsAvailable(t *testing.T) {
	all := fmt.Sprintf("%+v", cookbookFilterFromValues(url.Values{}, nil, "19.0"))
	narrowed := fmt.Sprintf("%+v",
		cookbookFilterFromValues(url.Values{"name": {"apache"}, "compatibility": {"incompatible"}}, nil, "19.0"))
	if all == narrowed {
		t.Error("narrowing a list the way the screen does has no effect on what is fetched, " +
			"so an assistant has to pull everything and filter it in its own head")
	}
}

// "It should be able to ask for the shape of a thing first — how many, grouped
// how — and only then pull the handful it actually needs to read closely."
func TestJourney_CanAskForTheShapeBeforeTheDetail(t *testing.T) {
	if _, ok := agentJourneyServes(t, "/api/v1/cookstyle/cops"); !ok {
		t.Error("there is no way to ask how the findings group up before reading them, so the " +
			"only route to an overview is to read every finding")
	}
}

// "when an entry is being carried by a ticket rather than a person, the
// reference is already sitting in the entry."
//
// This is what makes coordinating through a ticketing system possible at all,
// so it is worth pinning here even though it was built for other reasons: if a
// commitment stopped being able to name a ticket, this journey would quietly
// lose half of what it is for.
func TestJourney_AnEntryCanNameTheTicketCarryingIt(t *testing.T) {
	var recorded datastore.RecordFailureVerdictParams
	store := &mockStore{
		RecordFailureVerdictFn: func(_ context.Context, p datastore.RecordFailureVerdictParams) (
			datastore.FailureRegisterEntry, error) {
			recorded = p
			return datastore.FailureRegisterEntry{}, nil
		},
	}
	body := `{"subject_name":"example-cookbook","subject_type":"git_repo",` +
		`"cookbook_name":"example-cookbook","verdict":"broken","reason":"it is",` +
		`"holder_type":"` + datastore.HolderTypeTicket + `","holder_ref":"EXAMPLE-123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/failure-register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0")).
		ServeHTTP(w, withAdminSession(req))

	if recorded.HolderRef != "EXAMPLE-123" || recorded.HolderType != datastore.HolderTypeTicket {
		t.Errorf("an entry cannot say which ticket is carrying it (status %d, recorded %+v), so "+
			"an assistant that can also reach our ticketing system has no thread to follow",
			w.Code, recorded)
	}
}

// "an entry it wrote must be visibly not mine" — the condition the open
// question turns on.
func TestJourney_AnEntryAMachineWroteIsMarkedAsSuch(t *testing.T) {
	var found string
	entry := reflect.TypeOf(datastore.FailureRegisterEntry{})
	for i := range entry.NumField() {
		switch name := entry.Field(i).Name; {
		case strings.Contains(name, "Author"),
			strings.Contains(name, "Machine"),
			strings.Contains(name, "Agent"),
			strings.Contains(name, "Assistant"),
			strings.Contains(name, "Origin"):
			found = name
		}
	}
	if found == "" {
		t.Error("an entry records who raised it but not what raised it, so a note written by " +
			"an assistant would read as its owner's own judgement and no later reader could " +
			"tell — which is the condition that has to hold before writing is allowed at all")
	}
}

// "How something got in is settled when it signs in, not claimed as it asks...
// the service attaches that, never the caller."
//
// The session is what every request is judged by and the only thing the caller
// cannot write, so it is where the way in has to be recorded. A field on the
// request — a header, a client name — would be a claim, and an audit entry a
// caller can set reads as fact without being one.
//
// It sits beside the provider a session already records, which is the same kind
// of fact assigned the same way, so this is checked against the session rather
// than against anything an assistant would have to build.
func TestJourney_TheWayInIsRecordedOnTheSessionNotTheRequest(t *testing.T) {
	var found string
	session := reflect.TypeOf(auth.SessionInfo{})
	for i := range session.NumField() {
		switch name := session.Field(i).Name; {
		case strings.Contains(name, "AccessMethod"),
			strings.Contains(name, "Origin"),
			strings.Contains(name, "Channel"),
			strings.Contains(name, "ViaCredential"):
			found = name
		}
	}
	if found == "" {
		t.Error("a session records who signed in but not how they got in, so nothing can tell " +
			"an entry made at a screen from one made by a credential somebody handed to a tool " +
			"— and the only other place to put it is something the caller sends, which it could " +
			"set to anything")
	}
}

// "I want to choose, at the moment I make a credential, whether it can only
// read or can also write" — and "read only is what they get if they do not
// choose."
func TestJourney_ICanChooseWhetherMyCredentialCanWrite(t *testing.T) {
	path, ok := agentJourneyServes(t,
		"/api/v1/auth/me/tokens",
		"/api/v1/auth/me/api-tokens",
		"/api/v1/me/tokens",
		"/api/v1/users/me/tokens",
	)
	if !ok {
		t.Error("credentials cannot be issued at all, so there is no point at which a person " +
			"chooses read-only or read-and-write, and no default for them to fall back to")
		return
	}
	var listing struct {
		Tokens []struct {
			CanWrite *bool `json:"can_write"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(agentJourneyGet(t, path).Body.Bytes(), &listing); err != nil {
		t.Errorf("what a credential is allowed to do is not visible to its owner: %v", err)
		return
	}
	for i, tok := range listing.Tokens {
		if tok.CanWrite == nil {
			t.Errorf("credential %d does not say whether it can write, so its owner cannot "+
				"tell a question-asking credential from one that records findings", i)
		}
	}
}

// "Writing means the register of failures, and nothing else."
//
// Exercised end to end, with a real credential in the header, in
// credential_scope_test.go — TestAWritingCredentialCannotReachAnythingElse and
// TestAReadOnlyCredentialCannotWriteAnywhere. The check is repeated here in
// one line so that this journey's list does not read as though the question is
// still open.
func TestJourney_AWritingCredentialCannotReachAnythingElse(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", true)

	verdict := `{"subject_name":"example-cookbook","subject_type":"git_repo",` +
		`"cookbook_name":"example-cookbook","verdict":"broken","reason":"it fails"}`
	if w := credentialRequest(t, router, secret, http.MethodPost,
		"/api/v1/failure-register", verdict); w.Code == http.StatusForbidden {
		t.Errorf("a credential made to record findings cannot record one: %s", w.Body.String())
	}
	if w := credentialRequest(t, router, secret, http.MethodPut,
		"/api/v1/admin/config/collection", `{}`); w.Code != http.StatusForbidden {
		t.Errorf("a credential that can write reached beyond the failure register (%d), so "+
			"handing one to a tool hands over the whole service", w.Code)
	}
}

// "It acts as me, at my level of access, and it can see exactly what I can see
// on the screen and nothing else."
//
// A credential belongs to an account, and an account may belong to a machine as
// easily as to a person — an unattended job gets its own rather than borrowing
// somebody's, and the assistant in an editor holds one of mine and acts as me.
// Either way there is one permissions model, and this is the test of that.
func TestJourney_TheCredentialCarriesItsAccountsLevelAndNoMore(t *testing.T) {
	// Reads as its own account.
	admin, adminSecret := credentialScopeFixture(t, "admin", false)
	if w := credentialRequest(t, admin, adminSecret, http.MethodGet,
		"/api/v1/admin/users", ""); w.Code == http.StatusForbidden {
		t.Errorf("an administrator's credential cannot see what that administrator sees on "+
			"screen: %s", w.Body.String())
	}

	// And no further. Same address, an account a level below.
	viewer, viewerSecret := credentialScopeFixture(t, "viewer", false)
	if w := credentialRequest(t, viewer, viewerSecret, http.MethodGet,
		"/api/v1/admin/users", ""); w.Code != http.StatusForbidden {
		t.Errorf("a viewer's credential reached an administrator's address (%d), so a "+
			"credential is a second permissions model rather than another way into the "+
			"same account", w.Code)
	}
}

// agentJourneyTemplateToPath turns an OpenAPI path template into something
// routable by substituting a placeholder for each named parameter.
func agentJourneyTemplateToPath(template string) string {
	var b strings.Builder
	depth := 0
	for _, r := range template {
		switch {
		case r == '{':
			depth++
			if depth == 1 {
				b.WriteString("placeholder")
			}
		case r == '}':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}
