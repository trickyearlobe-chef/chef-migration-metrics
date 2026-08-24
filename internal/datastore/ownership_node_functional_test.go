// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"
)

// Node ownership answers "which machines am I on the hook for". The import path
// accepts `node` as an entity type, so the path exists whether or not any node
// ownership data has been loaded. These tests hold it down so it cannot rot
// while nothing is loaded.

func TestFunctional_NodeOwnership_AssignedNodesResolveToTheirOwner(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM ownership_assignments WHERE entity_key LIKE 'nodeown-%'",
		"DELETE FROM owners WHERE name IN ('nodeown.alice', 'nodeown.bob')",
	)

	for _, name := range []string{"nodeown.alice", "nodeown.bob"} {
		if _, err := db.InsertOwner(ctx, InsertOwnerParams{Name: name, OwnerType: "individual"}); err != nil {
			t.Fatalf("creating owner %s: %v", name, err)
		}
	}

	assign := []struct{ owner, node string }{
		{"nodeown.alice", "nodeown-web-01"},
		{"nodeown.alice", "nodeown-web-02"},
		{"nodeown.bob", "nodeown-db-01"},
	}
	for _, a := range assign {
		if _, err := db.InsertAssignment(ctx, InsertAssignmentParams{
			OwnerName: a.owner, EntityType: "node", EntityKey: a.node,
			AssignmentSource: "import", Confidence: "definitive",
		}); err != nil {
			t.Fatalf("assigning %s to %s: %v", a.node, a.owner, err)
		}
	}

	// This is what the node list's owner filter resolves through.
	got, _, err := db.ListAssignmentsByOwner(ctx, AssignmentListFilter{
		OwnerName: "nodeown.alice", EntityType: "node", Limit: 100,
	})
	if err != nil {
		t.Fatalf("listing alice's nodes: %v", err)
	}
	keys := map[string]bool{}
	for _, a := range got {
		keys[a.EntityKey] = true
	}
	if !keys["nodeown-web-01"] || !keys["nodeown-web-02"] {
		t.Errorf("alice's nodes = %v, want both web nodes", keys)
	}
	if keys["nodeown-db-01"] {
		t.Error("bob's node came back as alice's")
	}
}

// The importer reports which node names CMM has actually collected, so an
// operator can see that a row named a machine nobody has ever seen. It must
// never reject the row: assigning ownership before collection has run is a
// normal way to work.
func TestFunctional_NodeOwnership_ImporterConfirmsCollectedNodes(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	org, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name:          "nodeown-org",
		ChefServerURL: "https://example.com/organizations/test",
		OrgName:       "test",
		ClientName:    "test-client",
	})
	if err != nil {
		t.Fatalf("creating org: %v", err)
	}
	run, err := db.CreateCollectionRun(ctx, CreateCollectionRunParams{OrganisationName: org.Name})
	if err != nil {
		t.Fatalf("creating collection run: %v", err)
	}
	cleanupTestData(t, db,
		"DELETE FROM node_snapshots WHERE collection_run_org = '"+run.OrganisationName+"'",
		"DELETE FROM collection_runs WHERE organisation_name = '"+run.OrganisationName+"'",
		"DELETE FROM organisations WHERE name = '"+org.Name+"'",
	)

	if _, err := db.UpsertNodeSnapshot(ctx, InsertNodeSnapshotParams{
		CollectionRunOrg: run.OrganisationName,
		OrganisationName: org.Name,
		NodeName:         "nodeown-collected-01",
		Platform:         "ubuntu",
		CollectedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("creating node snapshot: %v", err)
	}

	found, err := db.EntityKeysExist(ctx, "node",
		[]string{"nodeown-collected-01", "nodeown-never-collected"})
	if err != nil {
		t.Fatalf("checking node keys: %v", err)
	}
	if !found["nodeown-collected-01"] {
		t.Error("a node CMM has collected was reported as unknown")
	}
	if found["nodeown-never-collected"] {
		t.Error("a node nobody has collected was reported as known")
	}
}
