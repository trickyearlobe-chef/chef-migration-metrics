// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
)

func TestHandleAuthInfo_NoProviders(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/info", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		LocalEnabled bool `json:"local_enabled"`
		SAMLEnabled  bool `json:"saml_enabled"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.LocalEnabled {
		t.Error("expected local_enabled=false when localAuth not wired")
	}
	if resp.SAMLEnabled {
		t.Error("expected saml_enabled=false when samlHandler not wired")
	}
}

func TestHandleAuthInfo_LocalOnly(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})
	r.localAuth = auth.NewLocalAuthenticator(nil, 5)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/info", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		LocalEnabled bool `json:"local_enabled"`
		SAMLEnabled  bool `json:"saml_enabled"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.LocalEnabled {
		t.Error("expected local_enabled=true")
	}
	if resp.SAMLEnabled {
		t.Error("expected saml_enabled=false")
	}
}

func TestHandleAuthInfo_SAMLEnabled(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})
	r.samlHandler = &SAMLHandler{logger: func(string, string) {}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/info", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		LocalEnabled bool `json:"local_enabled"`
		SAMLEnabled  bool `json:"saml_enabled"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.LocalEnabled {
		t.Error("expected local_enabled=false")
	}
	if !resp.SAMLEnabled {
		t.Error("expected saml_enabled=true")
	}
}

func TestHandleAuthInfo_MethodNotAllowed(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/info", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}
