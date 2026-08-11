//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// The journey suite for journeys/api-integration.md. Run it with `make journey`.
//
// One test per thing the journey says has to be in place. Green means built,
// red means still to do.
//
// Shares helpers with the assistant journey's suite in
// agent_access_journey_test.go — the two journeys deliberately read the same
// API description, so testing it twice from two sets of probes would be the
// second copy this whole idea exists to avoid.

// "The whole surface written down, in OpenAPI."
//
// The same requirement the assistant journey carries, asserted once. This test
// exists so this journey's list is not silently missing the thing it depends on
// most; it will go green at the same moment the other one does.
func TestJourney_TheDescriptionThisIsBuiltAgainstExists(t *testing.T) {
	if agentJourneyDescription(t) == nil {
		t.Error("there is no OpenAPI document, so there is nothing to generate a client from " +
			"and the only way to learn the API is to read the browser's network traffic")
	}
}

// "All of it, not a page at a time. When I load the estate into another system
// I need the estate, not the first fifty of it."
func TestJourney_CanPullTheWholeOfSomething(t *testing.T) {
	w := httptest.NewRecorder()
	agentJourneyRouter().ServeHTTP(w, withAdminSession(httptest.NewRequest(
		http.MethodPost, "/api/v1/exports?export_type=nodes&format=csv", nil)))
	if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
		t.Errorf("there is no way to ask for the whole of something (status %d), so a load "+
			"into another system has to be assembled a page at a time", w.Code)
	}
}

// "Completeness is asked for, not stumbled into."
func TestJourney_CompletenessIsADeliberateRequest(t *testing.T) {
	greedy := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks?per_page=1000000", nil)
	if ParsePagination(greedy).Limit() > 500 {
		t.Error("an ordinary list hands over everything to a caller that asks for it, so the " +
			"whole estate can arrive by accident rather than on purpose")
	}
}

// journeyRoutes reports whether a path is served at all.
//
// A missing record and a missing endpoint both answer 404, and only the message
// tells them apart — asking about a cookbook the empty mock does not hold is
// "not found", not "there is no way to ask". Treating the two the same would
// have reported four capabilities as unbuilt when all four exist, which is the
// kind of red that teaches people to stop reading the list.
func journeyRoutes(t *testing.T, path string) bool {
	t.Helper()
	w := agentJourneyGet(t, path)
	if w.Code != http.StatusNotFound {
		return true
	}
	return !strings.Contains(w.Body.String(), "API endpoint")
}

// "The organisations, the machines, the cookbooks, the repositories — get the
// list, then get everything we hold about any single one of them."
func TestJourney_EveryEntityCanBeListedAndThenAskedAbout(t *testing.T) {
	for _, e := range []struct{ list, one string }{
		{"/api/v1/organisations", ""},
		{"/api/v1/nodes", "/api/v1/nodes/example-org/example-node"},
		{"/api/v1/cookbooks", "/api/v1/cookbooks/example-cookbook"},
		{"/api/v1/git-repos", "/api/v1/git-repos/example-repo"},
		{"/api/v1/roles", "/api/v1/roles/example-role"},
	} {
		if !journeyRoutes(t, e.list) {
			t.Errorf("%s cannot be listed, so there is nothing to start from", e.list)
		}
		if e.one != "" && !journeyRoutes(t, e.one) {
			t.Errorf("%s can be listed but no single one of them can be asked about, so the "+
				"list is all anybody outside can have", e.list)
		}
	}
}

// "what the static check said about it, what happened when it was last run on a
// real machine."
//
// Seeded, because an empty store answers with an empty list and every field
// would read as absent.
func TestJourney_TheResultsWeHoldComeBackWithTheThing(t *testing.T) {
	store := &mockStore{
		ListCookbooksFilteredFn: func(_ context.Context, _ datastore.CookbookFilter) (
			[]datastore.CookbookFilterRow, int, error) {
			return []datastore.CookbookFilterRow{{
				Name: "example-cookbook", Version: "1.0.0",
				Compatibility:   "incompatible",
				CookstyleStatus: "blocked",
				TKStatus:        "failed",
			}}, 1, nil
		},
	}
	w := httptest.NewRecorder()
	newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0")).ServeHTTP(
		w, withAdminSession(httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)))

	for _, held := range []string{"cookstyle_status", "tk_status", "compatibility"} {
		if !strings.Contains(w.Body.String(), held) {
			t.Errorf("a cookbook comes back without what we know about it (%q absent), so an "+
				"integration gets names and has to ask somebody here where the results live",
				held)
		}
	}
}

// "It has to be possible to tell 'nothing changed' from 'this failed' without a
// human reading a message."
func TestJourney_FailureIsMachineReadable(t *testing.T) {
	w := httptest.NewRecorder()
	agentJourneyRouter().ServeHTTP(w, withAdminSession(httptest.NewRequest(
		http.MethodPost, "/api/v1/exports?export_type=nonsense&format=csv", nil)))
	if w.Code < 400 {
		t.Errorf("a request that cannot be satisfied answers %d, so an unattended job cannot "+
			"tell it failed", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct == "" || ct[:16] != "application/json" {
		t.Errorf("a failure is reported as %q rather than something a program can read", ct)
	}
}

// "Answers that keep their shape... I would rather be told an upgrade will
// break me than find out that way."
func TestJourney_TheShapeCannotChangeUnderACaller(t *testing.T) {
	t.Skip("nothing records what an answer looked like at a release, so nothing can fail when " +
		"a field changes meaning. Generating the description from the code stops it going " +
		"stale but says nothing about it changing — a caller finds out weeks later, in another " +
		"system's numbers. Closing this needs the shape recorded and compared, not just described")
}

// "Whether it survives contact with an unattended job is the open question."
func TestJourney_AnUnattendedJobOutlivesThePersonWhoSetItUp(t *testing.T) {
	t.Skip("credentials cannot be issued at all yet, so nothing about an unattended job's " +
		"access can be checked. Recorded so it is not decided by whoever builds it first: the " +
		"assistant journey decided a credential is a person at that person's level of access, " +
		"and nobody has decided whether that holds for a program that runs nightly and outlives " +
		"the engineer who set it up. It is the owner's call, not an implementation detail")
}
