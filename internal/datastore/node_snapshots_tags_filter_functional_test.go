// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"sort"
	"testing"
	"time"
)

// The tags filter (TEXT[] && array-overlap, OR semantics) and the count-ranked
// tags facet (ListDistinctNodeTags) must resolve against the real tags column.
// Mock-level tests cannot exercise the SQL, so this covers both end to end.
func TestFunctional_NodeSnapshot_TagsFilterAndFacet(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	org, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name:          "func-tags-filter-org",
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
	mk := func(name string, tags []string) InsertNodeSnapshotParams {
		return InsertNodeSnapshotParams{
			CollectionRunOrg: run.OrganisationName, OrganisationName: org.Name,
			NodeName: name, Platform: "ubuntu", CollectedAt: now, Tags: tags,
		}
	}
	if _, err := db.BulkUpsertNodeSnapshots(ctx, []InsertNodeSnapshotParams{
		mk("n-prepare", []string{"prepare", "eu-west"}),
		mk("n-upgrade", []string{"upgrade", "eu-west"}),
		mk("n-rollback", []string{"rollback"}),
		mk("n-none", []string{}),
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
		sort.Strings(out)
		return out
	}

	// Single tag → exact overlap.
	if got := names(NodeSnapshotFilter{Tags: []string{"prepare"}}); len(got) != 1 || got[0] != "n-prepare" {
		t.Errorf("Tags=[prepare] = %v, want [n-prepare]", got)
	}
	// OR semantics — union across selected tags.
	if got := names(NodeSnapshotFilter{Tags: []string{"prepare", "rollback"}}); len(got) != 2 ||
		got[0] != "n-prepare" || got[1] != "n-rollback" {
		t.Errorf("Tags=[prepare,rollback] = %v, want [n-prepare n-rollback]", got)
	}
	// A shared tag matches every node carrying it.
	if got := names(NodeSnapshotFilter{Tags: []string{"eu-west"}}); len(got) != 2 ||
		got[0] != "n-prepare" || got[1] != "n-upgrade" {
		t.Errorf("Tags=[eu-west] = %v, want [n-prepare n-upgrade]", got)
	}
	// A tag on no node → no matches (and the untagged node never matches).
	if got := names(NodeSnapshotFilter{Tags: []string{"absent"}}); len(got) != 0 {
		t.Errorf("Tags=[absent] = %v, want []", got)
	}

	// Facet: count-ranked (eu-west appears on 2 nodes, so it ranks first),
	// ties broken alphabetically.
	f := NodeSnapshotFilter{OrganisationNames: []string{org.Name}}
	all, err := db.ListDistinctNodeTags(ctx, f, DistinctValueOpts{Limit: 50})
	if err != nil {
		t.Fatalf("facet: %v", err)
	}
	if len(all) != 4 || all[0] != "eu-west" {
		t.Errorf("facet = %v, want eu-west first (count-ranked), 4 distinct tags", all)
	}

	// Prefix filter for typeahead.
	pre, err := db.ListDistinctNodeTags(ctx, f, DistinctValueOpts{SearchPrefix: "up", Limit: 50})
	if err != nil {
		t.Fatalf("facet prefix: %v", err)
	}
	if len(pre) != 1 || pre[0] != "upgrade" {
		t.Errorf("facet prefix 'up' = %v, want [upgrade]", pre)
	}

	// Cap is enforced.
	capped, err := db.ListDistinctNodeTags(ctx, f, DistinctValueOpts{Limit: 2})
	if err != nil {
		t.Fatalf("facet cap: %v", err)
	}
	if len(capped) != 2 {
		t.Errorf("facet Limit=2 = %v, want 2 values", capped)
	}
}
