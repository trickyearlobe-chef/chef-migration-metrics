// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package frontend provides the built React SPA assets for embedding into
// the Go binary. During development the frontend may not be built, so the
// package gracefully handles a missing dist/ directory.
package frontend

import (
	"io/fs"
	"os"
)

// DistDir is the default path where the Vite build output is expected.
// In Docker this is /src/frontend/dist; in local dev it's ./frontend/dist.
const DistDir = "frontend/dist"

// FS returns an fs.FS rooted at the given directory, or nil if the
// directory does not exist. The caller should check for nil and fall
// back to a "frontend not built" message.
func FS(dir string) fs.FS {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return os.DirFS(dir)
}
