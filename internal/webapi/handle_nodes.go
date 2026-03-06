// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// handleNodes handles GET /api/v1/nodes — lists all node snapshots across
// all organisations, optionally filtered by query parameters.
func (r *Router) handleNodes(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for nodes: %v", err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	// Build a map from organisation ID to name so we can include the
	// human-readable org name in each node response row. The node detail
	// endpoint uses org name in the URL path, so the frontend needs it
	// for constructing links.
	orgNameByID := make(map[string]string, len(orgs))
	for _, org := range orgs {
		orgNameByID[org.ID] = org.Name
	}

	// Collect snapshots from all organisations (from their most recent
	// completed collection run).
	var allNodes []datastore.NodeSnapshot
	for _, org := range orgs {
		nodes, err := r.db.ListNodeSnapshotsByOrganisation(req.Context(), org.ID)
		if err != nil {
			r.logf("WARN", "listing nodes for org %s: %v", org.Name, err)
			continue
		}
		allNodes = append(allNodes, nodes...)
	}

	// Apply optional query-parameter filters.
	allNodes = filterNodes(req, allNodes)

	// Paginate the results.
	pg := ParsePagination(req)
	total := len(allNodes)
	start := pg.Offset()
	if start > total {
		start = total
	}
	end := start + pg.Limit()
	if end > total {
		end = total
	}

	type nodeResp struct {
		ID               string `json:"id"`
		OrganisationID   string `json:"organisation_id"`
		OrganisationName string `json:"organisation_name"`
		NodeName         string `json:"node_name"`
		ChefEnvironment  string `json:"chef_environment,omitempty"`
		ChefVersion      string `json:"chef_version,omitempty"`
		Platform         string `json:"platform,omitempty"`
		PlatformVersion  string `json:"platform_version,omitempty"`
		PlatformFamily   string `json:"platform_family,omitempty"`
		PolicyName       string `json:"policy_name,omitempty"`
		PolicyGroup      string `json:"policy_group,omitempty"`
		IsStale          bool   `json:"is_stale"`
		CollectedAt      string `json:"collected_at"`
	}

	result := make([]nodeResp, 0, end-start)
	for _, n := range allNodes[start:end] {
		result = append(result, nodeResp{
			ID:               n.ID,
			OrganisationID:   n.OrganisationID,
			OrganisationName: orgNameByID[n.OrganisationID],
			NodeName:         n.NodeName,
			ChefEnvironment:  n.ChefEnvironment,
			ChefVersion:      n.ChefVersion,
			Platform:         n.Platform,
			PlatformVersion:  n.PlatformVersion,
			PlatformFamily:   n.PlatformFamily,
			PolicyName:       n.PolicyName,
			PolicyGroup:      n.PolicyGroup,
			IsStale:          n.IsStale,
			CollectedAt:      n.CollectedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	WritePaginated(w, result, pg, total)
}

// handleNodeDetail handles GET /api/v1/nodes/:organisation/:name — returns
// a single node's detail including readiness information.
func (r *Router) handleNodeDetail(w http.ResponseWriter, req *http.Request) {
	// Routes like /api/v1/nodes/by-version/ and /api/v1/nodes/by-cookbook/
	// are registered with more specific prefixes and matched first by the
	// ServeMux. This handler only fires for other /api/v1/nodes/* paths.
	segs := pathSegments(req.URL.Path, "/api/v1/nodes/")
	if len(segs) < 2 {
		WriteNotFound(w, "Node detail requires /api/v1/nodes/:organisation/:name.")
		return
	}

	if !requireGET(w, req) {
		return
	}

	orgName := segs[0]
	nodeName := strings.Join(segs[1:], "/") // node names may contain slashes

	// Resolve organisation by name.
	org, err := r.db.GetOrganisationByName(req.Context(), orgName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Organisation %q not found.", orgName))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting organisation %s: %v", orgName, err)
		WriteInternalError(w, "Failed to get organisation.")
		return
	}

	// Get the most recent snapshot for this node.
	snapshot, err := r.db.GetNodeSnapshotByName(req.Context(), org.ID, nodeName)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Node %q not found in organisation %q.", nodeName, orgName))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting node snapshot %s/%s: %v", orgName, nodeName, err)
		WriteInternalError(w, "Failed to get node.")
		return
	}

	// Fetch readiness records for this snapshot.
	readiness, err := r.db.ListNodeReadinessForSnapshot(req.Context(), snapshot.ID)
	if err != nil {
		r.logf("WARN", "listing readiness for node %s/%s: %v", orgName, nodeName, err)
		// Non-fatal — we still return the snapshot.
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"node":      snapshot,
		"readiness": readiness,
	})
}

// handleNodesByVersion handles GET /api/v1/nodes/by-version/:chef_version —
// returns all nodes running the specified Chef client version.
func (r *Router) handleNodesByVersion(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	chefVersion := pathParam(req, "/api/v1/nodes/by-version/")
	if chefVersion == "" {
		WriteBadRequest(w, "Chef version is required.")
		return
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for nodes-by-version: %v", err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	var matched []datastore.NodeSnapshot
	for _, org := range orgs {
		nodes, err := r.db.ListNodeSnapshotsByOrganisation(req.Context(), org.ID)
		if err != nil {
			r.logf("WARN", "listing nodes for org %s: %v", org.Name, err)
			continue
		}
		for _, n := range nodes {
			if n.ChefVersion == chefVersion {
				matched = append(matched, n)
			}
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"chef_version": chefVersion,
		"total":        len(matched),
		"data":         matched,
	})
}

// handleNodesByCookbook handles GET /api/v1/nodes/by-cookbook/:cookbook_name —
// returns all nodes that use the specified cookbook.
func (r *Router) handleNodesByCookbook(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	cookbookName := pathParam(req, "/api/v1/nodes/by-cookbook/")
	if cookbookName == "" {
		WriteBadRequest(w, "Cookbook name is required.")
		return
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for nodes-by-cookbook: %v", err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	type nodeWithOrg struct {
		OrganisationName string                 `json:"organisation_name"`
		Node             datastore.NodeSnapshot `json:"node"`
	}

	var matched []nodeWithOrg
	for _, org := range orgs {
		nodes, err := r.db.ListNodeSnapshotsByOrganisation(req.Context(), org.ID)
		if err != nil {
			r.logf("WARN", "listing nodes for org %s: %v", org.Name, err)
			continue
		}
		for _, n := range nodes {
			if nodeUsesCookbook(n, cookbookName) {
				matched = append(matched, nodeWithOrg{
					OrganisationName: org.Name,
					Node:             n,
				})
			}
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"cookbook_name": cookbookName,
		"total":         len(matched),
		"data":          matched,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// filterNodes applies optional query-parameter filters (environment, platform,
// chef_version, policy_name, policy_group, stale) to the given slice,
// returning only matching nodes.
func filterNodes(req *http.Request, nodes []datastore.NodeSnapshot) []datastore.NodeSnapshot {
	q := req.URL.Query()
	env := q.Get("environment")
	platform := q.Get("platform")
	version := q.Get("chef_version")
	policyName := q.Get("policy_name")
	policyGroup := q.Get("policy_group")
	stale := q.Get("stale")

	if env == "" && platform == "" && version == "" && policyName == "" && policyGroup == "" && stale == "" {
		return nodes
	}

	filtered := make([]datastore.NodeSnapshot, 0, len(nodes))
	for _, n := range nodes {
		if env != "" && n.ChefEnvironment != env {
			continue
		}
		if platform != "" && n.Platform != platform {
			continue
		}
		if version != "" && n.ChefVersion != version {
			continue
		}
		if policyName != "" && n.PolicyName != policyName {
			continue
		}
		if policyGroup != "" && n.PolicyGroup != policyGroup {
			continue
		}
		if stale == "true" && !n.IsStale {
			continue
		}
		if stale == "false" && n.IsStale {
			continue
		}
		filtered = append(filtered, n)
	}
	return filtered
}

// nodeUsesCookbook checks whether a node snapshot's Cookbooks JSON contains
// the given cookbook name. The Cookbooks field is a JSON object mapping
// cookbook names to version info, e.g. {"apt": {"version": "7.4.0"}, ...}.
func nodeUsesCookbook(n datastore.NodeSnapshot, cookbookName string) bool {
	if len(n.Cookbooks) == 0 {
		return false
	}
	// Quick substring check before full parse — the cookbook name will
	// appear as a JSON key in the form `"cookbook_name":`.
	return strings.Contains(string(n.Cookbooks), fmt.Sprintf("%q", cookbookName))
}
