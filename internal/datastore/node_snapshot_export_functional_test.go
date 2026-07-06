// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"
)

// TestFunctional_NodeExport_KeysetPagingCoversAllRows verifies that keyset
// pagination over ListNodeSnapshotsForExport returns every matching row exactly
// once, in (organisation_name, node_name) order, with no dup/skip across page
// boundaries — the core correctness property of the streamed node export.
func TestFunctional_NodeExport_KeysetPagingCoversAllRows(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	org, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name:          "func-export-org",
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
	names := []string{"fe-node-05", "fe-node-01", "fe-node-03", "fe-node-02", "fe-node-04"}
	nodes := make([]InsertNodeSnapshotParams, 0, len(names))
	for i, n := range names {
		plat := "ubuntu"
		if i%2 == 1 {
			plat = "centos"
		}
		nodes = append(nodes, InsertNodeSnapshotParams{
			CollectionRunOrg: run.OrganisationName, OrganisationName: org.Name,
			NodeName: n, Platform: plat, PlatformVersion: "1.0", CollectedAt: now,
		})
	}
	if _, err := db.BulkUpsertNodeSnapshots(ctx, nodes); err != nil {
		t.Fatalf("inserting node snapshots: %v", err)
	}

	f := NodeSnapshotFilter{OrganisationNames: []string{org.Name}}

	// Count must match the seeded rows.
	count, err := db.CountNodeSnapshotsFiltered(ctx, f)
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != len(names) {
		t.Fatalf("CountNodeSnapshotsFiltered = %d, want %d", count, len(names))
	}

	// Keyset-page with a small page size (2) and collect every row.
	var got []string
	cursor := NodeSnapshotCursor{}
	for {
		page, err := db.ListNodeSnapshotsForExport(ctx, f, cursor, 2)
		if err != nil {
			t.Fatalf("export page: %v", err)
		}
		if len(page) == 0 {
			break
		}
		for _, ns := range page {
			got = append(got, ns.NodeName)
		}
		last := page[len(page)-1]
		cursor = NodeSnapshotCursor{OrganisationName: last.OrganisationName, NodeName: last.NodeName, Valid: true}
		if len(page) < 2 {
			break
		}
	}

	// Every node exactly once, in sorted node_name order (single org).
	want := []string{"fe-node-01", "fe-node-02", "fe-node-03", "fe-node-04", "fe-node-05"}
	if len(got) != len(want) {
		t.Fatalf("keyset returned %d rows, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestFunctional_NodeExport_CarriesTags verifies the tags TEXT[] column is
// projected and scanned by the export path (light scan via
// ListNodeSnapshotsForExport), so exported rows carry each node's tags.
func TestFunctional_NodeExport_CarriesTags(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	org, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name:          "func-export-tags-org",
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
	if _, err := db.BulkUpsertNodeSnapshots(ctx, []InsertNodeSnapshotParams{
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org.Name,
			NodeName: "ex-tagged", Platform: "ubuntu", CollectedAt: now,
			Tags: []string{"prepare", "eu-west"}},
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org.Name,
			NodeName: "ex-untagged", Platform: "ubuntu", CollectedAt: now,
			Tags: []string{}},
	}); err != nil {
		t.Fatalf("inserting node snapshots: %v", err)
	}

	f := NodeSnapshotFilter{OrganisationNames: []string{org.Name}}
	page, err := db.ListNodeSnapshotsForExport(ctx, f, NodeSnapshotCursor{}, 100)
	if err != nil {
		t.Fatalf("export page: %v", err)
	}
	byName := map[string][]string{}
	for _, ns := range page {
		byName[ns.NodeName] = ns.Tags
	}
	if tags := byName["ex-tagged"]; len(tags) != 2 || tags[0] != "prepare" || tags[1] != "eu-west" {
		t.Errorf("ex-tagged tags = %v, want [prepare eu-west]", tags)
	}
	if tags := byName["ex-untagged"]; len(tags) != 0 {
		t.Errorf("ex-untagged tags = %v, want empty", tags)
	}
}

// TestFunctional_NodeExport_FilterParity verifies the export query and the list
// query return the same row set for the same filter (a platform filter here).
func TestFunctional_NodeExport_FilterParity(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	org, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name:          "func-export-parity-org",
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
	nodes := []InsertNodeSnapshotParams{
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org.Name, NodeName: "fp-ubuntu-1", Platform: "ubuntu", PlatformVersion: "22.04", CollectedAt: now},
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org.Name, NodeName: "fp-ubuntu-2", Platform: "ubuntu", PlatformVersion: "22.04", CollectedAt: now},
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org.Name, NodeName: "fp-centos-1", Platform: "centos", PlatformVersion: "7.9", CollectedAt: now},
	}
	if _, err := db.BulkUpsertNodeSnapshots(ctx, nodes); err != nil {
		t.Fatalf("inserting node snapshots: %v", err)
	}

	f := NodeSnapshotFilter{OrganisationNames: []string{org.Name}, Platform: "ubuntu"}

	listRows, listTotal, err := db.ListNodeSnapshotsFiltered(ctx, f)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	exportRows, err := db.ListNodeSnapshotsForExport(ctx, f, NodeSnapshotCursor{}, 100)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if listTotal != 2 || len(exportRows) != 2 {
		t.Fatalf("expected 2 ubuntu nodes; list total=%d, export=%d", listTotal, len(exportRows))
	}
	listSet := map[string]bool{}
	for _, r := range listRows {
		listSet[r.NodeName] = true
	}
	for _, r := range exportRows {
		if !listSet[r.NodeName] {
			t.Errorf("export returned %q not in list result — filter parity broken", r.NodeName)
		}
	}
}
