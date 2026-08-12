//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

// "If a cookbook has no repository, say so — do not send me the same blank I
// would get for a question nobody has answered yet."
func TestJourney_StatesCanBeToldApart(t *testing.T) {
	rows := []datastore.CookbookFilterRow{
		{Name: "has-no-repo", Version: "1.0.0", TKStatus: "no_repo"},
		{Name: "never-tested", Version: "1.0.0", TKStatus: ""},
	}
	store := &mockStore{
		ListCookbooksFilteredFn: func(_ context.Context, _ datastore.CookbookFilter) (
			[]datastore.CookbookFilterRow, int, error) {
			return rows, len(rows), nil
		},
	}
	w := httptest.NewRecorder()
	newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0")).ServeHTTP(
		w, withAdminSession(httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks", nil)))

	var body struct {
		Data []struct {
			Name     string  `json:"name"`
			TKStatus *string `json:"tk_status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("cookbook list did not answer: %v", err)
	}
	if len(body.Data) != len(rows) {
		t.Fatalf("expected %d cookbooks, got %d", len(rows), len(body.Data))
	}
	for _, cb := range body.Data {
		if cb.TKStatus == nil {
			t.Errorf("%s: the state is missing from the answer entirely, so a program cannot "+
				"tell 'we do not know' from 'this version has no such field'", cb.Name)
		}
	}
	if body.Data[0].TKStatus != nil && *body.Data[0].TKStatus != "no_repo" {
		t.Errorf("having no repository is sent as %q, the same thing an unanswered question "+
			"sends, so two different states arrive identical", *body.Data[0].TKStatus)
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
	// What an answer looked like is recorded, and compared on every build —
	// see openapi_shape_record_test.go, which is in the gating suite rather
	// than in this one, because a guard that only runs when somebody asks for
	// the journey list is not a guard.
	raw, err := os.ReadFile(shapeRecordPath)
	if err != nil {
		t.Fatalf("nothing records what an answer looked like, so nothing fails when a field "+
			"changes meaning and the first anybody hears of it is another system's numbers "+
			"being wrong: %v", err)
	}
	var recorded map[string]any
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatalf("the recording cannot be read, so nothing is being compared against it: %v", err)
	}
	if len(recorded) == 0 {
		t.Fatal("the recording is empty, so every shape can change without anything failing")
	}

	// The recording is of this build, not of some build. One that has drifted
	// from what is served is worse than none: it passes, and it is describing
	// something nobody is running.
	current := recordedShapes(t)
	for op, was := range recorded {
		now, still := current[op]
		if !still {
			t.Errorf("%s is recorded but no longer describes an answer at all", op)
			continue
		}
		before, _ := json.Marshal(was)
		after, _ := json.Marshal(now)
		if string(before) != string(after) {
			t.Errorf("%s answers with a shape the recording does not match, so an upgrade "+
				"changed it under whoever built against it", op)
		}
	}
}

// "Whether it survives contact with an unattended job is the open question."
//
// Answered: a job that runs unattended gets an account of its own — local, or
// one the identity provider already carries for a machine — rather than
// borrowing the account of whoever set it up. So there is nothing here that
// outlives anybody, and what is left to check is that a credential issued from
// a machine's own record carries that account's level and no more.
func TestJourney_AnUnattendedJobOutlivesThePersonWhoSetItUp(t *testing.T) {
	// The machine's own account, at whatever level it was given — not the
	// level of the person who configured the job.
	router, secret := credentialScopeFixture(t, "viewer", false)

	if w := credentialRequest(t, router, secret, http.MethodGet,
		"/api/v1/auth/me", ""); w.Code != http.StatusOK {
		t.Fatalf("a machine account's credential cannot read at all (%d), so an unattended "+
			"job has no way in of its own: %s", w.Code, w.Body.String())
	}
	if w := credentialRequest(t, router, secret, http.MethodGet,
		"/api/v1/admin/users", ""); w.Code != http.StatusForbidden {
		t.Errorf("the job's credential reached beyond its own account's level (%d), which "+
			"means it is carrying somebody else's access and would outlive them", w.Code)
	}
}
