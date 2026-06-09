// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package nodekitchen

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

type mockCBDownloader struct {
	downloaded map[string]string
	err        error
}

func (m *mockCBDownloader) DownloadCookbook(_ context.Context, name, version, destDir string) error {
	if m.err != nil {
		return m.err
	}
	if m.downloaded == nil {
		m.downloaded = make(map[string]string)
	}
	m.downloaded[name+"/"+version] = destDir
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destDir, "metadata.rb"), []byte("name '"+name+"'"), 0o644)
}

type assembleMockGitLocator struct {
	paths map[string]string
}

func (m *assembleMockGitLocator) LocateCookbook(name string) string {
	if m.paths == nil {
		return ""
	}
	return m.paths[name]
}

// ---------------------------------------------------------------------------
// CreateWorkingDir
// ---------------------------------------------------------------------------

func TestCreateWorkingDir(t *testing.T) {
	wd, err := CreateWorkingDir("testnode")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer wd.Cleanup()

	if wd.Path == "" {
		t.Fatal("expected non-empty path")
	}

	for _, sub := range []string{"cookbooks", "roles"} {
		info, err := os.Stat(filepath.Join(wd.Path, sub))
		if err != nil {
			t.Errorf("subdir %s missing: %v", sub, err)
		} else if !info.IsDir() {
			t.Errorf("%s is not a directory", sub)
		}
	}
}

func TestCreateWorkingDir_Cleanup(t *testing.T) {
	wd, err := CreateWorkingDir("cleanup-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	path := wd.Path
	wd.Cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected dir to be removed after Cleanup, got err=%v", err)
	}
}

// ---------------------------------------------------------------------------
// copyFile
// ---------------------------------------------------------------------------

func TestCopyFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "hello.txt")
	dstPath := filepath.Join(dstDir, "hello.txt")
	content := []byte("hello world")

	if err := os.WriteFile(srcPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile error: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("reading dest: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q", got)
	}
}

// ---------------------------------------------------------------------------
// copyDir
// ---------------------------------------------------------------------------

func TestCopyDir(t *testing.T) {
	src := t.TempDir()

	os.MkdirAll(filepath.Join(src, "recipes"), 0o755)
	os.WriteFile(filepath.Join(src, "metadata.rb"), []byte("name 'test'"), 0o644)
	os.WriteFile(filepath.Join(src, "recipes", "default.rb"), []byte("# recipe"), 0o644)

	// Create a .git dir that should be skipped.
	os.MkdirAll(filepath.Join(src, ".git", "objects"), 0o755)
	os.WriteFile(filepath.Join(src, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0o644)

	dst := filepath.Join(t.TempDir(), "copied")

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir error: %v", err)
	}

	// Verify copied files exist.
	for _, rel := range []string{"metadata.rb", "recipes/default.rb"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	// Verify .git was skipped.
	if _, err := os.Stat(filepath.Join(dst, ".git")); !os.IsNotExist(err) {
		t.Error(".git directory should have been skipped")
	}
}

func TestCopyDir_SourceNotDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(f, []byte("x"), 0o644)

	err := copyDir(f, filepath.Join(t.TempDir(), "out"))
	if err == nil {
		t.Fatal("expected error when source is a file")
	}
}

// ---------------------------------------------------------------------------
// FSGitCookbookLocator
// ---------------------------------------------------------------------------

func TestFSGitCookbookLocator_Found(t *testing.T) {
	base := t.TempDir()
	os.MkdirAll(filepath.Join(base, "nginx"), 0o755)

	loc := &FSGitCookbookLocator{BaseDir: base}
	got := loc.LocateCookbook("nginx")
	if got != filepath.Join(base, "nginx") {
		t.Errorf("expected path, got %q", got)
	}
}

func TestFSGitCookbookLocator_NotFound(t *testing.T) {
	loc := &FSGitCookbookLocator{BaseDir: t.TempDir()}
	if got := loc.LocateCookbook("nonexistent"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFSGitCookbookLocator_EmptyBaseDir(t *testing.T) {
	loc := &FSGitCookbookLocator{BaseDir: ""}
	if got := loc.LocateCookbook("anything"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// WriteRoles
// ---------------------------------------------------------------------------

func TestWriteRoles(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, "roles"), 0o755)

	fetcher := &mockRoleFetcher{
		roles: map[string]*chefapi.RoleDetail{
			"webserver": {
				Name:        "webserver",
				Description: "Web server role",
				RunList:     []string{"recipe[nginx]"},
			},
		},
	}

	err := WriteRoles(context.Background(), workDir, []string{"webserver"}, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(workDir, "roles", "webserver.json"))
	if err != nil {
		t.Fatalf("reading role file: %v", err)
	}

	var rj roleJSON
	if err := json.Unmarshal(data, &rj); err != nil {
		t.Fatalf("unmarshalling role JSON: %v", err)
	}

	if rj.Name != "webserver" {
		t.Errorf("name: got %q", rj.Name)
	}
	if rj.JSONClass != "Chef::Role" {
		t.Errorf("json_class: got %q", rj.JSONClass)
	}
	if rj.ChefType != "role" {
		t.Errorf("chef_type: got %q", rj.ChefType)
	}
	if rj.Description != "Web server role" {
		t.Errorf("description: got %q", rj.Description)
	}
	if len(rj.RunList) != 1 || rj.RunList[0] != "recipe[nginx]" {
		t.Errorf("run_list: got %v", rj.RunList)
	}
	if len(rj.DefaultAttributes) != 0 {
		t.Errorf("default_attributes should be empty, got %v", rj.DefaultAttributes)
	}
	if len(rj.OverrideAttributes) != 0 {
		t.Errorf("override_attributes should be empty, got %v", rj.OverrideAttributes)
	}
	if rj.EnvRunLists == nil {
		t.Error("env_run_lists should not be nil")
	}
}

func TestWriteRoles_MissingRole(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, "roles"), 0o755)

	fetcher := &mockRoleFetcher{roles: map[string]*chefapi.RoleDetail{}}
	err := WriteRoles(context.Background(), workDir, []string{"missing"}, fetcher)
	if err == nil {
		t.Fatal("expected error for missing role")
	}
}

func TestWriteRoles_RejectsPathTraversal(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, "roles"), 0o755)

	const evil = "../../../tmp/evil"
	fetcher := &mockRoleFetcher{
		roles: map[string]*chefapi.RoleDetail{
			evil: {Name: evil, RunList: []string{}},
		},
	}

	err := WriteRoles(context.Background(), workDir, []string{evil}, fetcher)
	if err == nil {
		t.Fatal("expected error for traversal role name, got nil")
	}
	// Ensure nothing was written outside the roles dir.
	if _, statErr := os.Stat(filepath.Join(workDir, "..", "..", "..", "tmp", "evil.json")); statErr == nil {
		t.Fatal("traversal role name escaped the work directory")
	}
}

// ---------------------------------------------------------------------------
// AssembleCookbooks — server mode
// ---------------------------------------------------------------------------

func TestAssembleCookbooks_ServerMode(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, "cookbooks"), 0o755)

	dl := &mockCBDownloader{}
	cookbooks := map[string]string{"nginx": "1.0.0", "apt": "2.0.0"}

	err := AssembleCookbooks(context.Background(), workDir, cookbooks, AssemblyConfig{
		CookbookSource: "server",
		Concurrency:    1,
	}, dl, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, name := range []string{"nginx", "apt"} {
		ver := cookbooks[name]
		key := name + "/" + ver
		if _, ok := dl.downloaded[key]; !ok {
			t.Errorf("expected download for %s", key)
		}
		meta := filepath.Join(workDir, "cookbooks", name, "metadata.rb")
		if _, err := os.Stat(meta); err != nil {
			t.Errorf("expected metadata.rb for %s: %v", name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// AssembleCookbooks — git mode
// ---------------------------------------------------------------------------

func TestAssembleCookbooks_GitMode(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, "cookbooks"), 0o755)

	// Set up a fake git cookbook source dir.
	gitBase := t.TempDir()
	nginxSrc := filepath.Join(gitBase, "nginx")
	os.MkdirAll(nginxSrc, 0o755)
	os.WriteFile(filepath.Join(nginxSrc, "metadata.rb"), []byte("name 'nginx'"), 0o644)

	locator := &assembleMockGitLocator{paths: map[string]string{"nginx": nginxSrc}}

	err := AssembleCookbooks(context.Background(), workDir, map[string]string{"nginx": "1.0.0"}, AssemblyConfig{
		CookbookSource: "git",
		Concurrency:    1,
	}, nil, locator)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	meta := filepath.Join(workDir, "cookbooks", "nginx", "metadata.rb")
	data, err := os.ReadFile(meta)
	if err != nil {
		t.Fatalf("expected copied metadata.rb: %v", err)
	}
	if string(data) != "name 'nginx'" {
		t.Errorf("unexpected content: %q", data)
	}
}

// ---------------------------------------------------------------------------
// AssembleCookbooks — hybrid mode
// ---------------------------------------------------------------------------

func TestAssembleCookbooks_HybridMode(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, "cookbooks"), 0o755)

	// nginx available in git, apt only on server.
	gitBase := t.TempDir()
	nginxSrc := filepath.Join(gitBase, "nginx")
	os.MkdirAll(nginxSrc, 0o755)
	os.WriteFile(filepath.Join(nginxSrc, "metadata.rb"), []byte("name 'nginx'"), 0o644)

	locator := &assembleMockGitLocator{paths: map[string]string{"nginx": nginxSrc}}
	dl := &mockCBDownloader{}

	cookbooks := map[string]string{"nginx": "1.0.0", "apt": "2.0.0"}
	err := AssembleCookbooks(context.Background(), workDir, cookbooks, AssemblyConfig{
		CookbookSource: "hybrid",
		Concurrency:    1,
	}, dl, locator)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// nginx should come from git (no download call).
	if _, ok := dl.downloaded["nginx/1.0.0"]; ok {
		t.Error("nginx should not have been downloaded in hybrid mode")
	}

	// apt should come from server.
	if _, ok := dl.downloaded["apt/2.0.0"]; !ok {
		t.Error("apt should have been downloaded from server")
	}

	// Both should exist on disk.
	for _, name := range []string{"nginx", "apt"} {
		meta := filepath.Join(workDir, "cookbooks", name, "metadata.rb")
		if _, err := os.Stat(meta); err != nil {
			t.Errorf("expected metadata.rb for %s: %v", name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// AssembleCookbooks — empty map
// ---------------------------------------------------------------------------

func TestAssembleCookbooks_Empty(t *testing.T) {
	err := AssembleCookbooks(context.Background(), t.TempDir(), nil, AssemblyConfig{
		CookbookSource: "server",
	}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error for empty cookbooks: %v", err)
	}
}

// ---------------------------------------------------------------------------
// assembleSingleCookbook — unknown source
// ---------------------------------------------------------------------------

func TestAssembleSingleCookbook_UnknownSource(t *testing.T) {
	err := assembleSingleCookbook(context.Background(), "test", "1.0", t.TempDir(), AssemblyConfig{
		CookbookSource: "floppy_disk",
	}, nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown cookbook source")
	}
	if !strings.Contains(err.Error(), "unknown cookbook source") {
		t.Errorf("unexpected error message: %v", err)
	}
}
