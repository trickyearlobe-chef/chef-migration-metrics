// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

func TestHandleCookbookResetGit_Success(t *testing.T) {
	store := &mockStore{
		DeleteGitCookbooksByNameFn: func(ctx context.Context, cookbookName string) (datastore.DeleteGitCookbookResult, error) {
			if cookbookName != "cron" {
				t.Errorf("DeleteGitCookbooksByName called with %q, want %q", cookbookName, "cron")
			}
			return datastore.DeleteGitCookbookResult{
				CookbooksDeleted:  1,
				CommittersDeleted: 5,
				RepoURLs:          []string{"git@github.com:old-org/cron"},
			}, nil
		},
	}

	router := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/cron/reset-git", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp["cookbook_name"] != "cron" {
		t.Errorf("cookbook_name = %v, want %q", resp["cookbook_name"], "cron")
	}
	if resp["cookbooks_deleted"] != float64(1) {
		t.Errorf("cookbooks_deleted = %v, want 1", resp["cookbooks_deleted"])
	}
	if resp["committers_deleted"] != float64(5) {
		t.Errorf("committers_deleted = %v, want 5", resp["committers_deleted"])
	}

	repoURLs, ok := resp["repo_urls_removed"].([]any)
	if !ok {
		t.Fatalf("repo_urls_removed is not an array: %T", resp["repo_urls_removed"])
	}
	if len(repoURLs) != 1 || repoURLs[0] != "git@github.com:old-org/cron" {
		t.Errorf("repo_urls_removed = %v, want [\"git@github.com:old-org/cron\"]", repoURLs)
	}

	if resp["message"] == nil || resp["message"] == "" {
		t.Error("expected non-empty message")
	}
}

func TestHandleCookbookResetGit_SuccessNilRepoURLs(t *testing.T) {
	store := &mockStore{
		DeleteGitCookbooksByNameFn: func(ctx context.Context, cookbookName string) (datastore.DeleteGitCookbookResult, error) {
			return datastore.DeleteGitCookbookResult{
				CookbooksDeleted:  2,
				CommittersDeleted: 0,
				RepoURLs:          nil, // nil should be serialised as []
			}, nil
		},
	}

	router := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/apt/reset-git", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	// nil RepoURLs should be returned as an empty JSON array, not null.
	repoURLs, ok := resp["repo_urls_removed"].([]any)
	if !ok {
		t.Fatalf("repo_urls_removed should be a JSON array, got %T (%v)", resp["repo_urls_removed"], resp["repo_urls_removed"])
	}
	if len(repoURLs) != 0 {
		t.Errorf("repo_urls_removed length = %d, want 0", len(repoURLs))
	}
}

func TestHandleCookbookResetGit_NotFound(t *testing.T) {
	store := &mockStore{
		DeleteGitCookbooksByNameFn: func(ctx context.Context, cookbookName string) (datastore.DeleteGitCookbookResult, error) {
			return datastore.DeleteGitCookbookResult{}, datastore.ErrNotFound
		},
	}

	router := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/nonexistent/reset-git", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}

func TestHandleCookbookResetGit_MethodNotAllowed(t *testing.T) {
	store := &mockStore{}
	router := newTestRouterWithMock(store)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/cookbooks/cron/reset-git", nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s status = %d, want %d; body = %s", method, rr.Code, http.StatusMethodNotAllowed, rr.Body.String())
			}
		})
	}
}

func TestHandleCookbookResetGit_InternalError(t *testing.T) {
	store := &mockStore{
		DeleteGitCookbooksByNameFn: func(ctx context.Context, cookbookName string) (datastore.DeleteGitCookbookResult, error) {
			return datastore.DeleteGitCookbookResult{}, fmt.Errorf("db connection lost")
		},
	}

	router := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/cron/reset-git", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body = %s", rr.Code, http.StatusInternalServerError, rr.Body.String())
	}
}

func TestHandleCookbookResetGit_CookbookNamePassedCorrectly(t *testing.T) {
	const wantName = "my-special-cookbook"
	var gotName string

	store := &mockStore{
		DeleteGitCookbooksByNameFn: func(ctx context.Context, cookbookName string) (datastore.DeleteGitCookbookResult, error) {
			gotName = cookbookName
			return datastore.DeleteGitCookbookResult{
				CookbooksDeleted: 1,
			}, nil
		},
	}

	router := newTestRouterWithMock(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cookbooks/"+wantName+"/reset-git", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	if gotName != wantName {
		t.Errorf("DeleteGitCookbooksByName received name %q, want %q", gotName, wantName)
	}
}
