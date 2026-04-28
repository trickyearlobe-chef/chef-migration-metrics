// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/batch"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/gitkitchen"
)

// ---------------------------------------------------------------------------
// Kitchen Batches — list and create
// ---------------------------------------------------------------------------

// handleKitchenBatches dispatches GET (list) and POST (create) for
// /api/v1/kitchen/batches.
func (r *Router) handleKitchenBatches(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handleListKitchenBatches(w, req)
	case http.MethodPost:
		r.handleCreateKitchenBatch(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and POST.")
	}
}

// handleListKitchenBatches returns all kitchen batches ordered by
// created_at DESC.
func (r *Router) handleListKitchenBatches(w http.ResponseWriter, req *http.Request) {
	batches, err := r.db.ListKitchenBatches(req.Context())
	if err != nil {
		r.logf("ERROR", "kitchen-batches: listing batches: %v", err)
		WriteInternalError(w, "Failed to list kitchen batches.")
		return
	}
	if batches == nil {
		batches = []datastore.KitchenBatch{}
	}
	WriteJSON(w, http.StatusOK, batches)
}

type createBatchRequest struct {
	Name             string                 `json:"name"`
	Filters          datastore.BatchFilters `json:"filters"`
	MaxCount         *int                   `json:"max_count"`
	MaxConcurrentVMs *int                   `json:"max_concurrent_vms"`
	DryRun           bool                   `json:"dry_run"`
}

// handleCreateKitchenBatch creates a new kitchen batch definition.
func (r *Router) handleCreateKitchenBatch(w http.ResponseWriter, req *http.Request) {
	var body createBatchRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, fmt.Sprintf("Invalid JSON body: %v", err))
		return
	}
	if body.Name == "" {
		WriteBadRequest(w, "name is required.")
		return
	}

	b, err := r.db.CreateKitchenBatch(req.Context(), datastore.CreateKitchenBatchParams{
		Name:             body.Name,
		Filters:          body.Filters,
		MaxCount:         body.MaxCount,
		MaxConcurrentVMs: body.MaxConcurrentVMs,
		DryRun:           body.DryRun,
	})
	if err != nil {
		r.logf("ERROR", "kitchen-batches: creating batch: %v", err)
		WriteInternalError(w, "Failed to create kitchen batch.")
		return
	}
	WriteJSON(w, http.StatusCreated, b)
}

// ---------------------------------------------------------------------------
// Kitchen Batch detail — GET, PUT, DELETE, run, cancel
// ---------------------------------------------------------------------------

// handleKitchenBatchDetail dispatches based on path segments under
// /api/v1/kitchen/batches/:id[/action].
func (r *Router) handleKitchenBatchDetail(w http.ResponseWriter, req *http.Request) {
	segments := pathSegments(req.URL.Path, "/api/v1/kitchen/batches/")
	if len(segments) == 0 {
		WriteNotFound(w, "Batch ID is required in the path.")
		return
	}

	id := segments[0]

	// /api/v1/kitchen/batches/:id/results — removed (git kitchen rebuild)
	if len(segments) == 2 && segments[1] == "results" {
		WriteNotFound(w, "Batch results endpoint has been removed.")
		return
	}
	// /api/v1/kitchen/batches/:id/progress — removed (git kitchen rebuild)
	if len(segments) == 2 && segments[1] == "progress" {
		WriteNotFound(w, "Batch progress endpoint has been removed.")
		return
	}

	// /api/v1/kitchen/batches/:id/run
	if len(segments) == 2 && segments[1] == "run" {
		r.handleRunKitchenBatch(w, req, id)
		return
	}
	// /api/v1/kitchen/batches/:id/cancel
	if len(segments) == 2 && segments[1] == "cancel" {
		r.handleCancelKitchenBatch(w, req, id)
		return
	}

	// Single batch: GET, PUT, DELETE
	switch req.Method {
	case http.MethodGet:
		r.handleGetKitchenBatch(w, req, id)
	case http.MethodPut:
		r.handleUpdateKitchenBatch(w, req, id)
	case http.MethodDelete:
		r.handleDeleteKitchenBatch(w, req, id)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET, PUT, and DELETE.")
	}
}

// ---------------------------------------------------------------------------
// Batch resolver adapter
// ---------------------------------------------------------------------------

// dbRepoLister adapts the DataStore to the batch.GitRepoLister interface.
type dbRepoLister struct {
	db DataStore
}

func (a *dbRepoLister) ListGitRepos(ctx context.Context) ([]batch.GitRepo, error) {
	repos, err := a.db.ListGitRepos(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]batch.GitRepo, len(repos))
	for i, repo := range repos {
		result[i] = batch.GitRepo{
			Name:            repo.Name,
			GitRepoURL:      repo.GitRepoURL,
			HasTestSuite:    repo.HasTestSuite,
			KitchenExcluded: repo.KitchenExcluded,
		}
	}
	return result, nil
}

// toBatchFilters converts datastore.BatchFilters to batch.Filters.
func toBatchFilters(f datastore.BatchFilters) batch.Filters {
	return batch.Filters{
		CookbookNames:      f.CookbookNames,
		Platforms:          f.Platforms,
		ExcludeCookbooks:   f.ExcludeCookbooks,
		HasTestSuite:       f.HasTestSuite,
		PreviousStatus:     f.PreviousStatus,
		TargetChefVersions: f.TargetChefVersions,
		IncludeExcluded:    f.IncludeExcluded,
	}
}

// resolveBatch creates a batch.Resolver and resolves the given batch filters.
func (r *Router) resolveBatch(ctx context.Context, filters datastore.BatchFilters, maxCount *int) (*batch.BatchEstimate, error) {
	resolver := batch.NewResolver(&dbRepoLister{db: r.db})
	estimate, err := resolver.ResolveBatch(ctx, toBatchFilters(filters), maxCount)
	if err != nil {
		return nil, err
	}
	return &estimate, nil
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/batches/:id
// ---------------------------------------------------------------------------

type batchDetailResponse struct {
	datastore.KitchenBatch
	Estimate *batch.BatchEstimate `json:"estimate,omitempty"`
}

func (r *Router) handleGetKitchenBatch(w http.ResponseWriter, req *http.Request, id string) {
	ctx := req.Context()

	b, err := r.db.GetKitchenBatch(ctx, id)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Kitchen batch %q not found.", id))
			return
		}
		r.logf("ERROR", "kitchen-batches: getting batch %s: %v", id, err)
		WriteInternalError(w, "Failed to get kitchen batch.")
		return
	}

	resp := batchDetailResponse{KitchenBatch: b}

	estimate, err := r.resolveBatch(ctx, b.Filters, b.MaxCount)
	if err != nil {
		r.logf("ERROR", "kitchen-batches: resolving batch %s: %v", id, err)
		// Return the batch without estimate rather than failing entirely.
	} else {
		resp.Estimate = estimate
	}

	WriteJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// PUT /api/v1/kitchen/batches/:id
// ---------------------------------------------------------------------------

func (r *Router) handleUpdateKitchenBatch(w http.ResponseWriter, req *http.Request, id string) {
	var body createBatchRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, fmt.Sprintf("Invalid JSON body: %v", err))
		return
	}
	if body.Name == "" {
		WriteBadRequest(w, "name is required.")
		return
	}

	b, err := r.db.UpdateKitchenBatch(req.Context(), id, datastore.UpdateKitchenBatchParams{
		Name:             body.Name,
		Filters:          body.Filters,
		MaxCount:         body.MaxCount,
		MaxConcurrentVMs: body.MaxConcurrentVMs,
		DryRun:           body.DryRun,
	})
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Kitchen batch %q not found or not in draft status.", id))
			return
		}
		r.logf("ERROR", "kitchen-batches: updating batch %s: %v", id, err)
		WriteInternalError(w, "Failed to update kitchen batch.")
		return
	}
	WriteJSON(w, http.StatusOK, b)
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/kitchen/batches/:id
// ---------------------------------------------------------------------------

func (r *Router) handleDeleteKitchenBatch(w http.ResponseWriter, req *http.Request, id string) {
	err := r.db.DeleteKitchenBatch(req.Context(), id)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Kitchen batch %q not found or not in a deletable status.", id))
			return
		}
		r.logf("ERROR", "kitchen-batches: deleting batch %s: %v", id, err)
		WriteInternalError(w, "Failed to delete kitchen batch.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// POST /api/v1/kitchen/batches/:id/run
// ---------------------------------------------------------------------------

func (r *Router) handleRunKitchenBatch(w http.ResponseWriter, req *http.Request, id string) {
	if !requirePOST(w, req) {
		return
	}

	ctx := req.Context()

	b, err := r.db.GetKitchenBatch(ctx, id)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Kitchen batch %q not found.", id))
			return
		}
		r.logf("ERROR", "kitchen-batches: getting batch %s for run: %v", id, err)
		WriteInternalError(w, "Failed to get kitchen batch.")
		return
	}

	if b.Status != "draft" {
		WriteError(w, http.StatusConflict, "conflict",
			fmt.Sprintf("Batch is not in draft status (current: %s).", b.Status))
		return
	}

	// Resolve effective TK config and check enabled gate.
	var tkCfg config.TestKitchenConfig
	setting, settingErr := r.db.GetRuntimeSetting(ctx, "test_kitchen")
	if settingErr != nil {
		r.logf("ERROR", "kitchen-batches: load runtime setting: %v", settingErr)
		WriteInternalError(w, "Failed to load Test Kitchen configuration.")
		return
	}
	if setting != nil {
		if unmarshalErr := json.Unmarshal(setting.Value, &tkCfg); unmarshalErr != nil {
			r.logf("ERROR", "kitchen-batches: parse stored config: %v", unmarshalErr)
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

	// Dry-run batches don't need the scheduler or concurrency guard.
	if b.DryRun {
		r.handleRunDryRunBatch(w, ctx, b, tkCfg)
		return
	}

	if r.gitKitchenScheduler == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Git kitchen scheduler is not configured.")
		return
	}

	// Single-running-batch guard: only one non-dry-run batch active at a time.
	// Check in-memory map (covers preparing + running + draining).
	r.batchMu.Lock()
	if len(r.runningBatch) > 0 {
		r.batchMu.Unlock()
		WriteError(w, http.StatusConflict, "conflict",
			"Another batch is already running. Only one batch may execute at a time.")
		return
	}
	r.batchMu.Unlock()

	// Non-dry-run: transition to preparing, then launch background execution.
	now := time.Now().UTC()
	b, err = r.db.UpdateKitchenBatchStatus(ctx, id, "preparing", now)
	if err != nil {
		r.logf("ERROR", "kitchen-batches: transitioning batch %s to preparing: %v", id, err)
		WriteInternalError(w, "Failed to start kitchen batch.")
		return
	}

	// Resolve for the estimate (returned to the caller).
	estimate, resolveErr := r.resolveBatch(ctx, b.Filters, b.MaxCount)
	if resolveErr != nil {
		r.logf("ERROR", "kitchen-batches: resolving batch %s: %v", id, resolveErr)
	}

	// Register cancel func before launching goroutine.
	batchCtx, cancelFn := context.WithCancel(context.WithoutCancel(ctx))
	r.batchMu.Lock()
	r.runningBatch[id] = cancelFn
	r.batchMu.Unlock()

	go r.executeBatch(batchCtx, b, tkCfg, estimate)

	resp := batchDetailResponse{
		KitchenBatch: b,
		Estimate:     estimate,
	}
	WriteJSON(w, http.StatusAccepted, resp)
}

// handleRunDryRunBatch handles the dry-run path: resolve, preview, complete.
func (r *Router) handleRunDryRunBatch(w http.ResponseWriter, ctx context.Context, b datastore.KitchenBatch, tkCfg config.TestKitchenConfig) {
	_ = tkCfg // reserved for future dry-run planning

	now := time.Now().UTC()
	var err error
	b, err = r.db.UpdateKitchenBatchStatus(ctx, b.ID, "previewing", now)
	if err != nil {
		r.logf("ERROR", "kitchen-batches: transitioning batch %s to previewing: %v", b.ID, err)
		WriteInternalError(w, "Failed to start kitchen batch preview.")
		return
	}

	estimate, resolveErr := r.resolveBatch(ctx, b.Filters, b.MaxCount)
	if resolveErr != nil {
		r.logf("ERROR", "kitchen-batches: resolving batch %s: %v", b.ID, resolveErr)
	}

	b, err = r.db.UpdateKitchenBatchStatus(ctx, b.ID, "completed", time.Now().UTC())
	if err != nil {
		r.logf("ERROR", "kitchen-batches: completing dry-run batch %s: %v", b.ID, err)
		WriteInternalError(w, "Failed to complete dry-run batch.")
		return
	}

	resp := batchDetailResponse{
		KitchenBatch: b,
		Estimate:     estimate,
	}
	WriteJSON(w, http.StatusOK, resp)
}

// executeBatch runs in a background goroutine. It resolves repos, plans
// instances, persists batch instance records, then executes via the scheduler.
func (r *Router) executeBatch(ctx context.Context, b datastore.KitchenBatch, tkCfg config.TestKitchenConfig, estimate *batch.BatchEstimate) {
	batchID := b.ID

	// Always clean up: remove from running map on exit.
	defer func() {
		r.batchMu.Lock()
		delete(r.runningBatch, batchID)
		r.batchMu.Unlock()
	}()

	// Phase 1: resolve + plan + persist instances.
	plans, instanceMap, err := r.prepareBatchInstances(ctx, b, tkCfg, estimate)
	if err != nil {
		r.logf("ERROR", "kitchen-batches: preparing batch %s: %v", batchID, err)
		r.failBatch(ctx, batchID, "preparing")
		return
	}

	if len(plans) == 0 {
		r.logf("INFO", "kitchen-batches: batch %s resolved to zero instances", batchID)
		r.failBatch(ctx, batchID, "preparing")
		return
	}

	// Phase 2: transition preparing → running.
	_, casErr := r.db.UpdateKitchenBatchStatusIfCurrent(ctx, batchID, "preparing", "running", time.Now().UTC())
	if casErr != nil {
		r.logf("ERROR", "kitchen-batches: CAS preparing→running for %s: %v", batchID, casErr)
		return // someone cancelled while we were preparing
	}

	// Phase 3: execute.
	concurrency := 2 // conservative default
	if b.MaxConcurrentVMs != nil && *b.MaxConcurrentVMs > 0 {
		concurrency = *b.MaxConcurrentVMs
	}

	cfg := gitkitchen.SchedulerConfig{
		MaxConcurrency:    concurrency,
		TargetChefVersion: r.batchTargetVersion(b),
	}

	progressCB := func(completed, total int, repoName string, inst gitkitchen.PlannedInstance, result gitkitchen.RunInstanceResult) {
		// Update instance status in DB.
		instKey := repoName + "/" + inst.InstanceName
		if instID, ok := instanceMap[instKey]; ok {
			status := "passed"
			errMsg := ""
			if result.Passed == nil || !*result.Passed {
				status = "failed"
				if result.ErrorMessage != "" {
					status = "errored"
					errMsg = result.ErrorMessage
				}
			}
			_ = r.db.UpdateBatchInstanceStatus(ctx, instID, status, errMsg, time.Now().UTC())
		}

		// Broadcast progress.
		passed := result.Passed != nil && *result.Passed
		r.hub.Broadcast(NewEvent(EventBatchProgress, map[string]any{
			"batch_id":      batchID,
			"instance_name": inst.InstanceName,
			"git_repo_name": repoName,
			"passed":        passed,
			"completed":     completed,
			"total":         total,
		}))
	}

	result, runErr := r.gitKitchenScheduler.RunBatch(ctx, plans, cfg, tkCfg, progressCB)
	if runErr != nil {
		r.logf("ERROR", "kitchen-batches: running batch %s: %v", batchID, runErr)
	}

	// Phase 4: finalise.
	// Cancel any remaining pending instances.
	_, _ = r.db.CancelPendingBatchInstances(ctx, batchID)

	// Determine final status via CAS.
	finalStatus := "completed"
	if ctx.Err() != nil {
		finalStatus = "cancelled"
	}
	_, casErr = r.db.UpdateKitchenBatchStatusIfCurrent(ctx, batchID, "running", finalStatus, time.Now().UTC())
	if casErr != nil {
		r.logf("WARN", "kitchen-batches: CAS running→%s for %s failed (already transitioned): %v", finalStatus, batchID, casErr)
	}

	// Broadcast completion.
	evtData := map[string]any{
		"batch_id": batchID,
		"status":   finalStatus,
		"total":    0,
		"passed":   0,
		"failed":   0,
		"errored":  0,
	}
	if result != nil {
		evtData["total"] = result.Total
		evtData["passed"] = result.Passed
		evtData["failed"] = result.Failed
		evtData["errored"] = result.Errors
	}
	r.hub.Broadcast(NewEvent(EventBatchComplete, evtData))
}

// prepareBatchInstances resolves repos, plans instances, and persists
// batch instance records. Returns the plans and a map of
// "repoName/instanceName" → batch instance ID for progress tracking.
func (r *Router) prepareBatchInstances(
	ctx context.Context,
	b datastore.KitchenBatch,
	tkCfg config.TestKitchenConfig,
	estimate *batch.BatchEstimate,
) ([]*gitkitchen.PlanResult, map[string]string, error) {
	if estimate == nil || len(estimate.Cookbooks) == 0 {
		return nil, nil, fmt.Errorf("no cookbooks resolved")
	}

	var plans []*gitkitchen.PlanResult
	var allParams []datastore.CreateBatchInstanceParams

	targetVersion := r.batchTargetVersion(b)

	for _, cb := range estimate.Cookbooks {
		analysis, err := r.db.GetKitchenAnalysisResultByName(ctx, cb.Name)
		if err != nil || analysis == nil {
			r.logf("WARN", "kitchen-batches: skipping repo %q (no analysis): %v", cb.Name, err)
			continue
		}

		plan, err := gitkitchen.PlanRepo(*analysis, tkCfg.PlatformMap)
		if err != nil {
			r.logf("WARN", "kitchen-batches: skipping repo %q (plan error): %v", cb.Name, err)
			continue
		}

		hasMapped := false
		for _, inst := range plan.Instances {
			if inst.Status != gitkitchen.InstanceStatusMapped {
				continue
			}
			hasMapped = true
			allParams = append(allParams, datastore.CreateBatchInstanceParams{
				BatchID:           b.ID,
				GitRepoName:       plan.GitRepoName,
				GitRepoURL:        plan.GitRepoURL,
				InstanceName:      inst.InstanceName,
				PlatformName:      inst.PlatformName,
				SuiteName:         inst.SuiteName,
				TargetChefVersion: targetVersion,
			})
		}
		if hasMapped {
			plans = append(plans, plan)
		}
	}

	if len(allParams) == 0 {
		return nil, nil, fmt.Errorf("no mapped instances across resolved repos")
	}

	instances, err := r.db.CreateBatchInstances(ctx, allParams)
	if err != nil {
		return nil, nil, fmt.Errorf("persisting batch instances: %w", err)
	}

	// Build lookup map: "repoName/instanceName" → instance ID.
	instanceMap := make(map[string]string, len(instances))
	for _, inst := range instances {
		key := inst.GitRepoName + "/" + inst.InstanceName
		instanceMap[key] = inst.ID
	}

	return plans, instanceMap, nil
}

// failBatch transitions a batch to "failed" from the expected current status.
func (r *Router) failBatch(ctx context.Context, batchID, fromStatus string) {
	_, err := r.db.UpdateKitchenBatchStatusIfCurrent(ctx, batchID, fromStatus, "failed", time.Now().UTC())
	if err != nil {
		r.logf("ERROR", "kitchen-batches: CAS %s→failed for %s: %v", fromStatus, batchID, err)
	}
	_, _ = r.db.CancelPendingBatchInstances(ctx, batchID)
	r.hub.Broadcast(NewEvent(EventBatchComplete, map[string]any{
		"batch_id": batchID,
		"status":   "failed",
	}))
}

// batchTargetVersion extracts the target chef version from batch filters.
// Uses the first entry if multiple are specified.
func (r *Router) batchTargetVersion(b datastore.KitchenBatch) string {
	if len(b.Filters.TargetChefVersions) > 0 {
		return b.Filters.TargetChefVersions[0]
	}
	return ""
}

// ---------------------------------------------------------------------------
// POST /api/v1/kitchen/batches/:id/cancel
// ---------------------------------------------------------------------------

func (r *Router) handleCancelKitchenBatch(w http.ResponseWriter, req *http.Request, id string) {
	if !requirePOST(w, req) {
		return
	}

	ctx := req.Context()

	b, err := r.db.GetKitchenBatch(ctx, id)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Kitchen batch %q not found.", id))
			return
		}
		r.logf("ERROR", "kitchen-batches: getting batch %s for cancel: %v", id, err)
		WriteInternalError(w, "Failed to get kitchen batch.")
		return
	}

	if b.Status != "running" && b.Status != "previewing" && b.Status != "preparing" {
		WriteError(w, http.StatusConflict, "conflict",
			fmt.Sprintf("Batch cannot be cancelled (current status: %s).", b.Status))
		return
	}

	b, err = r.db.UpdateKitchenBatchStatus(ctx, id, "cancelled", time.Now().UTC())
	if err != nil {
		r.logf("ERROR", "kitchen-batches: cancelling batch %s: %v", id, err)
		WriteInternalError(w, "Failed to cancel kitchen batch.")
		return
	}

	// Signal the background goroutine to stop scheduling new work.
	// The goroutine itself removes from the map after in-flight work drains.
	r.batchMu.Lock()
	if cancelFn, ok := r.runningBatch[id]; ok {
		cancelFn()
	}
	r.batchMu.Unlock()

	// Cancel all pending instances that haven't started.
	_, _ = r.db.CancelPendingBatchInstances(ctx, id)

	WriteJSON(w, http.StatusOK, b)
}

// ---------------------------------------------------------------------------
// GET /api/v1/git-repos/excluded
// ---------------------------------------------------------------------------

// handleListExcludedGitRepos returns all git repos excluded from kitchen
// testing.
func (r *Router) handleListExcludedGitRepos(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	repos, err := r.db.ListExcludedGitRepos(req.Context())
	if err != nil {
		r.logf("ERROR", "git-repos: listing excluded repos: %v", err)
		WriteInternalError(w, "Failed to list excluded git repos.")
		return
	}
	if repos == nil {
		repos = []datastore.GitRepo{}
	}
	WriteJSON(w, http.StatusOK, repos)
}

// ---------------------------------------------------------------------------
// POST /api/v1/git-repos/:name/exclude
// ---------------------------------------------------------------------------

// handleGitRepoExclude marks a git repo as excluded from kitchen testing.
func (r *Router) handleGitRepoExclude(w http.ResponseWriter, req *http.Request, name string) {
	if !requirePOST(w, req) {
		return
	}

	var body struct {
		Reason     string `json:"reason"`
		ExcludedBy string `json:"excluded_by"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, fmt.Sprintf("Invalid JSON body: %v", err))
		return
	}

	err := r.db.SetGitRepoKitchenExclusion(req.Context(), name, body.Reason, body.ExcludedBy)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Git repo %q not found.", name))
			return
		}
		r.logf("ERROR", "git-repos: excluding repo %s: %v", name, err)
		WriteInternalError(w, "Failed to exclude git repo.")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Git repo %q excluded from kitchen testing.", name),
	})
}

// ---------------------------------------------------------------------------
// DELETE /api/v1/git-repos/:name/exclude
// ---------------------------------------------------------------------------

// handleGitRepoClearExclusion removes the kitchen exclusion flag from a
// git repo.
func (r *Router) handleGitRepoClearExclusion(w http.ResponseWriter, req *http.Request, name string) {
	if !requireMethod(w, req, http.MethodDelete) {
		return
	}

	err := r.db.ClearGitRepoKitchenExclusion(req.Context(), name)
	if err != nil {
		if errors.Is(err, datastore.ErrNotFound) {
			WriteNotFound(w, fmt.Sprintf("Git repo %q not found.", name))
			return
		}
		r.logf("ERROR", "git-repos: clearing exclusion for repo %s: %v", name, err)
		WriteInternalError(w, "Failed to clear git repo exclusion.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
