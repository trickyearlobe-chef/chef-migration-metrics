// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package datastore

import (
	"context"
	"testing"
)

func rejection(row int, reason, owner, key string) ImportRejection {
	return ImportRejection{
		SourceRow: row, Reason: reason, OwnerRaw: owner,
		EntityType: "git_repo", EntityKey: key,
	}
}

func cleanupRejections(t *testing.T, db *DB) {
	t.Helper()
	t.Cleanup(func() {
		mustExec(t, db, `DELETE FROM ownership_import_rejections WHERE import_label LIKE 'test-%'`)
	})
}

func TestFunctional_ImportRejections_RoundTrip(t *testing.T) {
	db := testDB(t)
	cleanupRejections(t, db)
	ctx := context.Background()

	stored, err := db.ReplaceImportRejections(ctx, "test-cmdb", nil, []ImportRejection{
		rejection(7, "missing_required_field", "", "web-app"),
		rejection(9, "unknown_owner", "a.jones@example.com", "db-tools"),
	})
	if err != nil {
		t.Fatalf("ReplaceImportRejections: %v", err)
	}
	if stored != 2 {
		t.Errorf("stored %d, want 2", stored)
	}

	got, err := db.ListImportRejections(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListImportRejections: %v", err)
	}

	var found []ImportRejection
	for _, r := range got {
		if r.ImportLabel == "test-cmdb" {
			found = append(found, r)
		}
	}
	if len(found) != 2 {
		t.Fatalf("read back %d rejections, want 2", len(found))
	}
	// Source order, so the file reads against the source somebody is fixing.
	if found[0].SourceRow != 7 || found[1].SourceRow != 9 {
		t.Errorf("rows came back as %d, %d — want source order", found[0].SourceRow, found[1].SourceRow)
	}
	if found[1].OwnerRaw != "a.jones@example.com" {
		t.Errorf("owner_raw = %q, want the value the source gave", found[1].OwnerRaw)
	}
	if found[0].RunAt.IsZero() {
		t.Error("run_at is unset, so the report cannot say when the source was last read")
	}
}

// A rejection is a statement about the source as it stands. Once a row is fixed
// the next run must stop reporting it — otherwise the list can never be worked
// down to nothing, which is the only state that makes it worth opening.
func TestFunctional_ImportRejections_ReplaceTheSetRatherThanAppend(t *testing.T) {
	db := testDB(t)
	cleanupRejections(t, db)
	ctx := context.Background()

	if _, err := db.ReplaceImportRejections(ctx, "test-cmdb", nil, []ImportRejection{
		rejection(7, "missing_required_field", "", "web-app"),
		rejection(9, "unknown_owner", "a.jones@example.com", "db-tools"),
	}); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Row 7 was fixed at source; row 9 is still wrong.
	if _, err := db.ReplaceImportRejections(ctx, "test-cmdb", nil, []ImportRejection{
		rejection(9, "unknown_owner", "a.jones@example.com", "db-tools"),
	}); err != nil {
		t.Fatalf("second run: %v", err)
	}

	got, _ := db.ListImportRejections(ctx, 100, 0)
	for _, r := range got {
		if r.ImportLabel == "test-cmdb" && r.SourceRow == 7 {
			t.Error("a row that was fixed at source is still reported as a problem")
		}
	}
}

// One import's findings must not clear another's — they are separate sources
// with separate owners to chase.
func TestFunctional_ImportRejections_AreScopedToTheirImport(t *testing.T) {
	db := testDB(t)
	cleanupRejections(t, db)
	ctx := context.Background()

	if _, err := db.ReplaceImportRejections(ctx, "test-cmdb", nil,
		[]ImportRejection{rejection(1, "unknown_owner", "x", "a")}); err != nil {
		t.Fatalf("seeding the first import: %v", err)
	}
	if _, err := db.ReplaceImportRejections(ctx, "test-other", nil,
		[]ImportRejection{rejection(2, "unknown_owner", "y", "b")}); err != nil {
		t.Fatalf("seeding the second import: %v", err)
	}

	got, _ := db.ListImportRejections(ctx, 100, 0)
	labels := map[string]int{}
	for _, r := range got {
		labels[r.ImportLabel]++
	}
	if labels["test-cmdb"] != 1 || labels["test-other"] != 1 {
		t.Errorf("labels = %v, want one rejection under each import", labels)
	}
}

// A very dirty source must not turn the table into a second copy of itself.
// The cap is reported rather than applied silently — the caller needs the
// difference to say "1000 of 40000 shown".
func TestFunctional_ImportRejections_AreCappedAndSayHowMany(t *testing.T) {
	db := testDB(t)
	cleanupRejections(t, db)

	many := make([]ImportRejection, maxStoredRejections+50)
	for i := range many {
		many[i] = rejection(i+1, "unknown_owner", "x", "a")
	}

	stored, err := db.ReplaceImportRejections(context.Background(), "test-dirty", nil, many)
	if err != nil {
		t.Fatalf("ReplaceImportRejections: %v", err)
	}
	if stored != maxStoredRejections {
		t.Errorf("stored %d, want the cap of %d", stored, maxStoredRejections)
	}
	if stored == len(many) {
		t.Error("the cap did not bite, so a very dirty source would be copied whole")
	}
}

// Deleting a saved import must take its findings with it, or the report keeps
// naming a source nothing points at any more.
func TestFunctional_ImportRejections_GoWithTheImportTheyCameFrom(t *testing.T) {
	db := testDB(t)
	cleanupRejections(t, db)
	ctx := context.Background()

	m := insertScheduledImport(t, db, "test-import-with-rejections", "")
	if _, err := db.ReplaceImportRejections(ctx, "test-scoped", &m.ID,
		[]ImportRejection{rejection(1, "unknown_owner", "x", "a")}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := db.DeleteImportMapping(ctx, m.ID); err != nil {
		t.Fatalf("DeleteImportMapping: %v", err)
	}

	got, _ := db.ListImportRejections(ctx, 100, 0)
	for _, r := range got {
		if r.ImportLabel == "test-scoped" {
			t.Error("the rejections outlived the import they came from")
		}
	}
}
