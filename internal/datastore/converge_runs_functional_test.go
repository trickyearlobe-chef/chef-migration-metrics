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

// runNode is like run() but lets a test vary the node name and org so the
// run-centric list view can be exercised across nodes/orgs (incl. an
// ingest-only org that has no node_snapshots — the DMZ case).
func runNode(id, org, node string, end time.Time, status, chefVersion string) ingest.ConvergeRun {
	r := run(id, end, status)
	r.Organisation = org
	r.NodeName = node
	r.ChefVersion = chefVersion
	return r
}

// The run-centric list view filters and paginates across nodes and orgs,
// reading converge_runs directly — including an ingest-only org with no
// node_snapshots (the DMZ population the view exists to surface).
func TestFunctional_ConvergeRuns_ListFiltered(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() { db.pool.ExecContext(ctx, `DROP TABLE IF EXISTS converge_runs_20400101`) })

	seed := []ingest.ConvergeRun{
		runNode("f-r1", "org-store-test", "web01.example.com", convD1, "success", "18.9.4"),
		runNode("f-r2", "org-store-test", "web02.example.com", convD1.Add(10*time.Minute), "failure", "18.9.4"),
		// A DMZ ingest-only org (never pulled → no node_snapshots).
		runNode("f-r3", "dmz-org-store-test", "dmz01.example.com", convD1.Add(20*time.Minute), "failure", "19.0.12"),
		runNode("f-r4", "dmz-org-store-test", "dmz02.example.com", convD1.Add(30*time.Minute), "success", "19.0.12"),
	}
	if _, err := db.BulkUpsertConvergeRuns(ctx, seed); err != nil {
		t.Fatalf("seed insert: %v", err)
	}
	t.Cleanup(func() {
		db.pool.ExecContext(ctx, `DELETE FROM converge_runs WHERE organisation IN ('org-store-test','dmz-org-store-test')`)
	})

	// Default (no filters here, but bound to our two test orgs via org filter one at a time).
	// Status filter: all failures across both orgs.
	fails, total, err := db.ListConvergeRunsFiltered(ctx, ConvergeRunFilter{Status: "failure", NodeName: ""})
	if err != nil {
		t.Fatalf("ListConvergeRunsFiltered(status=failure): %v", err)
	}
	// Filter may catch unrelated rows in the shared DB — assert our two are present.
	seenFail := map[string]bool{}
	for _, r := range fails {
		seenFail[r.RunID] = true
	}
	if !seenFail["f-r2"] || !seenFail["f-r3"] {
		t.Errorf("status=failure missing f-r2/f-r3; got %v (total=%d)", seenFail, total)
	}
	if seenFail["f-r1"] || seenFail["f-r4"] {
		t.Errorf("status=failure should exclude successes f-r1/f-r4; got %v", seenFail)
	}

	// Org filter surfaces the DMZ ingest-only org (no node_snapshots needed).
	dmz, total, err := db.ListConvergeRunsFiltered(ctx, ConvergeRunFilter{Organisation: "dmz-org-store-test"})
	if err != nil {
		t.Fatalf("ListConvergeRunsFiltered(org=dmz): %v", err)
	}
	if total != 2 || len(dmz) != 2 {
		t.Errorf("dmz org list = %d rows (total %d), want 2/2", len(dmz), total)
	}
	// Default sort is recency DESC → f-r4 (latest) first.
	if dmz[0].RunID != "f-r4" {
		t.Errorf("dmz first row = %q, want f-r4 (recency DESC)", dmz[0].RunID)
	}
	if dmz[0].NodeName == "" || dmz[0].Organisation != "dmz-org-store-test" {
		t.Errorf("list item missing org/node: %+v", dmz[0])
	}

	// chef_version discriminator (the CC19 target-version signal): 19.0.12 ∧ failure.
	cc, _, err := db.ListConvergeRunsFiltered(ctx, ConvergeRunFilter{ChefVersion: "19.0.12", Status: "failure"})
	if err != nil {
		t.Fatalf("ListConvergeRunsFiltered(cc19): %v", err)
	}
	if len(cc) != 1 || cc[0].RunID != "f-r3" {
		t.Errorf("cc19 failure filter = %v, want [f-r3]", runIDs(cc))
	}

	// Node substring.
	web, _, err := db.ListConvergeRunsFiltered(ctx, ConvergeRunFilter{Organisation: "org-store-test", NodeName: "web01"})
	if err != nil {
		t.Fatalf("ListConvergeRunsFiltered(node=web01): %v", err)
	}
	if len(web) != 1 || web[0].RunID != "f-r1" {
		t.Errorf("node substring = %v, want [f-r1]", runIDs(web))
	}

	// Pagination: per-org total is exact even when a page is smaller.
	page1, total, err := db.ListConvergeRunsFiltered(ctx, ConvergeRunFilter{Organisation: "dmz-org-store-test", Limit: 1, Offset: 0})
	if err != nil {
		t.Fatalf("paginated list: %v", err)
	}
	if len(page1) != 1 || total != 2 {
		t.Errorf("page1 = %d rows total %d, want 1 row total 2", len(page1), total)
	}
}

func runIDs(rows []ConvergeRunListItem) []string {
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].RunID
	}
	return ids
}

// The org options come from converge_runs itself (DISTINCT), NOT the
// organisations table — so DMZ ingest-only orgs are selectable.
func TestFunctional_ConvergeRuns_ListOrganisations(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() {
		db.pool.ExecContext(ctx, `DROP TABLE IF EXISTS converge_runs_20400101`)
		db.pool.ExecContext(ctx, `DELETE FROM converge_runs WHERE organisation IN ('org-store-test','dmz-org-store-test')`)
	})
	if _, err := db.BulkUpsertConvergeRuns(ctx, []ingest.ConvergeRun{
		runNode("o-r1", "org-store-test", "n1.example.com", convD1, "success", "18.9.4"),
		runNode("o-r2", "dmz-org-store-test", "n2.example.com", convD1, "failure", "19.0.12"),
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	orgs, err := db.ListConvergeRunOrganisations(ctx)
	if err != nil {
		t.Fatalf("ListConvergeRunOrganisations: %v", err)
	}
	seen := map[string]bool{}
	for _, o := range orgs {
		seen[o] = true
	}
	if !seen["org-store-test"] || !seen["dmz-org-store-test"] {
		t.Errorf("org options missing test orgs; got %v", orgs)
	}
}

// The node rollup (top-level Nodes tab) uses EXISTS / "any matching run"
// semantics: a node qualifies if it has ANY single run matching all filters,
// and the row shows that node's LATEST matching run — NOT its latest overall
// run. This is the case that distinguishes semantics B from A.
func TestFunctional_ConvergeRuns_NodeRollupExistsSemantics(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() {
		db.pool.ExecContext(ctx, `DROP TABLE IF EXISTS converge_runs_20400101`)
		db.pool.ExecContext(ctx, `DELETE FROM converge_runs WHERE organisation = 'org-store-test'`)
	})

	// web-mixed: a FAILED speculative-19 run, then a LATER green production-18 run.
	specFail := runNode("nr-spec-fail", "org-store-test", "web-mixed.example.com", convD1, "failure", "19.0.12")
	specFail.Error = &ingest.RunError{Class: "Mixlib::ShellOut::CommandTimeout", Message: "not enough space on /var"}
	prodOK := runNode("nr-prod-ok", "org-store-test", "web-mixed.example.com", convD1.Add(time.Hour), "success", "18.9.4")
	// web-clean: only a green production-18 run — must NOT match the failure filter.
	clean := runNode("nr-clean", "org-store-test", "web-clean.example.com", convD1.Add(2*time.Hour), "success", "18.9.4")

	if _, err := db.BulkUpsertConvergeRuns(ctx, []ingest.ConvergeRun{specFail, prodOK, clean}); err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	// Filter: failed 19.x runs. web-mixed qualifies via its speculative fail even
	// though its LATEST overall run is green; web-clean does not qualify at all.
	got, total, err := db.ListConvergeRunNodesFiltered(ctx, ConvergeRunFilter{
		Organisation: "org-store-test",
		Status:       "failure",
		ChefVersion:  "19.0.12",
	})
	if err != nil {
		t.Fatalf("ListConvergeRunNodesFiltered: %v", err)
	}
	if len(got) != 1 || total != 1 {
		t.Fatalf("distinct nodes = %d (total %d), want 1/1: %+v", len(got), total, got)
	}
	if got[0].NodeName != "web-mixed.example.com" {
		t.Errorf("node = %q, want web-mixed.example.com", got[0].NodeName)
	}
	// Row is the latest MATCHING run (the failed 19 run), not the later green run.
	if got[0].RunID != "nr-spec-fail" || got[0].Status != "failure" {
		t.Errorf("row = %q/%s, want nr-spec-fail/failure (latest matching, not latest overall)", got[0].RunID, got[0].Status)
	}

	// Failure-message substring isolates the authored abort reason.
	msg, _, err := db.ListConvergeRunNodesFiltered(ctx, ConvergeRunFilter{
		Organisation:   "org-store-test",
		FailureMessage: "not enough space",
	})
	if err != nil {
		t.Fatalf("failure-message filter: %v", err)
	}
	if len(msg) != 1 || msg[0].NodeName != "web-mixed.example.com" {
		t.Errorf("failure-message filter = %v, want [web-mixed]", runIDs(msg))
	}

	// Without filters, both nodes appear once each (latest overall run).
	all, total, err := db.ListConvergeRunNodesFiltered(ctx, ConvergeRunFilter{Organisation: "org-store-test"})
	if err != nil {
		t.Fatalf("unfiltered node rollup: %v", err)
	}
	if len(all) != 2 || total != 2 {
		t.Errorf("unfiltered nodes = %d (total %d), want 2/2", len(all), total)
	}
	for _, n := range all {
		if n.NodeName == "web-mixed.example.com" && n.RunID != "nr-prod-ok" {
			t.Errorf("unfiltered web-mixed row = %q, want nr-prod-ok (latest overall)", n.RunID)
		}
	}
}

// GetConvergeRunByID fetches a single run for the detail view; ErrNotFound
// when retention has dropped it.
func TestFunctional_ConvergeRuns_GetByID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	t.Cleanup(func() { db.pool.ExecContext(ctx, `DROP TABLE IF EXISTS converge_runs_20400101`) })

	r := runNode("get-r1", "org-store-test", "web01.example.com", convD1, "failure", "19.0.12")
	r.Error = &ingest.RunError{Class: "RuntimeError", Message: "boom", Backtrace: []string{"a", "b"}}
	if _, err := db.BulkUpsertConvergeRuns(ctx, []ingest.ConvergeRun{r}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := db.GetConvergeRunByID(ctx, "get-r1")
	if err != nil {
		t.Fatalf("GetConvergeRunByID: %v", err)
	}
	if got.NodeName != "web01.example.com" || got.Organisation != "org-store-test" {
		t.Errorf("run identity wrong: %+v", got)
	}
	if len(got.Error) == 0 {
		t.Errorf("failure detail missing on detail fetch")
	}
	if _, err := db.GetConvergeRunByID(ctx, "does-not-exist"); err != ErrNotFound {
		t.Errorf("missing run err = %v, want ErrNotFound", err)
	}
}
