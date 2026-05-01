// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// AppliedMigration represents a row from the schema_migrations table.
type AppliedMigration struct {
	Version   int       `json:"version"`
	Name      string    `json:"name"`
	AppliedAt time.Time `json:"applied_at"`
}

// InventoryStatsResult holds aggregate inventory counts across all organisations.
type InventoryStatsResult struct {
	NodesByOrg         map[string]int      `json:"nodes_by_org"`
	CookbooksByOrg     map[string]int      `json:"cookbooks_by_org"`
	RolesByOrg         map[string]int      `json:"roles_by_org"`
	RoleDepEdgesByOrg  map[string]int      `json:"role_dep_edges_by_org"`
	GitRepoCount       int                 `json:"git_repo_count"`
	CookbookNamesByOrg map[string][]string `json:"cookbook_names_by_org,omitempty"`
	RoleNamesByOrg     map[string][]string `json:"role_names_by_org,omitempty"`
	GitRepoNames       []string            `json:"git_repo_names,omitempty"`
}

// OrgDepthStats holds dependency depth statistics for a single organisation.
type OrgDepthStats struct {
	MaxDepth     int            `json:"max_depth"`
	AvgDepth     float64        `json:"avg_depth"`
	Distribution map[string]int `json:"distribution"` // keys: "0", "1-5", "6-10", "11+"
}

// DeepestRole identifies the deepest role dependency chain in an organisation.
type DeepestRole struct {
	Org   string `json:"org"`
	Role  string `json:"role"`
	Depth int    `json:"depth"`
}

// DepthStatsResult holds dependency depth statistics for all organisations.
type DepthStatsResult struct {
	RoleDepDepthByOrg     map[string]OrgDepthStats `json:"role_dep_depth_by_org"`
	CookbookDepDepthByOrg map[string]OrgDepthStats `json:"cookbook_dep_depth_by_org"`
	DeepestRoles          []DeepestRole            `json:"deepest_roles,omitempty"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// depthBucket maps an integer depth to its distribution bucket label.
func depthBucket(d int) string {
	switch {
	case d == 0:
		return "0"
	case d <= 5:
		return "1-5"
	case d <= 10:
		return "6-10"
	default:
		return "11+"
	}
}

// buildDepthDistribution counts depths into the four standard buckets.
// All four keys are always present even when their count is zero.
func buildDepthDistribution(depths []int) map[string]int {
	dist := map[string]int{"0": 0, "1-5": 0, "6-10": 0, "11+": 0}
	for _, d := range depths {
		dist[depthBucket(d)]++
	}
	return dist
}

// buildOrgDepthStats computes OrgDepthStats from a slice of per-item max depths.
func buildOrgDepthStats(depths []int) OrgDepthStats {
	if len(depths) == 0 {
		return OrgDepthStats{Distribution: buildDepthDistribution(nil)}
	}
	maxDepth := 0
	sum := 0
	for _, d := range depths {
		if d > maxDepth {
			maxDepth = d
		}
		sum += d
	}
	return OrgDepthStats{
		MaxDepth:     maxDepth,
		AvgDepth:     float64(sum) / float64(len(depths)),
		Distribution: buildDepthDistribution(depths),
	}
}

// scanOrgCounts executes a query that returns (organisation_name, count) rows
// and populates the provided map.
func scanOrgCounts(ctx context.Context, db *DB, query string, m map[string]int) error {
	rows, err := db.pool.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var org string
		var count int
		if err := rows.Scan(&org, &count); err != nil {
			return err
		}
		m[org] = count
	}
	return rows.Err()
}

// scanOrgStringLists executes a query returning (organisation_name, name) rows
// and builds a map of sorted name slices keyed by organisation.
func scanOrgStringLists(ctx context.Context, db *DB, query string) (map[string][]string, error) {
	rows, err := db.pool.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string][]string)
	for rows.Next() {
		var org, name string
		if err := rows.Scan(&org, &name); err != nil {
			return nil, err
		}
		m[org] = append(m[org], name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for org := range m {
		sort.Strings(m[org])
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Methods
// ---------------------------------------------------------------------------

// ListAppliedMigrations returns all rows from schema_migrations ordered by
// version ascending. Returns an empty (non-nil) slice if no migrations exist.
// Returns an error if the schema_migrations table does not exist.
func (db *DB) ListAppliedMigrations(ctx context.Context) ([]AppliedMigration, error) {
	rows, err := db.pool.QueryContext(ctx,
		`SELECT version, name, applied_at FROM schema_migrations ORDER BY version ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing applied migrations: %w", err)
	}
	defer rows.Close()

	result := []AppliedMigration{}
	for rows.Next() {
		var m AppliedMigration
		if err := rows.Scan(&m.Version, &m.Name, &m.AppliedAt); err != nil {
			return nil, fmt.Errorf("datastore: scanning applied migration: %w", err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating applied migrations: %w", err)
	}
	return result, nil
}

// InventoryStats returns aggregate counts of nodes, active cookbooks, roles,
// role dependency edges, and git repos across all organisations. When
// includeNames is true, name lists for cookbooks, roles, and git repos are
// also populated. All maps are always initialised (never nil).
func (db *DB) InventoryStats(ctx context.Context, includeNames bool) (InventoryStatsResult, error) {
	result := InventoryStatsResult{
		NodesByOrg:        make(map[string]int),
		CookbooksByOrg:    make(map[string]int),
		RolesByOrg:        make(map[string]int),
		RoleDepEdgesByOrg: make(map[string]int),
	}

	if err := scanOrgCounts(ctx, db,
		`SELECT organisation_name, COUNT(*) FROM node_snapshots GROUP BY organisation_name`,
		result.NodesByOrg,
	); err != nil {
		return InventoryStatsResult{}, fmt.Errorf("datastore: inventory stats nodes: %w", err)
	}

	if err := scanOrgCounts(ctx, db,
		`SELECT organisation_name, COUNT(*) FROM server_cookbooks WHERE is_active = true GROUP BY organisation_name`,
		result.CookbooksByOrg,
	); err != nil {
		return InventoryStatsResult{}, fmt.Errorf("datastore: inventory stats cookbooks: %w", err)
	}

	if err := scanOrgCounts(ctx, db,
		`SELECT organisation_name, COUNT(DISTINCT role_name) FROM role_dependencies GROUP BY organisation_name`,
		result.RolesByOrg,
	); err != nil {
		return InventoryStatsResult{}, fmt.Errorf("datastore: inventory stats roles: %w", err)
	}

	if err := scanOrgCounts(ctx, db,
		`SELECT organisation_name, COUNT(*) FROM role_dependencies GROUP BY organisation_name`,
		result.RoleDepEdgesByOrg,
	); err != nil {
		return InventoryStatsResult{}, fmt.Errorf("datastore: inventory stats role dep edges: %w", err)
	}

	if err := db.pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM git_repos`).Scan(&result.GitRepoCount); err != nil {
		return InventoryStatsResult{}, fmt.Errorf("datastore: inventory stats git repo count: %w", err)
	}

	if !includeNames {
		return result, nil
	}

	cookbookNames, err := scanOrgStringLists(ctx, db,
		`SELECT organisation_name, name FROM server_cookbooks WHERE is_active = true ORDER BY organisation_name, name`,
	)
	if err != nil {
		return InventoryStatsResult{}, fmt.Errorf("datastore: inventory stats cookbook names: %w", err)
	}
	result.CookbookNamesByOrg = cookbookNames

	roleNames, err := scanOrgStringLists(ctx, db,
		`SELECT DISTINCT organisation_name, role_name FROM role_dependencies ORDER BY organisation_name, role_name`,
	)
	if err != nil {
		return InventoryStatsResult{}, fmt.Errorf("datastore: inventory stats role names: %w", err)
	}
	result.RoleNamesByOrg = roleNames

	rows, err := db.pool.QueryContext(ctx, `SELECT name FROM git_repos ORDER BY name`)
	if err != nil {
		return InventoryStatsResult{}, fmt.Errorf("datastore: inventory stats git repo names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return InventoryStatsResult{}, fmt.Errorf("datastore: scanning git repo name: %w", err)
		}
		result.GitRepoNames = append(result.GitRepoNames, name)
	}
	if err := rows.Err(); err != nil {
		return InventoryStatsResult{}, fmt.Errorf("datastore: iterating git repo names: %w", err)
	}

	return result, nil
}

// DependencyDepthStats computes transitive dependency depths for roles and
// cookbooks across all organisations. When includeNames is true, DeepestRoles
// is populated with the top 10 deepest role chains. All maps are always
// initialised (never nil).
func (db *DB) DependencyDepthStats(ctx context.Context, includeNames bool) (DepthStatsResult, error) {
	result := DepthStatsResult{
		RoleDepDepthByOrg:     make(map[string]OrgDepthStats),
		CookbookDepDepthByOrg: make(map[string]OrgDepthStats),
	}

	// --- Role dependency depths via recursive CTE ---
	const roleDepthQuery = `
		WITH RECURSIVE role_depths(organisation_name, root_role, current_role, depth) AS (
			SELECT DISTINCT organisation_name, role_name, role_name, 0
			FROM role_dependencies
			UNION ALL
			SELECT rd.organisation_name, rd.root_role, dep.dependency_name, rd.depth + 1
			FROM role_depths rd
			JOIN role_dependencies dep
			  ON dep.organisation_name = rd.organisation_name
			  AND dep.role_name = rd.current_role
			  AND dep.dependency_type = 'role'
			WHERE rd.depth < 50
		),
		max_depths AS (
			SELECT organisation_name, root_role, MAX(depth) AS max_depth
			FROM role_depths
			GROUP BY organisation_name, root_role
		)
		SELECT organisation_name, root_role, max_depth
		FROM max_depths
		ORDER BY organisation_name, max_depth DESC
	`

	roleRows, err := db.pool.QueryContext(ctx, roleDepthQuery)
	if err != nil {
		return DepthStatsResult{}, fmt.Errorf("datastore: dependency depth stats role query: %w", err)
	}
	defer roleRows.Close()

	type roleEntry struct {
		role  string
		depth int
	}
	rolesByOrg := make(map[string][]roleEntry)

	for roleRows.Next() {
		var org, role string
		var depth int
		if err := roleRows.Scan(&org, &role, &depth); err != nil {
			return DepthStatsResult{}, fmt.Errorf("datastore: scanning role depth row: %w", err)
		}
		rolesByOrg[org] = append(rolesByOrg[org], roleEntry{role: role, depth: depth})
	}
	if err := roleRows.Err(); err != nil {
		return DepthStatsResult{}, fmt.Errorf("datastore: iterating role depth rows: %w", err)
	}

	for org, entries := range rolesByOrg {
		depths := make([]int, len(entries))
		for i, e := range entries {
			depths[i] = e.depth
		}
		result.RoleDepDepthByOrg[org] = buildOrgDepthStats(depths)
	}

	if includeNames {
		type triple struct {
			org   string
			role  string
			depth int
		}
		var triples []triple
		for org, entries := range rolesByOrg {
			for _, e := range entries {
				triples = append(triples, triple{org: org, role: e.role, depth: e.depth})
			}
		}
		sort.Slice(triples, func(i, j int) bool {
			if triples[i].depth != triples[j].depth {
				return triples[i].depth > triples[j].depth
			}
			if triples[i].org != triples[j].org {
				return triples[i].org < triples[j].org
			}
			return triples[i].role < triples[j].role
		})
		n := len(triples)
		if n > 10 {
			n = 10
		}
		result.DeepestRoles = make([]DeepestRole, n)
		for i, t := range triples[:n] {
			result.DeepestRoles[i] = DeepestRole{Org: t.org, Role: t.role, Depth: t.depth}
		}
	}

	// --- Cookbook dependency depths via recursive CTE on JSONB ---
	const cbDepthQuery = `
		WITH RECURSIVE cb_edges AS (
			SELECT organisation_name, name AS from_cb, jsonb_object_keys(dependencies) AS to_cb
			FROM server_cookbooks
			WHERE is_active = true AND dependencies IS NOT NULL AND dependencies != '{}'::jsonb
		),
		cb_depths(organisation_name, root_cb, current_cb, depth) AS (
			SELECT DISTINCT organisation_name, from_cb, from_cb, 0 FROM cb_edges
			UNION ALL
			SELECT cd.organisation_name, cd.root_cb, e.to_cb, cd.depth + 1
			FROM cb_depths cd
			JOIN cb_edges e
			  ON e.organisation_name = cd.organisation_name
			  AND e.from_cb = cd.current_cb
			WHERE cd.depth < 20
		),
		cb_max_depths AS (
			SELECT organisation_name, root_cb, MAX(depth) AS max_depth
			FROM cb_depths
			GROUP BY organisation_name, root_cb
		)
		SELECT organisation_name, root_cb, max_depth
		FROM cb_max_depths
		ORDER BY organisation_name, max_depth DESC
	`

	cbRows, err := db.pool.QueryContext(ctx, cbDepthQuery)
	if err != nil {
		return DepthStatsResult{}, fmt.Errorf("datastore: dependency depth stats cookbook query: %w", err)
	}
	defer cbRows.Close()

	cbDepthsByOrg := make(map[string][]int)
	for cbRows.Next() {
		var org, cb string
		var depth int
		if err := cbRows.Scan(&org, &cb, &depth); err != nil {
			return DepthStatsResult{}, fmt.Errorf("datastore: scanning cookbook depth row: %w", err)
		}
		cbDepthsByOrg[org] = append(cbDepthsByOrg[org], depth)
	}
	if err := cbRows.Err(); err != nil {
		return DepthStatsResult{}, fmt.Errorf("datastore: iterating cookbook depth rows: %w", err)
	}

	for org, depths := range cbDepthsByOrg {
		result.CookbookDepDepthByOrg[org] = buildOrgDepthStats(depths)
	}

	return result, nil
}
