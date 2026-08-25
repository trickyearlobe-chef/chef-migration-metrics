// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/export"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipimport"
)

// ---------------------------------------------------------------------------
// Two ownership exports, for two different readers.
//
//   • ownership              — the current state. The shape the source data
//                              should be corrected to match.
//   • ownership_corrections  — the fix-list. What was corrected here, so that
//                              whoever maintains the source system can correct
//                              it there rather than shipping the same faults
//                              again on the next import.
//
// They are separate because they answer different questions. A full state dump
// buries the handful of corrections in thousands of unchanged rows; a fix-list
// alone gives no target to reconcile against.
// ---------------------------------------------------------------------------

func (r *Router) ownershipExportSpec() exportSpec {
	return exportSpec{
		Filename:  "ownership",
		Columns:   ownershipExportColumns(),
		NewSource: newOwnershipExportSource,
	}
}

func newOwnershipExportSource(ctx context.Context, r *Router, req *http.Request) (export.RowSource, error) {
	return &ownershipExportSource{r: r}, nil
}

// ownershipExportSource pages through every assignment. Paged rather than
// materialised because this is the one ownership export whose size follows the
// estate — one assignment row per owned thing, so on a large fleet holding all
// of them to write a file would take the service down.
type ownershipExportSource struct {
	r      *Router
	offset int
	done   bool
}

func (s *ownershipExportSource) Next(ctx context.Context) ([]any, error) {
	if s.done {
		return nil, nil
	}
	rows, err := s.r.db.ListAllAssignments(ctx, exportPageSize, s.offset)
	if err != nil {
		return nil, err
	}
	if len(rows) < exportPageSize {
		s.done = true
	}
	s.offset += len(rows)

	out := make([]any, len(rows))
	for i := range rows {
		out[i] = rows[i]
	}
	return out, nil
}

func ownershipExportColumns() []export.Column {
	as := func(row any) datastore.AllAssignmentRow { return row.(datastore.AllAssignmentRow) }
	return []export.Column{
		{Header: "owner", Value: func(r any) any { return as(r).OwnerName }},
		{Header: "display_name", Value: func(r any) any { return as(r).DisplayName }},
		{Header: "contact_email", Value: func(r any) any { return as(r).ContactEmail }},
		{Header: "entity_type", Value: func(r any) any { return as(r).EntityType }},
		{Header: "entity_key", Value: func(r any) any { return as(r).EntityKey }},
		{Header: "organisation", Value: func(r any) any { return as(r).OrganisationName }},
		// Where this assignment came from. The reader needs it to tell an
		// imported row from one somebody corrected by hand — those are the
		// rows the source system is missing.
		{Header: "source", Value: func(r any) any { return as(r).AssignmentSource }},
		{Header: "confidence", Value: func(r any) any { return as(r).Confidence }},
		{Header: "notes", Value: func(r any) any { return as(r).Notes }},
		{Header: "last_changed", Value: func(r any) any { return as(r).UpdatedAt.UTC().Format("2006-01-02T15:04:05Z") }},
	}
}

// ---------------------------------------------------------------------------
// The fix-list
// ---------------------------------------------------------------------------

// correctionActions are the audit actions that represent somebody correcting
// what the source told us, as opposed to the product doing its job.
//
// Creating an assignment or an owner during an import is the import working, so
// those are absent: including them would bury the handful of real corrections
// under thousands of routine writes and make the file useless for the person it
// is written for.
var correctionActions = []string{
	"owner_merged",
	"assignment_reassigned",
	"assignment_deleted",
	"owner_updated",
	"owner_deleted",
	"owner_duplicate_dismissed",
}

// correctionCategories translate an audit action into what the source system's
// owner has to do about it. The audit log's vocabulary is CMM's; this file is
// read by somebody who has never used CMM.
var correctionCategories = map[string]struct{ category, meaning string }{
	"owner_merged": {"duplicate_person",
		"Two records are the same person. Merge them at source."},
	"assignment_reassigned": {"owner_changed",
		"The source names the wrong owner for this. Correct it at source."},
	"assignment_deleted": {"ownership_removed",
		"The source claims an owner for this that should not be there."},
	"owner_updated": {"owner_details_changed",
		"The person's details were wrong or missing. Correct them at source."},
	"owner_deleted": {"owner_removed",
		"This person should not be in the source at all — they have left, or never existed."},
	"owner_duplicate_dismissed": {"not_duplicates",
		"These two look alike but are different people. Do not merge them."},
}

func (r *Router) ownershipCorrectionsExportSpec() exportSpec {
	return exportSpec{
		Filename:  "ownership_corrections",
		Columns:   correctionExportColumns(),
		NewSource: newCorrectionsExportSource,
	}
}

func newCorrectionsExportSource(ctx context.Context, r *Router, req *http.Request) (export.RowSource, error) {
	return &correctionsExportSource{r: r}, nil
}

// correctionsExportSource reads the corrections somebody made, then the rows
// the source gave that could not be used. Two stores, one file, because they
// are one list of work.
type correctionsExportSource struct {
	r      *Router
	offset int
	// onRejections switches to the second store once the audit log is
	// exhausted. offset resets with it.
	onRejections bool
	done         bool
}

func (s *correctionsExportSource) Next(ctx context.Context) ([]any, error) {
	if s.done {
		return nil, nil
	}

	if !s.onRejections {
		entries, _, err := s.r.db.ListAuditLog(ctx, datastore.AuditLogFilter{
			Actions: correctionActions,
			Limit:   exportPageSize,
			Offset:  s.offset,
		})
		if err != nil {
			return nil, err
		}
		s.offset += len(entries)
		if len(entries) < exportPageSize {
			s.onRejections = true
			s.offset = 0
		}
		out := make([]any, len(entries))
		for i := range entries {
			out[i] = correctionFromAudit(entries[i])
		}
		// An empty page here is not the end of the file — the rejections are
		// still to come, so this returns an empty page rather than nil.
		if len(out) == 0 {
			return s.Next(ctx)
		}
		return out, nil
	}

	rejections, err := s.r.db.ListImportRejections(ctx, exportPageSize, s.offset)
	if err != nil {
		return nil, err
	}
	if len(rejections) < exportPageSize {
		s.done = true
	}
	s.offset += len(rejections)

	out := make([]any, len(rejections))
	for i := range rejections {
		out[i] = correctionFromRejection(rejections[i])
	}
	return out, nil
}

// correctionRow is one line of the fix-list, whatever it was built from.
//
// A neutral shape rather than two column sets, because the reader is one
// person with one list of work. A correction we made and a row we could not
// use are both "something wrong at source"; splitting them into two files
// would make them look like two problems.
type correctionRow struct {
	WhatToFix    string
	WhatItMeans  string
	EntityType   string
	EntityKey    string
	SourceSays   string
	ShouldBe     string
	Reason       string
	SourceImport string
	SourceRow    string
	RecordedBy   string
	RecordedAt   string
}

func correctionExportColumns() []export.Column {
	cr := func(row any) correctionRow { return row.(correctionRow) }
	return []export.Column{
		{Header: "what_to_fix", Value: func(r any) any { return cr(r).WhatToFix }},
		{Header: "what_it_means", Value: func(r any) any { return cr(r).WhatItMeans }},
		{Header: "entity_type", Value: func(r any) any { return cr(r).EntityType }},
		{Header: "entity_key", Value: func(r any) any { return cr(r).EntityKey }},
		{Header: "source_says", Value: func(r any) any { return cr(r).SourceSays }},
		{Header: "should_be", Value: func(r any) any { return cr(r).ShouldBe }},
		{Header: "reason", Value: func(r any) any { return cr(r).Reason }},
		// Which import and which row, so an unusable row can be found in the
		// source rather than merely counted.
		{Header: "source_import", Value: func(r any) any { return cr(r).SourceImport }},
		{Header: "source_row", Value: func(r any) any { return cr(r).SourceRow }},
		{Header: "recorded_by", Value: func(r any) any { return cr(r).RecordedBy }},
		{Header: "recorded_at", Value: func(r any) any { return cr(r).RecordedAt }},
	}
}

func correctionFromAudit(e datastore.OwnershipAuditEntry) correctionRow {
	return correctionRow{
		WhatToFix:   correctionCategory(e.Action),
		WhatItMeans: correctionMeaning(e.Action),
		EntityType:  e.EntityType,
		EntityKey:   e.EntityKey,
		SourceSays:  correctionWas(e),
		ShouldBe:    correctionNow(e),
		Reason:      correctionDetail(e, "reason"),
		RecordedBy:  e.Actor,
		RecordedAt:  e.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// rejectionMeanings say what each rejection reason means to somebody who has
// never used CMM and is looking at their own data.
var rejectionMeanings = map[string]string{
	ownershipimport.ReasonUnknownOwner:         "The source names an owner that could not be identified. Add the person, or correct the name on this row.",
	ownershipimport.ReasonMissingRequiredField: "A value this row needs is empty at source.",
	ownershipimport.ReasonInvalidEntityType:    "The kind of thing being owned is not one that is recognised.",
	ownershipimport.ReasonMalformedRow:         "The row does not have the right number of fields.",
	ownershipimport.ReasonInvalidOwnerName:     "The owner value cannot be turned into a name — it may be blank, punctuation, or a placeholder.",
}

func correctionFromRejection(r datastore.ImportRejection) correctionRow {
	meaning, ok := rejectionMeanings[r.Reason]
	if !ok {
		// A reason code with no sentence yet. Showing the code beats showing
		// nothing: it is still actionable, just less kindly worded.
		meaning = "This row could not be used."
	}
	return correctionRow{
		WhatToFix:   "unusable_row",
		WhatItMeans: meaning,
		EntityType:  r.EntityType,
		EntityKey:   r.EntityKey,
		SourceSays:  r.OwnerRaw,
		// Deliberately empty. Only whoever owns the source can say what this
		// row should have contained; guessing would put an invention into a
		// file somebody is about to act on.
		ShouldBe:     "",
		Reason:       r.Reason,
		SourceImport: r.ImportLabel,
		SourceRow:    strconv.Itoa(r.SourceRow),
		RecordedAt:   r.RunAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func correctionCategory(action string) string {
	if c, ok := correctionCategories[action]; ok {
		return c.category
	}
	return action
}

func correctionMeaning(action string) string {
	if c, ok := correctionCategories[action]; ok {
		return c.meaning
	}
	return ""
}

// correctionWas and correctionNow read the two sides of a correction out of the
// audit entry's details.
//
// The details document differs per action and the key names are NOT consistent
// between them — a merge records `into_owner` while a reassignment records
// `to_owner`, and a dismissal records `owner_a`/`owner_b`. That inconsistency
// is in the writers (`datastore.MergeOwnersResult` vs the reassign handler) and
// predates this file, so the keys are tried in order rather than assumed.
//
// **Checked against what the writers actually emit, not against what looks
// likely.** The first version of this read `to_owner` for a merge and produced
// an empty "should be" column on every real merge in the database — a report
// telling somebody to fix their data, with the fix missing.
//
// An action whose shape is not recognised yields an empty cell, which is
// honest: better a gap than a confidently wrong value in a file somebody is
// about to act on.
func correctionWas(e datastore.OwnershipAuditEntry) string {
	if v := correctionDetail(e, "from_owner"); v != "" {
		return v
	}
	if v := correctionDetail(e, "owner_a"); v != "" {
		return v
	}
	// owner_deleted and assignment_deleted both name the owner on the entry
	// rather than in the details: what the source says is that this person
	// owns this thing, and the correction is that they should not.
	if e.Action == "owner_deleted" || e.Action == "assignment_deleted" {
		return e.OwnerName
	}
	return correctionChangedFields(e)
}

func correctionNow(e datastore.OwnershipAuditEntry) string {
	// into_owner is the merge's word for it; to_owner is the reassignment's.
	if v := correctionDetail(e, "into_owner"); v != "" {
		return v
	}
	if v := correctionDetail(e, "to_owner"); v != "" {
		return v
	}
	if v := correctionDetail(e, "owner_b"); v != "" {
		return v
	}
	if e.Action == "owner_updated" {
		return e.OwnerName
	}
	// A deletion has no "should be" — the correction is that the row should
	// not exist. Left empty rather than invented.
	return ""
}

func correctionDetail(e datastore.OwnershipAuditEntry, key string) string {
	if len(e.Details) == 0 {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal(e.Details, &doc); err != nil {
		return ""
	}
	v, ok := doc[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// correctionChangedFields renders owner_updated's changed_fields list, which is
// what "the details were wrong" actually means in that case.
func correctionChangedFields(e datastore.OwnershipAuditEntry) string {
	if len(e.Details) == 0 {
		return ""
	}
	var doc struct {
		ChangedFields []string `json:"changed_fields"`
	}
	if err := json.Unmarshal(e.Details, &doc); err != nil {
		return ""
	}
	return strings.Join(doc.ChangedFields, ";")
}
