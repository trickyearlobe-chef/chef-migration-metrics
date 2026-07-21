// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Run events — a top-level view over converge_runs (ingest telemetry).
//
// Two tabs, two grains, one shared filter set (see specifications/event-ingest.md):
//   - Nodes tab: GET /api/v1/run-events/nodes   — distinct-node rollup, EXISTS
//     semantics (a node appears if ANY run matches all filters; row = latest
//     matching run). The default surface.
//   - Runs tab:  GET /api/v1/run-events/runs    — flat run list (one row/run).
//   - Detail:    GET /api/v1/run-events/nodes/{organisation}/{node} — every run
//     for one node, keyed by the DELIVERED org name (NOT organisations.name
//     resolution), so DMZ ingest-only nodes resolve.
//
// Filters are sourced from converge_runs itself and must NOT use the global
// organisations-table org filter (it never contains ingest-only DMZ orgs).
// ---------------------------------------------------------------------------

// runEventsHidden writes a 404 and reports true when the Run events feature is
// switched off (ingest.show_run_events). Gates every display/read surface so the
// feature is fully dormant in reserve, not just hidden in the nav.
func (r *Router) runEventsHidden(w http.ResponseWriter) bool {
	if r.liveConfig().Ingest.ShowsRunEvents() {
		return false
	}
	WriteNotFound(w, "Run events are not enabled.")
	return true
}

// convergeRunFilterFromValues builds the run-events filter from raw query values.
// Shared by both list handlers and the export path (Limit/Offset applied by the
// caller) so an export reproduces the list's filtering exactly.
func convergeRunFilterFromValues(q url.Values) datastore.ConvergeRunFilter {
	f := datastore.ConvergeRunFilter{
		Organisation:   valueOr(q, "organisation", ""),
		Status:         valueOr(q, "status", ""),
		NodeName:       valueOr(q, "node", ""),
		ChefVersion:    valueOr(q, "chef_version", ""),
		Cookbook:       valueOr(q, "cookbook", ""),
		FailureMessage: valueOr(q, "failure_message", ""),
		Sort:           valueOr(q, "sort", "end_time"),
		SortOrder:      valueOr(q, "order", "desc"),
	}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.EndTimeFrom = t
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.EndTimeTo = t
		}
	}
	return f
}

// handleRunEventNodes handles GET /api/v1/run-events/nodes — the distinct-node
// rollup (default tab).
func (r *Router) handleRunEventNodes(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	if r.runEventsHidden(w) {
		return
	}
	ctx := req.Context()

	f := convergeRunFilterFromValues(req.URL.Query())
	pg := ParsePagination(req)
	f.Limit = pg.PerPage
	f.Offset = (pg.Page - 1) * pg.PerPage

	nodes, total, err := r.db.ListConvergeRunNodesFiltered(ctx, f)
	if err != nil {
		r.logf("ERROR", "run-events: listing nodes (filtered): %v", err)
		WriteInternalError(w, "Failed to list run-event nodes.")
		return
	}
	if nodes == nil {
		nodes = []datastore.ConvergeRunListItem{}
	}
	WritePaginated(w, nodes, pg, total)
}

// handleRunEventRuns handles GET /api/v1/run-events/runs — the flat run list.
func (r *Router) handleRunEventRuns(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	if r.runEventsHidden(w) {
		return
	}
	ctx := req.Context()

	f := convergeRunFilterFromValues(req.URL.Query())
	pg := ParsePagination(req)
	f.Limit = pg.PerPage
	f.Offset = (pg.Page - 1) * pg.PerPage

	runs, total, err := r.db.ListConvergeRunsFiltered(ctx, f)
	if err != nil {
		r.logf("ERROR", "run-events: listing runs (filtered): %v", err)
		WriteInternalError(w, "Failed to list run events.")
		return
	}
	if runs == nil {
		runs = []datastore.ConvergeRunListItem{}
	}
	WritePaginated(w, runs, pg, total)
}

// handleRunEventNodeDetail handles GET /api/v1/run-events/nodes/{organisation}/{node}
// — every converge run for one node, most-recent first. It keys on the DELIVERED
// org name directly (no organisations-table resolution) so ingest-only DMZ nodes
// resolve.
func (r *Router) handleRunEventNodeDetail(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	if r.runEventsHidden(w) {
		return
	}
	segs := pathSegments(req.URL.Path, "/api/v1/run-events/nodes/")
	if len(segs) < 2 || segs[0] == "" || segs[1] == "" {
		WriteBadRequest(w, "Expected /api/v1/run-events/nodes/{organisation}/{node}.")
		return
	}
	organisation := segs[0]
	nodeName := strings.Join(segs[1:], "/")

	params := ParsePagination(req)
	runs, err := r.db.ListConvergeRunsForNode(req.Context(), organisation, nodeName, params.Limit())
	if err != nil {
		r.logf("ERROR", "run-events: node detail %s/%s: %v", organisation, nodeName, err)
		WriteInternalError(w, "Failed to load runs.")
		return
	}
	if runs == nil {
		runs = []datastore.ConvergeRunView{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"organisation": organisation,
		"node":         nodeName,
		"data":         runs,
	})
}

// handleFilterRunOrganisations handles GET /api/v1/filters/run-organisations —
// the org filter's option source, from converge_runs (NOT the organisations
// table), so ingest-only DMZ orgs are selectable.
func (r *Router) handleFilterRunOrganisations(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	if r.runEventsHidden(w) {
		return
	}
	orgs, err := r.db.ListConvergeRunOrganisations(req.Context())
	if err != nil {
		r.logf("ERROR", "run-events: listing organisations: %v", err)
		WriteInternalError(w, "Failed to list organisations.")
		return
	}
	if orgs == nil {
		orgs = []string{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": orgs})
}

// handleFilterRunChefVersions handles GET /api/v1/filters/run-chef-versions —
// the chef_version filter's option source, from converge_runs.
func (r *Router) handleFilterRunChefVersions(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	if r.runEventsHidden(w) {
		return
	}
	versions, err := r.db.ListConvergeRunChefVersions(req.Context())
	if err != nil {
		r.logf("ERROR", "run-events: listing chef versions: %v", err)
		WriteInternalError(w, "Failed to list chef versions.")
		return
	}
	if versions == nil {
		versions = []string{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": versions})
}
