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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/gitkitchen"
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

	// Create a scheduler with a no-op runner for testing.
	sched := gitkitchen.NewScheduler(nil, nil, nil, cfg.AnalysisTools.TestKitchen,
		func(name, url string) string { return "/repos/" + name })

	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(store, cfg, hub, WithGitKitchenScheduler(sched))

	body := `{"git_repo_name":"my-cookbook","instance_name":"default-ubuntu-2204","target_chef_version":"18.5.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git/run", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}
}

func TestHandleGitKitchenRun_POST_NoScheduler(t *testing.T) {
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
	sched := gitkitchen.NewScheduler(nil, nil, nil, config.TestKitchenConfig{},
		func(name, url string) string { return "/repos/" + name })

	hub := NewEventHub()
	go hub.Run()
	r := NewRouter(&mockStore{}, testConfig(), hub, WithGitKitchenScheduler(sched))

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
