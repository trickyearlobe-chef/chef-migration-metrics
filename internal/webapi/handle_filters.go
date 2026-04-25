// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"sort"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Filter endpoints — each returns a sorted list of distinct values drawn
// from the latest node snapshots across all organisations. The response
// shape is always {"data": ["value1", "value2", ...]}.
//
// These endpoints use SQL DISTINCT queries pushed down to Postgres for
// scalability at 100k+ nodes, replacing the previous in-memory approach
// that loaded all node snapshots.
// ---------------------------------------------------------------------------

// handleFilterEnvironments handles GET /api/v1/filters/environments.
func (r *Router) handleFilterEnvironments(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	f, err := r.filterOrgIDs(req)
	if err != nil {
		r.logf("ERROR", "resolving orgs for filter environments: %v", err)
		WriteInternalError(w, "Failed to list environments.")
		return
	}
	q := req.URL.Query().Get("q")
	opts := datastore.DistinctValueOpts{SearchPrefix: q}
	if q != "" {
		opts.Limit = 50
	}
	values, err := r.db.ListDistinctNodeValues(req.Context(), f, "cn.chef_environment", opts)
	if err != nil {
		r.logf("ERROR", "listing distinct environments: %v", err)
		WriteInternalError(w, "Failed to list environments.")
		return
	}
	if values == nil {
		values = []string{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": values})
}

// handleFilterRoles handles GET /api/v1/filters/roles.
func (r *Router) handleFilterRoles(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	f, err := r.filterOrgIDs(req)
	if err != nil {
		r.logf("ERROR", "resolving orgs for filter roles: %v", err)
		WriteInternalError(w, "Failed to list roles.")
		return
	}
	q := req.URL.Query().Get("q")
	opts := datastore.DistinctValueOpts{SearchPrefix: q}
	if q != "" {
		opts.Limit = 50
	}
	values, err := r.db.ListDistinctNodeRoles(req.Context(), f, opts)
	if err != nil {
		r.logf("ERROR", "listing distinct roles: %v", err)
		WriteInternalError(w, "Failed to list roles.")
		return
	}
	if values == nil {
		values = []string{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": values})
}

// handleFilterPolicyNames handles GET /api/v1/filters/policy-names.
func (r *Router) handleFilterPolicyNames(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	f, err := r.filterOrgIDs(req)
	if err != nil {
		r.logf("ERROR", "resolving orgs for filter policy names: %v", err)
		WriteInternalError(w, "Failed to list policy names.")
		return
	}
	q := req.URL.Query().Get("q")
	opts := datastore.DistinctValueOpts{SearchPrefix: q}
	if q != "" {
		opts.Limit = 50
	}
	values, err := r.db.ListDistinctNodeValues(req.Context(), f, "cn.policy_name", opts)
	if err != nil {
		r.logf("ERROR", "listing distinct policy names: %v", err)
		WriteInternalError(w, "Failed to list policy names.")
		return
	}
	if values == nil {
		values = []string{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": values})
}

// handleFilterPolicyGroups handles GET /api/v1/filters/policy-groups.
func (r *Router) handleFilterPolicyGroups(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	f, err := r.filterOrgIDs(req)
	if err != nil {
		r.logf("ERROR", "resolving orgs for filter policy groups: %v", err)
		WriteInternalError(w, "Failed to list policy groups.")
		return
	}
	q := req.URL.Query().Get("q")
	opts := datastore.DistinctValueOpts{SearchPrefix: q}
	if q != "" {
		opts.Limit = 50
	}
	values, err := r.db.ListDistinctNodeValues(req.Context(), f, "cn.policy_group", opts)
	if err != nil {
		r.logf("ERROR", "listing distinct policy groups: %v", err)
		WriteInternalError(w, "Failed to list policy groups.")
		return
	}
	if values == nil {
		values = []string{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": values})
}

// handleFilterPlatforms handles GET /api/v1/filters/platforms.
// Returns combined "platform platform_version" strings.
func (r *Router) handleFilterPlatforms(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	f, err := r.filterOrgIDs(req)
	if err != nil {
		r.logf("ERROR", "resolving orgs for filter platforms: %v", err)
		WriteInternalError(w, "Failed to list platforms.")
		return
	}
	// Use the same combined expression as the platform distribution query.
	expr := `CASE WHEN cn.platform IS NULL OR cn.platform = '' THEN NULL
	              WHEN cn.platform_version IS NOT NULL AND cn.platform_version != '' THEN cn.platform || ' ' || cn.platform_version
	              ELSE cn.platform
	         END`
	q := req.URL.Query().Get("q")
	opts := datastore.DistinctValueOpts{SearchPrefix: q}
	if q != "" {
		opts.Limit = 50
	}
	values, err := r.db.ListDistinctNodeValues(req.Context(), f, expr, opts)
	if err != nil {
		r.logf("ERROR", "listing distinct platforms: %v", err)
		WriteInternalError(w, "Failed to list platforms.")
		return
	}
	if values == nil {
		values = []string{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": values})
}

// handleFilterTargetChefVersions handles GET /api/v1/filters/target-chef-versions.
// Unlike the other filters that are derived from node snapshots, this returns
// the target Chef versions from the application configuration.
func (r *Router) handleFilterTargetChefVersions(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	versions := make([]string, len(r.cfg.TargetChefVersions))
	copy(versions, r.cfg.TargetChefVersions)
	sort.Strings(versions)
	WriteJSON(w, http.StatusOK, map[string]any{"data": versions})
}

// handleFilterComplexityLabels handles GET /api/v1/filters/complexity-labels.
// Returns the well-known set of complexity labels used by the system.
func (r *Router) handleFilterComplexityLabels(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	// These are the canonical complexity labels used by the cookbook
	// complexity scoring system, ordered from simplest to most complex.
	labels := []string{
		"trivial",
		"simple",
		"moderate",
		"complex",
		"very_complex",
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": labels})
}

// ---------------------------------------------------------------------------
// Shared helpers for filter collection
// ---------------------------------------------------------------------------

// filterOrgIDs resolves the organisation filter from the request and returns
// a NodeSnapshotFilter populated with the matching organisation IDs. This is
// used by filter endpoints that only need org scoping.
func (r *Router) filterOrgIDs(req *http.Request) (datastore.NodeSnapshotFilter, error) {
	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		return datastore.NodeSnapshotFilter{}, err
	}
	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.Name)
	}
	return datastore.NodeSnapshotFilter{OrganisationNames: orgIDs}, nil
}
