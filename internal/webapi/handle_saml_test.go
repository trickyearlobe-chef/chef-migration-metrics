// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleMetadata_MethodNotAllowed(t *testing.T) {
	h := &SAMLHandler{logger: func(string, string) {}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/saml/metadata", nil)
	h.HandleMetadata(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestHandleLogin_MethodNotAllowed(t *testing.T) {
	h := &SAMLHandler{logger: func(string, string) {}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/saml/login", nil)
	h.HandleLogin(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestHandleLogin_InvalidReturnTo(t *testing.T) {
	h := &SAMLHandler{logger: func(string, string) {}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/saml/login?returnTo=https://evil.com", nil)
	h.HandleLogin(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for absolute returnTo, got %d", rec.Code)
	}
}

func TestHandleLogin_ProtocolRelativeReturnTo(t *testing.T) {
	h := &SAMLHandler{logger: func(string, string) {}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/saml/login?returnTo=//evil.com/path", nil)
	h.HandleLogin(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for protocol-relative returnTo, got %d", rec.Code)
	}
}

func TestHandleACS_MethodNotAllowed(t *testing.T) {
	h := &SAMLHandler{logger: func(string, string) {}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/saml/acs", nil)
	h.HandleACS(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestHandleSLO_MethodNotAllowed(t *testing.T) {
	h := &SAMLHandler{logger: func(string, string) {}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/saml/slo", nil)
	h.HandleSLO(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestIsRelativePath(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/dashboard", true},
		{"/", true},
		{"/path/to/resource", true},
		{"//evil.com", false},
		{`/\evil.com`, false},   // backslash bypass: browsers read /\ as //
		{`/\/evil.com`, false},  // mixed slash/backslash authority bypass
		{`/\\evil.com`, false},  // backslash variant
		{"https://evil.com", false},
		{"http://evil.com", false},
		{"relative/path", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isRelativePath(tt.input)
		if got != tt.want {
			t.Errorf("isRelativePath(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
