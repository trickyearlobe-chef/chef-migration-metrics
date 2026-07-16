// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Enabling ingest via the admin API persists a complete section (numeric knobs
// defaulted) and takes effect without a restart (applied granularity). Sections
// serialise via yaml tags → snake_case JSON, so assert on the raw value map.
func TestAdminConfigIngest_PUT_EnableRoundTrip(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/ingest",
		strings.NewReader(`{"enabled":true,"retention_days":3}`))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	restartRequired := decodePutValue(t, w, &got)
	if got["enabled"] != true {
		t.Errorf("stored enabled = %v, want true", got["enabled"])
	}
	if got["retention_days"] != float64(3) {
		t.Errorf("stored retention_days = %v, want 3", got["retention_days"])
	}
	if n, _ := got["max_body_bytes"].(float64); n < 1 {
		t.Errorf("max_body_bytes not defaulted: %v", got["max_body_bytes"])
	}
	if n, _ := got["max_records_per_body"].(float64); n < 1 {
		t.Errorf("max_records_per_body not defaulted: %v", got["max_records_per_body"])
	}
	if restartRequired {
		t.Error("ingest is read live per request — PUT should not require a restart")
	}
}

func TestAdminConfigIngest_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/ingest",
		strings.NewReader(`{"enabled":true}`))
	r.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusServiceUnavailable)
}
