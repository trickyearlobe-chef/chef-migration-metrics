// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"encoding/json"
	"fmt"
)

// maxCookbookDepth caps recursive cookbook→cookbook expansion to prevent
// runaway traversal on malformed or extremely deep dep graphs.
const maxCookbookDepth = 50

// cookbookDepRow holds one row from the server_cookbooks query: a cookbook
// name and its parsed list of dep cookbook names.
type cookbookDepRow struct {
	cookbookName string
	deps         []string
}

// parseCookbookDepsJSON parses the JSONB `dependencies` column from
// server_cookbooks (format: {"dep_name": "version_constraint", ...}) and
// returns a slice of dependency cookbook names (the keys). Returns an empty
// slice for nil, JSON null, or an empty object. Returns an error only for
// malformed JSON that is not null/empty.
func parseCookbookDepsJSON(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Detect and skip JSON null or bare arrays/scalars — only objects are valid.
	// Trim whitespace first.
	trimmed := raw
	for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t' || trimmed[0] == '\n') {
		trimmed = trimmed[1:]
	}
	if len(trimmed) == 0 || trimmed[0] != '{' {
		// null, array, scalar — treat as empty (no error).
		// But we still need to reject truly malformed JSON.
		var discard interface{}
		if err := json.Unmarshal(raw, &discard); err != nil {
			return nil, fmt.Errorf("parseCookbookDepsJSON: %w", err)
		}
		return nil, nil
	}

	var obj map[string]string
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("parseCookbookDepsJSON: %w", err)
	}

	if len(obj) == 0 {
		return nil, nil
	}

	deps := make([]string, 0, len(obj))
	for k := range obj {
		deps = append(deps, k)
	}
	return deps, nil
}

// buildCookbookAdjMap groups a slice of cookbookDepRows into an adjacency map
// (cookbook name → deduplicated dep cookbook names). Multiple active versions
// of the same cookbook have their dependency sets unioned.
func buildCookbookAdjMap(rows []cookbookDepRow) map[string][]string {
	adj := make(map[string][]string, len(rows))
	seen := make(map[string]map[string]bool, len(rows))

	for _, row := range rows {
		if _, ok := seen[row.cookbookName]; !ok {
			seen[row.cookbookName] = make(map[string]bool)
		}
		for _, dep := range row.deps {
			if !seen[row.cookbookName][dep] {
				seen[row.cookbookName][dep] = true
				adj[row.cookbookName] = append(adj[row.cookbookName], dep)
			}
		}
		// Ensure entry exists even if deps slice is empty, so callers can
		// distinguish "known cookbook with no deps" from "unknown cookbook".
		if _, ok := adj[row.cookbookName]; !ok {
			adj[row.cookbookName] = nil
		}
	}
	return adj
}

// buildCookbookChain recursively expands cookbook→cookbook dependencies,
// returning a tree of RoleChainNodes rooted at the named cookbook. The
// visited map guards against cycles; depth caps expansion at maxCookbookDepth.
// All visited cookbook names are added to the cookbooks set.
func buildCookbookChain(name string, cbAdj map[string][]string, visited map[string]bool, cookbooks map[string]bool, depth int) *RoleChainNode {
	cookbooks[name] = true

	node := &RoleChainNode{Name: name, Type: "cookbook"}

	if visited[name] || depth >= maxCookbookDepth {
		return node
	}
	visited[name] = true

	for _, dep := range cbAdj[name] {
		if visited[dep] {
			// Already expanded elsewhere in this traversal — record in the
			// cookbook set but don't re-expand (prevents cycle re-appearance).
			cookbooks[dep] = true
			continue
		}
		child := buildCookbookChain(dep, cbAdj, visited, cookbooks, depth+1)
		node.Children = append(node.Children, child)
	}
	return node
}

// ListCookbookDependenciesByOrg returns a cookbook adjacency map for all
// active cookbooks in the given organisation. The map keys are cookbook names;
// values are the deduplicated list of their dependency cookbook names (from
// server_cookbooks.dependencies JSONB). Multiple active versions of the same
// cookbook have their dep sets unioned.
//
// The returned map is used by buildCookbookChain to expand cookbook→cookbook
// transitive dependencies when computing role detail and dependency graphs.
func (db *DB) ListCookbookDependenciesByOrg(ctx context.Context, orgName string) (map[string][]string, error) {
	const query = `
		SELECT name, dependencies
		FROM server_cookbooks
		WHERE organisation_name = $1
		  AND is_active = true
		  AND dependencies IS NOT NULL
		  AND jsonb_typeof(dependencies) = 'object'
	`

	rows, err := db.pool.QueryContext(ctx, query, orgName)
	if err != nil {
		return nil, fmt.Errorf("datastore: querying cookbook dependencies: %w", err)
	}
	defer rows.Close()

	var depRows []cookbookDepRow

	for rows.Next() {
		var name string
		var rawDeps []byte
		if err := rows.Scan(&name, &rawDeps); err != nil {
			return nil, fmt.Errorf("datastore: scanning cookbook dependency row: %w", err)
		}

		deps, err := parseCookbookDepsJSON(rawDeps)
		if err != nil {
			// Log and skip malformed rows — don't fail the whole request.
			deps = nil
		}

		depRows = append(depRows, cookbookDepRow{cookbookName: name, deps: deps})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating cookbook dependency rows: %w", err)
	}

	return buildCookbookAdjMap(depRows), nil
}
