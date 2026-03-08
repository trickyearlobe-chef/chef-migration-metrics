// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package frontendfs

import (
	"io/fs"
	"testing"
)

// TestDistFS_ReturnsValidFS verifies that the go:embed directive in embed.go
// successfully compiled and that DistFS() returns a usable filesystem
// containing at least an index.html file.
func TestDistFS_ReturnsValidFS(t *testing.T) {
	fsys, err := DistFS()
	if err != nil {
		t.Fatalf("DistFS() error: %v", err)
	}
	if fsys == nil {
		t.Fatal("DistFS() returned nil filesystem")
	}

	// The dist/ directory must contain an index.html — either the real
	// Vite build output or the Makefile-generated placeholder.
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatalf("opening index.html from embedded FS: %v", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		t.Fatalf("stat index.html: %v", err)
	}
	if stat.IsDir() {
		t.Fatal("index.html is a directory; expected a file")
	}
	if stat.Size() == 0 {
		t.Fatal("index.html is empty; expected content")
	}
}

// TestDistFS_IndexHTMLIsReadable verifies we can read the full content of
// index.html from the embedded filesystem.
func TestDistFS_IndexHTMLIsReadable(t *testing.T) {
	fsys, err := DistFS()
	if err != nil {
		t.Fatalf("DistFS() error: %v", err)
	}

	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		t.Fatalf("ReadFile(index.html): %v", err)
	}
	if len(data) == 0 {
		t.Fatal("index.html content is empty")
	}

	// Sanity-check that the content looks like HTML.
	content := string(data)
	if len(content) < 15 {
		t.Fatalf("index.html content too short (%d bytes): %q", len(data), content)
	}
}

// TestDistFS_EmbedContentVar verifies the package-level embed.FS variable
// is populated (not zero-value). We read the "dist" directory entry from it
// directly to confirm the go:embed directive worked.
func TestDistFS_EmbedContentVar(t *testing.T) {
	entries, err := fs.ReadDir(content, "dist")
	if err != nil {
		t.Fatalf("reading embedded dist/ directory: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embedded dist/ directory is empty; expected at least index.html")
	}

	found := false
	for _, e := range entries {
		if e.Name() == "index.html" {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("index.html not found in embedded dist/; entries: %v", names)
	}
}
