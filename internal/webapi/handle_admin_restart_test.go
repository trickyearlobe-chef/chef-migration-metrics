// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleAdminRestart_NotConfigured(t *testing.T) {
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(&mockStore{}, cfg, hub) // no restart trigger wired

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/restart", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when no restart trigger wired, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestHandleAdminRestart_MethodNotAllowed(t *testing.T) {
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	called := false
	r := NewRouter(&mockStore{}, cfg, hub, WithRestartTrigger(func() { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/restart", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET, got %d", w.Code)
	}
	if called {
		t.Fatal("restart trigger must not fire for a non-POST request")
	}
}

func TestHandleAdminRestart_TriggersRestart(t *testing.T) {
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	called := false
	r := NewRouter(&mockStore{}, cfg, hub, WithRestartTrigger(func() { called = true }))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/restart", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d (%s)", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("expected the restart trigger to be invoked")
	}

	var body struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "restarting" {
		t.Fatalf("expected status %q, got %q", "restarting", body.Status)
	}
	if body.Message == "" {
		t.Fatal("expected a non-empty message")
	}
}
