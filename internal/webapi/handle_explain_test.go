// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

func TestHandleExplainCatalog_Get(t *testing.T) {
	store := &mockStore{
		ExplainCatalogFn: func() []datastore.ExplainCatalogEntry {
			return []datastore.ExplainCatalogEntry{
				{Key: "roles_list", Label: "Roles list", Description: "…", Analyzable: true},
				{Key: "node_list_heavy", Label: "Node list (heavy JSON)", Description: "…", Analyzable: true},
			}
		},
	}
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance/explain/catalog", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var body struct {
		Entries []datastore.ExplainCatalogEntry `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(body.Entries) != 2 || body.Entries[0].Key != "roles_list" {
		t.Errorf("expected 2 entries starting with roles_list, got %+v", body.Entries)
	}
}

func TestHandleExplainCatalog_MethodNotAllowed(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/performance/explain/catalog", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func postExplain(t *testing.T, store *mockStore, jsonBody string) *httptest.ResponseRecorder {
	t.Helper()
	r := newTestRouterWithMock(store)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/performance/explain", strings.NewReader(jsonBody))
	r.ServeHTTP(w, req)
	return w
}

func TestHandleExplain_CatalogKey(t *testing.T) {
	var resolvedKey string
	store := &mockStore{
		ResolveCatalogExplainFn: func(ctx context.Context, key string, p datastore.CatalogParams) (string, []interface{}, string, string, error) {
			resolvedKey = key
			return "WITH rolled AS (…) SELECT …", []interface{}{}, "Roles list", "orgs=3, limit=50", nil
		},
		RunExplainFn: func(ctx context.Context, sqlText string, args []interface{}, opts datastore.ExplainOptions) (datastore.ExplainResult, error) {
			return datastore.ExplainResult{Run1: datastore.ExplainRun{PlanText: "Seq Scan on role_summary"}}, nil
		},
	}
	w := postExplain(t, store, `{"catalog_key":"roles_list","analyze":true}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if resolvedKey != "roles_list" {
		t.Errorf("ResolveCatalogExplain called with key %q, want roles_list", resolvedKey)
	}
	var body explainResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Label != "Roles list" || body.ParamSummary != "orgs=3, limit=50" {
		t.Errorf("unexpected label/summary: %+v", body)
	}
	if body.SQL != "" {
		t.Errorf("catalog SQL must not be echoed, got %q", body.SQL)
	}
	if !strings.Contains(body.Run1.PlanText, "Seq Scan") {
		t.Errorf("expected run1 plan text, got %q", body.Run1.PlanText)
	}
	if body.StatementTimeoutMs != explainStatementTimeoutMs {
		t.Errorf("statement_timeout_ms = %d, want %d", body.StatementTimeoutMs, explainStatementTimeoutMs)
	}
}

func TestHandleExplain_FreeTextSQL(t *testing.T) {
	var ranSQL string
	store := &mockStore{
		RunExplainFn: func(ctx context.Context, sqlText string, args []interface{}, opts datastore.ExplainOptions) (datastore.ExplainResult, error) {
			ranSQL = sqlText
			return datastore.ExplainResult{Run1: datastore.ExplainRun{PlanText: "Result"}}, nil
		},
	}
	w := postExplain(t, store, `{"sql":"SELECT 1 ;"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if ranSQL != "SELECT 1" {
		t.Errorf("RunExplain got sql %q, want cleaned \"SELECT 1\"", ranSQL)
	}
	var body explainResponse
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.SQL != "SELECT 1" {
		t.Errorf("free-text SQL should be echoed, got %q", body.SQL)
	}
	if body.Label != "Custom query" {
		t.Errorf("label = %q, want Custom query", body.Label)
	}
}

func TestHandleExplain_FreeTextRejected(t *testing.T) {
	called := false
	store := &mockStore{
		RunExplainFn: func(ctx context.Context, sqlText string, args []interface{}, opts datastore.ExplainOptions) (datastore.ExplainResult, error) {
			called = true
			return datastore.ExplainResult{}, nil
		},
	}
	// COPY is a utility statement — not explainable — so it is rejected pre-flight.
	w := postExplain(t, store, `{"sql":"COPY node_snapshots TO stdout"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if called {
		t.Error("RunExplain must not be called for a rejected utility statement")
	}
}

func TestHandleExplain_WriteAnalyzeFallsBackToPlanOnly(t *testing.T) {
	var optsSeen []datastore.ExplainOptions
	store := &mockStore{
		RunExplainFn: func(ctx context.Context, sqlText string, args []interface{}, opts datastore.ExplainOptions) (datastore.ExplainResult, error) {
			optsSeen = append(optsSeen, opts)
			if opts.Analyze {
				// Simulate the READ ONLY transaction rejecting a write under ANALYZE.
				return datastore.ExplainResult{}, errors.New("pq: cannot execute UPDATE in a read-only transaction")
			}
			return datastore.ExplainResult{Run1: datastore.ExplainRun{PlanText: "Update on kitchen_run_queue"}}, nil
		},
	}
	w := postExplain(t, store, `{"sql":"UPDATE kitchen_run_queue SET status = 1","analyze":true}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if len(optsSeen) != 2 || !optsSeen[0].Analyze || optsSeen[1].Analyze {
		t.Errorf("expected an ANALYZE attempt then a plan-only retry, got %+v", optsSeen)
	}
	var body explainResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Analyze {
		t.Error("response analyze flag should be false after fallback")
	}
	if body.Note == "" {
		t.Error("expected a note explaining the ANALYZE→plan-only fallback")
	}
	if !strings.Contains(body.Run1.PlanText, "Update") {
		t.Errorf("expected the plan-only Update plan, got %q", body.Run1.PlanText)
	}
}

func TestHandleExplain_NeitherOrBoth(t *testing.T) {
	for _, body := range []string{`{}`, `{"catalog_key":"roles_list","sql":"SELECT 1"}`} {
		w := postExplain(t, &mockStore{}, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
	}
}

func TestHandleExplain_Unavailable(t *testing.T) {
	store := &mockStore{
		ResolveCatalogExplainFn: func(ctx context.Context, key string, p datastore.CatalogParams) (string, []interface{}, string, string, error) {
			return "", nil, "", "", datastore.ErrExplainUnavailable
		},
	}
	w := postExplain(t, store, `{"catalog_key":"cookbook_coverage_containment"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for unavailable entry", w.Code)
	}
}

func TestHandleExplain_RunTwice(t *testing.T) {
	var gotOpts datastore.ExplainOptions
	store := &mockStore{
		RunExplainFn: func(ctx context.Context, sqlText string, args []interface{}, opts datastore.ExplainOptions) (datastore.ExplainResult, error) {
			gotOpts = opts
			run2 := datastore.ExplainRun{PlanText: "warm"}
			return datastore.ExplainResult{Run1: datastore.ExplainRun{PlanText: "cold"}, Run2: &run2}, nil
		},
	}
	w := postExplain(t, store, `{"sql":"SELECT 1","analyze":true,"run_twice":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !gotOpts.RunTwice || !gotOpts.Analyze {
		t.Errorf("options not passed through: %+v", gotOpts)
	}
	var body explainResponse
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Run2 == nil {
		t.Error("expected run2 in response")
	}
}

func TestHandleExplain_MethodNotAllowed(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance/explain", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}
