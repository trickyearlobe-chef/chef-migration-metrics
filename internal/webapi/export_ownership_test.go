// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// Two exports for two readers.
//
// `ownership` is the current state — the shape the source data should be
// corrected to match. `ownership_corrections` is the fix-list: what we changed
// here, so whoever maintains the source system can change it there. They answer
// different questions and neither substitutes for the other.

func exportRequest(exportType, format string) *http.Request {
	// Exports are created with POST: they are a download the server produces,
	// not a page a link can fetch.
	return httptest.NewRequest(http.MethodPost,
		"/api/v1/exports?export_type="+exportType+"&format="+format, nil)
}

func TestOwnershipExport_CarriesEveryAssignmentWhateverItsOrigin(t *testing.T) {
	store := &mockStore{
		ListAllAssignmentsFn: func(_ context.Context, limit, offset int) ([]datastore.AllAssignmentRow, error) {
			if offset > 0 {
				return nil, nil
			}
			return []datastore.AllAssignmentRow{
				{OwnerName: "thomas-smith", DisplayName: "Thomas Smith", ContactEmail: "t.smith@example.com",
					EntityType: "git_repo", EntityKey: "web-app", AssignmentSource: "import", Confidence: "definitive"},
				{OwnerName: "platform-team", DisplayName: "Platform Team",
					EntityType: "git_repo", EntityKey: "db-tools", AssignmentSource: "manual", Confidence: "definitive"},
			}, nil
		},
	}
	w := httptest.NewRecorder()
	ownershipRouter(store).ServeHTTP(w, withAdminSession(exportRequest("ownership", "csv")))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// The hand-made one especially: an export that showed only the imported
	// half would tell the source's owner to delete the corrections.
	for _, want := range []string{"thomas-smith", "web-app", "platform-team", "db-tools", "manual", "import"} {
		if !strings.Contains(body, want) {
			t.Errorf("the export omits %q:\n%s", want, body)
		}
	}
	// A display name and a contact, so the file is readable by somebody who
	// does not know CMM's owner handles.
	if !strings.Contains(body, "Thomas Smith") || !strings.Contains(body, "t.smith@example.com") {
		t.Errorf("the export omits the human-readable owner details:\n%s", body)
	}
}

func auditEntry(action, owner, entityType, entityKey string, details map[string]any) datastore.OwnershipAuditEntry {
	raw, _ := json.Marshal(details)
	return datastore.OwnershipAuditEntry{
		Timestamp: time.Date(2026, 8, 4, 11, 0, 0, 0, time.UTC),
		Action:    action, Actor: "admin", OwnerName: owner,
		EntityType: entityType, EntityKey: entityKey, Details: raw,
	}
}

func correctionsStore(entries []datastore.OwnershipAuditEntry) *mockStore {
	return &mockStore{
		ListAuditLogFn: func(_ context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error) {
			if f.Offset > 0 {
				return nil, len(entries), nil
			}
			return entries, len(entries), nil
		},
	}
}

func correctionsCSV(t *testing.T, entries []datastore.OwnershipAuditEntry) string {
	t.Helper()
	w := httptest.NewRecorder()
	ownershipRouter(correctionsStore(entries)).ServeHTTP(w,
		withAdminSession(exportRequest("ownership_corrections", "csv")))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// A merge is the single most useful thing to tell a source system's owner:
// two records in their data are the same person.
//
// The details use `into_owner`, NOT `to_owner` — that is what
// datastore.MergeOwnersResult serialises, and it differs from the
// reassignment's `to_owner` for no reason beyond history. The first version of
// this export read `to_owner` and produced an empty "should be" column on every
// real merge; the fixture below is the real shape, taken from the writer.
func TestCorrectionsExport_ReportsAMergeAsADuplicatePerson(t *testing.T) {
	body := correctionsCSV(t, []datastore.OwnershipAuditEntry{
		auditEntry("owner_merged", "thomas-smith", "", "", map[string]any{
			"from_owner": "t-smith", "into_owner": "thomas-smith",
		}),
	})

	if !strings.Contains(body, "t-smith") || !strings.Contains(body, "thomas-smith") {
		t.Errorf("the correction does not name both sides of the merge:\n%s", body)
	}
	// Categorised, so the file can be sorted into kinds of problem rather than
	// read as a flat log of clicks.
	if !strings.Contains(body, "duplicate_person") {
		t.Errorf("the merge is not categorised as a duplicate person:\n%s", body)
	}
}

func TestCorrectionsExport_ReportsAReassignmentAsAWrongOwner(t *testing.T) {
	body := correctionsCSV(t, []datastore.OwnershipAuditEntry{
		auditEntry("assignment_reassigned", "platform-team", "git_repo", "web-app", map[string]any{
			"from_owner": "a-jones", "to_owner": "platform-team",
		}),
	})

	if !strings.Contains(body, "owner_changed") {
		t.Errorf("the reassignment is not categorised:\n%s", body)
	}
	for _, want := range []string{"git_repo", "web-app", "a-jones", "platform-team"} {
		if !strings.Contains(body, want) {
			t.Errorf("the correction omits %q:\n%s", want, body)
		}
	}
}

// A dismissal is a correction too, and an easily-missed one: it tells the
// source's owner that two records which look alike are genuinely two people.
func TestCorrectionsExport_ReportsADismissalAsNotDuplicates(t *testing.T) {
	body := correctionsCSV(t, []datastore.OwnershipAuditEntry{
		auditEntry("owner_duplicate_dismissed", "alice-smith", "", "", map[string]any{
			"owner_a": "alice-smith", "owner_b": "alice-smyth", "reason": "different people",
		}),
	})

	if !strings.Contains(body, "not_duplicates") {
		t.Errorf("the dismissal is not categorised:\n%s", body)
	}
	if !strings.Contains(body, "alice-smyth") {
		t.Errorf("the correction omits the other side of the pair:\n%s", body)
	}
}

// The log carries actions that are not corrections to anything upstream —
// creating an assignment during an import is the import working, not somebody
// fixing it. Including them would bury the real corrections.
func TestCorrectionsExport_LeavesOutWhatIsNotACorrection(t *testing.T) {
	var asked datastore.AuditLogFilter
	store := &mockStore{
		ListAuditLogFn: func(_ context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error) {
			asked = f
			return nil, 0, nil
		},
	}
	w := httptest.NewRecorder()
	ownershipRouter(store).ServeHTTP(w, withAdminSession(exportRequest("ownership_corrections", "csv")))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if len(asked.Actions) == 0 {
		t.Fatal("the export asked for every audit action, so routine import writes would swamp the corrections")
	}
	for _, unwanted := range []string{"assignment_created", "owner_created"} {
		for _, got := range asked.Actions {
			if got == unwanted {
				t.Errorf("%q is an import doing its job, not a correction to the source", unwanted)
			}
		}
	}
}

// The merge writer's payload is datastore.MergeOwnersResult, so its keys are
// whatever that struct's json tags say. Asserting against the struct rather
// than against a literal is what stops the export drifting when the writer is
// next edited — a rename there would otherwise empty a column in a report
// somebody is about to act on, silently.
func TestCorrectionsExport_ReadsTheKeysTheMergeWriterActuallyEmits(t *testing.T) {
	raw, err := json.Marshal(datastore.MergeOwnersResult{
		FromOwner: "t-smith",
		IntoOwner: "thomas-smith",
	})
	if err != nil {
		t.Fatalf("marshalling a merge result: %v", err)
	}

	entry := datastore.OwnershipAuditEntry{Action: "owner_merged", Details: raw}
	if got := correctionWas(entry); got != "t-smith" {
		t.Errorf("source_says = %q, want the owner the source named", got)
	}
	if got := correctionNow(entry); got != "thomas-smith" {
		t.Errorf("should_be = %q, want the owner they were merged into", got)
	}
}

// A deleted assignment names its owner on the entry, not in the details. An
// empty "source says" would leave a row telling somebody to remove ownership
// without saying whose.
func TestCorrectionsExport_NamesTheOwnerOnADeletedAssignment(t *testing.T) {
	entry := auditEntry("assignment_deleted", "a-jones", "git_repo", "web-app", map[string]any{
		"assignment_source": "import", "confidence": "definitive",
	})
	if got := correctionWas(entry); got != "a-jones" {
		t.Errorf("source_says = %q, want the owner whose assignment was removed", got)
	}
	// There is no "should be" for a deletion, and inventing one would be worse
	// than the gap.
	if got := correctionNow(entry); got != "" {
		t.Errorf("should_be = %q, want it empty for a removal", got)
	}
}

// The rows a source gave that could not be used are the most direct statement
// of its quality there is — "this row names somebody who is not in your staff
// table". They belong in the same file as the corrections because they are the
// same person's list of work.
func TestCorrectionsExport_IncludesTheRowsThatCouldNotBeUsed(t *testing.T) {
	store := &mockStore{
		ListImportRejectionsFn: func(_ context.Context, limit, offset int) ([]datastore.ImportRejection, error) {
			if offset > 0 {
				return nil, nil
			}
			return []datastore.ImportRejection{{
				ImportLabel: "cmdb-nightly",
				RunAt:       time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC),
				SourceRow:   9,
				Reason:      "unknown_owner",
				OwnerRaw:    "a.jones@example.com",
				EntityType:  "git_repo",
				EntityKey:   "db-tools",
			}}, nil
		},
	}
	w := httptest.NewRecorder()
	ownershipRouter(store).ServeHTTP(w, withAdminSession(exportRequest("ownership_corrections", "csv")))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"unusable_row", "cmdb-nightly", "a.jones@example.com", "db-tools"} {
		if !strings.Contains(body, want) {
			t.Errorf("the export omits %q:\n%s", want, body)
		}
	}
	// The row number, so somebody can find the record rather than merely
	// learning that one is wrong.
	if !strings.Contains(body, ",9,") {
		t.Errorf("the export omits the source row number:\n%s", body)
	}
	// Said in words, not as a reason code, for a reader who has never used CMM.
	if !strings.Contains(body, "could not be identified") {
		t.Errorf("the export gives no plain-English explanation:\n%s", body)
	}
}

// Both kinds in one file. A corrections export that dropped one when the other
// was present would be silently half a report.
func TestCorrectionsExport_CarriesCorrectionsAndUnusableRowsTogether(t *testing.T) {
	store := &mockStore{
		ListAuditLogFn: func(_ context.Context, f datastore.AuditLogFilter) ([]datastore.OwnershipAuditEntry, int, error) {
			if f.Offset > 0 {
				return nil, 1, nil
			}
			return []datastore.OwnershipAuditEntry{
				auditEntry("owner_merged", "thomas-smith", "", "", map[string]any{
					"from_owner": "t-smith", "into_owner": "thomas-smith",
				}),
			}, 1, nil
		},
		ListImportRejectionsFn: func(_ context.Context, limit, offset int) ([]datastore.ImportRejection, error) {
			if offset > 0 {
				return nil, nil
			}
			return []datastore.ImportRejection{{
				ImportLabel: "cmdb-nightly", SourceRow: 9,
				Reason: "unknown_owner", OwnerRaw: "a.jones@example.com",
			}}, nil
		},
	}
	w := httptest.NewRecorder()
	ownershipRouter(store).ServeHTTP(w, withAdminSession(exportRequest("ownership_corrections", "csv")))

	body := w.Body.String()
	if !strings.Contains(body, "duplicate_person") {
		t.Errorf("the corrections are missing:\n%s", body)
	}
	if !strings.Contains(body, "unusable_row") {
		t.Errorf("the unusable rows are missing:\n%s", body)
	}
}

// Nothing can be said about what an unusable row *should* contain — only
// whoever owns the source knows. An invented value in a file somebody is about
// to act on is worse than a gap.
func TestCorrectionsExport_DoesNotInventAFixForAnUnusableRow(t *testing.T) {
	row := correctionFromRejection(datastore.ImportRejection{
		Reason: "unknown_owner", OwnerRaw: "a.jones@example.com",
	})
	if row.ShouldBe != "" {
		t.Errorf("should_be = %q, want it empty", row.ShouldBe)
	}
}
