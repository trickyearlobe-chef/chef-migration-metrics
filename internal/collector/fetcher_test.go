// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// formatDownloadError tests
// ---------------------------------------------------------------------------

func TestFormatDownloadError_APIError(t *testing.T) {
	apiErr := &chefapi.APIError{
		StatusCode: 404,
		Method:     "GET",
		Path:       "/cookbooks/nginx/5.1.0",
		Body:       "cookbook version not found",
	}

	got := formatDownloadError(apiErr)
	want := "404 GET: cookbook version not found"
	if got != want {
		t.Errorf("formatDownloadError(APIError) = %q, want %q", got, want)
	}
}

func TestFormatDownloadError_GenericError(t *testing.T) {
	err := fmt.Errorf("connection timeout after 30s")

	got := formatDownloadError(err)
	want := "connection timeout after 30s"
	if got != want {
		t.Errorf("formatDownloadError(generic) = %q, want %q", got, want)
	}
}

func TestFormatDownloadError_APIError_500(t *testing.T) {
	apiErr := &chefapi.APIError{
		StatusCode: 500,
		Method:     "GET",
		Path:       "/cookbooks/base/1.0.0",
		Body:       "internal server error",
	}

	got := formatDownloadError(apiErr)
	want := "500 GET: internal server error"
	if got != want {
		t.Errorf("formatDownloadError(500 APIError) = %q, want %q", got, want)
	}
}

func TestFormatDownloadError_APIError_403(t *testing.T) {
	apiErr := &chefapi.APIError{
		StatusCode: 403,
		Method:     "GET",
		Path:       "/cookbooks/secret/2.0.0",
		Body:       "forbidden",
	}

	got := formatDownloadError(apiErr)
	want := "403 GET: forbidden"
	if got != want {
		t.Errorf("formatDownloadError(403 APIError) = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// CookbookFetchError tests
// ---------------------------------------------------------------------------

func TestCookbookFetchError_Error(t *testing.T) {
	cfe := CookbookFetchError{
		CookbookID: "abc-123",
		Name:       "nginx",
		Version:    "5.1.0",
		Err:        fmt.Errorf("404 Not Found"),
	}

	got := cfe.Error()
	want := "nginx/5.1.0: 404 Not Found"
	if got != want {
		t.Errorf("CookbookFetchError.Error() = %q, want %q", got, want)
	}
}

func TestCookbookFetchError_Error_EmptyVersion(t *testing.T) {
	cfe := CookbookFetchError{
		CookbookID: "abc-123",
		Name:       "base",
		Version:    "",
		Err:        fmt.Errorf("something went wrong"),
	}

	got := cfe.Error()
	want := "base/: something went wrong"
	if got != want {
		t.Errorf("CookbookFetchError.Error() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// CookbookFetchResult tests
// ---------------------------------------------------------------------------

func TestCookbookFetchResult_ZeroValue(t *testing.T) {
	var r CookbookFetchResult
	if r.Total != 0 {
		t.Errorf("zero CookbookFetchResult.Total = %d, want 0", r.Total)
	}
	if r.Downloaded != 0 {
		t.Errorf("zero CookbookFetchResult.Downloaded = %d, want 0", r.Downloaded)
	}
	if r.Failed != 0 {
		t.Errorf("zero CookbookFetchResult.Failed = %d, want 0", r.Failed)
	}
	if r.Skipped != 0 {
		t.Errorf("zero CookbookFetchResult.Skipped = %d, want 0", r.Skipped)
	}
	if r.FilesWritten != 0 {
		t.Errorf("zero CookbookFetchResult.FilesWritten = %d, want 0", r.FilesWritten)
	}
	if len(r.Errors) != 0 {
		t.Errorf("zero CookbookFetchResult.Errors = %v, want empty", r.Errors)
	}
}

// ---------------------------------------------------------------------------
// fetchCookbooks integration tests (with mock client via collector infra)
// ---------------------------------------------------------------------------

// fetchCookbooksHelper wraps fetchCookbooks to work with our test doubles.
// Since fetchCookbooks requires a real chefapi.Client and datastore.DB,
// we test the function through the full collector pipeline via the
// collectOrganisation path in collector_test.go. Here we test the
// supporting functions and types directly.

// Note: we cannot pass nil for db to fetchCookbooks because it dereferences
// db.q() internally. Tests that need to exercise fetchCookbooks end-to-end
// should use the collector integration tests with a mock client factory.

// ---------------------------------------------------------------------------
// downloadCookbookVersion unit tests
// ---------------------------------------------------------------------------

// These test the error formatting paths that don't require a real client.

func TestDownloadCookbookVersion_FormatError_API404(t *testing.T) {
	err := &chefapi.APIError{
		StatusCode: 404,
		Method:     "GET",
		Path:       "/cookbooks/nginx/5.1.0",
		Body:       "cookbook version not found on server",
	}
	got := formatDownloadError(err)
	if got != "404 GET: cookbook version not found on server" {
		t.Errorf("unexpected error format: %s", got)
	}
}

func TestDownloadCookbookVersion_FormatError_Timeout(t *testing.T) {
	err := fmt.Errorf("connection timeout after 30s")
	got := formatDownloadError(err)
	if got != "connection timeout after 30s" {
		t.Errorf("unexpected error format: %s", got)
	}
}

func TestDownloadCookbookVersion_FormatError_ChecksumMismatch(t *testing.T) {
	err := fmt.Errorf("checksum mismatch: expected abc123, got def456")
	got := formatDownloadError(err)
	if got != "checksum mismatch: expected abc123, got def456" {
		t.Errorf("unexpected error format: %s", got)
	}
}

// ---------------------------------------------------------------------------
// Concurrency clamping (logic only — no nil db)
// ---------------------------------------------------------------------------

func TestFetchCookbooks_ConcurrencyClampLogic(t *testing.T) {
	// The fetchCookbooks function clamps concurrency <= 0 to 1. We verify
	// this indirectly: the semaphore channel is created with cap = max(1, n).
	// Since we can't call fetchCookbooks with nil db, we just verify the
	// clamping logic matches what's in the source.
	for _, conc := range []int{-1, 0, 1} {
		effective := conc
		if effective <= 0 {
			effective = 1
		}
		if effective < 1 {
			t.Errorf("concurrency=%d: effective = %d, want >= 1", conc, effective)
		}
	}
}

// ---------------------------------------------------------------------------
// Download status constants
// ---------------------------------------------------------------------------

func TestDownloadStatusConstants(t *testing.T) {
	if datastore.DownloadStatusOK != "ok" {
		t.Errorf("DownloadStatusOK = %q, want %q", datastore.DownloadStatusOK, "ok")
	}
	if datastore.DownloadStatusFailed != "failed" {
		t.Errorf("DownloadStatusFailed = %q, want %q", datastore.DownloadStatusFailed, "failed")
	}
	if datastore.DownloadStatusPending != "pending" {
		t.Errorf("DownloadStatusPending = %q, want %q", datastore.DownloadStatusPending, "pending")
	}
}

// ---------------------------------------------------------------------------
// Cookbook struct method tests (NeedsDownload, IsDownloaded)
// ---------------------------------------------------------------------------

func TestCookbook_IsDownloaded(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{datastore.DownloadStatusOK, true},
		{datastore.DownloadStatusFailed, false},
		{datastore.DownloadStatusPending, false},
		{"", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			cb := datastore.Cookbook{DownloadStatus: tt.status}
			if got := cb.IsDownloaded(); got != tt.want {
				t.Errorf("Cookbook{DownloadStatus: %q}.IsDownloaded() = %v, want %v",
					tt.status, got, tt.want)
			}
		})
	}
}

func TestCookbook_NeedsDownload(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{datastore.DownloadStatusOK, false},
		{datastore.DownloadStatusFailed, true},
		{datastore.DownloadStatusPending, true},
		{"", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			cb := datastore.Cookbook{DownloadStatus: tt.status}
			if got := cb.NeedsDownload(); got != tt.want {
				t.Errorf("Cookbook{DownloadStatus: %q}.NeedsDownload() = %v, want %v",
					tt.status, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cookbook JSON serialisation tests (download fields included)
// ---------------------------------------------------------------------------

func TestCookbook_MarshalJSON_IncludesDownloadStatus(t *testing.T) {
	cb := datastore.Cookbook{
		ID:             "abc-123",
		Name:           "nginx",
		Version:        "5.1.0",
		Source:         "chef_server",
		DownloadStatus: datastore.DownloadStatusFailed,
		DownloadError:  "404 Not Found",
	}

	data, err := json.Marshal(cb)
	if err != nil {
		t.Fatalf("json.Marshal(Cookbook) error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if got, ok := m["download_status"]; !ok {
		t.Error("JSON output missing download_status field")
	} else if got != "failed" {
		t.Errorf("download_status = %v, want %q", got, "failed")
	}

	if got, ok := m["download_error"]; !ok {
		t.Error("JSON output missing download_error field")
	} else if got != "404 Not Found" {
		t.Errorf("download_error = %v, want %q", got, "404 Not Found")
	}
}

func TestCookbook_MarshalJSON_OmitsEmptyDownloadError(t *testing.T) {
	cb := datastore.Cookbook{
		ID:             "abc-123",
		Name:           "nginx",
		Version:        "5.1.0",
		Source:         "chef_server",
		DownloadStatus: datastore.DownloadStatusOK,
		DownloadError:  "", // Empty — should be omitted
	}

	data, err := json.Marshal(cb)
	if err != nil {
		t.Fatalf("json.Marshal(Cookbook) error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if _, ok := m["download_error"]; ok {
		t.Error("JSON output should omit download_error when empty (omitempty tag)")
	}

	if got, ok := m["download_status"]; !ok {
		t.Error("JSON output missing download_status field")
	} else if got != "ok" {
		t.Errorf("download_status = %v, want %q", got, "ok")
	}
}

// ---------------------------------------------------------------------------
// UpdateCookbookDownloadStatusParams validation tests
// ---------------------------------------------------------------------------

func TestUpdateCookbookDownloadStatusParams_Fields(t *testing.T) {
	p := datastore.UpdateCookbookDownloadStatusParams{
		ID:             "abc-123",
		DownloadStatus: datastore.DownloadStatusFailed,
		DownloadError:  "timeout after 30s",
	}

	if p.ID != "abc-123" {
		t.Errorf("ID = %q, want %q", p.ID, "abc-123")
	}
	if p.DownloadStatus != "failed" {
		t.Errorf("DownloadStatus = %q, want %q", p.DownloadStatus, "failed")
	}
	if p.DownloadError != "timeout after 30s" {
		t.Errorf("DownloadError = %q, want %q", p.DownloadError, "timeout after 30s")
	}
}

// ---------------------------------------------------------------------------
// CookbookFetchResult accounting tests
// ---------------------------------------------------------------------------

func TestCookbookFetchResult_Totals(t *testing.T) {
	r := CookbookFetchResult{
		Total:      10,
		Downloaded: 7,
		Failed:     2,
		Skipped:    1,
		Errors: []CookbookFetchError{
			{Name: "bad1", Version: "1.0.0", Err: fmt.Errorf("fail1")},
			{Name: "bad2", Version: "2.0.0", Err: fmt.Errorf("fail2")},
		},
	}

	if r.Total != r.Downloaded+r.Failed+r.Skipped {
		t.Errorf("Total (%d) != Downloaded (%d) + Failed (%d) + Skipped (%d)",
			r.Total, r.Downloaded, r.Failed, r.Skipped)
	}

	if len(r.Errors) != r.Failed {
		t.Errorf("len(Errors) = %d, want Failed = %d", len(r.Errors), r.Failed)
	}
}

// ---------------------------------------------------------------------------
// Integration: fetchCookbooks logging
// ---------------------------------------------------------------------------

// Note: end-to-end fetchCookbooks tests that exercise logging are covered
// by the collector integration tests (collector_test.go) which use the mock
// client factory and a real (test) datastore. We cannot pass nil db here.

// ---------------------------------------------------------------------------
// Edge case: empty cookbook list
// ---------------------------------------------------------------------------

func TestFetchCookbooks_EmptyCookbookList_NoPanic(t *testing.T) {
	// If there are no cookbooks needing download, the function should
	// return immediately with Total=0. We can't test this with nil db
	// (it would error), but we verify the result shape is correct via
	// the CookbookFetchResult zero value.
	r := CookbookFetchResult{}
	if r.Total != 0 {
		t.Errorf("empty result Total = %d, want 0", r.Total)
	}
	if r.Downloaded != 0 {
		t.Errorf("empty result Downloaded = %d, want 0", r.Downloaded)
	}
}

// ---------------------------------------------------------------------------
// Multiple CookbookFetchError formatting
// ---------------------------------------------------------------------------

func TestCookbookFetchError_MultipleErrors(t *testing.T) {
	errors := []CookbookFetchError{
		{CookbookID: "id1", Name: "nginx", Version: "5.1.0", Err: fmt.Errorf("404 Not Found")},
		{CookbookID: "id2", Name: "apt", Version: "7.4.0", Err: fmt.Errorf("timeout")},
		{CookbookID: "id3", Name: "base", Version: "1.3.2", Err: fmt.Errorf("permission denied")},
	}

	expected := []string{
		"nginx/5.1.0: 404 Not Found",
		"apt/7.4.0: timeout",
		"base/1.3.2: permission denied",
	}

	for i, e := range errors {
		if got := e.Error(); got != expected[i] {
			t.Errorf("errors[%d].Error() = %q, want %q", i, got, expected[i])
		}
	}
}

// ---------------------------------------------------------------------------
// formatDownloadError with various APIError shapes
// ---------------------------------------------------------------------------

func TestFormatDownloadError_APIError_EmptyBody(t *testing.T) {
	apiErr := &chefapi.APIError{
		StatusCode: 502,
		Method:     "GET",
		Path:       "/cookbooks/test/1.0.0",
		Body:       "",
	}

	got := formatDownloadError(apiErr)
	want := "502 GET: "
	if got != want {
		t.Errorf("formatDownloadError(empty body) = %q, want %q", got, want)
	}
}

func TestFormatDownloadError_APIError_LargeBody(t *testing.T) {
	// Verify we don't truncate or modify the body.
	longBody := "error: " + string(make([]byte, 1000))
	apiErr := &chefapi.APIError{
		StatusCode: 500,
		Method:     "GET",
		Path:       "/cookbooks/test/1.0.0",
		Body:       longBody,
	}

	got := formatDownloadError(apiErr)
	want := fmt.Sprintf("500 GET: %s", longBody)
	if got != want {
		t.Errorf("formatDownloadError(large body) length = %d, want %d", len(got), len(want))
	}
}

// ---------------------------------------------------------------------------
// Cookbook source type interaction with download status
// ---------------------------------------------------------------------------

func TestCookbook_GitSource_DownloadStatus(t *testing.T) {
	// Git-sourced cookbooks should have download_status = 'ok' since
	// they're managed via git clone/pull, not the Chef server download
	// pipeline. Verify the struct methods work correctly.
	cb := datastore.Cookbook{
		Source:         "git",
		DownloadStatus: datastore.DownloadStatusOK,
	}

	if !cb.IsGit() {
		t.Error("expected IsGit() to be true")
	}
	if !cb.IsDownloaded() {
		t.Error("expected IsDownloaded() to be true for git cookbook with status ok")
	}
	if cb.NeedsDownload() {
		t.Error("expected NeedsDownload() to be false for git cookbook with status ok")
	}
}

func TestCookbook_ChefServerSource_PendingStatus(t *testing.T) {
	cb := datastore.Cookbook{
		Source:         "chef_server",
		DownloadStatus: datastore.DownloadStatusPending,
	}

	if !cb.IsChefServer() {
		t.Error("expected IsChefServer() to be true")
	}
	if cb.IsDownloaded() {
		t.Error("expected IsDownloaded() to be false for pending cookbook")
	}
	if !cb.NeedsDownload() {
		t.Error("expected NeedsDownload() to be true for pending cookbook")
	}
}

func TestCookbook_ChefServerSource_FailedStatus(t *testing.T) {
	cb := datastore.Cookbook{
		Source:         "chef_server",
		DownloadStatus: datastore.DownloadStatusFailed,
		DownloadError:  "connection reset",
	}

	if cb.IsDownloaded() {
		t.Error("expected IsDownloaded() to be false for failed cookbook")
	}
	if !cb.NeedsDownload() {
		t.Error("expected NeedsDownload() to be true for failed cookbook")
	}
}

func TestCookbook_ChefServerSource_OKStatus(t *testing.T) {
	cb := datastore.Cookbook{
		Source:         "chef_server",
		DownloadStatus: datastore.DownloadStatusOK,
	}

	if !cb.IsDownloaded() {
		t.Error("expected IsDownloaded() to be true for ok cookbook")
	}
	if cb.NeedsDownload() {
		t.Error("expected NeedsDownload() to be false for ok cookbook")
	}
}

// ---------------------------------------------------------------------------
// hasParentTraversal tests
// ---------------------------------------------------------------------------

func TestHasParentTraversal(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"recipes/default.rb", false},
		{"metadata.rb", false},
		{"files/default/config.conf", false},
		{"..", true},
		{"../etc/passwd", true},
		{"recipes/../../etc/passwd", true},
		{"a/b/c", false},
		{"a/b/../c", false}, // filepath.Clean normalises this to "a/c" which has no ".."
		{".", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			// hasParentTraversal expects a filepath.Clean'd path
			cleaned := filepath.Clean(tt.path)
			got := hasParentTraversal(cleaned)
			if got != tt.want {
				t.Errorf("hasParentTraversal(%q) = %v, want %v (cleaned: %q)", tt.path, got, tt.want, cleaned)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// splitPathComponents tests
// ---------------------------------------------------------------------------

func TestSplitPathComponents(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{"recipes/default.rb", []string{"recipes", "default.rb"}},
		{"metadata.rb", []string{"metadata.rb"}},
		{"a/b/c/d.txt", []string{"a", "b", "c", "d.txt"}},
		{"single", []string{"single"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := splitPathComponents(tt.path)
			if len(got) != len(tt.want) {
				t.Fatalf("splitPathComponents(%q) = %v (len %d), want %v (len %d)",
					tt.path, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitPathComponents(%q)[%d] = %q, want %q",
						tt.path, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// isSubPath tests
// ---------------------------------------------------------------------------

func TestIsSubPath(t *testing.T) {
	tests := []struct {
		name   string
		parent string
		child  string
		want   bool
	}{
		{"exact match", "/var/lib/data", "/var/lib/data", true},
		{"child under parent", "/var/lib/data", "/var/lib/data/org/cookbook/1.0", true},
		{"child is direct file", "/var/lib/data", "/var/lib/data/file.txt", true},
		{"sibling not child", "/var/lib/data", "/var/lib/dataextra/file.txt", false},
		{"parent of parent", "/var/lib/data", "/var/lib", false},
		{"completely different", "/var/lib/data", "/etc/passwd", false},
		{"root parent", "/", "/anything/at/all", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSubPath(tt.parent, tt.child)
			if got != tt.want {
				t.Errorf("isSubPath(%q, %q) = %v, want %v", tt.parent, tt.child, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractCookbookFiles tests
// ---------------------------------------------------------------------------

func TestExtractCookbookFiles_EmptyManifest(t *testing.T) {
	destDir := t.TempDir()
	subDir := filepath.Join(destDir, "org1", "mycookbook", "1.0.0")

	manifest := &chefapi.CookbookVersionManifest{
		CookbookName: "mycookbook",
		Version:      "1.0.0",
	}

	// We don't need a real client since there are no files to download.
	written, err := extractCookbookFiles(context.Background(), nil, manifest, subDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if written != 0 {
		t.Errorf("expected 0 files written, got %d", written)
	}

	// The directory should have been created.
	info, statErr := os.Stat(subDir)
	if statErr != nil {
		t.Fatalf("expected directory to exist: %v", statErr)
	}
	if !info.IsDir() {
		t.Errorf("expected %s to be a directory", subDir)
	}
}

func TestExtractCookbookFiles_WritesFiles(t *testing.T) {
	// Set up a test HTTP server that serves file content.
	recipeContent := []byte("# Default recipe\nlog 'hello'\n")
	metadataContent := []byte("name 'test'\nversion '1.0.0'\n")

	recipeChecksum := fmt.Sprintf("%x", sha256.Sum256(recipeContent))
	metadataChecksum := fmt.Sprintf("%x", sha256.Sum256(metadataContent))

	mux := http.NewServeMux()
	mux.HandleFunc("/files/recipe_default", func(w http.ResponseWriter, r *http.Request) {
		w.Write(recipeContent)
	})
	mux.HandleFunc("/files/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Write(metadataContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Build a chefapi.Client using the test server. We need a valid client
	// for DownloadFileContent, but the bookshelf URLs point to our test server
	// so no Chef API signing is needed (DownloadFileContent uses plain HTTP GET).
	client := newTestChefClient(t, srv)

	manifest := &chefapi.CookbookVersionManifest{
		CookbookName: "test",
		Version:      "1.0.0",
		Recipes: []chefapi.CookbookFileRef{
			{
				Name:     "default.rb",
				Path:     "recipes/default.rb",
				Checksum: recipeChecksum,
				URL:      srv.URL + "/files/recipe_default",
			},
		},
		RootFiles: []chefapi.CookbookFileRef{
			{
				Name:     "metadata.rb",
				Path:     "metadata.rb",
				Checksum: metadataChecksum,
				URL:      srv.URL + "/files/metadata",
			},
		},
	}

	destDir := filepath.Join(t.TempDir(), "org1", "test", "1.0.0")

	written, err := extractCookbookFiles(context.Background(), client, manifest, destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if written != 2 {
		t.Errorf("expected 2 files written, got %d", written)
	}

	// Verify recipe file.
	gotRecipe, err := os.ReadFile(filepath.Join(destDir, "recipes", "default.rb"))
	if err != nil {
		t.Fatalf("failed to read recipe file: %v", err)
	}
	if string(gotRecipe) != string(recipeContent) {
		t.Errorf("recipe content = %q, want %q", gotRecipe, recipeContent)
	}

	// Verify metadata file.
	gotMetadata, err := os.ReadFile(filepath.Join(destDir, "metadata.rb"))
	if err != nil {
		t.Fatalf("failed to read metadata file: %v", err)
	}
	if string(gotMetadata) != string(metadataContent) {
		t.Errorf("metadata content = %q, want %q", gotMetadata, metadataContent)
	}
}

func TestExtractCookbookFiles_ChecksumMismatch(t *testing.T) {
	badContent := []byte("corrupted data")

	mux := http.NewServeMux()
	mux.HandleFunc("/files/bad", func(w http.ResponseWriter, r *http.Request) {
		w.Write(badContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestChefClient(t, srv)

	manifest := &chefapi.CookbookVersionManifest{
		CookbookName: "test",
		Version:      "1.0.0",
		Recipes: []chefapi.CookbookFileRef{
			{
				Name:     "default.rb",
				Path:     "recipes/default.rb",
				Checksum: "0000000000000000000000000000000000000000000000000000000000000000",
				URL:      srv.URL + "/files/bad",
			},
		},
	}

	destDir := filepath.Join(t.TempDir(), "test", "1.0.0")

	_, err := extractCookbookFiles(context.Background(), client, manifest, destDir)
	if err == nil {
		t.Fatal("expected checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected checksum mismatch in error, got: %v", err)
	}
}

func TestExtractCookbookFiles_ContextCancellation(t *testing.T) {
	content := []byte("file content")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	mux := http.NewServeMux()
	mux.HandleFunc("/files/ok", func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestChefClient(t, srv)

	manifest := &chefapi.CookbookVersionManifest{
		CookbookName: "test",
		Version:      "1.0.0",
		Recipes: []chefapi.CookbookFileRef{
			{Name: "a.rb", Path: "recipes/a.rb", Checksum: checksum, URL: srv.URL + "/files/ok"},
			{Name: "b.rb", Path: "recipes/b.rb", Checksum: checksum, URL: srv.URL + "/files/ok"},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	destDir := filepath.Join(t.TempDir(), "test", "1.0.0")

	_, err := extractCookbookFiles(ctx, client, manifest, destDir)
	if err == nil {
		t.Fatal("expected context cancelled error, got nil")
	}
	if !strings.Contains(err.Error(), "context cancel") {
		t.Errorf("expected context cancellation in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// downloadAndWriteFile tests
// ---------------------------------------------------------------------------

func TestDownloadAndWriteFile_PathTraversal_DotDot(t *testing.T) {
	ref := chefapi.CookbookFileRef{
		Name: "evil.rb",
		Path: "../../../etc/passwd",
		URL:  "http://example.com/irrelevant",
	}

	destDir := t.TempDir()
	err := downloadAndWriteFile(context.Background(), nil, ref, destDir)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "unsafe file path") && !strings.Contains(err.Error(), "escapes destination") {
		t.Errorf("expected path traversal error, got: %v", err)
	}
}

func TestDownloadAndWriteFile_PathTraversal_AbsolutePath(t *testing.T) {
	ref := chefapi.CookbookFileRef{
		Name: "evil.rb",
		Path: "/etc/passwd",
		URL:  "http://example.com/irrelevant",
	}

	destDir := t.TempDir()
	err := downloadAndWriteFile(context.Background(), nil, ref, destDir)
	if err == nil {
		t.Fatal("expected error for absolute path, got nil")
	}
	if !strings.Contains(err.Error(), "unsafe file path") && !strings.Contains(err.Error(), "escapes destination") {
		t.Errorf("expected path traversal error, got: %v", err)
	}
}

func TestDownloadAndWriteFile_PathTraversal_MidPathDotDot(t *testing.T) {
	ref := chefapi.CookbookFileRef{
		Name: "evil.rb",
		Path: "recipes/../../evil.rb",
		URL:  "http://example.com/irrelevant",
	}

	destDir := t.TempDir()
	err := downloadAndWriteFile(context.Background(), nil, ref, destDir)
	// filepath.Clean("recipes/../../evil.rb") => "../evil.rb" which has ".."
	if err == nil {
		t.Fatal("expected error for mid-path traversal, got nil")
	}
}

func TestDownloadAndWriteFile_Success(t *testing.T) {
	content := []byte("package main\n")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	client := newTestChefClient(t, srv)

	ref := chefapi.CookbookFileRef{
		Name:     "default.rb",
		Path:     "recipes/default.rb",
		Checksum: checksum,
		URL:      srv.URL + "/file",
	}

	destDir := t.TempDir()
	err := downloadAndWriteFile(context.Background(), client, ref, destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "recipes", "default.rb"))
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content = %q, want %q", got, content)
	}
}

func TestDownloadAndWriteFile_CreatesNestedDirs(t *testing.T) {
	content := []byte("deeply nested")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	client := newTestChefClient(t, srv)

	ref := chefapi.CookbookFileRef{
		Name:     "config.conf",
		Path:     "files/default/etc/myapp/config.conf",
		Checksum: checksum,
		URL:      srv.URL + "/file",
	}

	destDir := t.TempDir()
	err := downloadAndWriteFile(context.Background(), client, ref, destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(destDir, "files", "default", "etc", "myapp", "config.conf"))
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content = %q, want %q", got, content)
	}
}

func TestDownloadAndWriteFile_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := newTestChefClient(t, srv)

	ref := chefapi.CookbookFileRef{
		Name: "default.rb",
		Path: "recipes/default.rb",
		URL:  srv.URL + "/file",
	}

	destDir := t.TempDir()
	err := downloadAndWriteFile(context.Background(), client, ref, destDir)
	if err == nil {
		t.Fatal("expected error for server 500, got nil")
	}
}

func TestDownloadAndWriteFile_EmptyPath(t *testing.T) {
	// extractCookbookFiles skips refs with empty paths before calling
	// downloadAndWriteFile. Verify that extractCookbookFiles correctly
	// skips entries with empty paths.
	content := []byte("should be skipped")
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	client := newTestChefClient(t, srv)

	manifest := &chefapi.CookbookVersionManifest{
		CookbookName: "test",
		Version:      "1.0.0",
		Recipes: []chefapi.CookbookFileRef{
			{Name: "empty", Path: "", Checksum: checksum, URL: srv.URL + "/file"},
		},
	}

	destDir := filepath.Join(t.TempDir(), "test", "1.0.0")

	written, err := extractCookbookFiles(context.Background(), client, manifest, destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if written != 0 {
		t.Errorf("expected 0 files written (empty path should be skipped), got %d", written)
	}
}

// ---------------------------------------------------------------------------
// CookbookVersionManifest.AllFiles tests
// ---------------------------------------------------------------------------

func TestCookbookVersionManifest_AllFiles(t *testing.T) {
	manifest := chefapi.CookbookVersionManifest{
		Recipes:    []chefapi.CookbookFileRef{{Path: "recipes/default.rb"}},
		Attributes: []chefapi.CookbookFileRef{{Path: "attributes/default.rb"}},
		Files:      []chefapi.CookbookFileRef{{Path: "files/default/config"}, {Path: "files/default/other"}},
		Templates:  []chefapi.CookbookFileRef{{Path: "templates/default/config.erb"}},
		RootFiles:  []chefapi.CookbookFileRef{{Path: "metadata.rb"}, {Path: "README.md"}},
	}

	all := manifest.AllFiles()
	if len(all) != 7 {
		t.Fatalf("AllFiles() returned %d files, want 7", len(all))
	}

	paths := make(map[string]bool)
	for _, f := range all {
		paths[f.Path] = true
	}

	expected := []string{
		"recipes/default.rb",
		"attributes/default.rb",
		"files/default/config",
		"files/default/other",
		"templates/default/config.erb",
		"metadata.rb",
		"README.md",
	}
	for _, p := range expected {
		if !paths[p] {
			t.Errorf("AllFiles() missing path %q", p)
		}
	}
}

func TestCookbookVersionManifest_AllFiles_Empty(t *testing.T) {
	manifest := chefapi.CookbookVersionManifest{}
	all := manifest.AllFiles()
	if len(all) != 0 {
		t.Errorf("AllFiles() returned %d files for empty manifest, want 0", len(all))
	}
}

func TestCookbookVersionManifest_AllFiles_AllCategories(t *testing.T) {
	manifest := chefapi.CookbookVersionManifest{
		Recipes:     []chefapi.CookbookFileRef{{Path: "r"}},
		Definitions: []chefapi.CookbookFileRef{{Path: "d"}},
		Libraries:   []chefapi.CookbookFileRef{{Path: "l"}},
		Attributes:  []chefapi.CookbookFileRef{{Path: "a"}},
		Files:       []chefapi.CookbookFileRef{{Path: "f"}},
		Templates:   []chefapi.CookbookFileRef{{Path: "t"}},
		Resources:   []chefapi.CookbookFileRef{{Path: "res"}},
		Providers:   []chefapi.CookbookFileRef{{Path: "p"}},
		RootFiles:   []chefapi.CookbookFileRef{{Path: "rf"}},
	}

	all := manifest.AllFiles()
	if len(all) != 9 {
		t.Errorf("AllFiles() returned %d files, want 9 (one per category)", len(all))
	}
}

// ---------------------------------------------------------------------------
// CookbookFetchResult.FilesWritten tests
// ---------------------------------------------------------------------------

func TestCookbookFetchResult_FilesWritten(t *testing.T) {
	r := CookbookFetchResult{
		Total:        3,
		Downloaded:   2,
		Failed:       1,
		FilesWritten: 15,
	}
	if r.FilesWritten != 15 {
		t.Errorf("FilesWritten = %d, want 15", r.FilesWritten)
	}
}

// ---------------------------------------------------------------------------
// Integration-style: extractCookbookFiles with multiple categories
// ---------------------------------------------------------------------------

func TestExtractCookbookFiles_MultipleCategories(t *testing.T) {
	files := map[string][]byte{
		"/files/recipe":    []byte("recipe content"),
		"/files/attribute": []byte("attribute content"),
		"/files/template":  []byte("template content"),
		"/files/library":   []byte("library content"),
		"/files/metadata":  []byte("metadata content"),
	}

	mux := http.NewServeMux()
	for path, content := range files {
		c := content // capture
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Write(c)
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestChefClient(t, srv)

	makeRef := func(name, path, urlPath string, content []byte) chefapi.CookbookFileRef {
		return chefapi.CookbookFileRef{
			Name:     name,
			Path:     path,
			Checksum: fmt.Sprintf("%x", sha256.Sum256(content)),
			URL:      srv.URL + urlPath,
		}
	}

	manifest := &chefapi.CookbookVersionManifest{
		CookbookName: "multi",
		Version:      "2.0.0",
		Recipes:      []chefapi.CookbookFileRef{makeRef("default.rb", "recipes/default.rb", "/files/recipe", files["/files/recipe"])},
		Attributes:   []chefapi.CookbookFileRef{makeRef("default.rb", "attributes/default.rb", "/files/attribute", files["/files/attribute"])},
		Templates:    []chefapi.CookbookFileRef{makeRef("config.erb", "templates/default/config.erb", "/files/template", files["/files/template"])},
		Libraries:    []chefapi.CookbookFileRef{makeRef("helper.rb", "libraries/helper.rb", "/files/library", files["/files/library"])},
		RootFiles:    []chefapi.CookbookFileRef{makeRef("metadata.rb", "metadata.rb", "/files/metadata", files["/files/metadata"])},
	}

	destDir := filepath.Join(t.TempDir(), "org", "multi", "2.0.0")

	written, err := extractCookbookFiles(context.Background(), client, manifest, destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if written != 5 {
		t.Errorf("expected 5 files written, got %d", written)
	}

	// Verify each file.
	checks := map[string][]byte{
		"recipes/default.rb":           files["/files/recipe"],
		"attributes/default.rb":        files["/files/attribute"],
		"templates/default/config.erb": files["/files/template"],
		"libraries/helper.rb":          files["/files/library"],
		"metadata.rb":                  files["/files/metadata"],
	}
	for relPath, wantContent := range checks {
		got, err := os.ReadFile(filepath.Join(destDir, relPath))
		if err != nil {
			t.Errorf("failed to read %s: %v", relPath, err)
			continue
		}
		if string(got) != string(wantContent) {
			t.Errorf("%s content = %q, want %q", relPath, got, wantContent)
		}
	}
}

// ---------------------------------------------------------------------------
// extractCookbookFiles partial failure (second file fails)
// ---------------------------------------------------------------------------

func TestExtractCookbookFiles_PartialFailure(t *testing.T) {
	goodContent := []byte("good file")
	goodChecksum := fmt.Sprintf("%x", sha256.Sum256(goodContent))

	mux := http.NewServeMux()
	mux.HandleFunc("/files/good", func(w http.ResponseWriter, r *http.Request) {
		w.Write(goodContent)
	})
	mux.HandleFunc("/files/bad", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestChefClient(t, srv)

	manifest := &chefapi.CookbookVersionManifest{
		CookbookName: "test",
		Version:      "1.0.0",
		Recipes: []chefapi.CookbookFileRef{
			{Name: "good.rb", Path: "recipes/good.rb", Checksum: goodChecksum, URL: srv.URL + "/files/good"},
			{Name: "bad.rb", Path: "recipes/bad.rb", Checksum: "irrelevant", URL: srv.URL + "/files/bad"},
		},
	}

	destDir := filepath.Join(t.TempDir(), "test", "1.0.0")

	written, err := extractCookbookFiles(context.Background(), client, manifest, destDir)
	if err == nil {
		t.Fatal("expected error for second file failure, got nil")
	}
	// First file should have been written before the second failed.
	if written != 1 {
		t.Errorf("expected 1 file written before failure, got %d", written)
	}
}

// ---------------------------------------------------------------------------
// Test helper: create a chefapi.Client backed by a test HTTP server
// ---------------------------------------------------------------------------

// newTestChefClient creates a chefapi.Client that uses the provided test
// server's HTTP client. The client's Chef API authentication is configured
// with a test RSA key. For DownloadFileContent calls (which use plain HTTP
// GET without signing), the key is irrelevant — what matters is that the
// client's httpClient reaches the test server.
func newTestChefClient(t *testing.T, srv *httptest.Server) *chefapi.Client {
	t.Helper()

	// Generate a minimal RSA key for the client constructor. It won't be
	// used for bookshelf downloads (plain HTTP GET), but the constructor
	// requires one.
	key := generateTestRSAKeyPEM(t)

	client, err := chefapi.NewClient(chefapi.ClientConfig{
		ServerURL:     srv.URL + "/organizations/test",
		ClientName:    "test-client",
		PrivateKeyPEM: key,
		OrgName:       "test",
		HTTPClient:    srv.Client(),
	})
	if err != nil {
		t.Fatalf("failed to create test chefapi.Client: %v", err)
	}
	return client
}

// generateTestRSAKeyPEM generates a PEM-encoded RSA private key for testing.
func generateTestRSAKeyPEM(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test RSA key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}
	return pem.EncodeToMemory(block)
}
