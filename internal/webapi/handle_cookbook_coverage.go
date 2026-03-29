// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
)

// ---------------------------------------------------------------------------
// Cookbook Platform Coverage endpoint
//
// GET /api/v1/cookbooks/:name/platform-coverage
//
// Returns the platform coverage analysis for a cookbook — comparing
// kitchen-tested platforms against production node platforms. See
// test-kitchen-drivers.md § Platform Coverage Analysis.
// ---------------------------------------------------------------------------

// coverageAPIResponse is the wire format for the platform coverage endpoint.
// It decouples the API from the datastore schema, omitting internal fields
// (id, created_at, updated_at) and properly typing the coverage data so
// integer fields are not serialised as float64.
type coverageAPIResponse struct {
	CookbookName         string                    `json:"cookbook_name"`
	EvaluatedAt          string                    `json:"evaluated_at"`
	KitchenPlatforms     []string                  `json:"kitchen_platforms"`
	ProductionPlatforms  []coverageAPIProdPlatform `json:"production_platforms"`
	TestedAndInProd      []coverageAPITestedMatch  `json:"tested_and_in_production"`
	TestedNotInProd      []string                  `json:"tested_not_in_production"`
	InProdNotTested      []coverageAPIProdPlatform `json:"in_production_not_tested"`
	GapCount             int                       `json:"gap_count"`
	TotalProductionNodes int                       `json:"total_production_nodes"`
	CoveredNodeCount     int                       `json:"covered_node_count"`
	CoveragePercentage   float64                   `json:"coverage_percentage"`
}

// coverageAPIProdPlatform is a production platform entry in the API response.
type coverageAPIProdPlatform struct {
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	PlatformFamily  string `json:"platform_family,omitempty"`
	NodeCount       int    `json:"node_count"`
}

// coverageAPITestedMatch records a match between a kitchen platform and
// production in the API response.
type coverageAPITestedMatch struct {
	KitchenName     string `json:"kitchen_name"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	NodeCount       int    `json:"node_count"`
}

// handleCookbookPlatformCoverage handles GET /api/v1/cookbooks/:name/platform-coverage.
func (r *Router) handleCookbookPlatformCoverage(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	// Extract path segments: /api/v1/cookbooks/{name}/platform-coverage
	segments := pathSegments(req.URL.Path, "/api/v1/cookbooks/")
	if len(segments) < 2 || segments[len(segments)-1] != "platform-coverage" {
		WriteNotFound(w, "Expected path: /api/v1/cookbooks/:name/platform-coverage")
		return
	}

	cookbookName := segments[0]
	if cookbookName == "" {
		WriteBadRequest(w, "Cookbook name is required.")
		return
	}

	ctx := req.Context()

	coverage, err := r.db.GetCookbookPlatformCoverage(ctx, cookbookName)
	if err != nil {
		r.logf("ERROR", "failed to get platform coverage for %q: %v", cookbookName, err)
		WriteInternalError(w, "Failed to retrieve platform coverage.")
		return
	}

	if coverage == nil {
		WriteNotFound(w, "No platform coverage data available for cookbook "+cookbookName+".")
		return
	}

	resp, err := buildCoverageAPIResponse(coverage.CookbookName, coverage.EvaluatedAt.Format("2006-01-02T15:04:05Z"), coverage.CoverageData)
	if err != nil {
		r.logf("ERROR", "failed to convert platform coverage for %q: %v", cookbookName, err)
		WriteInternalError(w, "Failed to process platform coverage data.")
		return
	}

	WriteJSON(w, http.StatusOK, resp)
}

// buildCoverageAPIResponse converts the raw coverage_data (which round-trips
// through any/json.Unmarshal and loses integer types) into a properly typed
// API response. It re-marshals and re-unmarshals through the typed struct so
// json.Number / float64 values are coerced to the correct Go types.
func buildCoverageAPIResponse(cookbookName, evaluatedAt string, coverageData any) (*coverageAPIResponse, error) {
	// Re-marshal the untyped coverage data back to JSON bytes, then
	// unmarshal into the typed struct. This corrects float64→int
	// round-trip issues from the JSONB any path.
	raw, err := json.Marshal(coverageData)
	if err != nil {
		return nil, err
	}

	var typed struct {
		KitchenPlatforms     []string                  `json:"kitchen_platforms"`
		ProductionPlatforms  []coverageAPIProdPlatform `json:"production_platforms"`
		TestedAndInProd      []coverageAPITestedMatch  `json:"tested_and_in_production"`
		TestedNotInProd      []string                  `json:"tested_not_in_production"`
		InProdNotTested      []coverageAPIProdPlatform `json:"in_production_not_tested"`
		GapCount             int                       `json:"gap_count"`
		TotalProductionNodes int                       `json:"total_production_nodes"`
		CoveredNodeCount     int                       `json:"covered_node_count"`
		CoveragePercentage   float64                   `json:"coverage_percentage"`
	}

	if err := json.Unmarshal(raw, &typed); err != nil {
		return nil, err
	}

	// Ensure slices are non-nil so JSON output uses [] not null.
	if typed.KitchenPlatforms == nil {
		typed.KitchenPlatforms = []string{}
	}
	if typed.ProductionPlatforms == nil {
		typed.ProductionPlatforms = []coverageAPIProdPlatform{}
	}
	if typed.TestedAndInProd == nil {
		typed.TestedAndInProd = []coverageAPITestedMatch{}
	}
	if typed.TestedNotInProd == nil {
		typed.TestedNotInProd = []string{}
	}
	if typed.InProdNotTested == nil {
		typed.InProdNotTested = []coverageAPIProdPlatform{}
	}

	return &coverageAPIResponse{
		CookbookName:         cookbookName,
		EvaluatedAt:          evaluatedAt,
		KitchenPlatforms:     typed.KitchenPlatforms,
		ProductionPlatforms:  typed.ProductionPlatforms,
		TestedAndInProd:      typed.TestedAndInProd,
		TestedNotInProd:      typed.TestedNotInProd,
		InProdNotTested:      typed.InProdNotTested,
		GapCount:             typed.GapCount,
		TotalProductionNodes: typed.TotalProductionNodes,
		CoveredNodeCount:     typed.CoveredNodeCount,
		CoveragePercentage:   typed.CoveragePercentage,
	}, nil
}
