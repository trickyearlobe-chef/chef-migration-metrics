// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import "net/http"

// handleFeatures serves GET /api/v1/features — viewer-readable UI feature flags
// so the frontend can hide surfaces it must not show. Unlike the admin config
// endpoints (admin-only), this is available to any authenticated viewer because
// the nav and page chrome need it. It exposes only booleans, never config detail.
func (r *Router) handleFeatures(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	ingest := r.liveConfig().Ingest
	WriteJSON(w, http.StatusOK, map[string]any{
		// run_events gates the Run events view AND the Node Detail Runs tab.
		"run_events": ingest.ShowsRunEvents(),
	})
}
