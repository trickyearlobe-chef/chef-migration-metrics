// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package frontend provides the built React SPA assets for serving from the
// Go binary. It supports two modes:
//
//  1. Embedded FS — when a compiled-in embed.FS is registered via
//     RegisterEmbedFS at init time (see frontend/embed.go), the assets are
//     baked into the binary and no filesystem access is needed at runtime.
//
//  2. Disk FS — when no embedded FS is registered, FS() falls back to
//     os.DirFS, reading assets from a directory on disk. This is useful
//     during development (where the Vite dev server rebuilds on the fly)
//     and in container/package deployments where frontend/dist is shipped
//     alongside the binary.
//
// The router calls FS() at startup and passes the result (which may be nil)
// to webapi.WithFrontendFS. A nil return means "no frontend available" and
// the router serves a plain-text placeholder instead.
package frontend

import (
	"io/fs"
	"os"
	"sync"
)

// DistDir is the default path where the Vite build output is expected
// when falling back to disk mode. In Docker this is frontend/dist relative
// to the working directory; in RPM/DEB installs it can be overridden.
const DistDir = "frontend/dist"

// mu protects the package-level embedFS variable.
var mu sync.RWMutex

// embedFS holds the compiled-in frontend assets, if any. It is set once
// during init by the frontend/embed.go file (which lives next to
// package.json and can see dist/ for the go:embed directive). When the
// frontend has not been built or the embed.go file is excluded by build
// tags, this remains nil.
var embedFS fs.FS

// RegisterEmbedFS is called (typically from an init function) to provide
// the compiled-in SPA assets. It must be called before FS() is used —
// in practice this means it runs during package initialisation before
// main() starts. It is safe to call from multiple goroutines but only
// the first non-nil registration takes effect.
func RegisterEmbedFS(fsys fs.FS) {
	if fsys == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if embedFS == nil {
		embedFS = fsys
	}
}

// HasEmbed reports whether compiled-in frontend assets are available.
func HasEmbed() bool {
	mu.RLock()
	defer mu.RUnlock()
	return embedFS != nil
}

// FS returns an fs.FS containing the built React SPA assets, or nil if
// no assets are available.
//
// Resolution order:
//  1. If an embedded FS was registered via RegisterEmbedFS, return it.
//  2. If the given directory exists on disk, return os.DirFS(dir).
//  3. Return nil — the caller should serve a "frontend not built" message.
func FS(dir string) fs.FS {
	// Prefer compiled-in assets — they are always up to date with the
	// binary and don't require files on disk.
	mu.RLock()
	efs := embedFS
	mu.RUnlock()
	if efs != nil {
		return efs
	}

	// Fall back to reading from disk.
	return diskFS(dir)
}

// diskFS returns an os.DirFS rooted at dir, or nil if the directory does
// not exist or is not a directory.
func diskFS(dir string) fs.FS {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return os.DirFS(dir)
}
