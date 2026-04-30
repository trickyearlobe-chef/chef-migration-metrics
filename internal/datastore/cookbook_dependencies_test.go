// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// parseCookbookDepsJSON — pure function tests
// ---------------------------------------------------------------------------

func TestParseCookbookDepsJSON_SimpleObject(t *testing.T) {
	input := []byte(`{"apache2": ">= 0.0.0", "apt": "~> 7.0"}`)
	got, err := parseCookbookDepsJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 deps, got %d: %v", len(got), got)
	}
	found := make(map[string]bool)
	for _, d := range got {
		found[d] = true
	}
	if !found["apache2"] || !found["apt"] {
		t.Errorf("expected apache2 and apt, got %v", got)
	}
}

func TestParseCookbookDepsJSON_EmptyObject(t *testing.T) {
	got, err := parseCookbookDepsJSON([]byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 deps, got %v", got)
	}
}

func TestParseCookbookDepsJSON_Nil(t *testing.T) {
	got, err := parseCookbookDepsJSON(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 deps for nil, got %v", got)
	}
}

func TestParseCookbookDepsJSON_NullLiteral(t *testing.T) {
	got, err := parseCookbookDepsJSON([]byte("null"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 deps for JSON null, got %v", got)
	}
}

func TestParseCookbookDepsJSON_NonObjectArray(t *testing.T) {
	// Arrays are not valid cookbook deps objects — should return empty, no error.
	got, err := parseCookbookDepsJSON([]byte(`["apache2"]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 deps for array input, got %v", got)
	}
}

func TestParseCookbookDepsJSON_MalformedJSON(t *testing.T) {
	// Malformed JSON — should return error.
	_, err := parseCookbookDepsJSON([]byte(`{bad`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// ---------------------------------------------------------------------------
// buildCookbookAdjMap — pure function tests
// ---------------------------------------------------------------------------

// buildCookbookAdjMap groups (name, deps) pairs into a map[string][]string.

func TestBuildCookbookAdjMap_Basic(t *testing.T) {
	rows := []cookbookDepRow{
		{cookbookName: "apache2", deps: []string{"apt", "compat_resource"}},
		{cookbookName: "apt", deps: []string{}},
	}
	adj := buildCookbookAdjMap(rows)
	if len(adj["apache2"]) != 2 {
		t.Errorf("expected 2 deps for apache2, got %v", adj["apache2"])
	}
	if len(adj["apt"]) != 0 {
		t.Errorf("expected 0 deps for apt, got %v", adj["apt"])
	}
}

func TestBuildCookbookAdjMap_Empty(t *testing.T) {
	adj := buildCookbookAdjMap(nil)
	if len(adj) != 0 {
		t.Errorf("expected empty map, got %v", adj)
	}
}

func TestBuildCookbookAdjMap_MultipleVersionsUnion(t *testing.T) {
	// Two active versions of the same cookbook with overlapping+distinct deps.
	// The adj map unions them (deduped).
	rows := []cookbookDepRow{
		{cookbookName: "apache2", deps: []string{"apt", "compat_resource"}},
		{cookbookName: "apache2", deps: []string{"apt", "build-essential"}},
	}
	adj := buildCookbookAdjMap(rows)
	if len(adj["apache2"]) != 3 {
		t.Errorf("expected 3 deduped deps for apache2, got %v", adj["apache2"])
	}
	found := make(map[string]bool)
	for _, d := range adj["apache2"] {
		found[d] = true
	}
	if !found["apt"] || !found["compat_resource"] || !found["build-essential"] {
		t.Errorf("unexpected dep set: %v", adj["apache2"])
	}
}

// ---------------------------------------------------------------------------
// buildCookbookChain — pure function tests
// ---------------------------------------------------------------------------

func TestBuildCookbookChain_Leaf(t *testing.T) {
	cbAdj := map[string][]string{"nginx": {}} // no deps
	visited := make(map[string]bool)
	cookbooks := make(map[string]bool)
	node := buildCookbookChain("nginx", cbAdj, visited, cookbooks, 0)
	if node.Name != "nginx" {
		t.Errorf("expected nginx, got %s", node.Name)
	}
	if node.Type != "cookbook" {
		t.Errorf("expected type cookbook, got %s", node.Type)
	}
	if len(node.Children) != 0 {
		t.Errorf("expected no children, got %v", node.Children)
	}
	if !cookbooks["nginx"] {
		t.Error("expected nginx in cookbooks set")
	}
}

func TestBuildCookbookChain_OneLevel(t *testing.T) {
	cbAdj := map[string][]string{
		"apache2": {"apt", "compat_resource"},
	}
	visited := make(map[string]bool)
	cookbooks := make(map[string]bool)
	node := buildCookbookChain("apache2", cbAdj, visited, cookbooks, 0)
	if len(node.Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(node.Children))
	}
	if !cookbooks["apache2"] || !cookbooks["apt"] || !cookbooks["compat_resource"] {
		t.Errorf("expected all cookbooks recorded: %v", cookbooks)
	}
}

func TestBuildCookbookChain_TransitiveTwoLevels(t *testing.T) {
	cbAdj := map[string][]string{
		"webserver": {"apache2"},
		"apache2":   {"apt"},
		"apt":       {},
	}
	visited := make(map[string]bool)
	cookbooks := make(map[string]bool)
	node := buildCookbookChain("webserver", cbAdj, visited, cookbooks, 0)
	if len(node.Children) != 1 {
		t.Fatalf("expected 1 child of webserver, got %d", len(node.Children))
	}
	apacheNode := node.Children[0]
	if apacheNode.Name != "apache2" {
		t.Errorf("expected apache2 child, got %s", apacheNode.Name)
	}
	if len(apacheNode.Children) != 1 || apacheNode.Children[0].Name != "apt" {
		t.Errorf("expected apt as child of apache2, got %v", apacheNode.Children)
	}
	if !cookbooks["webserver"] || !cookbooks["apache2"] || !cookbooks["apt"] {
		t.Errorf("expected all cookbooks recorded: %v", cookbooks)
	}
}

func TestBuildCookbookChain_CycleDetection(t *testing.T) {
	// A -> B -> A  (cycle)
	cbAdj := map[string][]string{
		"cookbookA": {"cookbookB"},
		"cookbookB": {"cookbookA"},
	}
	visited := make(map[string]bool)
	cookbooks := make(map[string]bool)
	// Should not infinite-loop or panic.
	node := buildCookbookChain("cookbookA", cbAdj, visited, cookbooks, 0)
	if node == nil {
		t.Fatal("expected non-nil node")
	}
	// cookbookB should be a child but cookbookA should not re-appear under it.
	if len(node.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(node.Children))
	}
	bNode := node.Children[0]
	for _, c := range bNode.Children {
		if c.Name == "cookbookA" {
			t.Errorf("cycle: cookbookA re-appeared as child of cookbookB")
		}
	}
}

func TestBuildCookbookChain_DepthLimit(t *testing.T) {
	// Linear chain exceeding maxCookbookDepth should stop expanding.
	cbAdj := make(map[string][]string)
	const depth = 60
	for i := 0; i < depth; i++ {
		name := fmt.Sprintf("cb%d", i)
		next := fmt.Sprintf("cb%d", i+1)
		cbAdj[name] = []string{next}
	}
	visited := make(map[string]bool)
	cookbooks := make(map[string]bool)
	// Should not panic, and should stop at maxCookbookDepth.
	node := buildCookbookChain("cb0", cbAdj, visited, cookbooks, 0)
	if node == nil {
		t.Fatal("expected non-nil node")
	}
}

func TestBuildCookbookChain_SharedSubgraph(t *testing.T) {
	// Two parents share the same dep.  Both should see it in their children
	// but it shouldn't be expanded twice (visited guard).
	cbAdj := map[string][]string{
		"parentA": {"shared"},
		"parentB": {"shared"},
		"shared":  {"leaf"},
		"leaf":    {},
	}
	visitedA := make(map[string]bool)
	cookbooksA := make(map[string]bool)
	nodeA := buildCookbookChain("parentA", cbAdj, visitedA, cookbooksA, 0)
	if len(nodeA.Children) != 1 {
		t.Fatalf("parentA expected 1 child, got %d", len(nodeA.Children))
	}
	// "shared" is a child of parentA and has "leaf" as its child.
	sharedNode := nodeA.Children[0]
	if sharedNode.Name != "shared" {
		t.Errorf("expected shared, got %s", sharedNode.Name)
	}
	if len(sharedNode.Children) != 1 || sharedNode.Children[0].Name != "leaf" {
		t.Errorf("expected leaf as child of shared, got %v", sharedNode.Children)
	}
}
