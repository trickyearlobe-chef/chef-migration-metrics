// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	frontendfs "github.com/trickyearlobe-chef/chef-migration-metrics/frontend"
)

func init() {
	fsys, err := frontendfs.DistFS()
	if err != nil {
		// This should only happen if the embed.go file has a bug in the
		// directory name passed to fs.Sub. Log and continue — the router
		// will fall back to disk mode or the plain-text placeholder.
		return
	}

	// Check that the embedded FS actually contains content (i.e. the
	// frontend was built before "go build" ran, not just the placeholder
	// directory). We probe for a Vite-generated asset directory — if it
	// exists we know the real SPA was embedded rather than an empty dir.
	// Even if the probe fails, we register anyway — the placeholder
	// index.html is still better than nothing.
	RegisterEmbedFS(fsys)
}
