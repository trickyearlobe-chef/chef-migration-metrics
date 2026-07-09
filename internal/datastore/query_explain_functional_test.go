// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// seedExplainFixture creates enough live data (org, collection run, node,
// active cookbook, role_summary) that every catalog resolver can find a sample.
func seedExplainFixture(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	const org = "func-explain-org"

	cleanupTestData(t, db,
		"DELETE FROM role_summary WHERE organisation_name = '"+org+"'",
		"DELETE FROM node_snapshots WHERE organisation_name = '"+org+"'",
		"DELETE FROM server_cookbooks WHERE organisation_name = '"+org+"'",
		"DELETE FROM collection_runs WHERE organisation_name = '"+org+"'",
		"DELETE FROM organisations WHERE name = '"+org+"'",
	)

	o, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name: org, ChefServerURL: "https://example.com/organizations/explain", OrgName: "explain", ClientName: "c",
	})
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	run, err := db.CreateCollectionRun(ctx, CreateCollectionRunParams{OrganisationName: o.Name})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	now := time.Now().UTC()
	roles, _ := json.Marshal([]string{"explain-web"})
	if _, err := db.BulkUpsertNodeSnapshots(ctx, []InsertNodeSnapshotParams{
		{CollectionRunOrg: run.OrganisationName, OrganisationName: org, NodeName: "explain-n1", Roles: roles, Platform: "ubuntu", PlatformVersion: "22.04", CollectedAt: now},
	}); err != nil {
		t.Fatalf("nodes: %v", err)
	}
	if _, err := db.UpsertServerCookbook(ctx, UpsertServerCookbookParams{
		OrganisationName: org, Name: "explain-nginx", Version: "1.0.0", IsActive: true, LastFetchedAt: now,
	}); err != nil {
		t.Fatalf("cookbook: %v", err)
	}
	seedRoleSummary(t, db, org, "explain-web",
		rsRow{nodeCount: 1, directCB: 1, transitiveCB: 1, compatible: 1}, "compatible", "passed")
}

func TestFunctional_RunExplain_PlanOnlyAndAnalyze(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	t.Run("plan only", func(t *testing.T) {
		res, err := db.RunExplain(ctx, "SELECT 1", nil, ExplainOptions{})
		if err != nil {
			t.Fatalf("RunExplain: %v", err)
		}
		if strings.TrimSpace(res.Run1.PlanText) == "" {
			t.Errorf("expected non-empty plan text, got empty")
		}
		if res.Run2 != nil {
			t.Errorf("expected no Run2 when RunTwice is false")
		}
	})

	t.Run("analyze adds execution time", func(t *testing.T) {
		res, err := db.RunExplain(ctx, "SELECT count(*) FROM role_summary", nil, ExplainOptions{Analyze: true})
		if err != nil {
			t.Fatalf("RunExplain analyze: %v", err)
		}
		if !strings.Contains(res.Run1.PlanText, "Execution Time") {
			t.Errorf("EXPLAIN ANALYZE plan should include \"Execution Time\":\n%s", res.Run1.PlanText)
		}
	})

	t.Run("run twice returns run2", func(t *testing.T) {
		res, err := db.RunExplain(ctx, "SELECT 1", nil, ExplainOptions{Analyze: true, RunTwice: true})
		if err != nil {
			t.Fatalf("RunExplain: %v", err)
		}
		if res.Run2 == nil {
			t.Errorf("expected Run2 when RunTwice is true")
		}
	})
}

func TestFunctional_RunExplain_ReadOnlyBlocksMutation(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	seedExplainFixture(t, db)

	// EXPLAIN (ANALYZE) executes the statement; the READ ONLY transaction must
	// reject a mutating statement before it runs.
	_, err := db.RunExplain(ctx, "UPDATE role_summary SET node_count = node_count + 1", nil, ExplainOptions{Analyze: true})
	if err == nil {
		t.Fatal("expected READ ONLY transaction to reject UPDATE under EXPLAIN ANALYZE, got nil error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "read-only") {
		t.Errorf("expected read-only transaction error, got: %v", err)
	}

	// The value must be unchanged (transaction rolled back / never executed).
	var nc int
	if err := db.pool.QueryRowContext(ctx,
		"SELECT node_count FROM role_summary WHERE organisation_name = 'func-explain-org' AND role_name = 'explain-web'").Scan(&nc); err != nil {
		t.Fatalf("reading node_count: %v", err)
	}
	if nc != 1 {
		t.Errorf("node_count mutated to %d, want 1 (no mutation)", nc)
	}
}

func TestFunctional_RunExplain_PlanOnlyWriteDoesNotMutate(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	seedExplainFixture(t, db)

	// Plan-only EXPLAIN of a write returns a plan and never executes the write.
	res, err := db.RunExplain(ctx,
		"UPDATE role_summary SET node_count = node_count + 100 WHERE organisation_name = 'func-explain-org'",
		nil, ExplainOptions{})
	if err != nil {
		t.Fatalf("plan-only EXPLAIN of UPDATE: %v", err)
	}
	if !strings.Contains(res.Run1.PlanText, "Update") {
		t.Errorf("expected an Update plan node, got:\n%s", res.Run1.PlanText)
	}

	var nc int
	if err := db.pool.QueryRowContext(ctx,
		"SELECT node_count FROM role_summary WHERE organisation_name = 'func-explain-org' AND role_name = 'explain-web'").Scan(&nc); err != nil {
		t.Fatalf("reading node_count: %v", err)
	}
	if nc != 1 {
		t.Errorf("node_count mutated to %d, want 1 (plan-only must not execute)", nc)
	}
}

func TestFunctional_RunExplain_StatementTimeout(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// A tiny timeout must abort a slow ANALYZE run and roll back cleanly.
	_, err := db.RunExplain(ctx, "SELECT pg_sleep(2)", nil, ExplainOptions{Analyze: true, StatementTimeoutMs: 100})
	if err == nil {
		t.Fatal("expected statement_timeout to abort pg_sleep(2), got nil error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Errorf("expected statement timeout error, got: %v", err)
	}

	// The connection must be usable afterwards (transaction rolled back).
	if _, err := db.RunExplain(ctx, "SELECT 1", nil, ExplainOptions{}); err != nil {
		t.Errorf("connection unusable after timeout: %v", err)
	}
}

func TestFunctional_ResolveCatalogExplain_AllKeysRunnable(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	seedExplainFixture(t, db)

	for _, entry := range db.ExplainCatalog() {
		t.Run(entry.Key, func(t *testing.T) {
			sqlText, args, label, summary, err := db.ResolveCatalogExplain(ctx, entry.Key, CatalogParams{DefaultTargetVersion: "19.3.15"})
			if err != nil {
				t.Fatalf("resolve %s: %v", entry.Key, err)
			}
			if label == "" || summary == "" {
				t.Errorf("%s: empty label/summary (%q/%q)", entry.Key, label, summary)
			}
			// Param summary must not leak raw identifier values (counts only).
			if strings.Contains(summary, "explain-nginx") || strings.Contains(summary, "explain-n1") || strings.Contains(summary, "func-explain-org") {
				t.Errorf("%s: param summary leaks an identifier: %q", entry.Key, summary)
			}
			res, err := db.RunExplain(ctx, sqlText, args, ExplainOptions{})
			if err != nil {
				t.Fatalf("run %s: %v", entry.Key, err)
			}
			if strings.TrimSpace(res.Run1.PlanText) == "" {
				t.Errorf("%s: empty plan text", entry.Key)
			}
		})
	}
}
