// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// testDB opens a connection to the test database, runs all migrations, and
// returns a ready-to-use DB handle. The CMM_TEST_DATABASE_URL environment
// variable must be set to a PostgreSQL connection string. The test is
// skipped if the variable is not set.
//
// Example:
//
//	CMM_TEST_DATABASE_URL=postgres://user:pass@localhost:5432/cmm_test?sslmode=disable
func testDB(t *testing.T) *DB {
	t.Helper()

	url := os.Getenv("CMM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CMM_TEST_DATABASE_URL not set — skipping functional test")
	}

	db, err := Open(url)
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	applied, err := db.MigrateUp(context.Background(), "../../migrations")
	if err != nil {
		t.Fatalf("running migrations: %v", err)
	}
	if applied > 0 {
		t.Logf("applied %d migration(s)", applied)
	}

	return db
}

// cleanupTestData removes rows created during the test to avoid polluting
// other test runs. Call via t.Cleanup.
func cleanupTestData(t *testing.T, db *DB, queries ...string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		for _, q := range queries {
			if _, err := db.pool.ExecContext(ctx, q); err != nil {
				t.Logf("cleanup query %q failed: %v", q, err)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Migration 0007: driver + platform_name columns on git_repo_test_kitchen_results
// ---------------------------------------------------------------------------

func TestFunctional_DriverColumn_NullableOnInsert(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Create a git repo to satisfy the FK.
	repo, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{
		Name:       "func-test-driver-nullable",
		GitRepoURL: "https://example.com/func-test-driver-nullable.git",
	})
	if err != nil {
		t.Fatalf("creating git repo: %v", err)
	}
	cleanupTestData(t, db,
		"DELETE FROM git_repo_test_kitchen_results WHERE git_repo_id = '"+repo.ID+"'",
		"DELETE FROM git_repos WHERE id = '"+repo.ID+"'",
	)

	// Insert a TK result WITHOUT driver or platform_name (simulates
	// pre-migration rows).
	now := time.Now().UTC()
	result, err := db.UpsertGitRepoTestKitchenResult(ctx, UpsertGitRepoTestKitchenResultParams{
		GitRepoID:         repo.ID,
		TargetChefVersion: "18.0.0",
		CommitSHA:         "abc123",
		ConvergePassed:    true,
		TestsPassed:       true,
		Compatible:        true,
		StartedAt:         now,
		CompletedAt:       now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("upserting TK result: %v", err)
	}

	// driver and platform_tested should be empty strings (Go zero value
	// from NULL column).
	if result.DriverUsed != "" {
		t.Errorf("DriverUsed: got %q, want empty (NULL)", result.DriverUsed)
	}
	if result.PlatformTested != "" {
		t.Errorf("PlatformTested: got %q, want empty (NULL)", result.PlatformTested)
	}
}

func TestFunctional_DriverColumn_RoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	repo, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{
		Name:       "func-test-driver-roundtrip",
		GitRepoURL: "https://example.com/func-test-driver-roundtrip.git",
	})
	if err != nil {
		t.Fatalf("creating git repo: %v", err)
	}
	cleanupTestData(t, db,
		"DELETE FROM git_repo_test_kitchen_results WHERE git_repo_id = '"+repo.ID+"'",
		"DELETE FROM git_repos WHERE id = '"+repo.ID+"'",
	)

	now := time.Now().UTC()
	result, err := db.UpsertGitRepoTestKitchenResult(ctx, UpsertGitRepoTestKitchenResultParams{
		GitRepoID:         repo.ID,
		TargetChefVersion: "18.0.0",
		CommitSHA:         "def456",
		ConvergePassed:    true,
		TestsPassed:       true,
		Compatible:        true,
		DriverUsed:        "vcenter",
		PlatformTested:    "centos-7",
		OverridesApplied:  true,
		DurationSeconds:   120,
		StartedAt:         now,
		CompletedAt:       now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("upserting TK result: %v", err)
	}

	if result.DriverUsed != "vcenter" {
		t.Errorf("DriverUsed: got %q, want %q", result.DriverUsed, "vcenter")
	}
	if result.PlatformTested != "centos-7" {
		t.Errorf("PlatformTested: got %q, want %q", result.PlatformTested, "centos-7")
	}

	// Re-read from DB to confirm persistence.
	fetched, err := db.GetGitRepoTestKitchenResultByID(ctx, result.ID)
	if err != nil {
		t.Fatalf("re-reading TK result: %v", err)
	}
	if fetched.DriverUsed != "vcenter" {
		t.Errorf("re-read DriverUsed: got %q, want %q", fetched.DriverUsed, "vcenter")
	}
	if fetched.PlatformTested != "centos-7" {
		t.Errorf("re-read PlatformTested: got %q, want %q", fetched.PlatformTested, "centos-7")
	}
}

func TestFunctional_DriverColumn_UpdatePreservesValues(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	repo, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{
		Name:       "func-test-driver-update",
		GitRepoURL: "https://example.com/func-test-driver-update.git",
	})
	if err != nil {
		t.Fatalf("creating git repo: %v", err)
	}
	cleanupTestData(t, db,
		"DELETE FROM git_repo_test_kitchen_results WHERE git_repo_id = '"+repo.ID+"'",
		"DELETE FROM git_repos WHERE id = '"+repo.ID+"'",
	)

	now := time.Now().UTC()

	// First insert with dokken.
	_, err = db.UpsertGitRepoTestKitchenResult(ctx, UpsertGitRepoTestKitchenResultParams{
		GitRepoID:         repo.ID,
		TargetChefVersion: "18.0.0",
		CommitSHA:         "aaa111",
		DriverUsed:        "dokken",
		PlatformTested:    "ubuntu-22.04",
		StartedAt:         now,
		CompletedAt:       now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Upsert again with different driver (conflict on git_repo_id +
	// target_chef_version triggers UPDATE).
	updated, err := db.UpsertGitRepoTestKitchenResult(ctx, UpsertGitRepoTestKitchenResultParams{
		GitRepoID:         repo.ID,
		TargetChefVersion: "18.0.0",
		CommitSHA:         "bbb222",
		DriverUsed:        "ec2",
		PlatformTested:    "rhel-9",
		ConvergePassed:    true,
		Compatible:        true,
		StartedAt:         now.Add(time.Hour),
		CompletedAt:       now.Add(time.Hour + time.Minute),
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if updated.DriverUsed != "ec2" {
		t.Errorf("updated DriverUsed: got %q, want %q", updated.DriverUsed, "ec2")
	}
	if updated.PlatformTested != "rhel-9" {
		t.Errorf("updated PlatformTested: got %q, want %q", updated.PlatformTested, "rhel-9")
	}
	if updated.CommitSHA != "bbb222" {
		t.Errorf("updated CommitSHA: got %q, want %q", updated.CommitSHA, "bbb222")
	}
}

// ---------------------------------------------------------------------------
// Migration 0008: cookbook_platform_coverage table
// ---------------------------------------------------------------------------

func TestFunctional_CookbookPlatformCoverage_InsertAndGet(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM cookbook_platform_coverage WHERE cookbook_name = 'func-test-coverage-insert'",
	)

	coverageData := map[string]any{
		"kitchen_platforms":        []string{"ubuntu-22.04", "centos-7"},
		"production_platforms":     []any{},
		"tested_and_in_production": []any{},
		"tested_not_in_production": []string{},
		"in_production_not_tested": []any{},
		"gap_count":                0,
		"total_production_nodes":   0,
		"covered_node_count":       0,
		"coverage_percentage":      0.0,
	}

	result, err := db.UpsertCookbookPlatformCoverage(ctx, UpsertCookbookPlatformCoverageParams{
		CookbookName: "func-test-coverage-insert",
		CoverageData: coverageData,
	})
	if err != nil {
		t.Fatalf("upserting coverage: %v", err)
	}

	if result.CookbookName != "func-test-coverage-insert" {
		t.Errorf("CookbookName: got %q, want %q", result.CookbookName, "func-test-coverage-insert")
	}
	if result.ID == "" {
		t.Error("ID should not be empty")
	}
	if result.EvaluatedAt.IsZero() {
		t.Error("EvaluatedAt should not be zero")
	}

	// Re-read.
	fetched, err := db.GetCookbookPlatformCoverage(ctx, "func-test-coverage-insert")
	if err != nil {
		t.Fatalf("getting coverage: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected non-nil coverage row")
	}
	if fetched.CookbookName != "func-test-coverage-insert" {
		t.Errorf("fetched CookbookName: got %q, want %q", fetched.CookbookName, "func-test-coverage-insert")
	}
}

func TestFunctional_CookbookPlatformCoverage_UpsertUpdatesExisting(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM cookbook_platform_coverage WHERE cookbook_name = 'func-test-coverage-upsert'",
	)

	// First insert.
	first, err := db.UpsertCookbookPlatformCoverage(ctx, UpsertCookbookPlatformCoverageParams{
		CookbookName: "func-test-coverage-upsert",
		CoverageData: map[string]any{"gap_count": 0},
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Second upsert — same cookbook_name triggers ON CONFLICT UPDATE.
	second, err := db.UpsertCookbookPlatformCoverage(ctx, UpsertCookbookPlatformCoverageParams{
		CookbookName: "func-test-coverage-upsert",
		CoverageData: map[string]any{"gap_count": 3, "coverage_percentage": 75.5},
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	// Same row, same ID.
	if second.ID != first.ID {
		t.Errorf("expected same ID after upsert: first=%s, second=%s", first.ID, second.ID)
	}

	// Coverage data should reflect the second write.
	dataJSON, err := json.Marshal(second.CoverageData)
	if err != nil {
		t.Fatalf("marshalling coverage_data: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(dataJSON, &parsed); err != nil {
		t.Fatalf("unmarshalling coverage_data: %v", err)
	}
	if gapCount, ok := parsed["gap_count"].(float64); !ok || int(gapCount) != 3 {
		t.Errorf("gap_count: got %v, want 3", parsed["gap_count"])
	}
}

func TestFunctional_CookbookPlatformCoverage_CoverageDataJSONB(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM cookbook_platform_coverage WHERE cookbook_name = 'func-test-coverage-jsonb'",
	)

	// Insert a realistic coverage report.
	report := map[string]any{
		"kitchen_platforms": []string{"ubuntu-22.04", "centos-7"},
		"production_platforms": []map[string]any{
			{"platform": "ubuntu", "platform_version": "22.04", "platform_family": "debian", "node_count": 47},
			{"platform": "centos", "platform_version": "7.9.2009", "platform_family": "rhel", "node_count": 12},
			{"platform": "rocky", "platform_version": "9.3", "platform_family": "rhel", "node_count": 8},
		},
		"tested_and_in_production": []map[string]any{
			{"kitchen_name": "ubuntu-22.04", "platform": "ubuntu", "platform_version": "22.04", "node_count": 47},
			{"kitchen_name": "centos-7", "platform": "centos", "platform_version": "7.9.2009", "node_count": 12},
		},
		"tested_not_in_production": []string{},
		"in_production_not_tested": []map[string]any{
			{"platform": "rocky", "platform_version": "9.3", "platform_family": "rhel", "node_count": 8},
		},
		"gap_count":              1,
		"total_production_nodes": 67,
		"covered_node_count":     59,
		"coverage_percentage":    88.1,
	}

	_, err := db.UpsertCookbookPlatformCoverage(ctx, UpsertCookbookPlatformCoverageParams{
		CookbookName: "func-test-coverage-jsonb",
		CoverageData: report,
	})
	if err != nil {
		t.Fatalf("upserting coverage: %v", err)
	}

	// Re-read and verify JSONB round-trip.
	fetched, err := db.GetCookbookPlatformCoverage(ctx, "func-test-coverage-jsonb")
	if err != nil {
		t.Fatalf("getting coverage: %v", err)
	}

	dataJSON, err := json.Marshal(fetched.CoverageData)
	if err != nil {
		t.Fatalf("marshalling fetched coverage_data: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(dataJSON, &parsed); err != nil {
		t.Fatalf("unmarshalling coverage_data: %v", err)
	}

	// Verify key fields survived the JSONB round-trip.
	if gapCount, ok := parsed["gap_count"].(float64); !ok || int(gapCount) != 1 {
		t.Errorf("gap_count: got %v, want 1", parsed["gap_count"])
	}
	if coverage, ok := parsed["coverage_percentage"].(float64); !ok || coverage != 88.1 {
		t.Errorf("coverage_percentage: got %v, want 88.1", parsed["coverage_percentage"])
	}
	if totalNodes, ok := parsed["total_production_nodes"].(float64); !ok || int(totalNodes) != 67 {
		t.Errorf("total_production_nodes: got %v, want 67", parsed["total_production_nodes"])
	}

	kitchenPlatforms, ok := parsed["kitchen_platforms"].([]any)
	if !ok {
		t.Fatalf("kitchen_platforms: expected array, got %T", parsed["kitchen_platforms"])
	}
	if len(kitchenPlatforms) != 2 {
		t.Errorf("kitchen_platforms length: got %d, want 2", len(kitchenPlatforms))
	}
}

func TestFunctional_CookbookPlatformCoverage_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	fetched, err := db.GetCookbookPlatformCoverage(ctx, "func-test-nonexistent-cookbook-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fetched != nil {
		t.Errorf("expected nil for nonexistent cookbook, got %+v", fetched)
	}
}

func TestFunctional_CookbookPlatformCoverage_Delete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM cookbook_platform_coverage WHERE cookbook_name = 'func-test-coverage-delete'",
	)

	_, err := db.UpsertCookbookPlatformCoverage(ctx, UpsertCookbookPlatformCoverageParams{
		CookbookName: "func-test-coverage-delete",
		CoverageData: map[string]any{"gap_count": 0},
	})
	if err != nil {
		t.Fatalf("upserting coverage: %v", err)
	}

	// Delete.
	if err := db.DeleteCookbookPlatformCoverage(ctx, "func-test-coverage-delete"); err != nil {
		t.Fatalf("deleting coverage: %v", err)
	}

	// Verify gone.
	fetched, err := db.GetCookbookPlatformCoverage(ctx, "func-test-coverage-delete")
	if err != nil {
		t.Fatalf("unexpected error after delete: %v", err)
	}
	if fetched != nil {
		t.Error("expected nil after delete")
	}
}

func TestFunctional_CookbookPlatformCoverage_DeleteNotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	err := db.DeleteCookbookPlatformCoverage(ctx, "func-test-nonexistent-delete-xyz")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFunctional_CookbookPlatformCoverage_List(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM cookbook_platform_coverage WHERE cookbook_name LIKE 'func-test-coverage-list-%'",
	)

	// Insert two rows.
	for _, name := range []string{"func-test-coverage-list-alpha", "func-test-coverage-list-beta"} {
		_, err := db.UpsertCookbookPlatformCoverage(ctx, UpsertCookbookPlatformCoverageParams{
			CookbookName: name,
			CoverageData: map[string]any{"gap_count": 0},
		})
		if err != nil {
			t.Fatalf("upserting %s: %v", name, err)
		}
	}

	all, err := db.ListCookbookPlatformCoverages(ctx)
	if err != nil {
		t.Fatalf("listing coverages: %v", err)
	}

	// We may have rows from other tests, so just verify our two exist.
	found := map[string]bool{}
	for _, c := range all {
		if c.CookbookName == "func-test-coverage-list-alpha" || c.CookbookName == "func-test-coverage-list-beta" {
			found[c.CookbookName] = true
		}
	}
	if len(found) != 2 {
		t.Errorf("expected 2 func-test rows in list, found %d", len(found))
	}
}

func TestFunctional_CookbookPlatformCoverage_WithGitRepoFK(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	repo, err := db.UpsertGitRepo(ctx, UpsertGitRepoParams{
		Name:       "func-test-coverage-fk",
		GitRepoURL: "https://example.com/func-test-coverage-fk.git",
	})
	if err != nil {
		t.Fatalf("creating git repo: %v", err)
	}
	cleanupTestData(t, db,
		"DELETE FROM cookbook_platform_coverage WHERE cookbook_name = 'func-test-coverage-fk'",
		"DELETE FROM git_repos WHERE id = '"+repo.ID+"'",
	)

	result, err := db.UpsertCookbookPlatformCoverage(ctx, UpsertCookbookPlatformCoverageParams{
		GitRepoID:    repo.ID,
		CookbookName: "func-test-coverage-fk",
		CoverageData: map[string]any{"gap_count": 2},
	})
	if err != nil {
		t.Fatalf("upserting coverage with FK: %v", err)
	}
	if result.GitRepoID != repo.ID {
		t.Errorf("GitRepoID: got %q, want %q", result.GitRepoID, repo.ID)
	}

	// Verify FK cascade: deleting the git repo should cascade-delete coverage.
	if err := db.DeleteGitRepo(ctx, repo.ID); err != nil {
		t.Fatalf("deleting git repo: %v", err)
	}
	fetched, err := db.GetCookbookPlatformCoverage(ctx, "func-test-coverage-fk")
	if err != nil {
		t.Fatalf("unexpected error after cascade: %v", err)
	}
	if fetched != nil {
		t.Error("expected nil after cascade delete of git repo")
	}
}

func TestFunctional_CookbookPlatformCoverage_UniqueConstraint(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM cookbook_platform_coverage WHERE cookbook_name = 'func-test-coverage-unique'",
	)

	// First insert.
	first, err := db.UpsertCookbookPlatformCoverage(ctx, UpsertCookbookPlatformCoverageParams{
		CookbookName: "func-test-coverage-unique",
		CoverageData: map[string]any{"gap_count": 1},
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	// Second upsert with same cookbook_name — should update, not fail.
	second, err := db.UpsertCookbookPlatformCoverage(ctx, UpsertCookbookPlatformCoverageParams{
		CookbookName: "func-test-coverage-unique",
		CoverageData: map[string]any{"gap_count": 5},
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	if second.ID != first.ID {
		t.Errorf("upsert should reuse ID: first=%s, second=%s", first.ID, second.ID)
	}

	// updated_at should be >= first insert.
	if second.UpdatedAt.Before(first.CreatedAt) {
		t.Errorf("updated_at (%v) should not be before created_at (%v)", second.UpdatedAt, first.CreatedAt)
	}
}

// ---------------------------------------------------------------------------
// GetProductionPlatformsForCookbook (requires node_snapshots with cookbook data)
// ---------------------------------------------------------------------------

func TestFunctional_GetProductionPlatformsForCookbook(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// We need an organisation and a collection run for the FK constraints
	// on node_snapshots.
	org, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name:          "func-test-coverage-org",
		ChefServerURL: "https://example.com/organizations/test",
		OrgName:       "test",
		ClientName:    "test-client",
	})
	if err != nil {
		t.Fatalf("creating org: %v", err)
	}

	run, err := db.CreateCollectionRun(ctx, CreateCollectionRunParams{
		OrganisationID: org.Name,
	})
	if err != nil {
		t.Fatalf("creating collection run: %v", err)
	}

	cleanupTestData(t, db,
		"DELETE FROM node_snapshots WHERE collection_run_id = '"+run.ID+"'",
		"DELETE FROM collection_runs WHERE id = '"+run.ID+"'",
		"DELETE FROM organisations WHERE id = '"+org.Name+"'",
	)

	// Insert node snapshots with cookbook data. The cookbooks column is
	// JSONB in the format: {"cookbook_name": {"version": "x.y.z"}, ...}
	now := time.Now().UTC()
	cookbooksWithTarget, _ := json.Marshal(map[string]any{
		"func-test-cb": map[string]string{"version": "1.0.0"},
		"ntp":          map[string]string{"version": "3.0.0"},
	})
	cookbooksWithout, _ := json.Marshal(map[string]any{
		"ntp": map[string]string{"version": "3.0.0"},
	})

	nodes := []InsertNodeSnapshotParams{
		{
			CollectionRunID: run.ID, OrganisationID: org.Name,
			NodeName: "func-web1", Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian",
			Cookbooks: cookbooksWithTarget, CollectedAt: now,
		},
		{
			CollectionRunID: run.ID, OrganisationID: org.Name,
			NodeName: "func-web2", Platform: "ubuntu", PlatformVersion: "22.04", PlatformFamily: "debian",
			Cookbooks: cookbooksWithTarget, CollectedAt: now,
		},
		{
			CollectionRunID: run.ID, OrganisationID: org.Name,
			NodeName: "func-db1", Platform: "centos", PlatformVersion: "7.9.2009", PlatformFamily: "rhel",
			Cookbooks: cookbooksWithTarget, CollectedAt: now,
		},
		{
			// This node does NOT have func-test-cb — should not appear.
			CollectionRunID: run.ID, OrganisationID: org.Name,
			NodeName: "func-other1", Platform: "rocky", PlatformVersion: "9.3", PlatformFamily: "rhel",
			Cookbooks: cookbooksWithout, CollectedAt: now,
		},
	}

	if _, err := db.BulkUpsertNodeSnapshots(ctx, nodes); err != nil {
		t.Fatalf("inserting node snapshots: %v", err)
	}

	// Query production platforms for func-test-cb.
	rows, err := db.GetProductionPlatformsForCookbook(ctx, "func-test-cb")
	if err != nil {
		t.Fatalf("querying production platforms: %v", err)
	}

	// Expect 2 groups: ubuntu/22.04 (2 nodes) and centos/7.9.2009 (1 node).
	if len(rows) != 2 {
		t.Fatalf("expected 2 platform groups, got %d: %+v", len(rows), rows)
	}

	// Results are ordered by node_count DESC.
	if rows[0].Platform != "ubuntu" || rows[0].PlatformVersion != "22.04" {
		t.Errorf("rows[0]: got %s/%s, want ubuntu/22.04", rows[0].Platform, rows[0].PlatformVersion)
	}
	if rows[0].PlatformFamily != "debian" {
		t.Errorf("rows[0].PlatformFamily: got %q, want %q", rows[0].PlatformFamily, "debian")
	}
	if rows[0].NodeCount != 2 {
		t.Errorf("rows[0].NodeCount: got %d, want 2", rows[0].NodeCount)
	}

	if rows[1].Platform != "centos" || rows[1].PlatformVersion != "7.9.2009" {
		t.Errorf("rows[1]: got %s/%s, want centos/7.9.2009", rows[1].Platform, rows[1].PlatformVersion)
	}
	if rows[1].NodeCount != 1 {
		t.Errorf("rows[1].NodeCount: got %d, want 1", rows[1].NodeCount)
	}
}

func TestFunctional_GetProductionPlatformsForCookbook_NoCookbook(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	rows, err := db.GetProductionPlatformsForCookbook(ctx, "func-test-nonexistent-cookbook-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty slice, got %d rows", len(rows))
	}
	if rows == nil {
		t.Error("expected non-nil empty slice")
	}
}

func TestFunctional_GetProductionPlatformsForCookbook_EmptyName(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetProductionPlatformsForCookbook(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty cookbook name")
	}
}
