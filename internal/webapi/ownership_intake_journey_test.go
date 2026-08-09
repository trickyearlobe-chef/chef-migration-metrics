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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// The journey suite for journeys/ownership-intake.md. Run it with `make journey`.
//
// One test per thing the journey says has to be in place. A green one is built;
// a red one is not yet. That makes this the todo list for the journey, and a
// todo list made of tests cannot go stale: nobody has to remember to update it,
// because running it recomputes it.
//
// It is deliberately OUTSIDE the gating suite. Most of a journey is unbuilt for
// most of its life, so a red here is the normal state and must never block a
// build — a red that stops a release gets deleted, and then the list is gone.
//
// Two rules:
//
//   - Assert the real thing, so building the feature turns the test green with
//     no edit. A test that says "not implemented" has to be rewritten by the
//     person it was meant to help.
//   - Name the journey line it comes from, in the journey's words, so the
//     reason outlives whoever wrote it.
//
// This is not where regressions go. Something that used to work and now fails
// is a broken build, not a todo — parking it here hides it among the honest
// gaps, which are indistinguishable from it once they are in the same list.

func journeyRouter() *Router {
	return newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0"))
}

// reaches reports whether an endpoint exists at all. Any answer other than "no
// such route" counts: the mock store cannot satisfy a real request, so the
// question here is whether the capability is wired, not whether it works.
func reaches(t *testing.T, method, path string) bool {
	t.Helper()
	w := httptest.NewRecorder()
	r := journeyRouter()
	r.ServeHTTP(w, withAdminSession(httptest.NewRequest(method, path, nil)))
	return w.Code != http.StatusNotFound
}

// "To load it from a file, or straight from the database that holds it."
func TestJourney_CanLoadFromAFileOrADatabase(t *testing.T) {
	if !reaches(t, http.MethodPost, "/api/v1/ownership/import/preview") {
		t.Error("there is no way to load ownership from a source")
	}
}

// "The connection has to name its database, and I would rather it did."
func TestJourney_ConnectionMustNameItsDatabase(t *testing.T) {
	stored := secrets.ValidateCredentialValue(
		secrets.CredentialTypeDatabaseURL, []byte("postgres://user:pass@host:5432"))
	if stored.Valid {
		t.Error("a connection naming no database can be stored, and fails later " +
			"in front of somebody who cannot fix it")
	}
}

// "Then the tables in it, and the views as well."
func TestJourney_CanSeeTheTablesAndViews(t *testing.T) {
	if !reaches(t, http.MethodPost, "/api/v1/ownership/import/tables") {
		t.Error("there is no way to see what a connection holds")
	}
}

// "Then the fields, with sample data in them. Names lie."
func TestJourney_CanSeeTheFieldsWithSampleDataInThem(t *testing.T) {
	if !reaches(t, http.MethodPost, "/api/v1/ownership/import/profile") {
		t.Error("there is no way to see what is actually in a field")
	}
}

// "Something guessing the field names for me, which already helps — as long as
// I can see what it chose and change it."
func TestJourney_FieldNamesAreGuessedForMe(t *testing.T) {
	t.Skip("TODO: locate this. The owner reports it exists and helps; searching " +
		"for guess/suggest/infer/automap across the intake handler and the import " +
		"screen did not find it. Until it is found it can be neither pinned nor " +
		"honestly called missing.")
}

// "To see what it will do before it does it."
func TestJourney_ShowsWhatItWillDoBeforeItDoesIt(t *testing.T) {
	if !reaches(t, http.MethodPost, "/api/v1/ownership/import/preview") {
		t.Error("there is no preview, so an import can only be judged by running it")
	}
}

// "To try the row filter and watch it work before I commit anything."
//
// The filter is applied when a saved import runs unattended, which is pinned in
// the ordinary suite. What is not established is whether it can be TRIED from a
// database source before committing — the checking step the journey asks for.
func TestJourney_CanTryTheRowFilterBeforeCommitting(t *testing.T) {
	t.Skip("TODO: needs a database to answer honestly. The preview path applies a " +
		"row filter and reports how many rows it removed; whether that is reachable " +
		"when the source is a database rather than a file is asserted by nothing.")
}

// "To have the rows it could not use handed back to me as a worklist."
func TestJourney_HandsBackTheRowsItCouldNotUse(t *testing.T) {
	var summary ImportRunSummary
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshalling a run summary: %v", err)
	}
	if !strings.Contains(string(encoded), "filtered_out") {
		t.Error("a run does not account for the rows it did not use")
	}
}

// "That decision turns on having watched it run once, including how long it
// took — a job that takes forty minutes is a different proposition from one
// that takes four."
func TestJourney_ARunSaysHowLongItTook(t *testing.T) {
	encoded, err := json.Marshal(ImportRunSummary{RowCount: 10})
	if err != nil {
		t.Fatalf("marshalling a run summary: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decoding a run summary: %v", err)
	}
	for key := range fields {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "duration") || strings.Contains(lower, "elapsed") {
			return
		}
	}
	t.Error("a finished run does not say how long it took, which is the fact the " +
		"decision to schedule it turns on")
}

// "To be able to run it again on a schedule once I trust it."
func TestJourney_CanRunItAgainOnASchedule(t *testing.T) {
	if !reaches(t, http.MethodGet, "/api/v1/ownership/import/mappings") {
		t.Error("an import cannot be saved, so it cannot be repeated")
	}
}

// "Importing is an administrator's act, not an operator's."
func TestJourney_ImportingIsAnAdministratorsAct(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ownership/import/commit", nil)
	journeyRouter().ServeHTTP(w, withOperatorSession(req))
	if w.Code != http.StatusForbidden {
		t.Errorf("an operator can commit an import (answered %d)", w.Code)
	}
}

// "Nothing I do while looking around can change anything."
func TestJourney_LookingAroundCannotWrite(t *testing.T) {
	t.Skip("TODO: no way to assert this from outside. Listing, sampling and " +
		"filtering are described as ways to find your way around somebody else's " +
		"database, and that is only safe if none of them can write. Proving it " +
		"needs a source that records what it was asked to do.")
}
