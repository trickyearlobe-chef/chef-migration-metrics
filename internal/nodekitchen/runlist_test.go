// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package nodekitchen

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

type mockRoleFetcher struct {
	roles map[string]*chefapi.RoleDetail
}

func (m *mockRoleFetcher) GetRole(_ context.Context, name string) (*chefapi.RoleDetail, error) {
	r, ok := m.roles[name]
	if !ok {
		return nil, fmt.Errorf("role %q not found", name)
	}
	return r, nil
}

type mockDepResolver struct {
	// key: "org/cookbook/version"
	deps map[string]map[string]string
}

func (m *mockDepResolver) GetCookbookDependencies(_ context.Context, orgName, cookbookName, version string) (map[string]string, error) {
	key := orgName + "/" + cookbookName + "/" + version
	d, ok := m.deps[key]
	if !ok {
		return nil, nil
	}
	return d, nil
}

// ---------------------------------------------------------------------------
// ParseRunListEntry
// ---------------------------------------------------------------------------

func TestParseRunListEntry_Role(t *testing.T) {
	e, err := ParseRunListEntry("role[webserver]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Type != "role" || e.Name != "webserver" || e.RecipeName != "" {
		t.Errorf("got %+v", e)
	}
}

func TestParseRunListEntry_Recipe(t *testing.T) {
	e, err := ParseRunListEntry("recipe[nginx]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Type != "recipe" || e.Name != "nginx" || e.RecipeName != "default" {
		t.Errorf("got %+v", e)
	}
}

func TestParseRunListEntry_RecipeWithName(t *testing.T) {
	e, err := ParseRunListEntry("recipe[nginx::ssl]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Type != "recipe" || e.Name != "nginx" || e.RecipeName != "ssl" {
		t.Errorf("got %+v", e)
	}
}

func TestParseRunListEntry_Bare(t *testing.T) {
	e, err := ParseRunListEntry("nginx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Type != "recipe" || e.Name != "nginx" || e.RecipeName != "default" {
		t.Errorf("got %+v", e)
	}
}

func TestParseRunListEntry_BareWithRecipe(t *testing.T) {
	e, err := ParseRunListEntry("nginx::ssl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Type != "recipe" || e.Name != "nginx" || e.RecipeName != "ssl" {
		t.Errorf("got %+v", e)
	}
}

func TestParseRunListEntry_Empty(t *testing.T) {
	_, err := ParseRunListEntry("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

func TestParseRunListEntry_MalformedBrackets(t *testing.T) {
	for _, s := range []string{"role[", "recipe]foo", "role[]", "recipe[foo"} {
		_, err := ParseRunListEntry(s)
		if err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestParseRunListEntry_UnknownType(t *testing.T) {
	_, err := ParseRunListEntry("environment[prod]")
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

// ---------------------------------------------------------------------------
// ParseRunList
// ---------------------------------------------------------------------------

func TestParseRunList(t *testing.T) {
	raw := json.RawMessage(`["role[base]","recipe[nginx::ssl]","apt"]`)
	entries, err := ParseRunList(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Type != "role" || entries[0].Name != "base" {
		t.Errorf("entry 0: %+v", entries[0])
	}
	if entries[1].Type != "recipe" || entries[1].Name != "nginx" || entries[1].RecipeName != "ssl" {
		t.Errorf("entry 1: %+v", entries[1])
	}
	if entries[2].Type != "recipe" || entries[2].Name != "apt" || entries[2].RecipeName != "default" {
		t.Errorf("entry 2: %+v", entries[2])
	}
}

func TestParseRunList_Null(t *testing.T) {
	entries, err := ParseRunList(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty, got %d", len(entries))
	}
}

func TestParseRunList_EmptyArray(t *testing.T) {
	raw := json.RawMessage(`[]`)
	entries, err := ParseRunList(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty, got %d", len(entries))
	}
}

func TestParseRunList_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not json`)
	_, err := ParseRunList(raw)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// ExpandRunList
// ---------------------------------------------------------------------------

func TestExpandRunList_NoRoles(t *testing.T) {
	entries := []RunListEntry{
		{Type: "recipe", Name: "nginx", RecipeName: "default"},
		{Type: "recipe", Name: "apt", RecipeName: "default"},
	}
	result, err := ExpandRunList(context.Background(), entries, &mockRoleFetcher{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestExpandRunList_SingleRole(t *testing.T) {
	fetcher := &mockRoleFetcher{
		roles: map[string]*chefapi.RoleDetail{
			"webserver": {
				Name:    "webserver",
				RunList: []string{"recipe[nginx]", "recipe[logrotate]"},
			},
		},
	}
	entries := []RunListEntry{
		{Type: "role", Name: "webserver"},
	}
	result, err := ExpandRunList(context.Background(), entries, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0].Name != "nginx" || result[1].Name != "logrotate" {
		t.Errorf("got %+v", result)
	}
}

func TestExpandRunList_NestedRoles(t *testing.T) {
	fetcher := &mockRoleFetcher{
		roles: map[string]*chefapi.RoleDetail{
			"base": {
				Name:    "base",
				RunList: []string{"recipe[apt]", "role[monitoring]"},
			},
			"monitoring": {
				Name:    "monitoring",
				RunList: []string{"recipe[datadog]"},
			},
		},
	}
	entries := []RunListEntry{
		{Type: "role", Name: "base"},
		{Type: "recipe", Name: "nginx", RecipeName: "default"},
	}
	result, err := ExpandRunList(context.Background(), entries, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
	if result[0].Name != "apt" || result[1].Name != "datadog" || result[2].Name != "nginx" {
		t.Errorf("got %+v", result)
	}
}

func TestExpandRunList_CycleDetection(t *testing.T) {
	fetcher := &mockRoleFetcher{
		roles: map[string]*chefapi.RoleDetail{
			"a": {Name: "a", RunList: []string{"role[b]"}},
			"b": {Name: "b", RunList: []string{"role[a]"}},
		},
	}
	entries := []RunListEntry{{Type: "role", Name: "a"}}
	_, err := ExpandRunList(context.Background(), entries, fetcher)
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestExpandRunList_MissingRole(t *testing.T) {
	fetcher := &mockRoleFetcher{roles: map[string]*chefapi.RoleDetail{}}
	entries := []RunListEntry{{Type: "role", Name: "nonexistent"}}
	_, err := ExpandRunList(context.Background(), entries, fetcher)
	if err == nil {
		t.Fatal("expected error for missing role")
	}
}

func TestExpandRunList_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fetcher := &mockRoleFetcher{
		roles: map[string]*chefapi.RoleDetail{
			"x": {Name: "x", RunList: []string{"recipe[foo]"}},
		},
	}
	entries := []RunListEntry{{Type: "role", Name: "x"}}
	_, err := ExpandRunList(ctx, entries, fetcher)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// ---------------------------------------------------------------------------
// ParseNodeCookbooks
// ---------------------------------------------------------------------------

func TestParseNodeCookbooks(t *testing.T) {
	raw := json.RawMessage(`{"nginx":{"version":"1.2.3"},"apt":{"version":"7.0.0"}}`)
	m, err := ParseNodeCookbooks(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["nginx"] != "1.2.3" || m["apt"] != "7.0.0" {
		t.Errorf("got %v", m)
	}
}

func TestParseNodeCookbooks_Null(t *testing.T) {
	m, err := ParseNodeCookbooks(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("expected empty, got %v", m)
	}
}

func TestParseNodeCookbooks_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{bad}`)
	_, err := ParseNodeCookbooks(raw)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// ResolveCookbookSet
// ---------------------------------------------------------------------------

func TestResolveCookbookSet_Simple(t *testing.T) {
	recipes := []RunListEntry{
		{Type: "recipe", Name: "nginx", RecipeName: "default"},
	}
	nodeCookbooks := map[string]string{"nginx": "1.0.0", "apt": "2.0.0"}
	resolver := &mockDepResolver{deps: map[string]map[string]string{
		"myorg/nginx/1.0.0": {"apt": ">= 1.0"},
	}}
	result, err := ResolveCookbookSet(context.Background(), recipes, nodeCookbooks, resolver, "myorg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["nginx"] != "1.0.0" {
		t.Errorf("missing nginx: %v", result)
	}
	if result["apt"] != "2.0.0" {
		t.Errorf("missing apt: %v", result)
	}
}

func TestResolveCookbookSet_TransitiveDeps(t *testing.T) {
	recipes := []RunListEntry{
		{Type: "recipe", Name: "nginx", RecipeName: "default"},
	}
	nodeCookbooks := map[string]string{"nginx": "1.0.0", "apt": "2.0.0", "compat_resource": "1.0.0"}
	resolver := &mockDepResolver{deps: map[string]map[string]string{
		"myorg/nginx/1.0.0": {"apt": ">= 1.0"},
		"myorg/apt/2.0.0":   {"compat_resource": ">= 0.1"},
	}}
	result, err := ResolveCookbookSet(context.Background(), recipes, nodeCookbooks, resolver, "myorg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 cookbooks, got %v", result)
	}
	if result["compat_resource"] != "1.0.0" {
		t.Errorf("missing transitive dep: %v", result)
	}
}

func TestResolveCookbookSet_MissingCookbookVersion(t *testing.T) {
	recipes := []RunListEntry{
		{Type: "recipe", Name: "missing_cb", RecipeName: "default"},
	}
	nodeCookbooks := map[string]string{}
	resolver := &mockDepResolver{}
	_, err := ResolveCookbookSet(context.Background(), recipes, nodeCookbooks, resolver, "myorg")
	if err == nil {
		t.Fatal("expected error for missing cookbook version")
	}
}
