// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newFileTestRouter(gitDir string) *Router {
	cfg := testConfig()
	cfg.Storage.GitCookbookDir = gitDir
	hub := NewEventHub()
	go hub.Run()
	store := &mockStore{}
	return NewRouter(store, cfg, hub)
}

func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()

	repoDir := filepath.Join(tmpDir, "test-cookbook")
	if err := os.MkdirAll(filepath.Join(repoDir, "recipes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "test", "integration"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Text file
	if err := os.WriteFile(filepath.Join(repoDir, "metadata.rb"), []byte("name 'test-cookbook'\nversion '1.0.0'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// File in subdirectory
	if err := os.WriteFile(filepath.Join(repoDir, "recipes", "default.rb"), []byte("log 'hello world'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Binary file
	binData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	if err := os.WriteFile(filepath.Join(repoDir, "icon.png"), binData, 0o644); err != nil {
		t.Fatal(err)
	}
	// File in .git directory (should not be accessible)
	if err := os.WriteFile(filepath.Join(repoDir, ".git", "config"), []byte("[core]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return tmpDir, func() {}
}

// ---------------------------------------------------------------------------
// Tests: File tree listing
// ---------------------------------------------------------------------------

func TestHandleGitRepoFileTree_RootListing(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/test-cookbook/files", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Size int64  `json:"size"`
	}
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Should have: icon.png, metadata.rb, recipes, test — but NOT .git
	nameSet := make(map[string]bool)
	for _, e := range entries {
		nameSet[e.Name] = true
	}

	if nameSet[".git"] {
		t.Error(".git should not appear in listing")
	}
	if !nameSet["metadata.rb"] {
		t.Error("metadata.rb should appear in listing")
	}
	if !nameSet["recipes"] {
		t.Error("recipes directory should appear in listing")
	}
}

func TestHandleGitRepoFileTree_Subdirectory(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/test-cookbook/files?path=recipes", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(entries) != 1 || entries[0].Name != "default.rb" {
		t.Errorf("expected [default.rb], got %+v", entries)
	}
}

func TestHandleGitRepoFileTree_PathTraversal(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/test-cookbook/files?path=../../etc", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for traversal, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGitRepoFileTree_DotGitAccess(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/test-cookbook/files?path=.git", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for .git access, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGitRepoFileTree_InvalidRepoName(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/../files", nil)
	r.ServeHTTP(w, req)

	// Should be caught by name validation — either 400 or 404.
	if w.Code == http.StatusOK {
		t.Errorf("expected error for traversal via repo name, got 200")
	}
}

func TestHandleGitRepoFileTree_MissingClone(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/nonexistent/files", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing clone, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Tests: File content
// ---------------------------------------------------------------------------

func TestHandleGitRepoFileContent_TextFile(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/test-cookbook/files/content?path=metadata.rb", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Path     string `json:"path"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
		Size     int    `json:"size"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Encoding != "text" {
		t.Errorf("expected text encoding, got %q", resp.Encoding)
	}
	if resp.Content != "name 'test-cookbook'\nversion '1.0.0'\n" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

func TestHandleGitRepoFileContent_BinaryFile(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/test-cookbook/files/content?path=icon.png", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Path     string `json:"path"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
		Size     int    `json:"size"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Encoding != "base64" {
		t.Errorf("expected base64 encoding, got %q", resp.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(resp.Content)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if len(decoded) != 10 {
		t.Errorf("expected 10 bytes, got %d", len(decoded))
	}
}

func TestHandleGitRepoFileContent_PathTraversal(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/test-cookbook/files/content?path=../../etc/passwd", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for path traversal, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGitRepoFileContent_DotGitFile(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/test-cookbook/files/content?path=.git/config", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for .git access, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGitRepoFileContent_NoPath(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/test-cookbook/files/content", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing path, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGitRepoFileContent_SizeLimit(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create a file larger than 1MB
	repoDir := filepath.Join(tmpDir, "test-cookbook")
	bigData := make([]byte, maxFileContentSize+100)
	for i := range bigData {
		bigData[i] = 'A'
	}
	if err := os.WriteFile(filepath.Join(repoDir, "huge.txt"), bigData, 0o644); err != nil {
		t.Fatal(err)
	}

	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/test-cookbook/files/content?path=huge.txt", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 for large file, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGitRepoFileContent_SubdirFile(t *testing.T) {
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()
	r := newFileTestRouter(tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/git-repos/test-cookbook/files/content?path=recipes/default.rb", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Content != "log 'hello world'\n" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

// ---------------------------------------------------------------------------
// Tests: isBinary helper
// ---------------------------------------------------------------------------

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		binary bool
	}{
		{"empty", []byte{}, false},
		{"text", []byte("hello world"), false},
		{"null byte", []byte("hel\x00lo"), true},
		{"binary header", []byte{0x89, 0x50, 0x4E, 0x47, 0x00}, true},
		{"long text", []byte("a long text file without any null bytes at all"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isBinary(tc.data)
			if got != tc.binary {
				t.Errorf("isBinary(%q) = %v, want %v", tc.name, got, tc.binary)
			}
		})
	}
}
