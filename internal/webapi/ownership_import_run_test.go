// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipimport"
)

// A scheduled import runs with nobody watching, so what it does has to be
// decided entirely by what was saved. These tests drive the run from a saved
// definition and a source, with no HTTP request anywhere.

// fakeRowSource is a RowSource over rows held in memory.
type fakeRowSource struct {
	columns []string
	rows    []map[string]string
	i       int
	err     error
}

func (f *fakeRowSource) Columns() []string { return f.columns }
func (f *fakeRowSource) Next() bool        { f.i++; return f.i <= len(f.rows) }
func (f *fakeRowSource) Row() ownershipimport.Row {
	return ownershipimport.Row{Number: f.i, Values: f.rows[f.i-1]}
}
func (f *fakeRowSource) Err() error   { return f.err }
func (f *fakeRowSource) Close() error { return nil }

func savedDatabaseImport(t *testing.T) datastore.ImportMapping {
	t.Helper()
	return datastore.ImportMapping{
		ID:           7,
		Name:         "cmdb-nightly",
		SourceKind:   "database",
		FieldMap:     json.RawMessage(repoFieldMap(t)),
		DBDriver:     "postgres",
		DBQuery:      "SELECT 1",
		CreateOwners: true,
	}
}

// The same two rows the file-import tests use, so a scheduled run and an
// interactive one are demonstrably reading the same shape.
func twoRowSource() *fakeRowSource {
	return &fakeRowSource{
		columns: []string{"Owner", "Repo"},
		rows: []map[string]string{
			{"Owner": "Alice Smith", "Repo": "web-app"},
			{"Owner": "Bob Jones", "Repo": "db-tools"},
		},
	}
}

func TestRunSavedImport_CommitsWithoutARequest(t *testing.T) {
	var assignments []string
	store := &mockStore{
		InsertAssignmentFn: func(_ context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			assignments = append(assignments, p.OwnerName+"/"+p.EntityKey)
			return datastore.OwnershipAssignment{}, nil
		},
	}
	r := ownershipRouter(store)

	summary, err := r.runImportFromSource(context.Background(), twoRowSource(), savedDatabaseImport(t))
	if err != nil {
		t.Fatalf("runImportFromSource: %v", err)
	}
	if summary.RowCount != 2 {
		t.Errorf("RowCount = %d, want 2", summary.RowCount)
	}
	if len(assignments) != 2 {
		t.Fatalf("wrote %d assignments, want 2: %v", len(assignments), assignments)
	}
}

// The audit log has to be able to say who did this. "unknown" would make a
// scheduled import indistinguishable from an unauthenticated one.
func TestRunSavedImport_AttributesTheAuditEntriesToTheSchedule(t *testing.T) {
	var actors []string
	store := &mockStore{
		InsertAuditEntryFn: func(_ context.Context, p datastore.InsertAuditEntryParams) error {
			actors = append(actors, p.Actor)
			return nil
		},
	}
	r := ownershipRouter(store)

	if _, err := r.runImportFromSource(context.Background(), twoRowSource(), savedDatabaseImport(t)); err != nil {
		t.Fatalf("runImportFromSource: %v", err)
	}
	if len(actors) == 0 {
		t.Fatal("the run wrote no audit entries")
	}
	for _, a := range actors {
		if !strings.Contains(a, "cmdb-nightly") {
			t.Errorf("audit actor = %q, want it to name the saved import", a)
		}
	}
}

// The row filter is part of the import. An unattended run that dropped it
// would import every kind of asset under whichever entity type is mapped.
func TestRunSavedImport_AppliesTheSavedRowFilter(t *testing.T) {
	var assignments []string
	store := &mockStore{
		InsertAssignmentFn: func(_ context.Context, p datastore.InsertAssignmentParams) (datastore.OwnershipAssignment, error) {
			assignments = append(assignments, p.EntityKey)
			return datastore.OwnershipAssignment{}, nil
		},
	}
	r := ownershipRouter(store)

	m := savedDatabaseImport(t)
	m.FilterColumn = "Repo"
	m.FilterValue = "web-app"

	summary, err := r.runImportFromSource(context.Background(), twoRowSource(), m)
	if err != nil {
		t.Fatalf("runImportFromSource: %v", err)
	}
	if summary.FilteredOut != 1 {
		t.Errorf("FilteredOut = %d, want 1", summary.FilteredOut)
	}
	if len(assignments) != 1 || assignments[0] != "web-app" {
		t.Errorf("assignments = %v, want just web-app", assignments)
	}
}

// A source that fails part way through must not be recorded as a short but
// successful import — that is how a partial import gets believed.
func TestRunSavedImport_ReportsAReadFailureRatherThanAShortRun(t *testing.T) {
	src := twoRowSource()
	src.err = errors.New("connection reset")
	r := ownershipRouter(&mockStore{})

	if _, err := r.runImportFromSource(context.Background(), src, savedDatabaseImport(t)); err == nil {
		t.Fatal("a source that failed mid-read was reported as a successful import")
	}
}

// The run history is a record of what happened, so it has to use the past
// tense. The report carries the preview vocabulary — a committed row is still
// labelled "would_create" — and reporting that verbatim would say the run
// stopped short of writing when it did not. Found by watching a real
// scheduled run, which reported "8 would create" after creating 8.
func TestImportRunSummary_ReportsWhatHappenedNotWhatWouldHave(t *testing.T) {
	summary := ImportRunSummary{
		RowCount:    11,
		FilteredOut: 3,
		Counts: map[string]int{
			ownershipimport.OutcomeWouldCreate:     8,
			ownershipimport.OutcomeOwnedByOther:    1,
			ownershipimport.OutcomeDuplicateExists: 0,
			ownershipimport.OutcomeRejected:        2,
		},
	}

	got := summary.String()
	if strings.Contains(got, "would") {
		t.Errorf("summary %q talks about what would happen, but it already has", got)
	}
	for _, want := range []string{"11 rows", "8 created", "2 rejected", "3 filtered out"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q omits %q", got, want)
		}
	}
	// An outcome with no rows must not be listed, or every run reads as though
	// something happened in every category.
	if strings.Contains(got, "already there") {
		t.Errorf("summary %q lists an outcome that had no rows", got)
	}
}

// The rows a source gave that could not be used are what the source's owner
// has to fix, so a commit has to keep them. Until now they were computed,
// shown on screen once, and lost when the page closed.
func TestRunSavedImport_KeepsTheRowsItCouldNotUse(t *testing.T) {
	var storedLabel string
	var storedMappingID *int64
	var stored []datastore.ImportRejection
	store := &mockStore{
		ReplaceImportRejectionsFn: func(_ context.Context, label string, id *int64, rs []datastore.ImportRejection) (int, error) {
			storedLabel, storedMappingID, stored = label, id, rs
			return len(rs), nil
		},
	}
	r := ownershipRouter(store)

	// A row with no owner value cannot be used.
	src := twoRowSource()
	src.rows[1]["Owner"] = ""

	if _, err := r.runImportFromSource(context.Background(), src, savedDatabaseImport(t)); err != nil {
		t.Fatalf("runImportFromSource: %v", err)
	}

	if len(stored) != 1 {
		t.Fatalf("stored %d rejections, want the one unusable row: %+v", len(stored), stored)
	}
	if stored[0].SourceRow != 2 {
		t.Errorf("source_row = %d, want 2 so the row can be found at source", stored[0].SourceRow)
	}
	if stored[0].Reason == "" {
		t.Error("the rejection has no reason, so the report cannot say what is wrong")
	}
	// Named and keyed to the import, so a report can say which source it is
	// talking about and deleting the import takes its findings with it.
	if storedLabel != "cmdb-nightly" {
		t.Errorf("label = %q, want the import's name", storedLabel)
	}
	if storedMappingID == nil || *storedMappingID != 7 {
		t.Errorf("mapping id = %v, want the saved import's id", storedMappingID)
	}
}

// A run with nothing wrong must clear the previous run's findings, not leave
// them standing. Otherwise a source that has been fixed still reads as broken.
func TestRunSavedImport_ClearsPreviousFindingsWhenNothingIsWrong(t *testing.T) {
	called := false
	store := &mockStore{
		ReplaceImportRejectionsFn: func(_ context.Context, _ string, _ *int64, rs []datastore.ImportRejection) (int, error) {
			called = true
			if len(rs) != 0 {
				t.Errorf("stored %d rejections for a clean run", len(rs))
			}
			return 0, nil
		},
	}
	r := ownershipRouter(store)

	if _, err := r.runImportFromSource(context.Background(), twoRowSource(), savedDatabaseImport(t)); err != nil {
		t.Fatalf("runImportFromSource: %v", err)
	}
	if !called {
		t.Error("a clean run did not clear the previous run's findings, so a fixed source still reads as broken")
	}
}

// Failing to store the findings must not be reported as the import failing:
// the assignments are already written by that point.
func TestRunSavedImport_SurvivesFailingToStoreItsFindings(t *testing.T) {
	store := &mockStore{
		ReplaceImportRejectionsFn: func(context.Context, string, *int64, []datastore.ImportRejection) (int, error) {
			return 0, errors.New("disk full")
		},
	}
	r := ownershipRouter(store)

	if _, err := r.runImportFromSource(context.Background(), twoRowSource(), savedDatabaseImport(t)); err != nil {
		t.Errorf("the import was reported as failed because its findings could not be stored: %v", err)
	}
}
