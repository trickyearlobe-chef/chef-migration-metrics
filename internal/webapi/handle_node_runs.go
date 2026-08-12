// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// handleNodeRuns serves GET /api/v1/nodes/runs/{organisation}/{node} — the recent
// converge runs for a node, most-recent first, for the Node Detail Runs panel.
//
// converge_runs keys on the delivered chef organization_name, which is the org's
// OrgName (chef slug), NOT its CMM display Name — so we resolve the URL org name
// and query by org.OrgName. A node with no runs returns an empty list, not 404
// (short-retention telemetry may simply not exist yet).
// nodeRunsResponse is the recent converge runs for one machine.
//
// Capped rather than paged — the store call behind it takes a limit and no
// offset — which is why the address describes per_page and no page. See
// subCappedNotPaged.
type nodeRunsResponse struct {
	Data []datastore.ConvergeRunView `json:"data"`
}

func (r *Router) handleNodeRuns(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	// The Node Detail Runs tab is part of the Run events feature — hidden when
	// ingest.show_run_events is off (feature in reserve).
	if r.runEventsHidden(w) {
		return
	}

	segs := pathSegments(req.URL.Path, "/api/v1/nodes/runs/")
	if len(segs) < 2 || segs[0] == "" || segs[1] == "" {
		WriteBadRequest(w, "Expected /api/v1/nodes/runs/{organisation}/{node}.")
		return
	}
	orgName := segs[0]
	nodeName := strings.Join(segs[1:], "/")

	ctx := req.Context()
	org, err := r.db.GetOrganisationByName(ctx, orgName)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteError(w, http.StatusNotFound, ErrCodeNotFound, "Organisation not found.")
			return
		}
		r.logf("ERROR", "node runs: get organisation %q: %v", orgName, err)
		WriteInternalError(w, "Failed to load organisation.")
		return
	}

	params := ParsePagination(req)
	runs, err := r.db.ListConvergeRunsForNode(ctx, org.OrgName, nodeName, params.Limit())
	if err != nil {
		r.logf("ERROR", "node runs: list %s/%s: %v", org.OrgName, nodeName, err)
		WriteInternalError(w, "Failed to load runs.")
		return
	}
	if runs == nil {
		runs = []datastore.ConvergeRunView{}
	}

	WriteJSON(w, http.StatusOK, nodeRunsResponse{Data: runs})
}
