// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package nodekitchen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
)

// maxExpansionDepth is the safety net for recursive role expansion.
const maxExpansionDepth = 50

// RunListEntry represents a parsed entry from a node's run_list.
type RunListEntry struct {
	Type       string // "role" or "recipe"
	Name       string // role name or cookbook name
	RecipeName string // recipe name (empty for roles, "default" if not specified)
}

// ExpandedRunList is the result of expanding a node's run_list.
type ExpandedRunList struct {
	// Recipes is the ordered, fully expanded list of recipe entries.
	Recipes []RunListEntry
	// Cookbooks is the set of cookbook names (with versions) needed.
	// Key is cookbook name, value is the version string.
	Cookbooks map[string]string
}

// RoleFetcher is the interface for fetching role details from Chef Server.
type RoleFetcher interface {
	GetRole(ctx context.Context, name string) (*chefapi.RoleDetail, error)
}

// CookbookDependencyResolver resolves transitive cookbook dependencies.
type CookbookDependencyResolver interface {
	// GetCookbookDependencies returns the dependencies map for a cookbook version.
	// The map keys are dependency cookbook names, values are version constraints.
	GetCookbookDependencies(ctx context.Context, orgName, cookbookName, version string) (map[string]string, error)
}

// ParseRunListEntry parses a single run_list entry string into a RunListEntry.
//
// Supported formats:
//   - "role[webserver]"
//   - "recipe[nginx]"
//   - "recipe[nginx::ssl]"
//   - "nginx"          (bare cookbook, implies recipe type, default recipe)
//   - "nginx::ssl"     (bare cookbook::recipe)
func ParseRunListEntry(entry string) (RunListEntry, error) {
	if entry == "" {
		return RunListEntry{}, fmt.Errorf("nodekitchen: empty run_list entry")
	}

	// Check for type[value] format.
	if idx := strings.Index(entry, "["); idx != -1 {
		if !strings.HasSuffix(entry, "]") {
			return RunListEntry{}, fmt.Errorf("nodekitchen: malformed run_list entry %q: missing closing bracket", entry)
		}
		typ := entry[:idx]
		value := entry[idx+1 : len(entry)-1]
		if value == "" {
			return RunListEntry{}, fmt.Errorf("nodekitchen: malformed run_list entry %q: empty value", entry)
		}
		switch typ {
		case "role":
			return RunListEntry{Type: "role", Name: value}, nil
		case "recipe":
			return parseRecipeValue(value, entry)
		default:
			return RunListEntry{}, fmt.Errorf("nodekitchen: unknown run_list entry type %q in %q", typ, entry)
		}
	}

	// Check for stray closing bracket without opening bracket.
	if strings.Contains(entry, "]") {
		return RunListEntry{}, fmt.Errorf("nodekitchen: malformed run_list entry %q", entry)
	}

	// Bare entry — treat as recipe.
	return parseRecipeValue(entry, entry)
}

// parseRecipeValue parses the inner value of a recipe entry (cookbook or cookbook::recipe).
func parseRecipeValue(value, original string) (RunListEntry, error) {
	if value == "" {
		return RunListEntry{}, fmt.Errorf("nodekitchen: malformed run_list entry %q: empty recipe value", original)
	}
	parts := strings.SplitN(value, "::", 2)
	name := parts[0]
	recipeName := "default"
	if len(parts) == 2 {
		recipeName = parts[1]
	}
	return RunListEntry{Type: "recipe", Name: name, RecipeName: recipeName}, nil
}

// ParseRunList unmarshals a JSON array of run_list strings and parses each entry.
func ParseRunList(raw json.RawMessage) ([]RunListEntry, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("nodekitchen: unmarshalling run_list: %w", err)
	}

	entries := make([]RunListEntry, 0, len(items))
	for i, item := range items {
		e, err := ParseRunListEntry(item)
		if err != nil {
			return nil, fmt.Errorf("nodekitchen: run_list entry %d: %w", i, err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// ExpandRunList recursively expands role entries into recipes via the RoleFetcher.
// It detects cycles and enforces a maximum recursion depth of 50.
func ExpandRunList(ctx context.Context, entries []RunListEntry, roleFetcher RoleFetcher) ([]RunListEntry, error) {
	visited := make(map[string]bool)
	return expandRunListRecursive(ctx, entries, roleFetcher, visited, 0)
}

func expandRunListRecursive(ctx context.Context, entries []RunListEntry, roleFetcher RoleFetcher, visited map[string]bool, depth int) ([]RunListEntry, error) {
	if depth > maxExpansionDepth {
		return nil, fmt.Errorf("nodekitchen: run_list expansion exceeded maximum depth of %d", maxExpansionDepth)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("nodekitchen: run_list expansion cancelled: %w", err)
	}

	var result []RunListEntry
	for _, entry := range entries {
		if entry.Type == "recipe" {
			result = append(result, entry)
			continue
		}

		// Role entry — expand it.
		if visited[entry.Name] {
			return nil, fmt.Errorf("nodekitchen: role cycle detected: %q already visited", entry.Name)
		}
		visited[entry.Name] = true

		role, err := roleFetcher.GetRole(ctx, entry.Name)
		if err != nil {
			return nil, fmt.Errorf("nodekitchen: fetching role %q: %w", entry.Name, err)
		}

		roleEntries := make([]RunListEntry, 0, len(role.RunList))
		for i, item := range role.RunList {
			e, err := ParseRunListEntry(item)
			if err != nil {
				return nil, fmt.Errorf("nodekitchen: role %q run_list entry %d: %w", entry.Name, i, err)
			}
			roleEntries = append(roleEntries, e)
		}

		expanded, err := expandRunListRecursive(ctx, roleEntries, roleFetcher, visited, depth+1)
		if err != nil {
			return nil, err
		}
		result = append(result, expanded...)
	}
	return result, nil
}

// ParseNodeCookbooks parses the node_snapshots.cookbooks JSONB field.
// The expected format is: {"cookbook_name": {"version": "1.0.0"}, ...}
// Returns a map of cookbook name to version string.
func ParseNodeCookbooks(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return make(map[string]string), nil
	}

	var outer map[string]struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &outer); err != nil {
		return nil, fmt.Errorf("nodekitchen: unmarshalling node cookbooks: %w", err)
	}

	result := make(map[string]string, len(outer))
	for name, info := range outer {
		result[name] = info.Version
	}
	return result, nil
}

// ResolveCookbookSet collects the complete set of cookbooks (with versions)
// needed for a list of recipes, including transitive dependencies.
func ResolveCookbookSet(
	ctx context.Context,
	recipes []RunListEntry,
	nodeCookbooks map[string]string,
	depResolver CookbookDependencyResolver,
	orgName string,
) (map[string]string, error) {
	result := make(map[string]string)

	// Seed with the directly-referenced cookbooks.
	var queue []string
	for _, r := range recipes {
		if _, exists := result[r.Name]; exists {
			continue
		}
		version, ok := nodeCookbooks[r.Name]
		if !ok {
			return nil, fmt.Errorf("nodekitchen: cookbook %q not found in node's cookbook versions", r.Name)
		}
		result[r.Name] = version
		queue = append(queue, r.Name)
	}

	// BFS to resolve transitive dependencies.
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("nodekitchen: cookbook resolution cancelled: %w", err)
		}

		current := queue[0]
		queue = queue[1:]

		deps, err := depResolver.GetCookbookDependencies(ctx, orgName, current, result[current])
		if err != nil {
			return nil, fmt.Errorf("nodekitchen: resolving dependencies for %s %s: %w", current, result[current], err)
		}

		for depName := range deps {
			if _, exists := result[depName]; exists {
				continue
			}
			version, ok := nodeCookbooks[depName]
			if !ok {
				return nil, fmt.Errorf("nodekitchen: dependency cookbook %q (required by %q) not found in node's cookbook versions", depName, current)
			}
			result[depName] = version
			queue = append(queue, depName)
		}
	}

	return result, nil
}
