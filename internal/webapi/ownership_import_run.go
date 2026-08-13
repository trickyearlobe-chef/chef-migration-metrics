// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipimport"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipschedule"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ownershipsql"
)

// ---------------------------------------------------------------------------
// Running a saved import with nobody watching
//
// The interactive path builds an import out of a form: the source, the mapping,
// the row filter and the create-owners switch all arrive per request. A
// scheduled run has no form, so everything it needs comes from the saved
// definition instead — which is why those options are stored (migration 0064)
// rather than left as defaults to be re-applied.
//
// It lives beside the HTTP handlers because it shares their classify-and-commit
// seam, not because it is part of the API. Recorded as tech debt: the import
// service wants extracting out of the web layer.
// ---------------------------------------------------------------------------

// ImportRunSummary is what an unattended run leaves behind. It is deliberately
// small: the per-row detail goes to the ownership audit log, and this is what
// the list has to show so that "scheduled" and "working" stay different claims.
type ImportRunSummary struct {
	RowCount    int            `json:"row_count"`
	FilteredOut int            `json:"filtered_out"`
	Counts      map[string]int `json:"counts"`

	// DurationMS is how long the run took, wall clock, including reading the
	// source. The decision to put an import on a schedule turns on it: a job
	// that takes forty minutes is a different proposition from one that takes
	// four, and until this was reported the only way to find out was to sit and
	// watch one. Zero when the run did not complete.
	DurationMS int64 `json:"duration_ms"`
}

// runOutcomeLabels name each outcome in the past tense.
//
// A scheduled run always commits, so the preview vocabulary the report carries
// would be a lie here: would_create rows were created. Reading "8 would create"
// in a history of things that already happened invites exactly the wrong
// conclusion — that the run stopped short of writing.
var runOutcomeLabels = []struct{ outcome, label string }{
	{ownershipimport.OutcomeWouldCreate, "created"},
	{ownershipimport.OutcomeOwnedByOther, "also owned by somebody else"},
	{ownershipimport.OutcomeDuplicateExists, "already there"},
	{ownershipimport.OutcomeRejected, "rejected"},
}

// String renders the summary for the run history, in the terms somebody
// reading a list of imports actually asks in: how many rows, and how many of
// them needed somebody to do something.
func (s ImportRunSummary) String() string {
	parts := []string{fmt.Sprintf("%d rows", s.RowCount)}
	for _, o := range runOutcomeLabels {
		if n := s.Counts[o.outcome]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, o.label))
		}
	}
	if s.FilteredOut > 0 {
		parts = append(parts, fmt.Sprintf("%d filtered out", s.FilteredOut))
	}
	if s.DurationMS > 0 {
		parts = append(parts, fmt.Sprintf("in %s", (time.Duration(s.DurationMS)*time.Millisecond).Round(time.Millisecond)))
	}
	return strings.Join(parts, ", ")
}

// scheduledImportActor names the schedule in the audit log.
//
// The audit log has to answer "who changed this". For a scheduled run the
// honest answer is the saved import that did it, not the administrator who set
// it up months ago and not "unknown" — which is what an unattributed write
// would look like, and is indistinguishable from an unauthenticated one.
func scheduledImportActor(m datastore.ImportMapping) string {
	return "scheduled import: " + m.Name
}

// RunSavedImport opens the source a saved import names and runs it to a commit.
//
// Only a database import can run this way. A file import has no stored source —
// somebody has to bring the file — so there is nothing for a schedule to read.
func (r *Router) RunSavedImport(ctx context.Context, m datastore.ImportMapping) (ImportRunSummary, error) {
	startedAt := time.Now()
	if m.SourceKind != intakeSourceDatabase {
		return ImportRunSummary{}, fmt.Errorf("import %q reads a file, so it cannot run unattended", m.Name)
	}

	src, err := r.openSavedDatabaseSource(ctx, m)
	if err != nil {
		return ImportRunSummary{}, err
	}
	defer func() { _ = src.Close() }()

	summary, err := r.runImportFromSource(ctx, src, m)
	if err != nil {
		return ImportRunSummary{}, err
	}
	// Timed around the whole run, connection included: an import that takes
	// forty minutes because the query is slow and one that takes forty minutes
	// because the server is far away are the same decision to the person
	// deciding whether to schedule it.
	summary.DurationMS = time.Since(startedAt).Milliseconds()
	return summary, nil
}

// openSavedDatabaseSource resolves the connection the import names and connects.
//
// A saved import holds the *name* of a connection, exactly as the interactive
// path does, so scheduling never becomes the reason a password is stored in
// plain text — and the two paths reach a database through the same code, so a
// connection that works when somebody watches works when nobody does.
func (r *Router) openSavedDatabaseSource(ctx context.Context, m datastore.ImportMapping) (ownershipimport.RowSource, error) {
	if strings.TrimSpace(m.DBQuery) == "" {
		return nil, fmt.Errorf("import %q has no query to run", m.Name)
	}

	cfg, cleanup, err := r.connectionConfig(ctx, m.DBConnection)
	if err != nil {
		return nil, fmt.Errorf("import %q cannot connect: %w", m.Name, err)
	}
	defer cleanup()
	cfg.Query = m.DBQuery

	src, err := ownershipsql.Open(ctx, cfg)
	if err != nil {
		// An unreadable source and an empty one look the same in a row count
		// and mean opposite things, so this is an error rather than nil rows.
		return nil, fmt.Errorf("import %q could not read from the database: %w", m.Name, err)
	}
	return src, nil
}

// runImportFromSource maps, classifies and commits every row the source yields.
//
// Takes the source rather than opening one so the decisions here — the filter,
// the row cap, what happens when a read fails half way — can be tested without
// a database.
func (r *Router) runImportFromSource(ctx context.Context, src ownershipimport.RowSource, m datastore.ImportMapping) (ImportRunSummary, error) {
	var fieldMap ownershipimport.FieldMap
	if err := json.Unmarshal(m.FieldMap, &fieldMap); err != nil {
		return ImportRunSummary{}, fmt.Errorf("import %q has an unreadable column mapping: %w", m.Name, err)
	}

	columns := src.Columns()
	if errs := fieldMap.Validate(columns); len(errs) > 0 {
		// The source changed shape under a mapping that used to fit it — a
		// renamed column, most likely. Importing what still matches would
		// quietly drop whatever the missing column fed.
		return ImportRunSummary{}, fmt.Errorf("import %q no longer fits its source: %s", m.Name, formatMappingErrors(errs))
	}

	mapper, err := ownershipimport.NewMapper(fieldMap, columns)
	if err != nil {
		return ImportRunSummary{}, fmt.Errorf("import %q has an unusable column mapping: %w", m.Name, err)
	}

	if m.FilterColumn != "" && !slicesContains(columns, m.FilterColumn) {
		return ImportRunSummary{}, fmt.Errorf("import %q filters on %q, which the source no longer has", m.Name, m.FilterColumn)
	}

	mapped := make([]ownershipimport.MappedRow, 0, 64)
	filteredOut := 0
	for src.Next() {
		row := src.Row()
		if m.FilterColumn != "" &&
			!strings.EqualFold(strings.TrimSpace(row.Values[m.FilterColumn]), m.FilterValue) {
			filteredOut++
			continue
		}
		if len(mapped) >= intakeMaxRows {
			return ImportRunSummary{}, fmt.Errorf(
				"import %q reads more than the %d row limit; narrow its query or its filter", m.Name, intakeMaxRows)
		}
		mapped = append(mapped, mapper.MapRow(row))
	}
	if err := src.Err(); err != nil {
		// A read that failed part way through must not be recorded as a short
		// but successful import — that is how a partial import gets believed.
		return ImportRunSummary{}, fmt.Errorf("import %q failed while reading its source: %w", m.Name, err)
	}

	report := r.classifyIntakeRows(ctx, mapped, m.CreateOwners)
	report.FilteredOut = filteredOut
	r.commitIntakeRows(ctx, scheduledImportActor(m), &report)

	// After the commit, because committing can reject a row the classifier
	// accepted — a write that failed, or an owner that could not be created.
	r.storeImportRejections(ctx, m.Name, &m.ID, report)

	return ImportRunSummary{
		RowCount:    report.RowCount,
		FilteredOut: report.FilteredOut,
		Counts:      report.Counts,
	}, nil
}

func slicesContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// POST /api/v1/ownership/import/mappings/{id}/run
// ---------------------------------------------------------------------------

// runSavedImportNow runs a saved import on demand and answers with what it did.
//
// Synchronous, which is deliberate: this exists for judging a source — import
// it, look at what arrived, adjust the query, go again — and a call that
// answered "started" would make the person poll for the thing they asked for.
// The trade is that a very large import can outlast a proxy's patience; the
// schedule is the right tool for those, and it records its own outcome.
//
// POST rather than GET because it writes. A link, a prefetch or a refresh must
// not be able to import ownership.
func (r *Router) runSavedImportNow(w http.ResponseWriter, req *http.Request, id int64) {
	if !requireMethod(w, req, http.MethodPost) {
		return
	}

	mapping, err := r.db.GetImportMapping(req.Context(), id)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("No import mapping with id %d.", id))
		return
	}
	if err != nil {
		r.logf("ERROR", "ownership/import: loading mapping %d to run it: %v", id, err)
		WriteInternalError(w, "Failed to load the import.")
		return
	}

	summary, runErr := r.RunSavedImport(req.Context(), mapping)
	if runErr != nil {
		// Recorded as well as returned. A run somebody watched fail still
		// belongs in the history, or the list would show the last *successful*
		// run and read as though nothing had gone wrong since.
		r.recordImportRun(req.Context(), mapping, ownershipschedule.StatusFailed, runErr.Error())
		WriteBadRequest(w, runErr.Error())
		return
	}

	r.recordImportRun(req.Context(), mapping, ownershipschedule.StatusSucceeded, summary.String())

	WriteJSON(w, http.StatusOK, map[string]any{
		"summary": summary,
		"detail":  summary.String(),
	})
}

// recordImportRun stores the outcome, logging rather than failing if it cannot.
// The import has already happened by this point, so reporting a bookkeeping
// failure as an import failure would be a lie in the more misleading direction.
func (r *Router) recordImportRun(ctx context.Context, m datastore.ImportMapping, status, detail string) {
	if err := r.db.RecordImportRun(ctx, m.ID, status, detail); err != nil {
		r.logf("WARN", "ownership/import: could not record the run of %q: %v", m.Name, err)
	}
}

// ---------------------------------------------------------------------------
// GET/POST /api/v1/ownership/import/clear
// ---------------------------------------------------------------------------

// handleClearImportedOwnership previews (GET) or performs (POST) the removal of
// everything imports brought in.
//
// It exists for judging a source. Without it a second trial import is judged
// against the residue of the first, and "is this data any good" becomes
// unanswerable.
//
// GET and POST are the same endpoint because they answer the same question in
// two tenses — what would go, and what went — but they run different queries.
// A dry-run flag on the delete would be one edit away from the preview doing
// the deleting.
func (r *Router) handleClearImportedOwnership(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		counts, err := r.db.CountImportedOwnership(req.Context())
		if err != nil {
			r.logf("ERROR", "ownership/import: counting imported ownership: %v", err)
			WriteInternalError(w, "Failed to work out what an import has brought in.")
			return
		}
		WriteJSON(w, http.StatusOK, counts)

	case http.MethodPost:
		cleared, err := r.db.ClearImportedOwnership(req.Context())
		if err != nil {
			r.logf("ERROR", "ownership/import: clearing imported ownership: %v", err)
			WriteInternalError(w, "Failed to remove the imported ownership.")
			return
		}

		// Removing ownership in bulk is exactly the kind of act somebody later
		// asks "who did that, and when?" about.
		details, _ := json.Marshal(cleared)
		r.auditOwnership(req, "imported_ownership_cleared", "", "", "", "", details)

		r.logf("INFO", "ownership/import: cleared %d imported assignments and %d imported owners",
			cleared.Assignments, cleared.Owners)
		WriteJSON(w, http.StatusOK, cleared)

	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "Method not allowed.")
	}
}

// ---------------------------------------------------------------------------
// Keeping the rows a source could not give us
// ---------------------------------------------------------------------------

// storeImportRejections keeps the rows the import could not use, so they can be
// sent back to whoever maintains the source.
//
// Called on every commit, including a clean one: the stored set is replaced,
// and a run with nothing wrong has to clear the last run's findings. Otherwise
// a source somebody has fixed still reads as broken and the list can never be
// worked down to nothing.
//
// Failures are logged and dropped. The assignments are already written by the
// time this runs, so reporting a bookkeeping failure as an import failure would
// be a lie in the more misleading direction.
func (r *Router) storeImportRejections(ctx context.Context, label string, mappingID *int64, report ownershipimport.Report) {
	if label == "" {
		return
	}

	rejections := make([]datastore.ImportRejection, 0, 16)
	for _, row := range report.Rows {
		if row.Outcome != ownershipimport.OutcomeRejected {
			continue
		}
		rejections = append(rejections, datastore.ImportRejection{
			SourceRow:  row.SourceRow,
			Reason:     row.RejectedReason,
			OwnerRaw:   row.OwnerRaw,
			EntityType: row.EntityType,
			EntityKey:  row.EntityKey,
		})
	}

	stored, err := r.db.ReplaceImportRejections(ctx, label, mappingID, rejections)
	if err != nil {
		r.logf("WARN", "ownership/import: could not store the unusable rows for %q: %v", label, err)
		return
	}
	if stored < len(rejections) {
		// Said out loud rather than truncated silently: "1000 of 40000 shown"
		// and "1000 unusable rows" are very different statements about a source.
		r.logf("INFO", "ownership/import: %q had %d unusable rows; kept the first %d",
			label, len(rejections), stored)
	}
}
