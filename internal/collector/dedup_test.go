// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"fmt"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

func makeSnapshotParam(orgID, nodeName, chefVersion string) datastore.InsertNodeSnapshotParams {
	return datastore.InsertNodeSnapshotParams{
		CollectionRunID: "run-1",
		OrganisationID:  orgID,
		NodeName:        nodeName,
		ChefVersion:     chefVersion,
		CollectedAt:     time.Now().UTC(),
	}
}

func TestDeduplicateSnapshotParams_NoDuplicates(t *testing.T) {
	params := []datastore.InsertNodeSnapshotParams{
		makeSnapshotParam("org-1", "node-a", "18.0"),
		makeSnapshotParam("org-1", "node-b", "18.0"),
		makeSnapshotParam("org-1", "node-c", "18.0"),
	}

	result, dupes := deduplicateSnapshotParams(params)
	if dupes != 0 {
		t.Errorf("expected 0 duplicates, got %d", dupes)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 results, got %d", len(result))
	}
}

func TestDeduplicateSnapshotParams_EmptySlice(t *testing.T) {
	result, dupes := deduplicateSnapshotParams(nil)
	if dupes != 0 {
		t.Errorf("expected 0 duplicates, got %d", dupes)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}

	result, dupes = deduplicateSnapshotParams([]datastore.InsertNodeSnapshotParams{})
	if dupes != 0 {
		t.Errorf("expected 0 duplicates, got %d", dupes)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestDeduplicateSnapshotParams_SingleElement(t *testing.T) {
	params := []datastore.InsertNodeSnapshotParams{
		makeSnapshotParam("org-1", "node-a", "18.0"),
	}

	result, dupes := deduplicateSnapshotParams(params)
	if dupes != 0 {
		t.Errorf("expected 0 duplicates, got %d", dupes)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
}

func TestDeduplicateSnapshotParams_DuplicateLastWins(t *testing.T) {
	params := []datastore.InsertNodeSnapshotParams{
		makeSnapshotParam("org-1", "node-a", "17.0"),
		makeSnapshotParam("org-1", "node-b", "18.0"),
		makeSnapshotParam("org-1", "node-a", "18.5"),
	}

	result, dupes := deduplicateSnapshotParams(params)
	if dupes != 1 {
		t.Errorf("expected 1 duplicate, got %d", dupes)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// node-a should have the last value (18.5), not the first (17.0).
	var nodeA *datastore.InsertNodeSnapshotParams
	for i := range result {
		if result[i].NodeName == "node-a" {
			nodeA = &result[i]
			break
		}
	}
	if nodeA == nil {
		t.Fatal("expected node-a in results")
	}
	if nodeA.ChefVersion != "18.5" {
		t.Errorf("expected node-a ChefVersion=18.5 (last wins), got %q", nodeA.ChefVersion)
	}
}

func TestDeduplicateSnapshotParams_MultipleDuplicates(t *testing.T) {
	params := []datastore.InsertNodeSnapshotParams{
		makeSnapshotParam("org-1", "node-a", "16.0"),
		makeSnapshotParam("org-1", "node-a", "17.0"),
		makeSnapshotParam("org-1", "node-a", "18.0"),
		makeSnapshotParam("org-1", "node-b", "18.0"),
	}

	result, dupes := deduplicateSnapshotParams(params)
	if dupes != 2 {
		t.Errorf("expected 2 duplicates, got %d", dupes)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// node-a should keep the last occurrence.
	for _, p := range result {
		if p.NodeName == "node-a" && p.ChefVersion != "18.0" {
			t.Errorf("expected node-a ChefVersion=18.0 (last wins), got %q", p.ChefVersion)
		}
	}
}

func TestDeduplicateSnapshotParams_CrossOrgSameNameNotDeduplicated(t *testing.T) {
	// Same node name in different orgs must NOT be treated as duplicates.
	params := []datastore.InsertNodeSnapshotParams{
		makeSnapshotParam("org-1", "web-01", "18.0"),
		makeSnapshotParam("org-2", "web-01", "17.0"),
	}

	result, dupes := deduplicateSnapshotParams(params)
	if dupes != 0 {
		t.Errorf("expected 0 duplicates (different orgs), got %d", dupes)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
}

func TestDeduplicateSnapshotParams_PreservesOrder(t *testing.T) {
	params := []datastore.InsertNodeSnapshotParams{
		makeSnapshotParam("org-1", "node-c", "18.0"),
		makeSnapshotParam("org-1", "node-a", "17.0"),
		makeSnapshotParam("org-1", "node-b", "18.0"),
		makeSnapshotParam("org-1", "node-a", "18.5"),
	}

	result, dupes := deduplicateSnapshotParams(params)
	if dupes != 1 {
		t.Errorf("expected 1 duplicate, got %d", dupes)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	// Order should be: node-c, node-b, node-a (with 18.5).
	// node-a's position moves to where its last occurrence was (index 3),
	// which after dedup becomes index 2 (after node-c and node-b).
	expectedNames := []string{"node-c", "node-b", "node-a"}
	for i, name := range expectedNames {
		if result[i].NodeName != name {
			t.Errorf("result[%d]: expected NodeName=%q, got %q", i, name, result[i].NodeName)
		}
	}

	// Verify node-a has the last-wins value.
	if result[2].ChefVersion != "18.5" {
		t.Errorf("expected node-a ChefVersion=18.5, got %q", result[2].ChefVersion)
	}
}

func TestDeduplicateSnapshotParams_AllDuplicates(t *testing.T) {
	params := []datastore.InsertNodeSnapshotParams{
		makeSnapshotParam("org-1", "node-a", "16.0"),
		makeSnapshotParam("org-1", "node-a", "17.0"),
		makeSnapshotParam("org-1", "node-a", "18.0"),
	}

	result, dupes := deduplicateSnapshotParams(params)
	if dupes != 2 {
		t.Errorf("expected 2 duplicates, got %d", dupes)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].ChefVersion != "18.0" {
		t.Errorf("expected ChefVersion=18.0 (last wins), got %q", result[0].ChefVersion)
	}
}

func TestDeduplicateSnapshotParams_DuplicateCountAccuracy(t *testing.T) {
	// 5 entries for 2 unique keys = 3 duplicates.
	params := []datastore.InsertNodeSnapshotParams{
		makeSnapshotParam("org-1", "node-a", "16.0"),
		makeSnapshotParam("org-1", "node-b", "17.0"),
		makeSnapshotParam("org-1", "node-a", "17.5"),
		makeSnapshotParam("org-1", "node-b", "18.0"),
		makeSnapshotParam("org-1", "node-a", "18.5"),
	}

	result, dupes := deduplicateSnapshotParams(params)
	if dupes != 3 {
		t.Errorf("expected 3 duplicates, got %d", dupes)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
	if len(result)+dupes != len(params) {
		t.Errorf("result count (%d) + dupes (%d) should equal input count (%d)",
			len(result), dupes, len(params))
	}
}

func TestDeduplicateSnapshotParams_LargeBatchWithDuplicates(t *testing.T) {
	// Simulate the scenario that caused the production failure:
	// a batch of 1000 nodes where one node appears twice due to
	// Chef Server search pagination overlap.
	params := make([]datastore.InsertNodeSnapshotParams, 1001)
	for i := 0; i < 1000; i++ {
		params[i] = makeSnapshotParam("org-1", fmt.Sprintf("node-%04d", i), "18.0")
	}
	// Duplicate of node-0500 at the end with a different version.
	params[1000] = makeSnapshotParam("org-1", "node-0500", "18.5")

	result, dupes := deduplicateSnapshotParams(params)
	if dupes != 1 {
		t.Errorf("expected 1 duplicate, got %d", dupes)
	}
	if len(result) != 1000 {
		t.Errorf("expected 1000 results, got %d", len(result))
	}

	// Find node-0500 and verify it has the last-wins value.
	for _, p := range result {
		if p.NodeName == "node-0500" {
			if p.ChefVersion != "18.5" {
				t.Errorf("expected node-0500 ChefVersion=18.5 (last wins), got %q", p.ChefVersion)
			}
			return
		}
	}
	t.Error("node-0500 not found in results")
}
