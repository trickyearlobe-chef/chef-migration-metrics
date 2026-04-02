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

const validAnalysisToolsBody = `{
	"cookstyle_timeout_minutes": 10,
	"test_kitchen_timeout_minutes": 0,
	"test_kitchen": {"timeout_minutes": 30, "driver": "dokken"}
}`

const validTKBody = `{"timeout_minutes": 30, "driver": "dokken"}`

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/analysis-tools
// ---------------------------------------------------------------------------

func TestAdminConfigAnalysisTools_GET(t *testing.T) {
	cfg := testConfig()
	cfg.AnalysisTools = config.AnalysisToolsConfig{
		CookstyleTimeoutMinutes: 10,
		TestKitchen:             config.TestKitchenConfig{Driver: "dokken"},
	}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/analysis-tools", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["cookstyle_timeout_minutes"] != float64(10) {
		t.Errorf("cookstyle_timeout_minutes = %v, want 10", got["cookstyle_timeout_minutes"])
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
	if got["cookstyle_timeout_minutes"] != float64(15) {
		t.Errorf("cookstyle_timeout_minutes = %v, want 15", got["cookstyle_timeout_minutes"])
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
	decodeBody(t, w, &got)
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

	body := `{"cookstyle_timeout_minutes": 0, "test_kitchen": {"driver": "dokken"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/analysis-tools", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigAnalysisTools_PUT_422_NegativeTKTimeout(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"cookstyle_timeout_minutes": 10, "test_kitchen": {"timeout_minutes": -1, "driver": "dokken"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/analysis-tools", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigAnalysisTools_PUT_422_UnknownDriver(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"cookstyle_timeout_minutes": 10, "test_kitchen": {"driver": "unknown-driver"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/analysis-tools", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigAnalysisTools_PUT_422_CustomDriverMissingImageField(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"cookstyle_timeout_minutes": 10, "test_kitchen": {"driver": "custom"}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/analysis-tools", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigAnalysisTools_PUT_422_PlatformMapMissingKitchenName(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"cookstyle_timeout_minutes": 10, "test_kitchen": {"driver": "dokken", "platform_map": [{"image": "ubuntu"}]}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/analysis-tools", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigAnalysisTools_PUT_422_PlatformMapDuplicateName(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"cookstyle_timeout_minutes": 10, "test_kitchen": {"driver": "dokken", "platform_map": [{"kitchen_name": "ubuntu-22.04", "image": "ubuntu"}, {"kitchen_name": "ubuntu-22.04", "image": "ubuntu2"}]}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/analysis-tools", strings.NewReader(body))
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
	decodeBody(t, w, &got)
	if got["driver"] != "dokken" {
		t.Errorf("driver = %v, want dokken", got["driver"])
	}

	if _, err := store.Get(context.Background(), configstore.KeyAnalysisTools); err != nil {
		t.Fatalf("KeyAnalysisTools not found in store: %v", err)
	}
}

func TestAdminConfigTestKitchen_PUT_MergesWithExistingAnalysisTools(t *testing.T) {
	store := newTestConfigStore(t)
	cfg := testConfig()
	cfg.AnalysisTools.CookstyleTimeoutMinutes = 20
	r := newTestRouterForAdminConfig(cfg, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/test-kitchen", strings.NewReader(validTKBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)

	stored, err := store.Get(context.Background(), configstore.KeyAnalysisTools)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	var storedMap map[string]any
	if err := json.Unmarshal(stored, &storedMap); err != nil {
		t.Fatalf("unmarshal stored: %v", err)
	}
	if storedMap["cookstyle_timeout_minutes"] != float64(20) {
		t.Errorf("stored cookstyle_timeout_minutes = %v, want 20 (merge not preserved)", storedMap["cookstyle_timeout_minutes"])
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

	body := `{"timeout_minutes": -1, "driver": "dokken"}`
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
	decodeBody(t, w, &got)

	// Verify KeyAnalysisTools entry still exists in store.
	stored, err := store.Get(context.Background(), configstore.KeyAnalysisTools)
	if err != nil {
		t.Fatalf("KeyAnalysisTools not in store after DELETE: %v", err)
	}

	// Verify TK driver is empty or "dokken" (config has been zeroed).
	var storedMap map[string]any
	if err := json.Unmarshal(stored, &storedMap); err != nil {
		t.Fatalf("unmarshal stored: %v", err)
	}
	if tkSection, ok := storedMap["test_kitchen"].(map[string]any); ok {
		if driver := tkSection["driver"]; driver != nil && driver != "" && driver != "dokken" {
			t.Errorf("driver after DELETE = %v, want empty or dokken", driver)
		}
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
