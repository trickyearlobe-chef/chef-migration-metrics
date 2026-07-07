// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"strings"
	"testing"
)

// TestFunctional_RoleSummarySchema verifies migration 0049 creates the
// role_summary materialised table with the expected grain, defaults, primary
// key, and CHECK-constrained status vocabularies. The table is written by the
// recompute functions (chunk 2) and read by the roles list (chunk 3); this
// test pins the contract the schema must satisfy.
func TestFunctional_RoleSummarySchema(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	cleanupTestData(t, db,
		"DELETE FROM role_summary WHERE role_name LIKE 'func-rs-%'",
	)

	// A minimal insert must succeed and pick up defaults for the aggregate
	// columns (grain is organisation_name + role_name only).
	if _, err := db.pool.ExecContext(ctx,
		`INSERT INTO role_summary (organisation_name, role_name) VALUES ($1, $2)`,
		"func-rs-org", "func-rs-role-defaults",
	); err != nil {
		t.Fatalf("inserting minimal role_summary row: %v", err)
	}

	var (
		nodeCount, directCB, transitiveCB  int
		compatible, incompatible, untested int
		tkPassed, tkTotal                  int
		compatStatus, tkStatus             string
	)
	if err := db.pool.QueryRowContext(ctx,
		`SELECT node_count, direct_cookbook_count, transitive_cookbook_count,
		        compatible_count, incompatible_count, untested_count,
		        compatibility_status, tk_status, tk_passed, tk_total
		 FROM role_summary WHERE organisation_name = $1 AND role_name = $2`,
		"func-rs-org", "func-rs-role-defaults",
	).Scan(&nodeCount, &directCB, &transitiveCB,
		&compatible, &incompatible, &untested,
		&compatStatus, &tkStatus, &tkPassed, &tkTotal); err != nil {
		t.Fatalf("reading role_summary defaults: %v", err)
	}

	for name, got := range map[string]int{
		"node_count": nodeCount, "direct_cookbook_count": directCB,
		"transitive_cookbook_count": transitiveCB, "compatible_count": compatible,
		"incompatible_count": incompatible, "untested_count": untested,
		"tk_passed": tkPassed, "tk_total": tkTotal,
	} {
		if got != 0 {
			t.Errorf("default %s = %d, want 0", name, got)
		}
	}
	if compatStatus != "untested" {
		t.Errorf("default compatibility_status = %q, want untested", compatStatus)
	}
	if tkStatus != "untested" {
		t.Errorf("default tk_status = %q, want untested", tkStatus)
	}

	// Primary key is (organisation_name, role_name): a duplicate must conflict.
	_, err := db.pool.ExecContext(ctx,
		`INSERT INTO role_summary (organisation_name, role_name) VALUES ($1, $2)`,
		"func-rs-org", "func-rs-role-defaults",
	)
	if err == nil {
		t.Error("expected duplicate (organisation_name, role_name) to violate the primary key")
	} else if !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Errorf("expected duplicate-key error, got: %v", err)
	}

	// The same role_name in a different org is a distinct row (per-org grain).
	if _, err := db.pool.ExecContext(ctx,
		`INSERT INTO role_summary (organisation_name, role_name) VALUES ($1, $2)`,
		"func-rs-org-b", "func-rs-role-defaults",
	); err != nil {
		t.Errorf("same role in a second org should be allowed: %v", err)
	}

	// CHECK constraints reject out-of-vocabulary status values.
	if _, err := db.pool.ExecContext(ctx,
		`INSERT INTO role_summary (organisation_name, role_name, compatibility_status)
		 VALUES ($1, $2, $3)`,
		"func-rs-org", "func-rs-role-badcompat", "bogus",
	); err == nil {
		t.Error("expected CHECK to reject an invalid compatibility_status")
	}
	// 'error' is a git_repos value but NOT a role-level status (roles collapse
	// errors to untested), so it must be rejected here.
	if _, err := db.pool.ExecContext(ctx,
		`INSERT INTO role_summary (organisation_name, role_name, compatibility_status)
		 VALUES ($1, $2, $3)`,
		"func-rs-org", "func-rs-role-errcompat", "error",
	); err == nil {
		t.Error("expected CHECK to reject compatibility_status = 'error' at the role level")
	}
	if _, err := db.pool.ExecContext(ctx,
		`INSERT INTO role_summary (organisation_name, role_name, tk_status)
		 VALUES ($1, $2, $3)`,
		"func-rs-org", "func-rs-role-badtk", "bogus",
	); err == nil {
		t.Error("expected CHECK to reject an invalid tk_status")
	}
}
