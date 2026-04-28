// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/gitkitchen"
)

// handleGitKitchenInstances handles GET /api/v1/kitchen/git/instances?repo=<name>.
// It runs the planner against the kitchen analysis for the given repo and
// returns the expanded instance list.
func (r *Router) handleGitKitchenInstances(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	repoName := req.URL.Query().Get("repo")
	if repoName == "" {
		WriteBadRequest(w, "repo query parameter is required")
		return
	}

	ctx := req.Context()
	analysis, err := r.db.GetKitchenAnalysisResultByName(ctx, repoName)
	if err != nil {
		r.logf("ERROR", "git kitchen instances: lookup %q: %v", repoName, err)
		WriteInternalError(w, "Failed to retrieve kitchen analysis.")
		return
	}
	if analysis == nil {
		WriteNotFound(w, "No kitchen analysis found for repo.")
		return
	}

	plan, err := gitkitchen.PlanRepo(*analysis, r.cfg.AnalysisTools.TestKitchen.PlatformMap)
	if err != nil {
		r.logf("ERROR", "git kitchen instances: plan %q: %v", repoName, err)
		WriteInternalError(w, "Failed to plan kitchen instances.")
		return
	}

	WriteJSON(w, http.StatusOK, plan)
}

// handleGitKitchenResults handles GET /api/v1/kitchen/git/results[?repo=<name>].
// Returns all git kitchen results, optionally filtered by repo name.
func (r *Router) handleGitKitchenResults(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	ctx := req.Context()
	repoName := req.URL.Query().Get("repo")

	if repoName != "" {
		results, err := r.db.ListGitKitchenResultsByRepo(ctx, repoName)
		if err != nil {
			r.logf("ERROR", "git kitchen results by repo %q: %v", repoName, err)
			WriteInternalError(w, "Failed to retrieve git kitchen results.")
			return
		}
		WriteJSON(w, http.StatusOK, results)
		return
	}

	results, err := r.db.ListGitKitchenResults(ctx)
	if err != nil {
		r.logf("ERROR", "git kitchen results: %v", err)
		WriteInternalError(w, "Failed to retrieve git kitchen results.")
		return
	}
	WriteJSON(w, http.StatusOK, results)
}

// gitKitchenRunRequest is the request body for POST /api/v1/kitchen/git/run.
type gitKitchenRunRequest struct {
	GitRepoName      string `json:"git_repo_name"`
	InstanceName     string `json:"instance_name"`
	TargetChefVersion string `json:"target_chef_version"`
}

// handleGitKitchenRun handles POST /api/v1/kitchen/git/run.
// It validates the request, plans the repo, and dispatches a single instance
// run asynchronously via the scheduler.
func (r *Router) handleGitKitchenRun(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}

	if r.gitKitchenScheduler == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Git kitchen scheduler is not configured.")
		return
	}

	var body gitKitchenRunRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid JSON request body.")
		return
	}

	if body.GitRepoName == "" {
		WriteBadRequest(w, "git_repo_name is required")
		return
	}
	if body.InstanceName == "" {
		WriteBadRequest(w, "instance_name is required")
		return
	}
	if body.TargetChefVersion == "" {
		WriteBadRequest(w, "target_chef_version is required")
		return
	}

	ctx := req.Context()
	analysis, err := r.db.GetKitchenAnalysisResultByName(ctx, body.GitRepoName)
	if err != nil {
		r.logf("ERROR", "git kitchen run: lookup %q: %v", body.GitRepoName, err)
		WriteInternalError(w, "Failed to retrieve kitchen analysis.")
		return
	}
	if analysis == nil {
		WriteNotFound(w, "No kitchen analysis found for repo.")
		return
	}

	plan, err := gitkitchen.PlanRepo(*analysis, r.cfg.AnalysisTools.TestKitchen.PlatformMap)
	if err != nil {
		r.logf("ERROR", "git kitchen run: plan %q: %v", body.GitRepoName, err)
		WriteInternalError(w, "Failed to plan kitchen instances.")
		return
	}

	cfg := gitkitchen.SchedulerConfig{
		MaxConcurrency:    1,
		TargetChefVersion: body.TargetChefVersion,
	}

	// Detach from the HTTP request context so the kitchen run is not
	// cancelled when the response is sent. A cancelled context would kill
	// the kitchen process mid-flight, potentially orphaning VMs on the
	// hypervisor before the destroy phase completes.
	bgCtx := context.WithoutCancel(ctx)

	go func() {
		_, runErr := r.gitKitchenScheduler.RunOne(bgCtx, plan, body.InstanceName, cfg)
		if runErr != nil {
			r.logf("ERROR", "git kitchen run async: %v", runErr)
		}
	}()

	WriteJSON(w, http.StatusAccepted, map[string]string{
		"message": "Run dispatched for instance " + body.InstanceName,
	})
}
