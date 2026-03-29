// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package perf

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMiddleware_RecordsLatency(t *testing.T) {
	rec := NewRecorder(5*time.Minute, 100, 1000)
	mw := NewMiddleware(rec)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	wrapped := mw.Wrap(handler)
	req := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	snap := rec.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 key, got %d", len(snap))
	}
	if snap[0].Count != 1 {
		t.Fatalf("expected Count=1, got %d", snap[0].Count)
	}
	if snap[0].P50 < 5*time.Millisecond {
		t.Fatalf("expected P50 >= 5ms, got %s", snap[0].P50)
	}
}

func TestMiddleware_KeyFormat(t *testing.T) {
	rec := NewRecorder(5*time.Minute, 100, 1000)
	mw := NewMiddleware(rec)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/nodes", mw.Wrap(handler))

	req := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	snap := rec.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 key, got %d", len(snap))
	}
	if snap[0].Key != "GET /api/v1/nodes" {
		t.Fatalf("expected key %q, got %q", "GET /api/v1/nodes", snap[0].Key)
	}
}

func TestMiddleware_RecordsErrors(t *testing.T) {
	rec := NewRecorder(5*time.Minute, 100, 1000)
	mw := NewMiddleware(rec)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	wrapped := mw.Wrap(handler)
	req := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	snap := rec.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 key, got %d", len(snap))
	}
	if snap[0].ErrorCount != 1 {
		t.Fatalf("expected ErrorCount=1, got %d", snap[0].ErrorCount)
	}
}

func TestMiddleware_NoErrorFor4xx(t *testing.T) {
	rec := NewRecorder(5*time.Minute, 100, 1000)
	mw := NewMiddleware(rec)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	wrapped := mw.Wrap(handler)
	req := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	snap := rec.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 key, got %d", len(snap))
	}
	if snap[0].ErrorCount != 0 {
		t.Fatalf("expected ErrorCount=0, got %d", snap[0].ErrorCount)
	}
}

func TestMiddleware_MultipleRequests(t *testing.T) {
	rec := NewRecorder(5*time.Minute, 100, 1000)
	mw := NewMiddleware(rec)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/nodes", mw.Wrap(handler))
	mux.Handle("GET /api/v1/cookbooks", mw.Wrap(handler))

	// 1 request to /api/v1/nodes
	req := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	// 2 requests to /api/v1/cookbooks
	for i := 0; i < 2; i++ {
		req = httptest.NewRequest("GET", "/api/v1/cookbooks", nil)
		rr = httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
	}

	snap := rec.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(snap))
	}

	counts := make(map[string]int)
	for _, ks := range snap {
		counts[ks.Key] = ks.Count
	}

	if counts["GET /api/v1/nodes"] != 1 {
		t.Fatalf("expected 1 request for nodes, got %d", counts["GET /api/v1/nodes"])
	}
	if counts["GET /api/v1/cookbooks"] != 2 {
		t.Fatalf("expected 2 requests for cookbooks, got %d", counts["GET /api/v1/cookbooks"])
	}
}
