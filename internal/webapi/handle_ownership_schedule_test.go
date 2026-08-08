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
)

// A saved import carries what it needs to run with nobody watching: the
// connection, the query, the row filter and when to run. These tests cover the
// save, because a schedule that is accepted and cannot run is worse than one
// that is refused — nothing fires and the screen says it should.

func scheduledMappingBody(t *testing.T, overrides map[string]any) string {
	t.Helper()
	body := map[string]any{
		"name":             "cmdb-nightly",
		"source_kind":      "database",
		"field_map":        json.RawMessage(repoFieldMap(t)),
		"db_driver":        "postgres",
		"db_credential":    "cmdb",
		"db_query":         "SELECT owner, repo FROM asset_owner",
		"schedule":         "0 2 * * *",
		"schedule_enabled": true,
	}
	for k, v := range overrides {
		if v == nil {
			delete(body, k)
			continue
		}
		body[k] = v
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshalling the request body: %v", err)
	}
	return string(encoded)
}

func postMapping(t *testing.T, r *Router, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, jsonRequest(http.MethodPost, "/api/v1/ownership/import/mappings", body))
	return w
}

func TestSaveScheduledImport_StoresTheConnectionAndTheSchedule(t *testing.T) {
	var got datastore.InsertImportMappingParams
	store := &mockStore{
		InsertImportMappingFn: func(_ context.Context, p datastore.InsertImportMappingParams) (datastore.ImportMapping, error) {
			got = p
			return datastore.ImportMapping{ID: 1, Name: p.Name}, nil
		},
	}
	w := postMapping(t, ownershipRouter(store), scheduledMappingBody(t, nil))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	if got.DBCredential != "cmdb" {
		t.Errorf("DBCredential = %q, want the stored credential name", got.DBCredential)
	}
	if !strings.Contains(got.DBQuery, "asset_owner") {
		t.Errorf("DBQuery = %q, want the query as written", got.DBQuery)
	}
	if got.Schedule != "0 2 * * *" || !got.ScheduleEnabled {
		t.Errorf("schedule = %q enabled = %v, want the cron as written and enabled", got.Schedule, got.ScheduleEnabled)
	}
}

// A cron expression the scheduler cannot parse must be refused at the point
// somebody types it, not stored and discovered at 02:00 by nobody.
func TestSaveScheduledImport_RefusesAnUnparseableExpression(t *testing.T) {
	for _, expr := range []string{"not a cron", "0 2 * *", "99 * * * *", "@daily"} {
		w := postMapping(t, ownershipRouter(&mockStore{}), scheduledMappingBody(t, map[string]any{"schedule": expr}))
		if w.Code != http.StatusBadRequest {
			t.Errorf("schedule %q: status = %d, want 400: %s", expr, w.Code, w.Body.String())
		}
	}
}

// A file import has no stored source — somebody has to bring the file — so
// there is nothing for a schedule to read.
func TestSaveScheduledImport_RefusesToScheduleAFileImport(t *testing.T) {
	w := postMapping(t, ownershipRouter(&mockStore{}), scheduledMappingBody(t, map[string]any{
		"source_kind":   "csv",
		"db_credential": "",
		"db_query":      "",
	}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "database") {
		t.Errorf("the refusal does not say a schedule needs a database source: %s", w.Body.String())
	}
}

// Enabling a schedule with nothing to connect to is the same fault as the one
// the database constraint catches, and it is better answered here where the
// person can see which field is missing.
func TestSaveScheduledImport_RefusesAScheduleWithNothingToRun(t *testing.T) {
	for field, missing := range map[string]map[string]any{
		"db_credential": {"db_credential": ""},
		"db_query":      {"db_query": "   "},
		"schedule":      {"schedule": ""},
	} {
		w := postMapping(t, ownershipRouter(&mockStore{}), scheduledMappingBody(t, missing))
		if w.Code != http.StatusBadRequest {
			t.Errorf("missing %s: status = %d, want 400: %s", field, w.Code, w.Body.String())
		}
	}
}

// Saving a definition without switching the schedule on has to stay possible:
// that is how somebody sets an import up before deciding to automate it.
func TestSaveScheduledImport_AllowsAnUnscheduledDatabaseImport(t *testing.T) {
	store := &mockStore{
		InsertImportMappingFn: func(_ context.Context, p datastore.InsertImportMappingParams) (datastore.ImportMapping, error) {
			return datastore.ImportMapping{ID: 1, Name: p.Name}, nil
		},
	}
	w := postMapping(t, ownershipRouter(store), scheduledMappingBody(t, map[string]any{
		"schedule":         "",
		"schedule_enabled": false,
	}))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
}

// The list has to carry the schedule and the last run, or checking whether the
// nightly import is working means opening every saved import in turn.
func TestListImports_ReportsTheScheduleAndTheLastRun(t *testing.T) {
	store := &mockStore{
		ListImportMappingsFn: func(context.Context, int, int) ([]datastore.ImportMapping, int, error) {
			return []datastore.ImportMapping{{
				ID: 1, Name: "cmdb-nightly", SourceKind: "database",
				Schedule: "0 2 * * *", ScheduleEnabled: true,
				LastRunStatus: "failed", LastRunDetail: "could not read the credential",
			}}, 1, nil
		},
	}
	w := httptest.NewRecorder()
	ownershipRouter(store).ServeHTTP(w, jsonRequest(http.MethodGet, "/api/v1/ownership/import/mappings", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	for _, want := range []string{"0 2 * * *", "failed", "could not read the credential"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("the list omits %q, so the screen cannot show it: %s", want, w.Body.String())
		}
	}
}

// A schedule the API accepts must be one the scheduler can parse. Asserted
// against the parser itself rather than a copy of its rules, because two
// independent notions of "valid cron" is exactly how an accepted expression
// ends up never firing.
func TestSaveScheduledImport_AcceptsWhatTheSchedulerCanParse(t *testing.T) {
	for _, expr := range []string{"0 2 * * *", "*/15 * * * *", "0 0 1 * *", "30 6 * * 1-5"} {
		store := &mockStore{
			InsertImportMappingFn: func(_ context.Context, p datastore.InsertImportMappingParams) (datastore.ImportMapping, error) {
				return datastore.ImportMapping{ID: 1, Name: p.Name}, nil
			},
		}
		w := postMapping(t, ownershipRouter(store), scheduledMappingBody(t, map[string]any{"schedule": expr}))
		if w.Code != http.StatusCreated {
			t.Errorf("schedule %q: status = %d, want 201: %s", expr, w.Code, w.Body.String())
		}
	}
}

// ---------------------------------------------------------------------------
// Running a saved import on demand, and throwing away what imports brought in
// ---------------------------------------------------------------------------

// Run now exists for judging a source: import it, look at what arrived, adjust
// the query, go again. So it answers with what the run did, not with "started".
func TestRunImportNow_ReportsWhatTheRunDid(t *testing.T) {
	store := &mockStore{
		GetImportMappingFn: func(context.Context, int64) (datastore.ImportMapping, error) {
			return datastore.ImportMapping{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, jsonRequest(http.MethodPost, "/api/v1/ownership/import/mappings/1/run", ""))

	// The mapping reads a file, so the run cannot happen — but the refusal has
	// to say why rather than 404 as though the endpoint were missing.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "file") {
		t.Errorf("the refusal does not explain that a file import cannot be re-run: %s", w.Body.String())
	}
}

func TestRunImportNow_IsNotFoundForAnUnknownImport(t *testing.T) {
	store := &mockStore{
		GetImportMappingFn: func(context.Context, int64) (datastore.ImportMapping, error) {
			return datastore.ImportMapping{}, datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, jsonRequest(http.MethodPost, "/api/v1/ownership/import/mappings/9/run", ""))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestRunImportNow_RefusesAGET(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, jsonRequest(http.MethodGet, "/api/v1/ownership/import/mappings/1/run", ""))

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405 — running an import is not something a link should do: %s",
			w.Code, w.Body.String())
	}
}

// The clear-down is destructive, so the confirmation has to be able to name a
// number. A GET reports what would go without removing anything.
func TestClearImportedOwnership_GETPreviewsWithoutDeleting(t *testing.T) {
	cleared := false
	store := &mockStore{
		CountImportedOwnershipFn: func(context.Context) (datastore.ClearedOwnership, error) {
			return datastore.ClearedOwnership{Assignments: 41, Owners: 7}, nil
		},
		ClearImportedOwnershipFn: func(context.Context) (datastore.ClearedOwnership, error) {
			cleared = true
			return datastore.ClearedOwnership{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, jsonRequest(http.MethodGet, "/api/v1/ownership/import/clear", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if cleared {
		t.Fatal("previewing the clear-down deleted the data it was asked to describe")
	}
	for _, want := range []string{"41", "7"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("the preview omits %q, so the confirmation cannot name a number: %s", want, w.Body.String())
		}
	}
}

func TestClearImportedOwnership_POSTRemovesAndReportsCounts(t *testing.T) {
	store := &mockStore{
		ClearImportedOwnershipFn: func(context.Context) (datastore.ClearedOwnership, error) {
			return datastore.ClearedOwnership{Assignments: 41, Owners: 7}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, jsonRequest(http.MethodPost, "/api/v1/ownership/import/clear", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	for _, want := range []string{"41", "7"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("the result omits %q, so nothing says what was removed: %s", want, w.Body.String())
		}
	}
}

// Removing ownership in bulk is exactly the kind of act somebody later asks
// "who did that?" about.
func TestClearImportedOwnership_IsAudited(t *testing.T) {
	var actions []string
	store := &mockStore{
		ClearImportedOwnershipFn: func(context.Context) (datastore.ClearedOwnership, error) {
			return datastore.ClearedOwnership{Assignments: 41, Owners: 7}, nil
		},
		InsertAuditEntryFn: func(_ context.Context, p datastore.InsertAuditEntryParams) error {
			actions = append(actions, p.Action)
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, jsonRequest(http.MethodPost, "/api/v1/ownership/import/clear", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if len(actions) == 0 {
		t.Fatal("a bulk removal of ownership left no audit entry")
	}
}

func TestClearImportedOwnership_RefusesAnOperator(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, withOperatorSession(jsonRequest(method, "/api/v1/ownership/import/clear", "")))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s status = %d, want 403", method, w.Code)
		}
	}
}
