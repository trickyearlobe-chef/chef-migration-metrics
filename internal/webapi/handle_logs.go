// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// handleLogs handles GET /api/v1/logs — lists log entries with optional
// filtering by scope, severity, organisation, cookbook, collection run, and
// time range. Results are paginated.
func (r *Router) handleLogs(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	pg := ParsePagination(req)
	q := req.URL.Query()

	filter := datastore.LogEntryFilter{
		Scope:           q.Get("scope"),
		Severity:        q.Get("severity"),
		MinSeverity:     q.Get("min_severity"),
		Organisation:    q.Get("organisation"),
		CookbookName:    q.Get("cookbook_name"),
		CollectionRunID: q.Get("collection_run_id"),
		Limit:           pg.Limit(),
		Offset:          pg.Offset(),
	}

	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			WriteBadRequest(w, fmt.Sprintf("Invalid 'since' parameter: %v", err))
			return
		}
		filter.Since = t
	}

	if v := q.Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			WriteBadRequest(w, fmt.Sprintf("Invalid 'until' parameter: %v", err))
			return
		}
		filter.Until = t
	}

	// Get total count for pagination metadata.
	total, err := r.db.CountLogEntries(req.Context(), filter)
	if err != nil {
		r.logf("ERROR", "counting log entries: %v", err)
		WriteInternalError(w, "Failed to count log entries.")
		return
	}

	entries, err := r.db.ListLogEntries(req.Context(), filter)
	if err != nil {
		r.logf("ERROR", "listing log entries: %v", err)
		WriteInternalError(w, "Failed to list log entries.")
		return
	}

	if entries == nil {
		entries = []datastore.LogEntry{}
	}

	WritePaginated(w, entries, pg, total)
}

// handleLogDetail handles GET /api/v1/logs/:id — returns a single log entry
// by its UUID. The route is registered as a prefix pattern on /api/v1/logs/
// so we need to handle sub-path routing carefully — /api/v1/logs/collection-runs
// is registered with a more specific pattern and will match first.
func (r *Router) handleLogDetail(w http.ResponseWriter, req *http.Request) {
	id := pathParam(req, "/api/v1/logs/")
	if id == "" {
		WriteNotFound(w, "Log entry ID is required.")
		return
	}

	// Guard against sub-paths that should not reach this handler. The
	// ServeMux should have already routed /api/v1/logs/collection-runs to
	// handleCollectionRuns, but be defensive about unexpected sub-paths.
	segs := pathSegments(req.URL.Path, "/api/v1/logs/")
	if len(segs) != 1 {
		WriteNotFound(w, fmt.Sprintf("Log endpoint %s not found.", req.URL.Path))
		return
	}

	if !requireGET(w, req) {
		return
	}

	entry, err := r.db.GetLogEntry(req.Context(), id)
	if errors.Is(err, datastore.ErrNotFound) {
		WriteNotFound(w, fmt.Sprintf("Log entry %q not found.", id))
		return
	}
	if err != nil {
		r.logf("ERROR", "getting log entry %s: %v", id, err)
		WriteInternalError(w, "Failed to get log entry.")
		return
	}

	WriteJSON(w, http.StatusOK, entry)
}

// handleCollectionRuns handles GET /api/v1/logs/collection-runs — lists
// collection runs across all organisations, optionally filtered by
// organisation and status. Results are ordered by started_at descending.
func (r *Router) handleCollectionRuns(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	orgs, err := r.db.ListOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "listing organisations for collection runs: %v", err)
		WriteInternalError(w, "Failed to list collection runs.")
		return
	}

	q := req.URL.Query()
	orgFilter := q.Get("organisation")
	statusFilter := q.Get("status")
	limitParam := 0 // 0 = no limit from the datastore method

	type runWithOrg struct {
		OrganisationName string                  `json:"organisation_name"`
		Run              datastore.CollectionRun `json:"run"`
	}

	var allRuns []runWithOrg
	for _, org := range orgs {
		if orgFilter != "" && org.Name != orgFilter {
			continue
		}

		runs, err := r.db.ListCollectionRuns(req.Context(), org.ID, limitParam)
		if err != nil {
			r.logf("WARN", "listing collection runs for org %s: %v", org.Name, err)
			continue
		}
		for _, run := range runs {
			if statusFilter != "" && run.Status != statusFilter {
				continue
			}
			allRuns = append(allRuns, runWithOrg{
				OrganisationName: org.Name,
				Run:              run,
			})
		}
	}

	// Paginate the combined results.
	pg := ParsePagination(req)
	total := len(allRuns)
	start := pg.Offset()
	if start > total {
		start = total
	}
	end := start + pg.Limit()
	if end > total {
		end = total
	}

	if allRuns == nil {
		allRuns = []runWithOrg{}
	}

	WritePaginated(w, allRuns[start:end], pg, total)
}
