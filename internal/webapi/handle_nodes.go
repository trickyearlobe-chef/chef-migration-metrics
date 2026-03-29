// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
)

// nodeResp is the JSON response struct for a single node in list endpoints.
// Defined at package level to avoid duplication across handleNodes and
// handleNodesWithOwnerFilter.
type nodeResp struct {
	ID               string                      `json:"id"`
	OrganisationID   string                      `json:"organisation_id"`
	OrganisationName string                      `json:"organisation_name"`
	NodeName         string                      `json:"node_name"`
	ChefEnvironment  string                      `json:"chef_environment,omitempty"`
	ChefVersion      string                      `json:"chef_version,omitempty"`
	Platform         string                      `json:"platform,omitempty"`
	PlatformVersion  string                      `json:"platform_version,omitempty"`
	PlatformFamily   string                      `json:"platform_family,omitempty"`
	PolicyName       string                      `json:"policy_name,omitempty"`
	PolicyGroup      string                      `json:"policy_group,omitempty"`
	IsStale          bool                        `json:"is_stale"`
	OhaiTime         float64                     `json:"ohai_time,omitempty"`
	CollectedAt      string                      `json:"collected_at"`
	Readiness        []nodeReadinessSummaryEntry `json:"readiness,omitempty"`
}

// handleNodes handles GET /api/v1/nodes — lists all node snapshots across
// all organisations, optionally filtered by query parameters.
//
// Filters are pushed down to SQL WHERE clauses for scalability. The in-memory
// filterNodes path is only used as a fallback when ownership filtering is
// active (which requires cross-referencing ownership assignments).
func (r *Router) handleNodes(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
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
	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgNameByID[org.ID] = org.Name
		orgIDs = append(orgIDs, org.ID)
	}

	pg := ParsePagination(req)

	// Build SQL filter from query parameters.
	f := nodeSnapshotFilterFromRequest(req, orgIDs)
	f.Limit = pg.Limit()
	f.Offset = pg.Offset()

	// When ownership filtering is active, we need to apply it post-query
	// because ownership is tracked in a separate assignments table and
	// can't easily be pushed into the node snapshot SQL. In this case,
	// we fall back to the in-memory path but still use SQL for the other
	// filters (without pagination, which we apply after ownership
	// filtering).
	ownerFilterActive := of.Active && r.cfg.Ownership.Enabled
	if ownerFilterActive {
		r.handleNodesWithOwnerFilter(w, req, orgs, orgNameByID, of, pg)
		return
	}

	// --- SQL push-down path (no ownership filter) ---

	nodes, total, err := r.db.ListNodeSnapshotsFiltered(req.Context(), f)
	if err != nil {
		r.logf("ERROR", "listing filtered nodes: %v", err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	// Pre-load readiness data for the page's nodes so we can attach a
	// summary to each row without N+1 queries. Index by node_snapshot_id
	// for O(1) lookup. We query by (organisation_id, node_name) rather
	// than by snapshot ID because the readiness record's snapshot ID may
	// be stale if the evaluator ran against an earlier snapshot.
	readinessBySnapshotID := make(map[string][]nodeReadinessSummaryEntry)
	for _, n := range nodes {
		recs, err := r.db.ListNodeReadinessByNodeName(req.Context(), n.OrganisationID, n.NodeName)
		if err != nil {
			continue // non-fatal — readiness just won't be shown
		}
		for _, rec := range recs {
			readinessBySnapshotID[n.ID] = append(readinessBySnapshotID[n.ID], nodeReadinessSummaryEntry{
				TargetChefVersion:      rec.TargetChefVersion,
				IsReady:                rec.IsReady,
				AllCookbooksCompatible: rec.AllCookbooksCompatible,
				SufficientDiskSpace:    rec.SufficientDiskSpace,
				BlockingCookbookCount:  countBlockingCookbooks(rec.BlockingCookbooks),
				StaleData:              rec.StaleData,
			})
		}
	}

	result := make([]nodeResp, 0, len(nodes))
	for _, n := range nodes {
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
			OhaiTime:         n.OhaiTime,
			CollectedAt:      n.CollectedAt.Format("2006-01-02T15:04:05Z"),
			Readiness:        readinessBySnapshotID[n.ID],
		})
	}

	WritePaginated(w, result, pg, total)
}

// handleNodesWithOwnerFilter is the fallback path for handleNodes when
// ownership filtering is active. It uses SQL push-down for node-level
// filters but applies ownership filtering in memory because the ownership
// assignments table is not directly joinable with node snapshots.
func (r *Router) handleNodesWithOwnerFilter(
	w http.ResponseWriter,
	req *http.Request,
	orgs []datastore.Organisation,
	orgNameByID map[string]string,
	of ownerFilter,
	pg PaginationParams,
) {
	ctx := req.Context()

	// Resolve owned node keys.
	var ownedKeys map[string]bool
	if of.Unowned {
		keys, err := r.resolveAllOwnedEntityKeys(ctx, "node")
		if err != nil {
			r.logf("ERROR", "resolving all owned node keys: %v", err)
			WriteInternalError(w, "Failed to resolve ownership filter.")
			return
		}
		ownedKeys = keys
	} else if len(of.OwnerNames) > 0 {
		keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "node")
		if err != nil {
			r.logf("ERROR", "resolving owned node keys: %v", err)
			WriteInternalError(w, "Failed to resolve ownership filter.")
			return
		}
		ownedKeys = keys
	}

	// Use SQL push-down for node-level filters but without pagination
	// (we'll paginate after ownership filtering).
	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.ID)
	}
	f := nodeSnapshotFilterFromRequest(req, orgIDs)
	// No limit/offset — we need all matching nodes for ownership filtering.

	allNodes, _, err := r.db.ListNodeSnapshotsFiltered(ctx, f)
	if err != nil {
		r.logf("ERROR", "listing filtered nodes for owner filter: %v", err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	// Apply ownership filter in memory.
	if ownedKeys != nil {
		if of.Unowned {
			filtered := allNodes[:0]
			for _, n := range allNodes {
				if !ownedKeys[n.NodeName] {
					filtered = append(filtered, n)
				}
			}
			allNodes = filtered
		} else {
			filtered := allNodes[:0]
			for _, n := range allNodes {
				if ownedKeys[n.NodeName] {
					filtered = append(filtered, n)
				}
			}
			allNodes = filtered
		}
	}

	// Paginate the ownership-filtered results.
	total := len(allNodes)
	start := pg.Offset()
	if start > total {
		start = total
	}
	end := start + pg.Limit()
	if end > total {
		end = total
	}

	pageNodes := allNodes[start:end]

	// Pre-load readiness data for the page. Query by (organisation_id,
	// node_name) rather than snapshot ID — see comment in handleNodes.
	readinessBySnapshotID := make(map[string][]nodeReadinessSummaryEntry)
	for _, n := range pageNodes {
		recs, err := r.db.ListNodeReadinessByNodeName(ctx, n.OrganisationID, n.NodeName)
		if err != nil {
			continue
		}
		for _, rec := range recs {
			readinessBySnapshotID[n.ID] = append(readinessBySnapshotID[n.ID], nodeReadinessSummaryEntry{
				TargetChefVersion:      rec.TargetChefVersion,
				IsReady:                rec.IsReady,
				AllCookbooksCompatible: rec.AllCookbooksCompatible,
				SufficientDiskSpace:    rec.SufficientDiskSpace,
				BlockingCookbookCount:  countBlockingCookbooks(rec.BlockingCookbooks),
				StaleData:              rec.StaleData,
			})
		}
	}

	result := make([]nodeResp, 0, len(pageNodes))
	for _, n := range pageNodes {
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
			OhaiTime:         n.OhaiTime,
			CollectedAt:      n.CollectedAt.Format("2006-01-02T15:04:05Z"),
			Readiness:        readinessBySnapshotID[n.ID],
		})
	}

	WritePaginated(w, result, pg, total)
}

// nodeReadinessSummaryEntry is a compact readiness summary for the node list.
type nodeReadinessSummaryEntry struct {
	TargetChefVersion      string `json:"target_chef_version"`
	IsReady                bool   `json:"is_ready"`
	AllCookbooksCompatible bool   `json:"all_cookbooks_compatible"`
	SufficientDiskSpace    *bool  `json:"sufficient_disk_space"`
	BlockingCookbookCount  int    `json:"blocking_cookbook_count"`
	StaleData              bool   `json:"stale_data"`
}

// countBlockingCookbooks returns the number of blocking cookbooks from the
// JSONB column. It handles both the legacy string array and structured
// BlockingCookbook array formats.
func countBlockingCookbooks(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	// Try as array of any — just count the elements.
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		return len(arr)
	}
	return 0
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

	// Fetch readiness records by (organisation_id, node_name) rather than
	// by node_snapshot_id. The snapshot ID in node_readiness may be stale
	// if the readiness evaluator ran against an earlier snapshot that has
	// since been replaced by a newer collection run. Querying by name
	// matches the same path the dashboard uses and is resilient to
	// snapshot ID drift.
	readiness, err := r.db.ListNodeReadinessByNodeName(req.Context(), org.ID, nodeName)
	if err != nil {
		r.logf("WARN", "listing readiness for node %s/%s: %v", orgName, nodeName, err)
		// Non-fatal — we still return the snapshot.
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"node":              snapshot,
		"organisation_name": org.Name,
		"readiness":         readiness,
	})
}

// handleNodesByVersion handles GET /api/v1/nodes/by-version/:chef_version —
// returns all nodes running the specified Chef client version.
//
// Uses SQL push-down with an exact chef_version match.
func (r *Router) handleNodesByVersion(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	chefVersion := pathParam(req, "/api/v1/nodes/by-version/")
	if chefVersion == "" {
		WriteBadRequest(w, "Chef version is required.")
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for nodes-by-version: %v", err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	orgIDs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.ID)
	}

	// Use SQL push-down with exact chef_version match.
	f := datastore.NodeSnapshotFilter{
		OrganisationIDs:  orgIDs,
		ChefVersionExact: chefVersion,
	}

	matched, _, err := r.db.ListNodeSnapshotsFiltered(req.Context(), f)
	if err != nil {
		r.logf("ERROR", "listing nodes by version %s: %v", chefVersion, err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	// Apply owner filter if active and ownership is enabled.
	matched = applyOwnerFilter(req.Context(), r, matched, of)

	WriteJSON(w, http.StatusOK, map[string]any{
		"chef_version": chefVersion,
		"total":        len(matched),
		"data":         matched,
	})
}

// handleNodesByCookbook handles GET /api/v1/nodes/by-cookbook/:cookbook_name —
// returns all nodes that use the specified cookbook.
//
// This endpoint requires scanning the cookbooks JSONB field, so it uses
// SQL push-down for org filtering + IncludeHeavyJSON, then checks the
// cookbook name in memory.
func (r *Router) handleNodesByCookbook(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	cookbookName := pathParam(req, "/api/v1/nodes/by-cookbook/")
	if cookbookName == "" {
		WriteBadRequest(w, "Cookbook name is required.")
		return
	}

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for nodes-by-cookbook: %v", err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	orgIDs := make([]string, 0, len(orgs))
	orgNameByID := make(map[string]string, len(orgs))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.ID)
		orgNameByID[org.ID] = org.Name
	}

	// Use SQL push-down for org filtering. We need the cookbooks JSONB
	// for the in-memory cookbook check.
	f := datastore.NodeSnapshotFilter{
		OrganisationIDs:  orgIDs,
		IncludeHeavyJSON: true,
	}

	allNodes, _, err := r.db.ListNodeSnapshotsFiltered(req.Context(), f)
	if err != nil {
		r.logf("ERROR", "listing nodes for cookbook %s: %v", cookbookName, err)
		WriteInternalError(w, "Failed to list nodes.")
		return
	}

	type nodeWithOrg struct {
		OrganisationName string                 `json:"organisation_name"`
		Node             datastore.NodeSnapshot `json:"node"`
	}

	var matched []nodeWithOrg
	for _, n := range allNodes {
		if nodeUsesCookbook(n, cookbookName) {
			matched = append(matched, nodeWithOrg{
				OrganisationName: orgNameByID[n.OrganisationID],
				Node:             n,
			})
		}
	}

	// Apply owner filter if active and ownership is enabled.
	if of.Active && r.cfg.Ownership.Enabled {
		var ownedKeys map[string]bool
		ctx := req.Context()
		if of.Unowned {
			keys, err := r.resolveAllOwnedEntityKeys(ctx, "node")
			if err != nil {
				r.logf("ERROR", "resolving all owned node keys: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		} else if len(of.OwnerNames) > 0 {
			keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "node")
			if err != nil {
				r.logf("ERROR", "resolving owned node keys: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		}
		if ownedKeys != nil {
			if of.Unowned {
				filtered := matched[:0]
				for _, n := range matched {
					if !ownedKeys[n.Node.NodeName] {
						filtered = append(filtered, n)
					}
				}
				matched = filtered
			} else {
				filtered := matched[:0]
				for _, n := range matched {
					if ownedKeys[n.Node.NodeName] {
						filtered = append(filtered, n)
					}
				}
				matched = filtered
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

// nodeSnapshotFilterFromRequest constructs a NodeSnapshotFilter from the
// query-string parameters of an HTTP request. It maps the same parameters
// that were previously consumed by export.FilterNodes, so the semantics
// are identical (case-insensitive substring matching).
func nodeSnapshotFilterFromRequest(req *http.Request, orgIDs []string) datastore.NodeSnapshotFilter {
	q := req.URL.Query()

	f := datastore.NodeSnapshotFilter{
		OrganisationIDs: orgIDs,
		NodeName:        q.Get("node_name"),
		Environment:     q.Get("environment"),
		Platform:        q.Get("platform"),
		ChefVersion:     q.Get("chef_version"),
		PolicyName:      q.Get("policy_name"),
		PolicyGroup:     q.Get("policy_group"),
		Role:            q.Get("role"),
	}

	// Sort parameters.
	f.Sort = q.Get("sort")
	f.SortOrder = q.Get("order")

	// Map the string "true"/"false" stale parameter to *bool.
	switch q.Get("stale") {
	case "true":
		v := true
		f.Stale = &v
	case "false":
		v := false
		f.Stale = &v
	}

	return f
}

// filterNodes applies optional query-parameter filters (environment, platform,
// chef_version, policy_name, policy_group, stale) to the given slice,
// returning only matching nodes. It delegates to the shared export.FilterNodes
// implementation so that API handlers and export generators use identical
// filtering logic.
//
// Deprecated: prefer nodeSnapshotFilterFromRequest + ListNodeSnapshotsFiltered
// for SQL push-down. This function is retained for the export system and as a
// fallback.
func filterNodes(req *http.Request, nodes []datastore.NodeSnapshot) []datastore.NodeSnapshot {
	q := req.URL.Query()
	return export.FilterNodes(nodes, export.Filters{
		Environment: q.Get("environment"),
		Platform:    q.Get("platform"),
		ChefVersion: q.Get("chef_version"),
		PolicyName:  q.Get("policy_name"),
		PolicyGroup: q.Get("policy_group"),
		Role:        q.Get("role"),
		Stale:       q.Get("stale"),
		NodeName:    q.Get("node_name"),
	})
}

// applyOwnerFilter applies ownership filtering to a slice of node snapshots
// when ownership is active. It resolves ownership keys and filters in memory.
// Returns the original slice unmodified when ownership is not active.
func applyOwnerFilter(ctx context.Context, r *Router, nodes []datastore.NodeSnapshot, of ownerFilter) []datastore.NodeSnapshot {
	if !of.Active || !r.cfg.Ownership.Enabled {
		return nodes
	}

	var ownedKeys map[string]bool
	if of.Unowned {
		keys, err := r.resolveAllOwnedEntityKeys(ctx, "node")
		if err != nil {
			r.logf("WARN", "resolving all owned node keys: %v", err)
			return nodes
		}
		ownedKeys = keys
	} else if len(of.OwnerNames) > 0 {
		keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "node")
		if err != nil {
			r.logf("WARN", "resolving owned node keys: %v", err)
			return nodes
		}
		ownedKeys = keys
	}

	if ownedKeys == nil {
		return nodes
	}

	filtered := nodes[:0]
	for _, n := range nodes {
		if of.Unowned {
			if !ownedKeys[n.NodeName] {
				filtered = append(filtered, n)
			}
		} else {
			if ownedKeys[n.NodeName] {
				filtered = append(filtered, n)
			}
		}
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
