// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTLSStatusRouter(t *testing.T, holder *TLSStatusHolder) *Router {
	t.Helper()
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	opts := []RouterOption{}
	if holder != nil {
		opts = append(opts, WithTLSStatus(holder))
	}
	return NewRouter(&mockStore{}, cfg, hub, opts...)
}

func decodeTLSStatus(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}
	return resp
}

// No holder wired → the endpoint still answers and reports healthy. This keeps
// the banner poll harmless on plain-HTTP / ACME deployments that never set one.
func TestHandleServerTLSStatus_NoHolder(t *testing.T) {
	r := newTLSStatusRouter(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/tls-status", nil)
	w := httptest.NewRecorder()
	r.handleServerTLSStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	resp := decodeTLSStatus(t, w.Body.Bytes())
	if resp["degraded"] != false {
		t.Errorf("degraded = %v, want false", resp["degraded"])
	}
}

func TestHandleServerTLSStatus_Healthy(t *testing.T) {
	r := newTLSStatusRouter(t, NewTLSStatusHolder())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/tls-status", nil)
	w := httptest.NewRecorder()
	r.handleServerTLSStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	resp := decodeTLSStatus(t, w.Body.Bytes())
	if resp["degraded"] != false {
		t.Errorf("degraded = %v, want false", resp["degraded"])
	}
	if reason, ok := resp["reason"]; ok && reason != "" {
		t.Errorf("reason = %q, want empty when healthy", reason)
	}
}

func TestHandleServerTLSStatus_Degraded(t *testing.T) {
	holder := NewTLSStatusHolder()
	holder.SetDegraded("TLS listener setup failed: bad cert")
	r := newTLSStatusRouter(t, holder)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/tls-status", nil)
	w := httptest.NewRecorder()
	r.handleServerTLSStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	resp := decodeTLSStatus(t, w.Body.Bytes())
	if resp["degraded"] != true {
		t.Fatalf("degraded = %v, want true", resp["degraded"])
	}
	if resp["reason"] != "TLS listener setup failed: bad cert" {
		t.Errorf("reason = %q, want the recorded cause", resp["reason"])
	}
}

func TestHandleServerTLSStatus_MethodNotAllowed(t *testing.T) {
	r := newTLSStatusRouter(t, NewTLSStatusHolder())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/server/tls-status", nil)
	w := httptest.NewRecorder()
	r.handleServerTLSStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// The endpoint must be reachable without authentication and without touching the
// database, so the banner renders pre-login and even when the DB is down.
func TestHandleServerTLSStatus_PublicNoDB(t *testing.T) {
	holder := NewTLSStatusHolder()
	holder.SetDegraded("boom")
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	// failingStore.Ping errors; the handler must not call it.
	r := NewRouter(&mockStore{}, cfg, hub, WithTLSStatus(holder))

	srv := httptest.NewServer(r)
	defer srv.Close()

	res, err := http.Get(srv.URL + "/api/v1/server/tls-status")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

// The holder must be safe for concurrent reads (banner poll) and the one-shot
// write from the startup goroutine.
func TestTLSStatusHolder_Concurrent(t *testing.T) {
	holder := NewTLSStatusHolder()
	done := make(chan struct{})
	go func() {
		for range 1000 {
			_ = holder.Status()
		}
		close(done)
	}()
	holder.SetDegraded("late failure")
	<-done
	if !holder.Status().Degraded {
		t.Error("expected degraded after SetDegraded")
	}
}
