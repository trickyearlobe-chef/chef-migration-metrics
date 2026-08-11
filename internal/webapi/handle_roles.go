// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// roleFilterFromValues builds the role list filter from raw query values. Shared
// by the list handler and the export path (Limit/Offset applied by the caller).
// tk_status is a materialised column, so its filter/sort/pagination all run in
// SQL — parsed here into the shared filter rather than applied post-query.
func roleFilterFromValues(q url.Values, orgNames []string, targetChefVersion string) datastore.RoleFilter {
	var tkStatuses []string
	if raw := valueOr(q, "tk_status", ""); raw != "" {
		for _, v := range strings.Split(raw, ",") {
			if v = strings.TrimSpace(v); v != "" {
				tkStatuses = append(tkStatuses, v)
			}
		}
	}
	return datastore.RoleFilter{
		OrganisationNames:   orgNames,
		Name:                valueOr(q, "name", ""),
		CompatibilityStatus: valueOr(q, "compatibility_status", ""),
		TKStatuses:          tkStatuses,
		TargetChefVersion:   targetChefVersion,
		Sort:                valueOr(q, "sort", "name"),
		SortOrder:           valueOr(q, "order", "asc"),
	}
}

// handleRoles handles GET /api/v1/roles — paginated list of roles with
// derived compatibility status and summary bar data.
//
// Query parameters:
//   - name: case-insensitive substring match
//   - organisation: comma-separated org names
//   - compatibility_status: compatible, incompatible, untested, all
//   - tk_status: comma-separated TK statuses (passed, failed, partial, untested)
//   - target_chef_version: target version for compatibility evaluation
//   - sort: name (default), node_count, incompatible_cookbook_count, tk_status
//   - order: asc (default) or desc
//   - page, per_page: pagination
func (r *Router) handleRoles(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	orgs, err := r.resolveOrganisationFilter(req)
	if err != nil {
		r.logf("ERROR", "listing organisations for roles: %v", err)
		WriteInternalError(w, "Failed to list roles.")
		return
	}

	orgNames := make([]string, 0, len(orgs))
	for _, org := range orgs {
		orgNames = append(orgNames, org.Name)
	}

	targetChefVersion := queryString(req, "target_chef_version", "")
	if targetChefVersion == "" {
		targetChefVersion = r.defaultTargetVersion()
	}

	pg := ParsePagination(req)

	// Shared with the export path so an export reproduces the list view's filtering.
	// tk_status is a materialised column, so its filter/sort/pagination all run in
	// SQL alongside the other derived fields — no post-query enrichment needed.
	f := roleFilterFromValues(req.URL.Query(), orgNames, targetChefVersion)
	f.Limit = pg.Limit()
	f.Offset = pg.Offset()

	// Pre-fetch the compat summary from cache so ListRolesFiltered skips its
	// internal GetRoleCompatSummary call (which is O(all-roles) and slow).
	summaryFilter := f
	summaryFilter.CompatibilityStatus = ""
	summaryFilter.TKStatuses = nil
	summaryFilter.Limit = 0
	summaryFilter.Offset = 0
	cachedSummary, compatMap := r.cachedRoleCompatSummary(ctx, summaryFilter)
	f.PrecomputedCompatMap = compatMap

	rows, total, dbSummary, err := r.db.ListRolesFiltered(ctx, f)
	if err != nil {
		r.logf("ERROR", "listing filtered roles: %v", err)
		WriteInternalError(w, "Failed to list roles.")
		return
	}

	if rows == nil {
		rows = []datastore.RoleFilterRow{}
	}

	type roleResp struct {
		RoleName                string   `json:"role_name"`
		Organisations           []string `json:"organisations"`
		NodeCount               int      `json:"node_count"`
		DirectCookbookCount     int      `json:"direct_cookbook_count"`
		TransitiveCookbookCount int      `json:"transitive_cookbook_count"`
		TotalCookbookCount      int      `json:"total_cookbook_count"`
		CompatibilityStatus     string   `json:"compatibility_status"`
		CompatibleCount         int      `json:"compatible_count"`
		IncompatibleCount       int      `json:"incompatible_count"`
		UntestedCount           int      `json:"untested_count"`
		// Both always present, and never blank: a role that nothing has tested
		// is untested, which is a state, not the absence of one. Blank left a
		// caller unable to tell it from a field this version does not have,
		// and left the screen rendering a badge with no status in it.
		TKStatus string `json:"tk_status"`
	}

	result := make([]roleResp, 0, len(rows))
	for _, row := range rows {
		compatibilityStatus := row.CompatibilityStatus
		if compatibilityStatus == "" {
			compatibilityStatus = "untested"
		}
		tkStatus := row.TKStatus
		if tkStatus == "" {
			tkStatus = "untested"
		}
		result = append(result, roleResp{
			RoleName:                row.RoleName,
			Organisations:           row.Organisations,
			NodeCount:               row.NodeCount,
			DirectCookbookCount:     row.DirectCookbookCount,
			TransitiveCookbookCount: row.TransitiveCookbookCount,
			TotalCookbookCount:      row.TotalCookbookCount,
			CompatibilityStatus:     compatibilityStatus,
			CompatibleCount:         row.CompatibleCount,
			IncompatibleCount:       row.IncompatibleCount,
			UntestedCount:           row.UntestedCount,
			TKStatus:                tkStatus,
		})
	}

	// Use cached summary when available; fall back to summary from ListRolesFiltered.
	summary := dbSummary
	if compatMap != nil {
		summary = cachedSummary
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"data":       result,
		"summary":    summary,
		"pagination": NewPaginationResponse(pg, total),
	})
}

// handleRoleDetail handles GET /api/v1/roles/:name — returns full detail
// for a single role including compatibility, blocking cookbooks, blast
// radius, and nested role chain.
func (r *Router) handleRoleDetail(w http.ResponseWriter, req *http.Request) {
	segments := pathSegments(req.URL.Path, "/api/v1/roles/")

	// /api/v1/roles/:name/dependency-graph
	if len(segments) >= 2 && segments[len(segments)-1] == "dependency-graph" {
		r.handleRoleDependencyGraph(w, req)
		return
	}

	name := pathParam(req, "/api/v1/roles/")
	// Trim trailing slash or sub-path segments for the detail view.
	if idx := strings.Index(name, "/"); idx >= 0 {
		name = name[:idx]
	}
	if name == "" {
		WriteNotFound(w, "Role name is required.")
		return
	}

	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()

	targetChefVersion := queryString(req, "target_chef_version", "")
	if targetChefVersion == "" {
		targetChefVersion = r.defaultTargetVersion()
	}

	detail, err := r.db.GetRoleDetail(ctx, name, targetChefVersion)
	if err != nil {
		if isNotFound(err) {
			WriteNotFound(w, "Role not found: "+name)
			return
		}
		r.logf("ERROR", "getting role detail for %s: %v", name, err)
		WriteInternalError(w, "Failed to get role detail.")
		return
	}

	WriteJSON(w, http.StatusOK, detail)
}

// handleRoleDependencyGraph handles GET /api/v1/roles/:name/dependency-graph
// Returns the dependency graph scoped to a single role.
func (r *Router) handleRoleDependencyGraph(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	segments := pathSegments(req.URL.Path, "/api/v1/roles/")
	if len(segments) < 2 {
		WriteNotFound(w, "Role name is required.")
		return
	}
	roleName := segments[0]

	ctx := req.Context()

	targetChefVersion := queryString(req, "target_chef_version", "")
	if targetChefVersion == "" {
		targetChefVersion = r.defaultTargetVersion()
	}

	orgName := queryString(req, "organisation", "")

	// Find the organisations where this role exists.
	detail, err := r.db.GetRoleDetail(ctx, roleName, targetChefVersion)
	if err != nil {
		if isNotFound(err) {
			WriteNotFound(w, "Role not found: "+roleName)
			return
		}
		r.logf("ERROR", "getting role detail for graph %s: %v", roleName, err)
		WriteInternalError(w, "Failed to load role data.")
		return
	}

	// Determine which org to use for dependency data.
	scopeOrg := orgName
	if scopeOrg == "" && len(detail.Organisations) > 0 {
		scopeOrg = detail.Organisations[0]
	}
	if scopeOrg == "" {
		WriteBadRequest(w, "No organisation found for this role.")
		return
	}

	// Get all dependencies for the org.
	deps, err := r.db.ListRoleDependenciesByOrg(ctx, scopeOrg)
	if err != nil {
		r.logf("ERROR", "listing dependencies for role graph %s: %v", roleName, err)
		WriteInternalError(w, "Failed to load dependency data.")
		return
	}

	// Build role adjacency map.
	adj := make(map[string][]datastore.RoleDependency)
	for _, d := range deps {
		adj[d.RoleName] = append(adj[d.RoleName], d)
	}

	// Load cookbook→cookbook deps for transitive expansion.
	cbAdj, err := r.db.ListCookbookDependenciesByOrg(ctx, scopeOrg)
	if err != nil {
		r.logf("ERROR", "listing cookbook dependencies for role graph %s: %v", roleName, err)
		WriteInternalError(w, "Failed to load cookbook dependency data.")
		return
	}

	// Walk from the role to build the scoped graph.
	type graphNode struct {
		ID                  string `json:"id"`
		Type                string `json:"type"`
		Name                string `json:"name"`
		CompatibilityStatus string `json:"compatibility_status,omitempty"`
		ComplexityLabel     string `json:"complexity_label,omitempty"`
	}
	type graphEdge struct {
		From string `json:"from"`
		To   string `json:"to"`
		Type string `json:"type"`
	}

	nodeMap := make(map[string]graphNode)
	var edges []graphEdge
	visited := make(map[string]bool)
	cbVisited := make(map[string]bool)

	// cbWalk recursively expands cookbook→cookbook edges.
	var cbWalk func(cbName string)
	cbWalk = func(cbName string) {
		if cbVisited[cbName] {
			return
		}
		cbVisited[cbName] = true

		cbID := "cookbook:" + cbName
		if _, ok := nodeMap[cbID]; !ok {
			nodeMap[cbID] = graphNode{ID: cbID, Type: "cookbook", Name: cbName}
		}

		for _, dep := range cbAdj[cbName] {
			depID := "cookbook:" + dep
			if _, ok := nodeMap[depID]; !ok {
				nodeMap[depID] = graphNode{ID: depID, Type: "cookbook", Name: dep}
			}
			edges = append(edges, graphEdge{From: cbID, To: depID, Type: "depends_on"})
			cbWalk(dep)
		}
	}

	var walk func(role string)
	walk = func(role string) {
		if visited[role] {
			return
		}
		visited[role] = true

		roleID := "role:" + role
		if _, ok := nodeMap[roleID]; !ok {
			nodeMap[roleID] = graphNode{ID: roleID, Type: "role", Name: role}
		}

		for _, d := range adj[role] {
			targetID := d.DependencyType + ":" + d.DependencyName
			edgeType := "includes_" + d.DependencyType

			if _, ok := nodeMap[targetID]; !ok {
				nodeMap[targetID] = graphNode{
					ID:   targetID,
					Type: d.DependencyType,
					Name: d.DependencyName,
				}
			}

			edges = append(edges, graphEdge{
				From: roleID,
				To:   targetID,
				Type: edgeType,
			})

			if d.DependencyType == "role" {
				walk(d.DependencyName)
			} else if d.DependencyType == "cookbook" {
				cbWalk(d.DependencyName)
			}
		}
	}
	walk(roleName)

	// Add compatibility info to cookbook nodes if target version given.
	if targetChefVersion != "" {
		compatMap, _ := db_getCookbookCompatMapViaDetail(detail)
		// Use blocking cookbooks to mark incompatible
		blocking := make(map[string]bool)
		for _, bc := range detail.BlockingCookbooks {
			blocking[bc.CookbookName] = true
		}

		for id, n := range nodeMap {
			if n.Type == "cookbook" {
				if blocking[n.Name] {
					n.CompatibilityStatus = "incompatible"
				} else if compatMap[n.Name] != "" {
					n.CompatibilityStatus = compatMap[n.Name]
				}
				nodeMap[id] = n
			}
		}
	}

	// Convert to sorted slices.
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

	roleCount := 0
	cookbookCount := 0
	incompatibleCount := 0
	for _, n := range nodes {
		switch n.Type {
		case "role":
			roleCount++
		case "cookbook":
			cookbookCount++
			if n.CompatibilityStatus == "incompatible" {
				incompatibleCount++
			}
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"nodes": nodes,
		"edges": edges,
		"metadata": map[string]any{
			"total_roles":            roleCount,
			"total_cookbooks":        cookbookCount,
			"incompatible_cookbooks": incompatibleCount,
		},
	})
}

// db_getCookbookCompatMapViaDetail extracts a cookbook compatibility map
// from the role detail's nested chain tree.
func db_getCookbookCompatMapViaDetail(detail *datastore.RoleDetail) (map[string]string, []string) {
	compatMap := make(map[string]string)
	var incompatible []string

	var walk func(node *datastore.RoleChainNode)
	walk = func(node *datastore.RoleChainNode) {
		if node == nil {
			return
		}
		if node.Type == "cookbook" && node.CompatibilityStatus != "" {
			compatMap[node.Name] = node.CompatibilityStatus
			if node.CompatibilityStatus == "incompatible" {
				incompatible = append(incompatible, node.Name)
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(detail.NestedRoleChain)

	return compatMap, incompatible
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err == datastore.ErrNotFound || err.Error() == "not found"
}
