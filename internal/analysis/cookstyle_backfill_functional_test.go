// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package analysis_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// backfillTestDB opens the test database and runs all migrations. Mirrors the
// datastore package's unexported testDB helper. Skips when CMM_TEST_DATABASE_URL
// is unset.
func backfillTestDB(t *testing.T) *datastore.DB {
	t.Helper()
	url := os.Getenv("CMM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CMM_TEST_DATABASE_URL not set — skipping functional test")
	}
	db, err := datastore.Open(url)
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.MigrateUp(context.Background(), "../../migrations"); err != nil {
		t.Fatalf("running migrations: %v", err)
	}
	return db
}

// TestFunctional_BackfillCookstyleStatus_RecoversNeedsReview is the end-to-end
// proof of the precise backfill: a result the coarse SQL migration left at
// "ready" (passed=true) but whose only offence is review-classified must be
// re-derived to needs_review by the Go path — a value SQL can never recover.
// Re-running is a no-op (idempotent).
func TestFunctional_BackfillCookstyleStatus_RecoversNeedsReview(t *testing.T) {
	db := backfillTestDB(t)
	ctx := context.Background()

	const org = "func-backfill-org"
	const cb = "func-backfill-cb"
	cleanup(t, db,
		"DELETE FROM server_cookbook_cookstyle_results WHERE cookbook_name = '"+cb+"'",
		"DELETE FROM server_cookbooks WHERE name = '"+cb+"'",
		"DELETE FROM organisations WHERE name = '"+org+"'",
	)

	if _, err := db.UpsertOrganisationFromConfig(ctx, datastore.UpsertOrganisationParams{
		Name: org, ChefServerURL: "https://chef.example.com", OrgName: org, ClientName: "c",
	}); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if _, err := db.UpsertServerCookbook(ctx, datastore.UpsertServerCookbookParams{
		OrganisationName: org, Name: cb, Version: "1.0.0", IsActive: true,
	}); err != nil {
		t.Fatalf("seed cookbook: %v", err)
	}

	// Seed the coarse state migration 0041 would produce: passed=true so the
	// SQL backfill set status='ready', but the stored offence is review-level.
	const reviewOffence = `[{"cop_name":"Chef/Correctness/NodeNormal","severity":"warning"}]`
	if _, err := db.UpsertServerCookbookCookstyleResult(ctx, datastore.UpsertServerCookbookCookstyleResultParams{
		OrganisationName: org, CookbookName: cb, CookbookVersion: "1.0.0", TargetChefVersion: "18",
		Passed: true, CookstyleStatus: "ready", Offences: []byte(reviewOffence),
		DurationSeconds: 1, ScannedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed coarse result: %v", err)
	}

	// Sanity: the coarse SQL value is the wrong one we are correcting.
	before, err := db.GetServerCookbookCookstyleResult(ctx, org, cb, "1.0.0", "18")
	if err != nil {
		t.Fatalf("get before: %v", err)
	}
	if before.CookstyleStatus != "ready" {
		t.Fatalf("precondition: coarse status = %q, want ready", before.CookstyleStatus)
	}

	res, err := analysis.BackfillCookstyleStatus(ctx, db, analysis.DefaultFailureRules())
	if err != nil {
		t.Fatalf("BackfillCookstyleStatus: %v", err)
	}
	if res.ServerResultsChanged < 1 {
		t.Fatalf("ServerResultsChanged = %d, want >= 1", res.ServerResultsChanged)
	}

	after, err := db.GetServerCookbookCookstyleResult(ctx, org, cb, "1.0.0", "18")
	if err != nil {
		t.Fatalf("get after: %v", err)
	}
	if after.CookstyleStatus != analysis.StatusNeedsReview {
		t.Errorf("after backfill status = %q, want %q", after.CookstyleStatus, analysis.StatusNeedsReview)
	}
	if !after.Passed {
		t.Error("passed must remain true for a needs_review row")
	}

	// Idempotent: a second pass leaves the now-precise row at needs_review.
	// (The global change count may be non-zero if other tests left coarse rows
	// in the shared DB, so the invariant is asserted at the row level.)
	if _, err := analysis.BackfillCookstyleStatus(ctx, db, analysis.DefaultFailureRules()); err != nil {
		t.Fatalf("second BackfillCookstyleStatus: %v", err)
	}
	again, err := db.GetServerCookbookCookstyleResult(ctx, org, cb, "1.0.0", "18")
	if err != nil {
		t.Fatalf("get after second pass: %v", err)
	}
	if again.CookstyleStatus != analysis.StatusNeedsReview {
		t.Errorf("second pass changed a precise row: status = %q, want %q", again.CookstyleStatus, analysis.StatusNeedsReview)
	}
}

// cleanup registers DELETE statements to run after the test.
func cleanup(t *testing.T, db *datastore.DB, queries ...string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		for _, q := range queries {
			if _, err := db.Pool().ExecContext(ctx, q); err != nil {
				t.Logf("cleanup query %q failed: %v", q, err)
			}
		}
	})
}
