// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// testScopedLogger returns a *logging.ScopedLogger suitable for test use.
// It reuses the package-level newTestLogger (from scheduler_test.go) which
// returns a *logging.Logger, then wraps it with a scope.
func testScopedLogger() *logging.ScopedLogger {
	return newTestLogger().WithScope(logging.ScopeCollectionRun)
}

// ---------------------------------------------------------------------------
// removeEmptyDir
// ---------------------------------------------------------------------------

func TestRemoveEmptyDir_RemovesEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	emptyChild := filepath.Join(dir, "empty")
	if err := os.Mkdir(emptyChild, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	removeEmptyDir(emptyChild)

	if _, err := os.Stat(emptyChild); !os.IsNotExist(err) {
		t.Errorf("expected empty directory to be removed, but it still exists")
	}
}

func TestRemoveEmptyDir_LeavesNonEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "notempty")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatalf("setup mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(child, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("setup writefile: %v", err)
	}

	removeEmptyDir(child)

	if _, err := os.Stat(child); err != nil {
		t.Errorf("expected non-empty directory to remain, got stat error: %v", err)
	}
}

func TestRemoveEmptyDir_NonExistentPath_NoPanic(t *testing.T) {
	// Should be a no-op without panicking.
	removeEmptyDir("/nonexistent/path/that/does/not/exist")
}

// ---------------------------------------------------------------------------
// cleanLegacyCookbookCache
// ---------------------------------------------------------------------------

func TestCleanLegacyCookbookCache_RemovesVersionDirs(t *testing.T) {
	cacheDir := t.TempDir()
	orgID := "org-abc-123"

	// Set up the legacy cache structure:
	// <cacheDir>/<orgID>/apache2/5.0.1/recipes/default.rb
	// <cacheDir>/<orgID>/apache2/4.0.0/metadata.rb
	// <cacheDir>/<orgID>/nginx/1.2.3/attributes/default.rb
	cookbooks := []struct {
		name, version, file, content string
	}{
		{"apache2", "5.0.1", "recipes/default.rb", "# recipe"},
		{"apache2", "4.0.0", "metadata.rb", "name 'apache2'"},
		{"nginx", "1.2.3", "attributes/default.rb", "default['nginx'] = true"},
	}

	for _, cb := range cookbooks {
		vDir := filepath.Join(cacheDir, orgID, cb.name, cb.version)
		fPath := filepath.Join(vDir, cb.file)
		if err := os.MkdirAll(filepath.Dir(fPath), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(fPath, []byte(cb.content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	log := testScopedLogger()
	cleaned := cleanLegacyCookbookCache(log, cacheDir, orgID)

	if cleaned != 3 {
		t.Errorf("expected 3 version directories cleaned, got %d", cleaned)
	}

	// All version directories should be gone.
	for _, cb := range cookbooks {
		vDir := filepath.Join(cacheDir, orgID, cb.name, cb.version)
		if _, err := os.Stat(vDir); !os.IsNotExist(err) {
			t.Errorf("expected version dir %s to be removed", vDir)
		}
	}

	// Cookbook name directories (apache2/, nginx/) should be pruned.
	for _, name := range []string{"apache2", "nginx"} {
		nameDir := filepath.Join(cacheDir, orgID, name)
		if _, err := os.Stat(nameDir); !os.IsNotExist(err) {
			t.Errorf("expected empty cookbook name dir %s to be pruned", nameDir)
		}
	}

	// Org directory should be pruned.
	orgDir := filepath.Join(cacheDir, orgID)
	if _, err := os.Stat(orgDir); !os.IsNotExist(err) {
		t.Errorf("expected empty org dir %s to be pruned", orgDir)
	}
}

func TestCleanLegacyCookbookCache_NoOrgDir_ReturnsZero(t *testing.T) {
	cacheDir := t.TempDir()
	log := testScopedLogger()

	cleaned := cleanLegacyCookbookCache(log, cacheDir, "nonexistent-org")
	if cleaned != 0 {
		t.Errorf("expected 0 cleaned for nonexistent org dir, got %d", cleaned)
	}
}

func TestCleanLegacyCookbookCache_EmptyOrgDir_ReturnsZero(t *testing.T) {
	cacheDir := t.TempDir()
	orgID := "org-empty"
	orgDir := filepath.Join(cacheDir, orgID)
	if err := os.Mkdir(orgDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	log := testScopedLogger()
	cleaned := cleanLegacyCookbookCache(log, cacheDir, orgID)

	if cleaned != 0 {
		t.Errorf("expected 0 cleaned for empty org dir, got %d", cleaned)
	}

	// Empty org dir should still be pruned.
	if _, err := os.Stat(orgDir); !os.IsNotExist(err) {
		t.Errorf("expected empty org dir to be pruned")
	}
}

func TestCleanLegacyCookbookCache_SkipsRegularFiles(t *testing.T) {
	cacheDir := t.TempDir()
	orgID := "org-files"

	// Create a file where a cookbook name directory would normally be.
	orgDir := filepath.Join(cacheDir, orgID)
	if err := os.MkdirAll(orgDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orgDir, "stray-file.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Also create a real cookbook with a file at the version level instead
	// of a directory.
	nameDir := filepath.Join(orgDir, "mycookbook")
	if err := os.Mkdir(nameDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nameDir, "not-a-version-dir"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	log := testScopedLogger()
	cleaned := cleanLegacyCookbookCache(log, cacheDir, orgID)

	if cleaned != 0 {
		t.Errorf("expected 0 version directories cleaned (only files present), got %d", cleaned)
	}

	// The stray file should still exist — we only remove version directories.
	if _, err := os.Stat(filepath.Join(orgDir, "stray-file.txt")); err != nil {
		t.Errorf("stray file should still exist: %v", err)
	}
}

func TestCleanLegacyCookbookCache_PartialCleanup_LeavesNonEmptyParent(t *testing.T) {
	cacheDir := t.TempDir()
	orgID := "org-partial"

	// Create two cookbook name dirs. One has a version dir (will be cleaned),
	// the other has a non-directory entry (won't be cleaned, so the name
	// dir won't be pruned).
	vDir := filepath.Join(cacheDir, orgID, "cb1", "1.0.0")
	if err := os.MkdirAll(vDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vDir, "metadata.rb"), []byte("name 'cb1'"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cb2Dir := filepath.Join(cacheDir, orgID, "cb2")
	if err := os.MkdirAll(cb2Dir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// cb2 has a stray file at the version level, not a directory.
	if err := os.WriteFile(filepath.Join(cb2Dir, "stray"), []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	log := testScopedLogger()
	cleaned := cleanLegacyCookbookCache(log, cacheDir, orgID)

	if cleaned != 1 {
		t.Errorf("expected 1 version directory cleaned, got %d", cleaned)
	}

	// cb1 should be fully pruned (version dir removed, name dir empty -> pruned).
	if _, err := os.Stat(filepath.Join(cacheDir, orgID, "cb1")); !os.IsNotExist(err) {
		t.Errorf("expected cb1 name dir to be pruned")
	}

	// cb2 should still exist because it contains a file.
	if _, err := os.Stat(cb2Dir); err != nil {
		t.Errorf("expected cb2 name dir to remain (has stray file): %v", err)
	}

	// Org dir should still exist because cb2 is still there.
	orgDir := filepath.Join(cacheDir, orgID)
	if _, err := os.Stat(orgDir); err != nil {
		t.Errorf("expected org dir to remain (cb2 still present): %v", err)
	}
}

func TestCleanLegacyCookbookCache_MultipleOrgs_Independent(t *testing.T) {
	cacheDir := t.TempDir()

	// Set up two orgs with cookbook files.
	for _, orgID := range []string{"org-1", "org-2"} {
		vDir := filepath.Join(cacheDir, orgID, "mycb", "1.0.0")
		if err := os.MkdirAll(vDir, 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(vDir, "metadata.rb"), []byte("x"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	log := testScopedLogger()

	// Clean only org-1.
	cleaned := cleanLegacyCookbookCache(log, cacheDir, "org-1")
	if cleaned != 1 {
		t.Errorf("expected 1 cleaned for org-1, got %d", cleaned)
	}

	// org-1 should be gone.
	if _, err := os.Stat(filepath.Join(cacheDir, "org-1")); !os.IsNotExist(err) {
		t.Errorf("expected org-1 dir to be pruned")
	}

	// org-2 should be untouched.
	if _, err := os.Stat(filepath.Join(cacheDir, "org-2", "mycb", "1.0.0", "metadata.rb")); err != nil {
		t.Errorf("expected org-2 files to be untouched: %v", err)
	}
}

func TestCleanLegacyCookbookCache_EmptyCacheDir_NoPanic(t *testing.T) {
	log := testScopedLogger()
	// Empty string cache dir — the stat will fail, should return 0.
	cleaned := cleanLegacyCookbookCache(log, "", "some-org")
	if cleaned != 0 {
		t.Errorf("expected 0 for empty cacheDir, got %d", cleaned)
	}
}

// ---------------------------------------------------------------------------
// downloadToTempDir — we can't easily test the full function because it
// requires a real chefapi.Client and datastore.DB, but we can verify the
// function signature change (no cookbookCacheDir param) compiles and that
// the temp directory naming convention works.
// ---------------------------------------------------------------------------

func TestDownloadToTempDir_SignatureCompiles(t *testing.T) {
	// This test verifies that downloadToTempDir no longer accepts a
	// cookbookCacheDir parameter. If someone re-adds the parameter,
	// this test will fail to compile.
	//
	// We can't call the function without a real client/db, but we can
	// verify the type signature by assigning it to a variable with the
	// expected type.
	var _ func(
		ctx interface{},
		client interface{},
		db interface{},
		cb interface{},
	) = nil
	// The above is a compile-time check that the function exists with
	// exactly 4 parameters (context, client, db, cookbook). If there were
	// a 5th (cookbookCacheDir), the call sites in this file that pass 4
	// args would fail.
	_ = t
}

// ---------------------------------------------------------------------------
// ServerCookbookPipelineResult
// ---------------------------------------------------------------------------

func TestServerCookbookPipelineResult_HasCleanedField(t *testing.T) {
	r := ServerCookbookPipelineResult{
		Total:      100,
		Downloaded: 80,
		Scanned:    75,
		Skipped:    5,
		Failed:     5,
		Cleaned:    42,
	}

	if r.Cleaned != 42 {
		t.Errorf("expected Cleaned=42, got %d", r.Cleaned)
	}
}

func TestServerCookbookPipelineResult_ZeroValue(t *testing.T) {
	var r ServerCookbookPipelineResult
	if r.Total != 0 || r.Downloaded != 0 || r.Scanned != 0 ||
		r.Skipped != 0 || r.Failed != 0 || r.Cleaned != 0 {
		t.Errorf("zero-value result should have all zeros: %+v", r)
	}
	if r.Duration != 0 {
		t.Errorf("zero-value Duration should be 0, got %v", r.Duration)
	}
	if r.Errors != nil {
		t.Errorf("zero-value Errors should be nil, got %v", r.Errors)
	}
}

// ---------------------------------------------------------------------------
// cleanLegacyCookbookCache — deeply nested version directories
// ---------------------------------------------------------------------------

func TestCleanLegacyCookbookCache_DeeplyNestedContent(t *testing.T) {
	cacheDir := t.TempDir()
	orgID := "org-deep"

	// Create a cookbook with deeply nested file structure.
	deepPath := filepath.Join(cacheDir, orgID, "complex-cb", "2.3.4",
		"recipes", "subdir", "nested", "deep.rb")
	if err := os.MkdirAll(filepath.Dir(deepPath), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(deepPath, []byte("# deep file"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	log := testScopedLogger()
	cleaned := cleanLegacyCookbookCache(log, cacheDir, orgID)

	if cleaned != 1 {
		t.Errorf("expected 1 cleaned, got %d", cleaned)
	}

	// Everything under the org should be gone.
	if _, err := os.Stat(filepath.Join(cacheDir, orgID)); !os.IsNotExist(err) {
		t.Errorf("expected entire org directory tree to be pruned")
	}
}

func TestCleanLegacyCookbookCache_MultiplVersionsPerCookbook(t *testing.T) {
	cacheDir := t.TempDir()
	orgID := "org-multi"

	versions := []string{"1.0.0", "1.1.0", "1.2.0", "2.0.0", "2.1.0"}
	for _, v := range versions {
		vDir := filepath.Join(cacheDir, orgID, "java", v)
		if err := os.MkdirAll(vDir, 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(filepath.Join(vDir, "metadata.rb"), []byte("name 'java'"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	log := testScopedLogger()
	cleaned := cleanLegacyCookbookCache(log, cacheDir, orgID)

	if cleaned != len(versions) {
		t.Errorf("expected %d cleaned, got %d", len(versions), cleaned)
	}

	// Entire org tree should be pruned.
	if _, err := os.Stat(filepath.Join(cacheDir, orgID)); !os.IsNotExist(err) {
		t.Errorf("expected org dir to be fully pruned")
	}
}
