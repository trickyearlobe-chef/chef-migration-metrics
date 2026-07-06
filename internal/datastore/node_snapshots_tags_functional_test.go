// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"
)

// Node tags (migration 0048) must round-trip through the single-row upsert +
// GetNodeSnapshotByName, and through the bulk upsert +
// ListNodeSnapshotsByOrganisation — covering both scan paths. An empty slice
// ("collected, no tags") must come back as an empty (non-nil) slice, distinct
// from a node that was never collected.
func TestFunctional_NodeSnapshot_TagsRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	org, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name:          "func-tags-org",
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

	now := time.Now().UTC()

	// 1) Single upsert + Get — a node with tags.
	tagged := InsertNodeSnapshotParams{
		CollectionRunOrg: run.OrganisationName,
		OrganisationName: org.Name,
		NodeName:         "tagged",
		Platform:         "ubuntu",
		CollectedAt:      now,
		Tags:             []string{"prepare", "eu-west"},
	}
	if _, err := db.UpsertNodeSnapshot(ctx, tagged); err != nil {
		t.Fatalf("upsert tagged: %v", err)
	}
	got, err := db.GetNodeSnapshotByName(ctx, org.Name, "tagged")
	if err != nil {
		t.Fatalf("get tagged: %v", err)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "prepare" || got.Tags[1] != "eu-west" {
		t.Errorf("Tags = %v, want [prepare eu-west]", got.Tags)
	}

	// 2) Empty tags ("collected, no tags") round-trips as an empty slice.
	untagged := InsertNodeSnapshotParams{
		CollectionRunOrg: run.OrganisationName,
		OrganisationName: org.Name,
		NodeName:         "untagged",
		Platform:         "ubuntu",
		CollectedAt:      now,
		Tags:             []string{},
	}
	if _, err := db.UpsertNodeSnapshot(ctx, untagged); err != nil {
		t.Fatalf("upsert untagged: %v", err)
	}
	gotU, err := db.GetNodeSnapshotByName(ctx, org.Name, "untagged")
	if err != nil {
		t.Fatalf("get untagged: %v", err)
	}
	if len(gotU.Tags) != 0 {
		t.Errorf("Tags = %v, want empty", gotU.Tags)
	}

	// 3) Bulk upsert + list scan path carries tags too.
	bulk := tagged
	bulk.NodeName = "tagged-bulk"
	bulk.Tags = []string{"rollback"}
	if _, err := db.BulkUpsertNodeSnapshots(ctx, []InsertNodeSnapshotParams{bulk}); err != nil {
		t.Fatalf("bulk upsert: %v", err)
	}
	list, err := db.ListNodeSnapshotsByOrganisation(ctx, org.Name)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byName := map[string]NodeSnapshot{}
	for _, ns := range list {
		byName[ns.NodeName] = ns
	}
	b, ok := byName["tagged-bulk"]
	if !ok {
		t.Fatal("tagged-bulk not in list")
	}
	if len(b.Tags) != 1 || b.Tags[0] != "rollback" {
		t.Errorf("list Tags = %v, want [rollback]", b.Tags)
	}
}
