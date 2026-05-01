// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"testing"
)

// ---------------------------------------------------------------------------
// buildRoleChain — pure function tests with cookbook→cookbook expansion
// ---------------------------------------------------------------------------

func TestBuildRoleChain_RoleWithNoCbAdj(t *testing.T) {
	// Pre-existing: role with a direct cookbook dep, no cbAdj.
	adj := map[string][]RoleDependency{
		"webserver": {
			{RoleName: "webserver", DependencyType: "cookbook", DependencyName: "nginx"},
		},
	}
	visited := make(map[string]bool)
	cookbooks := make(map[string]bool)
	node := buildRoleChain("webserver", adj, visited, cookbooks, nil)
	if node.Name != "webserver" {
		t.Fatalf("expected webserver, got %s", node.Name)
	}
	if len(node.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(node.Children))
	}
	if node.Children[0].Name != "nginx" {
		t.Errorf("expected nginx child, got %s", node.Children[0].Name)
	}
	if !cookbooks["nginx"] {
		t.Error("expected nginx in cookbooks set")
	}
}

func TestBuildRoleChain_CookbookTransitiveDepsExpanded(t *testing.T) {
	// role:app → cookbook:nginx; nginx → apt (in cbAdj).
	adj := map[string][]RoleDependency{
		"app": {
			{RoleName: "app", DependencyType: "cookbook", DependencyName: "nginx"},
		},
	}
	cbAdj := map[string][]string{
		"nginx": {"apt"},
	}
	visited := make(map[string]bool)
	cookbooks := make(map[string]bool)
	node := buildRoleChain("app", adj, visited, cookbooks, cbAdj)

	if len(node.Children) != 1 {
		t.Fatalf("expected 1 child of app, got %d", len(node.Children))
	}
	nginxNode := node.Children[0]
	if nginxNode.Name != "nginx" {
		t.Fatalf("expected nginx child, got %s", nginxNode.Name)
	}
	// nginx should have apt as a child via cbAdj expansion.
	if len(nginxNode.Children) != 1 {
		t.Fatalf("expected 1 child of nginx, got %d: %v", len(nginxNode.Children), nginxNode.Children)
	}
	if nginxNode.Children[0].Name != "apt" {
		t.Errorf("expected apt as child of nginx, got %s", nginxNode.Children[0].Name)
	}
	// All cookbooks should be in the set.
	if !cookbooks["nginx"] || !cookbooks["apt"] {
		t.Errorf("expected nginx and apt in cookbooks set, got %v", cookbooks)
	}
}

func TestBuildRoleChain_NestedRoleWithCookbookDeps(t *testing.T) {
	// role:web → role:base → cookbook:apt → compat_resource
	adj := map[string][]RoleDependency{
		"web": {
			{RoleName: "web", DependencyType: "role", DependencyName: "base"},
		},
		"base": {
			{RoleName: "base", DependencyType: "cookbook", DependencyName: "apt"},
		},
	}
	cbAdj := map[string][]string{
		"apt": {"compat_resource"},
	}
	visited := make(map[string]bool)
	cookbooks := make(map[string]bool)
	node := buildRoleChain("web", adj, visited, cookbooks, cbAdj)

	// web → base (role child)
	if len(node.Children) != 1 {
		t.Fatalf("expected 1 child of web, got %d", len(node.Children))
	}
	baseNode := node.Children[0]
	if baseNode.Type != "role" || baseNode.Name != "base" {
		t.Fatalf("expected role:base, got %s:%s", baseNode.Type, baseNode.Name)
	}
	// base → apt (cookbook child)
	if len(baseNode.Children) != 1 {
		t.Fatalf("expected 1 child of base, got %d", len(baseNode.Children))
	}
	aptNode := baseNode.Children[0]
	if aptNode.Type != "cookbook" || aptNode.Name != "apt" {
		t.Fatalf("expected cookbook:apt, got %s:%s", aptNode.Type, aptNode.Name)
	}
	// apt → compat_resource (transitive via cbAdj)
	if len(aptNode.Children) != 1 || aptNode.Children[0].Name != "compat_resource" {
		t.Errorf("expected compat_resource as child of apt, got %v", aptNode.Children)
	}
	if !cookbooks["apt"] || !cookbooks["compat_resource"] {
		t.Errorf("expected apt+compat_resource in cookbook set, got %v", cookbooks)
	}
}
