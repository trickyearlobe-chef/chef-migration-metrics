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
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// The failure register — journeys 4 (record a failure nobody predicted) and
// 6 (the standup view). Behaviour: specifications/failure-register.md.
// ---------------------------------------------------------------------------

func registerRequest(method, path, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// Journey 4. An engineer records what broke, and it is answerable for
// afterwards.
func TestRecordFailure_RecordsTheVerdictAndAudits(t *testing.T) {
	var got datastore.RecordFailureVerdictParams
	var audited []string
	store := &mockStore{
		RecordFailureVerdictFn: func(_ context.Context, p datastore.RecordFailureVerdictParams) (datastore.FailureRegisterEntry, error) {
			got = p
			return datastore.FailureRegisterEntry{
				ID: "entry-1", SubjectName: p.SubjectName, CookbookName: p.CookbookName,
				Verdict: p.Verdict, Reason: p.Reason, Status: datastore.FailureStatusOpen,
			}, nil
		},
		InsertAuditEntryFn: func(_ context.Context, p datastore.InsertAuditEntryParams) error {
			audited = append(audited, p.Action)
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodPost, "/api/v1/failure-register", `{
		"subject_name": "acme-nginx",
		"cookbook_name": "nginx",
		"verdict": "broken",
		"reason": "the service resource fails on a real converge",
		"evidence": "NoMethodError: undefined method ...",
		"diagnosis": "removed DSL method"
	}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}
	if got.SubjectName != "acme-nginx" || got.CookbookName != "nginx" {
		t.Errorf("recorded against %q/%q", got.SubjectName, got.CookbookName)
	}
	if got.Verdict != "broken" || got.Reason == "" {
		t.Errorf("verdict = %q, reason = %q", got.Verdict, got.Reason)
	}
	if got.Evidence == "" || got.Diagnosis == "" {
		t.Error("the evidence or the diagnosis was dropped on the way to the store")
	}
	if got.RaisedBy == "" {
		t.Error("the entry does not say who recorded it")
	}
	if len(audited) != 1 || audited[0] != "failure_recorded" {
		t.Errorf("audit actions = %v, want one failure_recorded entry", audited)
	}
}

// A verdict with no reason is an opinion. The API refuses it before the store
// is troubled, so the caller gets a sentence rather than a constraint name.
func TestRecordFailure_RefusesAVerdictWithNoReason(t *testing.T) {
	called := false
	store := &mockStore{
		RecordFailureVerdictFn: func(_ context.Context, _ datastore.RecordFailureVerdictParams) (datastore.FailureRegisterEntry, error) {
			called = true
			return datastore.FailureRegisterEntry{}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodPost, "/api/v1/failure-register",
		`{"subject_name":"acme-nginx","cookbook_name":"nginx","verdict":"broken","reason":"   "}`))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if called {
		t.Error("a verdict with no reason reached the store")
	}
}

// Entries are never keyed on a version. A caller that sends one is told, not
// quietly obeyed — the vocabulary rule is the whole reason this is a repo-level
// list rather than a per-version one.
func TestRecordFailure_RefusesAVersion(t *testing.T) {
	store := &mockStore{}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodPost, "/api/v1/failure-register",
		`{"subject_name":"acme-nginx","cookbook_name":"nginx","cookbook_version":"1.2.3","verdict":"broken","reason":"broken"}`))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 — a verdict is about a repo, not a version", w.Code)
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "version") {
		t.Errorf("the refusal does not say why: %s", w.Body.String())
	}
}

// The subject and the label are both required: without the repo there is
// nothing to key on, and without the cookbook nobody recognises it at standup.
func TestRecordFailure_RequiresRepoAndCookbook(t *testing.T) {
	for _, body := range []string{
		`{"cookbook_name":"nginx","verdict":"broken","reason":"broken"}`,
		`{"subject_name":"acme-nginx","verdict":"broken","reason":"broken"}`,
		`{"subject_name":"acme-nginx","cookbook_name":"nginx","reason":"broken"}`,
	} {
		r := ownershipRouter(&mockStore{})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, registerRequest(http.MethodPost, "/api/v1/failure-register", body))
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s gave status %d, want 400", body, w.Code)
		}
	}
}

// Recording a verdict changes which nodes are ready, so readiness has to be
// recomputed. Without this the register is a list nobody's readiness reflects.
func TestRecordFailure_RecomputesReadiness(t *testing.T) {
	store := &mockStore{
		RecordFailureVerdictFn: func(_ context.Context, p datastore.RecordFailureVerdictParams) (datastore.FailureRegisterEntry, error) {
			return datastore.FailureRegisterEntry{ID: "entry-1", SubjectName: p.SubjectName}, nil
		},
	}
	recomputed := false
	r := ownershipRouter(store)
	WithReadinessReconciler(func() error { recomputed = true; return nil })(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodPost, "/api/v1/failure-register",
		`{"subject_name":"acme-nginx","cookbook_name":"nginx","verdict":"broken","reason":"breaks on converge"}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if !recomputed {
		t.Error("recording a verdict did not trigger a readiness recompute")
	}
}

// A readiness recompute that fails must not lose the entry. The verdict is
// recorded; the rollup catches up on the next collection either way.
func TestRecordFailure_SurvivesAFailedRecompute(t *testing.T) {
	store := &mockStore{
		RecordFailureVerdictFn: func(_ context.Context, p datastore.RecordFailureVerdictParams) (datastore.FailureRegisterEntry, error) {
			return datastore.FailureRegisterEntry{ID: "entry-1", SubjectName: p.SubjectName}, nil
		},
	}
	r := ownershipRouter(store)
	WithReadinessReconciler(func() error { return context.DeadlineExceeded })(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodPost, "/api/v1/failure-register",
		`{"subject_name":"acme-nginx","cookbook_name":"nginx","verdict":"broken","reason":"breaks on converge"}`))

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201 — the verdict was recorded", w.Code)
	}
}

// Journey 6, the standup view. Read together every morning it must answer
// which repos are broken, why, what is being done, and whether the list is
// growing — all in one read, not one request per entry.
func TestListFailureRegister_TheStandupView(t *testing.T) {
	var gotFilter datastore.FailureRegisterFilter
	store := &mockStore{
		ListFailureRegisterEntriesFn: func(_ context.Context, f datastore.FailureRegisterFilter) ([]datastore.FailureRegisterEntry, int, error) {
			gotFilter = f
			return []datastore.FailureRegisterEntry{{
				ID: "entry-1", SubjectName: "acme-nginx", CookbookName: "nginx",
				Verdict: "broken", Reason: "fails on a real converge",
				Plan: "rewrite the template", TargetDate: "2026-09-30",
				HolderType: "ticket", HolderRef: "PLAT-4821",
				Status: datastore.FailureStatusOpen,
			}}, 1, nil
		},
		FailureRegisterSummaryFn: func(_ context.Context, windowDays int) (datastore.FailureRegisterSummary, error) {
			return datastore.FailureRegisterSummary{
				Open: 9, OpenBroken: 7, OpenNotBroken: 2, OpenWithoutHolder: 3, OpenOverdue: 1,
				WindowDays: windowDays, RaisedInWindow: 4, ResolvedInWindow: 1,
				TotalBroken: 11, TotalNotBroken: 3, Resolved: 5,
			}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodGet, "/api/v1/failure-register", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []struct {
			GitRepoName  string `json:"subject_name"`
			CookbookName string `json:"cookbook_name"`
			Reason       string `json:"reason"`
			Plan         string `json:"plan"`
			HolderRef    string `json:"holder_ref"`
		} `json:"data"`
		Summary datastore.FailureRegisterSummary `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("got %d entries, want 1", len(resp.Data))
	}
	e := resp.Data[0]
	// Why it is broken, and what is being done, at a glance rather than a
	// click away.
	if e.Reason == "" || e.Plan == "" || e.HolderRef == "" || e.CookbookName == "" {
		t.Errorf("the standup list is missing something it has to answer: %+v", e)
	}

	// Whether the list is getting too large — the size and the direction.
	if resp.Summary.Open != 9 {
		t.Errorf("summary open = %d, want 9", resp.Summary.Open)
	}
	if resp.Summary.RaisedInWindow != 4 || resp.Summary.ResolvedInWindow != 1 {
		t.Errorf("direction of travel is missing: %+v", resp.Summary)
	}

	// The default read is what is standing, not the whole history.
	if gotFilter.Status != datastore.FailureStatusOpen {
		t.Errorf("default status filter = %q, want %q", gotFilter.Status, datastore.FailureStatusOpen)
	}
}

// The summary failing must not take the list with it: a standup that can see
// what is broken but not the trend is still worth having.
func TestListFailureRegister_SummaryFailureKeepsTheList(t *testing.T) {
	store := &mockStore{
		ListFailureRegisterEntriesFn: func(_ context.Context, _ datastore.FailureRegisterFilter) ([]datastore.FailureRegisterEntry, int, error) {
			return []datastore.FailureRegisterEntry{{ID: "entry-1", SubjectName: "acme-nginx"}}, 1, nil
		},
		FailureRegisterSummaryFn: func(_ context.Context, _ int) (datastore.FailureRegisterSummary, error) {
			return datastore.FailureRegisterSummary{}, context.DeadlineExceeded
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodGet, "/api/v1/failure-register", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["data"]; !ok {
		t.Error("the list was dropped because the summary failed")
	}
}

// The register is readable as an accuracy report of the automated signals, so
// filtering by which way the verdict went has to reach the store.
func TestListFailureRegister_FiltersByVerdict(t *testing.T) {
	var gotFilter datastore.FailureRegisterFilter
	store := &mockStore{
		ListFailureRegisterEntriesFn: func(_ context.Context, f datastore.FailureRegisterFilter) ([]datastore.FailureRegisterEntry, int, error) {
			gotFilter = f
			return nil, 0, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodGet,
		"/api/v1/failure-register?verdict=not_broken&status=resolved", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if gotFilter.Verdict != "not_broken" || gotFilter.Status != "resolved" {
		t.Errorf("filter = %+v, want verdict not_broken and status resolved", gotFilter)
	}
}

// An empty register reports an empty list rather than a null, so the standup
// view can tell "nothing is broken" from "this failed to load".
func TestListFailureRegister_EmptyIsAList(t *testing.T) {
	store := &mockStore{
		ListFailureRegisterEntriesFn: func(_ context.Context, _ datastore.FailureRegisterFilter) ([]datastore.FailureRegisterEntry, int, error) {
			return nil, 0, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodGet, "/api/v1/failure-register", ""))

	if !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Errorf("an empty register serialised as %s, want an empty array", w.Body.String())
	}
}

// Resolution is recorded rather than deleted, and it says who did it.
func TestResolveFailure_RecordsWhoAndAudits(t *testing.T) {
	var gotID, gotBy, gotNote string
	var audited []string
	store := &mockStore{
		ResolveFailureEntryFn: func(_ context.Context, id, by, note string) (datastore.FailureRegisterEntry, error) {
			gotID, gotBy, gotNote = id, by, note
			now := time.Now()
			return datastore.FailureRegisterEntry{
				ID: id, Status: datastore.FailureStatusResolved, ResolvedBy: by, ResolvedAt: &now,
			}, nil
		},
		InsertAuditEntryFn: func(_ context.Context, p datastore.InsertAuditEntryParams) error {
			audited = append(audited, p.Action)
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodPost,
		"/api/v1/failure-register/entry-1/resolve", `{"note":"fixed in 4.2.0"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if gotID != "entry-1" {
		t.Errorf("resolved %q, want entry-1", gotID)
	}
	if gotBy == "" {
		t.Error("the resolution does not say who did it")
	}
	if gotNote != "fixed in 4.2.0" {
		t.Errorf("note = %q", gotNote)
	}
	if len(audited) != 1 || audited[0] != "failure_resolved" {
		t.Errorf("audit actions = %v, want one failure_resolved entry", audited)
	}
}

// Resolving something that is not there says so.
func TestResolveFailure_MissingEntry(t *testing.T) {
	store := &mockStore{
		ResolveFailureEntryFn: func(_ context.Context, _, _, _ string) (datastore.FailureRegisterEntry, error) {
			return datastore.FailureRegisterEntry{}, datastore.ErrNotFound
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodPost, "/api/v1/failure-register/nope/resolve", `{}`))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// The diagnosis, the plan, the holder and the target date arrive after the
// entry is raised — a failure is worth recording before anybody knows what to
// do about it.
func TestReviseFailure_UpdatesThePlan(t *testing.T) {
	var got datastore.ReviseFailureEntryParams
	var audited []string
	store := &mockStore{
		ReviseFailureEntryFn: func(_ context.Context, id string, p datastore.ReviseFailureEntryParams) (datastore.FailureRegisterEntry, error) {
			got = p
			return datastore.FailureRegisterEntry{ID: id, Status: datastore.FailureStatusOpen}, nil
		},
		InsertAuditEntryFn: func(_ context.Context, p datastore.InsertAuditEntryParams) error {
			audited = append(audited, p.Action)
			return nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodPatch, "/api/v1/failure-register/entry-1", `{
		"plan": "rewrite the template and re-release",
		"holder_type": "ticket",
		"holder_ref": "PLAT-4821",
		"target_date": "2026-09-30"
	}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if got.Plan == nil || *got.Plan != "rewrite the template and re-release" {
		t.Errorf("plan = %v", got.Plan)
	}
	if got.HolderRef == nil || *got.HolderRef != "PLAT-4821" {
		t.Errorf("holder ref = %v", got.HolderRef)
	}
	if got.TargetDate == nil || got.TargetDate.Format("2006-01-02") != "2026-09-30" {
		t.Errorf("target date = %v", got.TargetDate)
	}
	// A field nobody sent stays nil, so the store leaves it alone rather than
	// blanking what somebody else wrote.
	if got.Diagnosis != nil {
		t.Error("an unsent field was sent to the store as a value")
	}
	if len(audited) != 1 || audited[0] != "failure_revised" {
		t.Errorf("audit actions = %v, want one failure_revised entry", audited)
	}
}

// A revision must not be a way to change the verdict or the reason: a reversal
// is a new verdict, recorded separately, so the old one stays readable.
func TestReviseFailure_CannotChangeTheVerdictOrReason(t *testing.T) {
	var got datastore.ReviseFailureEntryParams
	store := &mockStore{
		ReviseFailureEntryFn: func(_ context.Context, id string, p datastore.ReviseFailureEntryParams) (datastore.FailureRegisterEntry, error) {
			got = p
			return datastore.FailureRegisterEntry{ID: id}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodPatch, "/api/v1/failure-register/entry-1",
		`{"verdict":"not_broken","reason":"changed my mind","plan":"none"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — a reversal is a new verdict: %s", w.Code, w.Body.String())
	}
	if got.Plan != nil {
		t.Error("the revision was partly applied before being refused")
	}
}

// The whole history for a repo: a scan called it incompatible, a person
// overruled it, and why.
func TestFailureRegisterHistory_ReturnsEveryVerdict(t *testing.T) {
	var gotSubject string
	store := &mockStore{
		ListFailureRegisterHistoryFn: func(_ context.Context, repo string) ([]datastore.FailureRegisterEntry, error) {
			gotSubject = repo
			return []datastore.FailureRegisterEntry{
				{ID: "entry-2", Verdict: "not_broken", Status: datastore.FailureStatusOpen},
				{ID: "entry-1", Verdict: "broken", Status: datastore.FailureStatusSuperseded, SupersededBy: "entry-2"},
			}, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodGet, "/api/v1/failure-register/subject/acme-nginx", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if gotSubject != "acme-nginx" {
		t.Errorf("history read for %q, want acme-nginx", gotSubject)
	}
	var resp struct {
		Data []datastore.FailureRegisterEntry `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("got %d entries, want both the standing verdict and the one it superseded", len(resp.Data))
	}
	if resp.Data[1].SupersededBy != "entry-2" {
		t.Error("the superseded verdict does not point at the one that replaced it")
	}
}

// A repo name with a slash in it (some hosting platforms allow one) must reach
// the store whole rather than being truncated at the first separator.
func TestFailureRegisterHistory_RepoNameWithASlash(t *testing.T) {
	var gotSubject string
	store := &mockStore{
		ListFailureRegisterHistoryFn: func(_ context.Context, repo string) ([]datastore.FailureRegisterEntry, error) {
			gotSubject = repo
			return nil, nil
		},
	}
	r := ownershipRouter(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodGet, "/api/v1/failure-register/subject/group/acme-nginx", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	if gotSubject != "group/acme-nginx" {
		t.Errorf("history read for %q, want the whole name group/acme-nginx", gotSubject)
	}
}

// Reading the register is a viewer action; recording, revising and resolving
// change what work gets dispatched and need operator or admin.
func TestFailureRegister_MethodNotAllowed(t *testing.T) {
	r := ownershipRouter(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, registerRequest(http.MethodDelete, "/api/v1/failure-register/entry-1", ""))

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405 — the register never deletes", w.Code)
	}
}
