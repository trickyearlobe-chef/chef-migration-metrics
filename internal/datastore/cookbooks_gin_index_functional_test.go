// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestFunctional_ProductionPlatformsQuery_UsesGINIndex proves the fix: the
// `cookbooks ? $1` coverage query is served by the GIN index
// idx_node_snapshots_cookbooks_gin (default jsonb_ops opclass, which — unlike
// jsonb_path_ops — indexes the `?` key-existence operator) rather than a
// sequential scan over all node_snapshots.
//
// Method: seed a handful of node_snapshots, then EXPLAIN the exact production
// query (referencing the shared const, so no drift) with enable_seqscan
// disabled. The plan must name the GIN index. Before migration 0050 the index
// does not exist, so the planner falls back to a (penalised) Seq Scan and the
// assertion fails — this is the TDD red state.
func TestFunctional_ProductionPlatformsQuery_UsesGINIndex(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	org, err := db.UpsertOrganisationFromConfig(ctx, UpsertOrganisationParams{
		Name:          "func-gin-org",
		ChefServerURL: "https://example.com/organizations/test",
		OrgName:       "test",
		ClientName:    "test-client",
	})
	if err != nil {
		t.Fatalf("creating org: %v", err)
	}

	run, err := db.CreateCollectionRun(ctx, CreateCollectionRunParams{
		OrganisationName: org.Name,
	})
	if err != nil {
		t.Fatalf("creating collection run: %v", err)
	}

	cleanupTestData(t, db,
		"DELETE FROM node_snapshots WHERE collection_run_org = '"+run.OrganisationName+"'",
		"DELETE FROM collection_runs WHERE organisation_name = '"+run.OrganisationName+"'",
		"DELETE FROM organisations WHERE name = '"+org.Name+"'",
	)

	// Seed a realistic, SELECTIVE distribution: the target cookbook appears on
	// only a couple of nodes out of ~1500, so a GIN index scan (a few rows) is
	// unambiguously cheaper than any full scan + filter. This mirrors production
	// (a cookbook lives on a small fraction of the fleet) and makes the planner's
	// choice deterministic rather than dependent on a tiny-table cost tie.
	withTarget, _ := json.Marshal(map[string]any{
		"func-gin-cb": map[string]string{"version": "1.0.0"},
		"ntp":         map[string]string{"version": "3.0.0"},
	})
	without, _ := json.Marshal(map[string]any{
		"ntp": map[string]string{"version": "3.0.0"},
	})

	now := time.Now().UTC()
	const total, matches = 1500, 2
	var nodes []InsertNodeSnapshotParams
	for i := 0; i < total; i++ {
		cb := without
		if i < matches {
			cb = withTarget
		}
		nodes = append(nodes, InsertNodeSnapshotParams{
			CollectionRunOrg: run.OrganisationName, OrganisationName: org.Name,
			NodeName: fmt.Sprintf("func-gin-node-%d", i), Platform: "ubuntu",
			PlatformVersion: "22.04", PlatformFamily: "debian",
			Cookbooks: cb, CollectedAt: now,
		})
	}
	if _, err := db.BulkUpsertNodeSnapshots(ctx, nodes); err != nil {
		t.Fatalf("inserting node snapshots: %v", err)
	}

	// Refresh planner statistics so it sees the real row count and the
	// selectivity of the cookbook predicate.
	if _, err := db.pool.ExecContext(ctx, "ANALYZE node_snapshots"); err != nil {
		t.Fatalf("analyzing node_snapshots: %v", err)
	}

	// EXPLAIN the exact production query with seq scans disabled, in a single
	// transaction so SET LOCAL and the EXPLAIN share one session.
	tx, err := db.pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("beginning tx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after read-only tx

	if _, err := tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatalf("disabling seqscan: %v", err)
	}

	var planJSON string
	err = tx.QueryRowContext(ctx,
		"EXPLAIN (FORMAT JSON) "+productionPlatformsForCookbookQuery, "func-gin-cb",
	).Scan(&planJSON)
	if err != nil {
		t.Fatalf("explaining query: %v", err)
	}

	if !strings.Contains(planJSON, "idx_node_snapshots_cookbooks_gin") {
		t.Errorf("coverage query does not use the GIN index — plan was a sequential scan.\nPlan:\n%s", planJSON)
	}
	if strings.Contains(planJSON, `"Node Type": "Seq Scan"`) {
		t.Errorf("coverage query still contains a Seq Scan node.\nPlan:\n%s", planJSON)
	}
}
