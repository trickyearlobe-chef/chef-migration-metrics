// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
	"sort"
)

// ---------------------------------------------------------------------------
// Filter endpoints — each returns a sorted list of distinct values drawn
// from the latest node snapshots across all organisations. The response
// shape is always {"data": ["value1", "value2", ...]}.
// ---------------------------------------------------------------------------

// handleFilterEnvironments handles GET /api/v1/filters/environments.
func (r *Router) handleFilterEnvironments(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	values, err := r.collectDistinctNodeValues(req, func(n nodeFilterRecord) string {
		return n.chefEnvironment
	})
	if err != nil {
		r.logf("ERROR", "collecting filter environments: %v", err)
		WriteInternalError(w, "Failed to list environments.")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": values})
}

// handleFilterRoles handles GET /api/v1/filters/roles.
func (r *Router) handleFilterRoles(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	values, err := r.collectDistinctNodeJSONArrayValues(req, func(n nodeFilterRecord) []byte {
		return n.roles
	})
	if err != nil {
		r.logf("ERROR", "collecting filter roles: %v", err)
		WriteInternalError(w, "Failed to list roles.")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": values})
}

// handleFilterPolicyNames handles GET /api/v1/filters/policy-names.
func (r *Router) handleFilterPolicyNames(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	values, err := r.collectDistinctNodeValues(req, func(n nodeFilterRecord) string {
		return n.policyName
	})
	if err != nil {
		r.logf("ERROR", "collecting filter policy names: %v", err)
		WriteInternalError(w, "Failed to list policy names.")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": values})
}

// handleFilterPolicyGroups handles GET /api/v1/filters/policy-groups.
func (r *Router) handleFilterPolicyGroups(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	values, err := r.collectDistinctNodeValues(req, func(n nodeFilterRecord) string {
		return n.policyGroup
	})
	if err != nil {
		r.logf("ERROR", "collecting filter policy groups: %v", err)
		WriteInternalError(w, "Failed to list policy groups.")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": values})
}

// handleFilterPlatforms handles GET /api/v1/filters/platforms.
func (r *Router) handleFilterPlatforms(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	values, err := r.collectDistinctNodeValues(req, func(n nodeFilterRecord) string {
		return n.platform
	})
	if err != nil {
		r.logf("ERROR", "collecting filter platforms: %v", err)
		WriteInternalError(w, "Failed to list platforms.")
		return
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

// nodeFilterRecord is a lightweight projection of NodeSnapshot fields used
// by the filter collection helpers.
type nodeFilterRecord struct {
	chefEnvironment string
	platform        string
	policyName      string
	policyGroup     string
	roles           []byte // JSON array
}

// collectDistinctNodeValues iterates over all node snapshots across all
// organisations, extracts a string value from each using extractFn, and
// returns a sorted slice of distinct non-empty values.
func (r *Router) collectDistinctNodeValues(req *http.Request, extractFn func(nodeFilterRecord) string) ([]string, error) {
	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	for _, org := range orgs {
		nodes, err := r.db.ListNodeSnapshotsByOrganisation(req.Context(), org.ID)
		if err != nil {
			r.logf("WARN", "listing nodes for org %s in filter: %v", org.Name, err)
			continue
		}
		for _, n := range nodes {
			rec := nodeFilterRecord{
				chefEnvironment: n.ChefEnvironment,
				platform:        n.Platform,
				policyName:      n.PolicyName,
				policyGroup:     n.PolicyGroup,
				roles:           n.Roles,
			}
			v := extractFn(rec)
			if v != "" {
				seen[v] = true
			}
		}
	}

	result := make([]string, 0, len(seen))
	for v := range seen {
		result = append(result, v)
	}
	sort.Strings(result)
	return result, nil
}

// collectDistinctNodeJSONArrayValues is like collectDistinctNodeValues but
// for fields that are stored as JSON arrays of strings (e.g. roles). It
// unmarshals each array and collects all distinct non-empty elements.
func (r *Router) collectDistinctNodeJSONArrayValues(req *http.Request, extractFn func(nodeFilterRecord) []byte) ([]string, error) {
	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	for _, org := range orgs {
		nodes, err := r.db.ListNodeSnapshotsByOrganisation(req.Context(), org.ID)
		if err != nil {
			r.logf("WARN", "listing nodes for org %s in filter: %v", org.Name, err)
			continue
		}
		for _, n := range nodes {
			rec := nodeFilterRecord{
				chefEnvironment: n.ChefEnvironment,
				platform:        n.Platform,
				policyName:      n.PolicyName,
				policyGroup:     n.PolicyGroup,
				roles:           n.Roles,
			}
			raw := extractFn(rec)
			if len(raw) == 0 {
				continue
			}
			var items []string
			if err := json.Unmarshal(raw, &items); err != nil {
				// Not a string array — skip silently.
				continue
			}
			for _, item := range items {
				if item != "" {
					seen[item] = true
				}
			}
		}
	}

	result := make([]string, 0, len(seen))
	for v := range seen {
		result = append(result, v)
	}
	sort.Strings(result)
	return result, nil
}
