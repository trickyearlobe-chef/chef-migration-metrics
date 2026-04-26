// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"

	"github.com/lib/pq"
)

// RoleDetail is the full detail view for a single role, used by the
// GET /api/v1/roles/:name endpoint.
type RoleDetail struct {
	RoleName            string             `json:"role_name"`
	Organisations       []string           `json:"organisations"`
	NodeCount           int                `json:"node_count"`
	DirectCookbooks     []string           `json:"direct_cookbooks"`
	DirectRoles         []string           `json:"direct_roles"`
	TransitiveCookbooks []string           `json:"transitive_cookbooks"`
	BlockingCookbooks   []BlockingCookbook `json:"blocking_cookbooks"`
	NestedRoleChain     *RoleChainNode     `json:"nested_role_chain"`
	NodesByOrganisation []OrgCount         `json:"nodes_by_organisation"`
	NodesByEnvironment  []EnvCount         `json:"nodes_by_environment"`
	NodesByPlatform     []PlatformCount    `json:"nodes_by_platform"`
}

// BlockingCookbook represents a cookbook that makes a role incompatible.
type BlockingCookbook struct {
	CookbookName      string   `json:"cookbook_name"`
	CookbookVersion   string   `json:"cookbook_version"`
	TargetChefVersion string   `json:"target_chef_version"`
	ComplexityScore   int      `json:"complexity_score"`
	ComplexityLabel   string   `json:"complexity_label"`
	AutoCorrectable   int      `json:"auto_correctable"`
	ManualFix         int      `json:"manual_fix"`
	DependencyPath    []string `json:"dependency_path"`
}

// RoleChainNode represents a node in the nested role expansion tree.
type RoleChainNode struct {
	Name                string           `json:"name"`
	Type                string           `json:"type"` // "role" or "cookbook"
	CompatibilityStatus string           `json:"compatibility_status,omitempty"`
	Children            []*RoleChainNode `json:"children,omitempty"`
}

// pathEntry holds a cookbook name and the dependency path used to reach it.
type pathEntry struct {
	cookbook string
	path     []string
}

// OrgCount is a count grouped by organisation.
type OrgCount struct {
	Organisation string `json:"organisation"`
	Count        int    `json:"count"`
}

// EnvCount is a count grouped by environment.
type EnvCount struct {
	Environment string `json:"environment"`
	Count       int    `json:"count"`
}

// PlatformCount is a count grouped by platform+version.
type PlatformCount struct {
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platform_version"`
	Count           int    `json:"count"`
}

// GetRoleDetail builds the full detail view for a single role. It queries
// across organisations to find all places this role exists, then expands
// its dependency tree transitively and computes blast radius.
func (db *DB) GetRoleDetail(ctx context.Context, roleName, targetChefVersion string) (*RoleDetail, error) {
	// 1. Find all organisations where this role exists.
	orgs, err := db.getRoleOrganisations(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("datastore: getting role organisations: %w", err)
	}
	if len(orgs) == 0 {
		return nil, ErrNotFound
	}

	// 2. Get direct dependencies (use first org as canonical; role deps are
	// typically identical across orgs).
	directDeps, err := db.ListRoleDependenciesByRole(ctx, orgs[0], roleName)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing direct deps for role: %w", err)
	}

	var directCookbooks, directRoles []string
	for _, d := range directDeps {
		switch d.DependencyType {
		case "cookbook":
			directCookbooks = append(directCookbooks, d.DependencyName)
		case "role":
			directRoles = append(directRoles, d.DependencyName)
		}
	}

	// 3. Build transitive closure and nested role chain tree.
	allDeps, err := db.ListRoleDependenciesByOrg(ctx, orgs[0])
	if err != nil {
		return nil, fmt.Errorf("datastore: listing all org deps for role detail: %w", err)
	}

	// Build adjacency map: role -> []RoleDependency
	adj := make(map[string][]RoleDependency)
	for _, d := range allDeps {
		adj[d.RoleName] = append(adj[d.RoleName], d)
	}

	// Walk the tree to collect transitive cookbooks and build chain.
	visited := make(map[string]bool)
	var transitiveCookbooks []string
	cookbookSet := make(map[string]bool)

	chain := buildRoleChain(roleName, adj, visited, cookbookSet)

	for cb := range cookbookSet {
		transitiveCookbooks = append(transitiveCookbooks, cb)
	}

	// 4. Get blocking cookbooks (incompatible ones with complexity).
	var blockingCookbooks []BlockingCookbook
	if targetChefVersion != "" {
		blockingCookbooks, err = db.getBlockingCookbooks(ctx, orgs[0], roleName, targetChefVersion, adj)
		if err != nil {
			return nil, fmt.Errorf("datastore: getting blocking cookbooks: %w", err)
		}
	}

	// 5. Blast radius: node counts.
	nodeCount, nodesByOrg, nodesByEnv, nodesByPlatform, err := db.getRoleBlastRadius(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("datastore: getting role blast radius: %w", err)
	}

	// 6. Set compatibility status on chain tree nodes if target version given.
	if targetChefVersion != "" {
		compatMap, _ := db.getCookbookCompatMap(ctx, orgs[0], targetChefVersion)
		setChainCompatibility(chain, compatMap)
	}

	if directCookbooks == nil {
		directCookbooks = []string{}
	}
	if directRoles == nil {
		directRoles = []string{}
	}
	if transitiveCookbooks == nil {
		transitiveCookbooks = []string{}
	}
	if blockingCookbooks == nil {
		blockingCookbooks = []BlockingCookbook{}
	}
	if nodesByOrg == nil {
		nodesByOrg = []OrgCount{}
	}
	if nodesByEnv == nil {
		nodesByEnv = []EnvCount{}
	}
	if nodesByPlatform == nil {
		nodesByPlatform = []PlatformCount{}
	}

	return &RoleDetail{
		RoleName:            roleName,
		Organisations:       orgs,
		NodeCount:           nodeCount,
		DirectCookbooks:     directCookbooks,
		DirectRoles:         directRoles,
		TransitiveCookbooks: transitiveCookbooks,
		BlockingCookbooks:   blockingCookbooks,
		NestedRoleChain:     chain,
		NodesByOrganisation: nodesByOrg,
		NodesByEnvironment:  nodesByEnv,
		NodesByPlatform:     nodesByPlatform,
	}, nil
}

// getRoleOrganisations returns distinct organisation names where a role exists.
func (db *DB) getRoleOrganisations(ctx context.Context, roleName string) ([]string, error) {
	const query = `
		SELECT DISTINCT organisation_name
		FROM role_dependencies
		WHERE role_name = $1
		ORDER BY organisation_name
	`
	rows, err := db.pool.QueryContext(ctx, query, roleName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []string
	for rows.Next() {
		var org string
		if err := rows.Scan(&org); err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	return orgs, rows.Err()
}

// buildRoleChain recursively builds the nested role chain tree.
func buildRoleChain(roleName string, adj map[string][]RoleDependency, visited map[string]bool, cookbooks map[string]bool) *RoleChainNode {
	if visited[roleName] {
		return &RoleChainNode{Name: roleName, Type: "role"}
	}
	visited[roleName] = true

	node := &RoleChainNode{Name: roleName, Type: "role"}
	deps := adj[roleName]

	for _, d := range deps {
		switch d.DependencyType {
		case "role":
			child := buildRoleChain(d.DependencyName, adj, visited, cookbooks)
			node.Children = append(node.Children, child)
		case "cookbook":
			cookbooks[d.DependencyName] = true
			node.Children = append(node.Children, &RoleChainNode{
				Name: d.DependencyName,
				Type: "cookbook",
			})
		}
	}
	return node
}

// getBlockingCookbooks finds cookbooks that make a role incompatible for the
// given target version. Walks the dependency tree to find dependency paths.
func (db *DB) getBlockingCookbooks(ctx context.Context, orgName, roleName, targetChefVersion string, adj map[string][]RoleDependency) ([]BlockingCookbook, error) {
	// Get all transitive cookbooks with their dependency paths.
	var entries []pathEntry
	visited := make(map[string]bool)
	collectPaths(roleName, adj, visited, []string{"role:" + roleName}, &entries)

	if len(entries) == 0 {
		return nil, nil
	}

	// Collect unique cookbook names.
	cookbookNames := make([]string, 0, len(entries))
	seen := make(map[string]bool)
	for _, e := range entries {
		if !seen[e.cookbook] {
			cookbookNames = append(cookbookNames, e.cookbook)
			seen[e.cookbook] = true
		}
	}

	// Query cookstyle results + complexity for these cookbooks.
	const query = `
		SELECT sc.name, sc.version,
			CASE
				WHEN csr.passed = false THEN true
				ELSE false
			END AS is_incompatible,
			COALESCE(scc.complexity_score, 0),
			COALESCE(scc.complexity_label, 'none'),
			COALESCE(scc.auto_correctable_count, 0),
			COALESCE(scc.manual_fix_count, 0)
		FROM server_cookbooks sc
		LEFT JOIN server_cookbook_cookstyle_results csr
			ON csr.organisation_name = sc.organisation_name
			AND csr.cookbook_name = sc.name
			AND csr.cookbook_version = sc.version
			AND csr.target_chef_version = $1
		LEFT JOIN server_cookbook_complexities scc
			ON scc.organisation_name = sc.organisation_name
			AND scc.cookbook_name = sc.name
			AND scc.cookbook_version = sc.version
			AND scc.target_chef_version = $1
		WHERE sc.organisation_name = $2
			AND sc.name = ANY($3)
	`

	rows, err := db.pool.QueryContext(ctx, query, targetChefVersion, orgName, pq.Array(cookbookNames))
	if err != nil {
		return nil, fmt.Errorf("datastore: querying blocking cookbooks: %w", err)
	}
	defer rows.Close()

	// Build a lookup of incompatible cookbooks.
	type cbInfo struct {
		version         string
		complexityScore int
		complexityLabel string
		autoCorrectable int
		manualFix       int
	}
	incompatible := make(map[string]cbInfo)

	for rows.Next() {
		var name, version, complexityLabel string
		var isIncompat bool
		var score, auto, manual int
		if err := rows.Scan(&name, &version, &isIncompat, &score, &complexityLabel, &auto, &manual); err != nil {
			return nil, fmt.Errorf("datastore: scanning blocking cookbook: %w", err)
		}
		if isIncompat {
			incompatible[name] = cbInfo{
				version:         version,
				complexityScore: score,
				complexityLabel: complexityLabel,
				autoCorrectable: auto,
				manualFix:       manual,
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Build result with shortest dependency path for each incompatible cookbook.
	pathMap := make(map[string][]string)
	for _, e := range entries {
		if _, ok := incompatible[e.cookbook]; ok {
			if existing, exists := pathMap[e.cookbook]; !exists || len(e.path) < len(existing) {
				pathMap[e.cookbook] = e.path
			}
		}
	}

	var result []BlockingCookbook
	for name, info := range incompatible {
		depPath := pathMap[name]
		if depPath == nil {
			depPath = []string{"role:" + roleName, "cookbook:" + name}
		}
		result = append(result, BlockingCookbook{
			CookbookName:      name,
			CookbookVersion:   info.version,
			TargetChefVersion: targetChefVersion,
			ComplexityScore:   info.complexityScore,
			ComplexityLabel:   info.complexityLabel,
			AutoCorrectable:   info.autoCorrectable,
			ManualFix:         info.manualFix,
			DependencyPath:    depPath,
		})
	}

	return result, nil
}

// collectPaths walks the dependency tree collecting cookbook paths.
func collectPaths(roleName string, adj map[string][]RoleDependency, visited map[string]bool, currentPath []string, entries *[]pathEntry) {
	if visited[roleName] {
		return
	}
	visited[roleName] = true

	for _, d := range adj[roleName] {
		switch d.DependencyType {
		case "cookbook":
			path := make([]string, len(currentPath)+1)
			copy(path, currentPath)
			path[len(currentPath)] = "cookbook:" + d.DependencyName
			*entries = append(*entries, pathEntry{cookbook: d.DependencyName, path: path})
		case "role":
			newPath := make([]string, len(currentPath)+1)
			copy(newPath, currentPath)
			newPath[len(currentPath)] = "role:" + d.DependencyName
			collectPaths(d.DependencyName, adj, visited, newPath, entries)
		}
	}
}

// getRoleBlastRadius computes node counts by org, environment, and platform
// for nodes that have this role.
func (db *DB) getRoleBlastRadius(ctx context.Context, roleName string) (int, []OrgCount, []EnvCount, []PlatformCount, error) {
	// Total node count
	var totalCount int
	err := db.pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM node_snapshots WHERE roles @> to_jsonb(ARRAY[$1])`,
		roleName,
	).Scan(&totalCount)
	if err != nil {
		return 0, nil, nil, nil, fmt.Errorf("counting nodes for role: %w", err)
	}

	// By organisation
	orgRows, err := db.pool.QueryContext(ctx,
		`SELECT organisation_name, COUNT(*) FROM node_snapshots
		 WHERE roles @> to_jsonb(ARRAY[$1])
		 GROUP BY organisation_name ORDER BY COUNT(*) DESC`,
		roleName,
	)
	if err != nil {
		return totalCount, nil, nil, nil, fmt.Errorf("counting nodes by org: %w", err)
	}
	defer orgRows.Close()

	var orgCounts []OrgCount
	for orgRows.Next() {
		var oc OrgCount
		if err := orgRows.Scan(&oc.Organisation, &oc.Count); err != nil {
			return totalCount, nil, nil, nil, err
		}
		orgCounts = append(orgCounts, oc)
	}

	// By environment
	envRows, err := db.pool.QueryContext(ctx,
		`SELECT chef_environment, COUNT(*) FROM node_snapshots
		 WHERE roles @> to_jsonb(ARRAY[$1])
		 GROUP BY chef_environment ORDER BY COUNT(*) DESC`,
		roleName,
	)
	if err != nil {
		return totalCount, orgCounts, nil, nil, fmt.Errorf("counting nodes by env: %w", err)
	}
	defer envRows.Close()

	var envCounts []EnvCount
	for envRows.Next() {
		var ec EnvCount
		if err := envRows.Scan(&ec.Environment, &ec.Count); err != nil {
			return totalCount, orgCounts, nil, nil, err
		}
		envCounts = append(envCounts, ec)
	}

	// By platform
	platRows, err := db.pool.QueryContext(ctx,
		`SELECT platform, COALESCE(platform_version, ''), COUNT(*) FROM node_snapshots
		 WHERE roles @> to_jsonb(ARRAY[$1])
		 GROUP BY platform, platform_version ORDER BY COUNT(*) DESC`,
		roleName,
	)
	if err != nil {
		return totalCount, orgCounts, envCounts, nil, fmt.Errorf("counting nodes by platform: %w", err)
	}
	defer platRows.Close()

	var platCounts []PlatformCount
	for platRows.Next() {
		var pc PlatformCount
		if err := platRows.Scan(&pc.Platform, &pc.PlatformVersion, &pc.Count); err != nil {
			return totalCount, orgCounts, envCounts, nil, err
		}
		platCounts = append(platCounts, pc)
	}

	return totalCount, orgCounts, envCounts, platCounts, nil
}

// getCookbookCompatMap returns a map of cookbook_name -> compatibility_status
// for all server cookbooks in an org for the given target version.
func (db *DB) getCookbookCompatMap(ctx context.Context, orgName, targetChefVersion string) (map[string]string, error) {
	const query = `
		SELECT sc.name,
			CASE
				WHEN csr.error_message IS NOT NULL AND csr.error_message != '' THEN 'untested'
				WHEN csr.passed = true THEN 'compatible'
				WHEN csr.passed = false THEN 'incompatible'
				ELSE 'untested'
			END AS status
		FROM server_cookbooks sc
		LEFT JOIN server_cookbook_cookstyle_results csr
			ON csr.organisation_name = sc.organisation_name
			AND csr.cookbook_name = sc.name
			AND csr.cookbook_version = sc.version
			AND csr.target_chef_version = $1
		WHERE sc.organisation_name = $2
	`

	rows, err := db.pool.QueryContext(ctx, query, targetChefVersion, orgName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var name, status string
		if err := rows.Scan(&name, &status); err != nil {
			return nil, err
		}
		// If multiple versions, take worst status.
		existing, ok := result[name]
		if !ok || statusPriority(status) > statusPriority(existing) {
			result[name] = status
		}
	}
	return result, rows.Err()
}

// statusPriority returns a numeric priority for compatibility status
// (higher = worse). Used to pick worst status across multiple versions.
func statusPriority(status string) int {
	switch status {
	case "incompatible":
		return 2
	case "untested":
		return 1
	default:
		return 0
	}
}

// setChainCompatibility sets compatibility_status on cookbook nodes in the
// role chain tree using the provided compatibility map.
func setChainCompatibility(node *RoleChainNode, compatMap map[string]string) {
	if node == nil {
		return
	}
	if node.Type == "cookbook" {
		if status, ok := compatMap[node.Name]; ok {
			node.CompatibilityStatus = status
		} else {
			node.CompatibilityStatus = "untested"
		}
	}
	for _, child := range node.Children {
		setChainCompatibility(child, compatMap)
	}
}
