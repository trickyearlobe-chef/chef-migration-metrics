// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"encoding/json"
	"testing"
)

// Throwing away what a trial import brought in, so the next one can be judged
// on its own.
//
// Functional rather than mocked because the whole risk here is in what the SQL
// touches. A delete that reaches one table too far is not a behaviour a mock
// can catch — it is exactly the kind of mistake that only shows up against real
// foreign keys and real rows.

func clearTestFixture(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()

	// An owner an import created, carrying an imported assignment.
	mustExec(t, db, `INSERT INTO owners (name, display_name, owner_type) VALUES ('imported-person', 'Imported Person', 'individual')`)
	mustExec(t, db, `INSERT INTO ownership_assignments (owner_name, entity_type, entity_key, assignment_source, confidence)
	                 VALUES ('imported-person', 'git_repo', 'from-import', 'import', 'definitive')`)
	if err := db.InsertAuditEntry(ctx, InsertAuditEntryParams{
		Action: "owner_created", Actor: "tester", OwnerName: "imported-person",
		Details: json.RawMessage(`{"source":"import"}`),
	}); err != nil {
		t.Fatalf("seeding the audit entry: %v", err)
	}

	// An owner somebody added by hand, with a hand-made assignment.
	mustExec(t, db, `INSERT INTO owners (name, display_name, owner_type) VALUES ('hand-made-person', 'Hand Made', 'individual')`)
	mustExec(t, db, `INSERT INTO ownership_assignments (owner_name, entity_type, entity_key, assignment_source, confidence)
	                 VALUES ('hand-made-person', 'git_repo', 'by-hand', 'manual', 'definitive')`)

	// An owner an import created who has since been given a hand-made
	// assignment. The import brought them in, but somebody has since taken
	// responsibility for them, so they must survive.
	mustExec(t, db, `INSERT INTO owners (name, display_name, owner_type) VALUES ('adopted-person', 'Adopted', 'individual')`)
	mustExec(t, db, `INSERT INTO ownership_assignments (owner_name, entity_type, entity_key, assignment_source, confidence)
	                 VALUES ('adopted-person', 'git_repo', 'adopted-repo', 'manual', 'definitive')`)
	if err := db.InsertAuditEntry(ctx, InsertAuditEntryParams{
		Action: "owner_created", Actor: "tester", OwnerName: "adopted-person",
		Details: json.RawMessage(`{"source":"import"}`),
	}); err != nil {
		t.Fatalf("seeding the audit entry: %v", err)
	}

	t.Cleanup(func() {
		mustExec(t, db, `DELETE FROM owners WHERE name IN ('imported-person','hand-made-person','adopted-person')`)
		mustExec(t, db, `DELETE FROM ownership_audit_log WHERE actor = 'tester'`)
	})
}

func mustExec(t *testing.T, db *DB, query string) {
	t.Helper()
	if _, err := db.pool.ExecContext(context.Background(), query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func countRows(t *testing.T, db *DB, query string) int {
	t.Helper()
	var n int
	if err := db.pool.QueryRowContext(context.Background(), query).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func TestFunctional_ClearImportedOwnership_RemovesOnlyWhatAnImportBrought(t *testing.T) {
	db := testDB(t)
	clearTestFixture(t, db)

	result, err := db.ClearImportedOwnership(context.Background())
	if err != nil {
		t.Fatalf("ClearImportedOwnership: %v", err)
	}

	if n := countRows(t, db, `SELECT count(*) FROM ownership_assignments WHERE assignment_source = 'import'`); n != 0 {
		t.Errorf("%d imported assignments survived", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM ownership_assignments WHERE entity_key = 'by-hand'`); n != 1 {
		t.Error("the hand-made assignment was removed; only imported ownership was meant to go")
	}
	if n := countRows(t, db, `SELECT count(*) FROM owners WHERE name = 'hand-made-person'`); n != 1 {
		t.Error("an owner nobody imported was removed")
	}
	if n := countRows(t, db, `SELECT count(*) FROM owners WHERE name = 'imported-person'`); n != 0 {
		t.Error("an owner the import created, now with nothing attached, survived")
	}
	// The one that matters most: an imported person somebody has since made
	// real. Removing them would discard the work that made them real.
	if n := countRows(t, db, `SELECT count(*) FROM owners WHERE name = 'adopted-person'`); n != 1 {
		t.Error("an imported owner who has since been given a hand-made assignment was removed")
	}

	if result.Assignments < 1 {
		t.Errorf("reported %d assignments removed, want at least the one seeded", result.Assignments)
	}
	if result.Owners < 1 {
		t.Errorf("reported %d owners removed, want at least the one seeded", result.Owners)
	}
}

// Running it twice must be safe and must say plainly that the second run found
// nothing — otherwise a repeated click reads as repeated destruction.
func TestFunctional_ClearImportedOwnership_IsSafeToRepeat(t *testing.T) {
	db := testDB(t)
	clearTestFixture(t, db)

	if _, err := db.ClearImportedOwnership(context.Background()); err != nil {
		t.Fatalf("first clear: %v", err)
	}
	second, err := db.ClearImportedOwnership(context.Background())
	if err != nil {
		t.Fatalf("second clear: %v", err)
	}
	if second.Assignments != 0 || second.Owners != 0 {
		t.Errorf("the second clear reported %+v, want nothing left to remove", second)
	}
}

// ---------------------------------------------------------------------------
// The two export queries
//
// Both are hand-written and neither is exercised by the mocked webapi tests,
// which stub the store out entirely. A wrong column name or a placeholder the
// driver will not bind is invisible until somebody clicks Export.
// ---------------------------------------------------------------------------

func TestFunctional_ListAllAssignments_ReadsEveryOriginWithOwnerDetail(t *testing.T) {
	db := testDB(t)
	clearTestFixture(t, db)

	rows, err := db.ListAllAssignments(context.Background(), 500, 0)
	if err != nil {
		t.Fatalf("ListAllAssignments: %v", err)
	}

	bySource := map[string]AllAssignmentRow{}
	for _, r := range rows {
		if r.EntityKey == "from-import" || r.EntityKey == "by-hand" {
			bySource[r.EntityKey] = r
		}
	}
	// Both origins, because the export it feeds is the shape the source data
	// should be corrected to match — showing only the imported half would tell
	// the source's owner to delete the corrections.
	if _, ok := bySource["from-import"]; !ok {
		t.Error("the imported assignment is missing from the full-state export")
	}
	if _, ok := bySource["by-hand"]; !ok {
		t.Error("the hand-made assignment is missing from the full-state export")
	}
	// The display name comes from the join, so this is what proves the join
	// works rather than silently yielding blanks.
	if got := bySource["from-import"].DisplayName; got != "Imported Person" {
		t.Errorf("display_name = %q, want the owner's display name from the join", got)
	}
}

func TestFunctional_ListAuditLog_FiltersToSeveralActions(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	for _, action := range []string{"owner_merged", "assignment_reassigned", "assignment_created"} {
		if err := db.InsertAuditEntry(ctx, InsertAuditEntryParams{
			Action: action, Actor: "corrections-tester", OwnerName: "imported-person",
			Details: json.RawMessage(`{"from_owner":"a","to_owner":"b"}`),
		}); err != nil {
			t.Fatalf("seeding %s: %v", action, err)
		}
	}
	t.Cleanup(func() {
		mustExec(t, db, `DELETE FROM ownership_audit_log WHERE actor = 'corrections-tester'`)
	})

	entries, _, err := db.ListAuditLog(ctx, AuditLogFilter{
		Actions: []string{"owner_merged", "assignment_reassigned"},
		Actor:   "corrections-tester",
		Limit:   100,
	})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want the 2 correction actions", len(entries))
	}
	for _, e := range entries {
		// assignment_created is an import doing its job, not a correction. Its
		// presence would bury the real corrections in the export.
		if e.Action == "assignment_created" {
			t.Error("a routine import write came back as a correction")
		}
	}
}
