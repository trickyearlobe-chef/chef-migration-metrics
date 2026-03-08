// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"regexp"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// runListEntryPattern matches Chef run_list entries in the forms:
//
//	recipe[cookbook]
//	recipe[cookbook::recipe_name]
//	recipe[cookbook@1.0.0]
//	recipe[cookbook::recipe_name@1.0.0]
//	role[role_name]
//
// Capture groups:
//
//	1 = entry type ("recipe" or "role")
//	2 = full content inside brackets
var runListEntryPattern = regexp.MustCompile(`^(recipe|role)\[([^\]]+)\]$`)

// ParsedRunListEntry holds a single parsed run_list entry.
type ParsedRunListEntry struct {
	// Type is either "recipe" or "role".
	Type string

	// Name is the cookbook name (for recipes) or role name (for roles).
	// For recipe entries, this is the cookbook name only (without the recipe
	// name or version pin).
	Name string
}

// ParseRunListEntry parses a single Chef run_list entry string into its
// type and name. Returns ok=false if the entry does not match the expected
// pattern.
//
// Examples:
//
//	"recipe[apache2]"              → {Type: "recipe", Name: "apache2"}, true
//	"recipe[apache2::default]"     → {Type: "recipe", Name: "apache2"}, true
//	"recipe[apache2@2.0.0]"        → {Type: "recipe", Name: "apache2"}, true
//	"role[webserver]"              → {Type: "role", Name: "webserver"}, true
//	"garbage"                      → {}, false
func ParseRunListEntry(entry string) (ParsedRunListEntry, bool) {
	entry = strings.TrimSpace(entry)
	matches := runListEntryPattern.FindStringSubmatch(entry)
	if matches == nil {
		return ParsedRunListEntry{}, false
	}

	entryType := matches[1]
	content := matches[2]

	if entryType == "role" {
		return ParsedRunListEntry{
			Type: "role",
			Name: content,
		}, true
	}

	// For recipes, extract just the cookbook name:
	//   "cookbook::recipe@version" → "cookbook"
	//   "cookbook::recipe"         → "cookbook"
	//   "cookbook@version"         → "cookbook"
	//   "cookbook"                 → "cookbook"
	name := content

	// Strip version pin first (everything after @).
	if idx := strings.Index(name, "@"); idx >= 0 {
		name = name[:idx]
	}

	// Strip recipe name (everything after ::).
	if idx := strings.Index(name, "::"); idx >= 0 {
		name = name[:idx]
	}

	return ParsedRunListEntry{
		Type: "recipe",
		Name: name,
	}, true
}

// ParseRunList parses a complete Chef run_list (a slice of entry strings)
// and returns all parsed entries. Invalid entries are silently skipped.
func ParseRunList(runList []string) []ParsedRunListEntry {
	var entries []ParsedRunListEntry
	for _, raw := range runList {
		if parsed, ok := ParseRunListEntry(raw); ok {
			entries = append(entries, parsed)
		}
	}
	return entries
}

// BuildRoleDependencies builds role dependency records from a set of Chef
// role details. For each role, it parses the default run_list and all
// environment-specific run_lists (env_run_lists) to extract:
//   - cookbook dependencies (from recipe[...] entries)
//   - role dependencies (from role[...] entries)
//
// The results are deduplicated per role — if the same dependency appears in
// both the default run_list and an environment run_list, only one record is
// produced.
func BuildRoleDependencies(organisationID string, roles []*chefapi.RoleDetail) []datastore.InsertRoleDependencyParams {
	var params []datastore.InsertRoleDependencyParams

	for _, role := range roles {
		if role == nil {
			continue
		}

		// Use a set to deduplicate dependencies within a single role.
		// Key: "type:name"
		seen := make(map[string]bool)

		// Parse the default run_list.
		addDeps := func(runList []string) {
			for _, entry := range ParseRunList(runList) {
				depType := "cookbook"
				if entry.Type == "role" {
					depType = "role"
				}

				key := depType + ":" + entry.Name
				if seen[key] {
					continue
				}
				seen[key] = true

				params = append(params, datastore.InsertRoleDependencyParams{
					OrganisationID: organisationID,
					RoleName:       role.Name,
					DependencyType: depType,
					DependencyName: entry.Name,
				})
			}
		}

		addDeps(role.RunList)

		// Parse all environment-specific run_lists.
		for _, envRunList := range role.EnvRunLists {
			addDeps(envRunList)
		}
	}

	return params
}
