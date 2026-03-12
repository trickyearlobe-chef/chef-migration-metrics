// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"sort"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Dependency graph endpoints — role/cookbook dependency data for D3/Cytoscape
// visualisation and flat table views.
// ---------------------------------------------------------------------------

// handleDependencyGraph handles GET /api/v1/dependency-graph.
// Returns nodes and edges suitable for D3 force-directed or Cytoscape
// graph rendering. Each role and cookbook becomes a graph node; each
// dependency record becomes a directed edge.
//
// Query parameters:
//   - organisation: filter by organisation name (required — dependency data
//     is per-organisation)
func (r *Router) handleDependencyGraph(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	// Resolve owned role keys when ownership filtering is active.
	var ownedKeys map[string]bool
	if of.Active && r.cfg.Ownership.Enabled {
		if of.Unowned {
			keys, err := r.resolveAllOwnedEntityKeys(ctx, "role")
			if err != nil {
				r.logf("ERROR", "resolving all owned role keys for dependency graph: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		} else if len(of.OwnerNames) > 0 {
			keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "role")
			if err != nil {
				r.logf("ERROR", "resolving owned role keys for dependency graph: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		}
	}

	orgName := queryString(req, "organisation", "")
	if orgName == "" {
		WriteBadRequest(w, "Query parameter 'organisation' is required.")
		return
	}

	org, err := r.db.GetOrganisationByName(ctx, orgName)
	if err != nil {
		if isNotFound(err) {
			WriteNotFound(w, "Organisation not found: "+orgName)
			return
		}
		r.logf("ERROR", "getting organisation %s for dependency graph: %v", orgName, err)
		WriteInternalError(w, "Failed to resolve organisation.")
		return
	}

	deps, err := r.db.ListRoleDependenciesByOrg(ctx, org.ID)
	if err != nil {
		r.logf("ERROR", "listing role dependencies for org %s: %v", orgName, err)
		WriteInternalError(w, "Failed to load dependency data.")
		return
	}

	// Build the graph. We use two maps: one for nodes (keyed by
	// "type:name") and one to deduplicate edges.
	type graphNode struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"` // "role" or "cookbook"
	}

	type graphEdge struct {
		Source         string `json:"source"`
		Target         string `json:"target"`
		DependencyType string `json:"dependency_type"`
	}

	nodeMap := make(map[string]graphNode)
	var edges []graphEdge

	for _, dep := range deps {
		// Source is always a role.
		sourceID := "role:" + dep.RoleName
		if _, ok := nodeMap[sourceID]; !ok {
			nodeMap[sourceID] = graphNode{
				ID:   sourceID,
				Name: dep.RoleName,
				Type: "role",
			}
		}

		// Target type depends on the dependency type.
		targetID := dep.DependencyType + ":" + dep.DependencyName
		if _, ok := nodeMap[targetID]; !ok {
			nodeMap[targetID] = graphNode{
				ID:   targetID,
				Name: dep.DependencyName,
				Type: dep.DependencyType,
			}
		}

		edges = append(edges, graphEdge{
			Source:         sourceID,
			Target:         targetID,
			DependencyType: dep.DependencyType,
		})
	}

	// Apply owner filter: remove role nodes that don't match ownership,
	// and remove edges that reference removed nodes.
	if of.Active && r.cfg.Ownership.Enabled && ownedKeys != nil {
		// Determine which role nodes to keep.
		keepNodes := make(map[string]bool, len(nodeMap))
		for id, n := range nodeMap {
			if n.Type == "role" {
				if of.Unowned {
					if ownedKeys[n.Name] {
						delete(nodeMap, id)
						continue
					}
				} else {
					if !ownedKeys[n.Name] {
						delete(nodeMap, id)
						continue
					}
				}
			}
			keepNodes[id] = true
		}

		// Remove edges referencing removed nodes.
		filteredEdges := edges[:0]
		for _, e := range edges {
			if keepNodes[e.Source] && keepNodes[e.Target] {
				filteredEdges = append(filteredEdges, e)
			}
		}
		edges = filteredEdges

		// Remove orphaned cookbook nodes (no remaining edges reference them).
		referenced := make(map[string]bool)
		for _, e := range edges {
			referenced[e.Source] = true
			referenced[e.Target] = true
		}
		for id, n := range nodeMap {
			if n.Type == "cookbook" && !referenced[id] {
				delete(nodeMap, id)
			}
		}
	}

	// Convert node map to sorted slice for deterministic output.
	nodes := make([]graphNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodes[i].Type < nodes[j].Type
		}
		return nodes[i].Name < nodes[j].Name
	})

	if edges == nil {
		edges = []graphEdge{}
	}

	// Compute summary stats.
	roleCount := 0
	cookbookCount := 0
	for _, n := range nodes {
		switch n.Type {
		case "role":
			roleCount++
		case "cookbook":
			cookbookCount++
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"organisation": orgName,
		"summary": map[string]any{
			"total_nodes":    len(nodes),
			"total_edges":    len(edges),
			"role_count":     roleCount,
			"cookbook_count": cookbookCount,
		},
		"nodes": nodes,
		"edges": edges,
	})
}

// handleDependencyGraphTable handles GET /api/v1/dependency-graph/table.
// Returns a flat table view with each role's direct dependencies and a
// count of transitive dependencies (roles that depend on it).
//
// Query parameters:
//   - organisation: filter by organisation name (required)
//   - sort: field to sort by — "role_name", "cookbook_count", "role_count",
//     "total_dependencies" (default: "total_dependencies")
//   - order: "asc" or "desc" (default: "desc")
func (r *Router) handleDependencyGraphTable(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	// Parse and validate owner filter.
	of := parseOwnerFilter(req)
	if !validateOwnerFilter(w, of) {
		return
	}

	// Resolve owned role keys when ownership filtering is active.
	var ownedKeys map[string]bool
	if of.Active && r.cfg.Ownership.Enabled {
		if of.Unowned {
			keys, err := r.resolveAllOwnedEntityKeys(ctx, "role")
			if err != nil {
				r.logf("ERROR", "resolving all owned role keys for dependency table: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		} else if len(of.OwnerNames) > 0 {
			keys, err := r.resolveOwnedEntityKeys(ctx, of.OwnerNames, "role")
			if err != nil {
				r.logf("ERROR", "resolving owned role keys for dependency table: %v", err)
				WriteInternalError(w, "Failed to resolve ownership filter.")
				return
			}
			ownedKeys = keys
		}
	}

	orgName := queryString(req, "organisation", "")
	if orgName == "" {
		WriteBadRequest(w, "Query parameter 'organisation' is required.")
		return
	}

	org, err := r.db.GetOrganisationByName(ctx, orgName)
	if err != nil {
		if isNotFound(err) {
			WriteNotFound(w, "Organisation not found: "+orgName)
			return
		}
		r.logf("ERROR", "getting organisation %s for dependency table: %v", orgName, err)
		WriteInternalError(w, "Failed to resolve organisation.")
		return
	}

	// Get per-role dependency counts.
	roleCounts, err := r.db.CountDependenciesByRole(ctx, org.ID)
	if err != nil {
		r.logf("ERROR", "counting dependencies by role for org %s: %v", orgName, err)
		WriteInternalError(w, "Failed to load dependency counts.")
		return
	}

	// Get all dependencies so we can compute transitive (reverse) counts —
	// i.e. how many other roles depend on each role.
	allDeps, err := r.db.ListRoleDependenciesByOrg(ctx, org.ID)
	if err != nil {
		r.logf("ERROR", "listing dependencies for org %s in table view: %v", orgName, err)
		WriteInternalError(w, "Failed to load dependency data.")
		return
	}

	// Build reverse dependency counts: for each role, how many other roles
	// include it as a dependency.
	reverseCounts := make(map[string]int)
	for _, dep := range allDeps {
		if dep.DependencyType == "role" {
			reverseCounts[dep.DependencyName]++
		}
	}

	// Also collect per-role direct dependency names for the detail view.
	directDeps := make(map[string][]dependencyEntry)
	for _, dep := range allDeps {
		directDeps[dep.RoleName] = append(directDeps[dep.RoleName], dependencyEntry{
			Name: dep.DependencyName,
			Type: dep.DependencyType,
		})
	}

	type tableRow struct {
		RoleName          string            `json:"role_name"`
		CookbookCount     int               `json:"cookbook_count"`
		RoleCount         int               `json:"role_count"`
		TotalDependencies int               `json:"total_dependencies"`
		DependedOnBy      int               `json:"depended_on_by"` // transitive: roles that depend on this role
		Dependencies      []dependencyEntry `json:"dependencies"`
	}

	rows := make([]tableRow, 0, len(roleCounts))
	for _, rc := range roleCounts {
		deps := directDeps[rc.RoleName]
		if deps == nil {
			deps = []dependencyEntry{}
		}
		// Sort dependencies for deterministic output.
		sort.Slice(deps, func(i, j int) bool {
			if deps[i].Type != deps[j].Type {
				return deps[i].Type < deps[j].Type
			}
			return deps[i].Name < deps[j].Name
		})

		rows = append(rows, tableRow{
			RoleName:          rc.RoleName,
			CookbookCount:     rc.CookbookCount,
			RoleCount:         rc.RoleCount,
			TotalDependencies: rc.TotalDependency,
			DependedOnBy:      reverseCounts[rc.RoleName],
			Dependencies:      deps,
		})
	}

	// Apply owner filter if active and ownership is enabled.
	if of.Active && r.cfg.Ownership.Enabled && ownedKeys != nil {
		if of.Unowned {
			filtered := rows[:0]
			for _, row := range rows {
				if !ownedKeys[row.RoleName] {
					filtered = append(filtered, row)
				}
			}
			rows = filtered
		} else {
			filtered := rows[:0]
			for _, row := range rows {
				if ownedKeys[row.RoleName] {
					filtered = append(filtered, row)
				}
			}
			rows = filtered
		}
	}

	// Sort by the requested field. Default to descending for numeric fields
	// so the most-connected roles appear first.
	sp := ParseSort(req, "total_dependencies", []string{
		"role_name", "cookbook_count", "role_count", "total_dependencies",
	})
	if req.URL.Query().Get("order") == "" && sp.Field != "role_name" {
		sp.Order = "desc"
	}

	sort.Slice(rows, func(i, j int) bool {
		var less bool
		switch sp.Field {
		case "role_name":
			less = rows[i].RoleName < rows[j].RoleName
		case "cookbook_count":
			less = rows[i].CookbookCount < rows[j].CookbookCount
		case "role_count":
			less = rows[i].RoleCount < rows[j].RoleCount
		default: // total_dependencies
			less = rows[i].TotalDependencies < rows[j].TotalDependencies
		}
		if sp.Order == "desc" {
			return !less
		}
		return less
	})

	// Paginate.
	pg := ParsePagination(req)
	total := len(rows)
	start := pg.Offset()
	if start > total {
		start = total
	}
	end := start + pg.Limit()
	if end > total {
		end = total
	}

	// Also fetch cookbook-to-role counts for a summary.
	cbRoleCounts, err := r.db.CountRolesPerCookbook(ctx, org.ID)
	if err != nil {
		r.logf("WARN", "counting roles per cookbook for org %s: %v", orgName, err)
		cbRoleCounts = nil
	}

	// Compute top shared cookbooks (used by most roles).
	type sharedCookbook struct {
		CookbookName string `json:"cookbook_name"`
		RoleCount    int    `json:"role_count"`
	}
	topShared := make([]sharedCookbook, 0)
	for _, crc := range cbRoleCounts {
		if crc.RoleCount >= 2 {
			topShared = append(topShared, sharedCookbook{
				CookbookName: crc.CookbookName,
				RoleCount:    crc.RoleCount,
			})
		}
	}
	// cbRoleCounts is already sorted by role_count DESC from the datastore,
	// limit to top 20.
	if len(topShared) > 20 {
		topShared = topShared[:20]
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"organisation":     orgName,
		"total_roles":      total,
		"shared_cookbooks": topShared,
		"data":             rows[start:end],
		"pagination":       NewPaginationResponse(pg, total),
	})
}

// dependencyEntry is a minimal struct representing one dependency for the
// table view.
type dependencyEntry struct {
	Name string `json:"name"`
	Type string `json:"type"` // "role" or "cookbook"
}

// isNotFound returns true if the error represents a not-found condition.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err == datastore.ErrNotFound || err.Error() == "not found"
}
