// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ingest"
)

// Far-future dates keep this test's partitions clear of any real/other-test data
// in the shared cmm_test DB, and are dropped in cleanup.
var (
	convD1 = time.Date(2040, 1, 1, 9, 0, 0, 0, time.UTC)
	convD2 = time.Date(2040, 1, 2, 9, 0, 0, 0, time.UTC)
)

func run(id string, end time.Time, status string) ingest.ConvergeRun {
	return ingest.ConvergeRun{
		RunID:        id,
		Organisation: "org-store-test",
		NodeName:     "node-store-test.example.com",
		Status:       status,
		ChefVersion:  "18.9.4",
		StartTime:    end.Add(-time.Minute),
		EndTime:      end,
		RunList:      []string{"recipe[base::default]"},
		Cookbooks:    map[string]string{"base": "1.2.0"},
		Shape:        ingest.ShapeConverge,
	}
}

func countConvergeRuns(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	err := db.pool.QueryRowContext(context.Background(),
		`SELECT count(*) FROM converge_runs WHERE organisation = 'org-store-test'`).Scan(&n)
	if err != nil {
		t.Fatalf("counting converge_runs: %v", err)
	}
	return n
}

func TestFunctional_ConvergeRuns_UpsertDedupAndRetention(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() {
		db.pool.ExecContext(ctx, `DROP TABLE IF EXISTS converge_runs_20400101`)
		db.pool.ExecContext(ctx, `DROP TABLE IF EXISTS converge_runs_20400102`)
	})

	// Two distinct runs on day 1 → 2 inserted, partition auto-created.
	inserted, err := db.BulkUpsertConvergeRuns(ctx, []ingest.ConvergeRun{
		run("store-r1", convD1, "success"),
		run("store-r2", convD1, "failure"),
	})
	if err != nil {
		t.Fatalf("BulkUpsertConvergeRuns: %v", err)
	}
	if inserted != 2 {
		t.Errorf("inserted = %d, want 2", inserted)
	}
	if got := countConvergeRuns(t, db); got != 2 {
		t.Fatalf("row count = %d, want 2", got)
	}

	// Same (run_id, end_time) again → deduped, 0 inserted, count unchanged.
	inserted, err = db.BulkUpsertConvergeRuns(ctx, []ingest.ConvergeRun{
		run("store-r1", convD1, "success"),
	})
	if err != nil {
		t.Fatalf("re-insert: %v", err)
	}
	if inserted != 0 {
		t.Errorf("re-insert inserted = %d, want 0 (deduped)", inserted)
	}
	if got := countConvergeRuns(t, db); got != 2 {
		t.Errorf("row count after dedup = %d, want 2", got)
	}

	// A run on day 2 → new partition, count 3.
	if _, err := db.BulkUpsertConvergeRuns(ctx, []ingest.ConvergeRun{run("store-r3", convD2, "success")}); err != nil {
		t.Fatalf("day-2 insert: %v", err)
	}
	if got := countConvergeRuns(t, db); got != 3 {
		t.Fatalf("row count = %d, want 3", got)
	}

	// Purge everything strictly before day 2 00:00Z → drops the whole day-1
	// partition (upper bound == cutoff), keeps day 2.
	cutoff := time.Date(2040, 1, 2, 0, 0, 0, 0, time.UTC)
	dropped, err := db.PurgeConvergeRunPartitions(ctx, cutoff)
	if err != nil {
		t.Fatalf("PurgeConvergeRunPartitions: %v", err)
	}
	if dropped < 1 {
		t.Errorf("dropped = %d, want >= 1 (day-1 partition)", dropped)
	}
	if got := countConvergeRuns(t, db); got != 1 {
		t.Errorf("row count after purge = %d, want 1 (only day-2 survives)", got)
	}
}

// The read path returns a node's runs most-recent-first with failure detail
// passed through verbatim.
func TestFunctional_ConvergeRuns_ListForNode(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() { db.pool.ExecContext(ctx, `DROP TABLE IF EXISTS converge_runs_20400101`) })

	early := run("list-r1", convD1, "success")
	late := run("list-r2", convD1.Add(30*time.Minute), "failure")
	late.Error = &ingest.RunError{Class: "RuntimeError", Message: "boom", Backtrace: []string{"a", "b"}}
	if _, err := db.BulkUpsertConvergeRuns(ctx, []ingest.ConvergeRun{early, late}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := db.ListConvergeRunsForNode(ctx, "org-store-test", "node-store-test.example.com", 10)
	if err != nil {
		t.Fatalf("ListConvergeRunsForNode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d runs, want 2", len(got))
	}
	if got[0].RunID != "list-r2" {
		t.Errorf("first run = %q, want list-r2 (most recent first)", got[0].RunID)
	}
	if got[0].Status != "failure" || len(got[0].Error) == 0 {
		t.Errorf("first run failure detail missing: status=%q error=%s", got[0].Status, got[0].Error)
	}
	if got[0].Cookbooks["base"] != "1.2.0" {
		t.Errorf("cookbooks = %v, want base=1.2.0", got[0].Cookbooks)
	}
	// A node with no runs returns empty, not an error.
	none, err := db.ListConvergeRunsForNode(ctx, "org-store-test", "nonexistent", 10)
	if err != nil || len(none) != 0 {
		t.Errorf("empty case = (%v, %v), want (nil, empty)", none, err)
	}
}

// Failure detail (error + failing resource) must round-trip through the JSONB
// columns intact — it is the feature's whole point.
func TestFunctional_ConvergeRuns_FailureRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() { db.pool.ExecContext(ctx, `DROP TABLE IF EXISTS converge_runs_20400101`) })

	fail := run("store-fail", convD1, "failure")
	fail.Error = &ingest.RunError{
		Class:     "RuntimeError",
		Message:   "boom from failcb",
		Backtrace: []string{"a.rb:3", "b.rb:41"},
	}
	fail.FailedResource = &ingest.FailedResource{
		CookbookName: "failcb", RecipeName: "default", Name: "explode", Type: "ruby_block",
	}
	if _, err := db.BulkUpsertConvergeRuns(ctx, []ingest.ConvergeRun{fail}); err != nil {
		t.Fatalf("insert failure run: %v", err)
	}

	var class, resType string
	var backtraceLen int
	err := db.pool.QueryRowContext(ctx, `
		SELECT error->>'class',
		       failed_resource->>'type',
		       jsonb_array_length(error->'backtrace')
		FROM converge_runs WHERE run_id = 'store-fail'`).Scan(&class, &resType, &backtraceLen)
	if err != nil {
		t.Fatalf("reading back failure detail: %v", err)
	}
	if class != "RuntimeError" {
		t.Errorf("error.class = %q, want RuntimeError", class)
	}
	if resType != "ruby_block" {
		t.Errorf("failed_resource.type = %q, want ruby_block", resType)
	}
	if backtraceLen != 2 {
		t.Errorf("backtrace length = %d, want 2", backtraceLen)
	}
}
