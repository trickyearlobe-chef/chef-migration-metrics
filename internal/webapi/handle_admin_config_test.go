// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// fakeConfigStoreDB — in-memory DatastoreDB for admin config handler tests
// ---------------------------------------------------------------------------

type fakeConfigStoreDB struct {
	mu      sync.Mutex
	entries map[string]*datastore.ConfigEntry
}

func newFakeConfigStoreDB() *fakeConfigStoreDB {
	return &fakeConfigStoreDB{entries: make(map[string]*datastore.ConfigEntry)}
}

func (f *fakeConfigStoreDB) GetConfigEntry(_ context.Context, key string) (*datastore.ConfigEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[key]
	if !ok {
		return nil, nil
	}
	cp := *e
	cp.EncryptedValue = append([]byte(nil), e.EncryptedValue...)
	cp.Nonce = append([]byte(nil), e.Nonce...)
	return &cp, nil
}

func (f *fakeConfigStoreDB) SetConfigEntry(_ context.Context, e *datastore.ConfigEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *e
	cp.EncryptedValue = append([]byte(nil), e.EncryptedValue...)
	cp.Nonce = append([]byte(nil), e.Nonce...)
	cp.UpdatedAt = time.Now().UTC()
	f.entries[cp.Key] = &cp
	return nil
}

func (f *fakeConfigStoreDB) DeleteConfigEntry(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.entries, key)
	return nil
}

func (f *fakeConfigStoreDB) ListConfigEntries(_ context.Context) ([]datastore.ConfigEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]datastore.ConfigEntry, 0, len(f.entries))
	for _, e := range f.entries {
		cp := *e
		cp.EncryptedValue = append([]byte(nil), e.EncryptedValue...)
		cp.Nonce = append([]byte(nil), e.Nonce...)
		result = append(result, cp)
	}
	return result, nil
}

func (f *fakeConfigStoreDB) ListConfigEntriesByPrefix(_ context.Context, prefix string) ([]datastore.ConfigEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []datastore.ConfigEntry
	for k, e := range f.entries {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			cp := *e
			cp.EncryptedValue = append([]byte(nil), e.EncryptedValue...)
			cp.Nonce = append([]byte(nil), e.Nonce...)
			result = append(result, cp)
		}
	}
	return result, nil
}

func (f *fakeConfigStoreDB) CountConfigEntries(_ context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries), nil
}

func (f *fakeConfigStoreDB) ConfigStoreIsEmpty(_ context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries) == 0, nil
}

var _ configstore.DatastoreDB = (*fakeConfigStoreDB)(nil)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestConfigStore(t *testing.T) *configstore.Store {
	t.Helper()
	enc := testCredentialEncryptor(t)
	return configstore.NewStore(newFakeConfigStoreDB(), enc)
}

// newTestRouterForAdminConfig builds a Router with a config store and holder
// wired in. Pass nil store to get 503 on PUT. Pass nil holder to skip
// config reload after PUT (avoids validation failures on an incomplete store).
func newTestRouterForAdminConfig(cfg *config.Config, store *configstore.Store, holder *configstore.ConfigHolder) *Router {
	if cfg == nil {
		cfg = testConfig()
	}
	ms := &mockStore{}
	hub := NewEventHub()
	go hub.Run()
	return NewRouter(ms, cfg, hub, WithConfigStore(store, holder))
}

// decodeBody is a test helper that decodes a JSON response body into v.
func decodeBody(t *testing.T, r *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Fatalf("decode response body: %v\nbody: %s", err, r.Body.String())
	}
}

// assertStatus fails the test if the recorder's status code is not want.
func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status = %d, want %d; body: %s", w.Code, want, w.Body.String())
	}
}

// assertErrorCode fails the test if the response error code is not want.
func assertErrorCode(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	var resp ErrorResponse
	decodeBody(t, w, &resp)
	if resp.Error != want {
		t.Errorf("error code = %q, want %q", resp.Error, want)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/config/collection
// ---------------------------------------------------------------------------

func TestAdminConfigCollection_GET(t *testing.T) {
	cfg := testConfig()
	cfg.Collection = config.CollectionConfig{
		Schedule:                   "0 2 * * *",
		StaleNodeThresholdDays:     30,
		StaleCookbookThresholdDays: 90,
	}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/collection", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["schedule"] != "0 2 * * *" {
		t.Errorf("schedule = %v, want %q", got["schedule"], "0 2 * * *")
	}
	if got["stale_node_threshold_days"] != float64(30) {
		t.Errorf("stale_node_threshold_days = %v, want 30", got["stale_node_threshold_days"])
	}
	if got["stale_cookbook_threshold_days"] != float64(90) {
		t.Errorf("stale_cookbook_threshold_days = %v, want 90", got["stale_cookbook_threshold_days"])
	}
}

func TestAdminConfigCollection_GET_NilStore(t *testing.T) {
	cfg := testConfig()
	cfg.Collection = config.CollectionConfig{Schedule: "0 * * * *", StaleNodeThresholdDays: 7, StaleCookbookThresholdDays: 30}
	// GET must succeed even when configStore is nil.
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/collection", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigCollection_GET_UsesHolder(t *testing.T) {
	// When a ConfigHolder is wired in, liveConfig() should return the
	// holder's current config rather than the static router config.
	cfg := testConfig()
	cfg.Collection = config.CollectionConfig{Schedule: "0 3 * * *", StaleNodeThresholdDays: 14, StaleCookbookThresholdDays: 60}
	holder := configstore.NewConfigHolder(cfg, nil)
	r := newTestRouterForAdminConfig(testConfig(), nil, holder)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/collection", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["schedule"] != "0 3 * * *" {
		t.Errorf("schedule = %v, want %q", got["schedule"], "0 3 * * *")
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/config/collection
// ---------------------------------------------------------------------------

func TestAdminConfigCollection_PUT_Success(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"schedule":"0 2 * * *","stale_node_threshold_days":30,"stale_cookbook_threshold_days":90}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/collection", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["schedule"] != "0 2 * * *" {
		t.Errorf("schedule = %v, want %q", got["schedule"], "0 2 * * *")
	}
	if got["stale_node_threshold_days"] != float64(30) {
		t.Errorf("stale_node_threshold_days = %v, want 30", got["stale_node_threshold_days"])
	}

	// Verify value was persisted in the store.
	stored, err := store.Get(context.Background(), configstore.KeyCollection)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	var storedMap map[string]any
	if err := json.Unmarshal(stored, &storedMap); err != nil {
		t.Fatalf("unmarshal stored: %v", err)
	}
	if storedMap["schedule"] != "0 2 * * *" {
		t.Errorf("stored schedule = %v, want %q", storedMap["schedule"], "0 2 * * *")
	}
}

func TestAdminConfigCollection_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	body := `{"schedule":"0 2 * * *","stale_node_threshold_days":30,"stale_cookbook_threshold_days":90}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/collection", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestAdminConfigCollection_PUT_400_InvalidJSON(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/collection", strings.NewReader("{"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorCode(t, w, ErrCodeBadRequest)
}

func TestAdminConfigCollection_PUT_422_BadSchedule(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"schedule":"not-a-cron","stale_node_threshold_days":30,"stale_cookbook_threshold_days":90}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/collection", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigCollection_PUT_422_ZeroStaleNode(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"schedule":"0 2 * * *","stale_node_threshold_days":0,"stale_cookbook_threshold_days":90}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/collection", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigCollection_PUT_422_ZeroStaleCookbook(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"schedule":"0 2 * * *","stale_node_threshold_days":30,"stale_cookbook_threshold_days":0}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/collection", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigCollection_PUT_405_WrongMethod(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/config/collection", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}

func TestAdminConfigCollection_PUT_500_ReloadError(t *testing.T) {
	store := newTestConfigStore(t)
	// Holder with nil store: Reload() will always fail.
	holder := configstore.NewConfigHolder(testConfig(), nil)
	r := newTestRouterForAdminConfig(nil, store, holder)

	body := `{"schedule":"0 2 * * *","stale_node_threshold_days":30,"stale_cookbook_threshold_days":90}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/collection", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/config/target-versions
// ---------------------------------------------------------------------------

func TestAdminConfigTargetVersions_GET(t *testing.T) {
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"15.3.0", "17.10.0"}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/target-versions", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got []string
	decodeBody(t, w, &got)
	if len(got) != 2 || got[0] != "15.3.0" || got[1] != "17.10.0" {
		t.Errorf("versions = %v, want [15.3.0 17.10.0]", got)
	}
}

func TestAdminConfigTargetVersions_GET_NilStore(t *testing.T) {
	cfg := testConfig()
	cfg.TargetChefVersions = []string{"18.0.0"}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/target-versions", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/config/target-versions
// ---------------------------------------------------------------------------

func TestAdminConfigTargetVersions_PUT_Success(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `["15.3.0","17.10.0"]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/target-versions", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got []string
	decodeBody(t, w, &got)
	if len(got) != 2 || got[0] != "15.3.0" {
		t.Errorf("versions = %v, want [15.3.0 17.10.0]", got)
	}
}

func TestAdminConfigTargetVersions_PUT_EmptyList(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/target-versions", strings.NewReader("[]"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigTargetVersions_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/target-versions", strings.NewReader(`["15.3.0"]`))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestAdminConfigTargetVersions_PUT_400_InvalidJSON(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/target-versions", strings.NewReader("{"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
}

func TestAdminConfigTargetVersions_PUT_422_BadSemver(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `["15.3.0","not-a-version"]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/target-versions", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigTargetVersions_PUT_405_WrongMethod(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/config/target-versions", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/config/git-urls
// ---------------------------------------------------------------------------

func TestAdminConfigGitURLs_GET(t *testing.T) {
	cfg := testConfig()
	cfg.GitBaseURLs = []string{"https://github.com", "https://gitlab.com"}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/git-urls", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got []string
	decodeBody(t, w, &got)
	if len(got) != 2 || got[0] != "https://github.com" {
		t.Errorf("git_base_urls = %v", got)
	}
}

func TestAdminConfigGitURLs_GET_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/git-urls", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/config/git-urls
// ---------------------------------------------------------------------------

func TestAdminConfigGitURLs_PUT_Success(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `["https://github.com","https://gitlab.com"]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/git-urls", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got []string
	decodeBody(t, w, &got)
	if len(got) != 2 || got[0] != "https://github.com" {
		t.Errorf("git_base_urls = %v", got)
	}
}

func TestAdminConfigGitURLs_PUT_EmptyList(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/git-urls", strings.NewReader("[]"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigGitURLs_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/git-urls", strings.NewReader(`["https://github.com"]`))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestAdminConfigGitURLs_PUT_400_InvalidJSON(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/git-urls", strings.NewReader("{"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
}

func TestAdminConfigGitURLs_PUT_405_WrongMethod(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config/git-urls", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/config/concurrency
// ---------------------------------------------------------------------------

func TestAdminConfigConcurrency_GET(t *testing.T) {
	cfg := testConfig()
	cfg.Concurrency = config.ConcurrencyConfig{
		OrganisationCollection: 5,
		NodePageFetching:       10,
		GitPull:                8,
		CookbookDownload:       4,
		CookstyleScan:          6,
		TestKitchenRun:         2,
		ReadinessEvaluation:    20,
	}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/concurrency", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["organisation_collection"] != float64(5) {
		t.Errorf("organisation_collection = %v, want 5", got["organisation_collection"])
	}
	if got["node_page_fetching"] != float64(10) {
		t.Errorf("node_page_fetching = %v, want 10", got["node_page_fetching"])
	}
}

func TestAdminConfigConcurrency_GET_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/concurrency", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/config/concurrency
// ---------------------------------------------------------------------------

func TestAdminConfigConcurrency_PUT_Success(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"organisation_collection":5,"node_page_fetching":10,"git_pull":8,"cookbook_download":4,"cookstyle_scan":6,"test_kitchen_run":2,"readiness_evaluation":20}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/concurrency", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["organisation_collection"] != float64(5) {
		t.Errorf("organisation_collection = %v, want 5", got["organisation_collection"])
	}
}

func TestAdminConfigConcurrency_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	body := `{"organisation_collection":5,"node_page_fetching":10,"git_pull":8,"cookbook_download":4,"cookstyle_scan":6,"test_kitchen_run":2,"readiness_evaluation":20}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/concurrency", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestAdminConfigConcurrency_PUT_400_InvalidJSON(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/concurrency", strings.NewReader("{"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
}

func TestAdminConfigConcurrency_PUT_422_ZeroField(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	// organisation_collection is 0 — all fields must be >= 1.
	body := `{"organisation_collection":0,"node_page_fetching":10,"git_pull":8,"cookbook_download":4,"cookstyle_scan":6,"test_kitchen_run":2,"readiness_evaluation":20}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/concurrency", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigConcurrency_PUT_422_AllFieldsRequired(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	tests := []struct {
		name string
		body string
	}{
		{"zero node_page_fetching", `{"organisation_collection":5,"node_page_fetching":0,"git_pull":8,"cookbook_download":4,"cookstyle_scan":6,"test_kitchen_run":2,"readiness_evaluation":20}`},
		{"zero git_pull", `{"organisation_collection":5,"node_page_fetching":10,"git_pull":0,"cookbook_download":4,"cookstyle_scan":6,"test_kitchen_run":2,"readiness_evaluation":20}`},
		{"zero readiness_evaluation", `{"organisation_collection":5,"node_page_fetching":10,"git_pull":8,"cookbook_download":4,"cookstyle_scan":6,"test_kitchen_run":2,"readiness_evaluation":0}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/concurrency", strings.NewReader(tc.body))
			r.ServeHTTP(w, req)
			assertStatus(t, w, http.StatusUnprocessableEntity)
		})
	}
}

func TestAdminConfigConcurrency_PUT_405_WrongMethod(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/config/concurrency", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/config/logging
// ---------------------------------------------------------------------------

func TestAdminConfigLogging_GET(t *testing.T) {
	cfg := testConfig()
	cfg.Logging = config.LoggingConfig{Level: "WARN", RetentionDays: 30}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/logging", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["level"] != "WARN" {
		t.Errorf("level = %v, want WARN", got["level"])
	}
	if got["retention_days"] != float64(30) {
		t.Errorf("retention_days = %v, want 30", got["retention_days"])
	}
}

func TestAdminConfigLogging_GET_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/logging", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/config/logging
// ---------------------------------------------------------------------------

func TestAdminConfigLogging_PUT_Success(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `{"level":"DEBUG","retention_days":14}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/logging", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeBody(t, w, &got)
	if got["level"] != "DEBUG" {
		t.Errorf("level = %v, want DEBUG", got["level"])
	}
}

func TestAdminConfigLogging_PUT_CaseInsensitiveLevel(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	// Level check is case-insensitive: "info" should be accepted.
	body := `{"level":"info","retention_days":90}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/logging", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigLogging_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/logging", strings.NewReader(`{"level":"INFO","retention_days":90}`))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestAdminConfigLogging_PUT_400_InvalidJSON(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/logging", strings.NewReader("{"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
}

func TestAdminConfigLogging_PUT_422_BadLevel(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	tests := []string{"VERBOSE", "TRACE", "FATAL", "", "warning"}
	for _, lvl := range tests {
		t.Run(lvl, func(t *testing.T) {
			body := `{"level":"` + lvl + `","retention_days":90}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/logging", strings.NewReader(body))
			r.ServeHTTP(w, req)
			assertStatus(t, w, http.StatusUnprocessableEntity)
			assertErrorCode(t, w, ErrCodeValidationError)
		})
	}
}

func TestAdminConfigLogging_PUT_AllValidLevels(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	for _, lvl := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		t.Run(lvl, func(t *testing.T) {
			body := `{"level":"` + lvl + `","retention_days":90}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/logging", strings.NewReader(body))
			r.ServeHTTP(w, req)
			assertStatus(t, w, http.StatusOK)
		})
	}
}

func TestAdminConfigLogging_PUT_405_WrongMethod(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/config/logging", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/config/organisations
// ---------------------------------------------------------------------------

func TestAdminConfigOrganisations_GET(t *testing.T) {
	cfg := testConfig()
	cfg.Organisations = []config.Organisation{
		{
			Name:                "prod",
			ChefServerURL:       "https://chef.example.com",
			OrgName:             "prod",
			ClientName:          "client",
			ClientKeyCredential: "my-key",
		},
	}
	r := newTestRouterForAdminConfig(cfg, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/organisations", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got []map[string]any
	decodeBody(t, w, &got)
	if len(got) != 1 {
		t.Fatalf("expected 1 organisation, got %d", len(got))
	}
	if got[0]["name"] != "prod" {
		t.Errorf("name = %v, want %q", got[0]["name"], "prod")
	}
	if got[0]["chef_server_url"] != "https://chef.example.com" {
		t.Errorf("chef_server_url = %v, want %q", got[0]["chef_server_url"], "https://chef.example.com")
	}
}

func TestAdminConfigOrganisations_GET_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/organisations", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigOrganisations_GET_UsesHolder(t *testing.T) {
	cfg := testConfig()
	cfg.Organisations = []config.Organisation{
		{
			Name:                "holder-org",
			ChefServerURL:       "https://chef.example.com",
			OrgName:             "holder-org",
			ClientName:          "client",
			ClientKeyCredential: "my-key",
		},
	}
	holder := configstore.NewConfigHolder(cfg, nil)
	r := newTestRouterForAdminConfig(testConfig(), nil, holder)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config/organisations", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got []map[string]any
	decodeBody(t, w, &got)
	if len(got) != 1 {
		t.Fatalf("expected 1 organisation, got %d", len(got))
	}
	if got[0]["name"] != "holder-org" {
		t.Errorf("name = %v, want %q", got[0]["name"], "holder-org")
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v1/admin/config/organisations
// ---------------------------------------------------------------------------

const validOrgBody = `[{"name":"prod","chef_server_url":"https://chef.example.com","org_name":"prod","client_name":"client","client_key_credential":"my-key"}]`

func TestAdminConfigOrganisations_PUT_Success(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(validOrgBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got []map[string]any
	decodeBody(t, w, &got)
	if len(got) != 1 {
		t.Fatalf("expected 1 organisation, got %d", len(got))
	}
	if got[0]["name"] != "prod" {
		t.Errorf("name = %v, want %q", got[0]["name"], "prod")
	}

	// Verify value was persisted in the store.
	stored, err := store.Get(context.Background(), configstore.KeyOrganisations)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	var storedOrgs []map[string]any
	if err := json.Unmarshal(stored, &storedOrgs); err != nil {
		t.Fatalf("unmarshal stored: %v", err)
	}
	if len(storedOrgs) != 1 || storedOrgs[0]["name"] != "prod" {
		t.Errorf("stored orgs = %v, want [{name:prod ...}]", storedOrgs)
	}
}

func TestAdminConfigOrganisations_PUT_Success_SSLVerifyFalse(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `[{"name":"prod","chef_server_url":"https://chef.example.com","org_name":"prod","client_name":"client","client_key_credential":"my-key","ssl_verify":false}]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
	var got []map[string]any
	decodeBody(t, w, &got)
	if len(got) != 1 {
		t.Fatalf("expected 1 organisation, got %d", len(got))
	}
	if got[0]["ssl_verify"] != false {
		t.Errorf("ssl_verify = %v, want false", got[0]["ssl_verify"])
	}
}

func TestAdminConfigOrganisations_PUT_Success_SSLVerifyOmitted(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	// ssl_verify omitted — null pointer is valid.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(validOrgBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigOrganisations_PUT_Success_ClientKeyPath(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	f, err := os.CreateTemp(t.TempDir(), "*.pem")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()
	keyPath := filepath.ToSlash(f.Name())

	body := `[{"name":"prod","chef_server_url":"https://chef.example.com","org_name":"prod","client_name":"client","client_key_path":"` + keyPath + `"}]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusOK)
}

func TestAdminConfigOrganisations_PUT_503_NilStore(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(validOrgBody))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusServiceUnavailable)
	assertErrorCode(t, w, ErrCodeServiceUnavailable)
}

func TestAdminConfigOrganisations_PUT_400_InvalidJSON(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader("{"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorCode(t, w, ErrCodeBadRequest)
}

func TestAdminConfigOrganisations_PUT_422_EmptyList(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader("[]"))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigOrganisations_PUT_422_MissingName(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `[{"chef_server_url":"https://chef.example.com","org_name":"prod","client_name":"client","client_key_credential":"k"}]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigOrganisations_PUT_422_DuplicateName(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `[{"name":"prod","chef_server_url":"https://chef.example.com","org_name":"prod","client_name":"client","client_key_credential":"k"},{"name":"prod","chef_server_url":"https://chef2.example.com","org_name":"prod2","client_name":"client","client_key_credential":"k2"}]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigOrganisations_PUT_422_MissingChefServerURL(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `[{"name":"prod","org_name":"prod","client_name":"client","client_key_credential":"k"}]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigOrganisations_PUT_422_MissingOrgName(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `[{"name":"prod","chef_server_url":"https://chef.example.com","client_name":"client","client_key_credential":"k"}]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigOrganisations_PUT_422_MissingClientName(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `[{"name":"prod","chef_server_url":"https://chef.example.com","org_name":"prod","client_key_credential":"k"}]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigOrganisations_PUT_422_NoKeyField(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `[{"name":"prod","chef_server_url":"https://chef.example.com","org_name":"prod","client_name":"client"}]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigOrganisations_PUT_422_ClientKeyPathNotFound(t *testing.T) {
	store := newTestConfigStore(t)
	r := newTestRouterForAdminConfig(nil, store, nil)

	body := `[{"name":"prod","chef_server_url":"https://chef.example.com","org_name":"prod","client_name":"client","client_key_path":"/nonexistent/path/key.pem"}]`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config/organisations", strings.NewReader(body))
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusUnprocessableEntity)
	assertErrorCode(t, w, ErrCodeValidationError)
}

func TestAdminConfigOrganisations_PUT_405_WrongMethod(t *testing.T) {
	r := newTestRouterForAdminConfig(nil, nil, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/config/organisations", nil)
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}
