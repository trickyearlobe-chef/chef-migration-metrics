// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTestRouterWithTrigger builds a Router backed by a mockStore, a default
// config, and an optional CollectionTriggerFunc.
func newTestRouterWithTrigger(store *mockStore, trigger CollectionTriggerFunc) *Router {
	cfg := testConfig()
	hub := NewEventHub()
	go hub.Run()
	opts := []RouterOption{}
	if trigger != nil {
		opts = append(opts, WithCollectionTrigger(trigger))
	}
	return NewRouter(store, cfg, hub, opts...)
}

// succeedingTrigger returns a CollectionTriggerFunc that always succeeds and
// records how many times it was called.
func succeedingTrigger(calls *atomic.Int32) CollectionTriggerFunc {
	return func(ctx context.Context) error {
		calls.Add(1)
		return nil
	}
}

// failingTrigger returns a CollectionTriggerFunc that always returns an error.
func failingTrigger() CollectionTriggerFunc {
	return func(ctx context.Context) error {
		return errors.New("a collection run is already in progress")
	}
}

// decodeRescanResponse decodes common rescan response fields.
type rescanResponse struct {
	Message             string `json:"message"`
	CollectionTriggered bool   `json:"collection_triggered"`
	CookbookName        string `json:"cookbook_name,omitempty"`
	VersionsInvalidated int    `json:"versions_invalidated,omitempty"`
	GitRepoName         string `json:"git_repo_name,omitempty"`
	ReposInvalidated    int    `json:"repos_invalidated,omitempty"`
}

func decodeRescanResponse(t *testing.T, rec *httptest.ResponseRecorder) rescanResponse {
	t.Helper()
	var resp rescanResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return resp
}

// noopDeleteFn returns a function that always succeeds for delete operations.
func noopDeleteFn() func(ctx context.Context) error {
	return func(ctx context.Context) error { return nil }
}

func noopDeleteByIDFn() func(ctx context.Context, id string) error {
	return func(ctx context.Context, id string) error { return nil }
}

// minimalRescanAllStore returns a mockStore with all the methods required by
// handleAdminRescanAllCookstyle stubbed out to succeed.
func minimalRescanAllStore() *mockStore {
	return &mockStore{
		DeleteAllServerCookbookCookstyleResultsFn:    noopDeleteFn(),
		DeleteAllGitRepoCookstyleResultsFn:           noopDeleteFn(),
		DeleteAllServerCookbookComplexitiesFn:        noopDeleteFn(),
		DeleteAllGitRepoComplexitiesFn:               noopDeleteFn(),
		DeleteAllServerCookbookAutocorrectPreviewsFn: noopDeleteFn(),
		DeleteAllGitRepoAutocorrectPreviewsFn:        noopDeleteFn(),
		ResetAllServerCookbookDownloadStatusesFn: func(ctx context.Context) (int, error) {
			return 5, nil
		},
	}
}

// minimalCookbookRescanStore returns a mockStore with all the methods required
// by handleCookbookRescan stubbed out to succeed for a cookbook named "apt".
func minimalCookbookRescanStore() *mockStore {
	return &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return []datastore.ServerCookbook{
				{ID: "sc-1", Name: name, Version: "1.0.0"},
				{ID: "sc-2", Name: name, Version: "2.0.0"},
			}, nil
		},
		DeleteServerCookbookCookstyleResultsByCookbookFn:    noopDeleteByIDFn(),
		DeleteServerCookbookComplexitiesByCookbookFn:        noopDeleteByIDFn(),
		DeleteServerCookbookAutocorrectPreviewsByCookbookFn: noopDeleteByIDFn(),
		ResetServerCookbookDownloadStatusFn: func(ctx context.Context, id string) (datastore.ServerCookbook, error) {
			return datastore.ServerCookbook{ID: id}, nil
		},
		ListGitReposByNameFn: func(ctx context.Context, name string) ([]datastore.GitRepo, error) {
			return nil, nil
		},
	}
}

// minimalGitRepoRescanStore returns a mockStore with all the methods required
// by handleGitRepoRescan stubbed out to succeed.
func minimalGitRepoRescanStore() *mockStore {
	return &mockStore{
		ListGitReposByNameFn: func(ctx context.Context, name string) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "gr-1", Name: name},
			}, nil
		},
		DeleteGitRepoCookstyleResultsByRepoFn:    noopDeleteByIDFn(),
		DeleteGitRepoComplexitiesByRepoFn:        noopDeleteByIDFn(),
		DeleteGitRepoAutocorrectPreviewsByRepoFn: noopDeleteByIDFn(),
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/admin/rescan-all-cookstyle
// ---------------------------------------------------------------------------

func TestHandleAdminRescanAll_MethodNotAllowed(t *testing.T) {
	r := newTestRouterWithTrigger(minimalRescanAllStore(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/rescan-all-cookstyle", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleAdminRescanAll_HappyPath_WithTrigger(t *testing.T) {
	var calls atomic.Int32
	store := minimalRescanAllStore()
	r := newTestRouterWithTrigger(store, succeedingTrigger(&calls))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/rescan-all-cookstyle", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeRescanResponse(t, rec)

	if !resp.CollectionTriggered {
		t.Error("collection_triggered = false, want true")
	}
	if calls.Load() != 1 {
		t.Errorf("trigger called %d times, want 1", calls.Load())
	}
	if resp.Message == "" {
		t.Error("message is empty")
	}
	// Message should indicate the run was triggered, not deferred.
	if !strings.Contains(resp.Message, "triggered") {
		t.Errorf("message %q should mention 'triggered'", resp.Message)
	}
}

func TestHandleAdminRescanAll_HappyPath_WithoutTrigger(t *testing.T) {
	store := minimalRescanAllStore()
	r := newTestRouterWithTrigger(store, nil) // no trigger configured

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/rescan-all-cookstyle", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeRescanResponse(t, rec)

	if resp.CollectionTriggered {
		t.Error("collection_triggered = true, want false (no trigger configured)")
	}
	if !strings.Contains(resp.Message, "next collection cycle") {
		t.Errorf("message %q should mention 'next collection cycle' when trigger is absent", resp.Message)
	}
}

func TestHandleAdminRescanAll_TriggerFails_StillReturns200(t *testing.T) {
	store := minimalRescanAllStore()
	r := newTestRouterWithTrigger(store, failingTrigger())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/rescan-all-cookstyle", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeRescanResponse(t, rec)

	// Invalidation succeeded even though trigger failed — collection_triggered
	// should be false and the message should fall back to "next collection cycle".
	if resp.CollectionTriggered {
		t.Error("collection_triggered = true, want false (trigger returned error)")
	}
	if !strings.Contains(resp.Message, "next collection cycle") {
		t.Errorf("message %q should mention 'next collection cycle' when trigger fails", resp.Message)
	}
}

func TestHandleAdminRescanAll_DeleteCookstyleResultsFails(t *testing.T) {
	store := minimalRescanAllStore()
	store.DeleteAllServerCookbookCookstyleResultsFn = func(ctx context.Context) error {
		return errors.New("db error")
	}
	r := newTestRouterWithTrigger(store, succeedingTrigger(&atomic.Int32{}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/rescan-all-cookstyle", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleAdminRescanAll_ResetDownloadStatusFails(t *testing.T) {
	store := minimalRescanAllStore()
	store.ResetAllServerCookbookDownloadStatusesFn = func(ctx context.Context) (int, error) {
		return 0, errors.New("db error")
	}
	r := newTestRouterWithTrigger(store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/rescan-all-cookstyle", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/cookbooks/:name/rescan
// ---------------------------------------------------------------------------

func TestHandleCookbookRescan_MethodNotAllowed(t *testing.T) {
	r := newTestRouterWithTrigger(minimalCookbookRescanStore(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cookbooks/apt/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCookbookRescan_HappyPath_WithTrigger(t *testing.T) {
	var calls atomic.Int32
	store := minimalCookbookRescanStore()
	r := newTestRouterWithTrigger(store, succeedingTrigger(&calls))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/apt/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeRescanResponse(t, rec)

	if resp.CookbookName != "apt" {
		t.Errorf("cookbook_name = %q, want %q", resp.CookbookName, "apt")
	}
	if resp.VersionsInvalidated != 2 {
		t.Errorf("versions_invalidated = %d, want 2", resp.VersionsInvalidated)
	}
	if !resp.CollectionTriggered {
		t.Error("collection_triggered = false, want true")
	}
	if calls.Load() != 1 {
		t.Errorf("trigger called %d times, want 1", calls.Load())
	}
	if !strings.Contains(resp.Message, "triggered") {
		t.Errorf("message %q should mention 'triggered'", resp.Message)
	}
}

func TestHandleCookbookRescan_HappyPath_WithoutTrigger(t *testing.T) {
	store := minimalCookbookRescanStore()
	r := newTestRouterWithTrigger(store, nil) // no trigger

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/apt/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeRescanResponse(t, rec)

	if resp.CollectionTriggered {
		t.Error("collection_triggered = true, want false")
	}
	if !strings.Contains(resp.Message, "next collection cycle") {
		t.Errorf("message %q should mention 'next collection cycle'", resp.Message)
	}
}

func TestHandleCookbookRescan_TriggerFails_StillReturns200(t *testing.T) {
	store := minimalCookbookRescanStore()
	r := newTestRouterWithTrigger(store, failingTrigger())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/apt/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeRescanResponse(t, rec)

	if resp.CollectionTriggered {
		t.Error("collection_triggered = true, want false")
	}
	if !strings.Contains(resp.Message, "next collection cycle") {
		t.Errorf("message %q should mention 'next collection cycle' when trigger fails", resp.Message)
	}
}

func TestHandleCookbookRescan_NotFound(t *testing.T) {
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil
		},
		ListGitReposByNameFn: func(ctx context.Context, name string) ([]datastore.GitRepo, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithTrigger(store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nonexistent/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCookbookRescan_ListServerCookbooksFails(t *testing.T) {
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, errors.New("db error")
		},
	}
	r := newTestRouterWithTrigger(store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/apt/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleCookbookRescan_WithGitRepos(t *testing.T) {
	var calls atomic.Int32
	store := &mockStore{
		ListServerCookbooksByNameFn: func(ctx context.Context, name string) ([]datastore.ServerCookbook, error) {
			return nil, nil // no server cookbooks
		},
		ListGitReposByNameFn: func(ctx context.Context, name string) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{ID: "gr-1", Name: name},
			}, nil
		},
		DeleteGitRepoCookstyleResultsByRepoFn:    noopDeleteByIDFn(),
		DeleteGitRepoComplexitiesByRepoFn:        noopDeleteByIDFn(),
		DeleteGitRepoAutocorrectPreviewsByRepoFn: noopDeleteByIDFn(),
	}
	r := newTestRouterWithTrigger(store, succeedingTrigger(&calls))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/my-cookbook/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeRescanResponse(t, rec)
	if resp.VersionsInvalidated != 1 {
		t.Errorf("versions_invalidated = %d, want 1", resp.VersionsInvalidated)
	}
	if !resp.CollectionTriggered {
		t.Error("collection_triggered = false, want true")
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/git-repos/:name/rescan
// ---------------------------------------------------------------------------

func TestHandleGitRepoRescan_MethodNotAllowed(t *testing.T) {
	r := newTestRouterWithTrigger(minimalGitRepoRescanStore(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/my-repo/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleGitRepoRescan_HappyPath_WithTrigger(t *testing.T) {
	var calls atomic.Int32
	store := minimalGitRepoRescanStore()
	r := newTestRouterWithTrigger(store, succeedingTrigger(&calls))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/git-repos/my-repo/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeRescanResponse(t, rec)

	if resp.GitRepoName != "my-repo" {
		t.Errorf("git_repo_name = %q, want %q", resp.GitRepoName, "my-repo")
	}
	if resp.ReposInvalidated != 1 {
		t.Errorf("repos_invalidated = %d, want 1", resp.ReposInvalidated)
	}
	if !resp.CollectionTriggered {
		t.Error("collection_triggered = false, want true")
	}
	if calls.Load() != 1 {
		t.Errorf("trigger called %d times, want 1", calls.Load())
	}
	if !strings.Contains(resp.Message, "triggered") {
		t.Errorf("message %q should mention 'triggered'", resp.Message)
	}
}

func TestHandleGitRepoRescan_HappyPath_WithoutTrigger(t *testing.T) {
	store := minimalGitRepoRescanStore()
	r := newTestRouterWithTrigger(store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/git-repos/my-repo/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeRescanResponse(t, rec)

	if resp.CollectionTriggered {
		t.Error("collection_triggered = true, want false")
	}
	if !strings.Contains(resp.Message, "next collection cycle") {
		t.Errorf("message %q should mention 'next collection cycle'", resp.Message)
	}
}

func TestHandleGitRepoRescan_TriggerFails_StillReturns200(t *testing.T) {
	store := minimalGitRepoRescanStore()
	r := newTestRouterWithTrigger(store, failingTrigger())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/git-repos/my-repo/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	resp := decodeRescanResponse(t, rec)

	if resp.CollectionTriggered {
		t.Error("collection_triggered = true, want false")
	}
}

func TestHandleGitRepoRescan_NotFound(t *testing.T) {
	store := &mockStore{
		ListGitReposByNameFn: func(ctx context.Context, name string) ([]datastore.GitRepo, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithTrigger(store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/git-repos/nonexistent/rescan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// triggerCollectionInBackground unit tests
// ---------------------------------------------------------------------------

func TestTriggerCollectionInBackground_NilTrigger(t *testing.T) {
	r := &Router{} // no trigger configured
	if r.triggerCollectionInBackground() {
		t.Error("expected false when triggerCollection is nil")
	}
}

func TestTriggerCollectionInBackground_Success(t *testing.T) {
	var calls atomic.Int32
	r := &Router{
		triggerCollection: succeedingTrigger(&calls),
	}
	if !r.triggerCollectionInBackground() {
		t.Error("expected true when trigger succeeds")
	}
	if calls.Load() != 1 {
		t.Errorf("trigger called %d times, want 1", calls.Load())
	}
}

func TestTriggerCollectionInBackground_Error(t *testing.T) {
	r := &Router{
		triggerCollection: failingTrigger(),
	}
	if r.triggerCollectionInBackground() {
		t.Error("expected false when trigger returns error")
	}
}
