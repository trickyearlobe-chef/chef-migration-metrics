// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package frontendfs embeds the built React SPA assets (Vite output) into
// the Go binary. The //go:embed directive references the dist/ directory
// which is created by "npm run build" (or by the Makefile's build-frontend
// target which creates a placeholder when npm is unavailable).
//
// This package lives inside frontend/ (alongside package.json) because
// go:embed paths are relative to the source file and cannot use "..".
// Placing it here lets the directive reference dist/ directly.
//
// The internal/frontend package imports this and calls RegisterEmbedFS
// during init so the router receives the embedded assets automatically.
package frontendfs

import (
	"embed"
	"io/fs"
)

// content holds the entire dist/ directory tree. The "all:" prefix includes
// files starting with "." or "_" which Vite may produce (e.g. _assets/).
//
//go:embed all:dist
var content embed.FS

// DistFS returns an fs.FS rooted at the contents of dist/. The returned
// filesystem has index.html, assets/, etc. at its root — ready to be
// served by http.FileServerFS or the router's frontend fallback handler.
func DistFS() (fs.FS, error) {
	return fs.Sub(content, "dist")
}
