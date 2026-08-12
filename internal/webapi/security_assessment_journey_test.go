//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The journey suite for journeys/security-assessment.md. Run it with
// `make journey`.
//
// One test per thing the journey says has to be in place. Green means built,
// red means still to do — and red here is the normal state for most of a
// journey's life. Nothing in this file gates a release.
//
// Shares helpers with the other two API journeys' suites: all three read the
// same description, and testing it three times from three sets of probes would
// be the copy the whole idea exists to avoid.

// "The whole surface, from the service itself... fetched from the instance I am
// testing."
func TestJourney_TheSurfaceComesFromTheInstanceUnderTest(t *testing.T) {
	if agentJourneyDescription(t) == nil {
		t.Error("the instance does not describe itself, so an assessment is driven from " +
			"whatever the tester happened to find in the browser — and what they did not " +
			"find is what ships untested")
	}
}

// "What each call takes... to send the wrong thing I have to know what the
// right thing was."
func TestJourney_EveryWriteSaysWhatItTakes(t *testing.T) {
	doc := agentJourneyDescription(t)
	paths, _ := doc["paths"].(map[string]any)

	var silent []string
	for path, item := range paths {
		operations, _ := item.(map[string]any)
		for method, op := range operations {
			switch strings.ToUpper(method) {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
			default:
				continue
			}
			operation, _ := op.(map[string]any)
			if operation["requestBody"] == nil {
				continue // reads nothing from the body, which is said by saying nothing
			}
			content, _ := operation["requestBody"].(map[string]any)["content"].(map[string]any)
			media, _ := content["application/json"].(map[string]any)
			schema, _ := media["schema"].(map[string]any)
			if schema != nil && len(schema) == 0 {
				silent = append(silent, strings.ToUpper(method)+" "+path)
			}
		}
	}
	// One is deliberate and says so alongside itself: the telemetry sink does
	// not decide the shape it receives.
	if len(silent) > 1 {
		t.Errorf("%d calls take a body of an unstated shape (%s), so there is nothing to "+
			"build a corpus from and the tester sends guesses",
			len(silent), strings.Join(silent, ", "))
	}
}

// "What each call needs to be allowed... including where a call is refused
// inside the handler rather than at the door."
func TestJourney_EveryCallSaysWhatAccessItNeeds(t *testing.T) {
	doc := agentJourneyDescription(t)
	paths, _ := doc["paths"].(map[string]any)

	for path, item := range paths {
		operations, _ := item.(map[string]any)
		for method, op := range operations {
			operation, _ := op.(map[string]any)
			if operation["x-required-role"] == nil {
				t.Errorf("%s %s does not say what access it needs, so testing whether a low "+
					"account reaches it has nothing to compare against",
					strings.ToUpper(method), path)
			}
		}
	}
}

// "A refusal that is a refusal. If I send a field this thing does not
// understand and get back a 200, I cannot tell what it acted on."
//
// Red, and measured rather than assumed: every call that reads a JSON body
// decodes it without rejecting unknown fields, so a misspelled field is
// accepted and dropped. Recorded in plans/todo-tech-debt.md.
func TestJourney_SomethingItCannotUnderstandIsRefused(t *testing.T) {
	router, secret := credentialScopeFixture(t, "admin", true)

	// The baseline first, or this test proves nothing: the same call without
	// the strange field has to be one the service accepts. A refusal here means
	// the fixture is wrong, not that the service is strict — which is how a
	// test like this goes green while the thing it names is still true.
	good := `{"subject_name":"example-cookbook","subject_type":"git_repo",` +
		`"cookbook_name":"example-cookbook","verdict":"broken","reason":"it fails"}`
	if w := credentialRequest(t, router, secret, http.MethodPost,
		"/api/v1/failure-register", good); w.Code >= 400 {
		t.Fatalf("the body this test varies is not accepted to begin with (%d): %s",
			w.Code, w.Body.String())
	}

	// The same call, with one field nobody has ever heard of.
	strange := `{"subject_name":"example-cookbook","subject_type":"git_repo",` +
		`"cookbook_name":"example-cookbook","verdict":"broken","reason":"it fails",` +
		`"totally_unknown_field":"ignored"}`
	w := credentialRequest(t, router, secret, http.MethodPost,
		"/api/v1/failure-register", strange)

	if w.Code < 400 {
		t.Errorf("a body carrying a field the service does not understand was accepted (%d), "+
			"so a caller who misspells one is told it worked, and afterwards neither side "+
			"can say what was acted on", w.Code)
	}
}

// "Failures that give me nothing."
//
// Red: a failure is a consistent shape, which is not the same as being free of
// internals. This asserts the absence of the obvious ones from a refusal.
func TestJourney_AFailureGivesNothingAway(t *testing.T) {
	w := httptest.NewRecorder()
	agentJourneyRouter().ServeHTTP(w, withAdminSession(httptest.NewRequest(
		http.MethodGet, "/api/v1/nodes/example-org/example-node", nil)))

	body := w.Body.String()
	for _, leak := range []string{"/Users/", "/var/lib", "SELECT ", "goroutine ", ".go:"} {
		if strings.Contains(body, leak) {
			t.Errorf("a failure carries %q, which tells a tester what the service is made of "+
				"rather than that it failed", leak)
		}
	}
}

// "An account I can hold down to a level."
func TestJourney_ALowAccountStaysLow(t *testing.T) {
	router, secret := credentialScopeFixture(t, "viewer", false)

	if w := credentialRequest(t, router, secret, http.MethodGet,
		"/api/v1/admin/users", ""); w.Code != http.StatusForbidden {
		t.Errorf("a viewer's credential reached an administrator's address (%d), so there is "+
			"no account a tester can hold down to test with", w.Code)
	}
}

// "What the description says a call accepts is what the service reads."
//
// Red: several calls are described by reflecting an internal type whole, which
// advertises fields the service never reads — a false lead for anybody testing
// where an input reaches.
func TestJourney_NothingIsAdvertisedThatIsNeverRead(t *testing.T) {
	t.Skip("nothing compares a described body against the fields the handler really reads. " +
		"Custom cop definitions are described as accepting an id and timestamps because the " +
		"whole stored type is reflected; the store binds its columns explicitly and ignores " +
		"them. Closing this needs the write path measured the way the read path is, which " +
		"means sending bodies to a running instance — the probe deliberately only reads")
}

// "Nothing proves any of this survives a fuzzer, because one has never been
// run."
func TestJourney_ItHasBeenFuzzedAtAll(t *testing.T) {
	t.Skip("no fuzzing has been run against this API: no malformed input at volume, no long " +
		"strings, no deep nesting, no concurrency. Everything has been exercised by hand by " +
		"somebody sending what the service expected. This is a statement of fact, and it " +
		"stays red until a run has happened and its findings are somewhere")
}
