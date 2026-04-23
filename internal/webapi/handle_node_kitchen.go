// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/nodekitchen"
)

// handleNodeKitchenTrigger starts an asynchronous Node Kitchen run.
//
//	POST /api/v1/kitchen/node-run
func (r *Router) handleNodeKitchenTrigger(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}

	var body nodekitchen.RunRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, fmt.Sprintf("Invalid JSON body: %v", err))
		return
	}

	if body.NodeName == "" {
		WriteBadRequest(w, "node_name is required.")
		return
	}
	if body.OrganisationName == "" {
		WriteBadRequest(w, "organisation_name is required.")
		return
	}
	if body.TargetChefVersion == "" {
		WriteBadRequest(w, "target_chef_version is required.")
		return
	}
	switch body.CookbookSource {
	case "server", "git", "hybrid":
		// Valid.
	default:
		WriteBadRequest(w, fmt.Sprintf("Invalid cookbook_source %q; must be server, git, or hybrid.", body.CookbookSource))
		return
	}

	if r.nodeKitchenRunner == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Node Kitchen runner is not configured.")
		return
	}

	go r.nodeKitchenRunner.Run(context.Background(), body)

	WriteJSON(w, http.StatusAccepted, map[string]string{
		"status": "started",
		"message": fmt.Sprintf("Node Kitchen run started for %s/%s. Poll GET /api/v1/kitchen/node-runs?org=%s&node=%s for results.",
			body.OrganisationName, body.NodeName, body.OrganisationName, body.NodeName),
	})
}

// handleNodeKitchenRuns dispatches GET for /api/v1/kitchen/node-runs and
// routes /api/v1/kitchen/node-runs/{id} to the detail or delete handler.
//
//	GET /api/v1/kitchen/node-runs?org=<org>&node=<node>
func (r *Router) handleNodeKitchenRuns(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	org := req.URL.Query().Get("org")
	if org == "" {
		WriteBadRequest(w, "Query parameter 'org' is required.")
		return
	}

	node := req.URL.Query().Get("node")
	ctx := req.Context()

	var (
		runs []datastore.NodeKitchenRun
		err  error
	)
	if node != "" {
		runs, err = r.db.ListNodeKitchenRunsByNode(ctx, org, node)
	} else {
		runs, err = r.db.ListNodeKitchenRuns(ctx, org)
	}
	if err != nil {
		r.logf("ERROR", "node-kitchen-runs: listing runs: %v", err)
		WriteInternalError(w, "Failed to list node kitchen runs.")
		return
	}

	if runs == nil {
		runs = []datastore.NodeKitchenRun{}
	}
	WriteJSON(w, http.StatusOK, runs)
}

// handleNodeKitchenRunDetail handles GET and DELETE for a single node
// kitchen run identified by the trailing path segment.
//
//	GET    /api/v1/kitchen/node-runs/<id>
//	DELETE /api/v1/kitchen/node-runs/<id>
func (r *Router) handleNodeKitchenRunDetail(w http.ResponseWriter, req *http.Request) {
	id := pathParam(req, "/api/v1/kitchen/node-runs/")
	if id == "" {
		WriteNotFound(w, "Run ID is required in the path.")
		return
	}

	switch req.Method {
	case http.MethodGet:
		r.handleGetNodeKitchenRun(w, req, id)
	case http.MethodDelete:
		r.handleDeleteNodeKitchenRun(w, req, id)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and DELETE.")
	}
}

func (r *Router) handleGetNodeKitchenRun(w http.ResponseWriter, req *http.Request, id string) {
	run, err := r.db.GetNodeKitchenRun(req.Context(), id)
	if err != nil {
		r.logf("ERROR", "node-kitchen-runs: getting run %s: %v", id, err)
		WriteInternalError(w, "Failed to get node kitchen run.")
		return
	}
	if run == nil {
		WriteNotFound(w, fmt.Sprintf("Node kitchen run %q not found.", id))
		return
	}
	WriteJSON(w, http.StatusOK, run)
}

func (r *Router) handleDeleteNodeKitchenRun(w http.ResponseWriter, req *http.Request, id string) {
	err := r.db.DeleteNodeKitchenRun(req.Context(), id)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Node kitchen run %q not found.", id))
			return
		}
		r.logf("ERROR", "node-kitchen-runs: deleting run %s: %v", id, err)
		WriteInternalError(w, "Failed to delete node kitchen run.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
