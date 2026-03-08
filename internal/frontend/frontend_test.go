// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// ---------------------------------------------------------------------------
// RegisterEmbedFS + HasEmbed
// ---------------------------------------------------------------------------

func TestRegisterEmbedFS_NilIsIgnored(t *testing.T) {
	// Save and restore package state so tests are independent.
	orig := embedFS
	t.Cleanup(func() {
		mu.Lock()
		embedFS = orig
		mu.Unlock()
	})

	mu.Lock()
	embedFS = nil
	mu.Unlock()

	RegisterEmbedFS(nil)

	if HasEmbed() {
		t.Fatal("HasEmbed() = true after registering nil; want false")
	}
}

func TestRegisterEmbedFS_SetsFS(t *testing.T) {
	orig := embedFS
	t.Cleanup(func() {
		mu.Lock()
		embedFS = orig
		mu.Unlock()
	})

	mu.Lock()
	embedFS = nil
	mu.Unlock()

	fake := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
	}
	RegisterEmbedFS(fake)

	if !HasEmbed() {
		t.Fatal("HasEmbed() = false after registering a non-nil FS; want true")
	}
}

func TestRegisterEmbedFS_OnlyFirstWins(t *testing.T) {
	orig := embedFS
	t.Cleanup(func() {
		mu.Lock()
		embedFS = orig
		mu.Unlock()
	})

	mu.Lock()
	embedFS = nil
	mu.Unlock()

	first := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("first")},
	}
	second := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("second")},
	}

	RegisterEmbedFS(first)
	RegisterEmbedFS(second)

	if !HasEmbed() {
		t.Fatal("HasEmbed() = false; want true")
	}

	// Read through FS("nonexistent-dir") which should return the embed
	// since disk fallback won't find anything.
	fsys := FS("nonexistent-dir-that-does-not-exist")
	if fsys == nil {
		t.Fatal("FS() returned nil; expected embedded FS")
	}
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		t.Fatalf("reading index.html from embedded FS: %v", err)
	}
	if string(data) != "first" {
		t.Fatalf("index.html content = %q; want %q (first registration should win)", string(data), "first")
	}
}

// ---------------------------------------------------------------------------
// FS — embedded preferred over disk
// ---------------------------------------------------------------------------

func TestFS_PrefersEmbedOverDisk(t *testing.T) {
	orig := embedFS
	t.Cleanup(func() {
		mu.Lock()
		embedFS = orig
		mu.Unlock()
	})

	// Create a real directory on disk with different content.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("disk"), 0644); err != nil {
		t.Fatal(err)
	}

	// Register an embedded FS with different content.
	mu.Lock()
	embedFS = nil
	mu.Unlock()

	RegisterEmbedFS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("embedded")},
	})

	fsys := FS(dir)
	if fsys == nil {
		t.Fatal("FS() returned nil")
	}
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}
	if string(data) != "embedded" {
		t.Fatalf("FS() returned disk content %q; want embedded content %q", string(data), "embedded")
	}
}

// ---------------------------------------------------------------------------
// FS — disk fallback when no embed registered
// ---------------------------------------------------------------------------

func TestFS_FallsThroughToDisk(t *testing.T) {
	orig := embedFS
	t.Cleanup(func() {
		mu.Lock()
		embedFS = orig
		mu.Unlock()
	})

	mu.Lock()
	embedFS = nil
	mu.Unlock()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("from-disk"), 0644); err != nil {
		t.Fatal(err)
	}

	fsys := FS(dir)
	if fsys == nil {
		t.Fatal("FS() returned nil; expected disk FS")
	}
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		t.Fatalf("reading index.html from disk FS: %v", err)
	}
	if string(data) != "from-disk" {
		t.Fatalf("index.html = %q; want %q", string(data), "from-disk")
	}
}

// ---------------------------------------------------------------------------
// FS — nil when nothing available
// ---------------------------------------------------------------------------

func TestFS_ReturnsNilWhenNothingAvailable(t *testing.T) {
	orig := embedFS
	t.Cleanup(func() {
		mu.Lock()
		embedFS = orig
		mu.Unlock()
	})

	mu.Lock()
	embedFS = nil
	mu.Unlock()

	fsys := FS("/nonexistent/path/that/does/not/exist")
	if fsys != nil {
		t.Fatal("FS() returned non-nil for missing dir and no embed; want nil")
	}
}

// ---------------------------------------------------------------------------
// diskFS
// ---------------------------------------------------------------------------

func TestDiskFS_ReturnsNilForMissingDir(t *testing.T) {
	if got := diskFS("/nonexistent/path/surely"); got != nil {
		t.Fatalf("diskFS() = %v; want nil for missing directory", got)
	}
}

func TestDiskFS_ReturnsNilForFile(t *testing.T) {
	// diskFS should reject a path that is a regular file, not a directory.
	f, err := os.CreateTemp(t.TempDir(), "notadir")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if got := diskFS(f.Name()); got != nil {
		t.Fatalf("diskFS() = %v for a regular file; want nil", got)
	}
}

func TestDiskFS_ReturnsValidFS(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello world")
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), content, 0644); err != nil {
		t.Fatal(err)
	}

	fsys := diskFS(dir)
	if fsys == nil {
		t.Fatal("diskFS() returned nil for a valid directory")
	}

	data, err := fs.ReadFile(fsys, "test.txt")
	if err != nil {
		t.Fatalf("reading test.txt: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("test.txt = %q; want %q", string(data), string(content))
	}
}

// ---------------------------------------------------------------------------
// HasEmbed — false when nothing registered
// ---------------------------------------------------------------------------

func TestHasEmbed_FalseByDefault(t *testing.T) {
	orig := embedFS
	t.Cleanup(func() {
		mu.Lock()
		embedFS = orig
		mu.Unlock()
	})

	mu.Lock()
	embedFS = nil
	mu.Unlock()

	if HasEmbed() {
		t.Fatal("HasEmbed() = true with no registration; want false")
	}
}

// ---------------------------------------------------------------------------
// DistDir constant
// ---------------------------------------------------------------------------

func TestDistDirConstant(t *testing.T) {
	if DistDir != "frontend/dist" {
		t.Fatalf("DistDir = %q; want %q", DistDir, "frontend/dist")
	}
}
