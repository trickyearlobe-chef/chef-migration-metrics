// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"
)

func ptrBool(b bool) *bool { return &b }
func ptrInt(i int) *int    { return &i }

// The node-level disk verdict (migration 0037) must round-trip through the
// single-row upsert + GetNodeSnapshotByName, and through the bulk upsert +
// ListNodeSnapshotsByOrganisation — covering both scan paths. A nil verdict
// (indeterminate) must come back as nil, distinct from a determinate false.
func TestFunctional_NodeSnapshot_DiskVerdictRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	org, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name:          "func-disk-org",
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

	// 1) Single upsert + GetNodeSnapshotByName — determinate verdict.
	determinate := InsertNodeSnapshotParams{
		CollectionRunOrg:    run.OrganisationName,
		OrganisationName:    org.Name,
		NodeName:            "disk-sufficient",
		Platform:            "ubuntu",
		CollectedAt:         now,
		SufficientDiskSpace: ptrBool(true),
		AvailableDiskMB:     ptrInt(8192),
		RequiredDiskMB:      ptrInt(2048),
	}
	if _, err := db.UpsertNodeSnapshot(ctx, determinate); err != nil {
		t.Fatalf("upsert determinate: %v", err)
	}
	got, err := db.GetNodeSnapshotByName(ctx, org.Name, "disk-sufficient")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SufficientDiskSpace == nil || !*got.SufficientDiskSpace {
		t.Errorf("SufficientDiskSpace = %v, want true", got.SufficientDiskSpace)
	}
	if got.AvailableDiskMB == nil || *got.AvailableDiskMB != 8192 {
		t.Errorf("AvailableDiskMB = %v, want 8192", got.AvailableDiskMB)
	}
	if got.RequiredDiskMB == nil || *got.RequiredDiskMB != 2048 {
		t.Errorf("RequiredDiskMB = %v, want 2048", got.RequiredDiskMB)
	}

	// 2) Indeterminate verdict (nil sufficient/available) still records required.
	indeterminate := InsertNodeSnapshotParams{
		CollectionRunOrg: run.OrganisationName,
		OrganisationName: org.Name,
		NodeName:         "disk-unknown",
		Platform:         "ubuntu",
		CollectedAt:      now,
		RequiredDiskMB:   ptrInt(2048), // platform-only, set even when indeterminate
	}
	if _, err := db.UpsertNodeSnapshot(ctx, indeterminate); err != nil {
		t.Fatalf("upsert indeterminate: %v", err)
	}
	gotU, err := db.GetNodeSnapshotByName(ctx, org.Name, "disk-unknown")
	if err != nil {
		t.Fatalf("get unknown: %v", err)
	}
	if gotU.SufficientDiskSpace != nil {
		t.Errorf("SufficientDiskSpace = %v, want nil (indeterminate)", *gotU.SufficientDiskSpace)
	}
	if gotU.AvailableDiskMB != nil {
		t.Errorf("AvailableDiskMB = %v, want nil", *gotU.AvailableDiskMB)
	}
	if gotU.RequiredDiskMB == nil || *gotU.RequiredDiskMB != 2048 {
		t.Errorf("RequiredDiskMB = %v, want 2048", gotU.RequiredDiskMB)
	}

	// 3) Bulk upsert + list scan path carries the verdict too.
	insufficient := determinate
	insufficient.NodeName = "disk-insufficient"
	insufficient.SufficientDiskSpace = ptrBool(false)
	insufficient.AvailableDiskMB = ptrInt(512)
	if _, err := db.BulkUpsertNodeSnapshots(ctx, []InsertNodeSnapshotParams{insufficient}); err != nil {
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
	ins, ok := byName["disk-insufficient"]
	if !ok {
		t.Fatal("disk-insufficient not in list")
	}
	if ins.SufficientDiskSpace == nil || *ins.SufficientDiskSpace {
		t.Errorf("list SufficientDiskSpace = %v, want false", ins.SufficientDiskSpace)
	}
	if ins.AvailableDiskMB == nil || *ins.AvailableDiskMB != 512 {
		t.Errorf("list AvailableDiskMB = %v, want 512", ins.AvailableDiskMB)
	}
}

// The filtered-list path (CTE + light/heavy scan) must carry the disk verdict,
// and the disk_blocked / disk_unknown filters must resolve on the node_snapshots
// column — independent of any target version / readiness rows.
func TestFunctional_NodeSnapshot_DiskFilter(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	org, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name:          "func-disk-filter-org",
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
	mk := func(name string, suff *bool) InsertNodeSnapshotParams {
		return InsertNodeSnapshotParams{
			CollectionRunOrg: run.OrganisationName, OrganisationName: org.Name,
			NodeName: name, Platform: "ubuntu", CollectedAt: now,
			SufficientDiskSpace: suff, RequiredDiskMB: ptrInt(2048),
		}
	}
	if _, err := db.BulkUpsertNodeSnapshots(ctx, []InsertNodeSnapshotParams{
		mk("ff-ok", ptrBool(true)), mk("ff-low", ptrBool(false)), mk("ff-unknown", nil),
	}); err != nil {
		t.Fatalf("bulk upsert: %v", err)
	}

	names := func(f NodeSnapshotFilter) []string {
		f.OrganisationNames = []string{org.Name}
		list, _, lerr := db.ListNodeSnapshotsFiltered(ctx, f)
		if lerr != nil {
			t.Fatalf("filtered list: %v", lerr)
		}
		var out []string
		for _, ns := range list {
			out = append(out, ns.NodeName)
		}
		return out
	}

	// Light scan carries the verdict (no IncludeHeavyJSON).
	all := names(NodeSnapshotFilter{})
	if len(all) != 3 {
		t.Fatalf("unfiltered list = %v, want 3 nodes", all)
	}
	// disk_blocked → only the insufficient node.
	if got := names(NodeSnapshotFilter{ReadinessFilter: "disk_blocked"}); len(got) != 1 || got[0] != "ff-low" {
		t.Errorf("disk_blocked = %v, want [ff-low]", got)
	}
	// disk_unknown → only the indeterminate node.
	if got := names(NodeSnapshotFilter{ReadinessFilter: "disk_unknown"}); len(got) != 1 || got[0] != "ff-unknown" {
		t.Errorf("disk_unknown = %v, want [ff-unknown]", got)
	}

	// Heavy scan path also carries the verdict in the returned snapshot.
	heavy, _, err := db.ListNodeSnapshotsFiltered(ctx, NodeSnapshotFilter{
		OrganisationNames: []string{org.Name}, ReadinessFilter: "disk_blocked", IncludeHeavyJSON: true,
	})
	if err != nil {
		t.Fatalf("heavy filtered list: %v", err)
	}
	if len(heavy) != 1 || heavy[0].SufficientDiskSpace == nil || *heavy[0].SufficientDiskSpace {
		t.Errorf("heavy disk_blocked verdict = %+v, want one node with sufficient=false", heavy)
	}
}
