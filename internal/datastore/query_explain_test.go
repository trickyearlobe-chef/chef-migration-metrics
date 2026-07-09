// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateFreeformExplainSQL(t *testing.T) {
	// Explainable data statements (reads and writes) are accepted; plan-only
	// EXPLAIN never executes writes, and the READ ONLY txn blocks ANALYZE writes.
	accept := []string{
		"SELECT 1",
		"select 1",
		"  SELECT * FROM node_snapshots  ",
		"SELECT * FROM node_snapshots;",
		"SELECT * FROM node_snapshots ;  ",
		"WITH x AS (SELECT 1) SELECT * FROM x",
		"with rolled as (select 1) select * from rolled",
		"UPDATE kitchen_run_queue SET status = $1 WHERE id = 1 RETURNING id",
		"DELETE FROM node_snapshots WHERE node_name = 'x'",
		"INSERT INTO node_snapshots (node_name) VALUES ('x')",
		"WITH claimed AS (SELECT id FROM q LIMIT 1) UPDATE q SET s = 1",
		"VALUES (1), (2)",
		"TABLE role_summary",
	}
	for _, s := range accept {
		got, err := ValidateFreeformExplainSQL(s)
		if err != nil {
			t.Errorf("ValidateFreeformExplainSQL(%q) unexpected error: %v", s, err)
		}
		if strings.HasSuffix(got, ";") {
			t.Errorf("ValidateFreeformExplainSQL(%q) should strip trailing ;, got %q", s, got)
		}
	}

	// Utility statements are not explainable by PostgreSQL; multi-statement and
	// non-statements are rejected.
	reject := []string{
		"",
		"   ",
		";",
		"COPY node_snapshots TO stdout",
		"DROP TABLE node_snapshots",
		"TRUNCATE node_snapshots",
		"VACUUM node_snapshots",
		"ALTER TABLE node_snapshots ADD COLUMN x int",
		"SET statement_timeout = 0",
		"SELECT 1; DROP TABLE node_snapshots", // multi-statement
		"SELECT 1; SELECT 2",                  // multi-statement
		"-- just a comment",                   // no leading keyword
		"UPDATED_AT is not a keyword select",  // identifier prefix, not the UPDATE keyword
	}
	for _, s := range reject {
		if _, err := ValidateFreeformExplainSQL(s); !errors.Is(err, ErrExplainNotExplainable) {
			t.Errorf("ValidateFreeformExplainSQL(%q) = err %v, want ErrExplainNotExplainable", s, err)
		}
	}
}

func TestExplainCatalog_StableKeys(t *testing.T) {
	var db *DB // ExplainCatalog is static — no DB access.
	entries := db.ExplainCatalog()
	if len(entries) == 0 {
		t.Fatal("ExplainCatalog returned no entries")
	}
	want := map[string]bool{
		explainRolesList:     false,
		explainNodeListHeavy: false,
		explainNodeListLight: false,
		explainCookbookCover: false,
		explainNodeSingleRow: false,
		explainDistinctRoles: false,
	}
	seen := make(map[string]bool)
	for _, e := range entries {
		if e.Key == "" || e.Label == "" || e.Description == "" {
			t.Errorf("catalog entry has empty field: %+v", e)
		}
		if seen[e.Key] {
			t.Errorf("duplicate catalog key %q", e.Key)
		}
		seen[e.Key] = true
		if _, ok := want[e.Key]; !ok {
			t.Errorf("unexpected catalog key %q", e.Key)
		}
	}
	for k := range want {
		if !seen[k] {
			t.Errorf("missing expected catalog key %q", k)
		}
	}
}
