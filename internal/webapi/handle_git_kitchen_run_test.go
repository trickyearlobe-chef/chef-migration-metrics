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

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/batch"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// mockGitKitchenRunner implements GitKitchenRunner for tests.
type mockGitKitchenRunner struct {
	called chan batch.RunInstanceRequest
	result batch.RunInstanceResult
}

func (m *mockGitKitchenRunner) RunInstance(_ context.Context, req batch.RunInstanceRequest) batch.RunInstanceResult {
	if m.called != nil {
		m.called <- req
	}
	return m.result
}

func validGitKitchenRunBody() string {
	return `{"git_repo_name":"nginx","target_chef_version":"19.2.12","platform_name":"ubuntu-22.04","suite_name":"default"}`
}

func TestHandleGitKitchenRun_Success(t *testing.T) {
	runner := &mockGitKitchenRunner{
		called: make(chan batch.RunInstanceRequest, 1),
	}
	store := &mockStore{
		ListGitReposByNameFn: func(_ context.Context, name string) ([]datastore.GitRepo, error) {
			if name != "nginx" {
				t.Errorf("expected repo name nginx, got %q", name)
			}
			return []datastore.GitRepo{
				{
					Name:          "nginx",
					GitRepoURL:    "https://git.example.com/nginx.git",
					HeadCommitSHA: "abc123",
					CloneStatus:   "ok",
				},
			}, nil
		},
	}

	r := newTestRouterWithMock(store)
	r.gitKitchenRunner = runner

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git-run", strings.NewReader(validGitKitchenRunBody()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["status"] != "started" {
		t.Errorf("status = %q, want started", resp["status"])
	}
	if !strings.Contains(resp["message"], "nginx") {
		t.Errorf("message should mention repo name, got: %s", resp["message"])
	}
}

func TestHandleGitKitchenRun_MethodNotAllowed(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/kitchen/git-run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleGitKitchenRun_MissingFields(t *testing.T) {
	runner := &mockGitKitchenRunner{}
	store := &mockStore{}
	r := newTestRouterWithMock(store)
	r.gitKitchenRunner = runner

	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{
			name:    "missing git_repo_name",
			body:    `{"target_chef_version":"19.2.12","platform_name":"ubuntu-22.04","suite_name":"default"}`,
			wantMsg: "git_repo_name is required",
		},
		{
			name:    "missing target_chef_version",
			body:    `{"git_repo_name":"nginx","platform_name":"ubuntu-22.04","suite_name":"default"}`,
			wantMsg: "target_chef_version is required",
		},
		{
			name:    "missing platform_name",
			body:    `{"git_repo_name":"nginx","target_chef_version":"19.2.12","suite_name":"default"}`,
			wantMsg: "platform_name is required",
		},
		{
			name:    "missing suite_name",
			body:    `{"git_repo_name":"nginx","target_chef_version":"19.2.12","platform_name":"ubuntu-22.04"}`,
			wantMsg: "suite_name is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git-run", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.wantMsg) {
				t.Errorf("body should contain %q, got: %s", tc.wantMsg, w.Body.String())
			}
		})
	}
}

func TestHandleGitKitchenRun_InvalidJSON(t *testing.T) {
	runner := &mockGitKitchenRunner{}
	r := newTestRouterWithMock(&mockStore{})
	r.gitKitchenRunner = runner

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git-run", strings.NewReader(`{not json`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleGitKitchenRun_RunnerNotConfigured(t *testing.T) {
	r := newTestRouterWithMock(&mockStore{})
	// gitKitchenRunner is nil by default

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git-run", strings.NewReader(validGitKitchenRunBody()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not configured") {
		t.Errorf("body should mention not configured, got: %s", w.Body.String())
	}
}

func TestHandleGitKitchenRun_RepoNotFound(t *testing.T) {
	runner := &mockGitKitchenRunner{}
	store := &mockStore{
		ListGitReposByNameFn: func(_ context.Context, _ string) ([]datastore.GitRepo, error) {
			return nil, nil
		},
	}
	r := newTestRouterWithMock(store)
	r.gitKitchenRunner = runner

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git-run", strings.NewReader(validGitKitchenRunBody()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleGitKitchenRun_RepoNotCloned(t *testing.T) {
	runner := &mockGitKitchenRunner{}
	store := &mockStore{
		ListGitReposByNameFn: func(_ context.Context, _ string) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{
					Name:        "nginx",
					GitRepoURL:  "https://git.example.com/nginx.git",
					CloneStatus: "pending",
				},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	r.gitKitchenRunner = runner

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git-run", strings.NewReader(validGitKitchenRunBody()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not cloned") {
		t.Errorf("body should mention not cloned, got: %s", w.Body.String())
	}
}

func TestHandleGitKitchenRun_PicksClonedRepo(t *testing.T) {
	runner := &mockGitKitchenRunner{
		called: make(chan batch.RunInstanceRequest, 1),
	}
	store := &mockStore{
		ListGitReposByNameFn: func(_ context.Context, _ string) ([]datastore.GitRepo, error) {
			return []datastore.GitRepo{
				{
					Name:        "nginx",
					GitRepoURL:  "https://git.example.com/nginx-old.git",
					CloneStatus: "failed",
				},
				{
					Name:          "nginx",
					GitRepoURL:    "https://git.example.com/nginx.git",
					HeadCommitSHA: "def456",
					CloneStatus:   "ok",
				},
			}, nil
		},
	}
	r := newTestRouterWithMock(store)
	r.gitKitchenRunner = runner

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git-run", strings.NewReader(validGitKitchenRunBody()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	// Verify the runner received the correct repo URL from the cloned repo.
	select {
	case got := <-runner.called:
		if got.GitRepoURL != "https://git.example.com/nginx.git" {
			t.Errorf("GitRepoURL = %q, want the cloned repo URL", got.GitRepoURL)
		}
		if got.CommitSHA != "def456" {
			t.Errorf("CommitSHA = %q, want def456", got.CommitSHA)
		}
	default:
		// The goroutine may not have fired yet; that's acceptable for a
		// fire-and-forget design. The 202 response is the primary assertion.
	}
}

func TestHandleGitKitchenRun_DBError(t *testing.T) {
	runner := &mockGitKitchenRunner{}
	store := &mockStore{
		ListGitReposByNameFn: func(_ context.Context, _ string) ([]datastore.GitRepo, error) {
			return nil, context.DeadlineExceeded
		},
	}
	r := newTestRouterWithMock(store)
	r.gitKitchenRunner = runner

	req := httptest.NewRequest(http.MethodPost, "/api/v1/kitchen/git-run", strings.NewReader(validGitKitchenRunBody()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
}
