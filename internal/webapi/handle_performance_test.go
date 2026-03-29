// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/perf"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func perfEnabledConfig() *testPerfConfig {
	return &testPerfConfig{enabled: true, windowSeconds: 300}
}

type testPerfConfig struct {
	enabled       bool
	windowSeconds int
}

func newPerfRouter(store *mockStore, pc *testPerfConfig) *Router {
	cfg := testConfig()
	cfg.Performance.Enabled = pc.enabled
	cfg.Performance.WindowSeconds = pc.windowSeconds

	rec := perf.NewRecorder(time.Duration(pc.windowSeconds)*time.Second, 200, 1000)
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(store, cfg, hub, WithPerformance(rec))
}

func newPerfRouterDisabled(store *mockStore) *Router {
	cfg := testConfig()
	cfg.Performance.Enabled = false

	hub := NewEventHub()
	go hub.Run()
	return NewRouter(store, cfg, hub)
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/performance — request stats
// ---------------------------------------------------------------------------

func TestHandlePerformance_GET_HappyPath(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouter(store, perfEnabledConfig())

	// Record some samples so the response is non-empty.
	r.recorder.Record("GET /api/v1/nodes", 50*time.Millisecond)
	r.recorder.Record("GET /api/v1/nodes", 100*time.Millisecond)
	r.recorder.Record("POST /api/v1/exports", 200*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		WindowSeconds int `json:"window_seconds"`
		Endpoints     []struct {
			Method     string  `json:"method"`
			Path       string  `json:"path"`
			Count      int     `json:"count"`
			ErrorCount int     `json:"error_count"`
			P50Ms      float64 `json:"p50_ms"`
			P95Ms      float64 `json:"p95_ms"`
			P99Ms      float64 `json:"p99_ms"`
			MaxMs      float64 `json:"max_ms"`
		} `json:"endpoints"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body.WindowSeconds != 300 {
		t.Errorf("window_seconds = %d, want 300", body.WindowSeconds)
	}
	if len(body.Endpoints) < 2 {
		t.Fatalf("endpoints count = %d, want >= 2", len(body.Endpoints))
	}
}

func TestHandlePerformance_GET_EmptyStats(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Endpoints []any `json:"endpoints"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Endpoints == nil {
		t.Error("endpoints should be an empty array, not null")
	}
}

func TestHandlePerformance_GET_Disabled(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouterDisabled(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandlePerformance_GET_MethodNotAllowed(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouter(store, perfEnabledConfig())

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		req := httptest.NewRequest(method, "/api/v1/admin/performance", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s status = %d, want %d", method, w.Code, http.StatusMethodNotAllowed)
		}
	}
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/admin/performance — reset request stats
// ---------------------------------------------------------------------------

func TestHandlePerformance_DELETE_HappyPath(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouter(store, perfEnabledConfig())

	// Record some samples, then reset.
	r.recorder.Record("GET /api/v1/nodes", 50*time.Millisecond)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/performance", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify stats are cleared.
	snap := r.recorder.Snapshot()
	if len(snap) != 0 {
		t.Errorf("snapshot after reset has %d entries, want 0", len(snap))
	}
}

func TestHandlePerformance_DELETE_Disabled(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouterDisabled(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/performance", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// When disabled the route is not registered; DELETE falls through to the
	// frontend fallback which rejects non-GET/HEAD with 405.
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/performance/db — PostgreSQL stats
// ---------------------------------------------------------------------------

func TestHandlePerformanceDB_GET_HappyPath_WithStatements(t *testing.T) {
	lastVac := "2025-06-15T10:30:00Z"
	store := &mockStore{
		PgStatStatementsAvailableFn: func(ctx context.Context) bool { return true },
		TopQueryStatsFn: func(ctx context.Context, limit int) ([]datastore.TopQueryStat, error) {
			return []datastore.TopQueryStat{
				{
					Query:       "SELECT * FROM nodes",
					Calls:       100,
					TotalTimeMs: 5000.0,
					MeanTimeMs:  50.0,
					MinTimeMs:   1.0,
					MaxTimeMs:   500.0,
					Rows:        1000,
				},
			}, nil
		},
		TableStatsFn: func(ctx context.Context) ([]datastore.TableStat, error) {
			return []datastore.TableStat{
				{
					TableName:   "node_snapshots",
					SeqScan:     12,
					SeqTupRead:  480000,
					IdxScan:     9842,
					IdxTupFetch: 58200,
					NLiveTup:    60000,
					NDeadTup:    150,
					LastVacuum:  &lastVac,
				},
			}, nil
		},
		IndexStatsFn: func(ctx context.Context) ([]datastore.IndexStat, error) {
			return []datastore.IndexStat{
				{
					TableName: "node_snapshots",
					IndexName: "idx_node_snapshots_org",
					IdxScan:   9842,
					SizeBytes: 2457600,
				},
			}, nil
		},
		ActiveQueriesFn: func(ctx context.Context) ([]datastore.ActiveQuery, error) {
			return []datastore.ActiveQuery{
				{PID: 1234, State: "active", Query: "SELECT 1", DurationMs: 10.5},
			}, nil
		},
	}

	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		PgStatStatementsAvailable bool  `json:"pg_stat_statements_available"`
		TopQueries                []any `json:"top_queries"`
		TableStats                []any `json:"table_stats"`
		IndexStats                []any `json:"index_stats"`
		ActiveQueries             []any `json:"active_queries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !body.PgStatStatementsAvailable {
		t.Error("pg_stat_statements_available should be true")
	}
	if len(body.TopQueries) != 1 {
		t.Errorf("top_queries count = %d, want 1", len(body.TopQueries))
	}
	if len(body.TableStats) != 1 {
		t.Errorf("table_stats count = %d, want 1", len(body.TableStats))
	}
	if len(body.IndexStats) != 1 {
		t.Errorf("index_stats count = %d, want 1", len(body.IndexStats))
	}
	if len(body.ActiveQueries) != 1 {
		t.Errorf("active_queries count = %d, want 1", len(body.ActiveQueries))
	}
}

func TestHandlePerformanceDB_GET_WithoutStatements(t *testing.T) {
	store := &mockStore{
		PgStatStatementsAvailableFn: func(ctx context.Context) bool { return false },
		TopQueryStatsFn: func(ctx context.Context, limit int) ([]datastore.TopQueryStat, error) {
			return nil, nil
		},
		TableStatsFn: func(ctx context.Context) ([]datastore.TableStat, error) {
			return []datastore.TableStat{{TableName: "nodes"}}, nil
		},
		IndexStatsFn: func(ctx context.Context) ([]datastore.IndexStat, error) {
			return nil, nil
		},
		ActiveQueriesFn: func(ctx context.Context) ([]datastore.ActiveQuery, error) {
			return nil, nil
		},
	}

	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var body struct {
		PgStatStatementsAvailable bool  `json:"pg_stat_statements_available"`
		TopQueries                []any `json:"top_queries"`
		TableStats                []any `json:"table_stats"`
		IndexStats                []any `json:"index_stats"`
		ActiveQueries             []any `json:"active_queries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.PgStatStatementsAvailable {
		t.Error("pg_stat_statements_available should be false")
	}
	// top_queries should be empty array, not null
	if body.TopQueries == nil {
		t.Error("top_queries should be empty array, not null")
	}
	if len(body.TableStats) != 1 {
		t.Errorf("table_stats count = %d, want 1", len(body.TableStats))
	}
}

func TestHandlePerformanceDB_GET_TableStatsError(t *testing.T) {
	store := &mockStore{
		PgStatStatementsAvailableFn: func(ctx context.Context) bool { return false },
		TopQueryStatsFn: func(ctx context.Context, limit int) ([]datastore.TopQueryStat, error) {
			return nil, nil
		},
		TableStatsFn: func(ctx context.Context) ([]datastore.TableStat, error) {
			return nil, errors.New("connection refused")
		},
		IndexStatsFn: func(ctx context.Context) ([]datastore.IndexStat, error) {
			return nil, nil
		},
		ActiveQueriesFn: func(ctx context.Context) ([]datastore.ActiveQuery, error) {
			return nil, nil
		},
	}

	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandlePerformanceDB_GET_TopQueryStatsError(t *testing.T) {
	store := &mockStore{
		PgStatStatementsAvailableFn: func(ctx context.Context) bool { return true },
		TopQueryStatsFn: func(ctx context.Context, limit int) ([]datastore.TopQueryStat, error) {
			return nil, errors.New("db error")
		},
		TableStatsFn: func(ctx context.Context) ([]datastore.TableStat, error) {
			return nil, nil
		},
		IndexStatsFn: func(ctx context.Context) ([]datastore.IndexStat, error) {
			return nil, nil
		},
		ActiveQueriesFn: func(ctx context.Context) ([]datastore.ActiveQuery, error) {
			return nil, nil
		},
	}

	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandlePerformanceDB_GET_IndexStatsError(t *testing.T) {
	store := &mockStore{
		PgStatStatementsAvailableFn: func(ctx context.Context) bool { return false },
		TopQueryStatsFn: func(ctx context.Context, limit int) ([]datastore.TopQueryStat, error) {
			return nil, nil
		},
		TableStatsFn: func(ctx context.Context) ([]datastore.TableStat, error) {
			return nil, nil
		},
		IndexStatsFn: func(ctx context.Context) ([]datastore.IndexStat, error) {
			return nil, errors.New("db error")
		},
		ActiveQueriesFn: func(ctx context.Context) ([]datastore.ActiveQuery, error) {
			return nil, nil
		},
	}

	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandlePerformanceDB_GET_ActiveQueriesError(t *testing.T) {
	store := &mockStore{
		PgStatStatementsAvailableFn: func(ctx context.Context) bool { return false },
		TopQueryStatsFn: func(ctx context.Context, limit int) ([]datastore.TopQueryStat, error) {
			return nil, nil
		},
		TableStatsFn: func(ctx context.Context) ([]datastore.TableStat, error) {
			return nil, nil
		},
		IndexStatsFn: func(ctx context.Context) ([]datastore.IndexStat, error) {
			return nil, nil
		},
		ActiveQueriesFn: func(ctx context.Context) ([]datastore.ActiveQuery, error) {
			return nil, errors.New("db error")
		},
	}

	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandlePerformanceDB_GET_Disabled(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouterDisabled(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandlePerformanceDB_GET_MethodNotAllowed(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouter(store, perfEnabledConfig())

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		req := httptest.NewRequest(method, "/api/v1/admin/performance/db", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s status = %d, want %d", method, w.Code, http.StatusMethodNotAllowed)
		}
	}
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/admin/performance/db — reset PG stats
// ---------------------------------------------------------------------------

func TestHandlePerformanceDB_DELETE_HappyPath(t *testing.T) {
	var called bool
	store := &mockStore{
		ResetPgStatsFn: func(ctx context.Context) error {
			called = true
			return nil
		},
	}

	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusNoContent, w.Body.String())
	}
	if !called {
		t.Error("ResetPgStats was not called")
	}
}

func TestHandlePerformanceDB_DELETE_Error(t *testing.T) {
	store := &mockStore{
		ResetPgStatsFn: func(ctx context.Context) error {
			return errors.New("permission denied")
		},
	}

	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandlePerformanceDB_DELETE_Disabled(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouterDisabled(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// When disabled the route is not registered; DELETE falls through to the
	// frontend fallback which rejects non-GET/HEAD with 405.
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// Endpoint key parsing — method + path split
// ---------------------------------------------------------------------------

func TestHandlePerformance_GET_EndpointKeyParsing(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouter(store, perfEnabledConfig())

	// Record with a "METHOD path" key (as the middleware generates).
	r.recorder.Record("GET /api/v1/nodes", 10*time.Millisecond)
	r.recorder.Record("POST /api/v1/exports", 20*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body struct {
		Endpoints []struct {
			Method string `json:"method"`
			Path   string `json:"path"`
		} `json:"endpoints"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	found := map[string]bool{}
	for _, ep := range body.Endpoints {
		found[ep.Method+" "+ep.Path] = true
	}
	if !found["GET /api/v1/nodes"] {
		t.Error("missing GET /api/v1/nodes endpoint")
	}
	if !found["POST /api/v1/exports"] {
		t.Error("missing POST /api/v1/exports endpoint")
	}
}

// ---------------------------------------------------------------------------
// Content-Type checks
// ---------------------------------------------------------------------------

func TestHandlePerformance_GET_ContentType(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}
}

func TestHandlePerformanceDB_GET_ContentType(t *testing.T) {
	store := &mockStore{
		PgStatStatementsAvailableFn: func(ctx context.Context) bool { return false },
		TopQueryStatsFn: func(ctx context.Context, limit int) ([]datastore.TopQueryStat, error) {
			return nil, nil
		},
		TableStatsFn: func(ctx context.Context) ([]datastore.TableStat, error) {
			return nil, nil
		},
		IndexStatsFn: func(ctx context.Context) ([]datastore.IndexStat, error) {
			return nil, nil
		},
		ActiveQueriesFn: func(ctx context.Context) ([]datastore.ActiveQuery, error) {
			return nil, nil
		},
	}

	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}
}

// ---------------------------------------------------------------------------
// Null-safety: arrays are [] not null
// ---------------------------------------------------------------------------

func TestHandlePerformanceDB_GET_NilArraysSerialisedAsEmpty(t *testing.T) {
	store := &mockStore{
		PgStatStatementsAvailableFn: func(ctx context.Context) bool { return false },
		TopQueryStatsFn: func(ctx context.Context, limit int) ([]datastore.TopQueryStat, error) {
			return nil, nil
		},
		TableStatsFn: func(ctx context.Context) ([]datastore.TableStat, error) {
			return nil, nil
		},
		IndexStatsFn: func(ctx context.Context) ([]datastore.IndexStat, error) {
			return nil, nil
		},
		ActiveQueriesFn: func(ctx context.Context) ([]datastore.ActiveQuery, error) {
			return nil, nil
		},
	}

	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance/db", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Parse raw JSON to check for null vs [].
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"top_queries", "table_stats", "index_stats", "active_queries"} {
		v, ok := raw[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}
		if string(v) == "null" {
			t.Errorf("%s is null, want []", key)
		}
	}
}

func TestHandlePerformance_GET_NilEndpointsSerialisedAsEmpty(t *testing.T) {
	store := &mockStore{}
	r := newPerfRouter(store, perfEnabledConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/performance", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v, ok := raw["endpoints"]
	if !ok {
		t.Fatal("missing key endpoints")
	}
	if string(v) == "null" {
		t.Error("endpoints is null, want []")
	}
}
