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
// GET /api/v1/kitchen/queue
// ---------------------------------------------------------------------------

func TestHandleKitchenQueueList_ReturnsItems(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	items := []datastore.KitchenQueueItem{
		{ID: "item-1", RunType: "git", GitRepoName: "repo-a", Status: "queued", EnqueuedAt: now},
		{ID: "item-2", RunType: "node", NodeName: "web-1", Status: "running", EnqueuedAt: now},
	}

	store := &mockStore{
		ListKitchenQueueFn: func(_ context.Context, _ datastore.KitchenQueueFilter) ([]datastore.KitchenQueueItem, error) {
			return items, nil
		},
		GetKitchenQueueStatsFn: func(_ context.Context) (*datastore.KitchenQueueStats, error) {
			return &datastore.KitchenQueueStats{Queued: 3, Running: 1}, nil
		},
	}

	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/queue", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Items []datastore.KitchenQueueItem `json:"items"`
		Stats struct {
			Queued        int `json:"queued"`
			Running       int `json:"running"`
			WorkersActive int `json:"workers_active"`
		} `json:"stats"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items count = %d, want 2", len(resp.Items))
	}
	if resp.Items[0].ID != "item-1" {
		t.Errorf("items[0].ID = %q, want %q", resp.Items[0].ID, "item-1")
	}
	if resp.Items[1].ID != "item-2" {
		t.Errorf("items[1].ID = %q, want %q", resp.Items[1].ID, "item-2")
	}
	if resp.Stats.Queued != 3 {
		t.Errorf("stats.queued = %d, want 3", resp.Stats.Queued)
	}
	if resp.Stats.Running != 1 {
		t.Errorf("stats.running = %d, want 1", resp.Stats.Running)
	}
}

func TestHandleKitchenQueueList_FiltersPassedToStore(t *testing.T) {
	var captured datastore.KitchenQueueFilter

	store := &mockStore{
		ListKitchenQueueFn: func(_ context.Context, f datastore.KitchenQueueFilter) ([]datastore.KitchenQueueItem, error) {
			captured = f
			return []datastore.KitchenQueueItem{}, nil
		},
	}

	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/queue?repo=my-repo&type=git&status=queued,running", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if captured.RepoName != "my-repo" {
		t.Errorf("filter.RepoName = %q, want %q", captured.RepoName, "my-repo")
	}
	if captured.RunType != "git" {
		t.Errorf("filter.RunType = %q, want %q", captured.RunType, "git")
	}
	if len(captured.Statuses) != 2 {
		t.Fatalf("filter.Statuses length = %d, want 2", len(captured.Statuses))
	}
	if captured.Statuses[0] != "queued" {
		t.Errorf("filter.Statuses[0] = %q, want %q", captured.Statuses[0], "queued")
	}
	if captured.Statuses[1] != "running" {
		t.Errorf("filter.Statuses[1] = %q, want %q", captured.Statuses[1], "running")
	}
}

func TestHandleKitchenQueueList_EmptyReturnsEmptyArray(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/queue", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Items == nil {
		t.Fatal("items is null, want empty array")
	}
	if len(resp.Items) != 0 {
		t.Errorf("items count = %d, want 0", len(resp.Items))
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/queue/{id}
// ---------------------------------------------------------------------------

func TestHandleKitchenQueueGet_ReturnsItem(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	store := &mockStore{
		GetKitchenQueueItemFn: func(_ context.Context, id string) (*datastore.KitchenQueueItem, error) {
			return &datastore.KitchenQueueItem{
				ID:         id,
				RunType:    "git",
				GitRepoName: "example-repo",
				Status:     "running",
				EnqueuedAt: now,
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/queue/abc-123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var item datastore.KitchenQueueItem
	if err := json.NewDecoder(w.Body).Decode(&item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item.ID != "abc-123" {
		t.Errorf("id = %q, want %q", item.ID, "abc-123")
	}
	if item.Status != "running" {
		t.Errorf("status = %q, want %q", item.Status, "running")
	}
}

func TestHandleKitchenQueueGet_NotFound(t *testing.T) {
	store := &mockStore{
		GetKitchenQueueItemFn: func(_ context.Context, _ string) (*datastore.KitchenQueueItem, error) {
			return nil, datastore.ErrNotFound
		},
	}

	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/queue/nonexistent", nil)
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

func TestHandleKitchenQueueGet_MissingID(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	// Request to base path with trailing slash routes to handleKitchenQueueRouting
	// with an empty ID segment.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/queue/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/kitchen/queue/{id}/cancel
// ---------------------------------------------------------------------------

func TestHandleKitchenQueueCancel_Success(t *testing.T) {
	var cancelledID string
	store := &mockStore{
		CancelKitchenRunFn: func(_ context.Context, id string) error {
			cancelledID = id
			return nil
		},
	}

	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/queue/item-99/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if cancelledID != "item-99" {
		t.Errorf("cancelled ID = %q, want %q", cancelledID, "item-99")
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["id"] != "item-99" {
		t.Errorf("response id = %q, want %q", resp["id"], "item-99")
	}
}

func TestHandleKitchenQueueCancel_NotFound(t *testing.T) {
	store := &mockStore{
		CancelKitchenRunFn: func(_ context.Context, _ string) error {
			return datastore.ErrNotFound
		},
	}

	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/queue/gone-id/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleKitchenQueueCancel_WrongMethod(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/queue/item-1/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusMethodNotAllowed, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/kitchen/queue/{id}/retry
// ---------------------------------------------------------------------------

func TestHandleKitchenQueueRetry_Success(t *testing.T) {
	store := &mockStore{
		RetryKitchenRunFn: func(_ context.Context, _ string) (*datastore.KitchenQueueItem, error) {
			return &datastore.KitchenQueueItem{ID: "new-item-id", Status: "queued"}, nil
		},
	}

	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/queue/old-item/retry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp struct {
		Message string `json:"message"`
		QueueID string `json:"queue_id"`
		Status  string `json:"status"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.QueueID != "new-item-id" {
		t.Errorf("queue_id = %q, want %q", resp.QueueID, "new-item-id")
	}
	if resp.Status != "queued" {
		t.Errorf("status = %q, want %q", resp.Status, "queued")
	}
}

func TestHandleKitchenQueueRetry_NotFound(t *testing.T) {
	store := &mockStore{
		RetryKitchenRunFn: func(_ context.Context, _ string) (*datastore.KitchenQueueItem, error) {
			return nil, datastore.ErrNotFound
		},
	}

	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/queue/missing/retry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleKitchenQueueRetry_AlreadyExists(t *testing.T) {
	store := &mockStore{
		RetryKitchenRunFn: func(_ context.Context, _ string) (*datastore.KitchenQueueItem, error) {
			return nil, datastore.ErrAlreadyExists
		},
	}

	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/queue/dup-item/retry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["message"] == "" {
		t.Error("expected non-empty message in conflict response")
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/queue/stats
// ---------------------------------------------------------------------------

func TestHandleKitchenQueueStats_ReturnsStats(t *testing.T) {
	store := &mockStore{
		GetKitchenQueueStatsFn: func(_ context.Context) (*datastore.KitchenQueueStats, error) {
			return &datastore.KitchenQueueStats{Queued: 5, Running: 2}, nil
		},
	}

	r := newTestRouterWithMock(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/queue/stats", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Queued        int `json:"queued"`
		Running       int `json:"running"`
		WorkersActive int `json:"workers_active"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Queued != 5 {
		t.Errorf("queued = %d, want 5", resp.Queued)
	}
	if resp.Running != 2 {
		t.Errorf("running = %d, want 2", resp.Running)
	}
	if resp.WorkersActive != 0 {
		t.Errorf("workers_active = %d, want 0 (no kitchenQueue manager set)", resp.WorkersActive)
	}
}
