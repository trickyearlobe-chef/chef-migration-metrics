// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"sort"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

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
	tkFilter := queryString(req, "tk_status", "")
	sortField := queryString(req, "sort", "name")

	f := datastore.RoleFilter{
		OrganisationNames:   orgNames,
		Name:                queryString(req, "name", ""),
		CompatibilityStatus: queryString(req, "compatibility_status", ""),
		TargetChefVersion:   targetChefVersion,
		Sort:                sortField,
		SortOrder:           queryString(req, "order", "asc"),
		Limit:               pg.Limit(),
		Offset:              pg.Offset(),
	}

	// When TK filter or TK sort is active, disable SQL pagination —
	// TK status is computed post-query, so we must fetch all rows first.
	tkFilterActive := tkFilter != ""
	tkSortActive := sortField == "tk_status"
	if tkFilterActive || tkSortActive {
		f.Limit = 0
		f.Offset = 0
	}

	rows, total, summary, err := r.db.ListRolesFiltered(ctx, f)
	if err != nil {
		r.logf("ERROR", "listing filtered roles: %v", err)
		WriteInternalError(w, "Failed to list roles.")
		return
	}

	if rows == nil {
		rows = []datastore.RoleFilterRow{}
	}

	// Enrich rows with TK status from git_kitchen_results.
	if targetChefVersion != "" && len(rows) > 0 {
		roleNames := make([]string, 0, len(rows))
		for _, row := range rows {
			roleNames = append(roleNames, row.RoleName)
		}
		tkMap, _ := r.db.GetRoleTKStatuses(ctx, roleNames, orgNames, targetChefVersion)
		for i := range rows {
			if status, ok := tkMap[rows[i].RoleName]; ok {
				rows[i].TKStatus = status
			}
		}
	}

	// Apply TK status filter in memory.
	if tkFilterActive {
		allowed := make(map[string]bool)
		for _, v := range strings.Split(tkFilter, ",") {
			allowed[strings.TrimSpace(v)] = true
		}
		filtered := make([]datastore.RoleFilterRow, 0, len(rows))
		for _, row := range rows {
			tkVal := row.TKStatus
			if tkVal == "" {
				tkVal = "untested"
			}
			if allowed[tkVal] {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
		total = len(rows)
	}

	// Apply TK sort in memory.
	if tkSortActive {
		sortOrder := queryString(req, "order", "asc")
		sortRolesByTK(rows, sortOrder)
	}

	// Paginate in memory if we disabled SQL pagination.
	if tkFilterActive || tkSortActive {
		pageRows, _ := PaginateSlice(rows, pg)
		rows = pageRows
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
		TKStatus                string   `json:"tk_status,omitempty"`
	}

	result := make([]roleResp, 0, len(rows))
	for _, row := range rows {
		result = append(result, roleResp{
			RoleName:                row.RoleName,
			Organisations:           row.Organisations,
			NodeCount:               row.NodeCount,
			DirectCookbookCount:     row.DirectCookbookCount,
			TransitiveCookbookCount: row.TransitiveCookbookCount,
			TotalCookbookCount:      row.TotalCookbookCount,
			CompatibilityStatus:     row.CompatibilityStatus,
			CompatibleCount:         row.CompatibleCount,
			IncompatibleCount:       row.IncompatibleCount,
			UntestedCount:           row.UntestedCount,
			TKStatus:                row.TKStatus,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"data":       result,
		"summary":    summary,
		"pagination": NewPaginationResponse(pg, total),
	})
}

// sortRolesByTK sorts role rows by TK status in a deterministic order.
func sortRolesByTK(rows []datastore.RoleFilterRow, order string) {
	rank := map[string]int{
		"failed":   0,
		"partial":  1,
		"passed":   2,
		"untested": 3,
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri := rank[rows[i].TKStatus]
		rj := rank[rows[j].TKStatus]
		if _, ok := rank[rows[i].TKStatus]; !ok {
			ri = 3
		}
		if _, ok := rank[rows[j].TKStatus]; !ok {
			rj = 3
		}
		if order == "desc" {
			return ri > rj
		}
		return ri < rj
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

	// Build adjacency map.
	adj := make(map[string][]datastore.RoleDependency)
	for _, d := range deps {
		adj[d.RoleName] = append(adj[d.RoleName], d)
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
