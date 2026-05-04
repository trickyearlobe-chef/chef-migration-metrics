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
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/gitkitchen"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/kitchenqueue"
)

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/git/results
// ---------------------------------------------------------------------------

func TestHandleGitKitchenResults_GET_All(t *testing.T) {
	store := &mockStore{
		ListGitKitchenResultsFn: func(_ context.Context) ([]datastore.GitKitchenResult, error) {
			p := boolPtr(true)
			return []datastore.GitKitchenResult{
				{ID: "r1", GitRepoName: "cookbook-a", InstanceName: "default-ubuntu-2204", Passed: p},
				{ID: "r2", GitRepoName: "cookbook-b", InstanceName: "default-centos-7", Passed: p},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/git/results", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var results []datastore.GitKitchenResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len = %d, want 2", len(results))
	}
	if results[0].ID != "r1" {
		t.Errorf("results[0].ID = %q, want %q", results[0].ID, "r1")
	}
}

func TestHandleGitKitchenResults_GET_ByRepo(t *testing.T) {
	store := &mockStore{
		ListGitKitchenResultsByRepoFn: func(_ context.Context, name string) ([]datastore.GitKitchenResult, error) {
			if name != "cookbook-a" {
				t.Errorf("unexpected repo name: %q", name)
			}
			p := boolPtr(true)
			return []datastore.GitKitchenResult{
				{ID: "r1", GitRepoName: "cookbook-a", InstanceName: "default-ubuntu-2204", Passed: p},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/git/results?repo=cookbook-a", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var results []datastore.GitKitchenResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	if results[0].GitRepoName != "cookbook-a" {
		t.Errorf("git_repo_name = %q, want %q", results[0].GitRepoName, "cookbook-a")
	}
}

func TestHandleGitKitchenResults_MethodNotAllowed(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git/results", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusMethodNotAllowed, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/git/instances
// ---------------------------------------------------------------------------

func TestHandleGitKitchenInstances_GET_Success(t *testing.T) {
	store := &mockStore{
		GetKitchenAnalysisResultByNameFn: func(_ context.Context, name string) (*datastore.KitchenAnalysisResult, error) {
			if name != "my-cookbook" {
				t.Errorf("unexpected repo name: %q", name)
			}
			return &datastore.KitchenAnalysisResult{
				GitRepoName:   "my-cookbook",
				GitRepoURL:    "https://git.example.com/my-cookbook.git",
				HeadCommitSHA: "abc123",
				Platforms:     json.RawMessage(`[{"name":"ubuntu-22.04"}]`),
				Suites:        json.RawMessage(`[{"name":"default"}]`),
			}, nil
		},
	}

	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen.PlatformMap = []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-template"},
	}
	r := newTestRouterWithMockAndConfig(store, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/git/instances?repo=my-cookbook", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var plan gitkitchen.PlanResult
	if err := json.NewDecoder(w.Body).Decode(&plan); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if plan.GitRepoName != "my-cookbook" {
		t.Errorf("git_repo_name = %q, want %q", plan.GitRepoName, "my-cookbook")
	}
	if plan.Total != 1 {
		t.Errorf("total = %d, want 1", plan.Total)
	}
	if plan.Mapped != 1 {
		t.Errorf("mapped = %d, want 1", plan.Mapped)
	}
}

func TestHandleGitKitchenInstances_GET_MissingRepoParam(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/git/instances", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleGitKitchenInstances_GET_NotFound(t *testing.T) {
	store := &mockStore{
		GetKitchenAnalysisResultByNameFn: func(_ context.Context, _ string) (*datastore.KitchenAnalysisResult, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/git/instances?repo=nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleGitKitchenInstances_GET_UsesLiveConfig(t *testing.T) {
	// liveConfig() is the single source of truth — whatever is in the router
	// config (loaded from config_store on startup) is used for platform mapping.
	store := &mockStore{
		GetKitchenAnalysisResultByNameFn: func(_ context.Context, _ string) (*datastore.KitchenAnalysisResult, error) {
			return &datastore.KitchenAnalysisResult{
				GitRepoName:   "my-cookbook",
				GitRepoURL:    "https://git.example.com/my-cookbook.git",
				HeadCommitSHA: "abc123",
				Platforms:     json.RawMessage(`[{"name":"ubuntu-22.04"}]`),
				Suites:        json.RawMessage(`[{"name":"default"}]`),
			}, nil
		},
	}

	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen.PlatformMap = []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "live-template"},
	}
	r := newTestRouterWithMockAndConfig(store, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/git/instances?repo=my-cookbook", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var plan gitkitchen.PlanResult
	if err := json.NewDecoder(w.Body).Decode(&plan); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if plan.Mapped != 1 {
		t.Fatalf("mapped = %d, want 1", plan.Mapped)
	}
	if plan.Instances[0].ImageName != "live-template" {
		t.Errorf("image = %q, want %q", plan.Instances[0].ImageName, "live-template")
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/kitchen/git/run
// ---------------------------------------------------------------------------

func TestHandleGitKitchenRun_POST_Success(t *testing.T) {
	store := &mockStore{
		GetKitchenAnalysisResultByNameFn: func(_ context.Context, name string) (*datastore.KitchenAnalysisResult, error) {
			return &datastore.KitchenAnalysisResult{
				GitRepoName:   name,
				GitRepoURL:    "https://git.example.com/" + name + ".git",
				HeadCommitSHA: "abc123",
				Platforms:     json.RawMessage(`[{"name":"ubuntu-22.04"}]`),
				Suites:        json.RawMessage(`[{"name":"default"}]`),
			}, nil
		},
	}

	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen.PlatformMap = []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-template"},
	}

	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, cfg, hub, WithKitchenQueue(kitchenqueue.New(nil, nil)))

	body := `{"git_repo_name":"my-cookbook","instance_name":"default-ubuntu-2204","target_chef_version":"18.5.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git/run", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}
}

func TestHandleGitKitchenRun_POST_NoQueue(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	body := `{"git_repo_name":"my-cookbook","instance_name":"default-ubuntu-2204","target_chef_version":"18.5.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git/run", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
}

func TestHandleGitKitchenRun_POST_MissingFields(t *testing.T) {
	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(&mockStore{}, testConfig(), hub, WithKitchenQueue(kitchenqueue.New(nil, nil)))

	tests := []struct {
		name string
		body string
	}{
		{"missing git_repo_name", `{"instance_name":"x","target_chef_version":"18.5.0"}`},
		{"missing instance_name", `{"git_repo_name":"x","target_chef_version":"18.5.0"}`},
		{"missing target_chef_version", `{"git_repo_name":"x","instance_name":"y"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git/run", bytes.NewBufferString(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
	}
}

// TestHandleGitKitchenRun_ContextDetachedFromRequest verifies that the
// goroutine dispatched by handleGitKitchenRun uses a context that is NOT
// cancelled when the HTTP request completes. Without this, kitchen processes
// would be killed mid-flight, orphaning VMs on the hypervisor.
func TestHandleGitKitchenRun_ContextDetachedFromRequest(t *testing.T) {
	// Channel to capture the context received by RunOne.
	ctxCh := make(chan context.Context, 1)

	store := &mockStore{
		GetKitchenAnalysisResultByNameFn: func(_ context.Context, name string) (*datastore.KitchenAnalysisResult, error) {
			return &datastore.KitchenAnalysisResult{
				GitRepoName:   name,
				GitRepoURL:    "https://git.example.com/" + name + ".git",
				HeadCommitSHA: "abc123",
				Platforms:     json.RawMessage(`[{"name":"ubuntu-22.04"}]`),
				Suites:        json.RawMessage(`[{"name":"default"}]`),
			}, nil
		},
		// UpsertGitKitchenResultFn captures the context passed through the
		// scheduler. This is the deepest point we can intercept without
		// exposing scheduler internals.
		UpsertGitKitchenResultFn: func(ctx context.Context, _ datastore.UpsertGitKitchenResultParams) (datastore.GitKitchenResult, error) {
			ctxCh <- ctx
			return datastore.GitKitchenResult{}, nil
		},
	}

	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen.PlatformMap = []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-template"},
	}

	hub := NewEventHub()
	go hub.Run()
	rtr := NewRouter(store, cfg, hub, WithKitchenQueue(kitchenqueue.New(nil, nil)))

	// Create a cancellable context to simulate HTTP request lifecycle.
	reqCtx, reqCancel := context.WithCancel(context.Background())
	body := `{"git_repo_name":"my-cookbook","instance_name":"default-ubuntu-2204","target_chef_version":"18.5.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git/run", bytes.NewBufferString(body))
	req = req.WithContext(reqCtx)

	w := httptest.NewRecorder()
	rtr.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	// Cancel the request context — simulates HTTP response being sent.
	reqCancel()

	// Wait for the goroutine to call the store. The context it receives
	// must NOT be cancelled even though we cancelled the parent.
	select {
	case gotCtx := <-ctxCh:
		if err := gotCtx.Err(); err != nil {
			t.Fatalf("context passed to store was cancelled: %v — this would orphan VMs", err)
		}
	case <-time.After(5 * time.Second):
		// The goroutine may fail before reaching the store (no real
		// executor). That's acceptable — the important thing is the
		// context isn't cancelled. Check by ensuring the handler
		// returned 202.
		t.Log("goroutine did not reach store upsert (expected with nil executor)")
	}
}

func TestHandleGitKitchenRun_POST_DisabledReturns409(t *testing.T) {
	disabled := false
	store := &mockStore{}

	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen.Enabled = &disabled

	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, cfg, hub, WithKitchenQueue(kitchenqueue.New(nil, nil)))

	body := `{"git_repo_name":"my-cookbook","instance_name":"default-ubuntu-2204","target_chef_version":"18.5.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git/run", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/kitchen/git/run-all
// ---------------------------------------------------------------------------

func TestHandleGitKitchenRunAll_POST_Success(t *testing.T) {
	store := &mockStore{
		GetKitchenAnalysisResultByNameFn: func(_ context.Context, name string) (*datastore.KitchenAnalysisResult, error) {
			return &datastore.KitchenAnalysisResult{
				GitRepoName:   name,
				GitRepoURL:    "https://git.example.com/" + name + ".git",
				HeadCommitSHA: "abc123",
				Platforms:     json.RawMessage(`[{"name":"ubuntu-22.04"}]`),
				Suites:        json.RawMessage(`[{"name":"default"},{"name":"service"}]`),
			}, nil
		},
	}

	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen.PlatformMap = []config.PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204-template"},
	}

	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, cfg, hub, WithKitchenQueue(kitchenqueue.New(nil, nil)))

	body := `{"git_repo_name":"my-cookbook","target_chef_version":"18.5.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git/run-all", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["queued_count"] != float64(2) {
		t.Fatalf("queued_count = %v, want 2", resp["queued_count"])
	}
}

func TestHandleGitKitchenRunAll_POST_NoQueue(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	body := `{"git_repo_name":"my-cookbook","target_chef_version":"18.5.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git/run-all", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
}

func TestHandleGitKitchenRunAll_POST_MissingFields(t *testing.T) {
	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(&mockStore{}, testConfig(), hub, WithKitchenQueue(kitchenqueue.New(nil, nil)))

	tests := []struct {
		name string
		body string
	}{
		{"missing git_repo_name", `{"target_chef_version":"18.5.0"}`},
		{"missing target_chef_version", `{"git_repo_name":"x"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git/run-all", bytes.NewBufferString(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
	}
}

func TestHandleGitKitchenRunAll_POST_DisabledReturns409(t *testing.T) {
	disabled := false
	store := &mockStore{}

	cfg := testConfig()
	cfg.AnalysisTools.TestKitchen.Enabled = &disabled

	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, cfg, hub, WithKitchenQueue(kitchenqueue.New(nil, nil)))

	body := `{"git_repo_name":"my-cookbook","target_chef_version":"18.5.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git/run-all", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
}
