// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestRouterForTKConfig builds a Router backed by the given mockStore and
// a config with sensible Test Kitchen defaults for testing.
func newTestRouterForTKConfig(store *mockStore) *Router {
	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen = config.TestKitchenConfig{
		Driver:         "",
		TimeoutMinutes: 30,
	}
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(store, cfg, hub)
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return b
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/test-kitchen/config
// ---------------------------------------------------------------------------

func TestTestKitchenConfig_GET_FileDefault(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterForTKConfig(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/test-kitchen/config", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp testKitchenConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Source != "file" {
		t.Errorf("source = %q, want %q", resp.Source, "file")
	}
	if resp.Config.Driver != "" {
		t.Errorf("config.driver = %q, want %q", resp.Config.Driver, "")
	}
	if resp.UpdatedAt != nil {
		t.Errorf("updated_at should be nil for file source, got %v", resp.UpdatedAt)
	}
	if resp.UpdatedBy != "" {
		t.Errorf("updated_by should be empty for file source, got %q", resp.UpdatedBy)
	}
}

func TestTestKitchenConfig_GET_DatabaseOverride(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	storedCfg := config.TestKitchenConfig{
		Driver:         "vcenter",
		TimeoutMinutes: 60,
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-22.04", Image: "ubuntu-template"},
		},
	}
	storedJSON := mustMarshal(t, storedCfg)

	store := &mockStore{
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			if key == "test_kitchen" {
				return &datastore.RuntimeSetting{
					Key:       key,
					Value:     storedJSON,
					UpdatedAt: now,
					UpdatedBy: "admin-user",
				}, nil
			}
			return nil, nil
		},
	}
	r := newTestRouterForTKConfig(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/test-kitchen/config", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp testKitchenConfigResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Source != "database" {
		t.Errorf("source = %q, want %q", resp.Source, "database")
	}
	if resp.Config.Driver != "vcenter" {
		t.Errorf("config.driver = %q, want %q", resp.Config.Driver, "vcenter")
	}
	if resp.Config.TimeoutMinutes != 60 {
		t.Errorf("config.timeout_minutes = %d, want %d", resp.Config.TimeoutMinutes, 60)
	}
	if resp.UpdatedBy != "admin-user" {
		t.Errorf("updated_by = %q, want %q", resp.UpdatedBy, "admin-user")
	}
	if resp.UpdatedAt == nil {
		t.Fatal("updated_at should not be nil for database source")
	}
	if !resp.UpdatedAt.Equal(now) {
		t.Errorf("updated_at = %v, want %v", resp.UpdatedAt, now)
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/test-kitchen/config
// ---------------------------------------------------------------------------

func TestTestKitchenConfig_PUT_Valid(t *testing.T) {
	var savedKey string
	var savedValue json.RawMessage
	var savedBy string

	now := time.Now().UTC().Truncate(time.Second)

	store := &mockStore{
		SetRuntimeSettingFn: func(_ context.Context, key string, value json.RawMessage, updatedBy string) error {
			savedKey = key
			savedValue = value
			savedBy = updatedBy
			return nil
		},
		GetRuntimeSettingFn: func(_ context.Context, key string) (*datastore.RuntimeSetting, error) {
			if savedValue != nil && key == "test_kitchen" {
				return &datastore.RuntimeSetting{
					Key:       key,
					Value:     savedValue,
					UpdatedAt: now,
					UpdatedBy: savedBy,
				}, nil
			}
			return nil, nil
		},
	}
	r := newTestRouterForTKConfig(store)

	body := `{"driver":"ec2","timeout_minutes":45,"platform_map":[{"kitchen_name":"ubuntu-22.04","image":"ami-12345"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/test-kitchen/config", bytes.NewBufferString(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	if savedKey != "test_kitchen" {
		t.Errorf("saved key = %q, want %q", savedKey, "test_kitchen")
	}

	// Verify the saved value round-trips.
	var savedCfg config.TestKitchenConfig
	if err := json.Unmarshal(savedValue, &savedCfg); err != nil {
		t.Fatalf("unmarshal saved: %v", err)
	}
	if savedCfg.Driver != "ec2" {
		t.Errorf("saved driver = %q, want %q", savedCfg.Driver, "ec2")
	}

	// Verify response contains config and source.
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["source"] != "database" {
		t.Errorf("source = %v, want %q", resp["source"], "database")
	}
}

func TestTestKitchenConfig_PUT_InvalidJSON(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterForTKConfig(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/test-kitchen/config", bytes.NewBufferString("{invalid"))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != ErrCodeBadRequest {
		t.Errorf("error = %q, want %q", resp.Error, ErrCodeBadRequest)
	}
}

func TestTestKitchenConfig_PUT_ValidationError_MissingPlatformMap(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterForTKConfig(store)

	// vcenter (non-dokken) requires a platform map.
	body := `{"driver":"vcenter","timeout_minutes":30}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/test-kitchen/config", bytes.NewBufferString(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] != "validation_failed" {
		t.Errorf("error = %v, want %q", resp["error"], "validation_failed")
	}
	details, ok := resp["details"].([]any)
	if !ok || len(details) == 0 {
		t.Fatal("expected non-empty details array")
	}
}

func TestTestKitchenConfig_PUT_ValidationError_MissingImage(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterForTKConfig(store)

	body := `{"driver":"ec2","platform_map":[{"kitchen_name":"ubuntu-22.04","image":""},{"kitchen_name":"centos-7","image":"ami-67890"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/test-kitchen/config", bytes.NewBufferString(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	details, ok := resp["details"].([]any)
	if !ok {
		t.Fatal("expected details array")
	}
	found := false
	for _, d := range details {
		if s, ok := d.(string); ok && s == "platform_map[0]: image is required (or set skip to true)" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'image is required (or set skip to true)' in details, got %v", details)
	}
}

func TestTestKitchenConfig_PUT_ValidationError_DuplicateKitchenName(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterForTKConfig(store)

	body := `{"driver":"ec2","platform_map":[{"kitchen_name":"ubuntu-22.04","image":"ami-111"},{"kitchen_name":"ubuntu-22.04","image":"ami-222"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/test-kitchen/config", bytes.NewBufferString(body))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	details, ok := resp["details"].([]any)
	if !ok {
		t.Fatal("expected details array")
	}
	found := false
	for _, d := range details {
		if s, ok := d.(string); ok && s == `platform_map[1]: duplicate kitchen_name "ubuntu-22.04"` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'duplicate kitchen_name' in details, got %v", details)
	}
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/admin/test-kitchen/config
// ---------------------------------------------------------------------------

func TestTestKitchenConfig_DELETE_RequiresConfirm(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterForTKConfig(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/test-kitchen/config", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != ErrCodeBadRequest {
		t.Errorf("error = %q, want %q", resp.Error, ErrCodeBadRequest)
	}
}

func TestTestKitchenConfig_DELETE_Success(t *testing.T) {
	deleted := false
	store := &mockStore{
		DeleteRuntimeSettingFn: func(_ context.Context, key string) error {
			if key == "test_kitchen" {
				deleted = true
			}
			return nil
		},
	}
	r := newTestRouterForTKConfig(store)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/test-kitchen/config?confirm=true", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
	if !deleted {
		t.Error("expected DeleteRuntimeSetting to be called with key 'test_kitchen'")
	}
}

// ---------------------------------------------------------------------------
// Method not allowed
// ---------------------------------------------------------------------------

func TestTestKitchenConfig_MethodNotAllowed(t *testing.T) {
	store := &mockStore{}
	r := newTestRouterForTKConfig(store)

	methods := []string{http.MethodPost, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/api/v1/admin/test-kitchen/config", nil)
			r.ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusMethodNotAllowed, w.Body.String())
			}

			var resp ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Error != ErrCodeMethodNotAllowed {
				t.Errorf("error = %q, want %q", resp.Error, ErrCodeMethodNotAllowed)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// validateTestKitchenConfig unit tests
// ---------------------------------------------------------------------------

func TestValidateTestKitchenConfig_Valid(t *testing.T) {
	cfg := config.TestKitchenConfig{
		Driver:         "ec2",
		TimeoutMinutes: 30,
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-22.04", Image: "ami-12345"},
			{KitchenName: "centos-7", Image: "ami-67890"},
		},
	}
	problems := validateTestKitchenConfig(cfg)
	if len(problems) != 0 {
		t.Errorf("expected no problems, got %v", problems)
	}
}

func TestValidateTestKitchenConfig_MissingPlatformMap(t *testing.T) {
	cfg := config.TestKitchenConfig{
		Driver: "vcenter",
	}
	problems := validateTestKitchenConfig(cfg)
	if len(problems) != 1 {
		t.Fatalf("expected 1 problem, got %d: %v", len(problems), problems)
	}
}

func TestValidateTestKitchenConfig_MissingKitchenName(t *testing.T) {
	cfg := config.TestKitchenConfig{
		Driver: "ec2",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "", Image: "ami-12345"},
		},
	}
	problems := validateTestKitchenConfig(cfg)
	found := false
	for _, p := range problems {
		if p == "platform_map[0]: kitchen_name is required" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'kitchen_name is required', got %v", problems)
	}
}

func TestValidateTestKitchenConfig_MissingImage(t *testing.T) {
	cfg := config.TestKitchenConfig{
		Driver: "ec2",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-22.04", Image: ""},
		},
	}
	problems := validateTestKitchenConfig(cfg)
	found := false
	for _, p := range problems {
		if p == "platform_map[0]: image is required (or set skip to true)" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'image is required (or set skip to true)', got %v", problems)
	}
}

func TestValidateTestKitchenConfig_SkipAllowsEmptyImage(t *testing.T) {
	cfg := config.TestKitchenConfig{
		Driver: "ec2",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-22.04", Image: "", Skip: true},
		},
	}
	problems := validateTestKitchenConfig(cfg)
	if len(problems) != 0 {
		t.Errorf("expected no problems for skip entry with empty image, got %v", problems)
	}
}

func TestValidateTestKitchenConfig_PatternRequiresWildcard(t *testing.T) {
	cfg := config.TestKitchenConfig{
		Driver: "ec2",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "rhel7", Image: "rhel-tmpl", IsPattern: true},
		},
	}
	problems := validateTestKitchenConfig(cfg)
	found := false
	for _, p := range problems {
		if p == `platform_map[0]: is_pattern is true but kitchen_name "rhel7" contains no wildcards (* or ?)` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected wildcard warning, got %v", problems)
	}
}

func TestValidateTestKitchenConfig_PatternWithWildcardValid(t *testing.T) {
	cfg := config.TestKitchenConfig{
		Driver: "ec2",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "rhel*", Image: "rhel-tmpl", IsPattern: true},
		},
	}
	problems := validateTestKitchenConfig(cfg)
	if len(problems) != 0 {
		t.Errorf("expected no problems, got %v", problems)
	}
}

func TestValidateTestKitchenConfig_PatternNoDuplicateCheck(t *testing.T) {
	// Two pattern entries with the same name should not trigger duplicate check.
	cfg := config.TestKitchenConfig{
		Driver: "ec2",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "rhel*", Image: "rhel-tmpl", IsPattern: true},
			{KitchenName: "rhel*", Image: "rhel-tmpl-2", IsPattern: true},
		},
	}
	problems := validateTestKitchenConfig(cfg)
	for _, p := range problems {
		if p == `platform_map[1]: duplicate kitchen_name "rhel*"` {
			t.Errorf("pattern entries should not trigger duplicate check, got %v", problems)
		}
	}
}

func TestValidateTestKitchenConfig_DuplicateKitchenName(t *testing.T) {
	cfg := config.TestKitchenConfig{
		Driver: "ec2",
		PlatformMap: []config.PlatformMapEntry{
			{KitchenName: "ubuntu-22.04", Image: "ami-111"},
			{KitchenName: "ubuntu-22.04", Image: "ami-222"},
		},
	}
	problems := validateTestKitchenConfig(cfg)
	found := false
	for _, p := range problems {
		if p == `platform_map[1]: duplicate kitchen_name "ubuntu-22.04"` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'duplicate kitchen_name', got %v", problems)
	}
}
