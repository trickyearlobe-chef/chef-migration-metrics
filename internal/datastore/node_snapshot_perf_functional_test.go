// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Scale/EXPLAIN proof.
//
// At fleet scale a COUNT(*) OVER() forces the planner to materialise every
// matching wide row before LIMIT 50, then top-N sort them (temp spill). This
// test proves the split avoids both halves:
//   - the default-sort rows query is served by idx_node_snapshots_node_name as
//     an ordered index scan + LIMIT (no Seq Scan, no Sort, no window), and
//   - the count query is a lean aggregate with no window.
//
// Opt-in: seeding that many rows is slow, so the test is skipped unless
// CMM_TEST_PERF is set. Node count is overridable via CMM_TEST_PERF_NODES.
func TestFunctional_NodeListCountSplit_ScaleExplain(t *testing.T) {
	if os.Getenv("CMM_TEST_PERF") == "" {
		t.Skip("CMM_TEST_PERF not set — skipping the scale/EXPLAIN test")
	}
	db := testDB(t)
	ctx := context.Background()

	nodes := 150000
	if v := os.Getenv("CMM_TEST_PERF_NODES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			t.Fatalf("CMM_TEST_PERF_NODES=%q: must be a positive integer", v)
		}
		nodes = n
	}

	const org = "perf-scale-org"
	cleanupTestData(t, db,
		"DELETE FROM node_snapshots  WHERE organisation_name = '"+org+"'",
		"DELETE FROM collection_runs WHERE organisation_name = '"+org+"'",
		"DELETE FROM organisations   WHERE name              = '"+org+"'",
	)

	o, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name: org, ChefServerURL: "https://example.com/organizations/perf", OrgName: "perf", ClientName: "c",
	})
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	run, err := db.CreateCollectionRun(ctx, CreateCollectionRunParams{OrganisationName: o.Name})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Fast set-based seed: one INSERT ... SELECT generate_series builds all rows
	// server-side (seconds, not minutes) with varied env/version/platform/stale.
	t.Logf("seeding %d node_snapshots …", nodes)
	if _, err := db.pool.ExecContext(ctx, `
		INSERT INTO node_snapshots (
			collection_run_org, organisation_name, node_name,
			chef_environment, chef_version, platform, platform_version,
			ohai_time, is_stale, collected_at
		)
		SELECT $1, $2, 'perf-node-' || lpad(g::text, 7, '0'),
		       (ARRAY['production','staging','development'])[1 + g % 3],
		       (ARRAY['17.10.24','18.2.5','19.3.15'])[1 + (g / 9) % 3],
		       (ARRAY['ubuntu','centos','windows'])[1 + (g / 3) % 3],
		       '22.04',
		       1600000000 + g, (g % 2 = 0), now()
		FROM generate_series(1, $3) g`,
		run.OrganisationName, org, nodes,
	); err != nil {
		t.Fatalf("seeding %d nodes: %v", nodes, err)
	}

	// Refresh planner statistics so it sees the real row count/distribution.
	if _, err := db.pool.ExecContext(ctx, "ANALYZE node_snapshots"); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	explain := func(t *testing.T, query string, args []interface{}) string {
		t.Helper()
		var plan string
		if err := db.pool.QueryRowContext(ctx, "EXPLAIN (FORMAT JSON) "+query, args...).Scan(&plan); err != nil {
			t.Fatalf("explain: %v", err)
		}
		return plan
	}

	t.Run("rows query index-served", func(t *testing.T) {
		// Exact production default-view query (no filter, default sort, page of 50).
		q, args := buildNodeSnapshotFilterQuery(NodeSnapshotFilter{Limit: 50})
		plan := explain(t, q, args)

		if !strings.Contains(plan, "idx_node_snapshots_node_name") {
			t.Errorf("default-sort rows query is not served by idx_node_snapshots_node_name.\nPlan:\n%s", plan)
		}
		if strings.Contains(plan, `"Node Type": "Seq Scan"`) {
			t.Errorf("default-sort rows query still contains a Seq Scan (P3 regression).\nPlan:\n%s", plan)
		}
		if strings.Contains(plan, `"Node Type": "Sort"`) {
			t.Errorf("default-sort rows query still contains a full Sort (P3 regression).\nPlan:\n%s", plan)
		}
		if strings.Contains(plan, "WindowAgg") {
			t.Errorf("default-sort rows query still carries a window aggregate.\nPlan:\n%s", plan)
		}
	})

	t.Run("count query lean aggregate", func(t *testing.T) {
		q, args := buildNodeSnapshotCountQuery(NodeSnapshotFilter{})
		plan := explain(t, q, args)

		if !strings.Contains(plan, "Aggregate") {
			t.Errorf("count query is not a plain aggregate.\nPlan:\n%s", plan)
		}
		// The whole point of the split: no window materialising every wide row.
		if strings.Contains(plan, "WindowAgg") {
			t.Errorf("count query still carries a window aggregate (COUNT(*) OVER()).\nPlan:\n%s", plan)
		}
		if strings.Contains(plan, `"Node Type": "Sort"`) {
			t.Errorf("count query should not sort.\nPlan:\n%s", plan)
		}
	})
}
