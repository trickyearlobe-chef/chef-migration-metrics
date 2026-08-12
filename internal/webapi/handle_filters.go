// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"sort"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	platformPkg "github.com/trickyearlobe-chef/chef-migration-metrics/internal/platform"
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

// filterValuesResponse is what a filter can be set to: the values actually
// present in what we hold, so a caller offering a choice offers one that
// matches something. Always a list, empty when nothing matches, never null.
type filterValuesResponse struct {
	Data []string `json:"data"`
}

// platformFilterEntry is one operating system, with the friendlier name and
// the family it belongs to where anything maps it. DisplayName is null rather
// than blank where nothing does.
type platformFilterEntry struct {
	Value            string  `json:"value"`
	DisplayName      *string `json:"display_name"`
	GroupKey         string  `json:"group_key,omitempty"`
	GroupDisplayName string  `json:"group_display_name,omitempty"`
}

type platformFilterResponse struct {
	Data []platformFilterEntry `json:"data"`
}

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
	WriteJSON(w, http.StatusOK, filterValuesResponse{Data: values})
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
	WriteJSON(w, http.StatusOK, filterValuesResponse{Data: values})
}

// handleFilterTags handles GET /api/v1/filters/tags. Unlike roles, the tags
// facet is always bounded and count-ranked (see node-tags.md): a server cap is
// applied whether or not a prefix is supplied, so a fleet with thousands of
// distinct tags never degrades the filter UI.
func (r *Router) handleFilterTags(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	f, err := r.filterOrgIDs(req)
	if err != nil {
		r.logf("ERROR", "resolving orgs for filter tags: %v", err)
		WriteInternalError(w, "Failed to list tags.")
		return
	}
	q := req.URL.Query().Get("q")
	// Always cap — the facet returns a bounded, count-ranked page, never the
	// full set, regardless of whether a typeahead prefix is present.
	opts := datastore.DistinctValueOpts{SearchPrefix: q, Limit: 50}
	values, err := r.db.ListDistinctNodeTags(req.Context(), f, opts)
	if err != nil {
		r.logf("ERROR", "listing distinct tags: %v", err)
		WriteInternalError(w, "Failed to list tags.")
		return
	}
	if values == nil {
		values = []string{}
	}
	WriteJSON(w, http.StatusOK, filterValuesResponse{Data: values})
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
	WriteJSON(w, http.StatusOK, filterValuesResponse{Data: values})
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
	WriteJSON(w, http.StatusOK, filterValuesResponse{Data: values})
}

// handleFilterPlatforms handles GET /api/v1/filters/platforms.
// Returns platform entries with optional display names.
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

	mappings, _ := r.loadPlatformDisplayNames(req.Context())

	entries := make([]platformFilterEntry, 0, len(values))
	for _, v := range values {
		entry := platformFilterEntry{Value: v}
		parts := splitPlatformValue(v)
		if parts.platform != "" {
			family := platformPkg.DetectOSFamilyFromPlatform(parts.platform)
			// TODO: pass caption when filter endpoint provides per-value caption context.
			info := platformPkg.ResolveInfo(parts.platform, parts.version, family, "", mappings)
			entry.DisplayName = &info.DisplayName
			entry.GroupKey = info.GroupKey
			entry.GroupDisplayName = info.GroupDisplayName
		}
		entries = append(entries, entry)
	}

	WriteJSON(w, http.StatusOK, platformFilterResponse{Data: entries})
}

// handleFilterTargetChefVersions handles GET /api/v1/filters/target-chef-versions.
// Unlike the other filters that are derived from node snapshots, this returns
// the target Chef versions from the application configuration.
func (r *Router) handleFilterTargetChefVersions(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	cfg := r.liveConfig()
	versions := cfg.TargetChefVersionList()
	sort.Strings(versions)
	WriteJSON(w, http.StatusOK, filterValuesResponse{Data: versions})
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
	WriteJSON(w, http.StatusOK, filterValuesResponse{Data: labels})
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

type platformParts struct {
	platform string
	version  string
}

// splitPlatformValue splits a combined "platform version" string back into
// its platform and version components.
func splitPlatformValue(combined string) platformParts {
	idx := strings.IndexByte(combined, ' ')
	if idx < 0 {
		return platformParts{platform: combined}
	}
	return platformParts{
		platform: combined[:idx],
		version:  combined[idx+1:],
	}
}
