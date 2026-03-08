// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Phase 1: extractNodeTuples
// ---------------------------------------------------------------------------

func TestExtractNodeTuples_Empty(t *testing.T) {
	node := &NodeRecord{
		NodeName:         "empty-node",
		CookbookVersions: nil,
	}
	tuples := extractNodeTuples(node)
	if len(tuples) != 0 {
		t.Errorf("expected 0 tuples for node with no cookbooks, got %d", len(tuples))
	}
}

func TestExtractNodeTuples_EmptyMap(t *testing.T) {
	node := &NodeRecord{
		NodeName:         "empty-map-node",
		CookbookVersions: map[string]string{},
	}
	tuples := extractNodeTuples(node)
	if len(tuples) != 0 {
		t.Errorf("expected 0 tuples for node with empty cookbook map, got %d", len(tuples))
	}
}

func TestExtractNodeTuples_SingleCookbook(t *testing.T) {
	node := &NodeRecord{
		NodeName:         "web1",
		Platform:         "ubuntu",
		PlatformVersion:  "22.04",
		PlatformFamily:   "debian",
		Roles:            []string{"webserver", "base"},
		PolicyName:       "",
		PolicyGroup:      "",
		CookbookVersions: map[string]string{"apache2": "5.0.0"},
	}

	tuples := extractNodeTuples(node)
	if len(tuples) != 1 {
		t.Fatalf("expected 1 tuple, got %d", len(tuples))
	}

	tu := tuples[0]
	if tu.CookbookName != "apache2" {
		t.Errorf("expected CookbookName=apache2, got %q", tu.CookbookName)
	}
	if tu.CookbookVersion != "5.0.0" {
		t.Errorf("expected CookbookVersion=5.0.0, got %q", tu.CookbookVersion)
	}
	if tu.NodeName != "web1" {
		t.Errorf("expected NodeName=web1, got %q", tu.NodeName)
	}
	if tu.Platform != "ubuntu" {
		t.Errorf("expected Platform=ubuntu, got %q", tu.Platform)
	}
	if tu.PlatformVersion != "22.04" {
		t.Errorf("expected PlatformVersion=22.04, got %q", tu.PlatformVersion)
	}
	if tu.PlatformFamily != "debian" {
		t.Errorf("expected PlatformFamily=debian, got %q", tu.PlatformFamily)
	}
	if len(tu.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(tu.Roles))
	}
}

func TestExtractNodeTuples_MultipleCookbooks(t *testing.T) {
	node := &NodeRecord{
		NodeName: "multi",
		CookbookVersions: map[string]string{
			"apache2": "5.0.0",
			"ntp":     "3.1.0",
			"users":   "1.0.0",
		},
	}

	tuples := extractNodeTuples(node)
	if len(tuples) != 3 {
		t.Fatalf("expected 3 tuples, got %d", len(tuples))
	}

	names := make(map[string]bool)
	for _, tu := range tuples {
		names[tu.CookbookName] = true
	}
	for _, expected := range []string{"apache2", "ntp", "users"} {
		if !names[expected] {
			t.Errorf("expected cookbook %q in tuples", expected)
		}
	}
}

func TestExtractNodeTuples_PolicyfileNode(t *testing.T) {
	node := &NodeRecord{
		NodeName:         "policy-node",
		PolicyName:       "webserver-policy",
		PolicyGroup:      "production",
		CookbookVersions: map[string]string{"nginx": "2.0.0"},
	}

	tuples := extractNodeTuples(node)
	if len(tuples) != 1 {
		t.Fatalf("expected 1 tuple, got %d", len(tuples))
	}
	if tuples[0].PolicyName != "webserver-policy" {
		t.Errorf("expected PolicyName=webserver-policy, got %q", tuples[0].PolicyName)
	}
	if tuples[0].PolicyGroup != "production" {
		t.Errorf("expected PolicyGroup=production, got %q", tuples[0].PolicyGroup)
	}
}

// ---------------------------------------------------------------------------
// Phase 1: extractTuples (parallel path)
// ---------------------------------------------------------------------------

func TestExtractTuples_EmptyNodes(t *testing.T) {
	a := &Analyser{concurrency: 4}
	tuples := a.extractTuples(context.TODO(), nil)
	if len(tuples) != 0 {
		t.Errorf("expected 0 tuples for nil nodes, got %d", len(tuples))
	}

	tuples = a.extractTuples(context.TODO(), []NodeRecord{})
	if len(tuples) != 0 {
		t.Errorf("expected 0 tuples for empty nodes, got %d", len(tuples))
	}
}

func TestExtractTuples_ParallelExtraction(t *testing.T) {
	nodes := []NodeRecord{
		{
			NodeName:         "node1",
			CookbookVersions: map[string]string{"a": "1.0", "b": "2.0"},
		},
		{
			NodeName:         "node2",
			CookbookVersions: map[string]string{"b": "2.0", "c": "3.0"},
		},
		{
			NodeName:         "node3",
			CookbookVersions: nil, // No cookbooks
		},
	}

	a := &Analyser{concurrency: 2}
	tuples := a.extractTuples(context.TODO(), nodes)

	// node1: 2 tuples, node2: 2 tuples, node3: 0 tuples = 4 total
	if len(tuples) != 4 {
		t.Fatalf("expected 4 tuples, got %d", len(tuples))
	}
}

func TestExtractTuples_ConcurrencyOne(t *testing.T) {
	nodes := []NodeRecord{
		{NodeName: "n1", CookbookVersions: map[string]string{"x": "1.0"}},
		{NodeName: "n2", CookbookVersions: map[string]string{"y": "2.0"}},
	}

	a := &Analyser{concurrency: 1}
	tuples := a.extractTuples(context.TODO(), nodes)
	if len(tuples) != 2 {
		t.Fatalf("expected 2 tuples, got %d", len(tuples))
	}
}

func TestExtractTuples_LargeBatch(t *testing.T) {
	nodes := make([]NodeRecord, 100)
	for i := range nodes {
		nodes[i] = NodeRecord{
			NodeName:         "node-" + string(rune('a'+i%26)),
			CookbookVersions: map[string]string{"cb": "1.0"},
		}
	}

	a := &Analyser{concurrency: 8}
	tuples := a.extractTuples(context.TODO(), nodes)
	if len(tuples) != 100 {
		t.Errorf("expected 100 tuples, got %d", len(tuples))
	}
}

// ---------------------------------------------------------------------------
// Phase 2: aggregateTuples
// ---------------------------------------------------------------------------

func TestAggregateTuples_Empty(t *testing.T) {
	agg := aggregateTuples(nil)
	if len(agg) != 0 {
		t.Errorf("expected empty aggregation for nil input, got %d entries", len(agg))
	}

	agg = aggregateTuples([]extractedTuple{})
	if len(agg) != 0 {
		t.Errorf("expected empty aggregation for empty input, got %d entries", len(agg))
	}
}

func TestAggregateTuples_SingleTuple(t *testing.T) {
	tuples := []extractedTuple{
		{
			CookbookName:    "apache2",
			CookbookVersion: "5.0.0",
			NodeName:        "web1",
			Platform:        "ubuntu",
			PlatformVersion: "22.04",
			PlatformFamily:  "debian",
			Roles:           []string{"webserver"},
		},
	}

	agg := aggregateTuples(tuples)
	if len(agg) != 1 {
		t.Fatalf("expected 1 aggregated entry, got %d", len(agg))
	}

	key := cookbookVersionKey{Name: "apache2", Version: "5.0.0"}
	usage, ok := agg[key]
	if !ok {
		t.Fatal("expected entry for apache2/5.0.0")
	}
	if usage.NodeCount != 1 {
		t.Errorf("expected NodeCount=1, got %d", usage.NodeCount)
	}
	if !usage.NodeNames["web1"] {
		t.Error("expected web1 in NodeNames")
	}
	if !usage.Roles["webserver"] {
		t.Error("expected webserver in Roles")
	}
	if usage.PlatformCounts["ubuntu/22.04"] != 1 {
		t.Errorf("expected platform count 1 for ubuntu/22.04, got %d", usage.PlatformCounts["ubuntu/22.04"])
	}
	if usage.PlatformFamilyCounts["debian"] != 1 {
		t.Errorf("expected platform family count 1 for debian, got %d", usage.PlatformFamilyCounts["debian"])
	}
}

func TestAggregateTuples_SameCookbookMultipleNodes(t *testing.T) {
	tuples := []extractedTuple{
		{
			CookbookName: "ntp", CookbookVersion: "3.0.0",
			NodeName: "web1", Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian",
			Roles: []string{"webserver", "base"},
		},
		{
			CookbookName: "ntp", CookbookVersion: "3.0.0",
			NodeName: "db1", Platform: "centos", PlatformVersion: "7", PlatformFamily: "rhel",
			Roles: []string{"database", "base"},
		},
		{
			CookbookName: "ntp", CookbookVersion: "3.0.0",
			NodeName: "web2", Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian",
			Roles: []string{"webserver"},
		},
	}

	agg := aggregateTuples(tuples)
	key := cookbookVersionKey{Name: "ntp", Version: "3.0.0"}
	usage := agg[key]

	if usage.NodeCount != 3 {
		t.Errorf("expected NodeCount=3, got %d", usage.NodeCount)
	}

	expectedNodes := []string{"web1", "db1", "web2"}
	for _, n := range expectedNodes {
		if !usage.NodeNames[n] {
			t.Errorf("expected %q in NodeNames", n)
		}
	}

	// Roles: webserver, base, database (3 unique)
	if len(usage.Roles) != 3 {
		t.Errorf("expected 3 unique roles, got %d", len(usage.Roles))
	}
	for _, r := range []string{"webserver", "base", "database"} {
		if !usage.Roles[r] {
			t.Errorf("expected role %q", r)
		}
	}

	// Platform counts: ubuntu/22.04 = 2, centos/7 = 1
	if usage.PlatformCounts["ubuntu/22.04"] != 2 {
		t.Errorf("expected ubuntu/22.04=2, got %d", usage.PlatformCounts["ubuntu/22.04"])
	}
	if usage.PlatformCounts["centos/7"] != 1 {
		t.Errorf("expected centos/7=1, got %d", usage.PlatformCounts["centos/7"])
	}

	// Platform family counts: debian = 2, rhel = 1
	if usage.PlatformFamilyCounts["debian"] != 2 {
		t.Errorf("expected debian=2, got %d", usage.PlatformFamilyCounts["debian"])
	}
	if usage.PlatformFamilyCounts["rhel"] != 1 {
		t.Errorf("expected rhel=1, got %d", usage.PlatformFamilyCounts["rhel"])
	}
}

func TestAggregateTuples_DuplicateNodeDeduplicated(t *testing.T) {
	// Same node appears twice for the same cookbook version — should count once.
	tuples := []extractedTuple{
		{CookbookName: "ntp", CookbookVersion: "3.0.0", NodeName: "web1", PlatformFamily: "debian"},
		{CookbookName: "ntp", CookbookVersion: "3.0.0", NodeName: "web1", PlatformFamily: "debian"},
	}

	agg := aggregateTuples(tuples)
	key := cookbookVersionKey{Name: "ntp", Version: "3.0.0"}
	usage := agg[key]

	if usage.NodeCount != 1 {
		t.Errorf("expected NodeCount=1 (deduplicated), got %d", usage.NodeCount)
	}
	// Platform family count should still be 2 since it counts occurrences, not distinct nodes.
	if usage.PlatformFamilyCounts["debian"] != 2 {
		t.Errorf("expected platform family debian=2, got %d", usage.PlatformFamilyCounts["debian"])
	}
}

func TestAggregateTuples_DifferentVersionsSameBook(t *testing.T) {
	tuples := []extractedTuple{
		{CookbookName: "apache2", CookbookVersion: "5.0.0", NodeName: "web1"},
		{CookbookName: "apache2", CookbookVersion: "4.0.0", NodeName: "web2"},
		{CookbookName: "apache2", CookbookVersion: "5.0.0", NodeName: "web3"},
	}

	agg := aggregateTuples(tuples)

	if len(agg) != 2 {
		t.Fatalf("expected 2 aggregated entries (one per version), got %d", len(agg))
	}

	key50 := cookbookVersionKey{Name: "apache2", Version: "5.0.0"}
	key40 := cookbookVersionKey{Name: "apache2", Version: "4.0.0"}

	if agg[key50].NodeCount != 2 {
		t.Errorf("expected NodeCount=2 for 5.0.0, got %d", agg[key50].NodeCount)
	}
	if agg[key40].NodeCount != 1 {
		t.Errorf("expected NodeCount=1 for 4.0.0, got %d", agg[key40].NodeCount)
	}
}

func TestAggregateTuples_PolicyfileReferences(t *testing.T) {
	tuples := []extractedTuple{
		{
			CookbookName: "nginx", CookbookVersion: "2.0.0",
			NodeName: "pol1", PolicyName: "webserver-policy", PolicyGroup: "production",
		},
		{
			CookbookName: "nginx", CookbookVersion: "2.0.0",
			NodeName: "pol2", PolicyName: "webserver-policy", PolicyGroup: "staging",
		},
		{
			CookbookName: "nginx", CookbookVersion: "2.0.0",
			NodeName: "pol3", PolicyName: "api-policy", PolicyGroup: "production",
		},
	}

	agg := aggregateTuples(tuples)
	key := cookbookVersionKey{Name: "nginx", Version: "2.0.0"}
	usage := agg[key]

	if len(usage.PolicyNames) != 2 {
		t.Errorf("expected 2 unique policy names, got %d", len(usage.PolicyNames))
	}
	if !usage.PolicyNames["webserver-policy"] || !usage.PolicyNames["api-policy"] {
		t.Error("expected webserver-policy and api-policy in PolicyNames")
	}

	if len(usage.PolicyGroups) != 2 {
		t.Errorf("expected 2 unique policy groups, got %d", len(usage.PolicyGroups))
	}
	if !usage.PolicyGroups["production"] || !usage.PolicyGroups["staging"] {
		t.Error("expected production and staging in PolicyGroups")
	}
}

func TestAggregateTuples_EmptyPlatformSkipped(t *testing.T) {
	tuples := []extractedTuple{
		{CookbookName: "ntp", CookbookVersion: "3.0.0", NodeName: "n1", Platform: "", PlatformFamily: ""},
	}

	agg := aggregateTuples(tuples)
	key := cookbookVersionKey{Name: "ntp", Version: "3.0.0"}
	usage := agg[key]

	if len(usage.PlatformCounts) != 0 {
		t.Errorf("expected 0 platform counts for empty platform, got %d", len(usage.PlatformCounts))
	}
	if len(usage.PlatformFamilyCounts) != 0 {
		t.Errorf("expected 0 platform family counts for empty family, got %d", len(usage.PlatformFamilyCounts))
	}
}

func TestAggregateTuples_PlatformWithoutVersion(t *testing.T) {
	tuples := []extractedTuple{
		{CookbookName: "ntp", CookbookVersion: "1.0", NodeName: "n1", Platform: "linux", PlatformVersion: ""},
	}

	agg := aggregateTuples(tuples)
	key := cookbookVersionKey{Name: "ntp", Version: "1.0"}
	usage := agg[key]

	// When platform is set but version is empty, key should be just the platform name.
	if usage.PlatformCounts["linux"] != 1 {
		t.Errorf("expected platform count 1 for 'linux', got %d", usage.PlatformCounts["linux"])
	}
}

func TestAggregateTuples_EmptyPolicyFieldsIgnored(t *testing.T) {
	tuples := []extractedTuple{
		{CookbookName: "ntp", CookbookVersion: "1.0", NodeName: "n1", PolicyName: "", PolicyGroup: ""},
	}

	agg := aggregateTuples(tuples)
	key := cookbookVersionKey{Name: "ntp", Version: "1.0"}
	usage := agg[key]

	if len(usage.PolicyNames) != 0 {
		t.Errorf("expected 0 policy names for empty policy, got %d", len(usage.PolicyNames))
	}
	if len(usage.PolicyGroups) != 0 {
		t.Errorf("expected 0 policy groups for empty policy, got %d", len(usage.PolicyGroups))
	}
}

func TestAggregateTuples_MultipleRolesFromSameNode(t *testing.T) {
	tuples := []extractedTuple{
		{
			CookbookName: "ntp", CookbookVersion: "3.0.0",
			NodeName: "web1", Roles: []string{"webserver", "base", "monitoring"},
		},
	}

	agg := aggregateTuples(tuples)
	key := cookbookVersionKey{Name: "ntp", Version: "3.0.0"}
	usage := agg[key]

	if len(usage.Roles) != 3 {
		t.Errorf("expected 3 roles, got %d", len(usage.Roles))
	}
}

func TestAggregateTuples_NilRolesSlice(t *testing.T) {
	tuples := []extractedTuple{
		{CookbookName: "ntp", CookbookVersion: "3.0.0", NodeName: "web1", Roles: nil},
	}

	agg := aggregateTuples(tuples)
	key := cookbookVersionKey{Name: "ntp", Version: "3.0.0"}
	usage := agg[key]

	if len(usage.Roles) != 0 {
		t.Errorf("expected 0 roles for nil Roles slice, got %d", len(usage.Roles))
	}
}

// ---------------------------------------------------------------------------
// Phase 3: buildInventorySet / buildActiveSet
// ---------------------------------------------------------------------------

func TestBuildInventorySet_Empty(t *testing.T) {
	set := buildInventorySet(nil)
	if len(set) != 0 {
		t.Errorf("expected empty set for nil input, got %d", len(set))
	}
}

func TestBuildInventorySet_Populated(t *testing.T) {
	inv := []CookbookInventoryEntry{
		{Name: "apache2", Version: "5.0.0"},
		{Name: "apache2", Version: "4.0.0"},
		{Name: "ntp", Version: "3.0.0"},
	}

	set := buildInventorySet(inv)
	if len(set) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(set))
	}

	for _, e := range inv {
		key := cookbookVersionKey(e)
		if !set[key] {
			t.Errorf("expected %s/%s in inventory set", e.Name, e.Version)
		}
	}
}

func TestBuildInventorySet_Deduplicates(t *testing.T) {
	inv := []CookbookInventoryEntry{
		{Name: "ntp", Version: "3.0.0"},
		{Name: "ntp", Version: "3.0.0"},
	}

	set := buildInventorySet(inv)
	if len(set) != 1 {
		t.Errorf("expected 1 deduplicated entry, got %d", len(set))
	}
}

func TestBuildActiveSet_Empty(t *testing.T) {
	set := buildActiveSet(nil)
	if len(set) != 0 {
		t.Errorf("expected empty set for nil input, got %d", len(set))
	}
}

func TestBuildActiveSet_OnlyCountsPositiveNodes(t *testing.T) {
	agg := map[cookbookVersionKey]*aggregatedUsage{
		{Name: "active", Version: "1.0"}:      {NodeCount: 5},
		{Name: "zero", Version: "1.0"}:        {NodeCount: 0},
		{Name: "also-active", Version: "2.0"}: {NodeCount: 1},
	}

	set := buildActiveSet(agg)
	if len(set) != 2 {
		t.Fatalf("expected 2 active entries, got %d", len(set))
	}

	if !set[cookbookVersionKey{Name: "active", Version: "1.0"}] {
		t.Error("expected active/1.0 in active set")
	}
	if !set[cookbookVersionKey{Name: "also-active", Version: "2.0"}] {
		t.Error("expected also-active/2.0 in active set")
	}
	if set[cookbookVersionKey{Name: "zero", Version: "1.0"}] {
		t.Error("did not expect zero/1.0 in active set")
	}
}

// ---------------------------------------------------------------------------
// Active / unused counting logic
// ---------------------------------------------------------------------------

func TestActiveUnusedCounting(t *testing.T) {
	// Simulate Phase 3 counting logic used in RunUsageAnalysis.
	inventory := []CookbookInventoryEntry{
		{Name: "apache2", Version: "5.0.0"},
		{Name: "apache2", Version: "4.0.0"},
		{Name: "ntp", Version: "3.0.0"},
		{Name: "unused-cb", Version: "1.0.0"},
	}

	aggregated := map[cookbookVersionKey]*aggregatedUsage{
		{Name: "apache2", Version: "5.0.0"}: {NodeCount: 10},
		{Name: "ntp", Version: "3.0.0"}:     {NodeCount: 5},
	}

	inventorySet := buildInventorySet(inventory)
	activeSet := buildActiveSet(aggregated)

	activeCount := 0
	unusedCount := 0
	for key := range inventorySet {
		if activeSet[key] {
			activeCount++
		} else {
			unusedCount++
		}
	}

	if activeCount != 2 {
		t.Errorf("expected 2 active cookbooks, got %d", activeCount)
	}
	if unusedCount != 2 {
		t.Errorf("expected 2 unused cookbooks (apache2/4.0.0 and unused-cb/1.0.0), got %d", unusedCount)
	}
}

// ---------------------------------------------------------------------------
// buildDetailParams
// ---------------------------------------------------------------------------

func TestBuildDetailParams_Empty(t *testing.T) {
	params := buildDetailParams("a-1", "org-1", nil, nil, nil)
	if len(params) != 0 {
		t.Errorf("expected 0 params for empty input, got %d", len(params))
	}
}

func TestBuildDetailParams_ActiveCookbook(t *testing.T) {
	agg := map[cookbookVersionKey]*aggregatedUsage{
		{Name: "ntp", Version: "3.0.0"}: {
			NodeCount: 2,
			NodeNames: map[string]bool{"web1": true, "web2": true},
			Roles:     map[string]bool{"base": true},
			PlatformCounts: map[string]int{
				"ubuntu/22.04": 2,
			},
			PlatformFamilyCounts: map[string]int{
				"debian": 2,
			},
			PolicyNames:  map[string]bool{},
			PolicyGroups: map[string]bool{},
		},
	}

	inv := map[cookbookVersionKey]bool{
		{Name: "ntp", Version: "3.0.0"}: true,
	}

	active := map[cookbookVersionKey]bool{
		{Name: "ntp", Version: "3.0.0"}: true,
	}

	params := buildDetailParams("analysis-1", "org-1", agg, inv, active)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}

	p := params[0]
	if p.AnalysisID != "analysis-1" {
		t.Errorf("expected AnalysisID=analysis-1, got %q", p.AnalysisID)
	}
	if p.OrganisationID != "org-1" {
		t.Errorf("expected OrganisationID=org-1, got %q", p.OrganisationID)
	}
	if p.CookbookName != "ntp" {
		t.Errorf("expected CookbookName=ntp, got %q", p.CookbookName)
	}
	if p.CookbookVersion != "3.0.0" {
		t.Errorf("expected CookbookVersion=3.0.0, got %q", p.CookbookVersion)
	}
	if p.NodeCount != 2 {
		t.Errorf("expected NodeCount=2, got %d", p.NodeCount)
	}
	if !p.IsActive {
		t.Error("expected IsActive=true")
	}
	if p.NodeNames == nil {
		t.Error("expected NodeNames to be non-nil")
	}
	if p.Roles == nil {
		t.Error("expected Roles to be non-nil")
	}
	if p.PlatformCounts == nil {
		t.Error("expected PlatformCounts to be non-nil")
	}
	if p.PlatformFamilyCounts == nil {
		t.Error("expected PlatformFamilyCounts to be non-nil")
	}
}

func TestBuildDetailParams_UnusedCookbook(t *testing.T) {
	// Cookbook exists in inventory but has no nodes.
	agg := map[cookbookVersionKey]*aggregatedUsage{}

	inv := map[cookbookVersionKey]bool{
		{Name: "unused", Version: "1.0.0"}: true,
	}

	active := map[cookbookVersionKey]bool{}

	params := buildDetailParams("a-1", "org-1", agg, inv, active)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}

	p := params[0]
	if p.IsActive {
		t.Error("expected IsActive=false for unused cookbook")
	}
	if p.NodeCount != 0 {
		t.Errorf("expected NodeCount=0, got %d", p.NodeCount)
	}
	if p.NodeNames != nil {
		t.Error("expected NodeNames=nil for unused cookbook")
	}
	if p.Roles != nil {
		t.Error("expected Roles=nil for unused cookbook")
	}
}

func TestBuildDetailParams_MixedActiveAndUnused(t *testing.T) {
	agg := map[cookbookVersionKey]*aggregatedUsage{
		{Name: "apache2", Version: "5.0.0"}: {
			NodeCount:            3,
			NodeNames:            map[string]bool{"w1": true, "w2": true, "w3": true},
			Roles:                map[string]bool{},
			PolicyNames:          map[string]bool{},
			PolicyGroups:         map[string]bool{},
			PlatformCounts:       map[string]int{},
			PlatformFamilyCounts: map[string]int{},
		},
	}

	inv := map[cookbookVersionKey]bool{
		{Name: "apache2", Version: "5.0.0"}: true,
		{Name: "apache2", Version: "4.0.0"}: true,
		{Name: "legacy", Version: "0.1.0"}:  true,
	}

	active := map[cookbookVersionKey]bool{
		{Name: "apache2", Version: "5.0.0"}: true,
	}

	params := buildDetailParams("a-1", "org-1", agg, inv, active)
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}

	// Sort for deterministic checking.
	sort.Slice(params, func(i, j int) bool {
		if params[i].CookbookName != params[j].CookbookName {
			return params[i].CookbookName < params[j].CookbookName
		}
		return params[i].CookbookVersion < params[j].CookbookVersion
	})

	// apache2/4.0.0 — unused
	if params[0].CookbookName != "apache2" || params[0].CookbookVersion != "4.0.0" {
		t.Errorf("expected apache2/4.0.0, got %s/%s", params[0].CookbookName, params[0].CookbookVersion)
	}
	if params[0].IsActive {
		t.Error("expected apache2/4.0.0 to be inactive")
	}

	// apache2/5.0.0 — active
	if params[1].CookbookName != "apache2" || params[1].CookbookVersion != "5.0.0" {
		t.Errorf("expected apache2/5.0.0, got %s/%s", params[1].CookbookName, params[1].CookbookVersion)
	}
	if !params[1].IsActive {
		t.Error("expected apache2/5.0.0 to be active")
	}
	if params[1].NodeCount != 3 {
		t.Errorf("expected NodeCount=3, got %d", params[1].NodeCount)
	}

	// legacy/0.1.0 — unused
	if params[2].CookbookName != "legacy" || params[2].CookbookVersion != "0.1.0" {
		t.Errorf("expected legacy/0.1.0, got %s/%s", params[2].CookbookName, params[2].CookbookVersion)
	}
	if params[2].IsActive {
		t.Error("expected legacy/0.1.0 to be inactive")
	}
}

func TestBuildDetailParams_CookbookNotInInventoryButInAgg(t *testing.T) {
	// A node runs a cookbook that is somehow not in the server inventory.
	// (e.g. cached run or race condition.) Should still be included.
	agg := map[cookbookVersionKey]*aggregatedUsage{
		{Name: "ghost", Version: "1.0.0"}: {
			NodeCount:            1,
			NodeNames:            map[string]bool{"n1": true},
			Roles:                map[string]bool{},
			PolicyNames:          map[string]bool{},
			PolicyGroups:         map[string]bool{},
			PlatformCounts:       map[string]int{},
			PlatformFamilyCounts: map[string]int{},
		},
	}

	inv := map[cookbookVersionKey]bool{} // Empty inventory
	active := buildActiveSet(agg)

	params := buildDetailParams("a-1", "org-1", agg, inv, active)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}

	p := params[0]
	if p.CookbookName != "ghost" {
		t.Errorf("expected ghost, got %q", p.CookbookName)
	}
	if !p.IsActive {
		t.Error("expected ghost/1.0.0 to be active (has nodes)")
	}
}

func TestBuildDetailParams_DeterministicOrder(t *testing.T) {
	// Verify that output is sorted by name then version.
	inv := map[cookbookVersionKey]bool{
		{Name: "z-cookbook", Version: "1.0"}: true,
		{Name: "a-cookbook", Version: "2.0"}: true,
		{Name: "a-cookbook", Version: "1.0"}: true,
		{Name: "m-cookbook", Version: "1.0"}: true,
	}

	params := buildDetailParams("a-1", "org-1", nil, inv, nil)
	if len(params) != 4 {
		t.Fatalf("expected 4 params, got %d", len(params))
	}

	expected := []string{"a-cookbook/1.0", "a-cookbook/2.0", "m-cookbook/1.0", "z-cookbook/1.0"}
	for i, p := range params {
		got := p.CookbookName + "/" + p.CookbookVersion
		if got != expected[i] {
			t.Errorf("position %d: expected %q, got %q", i, expected[i], got)
		}
	}
}

// ---------------------------------------------------------------------------
// marshalSortedStringSet
// ---------------------------------------------------------------------------

func TestMarshalSortedStringSet_Empty(t *testing.T) {
	result := marshalSortedStringSet(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %q", string(result))
	}

	result = marshalSortedStringSet(map[string]bool{})
	if result != nil {
		t.Errorf("expected nil for empty map, got %q", string(result))
	}
}

func TestMarshalSortedStringSet_Sorted(t *testing.T) {
	set := map[string]bool{
		"charlie": true,
		"alpha":   true,
		"bravo":   true,
	}

	result := marshalSortedStringSet(set)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	var arr []string
	if err := json.Unmarshal(result, &arr); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	expected := []string{"alpha", "bravo", "charlie"}
	if len(arr) != len(expected) {
		t.Fatalf("expected %d elements, got %d", len(expected), len(arr))
	}
	for i, v := range expected {
		if arr[i] != v {
			t.Errorf("position %d: expected %q, got %q", i, v, arr[i])
		}
	}
}

func TestMarshalSortedStringSet_SingleEntry(t *testing.T) {
	set := map[string]bool{"only": true}
	result := marshalSortedStringSet(set)

	var arr []string
	if err := json.Unmarshal(result, &arr); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(arr) != 1 || arr[0] != "only" {
		t.Errorf("expected [\"only\"], got %v", arr)
	}
}

// ---------------------------------------------------------------------------
// marshalStringIntMap
// ---------------------------------------------------------------------------

func TestMarshalStringIntMap_Empty(t *testing.T) {
	result := marshalStringIntMap(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %q", string(result))
	}

	result = marshalStringIntMap(map[string]int{})
	if result != nil {
		t.Errorf("expected nil for empty map, got %q", string(result))
	}
}

func TestMarshalStringIntMap_Populated(t *testing.T) {
	m := map[string]int{
		"ubuntu/22.04": 5,
		"centos/7":     3,
	}

	result := marshalStringIntMap(m)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	var parsed map[string]int
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed["ubuntu/22.04"] != 5 {
		t.Errorf("expected ubuntu/22.04=5, got %d", parsed["ubuntu/22.04"])
	}
	if parsed["centos/7"] != 3 {
		t.Errorf("expected centos/7=3, got %d", parsed["centos/7"])
	}
}

// ---------------------------------------------------------------------------
// NodeRecordFromSnapshot
// ---------------------------------------------------------------------------

func TestNodeRecordFromSnapshot_Basic(t *testing.T) {
	snap := datastore.NodeSnapshot{
		NodeName:        "web1",
		Platform:        "ubuntu",
		PlatformVersion: "22.04",
		PlatformFamily:  "debian",
		PolicyName:      "web-policy",
		PolicyGroup:     "prod",
		Cookbooks:       json.RawMessage(`{"apache2":{"version":"5.0.0"},"ntp":{"version":"3.0.0"}}`),
		Roles:           json.RawMessage(`["webserver","base"]`),
	}

	nr, err := NodeRecordFromSnapshot(snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.NodeName != "web1" {
		t.Errorf("expected NodeName=web1, got %q", nr.NodeName)
	}
	if nr.Platform != "ubuntu" {
		t.Errorf("expected Platform=ubuntu, got %q", nr.Platform)
	}
	if nr.PlatformVersion != "22.04" {
		t.Errorf("expected PlatformVersion=22.04, got %q", nr.PlatformVersion)
	}
	if nr.PlatformFamily != "debian" {
		t.Errorf("expected PlatformFamily=debian, got %q", nr.PlatformFamily)
	}
	if nr.PolicyName != "web-policy" {
		t.Errorf("expected PolicyName=web-policy, got %q", nr.PolicyName)
	}
	if nr.PolicyGroup != "prod" {
		t.Errorf("expected PolicyGroup=prod, got %q", nr.PolicyGroup)
	}

	if len(nr.CookbookVersions) != 2 {
		t.Fatalf("expected 2 cookbooks, got %d", len(nr.CookbookVersions))
	}
	if nr.CookbookVersions["apache2"] != "5.0.0" {
		t.Errorf("expected apache2=5.0.0, got %q", nr.CookbookVersions["apache2"])
	}
	if nr.CookbookVersions["ntp"] != "3.0.0" {
		t.Errorf("expected ntp=3.0.0, got %q", nr.CookbookVersions["ntp"])
	}

	if len(nr.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(nr.Roles))
	}
	if nr.Roles[0] != "webserver" || nr.Roles[1] != "base" {
		t.Errorf("expected [webserver, base], got %v", nr.Roles)
	}
}

func TestNodeRecordFromSnapshot_SimplifiedCookbookFormat(t *testing.T) {
	// Some test data uses the simplified format: {"name": "version"}
	snap := datastore.NodeSnapshot{
		NodeName:  "simple-node",
		Cookbooks: json.RawMessage(`{"apache2":"5.0.0","ntp":"3.0.0"}`),
	}

	nr, err := NodeRecordFromSnapshot(snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nr.CookbookVersions) != 2 {
		t.Fatalf("expected 2 cookbooks, got %d", len(nr.CookbookVersions))
	}
	if nr.CookbookVersions["apache2"] != "5.0.0" {
		t.Errorf("expected apache2=5.0.0, got %q", nr.CookbookVersions["apache2"])
	}
}

func TestNodeRecordFromSnapshot_EmptyCookbooks(t *testing.T) {
	snap := datastore.NodeSnapshot{
		NodeName:  "empty-cb",
		Cookbooks: nil,
		Roles:     nil,
	}

	nr, err := NodeRecordFromSnapshot(snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.CookbookVersions != nil {
		t.Errorf("expected nil CookbookVersions, got %v", nr.CookbookVersions)
	}
	if nr.Roles != nil {
		t.Errorf("expected nil Roles, got %v", nr.Roles)
	}
}

func TestNodeRecordFromSnapshot_InvalidCookbookJSON(t *testing.T) {
	snap := datastore.NodeSnapshot{
		NodeName:  "bad-json",
		Cookbooks: json.RawMessage(`{not valid json}`),
	}

	_, err := NodeRecordFromSnapshot(snap)
	if err == nil {
		t.Error("expected error for invalid cookbook JSON")
	}
}

func TestNodeRecordFromSnapshot_InvalidRolesJSON(t *testing.T) {
	snap := datastore.NodeSnapshot{
		NodeName: "bad-roles",
		Roles:    json.RawMessage(`{not valid json}`),
	}

	_, err := NodeRecordFromSnapshot(snap)
	if err == nil {
		t.Error("expected error for invalid roles JSON")
	}
}

func TestNodeRecordFromSnapshot_EmptyJSONObjects(t *testing.T) {
	snap := datastore.NodeSnapshot{
		NodeName:  "empty-json",
		Cookbooks: json.RawMessage(`{}`),
		Roles:     json.RawMessage(`[]`),
	}

	nr, err := NodeRecordFromSnapshot(snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nr.CookbookVersions) != 0 {
		t.Errorf("expected 0 cookbooks, got %d", len(nr.CookbookVersions))
	}
	if len(nr.Roles) != 0 {
		t.Errorf("expected 0 roles, got %d", len(nr.Roles))
	}
}

func TestNodeRecordFromSnapshot_CookbookWithMissingVersion(t *testing.T) {
	// Map entry with no "version" key — should be skipped gracefully.
	snap := datastore.NodeSnapshot{
		NodeName:  "no-version",
		Cookbooks: json.RawMessage(`{"broken":{"name":"broken"}}`),
	}

	nr, err := NodeRecordFromSnapshot(snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The broken cookbook has no version, so it should have been skipped.
	if _, ok := nr.CookbookVersions["broken"]; ok {
		t.Error("expected broken cookbook to be skipped (no version key)")
	}
}

func TestNodeRecordFromSnapshot_MixedCookbookFormats(t *testing.T) {
	// Technically unusual, but test robustness: some entries have the
	// map format and one has a string format.
	snap := datastore.NodeSnapshot{
		NodeName:  "mixed",
		Cookbooks: json.RawMessage(`{"cb1":{"version":"1.0"},"cb2":"2.0"}`),
	}

	nr, err := NodeRecordFromSnapshot(snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.CookbookVersions["cb1"] != "1.0" {
		t.Errorf("expected cb1=1.0, got %q", nr.CookbookVersions["cb1"])
	}
	if nr.CookbookVersions["cb2"] != "2.0" {
		t.Errorf("expected cb2=2.0, got %q", nr.CookbookVersions["cb2"])
	}
}

// ---------------------------------------------------------------------------
// NodeRecordFromCollectedData
// ---------------------------------------------------------------------------

func TestNodeRecordFromCollectedData(t *testing.T) {
	nr := NodeRecordFromCollectedData(
		"web1",
		"ubuntu",
		"22.04",
		"debian",
		[]string{"webserver", "base"},
		"web-policy",
		"production",
		map[string]string{"apache2": "5.0.0", "ntp": "3.0.0"},
	)

	if nr.NodeName != "web1" {
		t.Errorf("expected NodeName=web1, got %q", nr.NodeName)
	}
	if nr.Platform != "ubuntu" {
		t.Errorf("expected Platform=ubuntu, got %q", nr.Platform)
	}
	if nr.PlatformVersion != "22.04" {
		t.Errorf("expected PlatformVersion=22.04, got %q", nr.PlatformVersion)
	}
	if nr.PlatformFamily != "debian" {
		t.Errorf("expected PlatformFamily=debian, got %q", nr.PlatformFamily)
	}
	if len(nr.Roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(nr.Roles))
	}
	if nr.PolicyName != "web-policy" {
		t.Errorf("expected PolicyName=web-policy, got %q", nr.PolicyName)
	}
	if nr.PolicyGroup != "production" {
		t.Errorf("expected PolicyGroup=production, got %q", nr.PolicyGroup)
	}
	if len(nr.CookbookVersions) != 2 {
		t.Errorf("expected 2 cookbooks, got %d", len(nr.CookbookVersions))
	}
}

func TestNodeRecordFromCollectedData_NilValues(t *testing.T) {
	nr := NodeRecordFromCollectedData("n1", "", "", "", nil, "", "", nil)

	if nr.NodeName != "n1" {
		t.Errorf("expected NodeName=n1, got %q", nr.NodeName)
	}
	if nr.Roles != nil {
		t.Errorf("expected nil Roles, got %v", nr.Roles)
	}
	if nr.CookbookVersions != nil {
		t.Errorf("expected nil CookbookVersions, got %v", nr.CookbookVersions)
	}
}

// ---------------------------------------------------------------------------
// End-to-end: extraction → aggregation → detail building
// ---------------------------------------------------------------------------

func TestEndToEnd_FullAnalysisPipeline(t *testing.T) {
	// Simulate a small fleet of 5 nodes across 2 platforms, running 3 cookbooks.
	nodes := []NodeRecord{
		{
			NodeName: "web1", Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian",
			Roles: []string{"webserver", "base"},
			CookbookVersions: map[string]string{
				"apache2": "5.0.0", "ntp": "3.0.0", "users": "1.0.0",
			},
		},
		{
			NodeName: "web2", Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian",
			Roles: []string{"webserver", "base"},
			CookbookVersions: map[string]string{
				"apache2": "5.0.0", "ntp": "3.0.0", "users": "1.0.0",
			},
		},
		{
			NodeName: "db1", Platform: "centos", PlatformVersion: "7", PlatformFamily: "rhel",
			Roles: []string{"database", "base"},
			CookbookVersions: map[string]string{
				"postgresql": "10.0.0", "ntp": "3.0.0", "users": "1.0.0",
			},
		},
		{
			NodeName: "db2", Platform: "centos", PlatformVersion: "7", PlatformFamily: "rhel",
			Roles: []string{"database", "base"},
			CookbookVersions: map[string]string{
				"postgresql": "10.0.0", "ntp": "3.0.0",
			},
		},
		{
			NodeName: "policy-node", Platform: "ubuntu", PlatformVersion: "24.04", PlatformFamily: "debian",
			PolicyName: "web-policy", PolicyGroup: "production",
			CookbookVersions: map[string]string{
				"apache2": "6.0.0", "ntp": "3.0.0",
			},
		},
	}

	inventory := []CookbookInventoryEntry{
		{Name: "apache2", Version: "5.0.0"},
		{Name: "apache2", Version: "6.0.0"},
		{Name: "apache2", Version: "4.0.0"}, // unused
		{Name: "ntp", Version: "3.0.0"},
		{Name: "ntp", Version: "2.0.0"}, // unused
		{Name: "users", Version: "1.0.0"},
		{Name: "postgresql", Version: "10.0.0"},
		{Name: "legacy-cookbook", Version: "0.1.0"}, // unused
	}

	// Phase 1: Extract
	a := &Analyser{concurrency: 4}
	tuples := a.extractTuples(context.TODO(), nodes)

	// Each node contributes len(CookbookVersions) tuples:
	// web1: 3, web2: 3, db1: 3, db2: 2, policy-node: 2 = 13
	if len(tuples) != 13 {
		t.Fatalf("expected 13 tuples, got %d", len(tuples))
	}

	// Phase 2: Aggregate
	aggregated := aggregateTuples(tuples)

	// Unique cookbook versions: apache2/5.0.0, apache2/6.0.0, ntp/3.0.0,
	// users/1.0.0, postgresql/10.0.0 = 5
	if len(aggregated) != 5 {
		t.Fatalf("expected 5 aggregated entries, got %d", len(aggregated))
	}

	// Verify ntp/3.0.0 — should have all 5 nodes.
	ntpKey := cookbookVersionKey{Name: "ntp", Version: "3.0.0"}
	ntpUsage := aggregated[ntpKey]
	if ntpUsage == nil {
		t.Fatal("expected ntp/3.0.0 in aggregation")
	}
	if ntpUsage.NodeCount != 5 {
		t.Errorf("expected ntp/3.0.0 NodeCount=5, got %d", ntpUsage.NodeCount)
	}

	// Verify apache2/5.0.0 — web1 and web2.
	apache5Key := cookbookVersionKey{Name: "apache2", Version: "5.0.0"}
	apache5Usage := aggregated[apache5Key]
	if apache5Usage == nil {
		t.Fatal("expected apache2/5.0.0 in aggregation")
	}
	if apache5Usage.NodeCount != 2 {
		t.Errorf("expected apache2/5.0.0 NodeCount=2, got %d", apache5Usage.NodeCount)
	}

	// Verify apache2/6.0.0 — only policy-node.
	apache6Key := cookbookVersionKey{Name: "apache2", Version: "6.0.0"}
	apache6Usage := aggregated[apache6Key]
	if apache6Usage == nil {
		t.Fatal("expected apache2/6.0.0 in aggregation")
	}
	if apache6Usage.NodeCount != 1 {
		t.Errorf("expected apache2/6.0.0 NodeCount=1, got %d", apache6Usage.NodeCount)
	}
	if !apache6Usage.PolicyNames["web-policy"] {
		t.Error("expected web-policy in apache2/6.0.0 PolicyNames")
	}
	if !apache6Usage.PolicyGroups["production"] {
		t.Error("expected production in apache2/6.0.0 PolicyGroups")
	}

	// Verify roles aggregation for ntp (should have: webserver, base, database).
	expectedNtpRoles := map[string]bool{"webserver": true, "base": true, "database": true}
	for role := range expectedNtpRoles {
		if !ntpUsage.Roles[role] {
			t.Errorf("expected role %q for ntp/3.0.0", role)
		}
	}

	// Phase 3: Active/unused flagging.
	inventorySet := buildInventorySet(inventory)
	activeSet := buildActiveSet(aggregated)

	activeCount := 0
	unusedCount := 0
	for key := range inventorySet {
		if activeSet[key] {
			activeCount++
		} else {
			unusedCount++
		}
	}

	// Active: apache2/5.0.0, apache2/6.0.0, ntp/3.0.0, users/1.0.0, postgresql/10.0.0 = 5
	// Unused: apache2/4.0.0, ntp/2.0.0, legacy-cookbook/0.1.0 = 3
	if activeCount != 5 {
		t.Errorf("expected 5 active cookbook versions, got %d", activeCount)
	}
	if unusedCount != 3 {
		t.Errorf("expected 3 unused cookbook versions, got %d", unusedCount)
	}

	// Build detail params.
	detailParams := buildDetailParams("analysis-1", "org-1", aggregated, inventorySet, activeSet)

	// Should have 8 entries (5 active + 3 unused from inventory).
	if len(detailParams) != 8 {
		t.Fatalf("expected 8 detail params, got %d", len(detailParams))
	}

	// Verify all params have correct IDs.
	for _, p := range detailParams {
		if p.AnalysisID != "analysis-1" {
			t.Errorf("expected AnalysisID=analysis-1, got %q", p.AnalysisID)
		}
		if p.OrganisationID != "org-1" {
			t.Errorf("expected OrganisationID=org-1, got %q", p.OrganisationID)
		}
	}

	// Verify that unused entries have NodeCount=0 and nil JSON fields.
	for _, p := range detailParams {
		if !p.IsActive {
			if p.NodeCount != 0 {
				t.Errorf("unused cookbook %s/%s should have NodeCount=0, got %d",
					p.CookbookName, p.CookbookVersion, p.NodeCount)
			}
		}
	}

	// Verify that the ntp/3.0.0 detail has correct JSON payload.
	var ntpDetail *datastore.InsertCookbookUsageDetailParams
	for i := range detailParams {
		if detailParams[i].CookbookName == "ntp" && detailParams[i].CookbookVersion == "3.0.0" {
			ntpDetail = &detailParams[i]
			break
		}
	}
	if ntpDetail == nil {
		t.Fatal("expected ntp/3.0.0 in detail params")
	}
	if ntpDetail.NodeCount != 5 {
		t.Errorf("expected ntp/3.0.0 NodeCount=5, got %d", ntpDetail.NodeCount)
	}

	// Verify NodeNames JSON contains all 5 nodes.
	var nodeNames []string
	if err := json.Unmarshal(ntpDetail.NodeNames, &nodeNames); err != nil {
		t.Fatalf("failed to unmarshal NodeNames: %v", err)
	}
	if len(nodeNames) != 5 {
		t.Errorf("expected 5 node names, got %d", len(nodeNames))
	}

	// Verify Roles JSON for ntp.
	var roles []string
	if err := json.Unmarshal(ntpDetail.Roles, &roles); err != nil {
		t.Fatalf("failed to unmarshal Roles: %v", err)
	}
	// Should have: base, database, webserver (sorted).
	sort.Strings(roles)
	expectedRoles := []string{"base", "database", "webserver"}
	if len(roles) != len(expectedRoles) {
		t.Fatalf("expected %d roles, got %d: %v", len(expectedRoles), len(roles), roles)
	}
	for i, r := range expectedRoles {
		if roles[i] != r {
			t.Errorf("role %d: expected %q, got %q", i, r, roles[i])
		}
	}

	// Verify PlatformCounts JSON for ntp.
	var platformCounts map[string]int
	if err := json.Unmarshal(ntpDetail.PlatformCounts, &platformCounts); err != nil {
		t.Fatalf("failed to unmarshal PlatformCounts: %v", err)
	}
	// ubuntu/22.04: 2 (web1, web2), centos/7: 2 (db1, db2), ubuntu/24.04: 1 (policy-node)
	if platformCounts["ubuntu/22.04"] != 2 {
		t.Errorf("expected ubuntu/22.04=2, got %d", platformCounts["ubuntu/22.04"])
	}
	if platformCounts["centos/7"] != 2 {
		t.Errorf("expected centos/7=2, got %d", platformCounts["centos/7"])
	}
	if platformCounts["ubuntu/24.04"] != 1 {
		t.Errorf("expected ubuntu/24.04=1, got %d", platformCounts["ubuntu/24.04"])
	}

	// Verify PlatformFamilyCounts JSON for ntp.
	var familyCounts map[string]int
	if err := json.Unmarshal(ntpDetail.PlatformFamilyCounts, &familyCounts); err != nil {
		t.Fatalf("failed to unmarshal PlatformFamilyCounts: %v", err)
	}
	// debian: 3 (web1, web2, policy-node), rhel: 2 (db1, db2)
	if familyCounts["debian"] != 3 {
		t.Errorf("expected debian=3, got %d", familyCounts["debian"])
	}
	if familyCounts["rhel"] != 2 {
		t.Errorf("expected rhel=2, got %d", familyCounts["rhel"])
	}

	// Verify policy-node's apache2/6.0.0 detail has PolicyNames and PolicyGroups.
	var apache6Detail *datastore.InsertCookbookUsageDetailParams
	for i := range detailParams {
		if detailParams[i].CookbookName == "apache2" && detailParams[i].CookbookVersion == "6.0.0" {
			apache6Detail = &detailParams[i]
			break
		}
	}
	if apache6Detail == nil {
		t.Fatal("expected apache2/6.0.0 in detail params")
	}

	var policyNames []string
	if err := json.Unmarshal(apache6Detail.PolicyNames, &policyNames); err != nil {
		t.Fatalf("failed to unmarshal PolicyNames: %v", err)
	}
	if len(policyNames) != 1 || policyNames[0] != "web-policy" {
		t.Errorf("expected [web-policy], got %v", policyNames)
	}

	var policyGroups []string
	if err := json.Unmarshal(apache6Detail.PolicyGroups, &policyGroups); err != nil {
		t.Fatalf("failed to unmarshal PolicyGroups: %v", err)
	}
	if len(policyGroups) != 1 || policyGroups[0] != "production" {
		t.Errorf("expected [production], got %v", policyGroups)
	}
}

func TestEndToEnd_NoNodesAllUnused(t *testing.T) {
	// No nodes, but there's inventory — everything should be unused.
	inventory := []CookbookInventoryEntry{
		{Name: "cb1", Version: "1.0"},
		{Name: "cb2", Version: "2.0"},
	}

	a := &Analyser{concurrency: 2}
	tuples := a.extractTuples(context.TODO(), nil)
	aggregated := aggregateTuples(tuples)
	inventorySet := buildInventorySet(inventory)
	activeSet := buildActiveSet(aggregated)

	activeCount := 0
	unusedCount := 0
	for key := range inventorySet {
		if activeSet[key] {
			activeCount++
		} else {
			unusedCount++
		}
	}

	if activeCount != 0 {
		t.Errorf("expected 0 active, got %d", activeCount)
	}
	if unusedCount != 2 {
		t.Errorf("expected 2 unused, got %d", unusedCount)
	}

	params := buildDetailParams("a1", "o1", aggregated, inventorySet, activeSet)
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(params))
	}
	for _, p := range params {
		if p.IsActive {
			t.Error("expected all params to be inactive")
		}
		if p.NodeCount != 0 {
			t.Errorf("expected NodeCount=0, got %d", p.NodeCount)
		}
	}
}

func TestEndToEnd_AllNodesNoCookbooks(t *testing.T) {
	nodes := []NodeRecord{
		{NodeName: "n1", CookbookVersions: nil},
		{NodeName: "n2", CookbookVersions: map[string]string{}},
	}

	a := &Analyser{concurrency: 2}
	tuples := a.extractTuples(context.TODO(), nodes)
	if len(tuples) != 0 {
		t.Errorf("expected 0 tuples, got %d", len(tuples))
	}

	aggregated := aggregateTuples(tuples)
	if len(aggregated) != 0 {
		t.Errorf("expected 0 aggregated entries, got %d", len(aggregated))
	}
}

// ---------------------------------------------------------------------------
// Validation helpers (datastore layer — tested indirectly)
// ---------------------------------------------------------------------------

func TestBuildDetailParams_AllFieldsPopulated(t *testing.T) {
	// Verify that a fully-populated aggregation produces params with all
	// JSON fields set.
	agg := map[cookbookVersionKey]*aggregatedUsage{
		{Name: "full", Version: "1.0"}: {
			NodeCount:            2,
			NodeNames:            map[string]bool{"n1": true, "n2": true},
			Roles:                map[string]bool{"r1": true},
			PolicyNames:          map[string]bool{"p1": true},
			PolicyGroups:         map[string]bool{"g1": true},
			PlatformCounts:       map[string]int{"linux/5.0": 2},
			PlatformFamilyCounts: map[string]int{"rhel": 2},
		},
	}

	inv := map[cookbookVersionKey]bool{
		{Name: "full", Version: "1.0"}: true,
	}

	active := map[cookbookVersionKey]bool{
		{Name: "full", Version: "1.0"}: true,
	}

	params := buildDetailParams("a1", "o1", agg, inv, active)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}

	p := params[0]

	fields := map[string]json.RawMessage{
		"NodeNames":            p.NodeNames,
		"Roles":                p.Roles,
		"PolicyNames":          p.PolicyNames,
		"PolicyGroups":         p.PolicyGroups,
		"PlatformCounts":       p.PlatformCounts,
		"PlatformFamilyCounts": p.PlatformFamilyCounts,
	}
	for name, data := range fields {
		if data == nil {
			t.Errorf("expected %s to be non-nil", name)
		}
	}
}
