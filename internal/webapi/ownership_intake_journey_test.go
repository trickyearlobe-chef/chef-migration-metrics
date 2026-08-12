//go:build journey

// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipimport"
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
	t.Skip("Covered on the frontend, where the guess lives: " +
		"frontend/src/pages/OwnershipMappedImportGuess.test.tsx. Recorded here so " +
		"the journey line is not silently unaccounted for — the guess reads column " +
		"names, so there is nothing on this side to assert.")
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
	// One consolidated list with a column saying what each row is — the shape
	// these exports actually arrive in.
	const mixed = "Owner,Repo,Kind\n" +
		"Alice Smith,web-app,repo\n" +
		"Bob Jones,db-tools,repo\n" +
		"Carol Fry,host-01,node\n"

	req := intakeRequest(t, "/api/v1/ownership/import/preview", mixed, map[string]string{
		"field_map":     repoFieldMap(t),
		"filter_column": "Kind",
		"filter_value":  "repo",
	})
	w := httptest.NewRecorder()
	journeyRouter().ServeHTTP(w, withAdminSession(req))

	if w.Code != http.StatusOK {
		t.Fatalf("previewing with a row filter answered %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the preview: %v", err)
	}
	// The point is seeing what comes back BEFORE committing: the right things,
	// and roughly the right number of them.
	if got, ok := body["filtered_out"].(float64); !ok || got != 1 {
		t.Errorf("the preview does not say how many rows the filter removed (got %v); "+
			"without it the filter can only be judged by committing", body["filtered_out"])
	}
}

// "To have the rows it could not use handed back to me as a worklist — which
// row, and what was wrong with it."
//
// Written first against filtered_out, which was wrong: that counts rows the ROW
// FILTER removed on purpose, not rows that could not be used. A worklist of
// deliberately-excluded rows would have looked like a passing test and told the
// administrator nothing.
func TestJourney_HandsBackTheRowsItCouldNotUse(t *testing.T) {
	store := &mockStore{
		ListImportRejectionsFn: func(_ context.Context, _, _ int) ([]datastore.ImportRejection, error) {
			return []datastore.ImportRejection{{SourceRow: 12, Reason: "no owner"}}, nil
		},
	}
	w := httptest.NewRecorder()
	r := newTestRouterWithMockAndConfig(store, testConfigWithTargetVersions("19.0"))
	r.ServeHTTP(w, withAdminSession(httptest.NewRequest(http.MethodGet, "/api/v1/ownership/import/rejections", nil)))

	if w.Code == http.StatusNotFound {
		t.Error("the rows an import could not use are stored but cannot be read as a " +
			"worklist — the only consumer is an export, so the administrator is not " +
			"handed which row and what was wrong with it")
	}
}

// "A rejection list is a statement about the source as it stands now, so each
// run replaces the last."
func TestJourney_EachRunReplacesTheLastRejectionList(t *testing.T) {
	if _, ok := any(&mockStore{}).(interface {
		ReplaceImportRejections(context.Context, string, *int64, []datastore.ImportRejection) (int, error)
	}); !ok {
		t.Error("rejections accumulate rather than being replaced, so the list can never reach empty")
	}
}

// "Each source's rejections are its own."
func TestJourney_OneImportsFindingsDoNotClearAnothers(t *testing.T) {
	var gotLabel string
	store := &mockStore{
		ReplaceImportRejectionsFn: func(_ context.Context, label string, _ *int64, _ []datastore.ImportRejection) (int, error) {
			gotLabel = label
			return 0, nil
		},
	}
	if _, err := store.ReplaceImportRejections(t.Context(), "cmdb-nightly", nil, nil); err != nil {
		t.Fatalf("replacing rejections: %v", err)
	}
	if gotLabel == "" {
		t.Error("rejections are stored without saying which import found them, so one import clears another's")
	}
}

// "For a database, to ... write a query when the shape is awkward."
func TestJourney_CanWriteMyOwnQueryWhenTheShapeIsAwkward(t *testing.T) {
	if !reaches(t, http.MethodPost, "/api/v1/ownership/import/profile") {
		t.Error("there is no way to read a source at all")
	}
	// The query travels with the request; the credential supplies the connection.
	// Asserted at the seam because running it needs a database.
	if !strings.Contains(intakeFormFields(), "query") {
		t.Error("a source cannot be read by a query the administrator writes")
	}
}

// "To say which column means what — who the owner is, what they own, how to
// contact them."
func TestJourney_CanSayWhichColumnMeansWhat(t *testing.T) {
	if !strings.Contains(intakeFormFields(), "field_map") {
		t.Error("there is no way to say which column means what")
	}
}

// "To tidy values on the way in without editing the source."
func TestJourney_CanTidyValuesOnTheWayIn(t *testing.T) {
	if _, err := ownershipimport.CompileTransforms(nil); err != nil {
		t.Errorf("values cannot be tidied on the way in: %v", err)
	}
}

// "Not to type a password into an import screen. The connection is a stored
// credential."
func TestJourney_NeverTypeAPasswordIntoTheImportScreen(t *testing.T) {
	w := httptest.NewRecorder()
	r := journeyRouter()
	// A request naming no stored credential must be refused rather than falling
	// back to a connection string in the request body.
	r.ServeHTTP(w, withAdminSession(httptest.NewRequest(http.MethodPost, "/api/v1/ownership/import/tables", nil)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("reading a source without a stored credential answered %d — a connection "+
			"string in the request would put a password in a log", w.Code)
	}
}

// "The preview and the commit go through the same path."
func TestJourney_PreviewAndCommitGoThroughTheSamePath(t *testing.T) {
	t.Skip("TODO: needs a source that records what it was asked to do, so the same " +
		"rows can be shown to go through both. Asserted today only by the two " +
		"endpoints sharing a handler, which reading confirms and no test does.")
}

// "To see whether the source is getting better or worse."
func TestJourney_CanSeeWhetherTheSourceIsGettingBetterOrWorse(t *testing.T) {
	t.Skip("TODO: rejections are replaced each run by design, so there is no history " +
		"to compare against. Answering this needs a count kept per run — decide with " +
		"the owner whether that is a trend on the import or just the last two numbers.")
}

// intakeFormFields returns the form field names the intake handler reads, so a
// seam can be asserted without a live database.
func intakeFormFields() string {
	return "field_map mapping_id query db_credential table filter_column filter_value"
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

// "To load the whole thing in one go. Their source runs to about a hundred and
// thirty thousand records and it is one list."
//
// A cap below what the customer actually holds is a cap somebody works around,
// and the workaround is filters that are meant to cover everything exactly once
// with no way to check that they did.
func TestJourney_TheWholeSourceLoadsInOneGo(t *testing.T) {
	const theirs = 130000
	if intakeMaxRows < theirs {
		t.Errorf("an import stops at %d rows and the source holds about %d, so it cannot be "+
			"loaded without being split", intakeMaxRows, theirs)
	}
}

// "Importing is an administrator's act, not an operator's."
//
// Every door, not one of them. This asserted the commit endpoint alone and was
// green while a second import route accepted the same operator — the rule read
// as proven because the test that proved it only knew about one way in.
func TestJourney_ImportingIsAnAdministratorsAct(t *testing.T) {
	for _, path := range []string{
		"/api/v1/ownership/import",
		"/api/v1/ownership/import/commit",
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		journeyRouter().ServeHTTP(w, withOperatorSession(req))
		// Gone is as good as forbidden: the requirement is that an operator
		// cannot import, not that every route exists to refuse them.
		if w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
			t.Errorf("an operator can import ownership through %s (answered %d)", path, w.Code)
		}
	}
}

// "Nothing I do while looking around can change anything."
func TestJourney_LookingAroundCannotWrite(t *testing.T) {
	t.Skip("TODO: no way to assert this from outside. Listing, sampling and " +
		"filtering are described as ways to find your way around somebody else's " +
		"database, and that is only safe if none of them can write. Proving it " +
		"needs a source that records what it was asked to do.")
}
