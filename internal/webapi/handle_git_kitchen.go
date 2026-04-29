// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
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

	// Resolve effective TK config: database override first, then file.
	var tkCfg config.TestKitchenConfig
	setting, settingErr := r.db.GetRuntimeSetting(ctx, "test_kitchen")
	if settingErr != nil {
		r.logf("ERROR", "git kitchen instances: load runtime setting: %v", settingErr)
		WriteInternalError(w, "Failed to load Test Kitchen configuration.")
		return
	}
	if setting != nil {
		if unmarshalErr := json.Unmarshal(setting.Value, &tkCfg); unmarshalErr != nil {
			r.logf("ERROR", "git kitchen instances: parse stored config: %v", unmarshalErr)
			WriteInternalError(w, "Failed to parse stored Test Kitchen configuration.")
			return
		}
	} else {
		tkCfg = r.liveConfig().AnalysisTools.TestKitchen
	}

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

	// Load user exclusions for this repo.
	exclusions, exclErr := r.loadInstanceExclusions(ctx, repoName)
	if exclErr != nil {
		r.logf("ERROR", "git kitchen instances: load exclusions for %q: %v", repoName, exclErr)
		WriteInternalError(w, "Failed to load kitchen exclusions.")
		return
	}

	plan, err := gitkitchen.PlanRepo(*analysis, tkCfg.PlatformMap, exclusions...)
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
		if results == nil {
			results = []datastore.GitKitchenResult{}
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
	if results == nil {
		results = []datastore.GitKitchenResult{}
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
// It validates the request, plans the repo, and enqueues a single instance
// via the kitchen queue (or falls back to direct dispatch if queue is not configured).
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

	// Resolve effective TK config: database override first, then file.
	var tkCfg config.TestKitchenConfig
	setting, settingErr := r.db.GetRuntimeSetting(ctx, "test_kitchen")
	if settingErr != nil {
		r.logf("ERROR", "git kitchen run: load runtime setting: %v", settingErr)
		WriteInternalError(w, "Failed to load Test Kitchen configuration.")
		return
	}
	if setting != nil {
		if unmarshalErr := json.Unmarshal(setting.Value, &tkCfg); unmarshalErr != nil {
			r.logf("ERROR", "git kitchen run: parse stored config: %v", unmarshalErr)
			WriteInternalError(w, "Failed to parse stored Test Kitchen configuration.")
			return
		}
	} else {
		tkCfg = r.liveConfig().AnalysisTools.TestKitchen
	}

	if !tkCfg.IsEnabled() {
		WriteError(w, http.StatusConflict, "conflict", "Test Kitchen is disabled.")
		return
	}

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

	exclusions, exclErr := r.loadInstanceExclusions(ctx, body.GitRepoName)
	if exclErr != nil {
		r.logf("ERROR", "git kitchen run: load exclusions for %q: %v", body.GitRepoName, exclErr)
		WriteInternalError(w, "Failed to load kitchen exclusions.")
		return
	}

	plan, err := gitkitchen.PlanRepo(*analysis, tkCfg.PlatformMap, exclusions...)
	if err != nil {
		r.logf("ERROR", "git kitchen run: plan %q: %v", body.GitRepoName, err)
		WriteInternalError(w, "Failed to plan kitchen instances.")
		return
	}

	// Find the requested instance in the plan
	var found *gitkitchen.PlannedInstance
	for i := range plan.Instances {
		if plan.Instances[i].InstanceName == body.InstanceName {
			found = &plan.Instances[i]
			break
		}
	}
	if found == nil {
		WriteNotFound(w, "Instance not found in plan.")
		return
	}
	if found.Status != gitkitchen.InstanceStatusMapped {
		WriteBadRequest(w, "Instance is not runnable (status: "+string(found.Status)+").")
		return
	}

	// Enqueue via kitchen queue if available
	if r.kitchenQueue != nil {
		item, enqErr := r.db.EnqueueKitchenRun(ctx, datastore.EnqueueKitchenRunParams{
			RunType:           "git",
			GitRepoName:       plan.GitRepoName,
			GitRepoURL:        plan.GitRepoURL,
			SuiteName:         found.SuiteName,
			PlatformName:      found.PlatformName,
			InstanceName:      found.InstanceName,
			TargetChefVersion: body.TargetChefVersion,
			HeadCommitSHA:     plan.CommitSHA,
			Priority:          20,
		})
		if enqErr != nil {
			if enqErr == datastore.ErrAlreadyExists {
				WriteJSON(w, http.StatusConflict, map[string]string{
					"message": "Instance is already queued or running",
				})
				return
			}
			r.logf("ERROR", "git kitchen run: enqueue: %v", enqErr)
			WriteInternalError(w, "Failed to enqueue kitchen run.")
			return
		}

		r.hub.Broadcast(NewEvent("kitchen_queue_update", item))

		WriteJSON(w, http.StatusAccepted, map[string]any{
			"message":  "Run queued for instance " + body.InstanceName,
			"queue_id": item.ID,
			"status":   item.Status,
		})
		return
	}

	// Legacy fallback: direct dispatch via goroutine
	cfg := gitkitchen.SchedulerConfig{
		MaxConcurrency:    1,
		TargetChefVersion: body.TargetChefVersion,
	}

	bgCtx := context.WithoutCancel(ctx)

	go func() {
		result, runErr := r.gitKitchenScheduler.RunOne(bgCtx, plan, body.InstanceName, cfg, tkCfg)
		if runErr != nil {
			r.logf("ERROR", "git kitchen run async: %v", runErr)
		}
		evt := map[string]any{
			"git_repo_name": body.GitRepoName,
			"instance_name": body.InstanceName,
		}
		if result != nil {
			evt["passed"] = result.Result.Passed
		}
		r.hub.Broadcast(NewEvent(EventGitKitchenRunComplete, evt))
	}()

	WriteJSON(w, http.StatusAccepted, map[string]string{
		"message": "Run dispatched for instance " + body.InstanceName,
	})
}

// ---------------------------------------------------------------------------
// POST /api/v1/kitchen/git/run-all
// ---------------------------------------------------------------------------

// gitKitchenRunAllRequest is the request body for POST /api/v1/kitchen/git/run-all.
type gitKitchenRunAllRequest struct {
	GitRepoName       string `json:"git_repo_name"`
	TargetChefVersion string `json:"target_chef_version"`
}

// handleGitKitchenRunAll handles POST /api/v1/kitchen/git/run-all.
// It plans the repo and enqueues all mapped (non-excluded) instances
// via the kitchen queue (or falls back to direct dispatch).
func (r *Router) handleGitKitchenRunAll(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}

	if r.gitKitchenScheduler == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Git kitchen scheduler is not configured.")
		return
	}

	var body gitKitchenRunAllRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid JSON request body.")
		return
	}

	if body.GitRepoName == "" {
		WriteBadRequest(w, "git_repo_name is required")
		return
	}
	if body.TargetChefVersion == "" {
		WriteBadRequest(w, "target_chef_version is required")
		return
	}

	ctx := req.Context()

	var tkCfg config.TestKitchenConfig
	setting, settingErr := r.db.GetRuntimeSetting(ctx, "test_kitchen")
	if settingErr != nil {
		r.logf("ERROR", "git kitchen run-all: load runtime setting: %v", settingErr)
		WriteInternalError(w, "Failed to load Test Kitchen configuration.")
		return
	}
	if setting != nil {
		if unmarshalErr := json.Unmarshal(setting.Value, &tkCfg); unmarshalErr != nil {
			r.logf("ERROR", "git kitchen run-all: parse stored config: %v", unmarshalErr)
			WriteInternalError(w, "Failed to parse stored Test Kitchen configuration.")
			return
		}
	} else {
		tkCfg = r.liveConfig().AnalysisTools.TestKitchen
	}

	if !tkCfg.IsEnabled() {
		WriteError(w, http.StatusConflict, "conflict", "Test Kitchen is disabled.")
		return
	}

	analysis, err := r.db.GetKitchenAnalysisResultByName(ctx, body.GitRepoName)
	if err != nil {
		r.logf("ERROR", "git kitchen run-all: lookup %q: %v", body.GitRepoName, err)
		WriteInternalError(w, "Failed to retrieve kitchen analysis.")
		return
	}
	if analysis == nil {
		WriteNotFound(w, "No kitchen analysis found for repo.")
		return
	}

	exclusions, exclErr := r.loadInstanceExclusions(ctx, body.GitRepoName)
	if exclErr != nil {
		r.logf("ERROR", "git kitchen run-all: load exclusions for %q: %v", body.GitRepoName, exclErr)
		WriteInternalError(w, "Failed to load kitchen exclusions.")
		return
	}

	plan, err := gitkitchen.PlanRepo(*analysis, tkCfg.PlatformMap, exclusions...)
	if err != nil {
		r.logf("ERROR", "git kitchen run-all: plan %q: %v", body.GitRepoName, err)
		WriteInternalError(w, "Failed to plan kitchen instances.")
		return
	}

	// Collect mapped instances
	var mapped []gitkitchen.PlannedInstance
	for _, inst := range plan.Instances {
		if inst.Status == gitkitchen.InstanceStatusMapped {
			mapped = append(mapped, inst)
		}
	}
	if len(mapped) == 0 {
		WriteBadRequest(w, "No mapped instances to run (all may be excluded or unmapped).")
		return
	}

	// Enqueue via kitchen queue if available
	if r.kitchenQueue != nil {
		var queued []string
		var skipped int
		for _, inst := range mapped {
			item, enqErr := r.db.EnqueueKitchenRun(ctx, datastore.EnqueueKitchenRunParams{
				RunType:           "git",
				GitRepoName:       plan.GitRepoName,
				GitRepoURL:        plan.GitRepoURL,
				SuiteName:         inst.SuiteName,
				PlatformName:      inst.PlatformName,
				InstanceName:      inst.InstanceName,
				TargetChefVersion: body.TargetChefVersion,
				HeadCommitSHA:     plan.CommitSHA,
				Priority:          10,
			})
			if enqErr != nil {
				if enqErr == datastore.ErrAlreadyExists {
					skipped++
					continue
				}
				r.logf("ERROR", "git kitchen run-all: enqueue %s: %v", inst.InstanceName, enqErr)
				continue
			}
			queued = append(queued, item.ID)
			r.hub.Broadcast(NewEvent("kitchen_queue_update", item))
		}

		WriteJSON(w, http.StatusAccepted, map[string]any{
			"message":        "Instances queued for execution",
			"queued_count":   len(queued),
			"skipped_count":  skipped,
			"queue_ids":      queued,
		})
		return
	}

	// Legacy fallback: direct dispatch via goroutine
	cfg := gitkitchen.SchedulerConfig{
		MaxConcurrency:    2,
		TargetChefVersion: body.TargetChefVersion,
	}

	bgCtx := context.WithoutCancel(ctx)

	go func() {
		_, runErr := r.gitKitchenScheduler.RunAll(bgCtx, plan, cfg, tkCfg, func(completed, total int, instance gitkitchen.PlannedInstance, result gitkitchen.RunInstanceResult) {
			r.hub.Broadcast(NewEvent(EventGitKitchenRunComplete, map[string]any{
				"git_repo_name": body.GitRepoName,
				"instance_name": instance.InstanceName,
				"passed":        result.Passed,
			}))
		})
		if runErr != nil {
			r.logf("ERROR", "git kitchen run-all async: %v", runErr)
		}
	}()

	WriteJSON(w, http.StatusAccepted, map[string]any{
		"message":        "Run dispatched for all mapped instances",
		"instance_count": len(mapped),
	})
}

// loadInstanceExclusions fetches exclusions for a repo and converts them to
// the planner's InstanceExclusion type.
func (r *Router) loadInstanceExclusions(ctx context.Context, repoName string) ([]gitkitchen.InstanceExclusion, error) {
	dbExclusions, err := r.db.ListKitchenExclusions(ctx, repoName)
	if err != nil {
		return nil, err
	}
	if len(dbExclusions) == 0 {
		return nil, nil
	}
	result := make([]gitkitchen.InstanceExclusion, len(dbExclusions))
	for i, e := range dbExclusions {
		result[i] = gitkitchen.InstanceExclusion{
			SuiteName:    e.SuiteName,
			PlatformName: e.PlatformName,
			Reason:       e.Reason,
		}
	}
	return result, nil
}
