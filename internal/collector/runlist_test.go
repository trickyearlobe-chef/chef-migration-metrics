// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"sort"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// ParseRunListEntry
// ---------------------------------------------------------------------------

func TestParseRunListEntry_RecipeSimple(t *testing.T) {
	entry, ok := ParseRunListEntry("recipe[apache2]")
	if !ok {
		t.Fatal("expected ok=true for recipe[apache2]")
	}
	if entry.Type != "recipe" {
		t.Errorf("expected Type=recipe, got %q", entry.Type)
	}
	if entry.Name != "apache2" {
		t.Errorf("expected Name=apache2, got %q", entry.Name)
	}
}

func TestParseRunListEntry_RecipeWithRecipeName(t *testing.T) {
	entry, ok := ParseRunListEntry("recipe[apache2::mod_ssl]")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Type != "recipe" {
		t.Errorf("expected Type=recipe, got %q", entry.Type)
	}
	if entry.Name != "apache2" {
		t.Errorf("expected Name=apache2, got %q", entry.Name)
	}
}

func TestParseRunListEntry_RecipeWithVersion(t *testing.T) {
	entry, ok := ParseRunListEntry("recipe[apache2@2.0.0]")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Type != "recipe" {
		t.Errorf("expected Type=recipe, got %q", entry.Type)
	}
	if entry.Name != "apache2" {
		t.Errorf("expected Name=apache2, got %q", entry.Name)
	}
}

func TestParseRunListEntry_RecipeWithRecipeNameAndVersion(t *testing.T) {
	entry, ok := ParseRunListEntry("recipe[apache2::mod_ssl@2.0.0]")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Type != "recipe" {
		t.Errorf("expected Type=recipe, got %q", entry.Type)
	}
	if entry.Name != "apache2" {
		t.Errorf("expected Name=apache2, got %q", entry.Name)
	}
}

func TestParseRunListEntry_RecipeDefault(t *testing.T) {
	entry, ok := ParseRunListEntry("recipe[apache2::default]")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Name != "apache2" {
		t.Errorf("expected Name=apache2, got %q", entry.Name)
	}
}

func TestParseRunListEntry_Role(t *testing.T) {
	entry, ok := ParseRunListEntry("role[webserver]")
	if !ok {
		t.Fatal("expected ok=true for role[webserver]")
	}
	if entry.Type != "role" {
		t.Errorf("expected Type=role, got %q", entry.Type)
	}
	if entry.Name != "webserver" {
		t.Errorf("expected Name=webserver, got %q", entry.Name)
	}
}

func TestParseRunListEntry_RoleWithDashes(t *testing.T) {
	entry, ok := ParseRunListEntry("role[my-web-server]")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Type != "role" {
		t.Errorf("expected Type=role, got %q", entry.Type)
	}
	if entry.Name != "my-web-server" {
		t.Errorf("expected Name=my-web-server, got %q", entry.Name)
	}
}

func TestParseRunListEntry_RoleWithUnderscores(t *testing.T) {
	entry, ok := ParseRunListEntry("role[my_web_server]")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Name != "my_web_server" {
		t.Errorf("expected Name=my_web_server, got %q", entry.Name)
	}
}

func TestParseRunListEntry_WhitespaceIsTrimmed(t *testing.T) {
	entry, ok := ParseRunListEntry("  recipe[ntp]  ")
	if !ok {
		t.Fatal("expected ok=true after trimming whitespace")
	}
	if entry.Name != "ntp" {
		t.Errorf("expected Name=ntp, got %q", entry.Name)
	}
}

func TestParseRunListEntry_InvalidEntries(t *testing.T) {
	invalids := []string{
		"",
		"garbage",
		"recipe[]",
		"role[]",
		"recipe[",
		"role[",
		"recipe",
		"role",
		"[apache2]",
		"unknown[apache2]",
		"RECIPE[apache2]",
		"ROLE[webserver]",
		"Recipe[apache2]",
		"recipe [apache2]",
	}
	for _, entry := range invalids {
		_, ok := ParseRunListEntry(entry)
		if ok {
			t.Errorf("expected ok=false for %q", entry)
		}
	}
}

func TestParseRunListEntry_CookbookNameWithHyphens(t *testing.T) {
	entry, ok := ParseRunListEntry("recipe[my-cookbook]")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Name != "my-cookbook" {
		t.Errorf("expected Name=my-cookbook, got %q", entry.Name)
	}
}

func TestParseRunListEntry_CookbookNameWithUnderscores(t *testing.T) {
	entry, ok := ParseRunListEntry("recipe[my_cookbook]")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Name != "my_cookbook" {
		t.Errorf("expected Name=my_cookbook, got %q", entry.Name)
	}
}

func TestParseRunListEntry_RecipeWithComplexVersion(t *testing.T) {
	entry, ok := ParseRunListEntry("recipe[java::default@11.0.2+9]")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if entry.Type != "recipe" {
		t.Errorf("expected Type=recipe, got %q", entry.Type)
	}
	if entry.Name != "java" {
		t.Errorf("expected Name=java, got %q", entry.Name)
	}
}

// ---------------------------------------------------------------------------
// ParseRunList
// ---------------------------------------------------------------------------

func TestParseRunList_Empty(t *testing.T) {
	entries := ParseRunList(nil)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for nil input, got %d", len(entries))
	}

	entries = ParseRunList([]string{})
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty input, got %d", len(entries))
	}
}

func TestParseRunList_MixedEntries(t *testing.T) {
	runList := []string{
		"recipe[apache2]",
		"role[webserver]",
		"recipe[ntp::default]",
		"garbage_entry",
		"role[base]",
		"recipe[java@11.0.0]",
	}

	entries := ParseRunList(runList)
	if len(entries) != 5 {
		t.Fatalf("expected 5 valid entries, got %d", len(entries))
	}

	expected := []ParsedRunListEntry{
		{Type: "recipe", Name: "apache2"},
		{Type: "role", Name: "webserver"},
		{Type: "recipe", Name: "ntp"},
		{Type: "role", Name: "base"},
		{Type: "recipe", Name: "java"},
	}

	for i, e := range expected {
		if entries[i].Type != e.Type {
			t.Errorf("entry %d: expected Type=%q, got %q", i, e.Type, entries[i].Type)
		}
		if entries[i].Name != e.Name {
			t.Errorf("entry %d: expected Name=%q, got %q", i, e.Name, entries[i].Name)
		}
	}
}

func TestParseRunList_AllInvalid(t *testing.T) {
	runList := []string{"garbage", "more garbage", ""}
	entries := ParseRunList(runList)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for all-invalid input, got %d", len(entries))
	}
}

func TestParseRunList_OnlyRecipes(t *testing.T) {
	runList := []string{
		"recipe[a]",
		"recipe[b::default]",
		"recipe[c@1.0.0]",
	}
	entries := ParseRunList(runList)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Type != "recipe" {
			t.Errorf("expected all entries to be recipe, got %q", e.Type)
		}
	}
}

func TestParseRunList_OnlyRoles(t *testing.T) {
	runList := []string{
		"role[base]",
		"role[webserver]",
		"role[database]",
	}
	entries := ParseRunList(runList)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Type != "role" {
			t.Errorf("expected all entries to be role, got %q", e.Type)
		}
	}
}

// ---------------------------------------------------------------------------
// BuildRoleDependencies
// ---------------------------------------------------------------------------

func TestBuildRoleDependencies_Empty(t *testing.T) {
	result := BuildRoleDependencies("org-1", nil)
	if len(result) != 0 {
		t.Errorf("expected 0 params for nil roles, got %d", len(result))
	}

	result = BuildRoleDependencies("org-1", []*chefapi.RoleDetail{})
	if len(result) != 0 {
		t.Errorf("expected 0 params for empty roles, got %d", len(result))
	}
}

func TestBuildRoleDependencies_NilRoleIgnored(t *testing.T) {
	roles := []*chefapi.RoleDetail{nil, nil}
	result := BuildRoleDependencies("org-1", roles)
	if len(result) != 0 {
		t.Errorf("expected 0 params for nil role entries, got %d", len(result))
	}
}

func TestBuildRoleDependencies_SingleRoleWithRecipesAndRoles(t *testing.T) {
	roles := []*chefapi.RoleDetail{
		{
			Name: "webserver",
			RunList: []string{
				"recipe[apache2]",
				"recipe[ntp::default]",
				"role[base]",
			},
		},
	}

	result := BuildRoleDependencies("org-1", roles)
	if len(result) != 3 {
		t.Fatalf("expected 3 dependency params, got %d", len(result))
	}

	// Sort by dependency name for deterministic checking.
	sort.Slice(result, func(i, j int) bool {
		if result[i].DependencyType != result[j].DependencyType {
			return result[i].DependencyType < result[j].DependencyType
		}
		return result[i].DependencyName < result[j].DependencyName
	})

	// Cookbooks should come first alphabetically after sorting by type.
	assertRoleDep(t, result[0], "org-1", "webserver", "cookbook", "apache2")
	assertRoleDep(t, result[1], "org-1", "webserver", "cookbook", "ntp")
	assertRoleDep(t, result[2], "org-1", "webserver", "role", "base")
}

func TestBuildRoleDependencies_EnvRunLists(t *testing.T) {
	roles := []*chefapi.RoleDetail{
		{
			Name:    "webserver",
			RunList: []string{"recipe[apache2]"},
			EnvRunLists: map[string][]string{
				"production":  {"recipe[apache2]", "recipe[monitoring]"},
				"development": {"recipe[apache2]", "role[dev-tools]"},
			},
		},
	}

	result := BuildRoleDependencies("org-1", roles)

	// Expect: apache2, monitoring, dev-tools (3 unique dependencies)
	// apache2 appears in default and both env run_lists but should be
	// deduplicated within the role.
	if len(result) != 3 {
		t.Fatalf("expected 3 unique dependency params (deduped), got %d", len(result))
	}

	depNames := make(map[string]string) // name → type
	for _, p := range result {
		depNames[p.DependencyName] = p.DependencyType
	}

	if depNames["apache2"] != "cookbook" {
		t.Error("expected apache2 as cookbook dependency")
	}
	if depNames["monitoring"] != "cookbook" {
		t.Error("expected monitoring as cookbook dependency")
	}
	if depNames["dev-tools"] != "role" {
		t.Error("expected dev-tools as role dependency")
	}
}

func TestBuildRoleDependencies_DeduplicationWithinRole(t *testing.T) {
	roles := []*chefapi.RoleDetail{
		{
			Name: "duplicated",
			RunList: []string{
				"recipe[ntp]",
				"recipe[ntp::default]",
				"recipe[ntp@1.0.0]",
			},
		},
	}

	result := BuildRoleDependencies("org-1", roles)
	if len(result) != 1 {
		t.Fatalf("expected 1 deduplicated dependency, got %d", len(result))
	}
	assertRoleDep(t, result[0], "org-1", "duplicated", "cookbook", "ntp")
}

func TestBuildRoleDependencies_MultipleRoles(t *testing.T) {
	roles := []*chefapi.RoleDetail{
		{
			Name:    "webserver",
			RunList: []string{"recipe[apache2]", "role[base]"},
		},
		{
			Name:    "database",
			RunList: []string{"recipe[postgresql]", "role[base]"},
		},
		{
			Name:    "base",
			RunList: []string{"recipe[ntp]", "recipe[users]"},
		},
	}

	result := BuildRoleDependencies("org-1", roles)

	// webserver: apache2 (cookbook), base (role)
	// database: postgresql (cookbook), base (role)
	// base: ntp (cookbook), users (cookbook)
	// Total: 6
	if len(result) != 6 {
		t.Fatalf("expected 6 dependency params, got %d", len(result))
	}

	// Check that all org IDs are correct.
	for _, p := range result {
		if p.OrganisationID != "org-1" {
			t.Errorf("expected OrganisationID=org-1, got %q", p.OrganisationID)
		}
	}

	// Check that we have the right role names.
	roleNames := make(map[string]int)
	for _, p := range result {
		roleNames[p.RoleName]++
	}
	if roleNames["webserver"] != 2 {
		t.Errorf("expected 2 deps for webserver, got %d", roleNames["webserver"])
	}
	if roleNames["database"] != 2 {
		t.Errorf("expected 2 deps for database, got %d", roleNames["database"])
	}
	if roleNames["base"] != 2 {
		t.Errorf("expected 2 deps for base, got %d", roleNames["base"])
	}
}

func TestBuildRoleDependencies_RoleWithEmptyRunList(t *testing.T) {
	roles := []*chefapi.RoleDetail{
		{
			Name:    "empty",
			RunList: nil,
		},
	}

	result := BuildRoleDependencies("org-1", roles)
	if len(result) != 0 {
		t.Errorf("expected 0 params for role with empty run_list, got %d", len(result))
	}
}

func TestBuildRoleDependencies_RoleWithOnlyInvalidEntries(t *testing.T) {
	roles := []*chefapi.RoleDetail{
		{
			Name:    "broken",
			RunList: []string{"garbage", "more_garbage", ""},
		},
	}

	result := BuildRoleDependencies("org-1", roles)
	if len(result) != 0 {
		t.Errorf("expected 0 params for role with invalid entries, got %d", len(result))
	}
}

func TestBuildRoleDependencies_EnvRunListOnlyNoDuplicateWithDefault(t *testing.T) {
	// A cookbook only in env_run_lists (not in default run_list) should still
	// be picked up, and one in both should be deduplicated.
	roles := []*chefapi.RoleDetail{
		{
			Name:    "mixed",
			RunList: []string{"recipe[common]"},
			EnvRunLists: map[string][]string{
				"staging": {"recipe[common]", "recipe[staging-only]"},
			},
		},
	}

	result := BuildRoleDependencies("org-1", roles)
	if len(result) != 2 {
		t.Fatalf("expected 2 dependency params, got %d", len(result))
	}

	names := make(map[string]bool)
	for _, p := range result {
		names[p.DependencyName] = true
	}
	if !names["common"] {
		t.Error("expected 'common' cookbook dependency")
	}
	if !names["staging-only"] {
		t.Error("expected 'staging-only' cookbook dependency")
	}
}

func TestBuildRoleDependencies_SameCookbookDifferentRecipesDeduplicated(t *testing.T) {
	roles := []*chefapi.RoleDetail{
		{
			Name: "multi-recipe",
			RunList: []string{
				"recipe[apache2::default]",
				"recipe[apache2::mod_ssl]",
				"recipe[apache2::mod_rewrite]",
			},
		},
	}

	result := BuildRoleDependencies("org-1", roles)
	// All three recipes are from apache2 — should produce one cookbook dep.
	if len(result) != 1 {
		t.Fatalf("expected 1 deduplicated cookbook dependency, got %d", len(result))
	}
	assertRoleDep(t, result[0], "org-1", "multi-recipe", "cookbook", "apache2")
}

func TestBuildRoleDependencies_RoleDependsOnItself(t *testing.T) {
	// Unusual but possible in Chef — a role referencing itself.
	roles := []*chefapi.RoleDetail{
		{
			Name:    "self-ref",
			RunList: []string{"role[self-ref]", "recipe[ntp]"},
		},
	}

	result := BuildRoleDependencies("org-1", roles)
	if len(result) != 2 {
		t.Fatalf("expected 2 dependency params, got %d", len(result))
	}

	depNames := make(map[string]string)
	for _, p := range result {
		depNames[p.DependencyName] = p.DependencyType
	}
	if depNames["self-ref"] != "role" {
		t.Error("expected self-ref as role dependency")
	}
	if depNames["ntp"] != "cookbook" {
		t.Error("expected ntp as cookbook dependency")
	}
}

func TestBuildRoleDependencies_MultipleEnvRunLists(t *testing.T) {
	roles := []*chefapi.RoleDetail{
		{
			Name:    "multi-env",
			RunList: []string{"recipe[base]"},
			EnvRunLists: map[string][]string{
				"production":  {"recipe[base]", "recipe[prod-monitoring]", "role[prod-lb]"},
				"staging":     {"recipe[base]", "recipe[staging-tools]"},
				"development": {"recipe[base]", "recipe[dev-tools]", "role[dev-infra]"},
			},
		},
	}

	result := BuildRoleDependencies("org-1", roles)

	// Unique deps: base, prod-monitoring, prod-lb, staging-tools, dev-tools, dev-infra = 6
	if len(result) != 6 {
		t.Fatalf("expected 6 unique dependency params, got %d", len(result))
	}

	depNames := make(map[string]string)
	for _, p := range result {
		depNames[p.DependencyName] = p.DependencyType
	}

	expectedCookbooks := []string{"base", "prod-monitoring", "staging-tools", "dev-tools"}
	for _, name := range expectedCookbooks {
		if depNames[name] != "cookbook" {
			t.Errorf("expected %q as cookbook dependency, got type %q", name, depNames[name])
		}
	}

	expectedRoles := []string{"prod-lb", "dev-infra"}
	for _, name := range expectedRoles {
		if depNames[name] != "role" {
			t.Errorf("expected %q as role dependency, got type %q", name, depNames[name])
		}
	}
}

func TestBuildRoleDependencies_LargeRoleSet(t *testing.T) {
	// Verify no issues with a moderately large role set.
	roles := make([]*chefapi.RoleDetail, 50)
	for i := range roles {
		roles[i] = &chefapi.RoleDetail{
			Name: "role-" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			RunList: []string{
				"recipe[cookbook-a]",
				"recipe[cookbook-b::special]",
				"role[shared]",
			},
		}
	}

	result := BuildRoleDependencies("org-1", roles)
	// Each role has 3 unique deps, 50 roles × 3 = 150.
	if len(result) != 150 {
		t.Errorf("expected 150 dependency params, got %d", len(result))
	}
}

func TestBuildRoleDependencies_MixedNilAndValidRoles(t *testing.T) {
	roles := []*chefapi.RoleDetail{
		nil,
		{Name: "valid", RunList: []string{"recipe[ntp]"}},
		nil,
		{Name: "also-valid", RunList: []string{"role[base]"}},
		nil,
	}

	result := BuildRoleDependencies("org-1", roles)
	if len(result) != 2 {
		t.Fatalf("expected 2 dependency params, got %d", len(result))
	}
}

func TestBuildRoleDependencies_EmptyOrganisationID(t *testing.T) {
	// The function itself does not validate organisation ID — that's the
	// datastore layer's responsibility. But it should propagate whatever
	// is passed.
	roles := []*chefapi.RoleDetail{
		{Name: "test", RunList: []string{"recipe[ntp]"}},
	}

	result := BuildRoleDependencies("", roles)
	if len(result) != 1 {
		t.Fatalf("expected 1 param, got %d", len(result))
	}
	if result[0].OrganisationID != "" {
		t.Errorf("expected empty OrganisationID, got %q", result[0].OrganisationID)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func assertRoleDep(t *testing.T, got datastore.InsertRoleDependencyParams, orgID, roleName, depType, depName string) {
	t.Helper()
	if got.OrganisationID != orgID {
		t.Errorf("expected OrganisationID=%q, got %q", orgID, got.OrganisationID)
	}
	if got.RoleName != roleName {
		t.Errorf("expected RoleName=%q, got %q", roleName, got.RoleName)
	}
	if got.DependencyType != depType {
		t.Errorf("expected DependencyType=%q, got %q", depType, got.DependencyType)
	}
	if got.DependencyName != depName {
		t.Errorf("expected DependencyName=%q, got %q", depName, got.DependencyName)
	}
}
