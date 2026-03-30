// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package perf

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helpers — mock ResponseWriters with optional interface support
// ---------------------------------------------------------------------------

// hijackableRecorder embeds httptest.ResponseRecorder and adds Hijack support
// so we can test that statusRecorder forwards the interface.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (hr *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hr.hijacked = true
	return nil, nil, nil
}

// flushableRecorder embeds httptest.ResponseRecorder and adds an explicit
// Flush that tracks whether it was called.
type flushableRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (fr *flushableRecorder) Flush() {
	fr.flushed = true
}

// hijackFlushRecorder implements both http.Hijacker and http.Flusher.
type hijackFlushRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
	flushed  bool
}

func (hfr *hijackFlushRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hfr.hijacked = true
	return nil, nil, nil
}

func (hfr *hijackFlushRecorder) Flush() {
	hfr.flushed = true
}

// plainResponseWriter is a minimal http.ResponseWriter that does NOT
// implement http.Hijacker or http.Flusher.
type plainResponseWriter struct {
	code int
}

func (pw *plainResponseWriter) Header() http.Header         { return http.Header{} }
func (pw *plainResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (pw *plainResponseWriter) WriteHeader(code int)        { pw.code = code }

// Compile-time check: plainResponseWriter is an http.ResponseWriter.
var _ http.ResponseWriter = (*plainResponseWriter)(nil)

func init() {
	// Runtime guard: plainResponseWriter must NOT satisfy the optional interfaces.
	var w http.ResponseWriter = &plainResponseWriter{}
	if _, ok := w.(http.Hijacker); ok {
		panic("plainResponseWriter must not implement http.Hijacker")
	}
	if _, ok := w.(http.Flusher); ok {
		panic("plainResponseWriter must not implement http.Flusher")
	}
}

// ---------------------------------------------------------------------------
// Existing latency / key / error recording tests
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Hijacker forwarding tests
// ---------------------------------------------------------------------------

func TestStatusRecorder_HijackDelegatesToUnderlyingWriter(t *testing.T) {
	inner := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	var w http.ResponseWriter = &statusRecorder{ResponseWriter: inner, status: http.StatusOK}

	hj, ok := w.(http.Hijacker)
	if !ok {
		t.Fatal("statusRecorder wrapping a Hijacker should implement http.Hijacker")
	}

	_, _, err := hj.Hijack()
	if err != nil {
		t.Fatalf("unexpected error from Hijack: %v", err)
	}
	if !inner.hijacked {
		t.Fatal("expected Hijack to delegate to the underlying writer")
	}
}

func TestStatusRecorder_HijackFailsWhenUnderlyingDoesNotSupport(t *testing.T) {
	inner := &plainResponseWriter{}
	var w http.ResponseWriter = &statusRecorder{ResponseWriter: inner, status: http.StatusOK}

	hj, ok := w.(http.Hijacker)
	if !ok {
		t.Fatal("statusRecorder should always implement http.Hijacker")
	}

	_, _, err := hj.Hijack()
	if err == nil {
		t.Fatal("expected error when underlying writer does not support Hijack")
	}
}

func TestMiddleware_HijackerPassesThroughWrap(t *testing.T) {
	rec := NewRecorder(5*time.Minute, 100, 1000)
	mw := NewMiddleware(rec)

	var sawHijacker bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawHijacker = w.(http.Hijacker)
		w.WriteHeader(http.StatusSwitchingProtocols)
	})

	wrapped := mw.Wrap(handler)
	inner := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest("GET", "/api/v1/ws", nil)
	wrapped.ServeHTTP(inner, req)

	if !sawHijacker {
		t.Fatal("handler should see http.Hijacker when underlying writer supports it")
	}
}

// ---------------------------------------------------------------------------
// Flusher forwarding tests
// ---------------------------------------------------------------------------

func TestStatusRecorder_FlushDelegatesToUnderlyingWriter(t *testing.T) {
	inner := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	var w http.ResponseWriter = &statusRecorder{ResponseWriter: inner, status: http.StatusOK}

	f, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("statusRecorder wrapping a Flusher should implement http.Flusher")
	}

	f.Flush()
	if !inner.flushed {
		t.Fatal("expected Flush to delegate to the underlying writer")
	}
}

func TestStatusRecorder_FlushNoOpWhenUnderlyingDoesNotSupport(t *testing.T) {
	inner := &plainResponseWriter{}
	var w http.ResponseWriter = &statusRecorder{ResponseWriter: inner, status: http.StatusOK}

	f, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("statusRecorder should always implement http.Flusher")
	}

	// Should not panic.
	f.Flush()
}

func TestMiddleware_FlusherPassesThroughWrap(t *testing.T) {
	rec := NewRecorder(5*time.Minute, 100, 1000)
	mw := NewMiddleware(rec)

	var sawFlusher bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawFlusher = w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
	})

	wrapped := mw.Wrap(handler)
	inner := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	wrapped.ServeHTTP(inner, req)

	if !sawFlusher {
		t.Fatal("handler should see http.Flusher when underlying writer supports it")
	}
}

// ---------------------------------------------------------------------------
// Combined Hijacker + Flusher test
// ---------------------------------------------------------------------------

func TestStatusRecorder_BothHijackerAndFlusher(t *testing.T) {
	inner := &hijackFlushRecorder{ResponseRecorder: httptest.NewRecorder()}
	var w http.ResponseWriter = &statusRecorder{ResponseWriter: inner, status: http.StatusOK}

	hj, ok := w.(http.Hijacker)
	if !ok {
		t.Fatal("statusRecorder should implement http.Hijacker")
	}

	f, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("statusRecorder should implement http.Flusher")
	}

	if _, _, err := hj.Hijack(); err != nil {
		t.Fatalf("unexpected Hijack error: %v", err)
	}
	f.Flush()

	if !inner.hijacked {
		t.Fatal("expected Hijack to delegate")
	}
	if !inner.flushed {
		t.Fatal("expected Flush to delegate")
	}
}
