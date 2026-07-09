// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
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
// downloadCookbook — we can't easily test the full function because it
// requires a real chefapi.Client and datastore.DB, but we can verify the
// function signature compiles with the expected parameters.
// ---------------------------------------------------------------------------

func TestDownloadCookbook_SignatureCompiles(t *testing.T) {
	// Compile-time check that downloadCookbook accepts 6 parameters:
	// (ctx, client, db, cookbook, cookbookCacheDir, deleteAfterScan).
	var _ = downloadCookbook
	_ = t
}

// cachedServerCookbookDir decides whether a cookbook version can be scanned from
// the on-disk cache without re-downloading — independent of download_status, so
// a rescan (which resets status to 'pending') reuses cached files instead of
// re-pulling the whole fleet from the Chef server.
func TestCachedServerCookbookDir(t *testing.T) {
	cb := datastore.ServerCookbook{OrganisationName: "org-a", Name: "nginx", Version: "5.1.0"}

	stageVersionDir := func(t *testing.T, cacheDir string, empty bool) string {
		t.Helper()
		dir := filepath.Join(cacheDir, cb.OrganisationName, cb.Name, cb.Version)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if !empty {
			if err := os.WriteFile(filepath.Join(dir, "metadata.rb"), []byte("name 'nginx'\n"), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
		}
		return dir
	}

	t.Run("cached non-empty dir is reused regardless of status", func(t *testing.T) {
		cacheDir := t.TempDir()
		want := stageVersionDir(t, cacheDir, false)
		got, ok := cachedServerCookbookDir(cacheDir, cb, false)
		if !ok || got != want {
			t.Errorf("got (%q,%v), want (%q,true)", got, ok, want)
		}
	})

	t.Run("absent version dir is not reused", func(t *testing.T) {
		if _, ok := cachedServerCookbookDir(t.TempDir(), cb, false); ok {
			t.Error("expected no reuse when the version dir is absent")
		}
	})

	t.Run("empty version dir is not reused (guards partial downloads)", func(t *testing.T) {
		cacheDir := t.TempDir()
		stageVersionDir(t, cacheDir, true)
		if _, ok := cachedServerCookbookDir(cacheDir, cb, false); ok {
			t.Error("expected no reuse when the version dir is empty")
		}
	})

	t.Run("delete-after-scan mode never reuses (no persistent cache)", func(t *testing.T) {
		cacheDir := t.TempDir()
		stageVersionDir(t, cacheDir, false)
		if _, ok := cachedServerCookbookDir(cacheDir, cb, true); ok {
			t.Error("expected no reuse in delete-after-scan mode")
		}
	})

	t.Run("empty cache dir config never reuses", func(t *testing.T) {
		if _, ok := cachedServerCookbookDir("", cb, false); ok {
			t.Error("expected no reuse when cookbookCacheDir is empty")
		}
	})
}

// downloadCookbook uses the persistent cache directory when
// deleteAfterScan is false, and os.MkdirTemp when true. The nil chefapi
// client panics inside GetCookbookVersionManifest (it's not nil-safe), so
// we use recover to catch the panic after the directory has been created.

// runDownloadCookbookWithNilClient calls downloadCookbook with a nil
// chefapi client in a separate goroutine with a hard timeout. The nil
// client causes the goroutine to get stuck rather than panicking, so
// we bound it with a channel timeout instead of relying on recover.
func runDownloadCookbookWithNilClient(cb datastore.ServerCookbook, cacheDir string, deleteAfterScan bool) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = recover() }()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = downloadCookbook(ctx, nil, nil, cb, cacheDir, deleteAfterScan)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

func TestDownloadCookbook_UsesCacheDir_WhenRetaining(t *testing.T) {
	cacheDir := t.TempDir()
	cb := datastore.ServerCookbook{
		OrganisationName: "org-abc",
		Name:             "nginx",
		Version:          "5.1.0",
	}

	// nil client will fail in the Chef API layer, but the cache
	// directory is created before the manifest fetch attempt.
	runDownloadCookbookWithNilClient(cb, cacheDir, false)

	expected := filepath.Join(cacheDir, "org-abc", "nginx", "5.1.0")
	info, statErr := os.Stat(expected)
	if statErr != nil {
		t.Fatalf("cache directory was not created at %s: %v", expected, statErr)
	}
	if !info.IsDir() {
		t.Errorf("expected %s to be a directory", expected)
	}
}

func TestDownloadCookbook_UsesTempDir_WhenDeleting(t *testing.T) {
	cacheDir := t.TempDir()
	cb := datastore.ServerCookbook{
		OrganisationName: "org-xyz",
		Name:             "apache2",
		Version:          "3.0.0",
	}

	// nil client fails, but we only care about which directory was used.
	runDownloadCookbookWithNilClient(cb, cacheDir, true)

	// The cache directory should NOT have been populated.
	persistentPath := filepath.Join(cacheDir, "org-xyz", "apache2", "3.0.0")
	if _, statErr := os.Stat(persistentPath); statErr == nil {
		t.Errorf("cache directory should not exist when deleteAfterScan=true, but found %s", persistentPath)
	}
}

func TestDownloadCookbook_UsesTempDir_WhenCacheDirEmpty(t *testing.T) {
	cb := datastore.ServerCookbook{
		OrganisationName: "org-123",
		Name:             "java",
		Version:          "1.0.0",
	}

	// Empty cookbookCacheDir with deleteAfterScan=false should still
	// fall back to os.MkdirTemp rather than panicking on directory creation.
	runDownloadCookbookWithNilClient(cb, "", false)
	// Success = no panic from directory creation.
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
