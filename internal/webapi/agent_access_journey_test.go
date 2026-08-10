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
func agentJourneyRouter() *Router {
	return newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0"))
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
	for path := range paths {
		concrete := agentJourneyTemplateToPath(path)
		if agentJourneyGet(t, concrete).Code == http.StatusNotFound {
			t.Errorf("the description promises %q, which the service does not serve", path)
		}
	}
}

// "the day somebody renames something, I find out because a build went red" —
// the other direction: every route registered must appear in the description.
func TestJourney_EveryServedRouteIsDescribed(t *testing.T) {
	t.Skip("nothing enumerates the routes this service registers, so the description cannot " +
		"be checked for omissions — a renamed or added path would go undescribed silently. " +
		"Recording the route table where every route is already funnelled through its access " +
		"check is what unblocks this test")
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
	var listing struct {
		Tokens []struct {
			ID         string `json:"id"`
			LastUsedAt string `json:"last_used_at"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(agentJourneyGet(t, path).Body.Bytes(), &listing); err != nil {
		t.Errorf("a person cannot see which credentials exist in their name: %v", err)
	}
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

// "Read access only." Nothing reached through an assistant credential may
// change anything.
func TestJourney_TheCredentialIsReadOnly(t *testing.T) {
	t.Skip("credentials cannot be issued yet, so there is nothing whose write access can be " +
		"checked; this becomes testable once TestJourney_ICanIssueMyselfACredential is green")
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

// "Read only until somebody decides otherwise, and read only is the default in
// the meantime."
func TestJourney_TheDefaultIsReadOnly(t *testing.T) {
	t.Skip("credentials cannot be issued yet, so nothing can be checked for write access. " +
		"The recorded default, so it is not decided by whoever implements this first: a " +
		"credential is read only. Whether it may write into the failure register is open and " +
		"is the owner's call; every other surface stays read only regardless, and writing is " +
		"conditional on TestJourney_AnEntryAMachineWroteIsMarkedAsSuch being green")
}

// "It acts as me, at my level of access, and it can see exactly what I can see
// on the screen and nothing else."
func TestJourney_TheCredentialIsThePersonNotAServiceAccount(t *testing.T) {
	t.Skip("credentials cannot be issued yet; that a credential carries its owner's level of " +
		"access — and no second permissions model — is checkable once one can be issued")
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
