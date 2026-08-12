// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// Test Kitchen is not in here: it keeps a record of its own, and this call
// refuses a body carrying a field it does not read.
const validAnalysisToolsBody = `{
	"cookstyle_timeout_minutes": 10,
	"test_kitchen_timeout_minutes": 0
}`

const validTKBody = `{"timeout_minutes": 30, "driver": "vcenter"}`

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/analysis-tools
// ---------------------------------------------------------------------------

func TestAdminConfigAnalysisTools_GET(t *testing.T) {
	cfg := testConfig()
	cfg.AnalysisTools = config.AnalysisToolsConfig{
		CookstyleTimeoutMinutes: 10,
		TestKitchen:             config.TestKitchenConfig{Driver: "vcenter"},
	}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/analysis-tools", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)

	// Config values are in the "value" envelope.
	value, ok := got["value"].(map[string]any)
	if !ok {
		t.Fatalf("value field missing or not an object; got: %v", got["value"])
	}
	if value["cookstyle_timeout_minutes"] != float64(10) {
		t.Errorf("cookstyle_timeout_minutes = %v, want 10", value["cookstyle_timeout_minutes"])
	}
}

func TestAdminConfigAnalysisTools_GET_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(testConfig(), nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/analysis-tools", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigAnalysisTools_GET_UsesHolder(t *testing.T) {
	cfg := testConfig()
	cfg.AnalysisTools = config.AnalysisToolsConfig{CookstyleTimeoutMinutes: 15}
	holder := configstore.NewConfigHolder(cfg, nil)
	r := newTestRouterForAdminConfig(testConfig(), nil, holder)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/analysis-tools", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	value, ok := got["value"].(map[string]any)
	if !ok {
		t.Fatalf("value field missing or not an object; got: %v", got["value"])
	}
	if value["cookstyle_timeout_minutes"] != float64(15) {
		t.Errorf("cookstyle_timeout_minutes = %v, want 15", value["cookstyle_timeout_minutes"])
	}
}

func TestAdminConfigAnalysisTools_PUT_Success(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/analysis-tools", strings.NewReader(validAnalysisToolsBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodePutValue(t, w, &got)
	if got["cookstyle_timeout_minutes"] != float64(10) {
		t.Errorf("cookstyle_timeout_minutes = %v, want 10", got["cookstyle_timeout_minutes"])
	}

	stored, err := store.Get(context.Background(), configstore.KeyAnalysisTools)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	var storedMap map[string]any
	if err := json.Unmarshal(stored, &storedMap); err != nil {
		t.Fatalf("unmarshal stored: %v", err)
	}
	if storedMap["cookstyle_timeout_minutes"] != float64(10) {
		t.Errorf("stored cookstyle_timeout_minutes = %v, want 10", storedMap["cookstyle_timeout_minutes"])
	}
}

func TestAdminConfigAnalysisTools_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/analysis-tools", strings.NewReader(validAnalysisToolsBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestAdminConfigAnalysisTools_PUT_400_InvalidJSON(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/analysis-tools", strings.NewReader("{"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
}

func TestAdminConfigAnalysisTools_PUT_422_ZeroCookstyleTimeout(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"cookstyle_timeout_minutes": 0}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/analysis-tools", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigTestKitchen_PUT_422_NegativeTKTimeout(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"timeout_minutes": -1, "driver": "vcenter"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigTestKitchen_PUT_422_NegativeStartRateWindow(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"driver": "vcenter", "start_rate_window_minutes": -1}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigTestKitchen_PUT_StartRateLimitRoundTrips(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"driver": "vcenter", "start_rate_window_minutes": 90, "start_rate_max_per_window": 25}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigTestKitchen_PUT_422_PlatformMapMissingKitchenName(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"driver": "vcenter", "platform_map": [{"image": "ubuntu"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigTestKitchen_PUT_422_PlatformMapDuplicateName(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"driver": "vcenter", "platform_map": [{"kitchen_name": "ubuntu-22.04", "image": "ubuntu"}, {"kitchen_name": "ubuntu-22.04", "image": "ubuntu2"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigAnalysisTools_PUT_405_WrongMethod(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/config/analysis-tools", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}

// ---------------------------------------------------------------------------
// PUT verdicts_changed (cookstyle rescore-on-save)
// ---------------------------------------------------------------------------

func TestAdminConfigAnalysisTools_PUT_VerdictsChangedInResponse(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	// PUT triggers a rescore — the mock store has no results so verdicts_changed = 0
	body := `{"driver": "vcenter"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var resp putConfigResponse
	decodeBody(t, w, &resp)
	// With no stored results, verdicts_changed is 0 (omitted in JSON, but
	// still 0 when decoded into the struct).
	if resp.VerdictsChanged != 0 {
		t.Errorf("verdicts_changed = %d, want 0 (no stored results)", resp.VerdictsChanged)
	}
	if resp.RestartRequired {
		t.Error("restart_required should be false")
	}
	if resp.Reload != "subsystem" {
		t.Errorf("reload = %q, want 'subsystem'", resp.Reload)
	}
}

// ---------------------------------------------------------------------------
// GET/PUT/DELETE /api/v1/admin/config/test-kitchen
// ---------------------------------------------------------------------------

func TestAdminConfigTestKitchen_GET(t *testing.T) {
	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen.Driver = "vcenter"
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/test-kitchen", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["driver"] != "vcenter" {
		t.Errorf("driver = %v, want vcenter", got["driver"])
	}
}

func TestAdminConfigTestKitchen_GET_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(testConfig(), nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/test-kitchen", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigTestKitchen_GET_UsesHolder(t *testing.T) {
	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen.Driver = "ec2"
	holder := configstore.NewConfigHolder(cfg, nil)
	r := newTestRouterForAdminConfig(testConfig(), nil, holder)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/test-kitchen", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["driver"] != "ec2" {
		t.Errorf("driver = %v, want ec2", got["driver"])
	}
}

func TestAdminConfigTestKitchen_PUT_Success(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(validTKBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodePutValue(t, w, &got)
	if got["driver"] != "vcenter" {
		t.Errorf("driver = %v, want vcenter", got["driver"])
	}

	if _, err := store.Get(context.Background(), configstore.KeyTestKitchen); err != nil {
		t.Fatalf("Test Kitchen has no record of its own in the store: %v", err)
	}
}

// Test Kitchen writes its own record and leaves the analysis tools alone.
//
// This used to check the opposite: that saving Test Kitchen merged its part
// back into the shared analysis tools record. That merge was the safe half of
// an arrangement whose other half silently wiped these settings, so the two
// are separate records now and neither writes the other's.
func TestAdminConfigTestKitchen_PUT_LeavesTheAnalysisToolsRecordAlone(t *testing.T) {
	store := newTestConfigStore(t)
	cfg := testConfig()
	cfg.AnalysisTools.CookstyleTimeoutMinutes = 20
	r := newTestRouterForAdminConfig(cfg, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(validTKBody))
	r.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)

	stored, err := store.Get(context.Background(), configstore.KeyTestKitchen)
	if err != nil {
		t.Fatalf("Test Kitchen has no record of its own: %v", err)
	}
	var storedMap map[string]any
	if err := json.Unmarshal(stored, &storedMap); err != nil {
		t.Fatalf("unmarshal stored: %v", err)
	}
	if storedMap["driver"] != "vcenter" {
		t.Errorf("stored driver = %v, want vcenter", storedMap["driver"])
	}

	// And it did not write the other screen's record at all.
	if _, err := store.Get(context.Background(), configstore.KeyAnalysisTools); err == nil {
		t.Error("saving Test Kitchen also wrote the analysis tools record, so the two can " +
			"still overwrite each other")
	}
}

func TestAdminConfigTestKitchen_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(validTKBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestAdminConfigTestKitchen_PUT_400_InvalidJSON(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader("{"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
}

func TestAdminConfigTestKitchen_PUT_422_NegativeTimeout(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"timeout_minutes": -1, "driver": "vcenter"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigTestKitchen_PUT_422_UnknownDriver(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"driver": "unknown-driver"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigTestKitchen_PUT_422_CustomDriverMissingImageField(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"driver": "custom"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigTestKitchen_DELETE_Success(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	// Pre-set the store using PUT to set driver to vcenter.
	putBody := `{"driver": "vcenter", "timeout_minutes": 30}`
	wPut := httptest.NewRecorder()
	reqPut := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(putBody))
	r.ServeHTTP(wPut, reqPut)
	assertStatus(t, wPut, http.StatusOK)

	// Now DELETE.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/config/test-kitchen", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodePutValue(t, w, &got)

	// The record is still there, cleared rather than removed: an absent record
	// and a deliberately emptied one would otherwise read the same.
	stored, err := store.Get(context.Background(), configstore.KeyTestKitchen)
	if err != nil {
		t.Fatalf("the Test Kitchen record is gone after DELETE rather than cleared: %v", err)
	}
	var storedMap map[string]any
	if err := json.Unmarshal(stored, &storedMap); err != nil {
		t.Fatalf("unmarshal stored: %v", err)
	}
	if driver := storedMap["driver"]; driver != nil && driver != "" {
		t.Errorf("driver after DELETE = %v, want empty", driver)
	}
}

func TestAdminConfigTestKitchen_DELETE_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/config/test-kitchen", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestAdminConfigTestKitchen_PUT_405_WrongMethod(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config/test-kitchen", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}
