// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// GET /api/v1/git-kitchen-results
// ---------------------------------------------------------------------------

func TestHandleGitKitchenResults_ListAll(t *testing.T) {
	store := &mockStore{
		ListGitKitchenResultsFn: func(_ context.Context) ([]datastore.GitKitchenResult, error) {
			return []datastore.GitKitchenResult{
				{ID: "r1", GitRepoName: "apache2", CreatedAt: time.Now()},
				{ID: "r2", GitRepoName: "nginx", CreatedAt: time.Now()},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-kitchen-results", nil)
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
}

func TestHandleGitKitchenResults_FilterByRepo(t *testing.T) {
	var calledWith string
	store := &mockStore{
		ListGitKitchenResultsByRepoFn: func(_ context.Context, gitRepoName string) ([]datastore.GitKitchenResult, error) {
			calledWith = gitRepoName
			return []datastore.GitKitchenResult{
				{ID: "r1", GitRepoName: "apache2", CreatedAt: time.Now()},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-kitchen-results?repo=apache2", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if calledWith != "apache2" {
		t.Errorf("ListGitKitchenResultsByRepo called with %q, want %q", calledWith, "apache2")
	}

	var results []datastore.GitKitchenResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	if results[0].GitRepoName != "apache2" {
		t.Errorf("git_repo_name = %q, want %q", results[0].GitRepoName, "apache2")
	}
}

func TestHandleGitKitchenResults_FilterByBatch(t *testing.T) {
	store := &mockStore{
		ListGitKitchenResultsByBatchFn: func(_ context.Context, batchID string) ([]datastore.GitKitchenResult, error) {
			if batchID != "some-uuid" {
				t.Errorf("unexpected batchID=%q", batchID)
			}
			return []datastore.GitKitchenResult{
				{ID: "r1", BatchID: "some-uuid", GitRepoName: "apache2", CreatedAt: time.Now()},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-kitchen-results?batch_id=some-uuid", nil)
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
}

func TestHandleGitKitchenResults_EmptyList(t *testing.T) {
	store := &mockStore{
		ListGitKitchenResultsFn: func(_ context.Context) ([]datastore.GitKitchenResult, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-kitchen-results", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	body := w.Body.String()
	// Must be an empty JSON array, not null.
	var results []datastore.GitKitchenResult
	if err := json.Unmarshal([]byte(body), &results); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if results == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(results) != 0 {
		t.Fatalf("len = %d, want 0", len(results))
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/git-kitchen-results/:id
// ---------------------------------------------------------------------------

func TestHandleGitKitchenResultDetail_Found(t *testing.T) {
	store := &mockStore{
		GetGitKitchenResultFn: func(_ context.Context, id string) (datastore.GitKitchenResult, error) {
			if id != "some-uuid" {
				t.Errorf("unexpected id=%q", id)
			}
			return datastore.GitKitchenResult{
				ID:          "some-uuid",
				GitRepoName: "apache2",
				CreatedAt:   time.Now(),
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-kitchen-results/some-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var result datastore.GitKitchenResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.ID != "some-uuid" {
		t.Errorf("id = %q, want %q", result.ID, "some-uuid")
	}
}

func TestHandleGitKitchenResultDetail_NotFound(t *testing.T) {
	store := &mockStore{
		GetGitKitchenResultFn: func(_ context.Context, _ string) (datastore.GitKitchenResult, error) {
			return datastore.GitKitchenResult{}, datastore.ErrNotFound
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-kitchen-results/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != ErrCodeNotFound {
		t.Errorf("error = %q, want %q", resp.Error, ErrCodeNotFound)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/batches/:id/results
// ---------------------------------------------------------------------------

func TestHandleBatchResults_Found(t *testing.T) {
	store := &mockStore{
		ListGitKitchenResultsByBatchFn: func(_ context.Context, batchID string) ([]datastore.GitKitchenResult, error) {
			if batchID != "some-uuid" {
				t.Errorf("unexpected batchID=%q", batchID)
			}
			return []datastore.GitKitchenResult{
				{ID: "r1", BatchID: "some-uuid", GitRepoName: "apache2", CreatedAt: time.Now()},
				{ID: "r2", BatchID: "some-uuid", GitRepoName: "nginx", CreatedAt: time.Now()},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/batches/some-uuid/results", nil)
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
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/batches/:id/progress
// ---------------------------------------------------------------------------

func TestHandleBatchProgress_Found(t *testing.T) {
	store := &mockStore{
		CountGitKitchenResultsByBatchFn: func(_ context.Context, batchID string) (int, int, int, int, int, error) {
			if batchID != "some-uuid" {
				t.Errorf("unexpected batchID=%q", batchID)
			}
			return 5, 2, 3, 1, 0, nil
		},
	}
	r := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/batches/some-uuid/progress", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var progress map[string]int
	if err := json.NewDecoder(w.Body).Decode(&progress); err != nil {
		t.Fatalf("decode: %v", err)
	}

	checks := map[string]int{
		"passed":    5,
		"failed":    2,
		"pending":   3,
		"timed_out": 1,
		"errored":   0,
		"total":     11,
	}
	for key, want := range checks {
		got, ok := progress[key]
		if !ok {
			t.Errorf("missing key %q in response", key)
			continue
		}
		if got != want {
			t.Errorf("%s = %d, want %d", key, got, want)
		}
	}
}
