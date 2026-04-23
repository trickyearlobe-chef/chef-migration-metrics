// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"strconv"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// handleKitchenAnalysisSummary handles GET /api/v1/kitchen/analysis/summary.
func (r *Router) handleKitchenAnalysisSummary(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	ctx := req.Context()
	summary, err := r.db.GetKitchenAnalysisSummary(ctx)
	if err != nil {
		r.logf("ERROR", "failed to get kitchen analysis summary: %v", err)
		WriteInternalError(w, "Failed to retrieve kitchen analysis summary.")
		return
	}
	WriteJSON(w, http.StatusOK, summary)
}

// handleKitchenAnalysisPlatforms handles GET /api/v1/kitchen/analysis/platforms.
func (r *Router) handleKitchenAnalysisPlatforms(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	ctx := req.Context()

	osFamily := req.URL.Query().Get("os_family")
	minCount := 0
	if mc := req.URL.Query().Get("min_count"); mc != "" {
		var err error
		minCount, err = strconv.Atoi(mc)
		if err != nil || minCount < 0 {
			WriteBadRequest(w, "min_count must be a non-negative integer")
			return
		}
	}

	var platforms []datastore.KitchenDiscoveredPlatform
	var err error
	if osFamily != "" || minCount > 0 {
		platforms, err = r.db.ListDiscoveredPlatformsFiltered(ctx, osFamily, minCount)
	} else {
		platforms, err = r.db.ListDiscoveredPlatforms(ctx)
	}
	if err != nil {
		r.logf("ERROR", "failed to list discovered platforms: %v", err)
		WriteInternalError(w, "Failed to retrieve discovered platforms.")
		return
	}
	if platforms == nil {
		platforms = []datastore.KitchenDiscoveredPlatform{}
	}
	WriteJSON(w, http.StatusOK, platforms)
}

// handleKitchenAnalysisCookbooks handles GET /api/v1/kitchen/analysis/cookbooks.
func (r *Router) handleKitchenAnalysisCookbooks(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	ctx := req.Context()

	driverName := req.URL.Query().Get("driver")
	var hasLocalOverride *bool
	if v := req.URL.Query().Get("has_local_override"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			WriteBadRequest(w, "has_local_override must be a boolean (true/false)")
			return
		}
		hasLocalOverride = &b
	}

	var results []datastore.KitchenAnalysisResult
	var err error
	if driverName != "" || hasLocalOverride != nil {
		results, err = r.db.ListKitchenAnalysisResultsFiltered(ctx, driverName, hasLocalOverride)
	} else {
		results, err = r.db.ListKitchenAnalysisResults(ctx)
	}
	if err != nil {
		r.logf("ERROR", "failed to list kitchen analysis results: %v", err)
		WriteInternalError(w, "Failed to retrieve kitchen analysis results.")
		return
	}
	if results == nil {
		results = []datastore.KitchenAnalysisResult{}
	}
	WriteJSON(w, http.StatusOK, results)
}

// handleKitchenAnalysisCookbookDetail handles
// GET /api/v1/kitchen/analysis/cookbooks/{name}.
func (r *Router) handleKitchenAnalysisCookbookDetail(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	segments := pathSegments(req.URL.Path, "/api/v1/kitchen/analysis/cookbooks/")
	if len(segments) == 0 || segments[0] == "" {
		WriteBadRequest(w, "Cookbook name is required.")
		return
	}
	name := segments[0]

	ctx := req.Context()
	result, err := r.db.GetKitchenAnalysisResultByName(ctx, name)
	if err != nil {
		r.logf("ERROR", "failed to get kitchen analysis result for %q: %v", name, err)
		WriteInternalError(w, "Failed to retrieve kitchen analysis result.")
		return
	}
	if result == nil {
		WriteNotFound(w, "No kitchen analysis result found for "+name+".")
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

// handleKitchenAnalysisCookbooksRouter dispatches requests under the
// /api/v1/kitchen/analysis/cookbooks prefix to either the list or detail
// handler based on the URL path.
func (r *Router) handleKitchenAnalysisCookbooksRouter(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/api/v1/kitchen/analysis/cookbooks" {
		r.handleKitchenAnalysisCookbooks(w, req)
		return
	}
	r.handleKitchenAnalysisCookbookDetail(w, req)
}

// handleKitchenAnalysisTrigger handles POST /api/v1/kitchen/analysis/trigger.
// This is a placeholder that returns 202 Accepted; the actual trigger logic
// will be wired when the analyser is fully integrated.
func (r *Router) handleKitchenAnalysisTrigger(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}
	WriteJSON(w, http.StatusAccepted, map[string]string{
		"status":  "accepted",
		"message": "Kitchen analysis will run on the next collection cycle.",
	})
}
