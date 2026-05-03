// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/platform"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestRouterForDisplayNames(t *testing.T) *Router {
	t.Helper()
	cs := newTestConfigStore(t)
	return newTestRouterForAdminConfig(nil, cs, nil)
}

func newTestRouterForDisplayNamesNilStore() *Router {
	return newTestRouterForAdminConfig(nil, nil, nil)
}

func decodeDisplayNamesResponse(t *testing.T, w *httptest.ResponseRecorder) platformDisplayNamesResponse {
	t.Helper()
	var resp platformDisplayNamesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response body: %v\nbody: %s", err, w.Body.String())
	}
	return resp
}

// ---------------------------------------------------------------------------
// GET tests
// ---------------------------------------------------------------------------

func TestHandlePlatformDisplayNames_Get_DefaultsWhenNoConfig(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-display-names", nil)
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusOK)

	resp := decodeDisplayNamesResponse(t, w)
	if !resp.IsDefault {
		t.Error("expected is_default=true when no config stored")
	}
	if len(resp.Mappings) != len(platform.DefaultMappings) {
		t.Errorf("mappings count = %d, want %d", len(resp.Mappings), len(platform.DefaultMappings))
	}
}

func TestHandlePlatformDisplayNames_Get_CustomMappings(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	// Store custom mappings via PUT first.
	custom := `[{"platform":"testos","version_prefix":"1.0","display_name":"Test OS 1.0"}]`
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/platform-display-names", strings.NewReader(custom))
	putW := httptest.NewRecorder()
	r.handlePlatformDisplayNames(putW, putReq)
	assertStatus(t, putW, http.StatusOK)

	// Now GET should return custom mappings.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-display-names", nil)
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusOK)

	resp := decodeDisplayNamesResponse(t, w)
	if resp.IsDefault {
		t.Error("expected is_default=false for custom mappings")
	}
	if len(resp.Mappings) != 1 {
		t.Fatalf("mappings count = %d, want 1", len(resp.Mappings))
	}
	if resp.Mappings[0].Platform != "testos" {
		t.Errorf("platform = %q, want %q", resp.Mappings[0].Platform, "testos")
	}
}

// ---------------------------------------------------------------------------
// PUT tests
// ---------------------------------------------------------------------------

func TestHandlePlatformDisplayNames_Put_Valid(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	body := `[
		{"platform":"windows","version_prefix":"10.0","display_name":"Windows 10+"},
		{"platform":"centos","version_prefix":"7","display_name":"CentOS 7"}
	]`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/platform-display-names", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusOK)

	resp := decodeDisplayNamesResponse(t, w)
	if len(resp.Mappings) != 2 {
		t.Fatalf("mappings count = %d, want 2", len(resp.Mappings))
	}
	if resp.IsDefault {
		t.Error("expected is_default=false for custom mappings")
	}
}

func TestHandlePlatformDisplayNames_Put_EmptyPlatform(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	body := `[{"platform":"","version_prefix":"1.0","display_name":"Foo"}]`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/platform-display-names", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestHandlePlatformDisplayNames_Put_EmptyVersionPrefix(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	body := `[{"platform":"windows","version_prefix":"","display_name":"Foo"}]`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/platform-display-names", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestHandlePlatformDisplayNames_Put_EmptyDisplayName(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	body := `[{"platform":"windows","version_prefix":"10.0","display_name":""}]`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/platform-display-names", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestHandlePlatformDisplayNames_Put_DuplicateEntry(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	body := `[
		{"platform":"windows","version_prefix":"10.0","display_name":"Win 10"},
		{"platform":"Windows","version_prefix":"10.0","display_name":"Windows 10"}
	]`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/platform-display-names", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestHandlePlatformDisplayNames_Put_NormalizesCase(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	body := `[{"platform":"Windows","version_prefix":"10.0","display_name":"Win 10+"}]`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/platform-display-names", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusOK)

	resp := decodeDisplayNamesResponse(t, w)
	if len(resp.Mappings) != 1 {
		t.Fatalf("mappings count = %d, want 1", len(resp.Mappings))
	}
	if resp.Mappings[0].Platform != "windows" {
		t.Errorf("platform = %q, want %q", resp.Mappings[0].Platform, "windows")
	}
}

func TestHandlePlatformDisplayNames_Put_InvalidJSON(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/platform-display-names", strings.NewReader("{not json"))
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorCode(t, w, ErrCodeBadRequest)
}

// ---------------------------------------------------------------------------
// POST reset tests
// ---------------------------------------------------------------------------

func TestHandlePlatformDisplayNames_Reset(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	// Store custom mappings first.
	custom := `[{"platform":"testos","version_prefix":"1.0","display_name":"Test OS 1.0"}]`
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/platform-display-names", strings.NewReader(custom))
	putW := httptest.NewRecorder()
	r.handlePlatformDisplayNames(putW, putReq)
	assertStatus(t, putW, http.StatusOK)

	// Reset.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/platform-display-names/reset", nil)
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNamesReset(w, req)

	assertStatus(t, w, http.StatusOK)

	resp := decodeDisplayNamesResponse(t, w)
	if !resp.IsDefault {
		t.Error("expected is_default=true after reset")
	}
	if len(resp.Mappings) != len(platform.DefaultMappings) {
		t.Errorf("mappings count = %d, want %d", len(resp.Mappings), len(platform.DefaultMappings))
	}

	// Verify subsequent GET returns defaults.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-display-names", nil)
	getW := httptest.NewRecorder()
	r.handlePlatformDisplayNames(getW, getReq)

	assertStatus(t, getW, http.StatusOK)
	getResp := decodeDisplayNamesResponse(t, getW)
	if !getResp.IsDefault {
		t.Error("expected is_default=true on GET after reset")
	}
}

func TestHandlePlatformDisplayNames_Reset_MethodNotAllowed(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-display-names/reset", nil)
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNamesReset(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}

// ---------------------------------------------------------------------------
// Method not allowed
// ---------------------------------------------------------------------------

func TestHandlePlatformDisplayNames_MethodNotAllowed(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/platform-display-names", nil)
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}

func TestHandlePlatformDisplayNames_DeleteNotAllowed(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/platform-display-names", nil)
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}

// ---------------------------------------------------------------------------
// No config store (503)
// ---------------------------------------------------------------------------

func TestHandlePlatformDisplayNames_NoConfigStore(t *testing.T) {
	r := newTestRouterForDisplayNamesNilStore()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-display-names", nil)
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestHandlePlatformDisplayNames_NoConfigStore_PUT(t *testing.T) {
	r := newTestRouterForDisplayNamesNilStore()

	body := `[{"platform":"windows","version_prefix":"10.0","display_name":"Win"}]`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/platform-display-names", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestHandlePlatformDisplayNames_NoConfigStore_Reset(t *testing.T) {
	r := newTestRouterForDisplayNamesNilStore()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/platform-display-names/reset", nil)
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNamesReset(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

// ---------------------------------------------------------------------------
// resolvePlatformDisplayName tests
// ---------------------------------------------------------------------------

func TestResolvePlatformDisplayName(t *testing.T) {
	mappings := []platform.DisplayNameMapping{
		{Platform: "windows", VersionPrefix: "10.0.22631", DisplayName: "Win11 23H2"},
		{Platform: "centos", VersionPrefix: "7", DisplayName: "CentOS 7 (EOL)"},
	}

	t.Run("match found", func(t *testing.T) {
		got := resolvePlatformDisplayName("windows", "10.0.22631.1234", mappings)
		if got == nil {
			t.Fatal("expected non-nil pointer for match")
		}
		if *got != "Win11 23H2" {
			t.Errorf("display name = %q, want %q", *got, "Win11 23H2")
		}
	})

	t.Run("no mapping uses abbreviation or raw fallback", func(t *testing.T) {
		got := resolvePlatformDisplayName("debian", "11.0", mappings)
		if got == nil {
			t.Fatal("expected non-nil with new resolver")
		}
		// debian has no abbreviation, so raw fallback
		if *got != "debian 11.0" {
			t.Errorf("display name = %q, want %q", *got, "debian 11.0")
		}
	})

	t.Run("case insensitive platform", func(t *testing.T) {
		got := resolvePlatformDisplayName("CentOS", "7.9.2009", mappings)
		if got == nil {
			t.Fatal("expected non-nil pointer for case-insensitive match")
		}
		if *got != "CentOS 7 (EOL)" {
			t.Errorf("display name = %q, want %q", *got, "CentOS 7 (EOL)")
		}
	})

	t.Run("empty platform", func(t *testing.T) {
		got := resolvePlatformDisplayName("", "10.0", mappings)
		if got != nil {
			t.Errorf("expected nil for empty platform, got %q", *got)
		}
	})

	t.Run("empty version still resolves", func(t *testing.T) {
		// With the centralized resolver, platform without version still resolves
		got := resolvePlatformDisplayName("windows", "", mappings)
		if got == nil {
			t.Fatal("expected non-nil for platform with empty version")
		}
		if *got != "windows" {
			t.Errorf("display name = %q, want %q", *got, "windows")
		}
	})

	t.Run("nil mappings uses abbreviation fallback", func(t *testing.T) {
		got := resolvePlatformDisplayName("redhat", "8.10", nil)
		if got == nil {
			t.Fatal("expected non-nil with abbreviation fallback")
		}
		if *got != "RHEL 8.10" {
			t.Errorf("display name = %q, want %q", *got, "RHEL 8.10")
		}
	})
}

// ---------------------------------------------------------------------------
// PUT with empty array (valid — clears all custom mappings)
// ---------------------------------------------------------------------------

func TestHandlePlatformDisplayNames_Put_EmptyArray(t *testing.T) {
	r := newTestRouterForDisplayNames(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/platform-display-names", strings.NewReader("[]"))
	w := httptest.NewRecorder()
	r.handlePlatformDisplayNames(w, req)

	assertStatus(t, w, http.StatusOK)

	resp := decodeDisplayNamesResponse(t, w)
	if len(resp.Mappings) != 0 {
		t.Errorf("mappings count = %d, want 0", len(resp.Mappings))
	}
	if resp.IsDefault {
		t.Error("expected is_default=false for empty mappings list")
	}
}
