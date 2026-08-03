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
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/tkstatus"
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
	Name     string                 `json:"name"`
	Filters  datastore.BatchFilters `json:"filters"`
	MaxCount *int                   `json:"max_count"`
	DryRun   bool                   `json:"dry_run"`
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
		Name:     body.Name,
		Filters:  body.Filters,
		MaxCount: body.MaxCount,
		DryRun:   body.DryRun,
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
	// /api/v1/kitchen/batches/:id/progress
	if len(segments) == 2 && segments[1] == "progress" {
		r.handleBatchProgress(w, req, id)
		return
	}
	// /api/v1/kitchen/batches/:id/instances
	if len(segments) == 2 && segments[1] == "instances" {
		r.handleListBatchInstances(w, req, id)
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

// dbAnalysisProvider adapts the DataStore to both batch.KitchenAnalysisProvider
// (for resolver platform filtering) and batch.AnalysisLoader (for planner).
type dbAnalysisProvider struct {
	db DataStore
}

func (a *dbAnalysisProvider) GetKitchenAnalysisPlatforms(ctx context.Context, repoName string) ([]string, error) {
	ar, err := a.db.GetKitchenAnalysisResultByName(ctx, repoName)
	if err != nil || ar == nil {
		return nil, err
	}
	var platforms []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(ar.Platforms, &platforms); err != nil {
		return nil, err
	}
	names := make([]string, len(platforms))
	for i, p := range platforms {
		names[i] = p.Name
	}
	return names, nil
}

func (a *dbAnalysisProvider) GetKitchenAnalysisResultByName(ctx context.Context, repoName string) (*datastore.KitchenAnalysisResult, error) {
	return a.db.GetKitchenAnalysisResultByName(ctx, repoName)
}

// dbResultProvider adapts the DataStore to batch.TestKitchenResultProvider.
type dbResultProvider struct {
	db DataStore
}

func (p *dbResultProvider) GetLatestTestKitchenStatus(ctx context.Context, repoName string) (string, error) {
	results, err := p.db.ListGitKitchenResultsByRepo(ctx, repoName)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "untested", nil
	}
	var passed, failed int
	for _, r := range results {
		switch {
		case r.Passed != nil && *r.Passed:
			passed++
		case (r.Passed != nil && !*r.Passed) || r.TimedOut:
			failed++
			// r.Passed == nil && !r.TimedOut: result incomplete — skip
		}
	}
	status := "untested"
	if passed > 0 || failed > 0 {
		status = tkstatus.ComputeTKStatus(passed, failed)
	}
	return status, nil
}

// dbExclusionsLoader adapts the Router's loadInstanceExclusions helper to
// the batch.ExclusionsLoader interface.
type dbExclusionsLoader struct {
	router *Router
}

func (l *dbExclusionsLoader) LoadInstanceExclusions(ctx context.Context, repoName string) ([]gitkitchen.InstanceExclusion, error) {
	return l.router.loadInstanceExclusions(ctx, repoName)
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

// resolveBatch creates a batch.Resolver with all providers wired, resolves the
// given batch filters, then calls PlanBatch for accurate VM estimates.
func (r *Router) resolveBatch(ctx context.Context, filters datastore.BatchFilters, maxCount *int) (*batch.BatchEstimate, error) {
	analysisProvider := &dbAnalysisProvider{db: r.db}
	resultProvider := &dbResultProvider{db: r.db}

	resolver := batch.NewResolver(
		&dbRepoLister{db: r.db},
		batch.WithAnalysisProvider(analysisProvider),
		batch.WithResultProvider(resultProvider),
	)
	estimate, err := resolver.ResolveBatch(ctx, toBatchFilters(filters), maxCount)
	if err != nil {
		return nil, err
	}

	// Enrich with accurate VM estimates via PlanBatch.
	platformMap := r.liveConfig().AnalysisTools.TestKitchen.PlatformMap
	exclusionsLoader := &dbExclusionsLoader{router: r}
	planned := batch.PlanBatch(ctx, estimate.Cookbooks, platformMap, analysisProvider, exclusionsLoader)
	return &planned, nil
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
		Name:     body.Name,
		Filters:  body.Filters,
		MaxCount: body.MaxCount,
		DryRun:   body.DryRun,
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
	tkCfg := r.liveConfig().AnalysisTools.TestKitchen

	if !tkCfg.IsEnabled() {
		WriteError(w, http.StatusConflict, "conflict", "Test Kitchen is disabled.")
		return
	}

	// Dry-run batches don't need the scheduler or concurrency guard.
	if b.DryRun {
		r.handleRunDryRunBatch(w, ctx, b, tkCfg)
		return
	}

	if r.kitchenQueue == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Kitchen queue is not configured.")
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

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/batches/:id/progress
// ---------------------------------------------------------------------------

func (r *Router) handleBatchProgress(w http.ResponseWriter, req *http.Request, id string) {
	if !requireGET(w, req) {
		return
	}

	counts, err := r.db.CountBatchInstancesByStatus(req.Context(), id)
	if err != nil {
		r.logf("ERROR", "kitchen-batches: counting instances for %s: %v", id, err)
		WriteInternalError(w, "Failed to retrieve batch progress.")
		return
	}

	total := 0
	for _, n := range counts {
		total += n
	}

	WriteJSON(w, http.StatusOK, map[string]int{
		"total":           total,
		"pending":         counts["pending"],
		"running":         counts["running"],
		"passed":          counts["passed"],
		"failed":          counts["failed"],
		"errored":         counts["errored"],
		"timed_out":       counts["timed_out"],
		"network_timeout": counts["network_timeout"],
		"cancelled":       counts["cancelled"],
	})
}

// ---------------------------------------------------------------------------
// GET /api/v1/kitchen/batches/:id/instances
// ---------------------------------------------------------------------------

// handleListBatchInstances returns every per-instance record for a batch,
// ordered by git_repo_name, instance_name. This is the authoritative source
// for the detail view's per-instance results table.
func (r *Router) handleListBatchInstances(w http.ResponseWriter, req *http.Request, id string) {
	if !requireGET(w, req) {
		return
	}

	instances, err := r.db.ListBatchInstances(req.Context(), id)
	if err != nil {
		r.logf("ERROR", "kitchen-batches: listing instances for %s: %v", id, err)
		WriteInternalError(w, "Failed to retrieve batch instances.")
		return
	}
	if instances == nil {
		instances = []datastore.KitchenBatchInstance{}
	}
	WriteJSON(w, http.StatusOK, instances)
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
// instances, persists batch instance records, then either enqueues via the
// kitchen run queue (preferred) or falls back to the scheduler for execution.
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

	// Phase 3: execute via queue.
	r.executeBatchViaQueue(ctx, batchID, plans, instanceMap, b)
}

// executeBatchViaQueue enqueues each batch instance into the kitchen run queue
// and polls for completion. The queue ensures bounded concurrency with other
// ad-hoc and run-all requests.
func (r *Router) executeBatchViaQueue(ctx context.Context, batchID string, plans []*gitkitchen.PlanResult, instanceMap map[string]string, b datastore.KitchenBatch) {
	targetVersion := r.batchTargetVersion(b)

	if targetVersion == "" {
		r.logf("ERROR", "kitchen-batches: batch %s has no target chef version configured", batchID)
		r.failBatch(ctx, batchID, "running")
		return
	}

	// Enqueue all mapped instances.
	enqueued := 0
	skipped := 0
	for _, plan := range plans {
		for _, inst := range plan.Instances {
			if inst.Status != gitkitchen.InstanceStatusMapped {
				continue
			}

			_, err := r.db.EnqueueKitchenRun(ctx, datastore.EnqueueKitchenRunParams{
				RunType:           "git",
				GitRepoName:       plan.GitRepoName,
				GitRepoURL:        plan.GitRepoURL,
				SuiteName:         inst.SuiteName,
				PlatformName:      inst.PlatformName,
				InstanceName:      inst.InstanceName,
				TargetChefVersion: targetVersion,
				HeadCommitSHA:     plan.CommitSHA,
				BatchID:           batchID,
				Priority:          5,
			})
			if err != nil {
				instKey := plan.GitRepoName + "/" + inst.InstanceName
				if instID, ok := instanceMap[instKey]; ok {
					_ = r.db.UpdateBatchInstanceStatus(ctx, instID, "errored", err.Error(), time.Now().UTC())
				}
				r.logf("WARN", "kitchen-batches: batch %s enqueue failed for %s/%s: %v", batchID, plan.GitRepoName, inst.InstanceName, err)
				skipped++
				continue
			}
			enqueued++
		}
	}

	r.logf("INFO", "kitchen-batches: batch %s enqueued %d items (skipped %d errors)", batchID, enqueued, skipped)

	if enqueued == 0 {
		r.finalizeBatch(ctx, batchID)
		return
	}

	// Poll for completion: check all queue items for this batch until all are terminal.
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Batch was cancelled — cancel all queued items.
			n, _ := r.db.CancelKitchenRunsByBatch(ctx, batchID)
			r.logf("INFO", "kitchen-batches: batch %s cancelled, cancelled %d queued items", batchID, n)
			// Cancel running items via manager.
			r.cancelRunningBatchItems(ctx, batchID)
			r.finalizeBatch(ctx, batchID)
			return
		case <-ticker.C:
			done := r.syncBatchProgress(ctx, batchID, instanceMap)
			if done {
				r.finalizeBatch(ctx, batchID)
				return
			}
		}
	}
}

// syncBatchProgress checks queue items for this batch and updates batch
// instance records. Returns true when all items are terminal.
func (r *Router) syncBatchProgress(ctx context.Context, batchID string, instanceMap map[string]string) bool {
	items, err := r.db.ListKitchenQueue(ctx, datastore.KitchenQueueFilter{
		BatchID: batchID,
		Limit:   500,
	})
	if err != nil {
		r.logf("ERROR", "kitchen-batches: polling queue for batch %s: %v", batchID, err)
		return false
	}

	allTerminal := true
	for _, item := range items {
		switch item.Status {
		case "queued", "running":
			allTerminal = false
		case "completed", "failed", "cancelled", "interrupted":
			// Map to batch instance status.
			instKey := item.GitRepoName + "/" + item.InstanceName
			if instID, ok := instanceMap[instKey]; ok {
				status := r.queueStatusToBatchStatus(item)
				_ = r.db.UpdateBatchInstanceStatus(ctx, instID, status, item.ErrorMessage, time.Now().UTC())
				// Remove from map to avoid re-processing.
				delete(instanceMap, instKey)

				// Broadcast progress.
				passed := status == "passed"
				r.hub.Broadcast(NewEvent(EventBatchProgress, map[string]any{
					"batch_id":      batchID,
					"instance_name": item.InstanceName,
					"git_repo_name": item.GitRepoName,
					"passed":        passed,
				}))
			}
		}
	}

	return allTerminal
}

// queueStatusToBatchStatus maps a terminal queue item status to the
// corresponding batch instance status.
func (r *Router) queueStatusToBatchStatus(item datastore.KitchenQueueItem) string {
	switch item.Status {
	case "completed":
		return "passed"
	case "failed":
		if item.ErrorMessage != "" {
			return "errored"
		}
		return "failed"
	case "cancelled":
		return "cancelled"
	case "interrupted":
		return "cancelled"
	default:
		return "failed"
	}
}

// cancelRunningBatchItems finds currently-running queue items for a batch
// and signals the queue manager to cancel them.
func (r *Router) cancelRunningBatchItems(ctx context.Context, batchID string) {
	if r.kitchenQueue == nil {
		return
	}
	items, err := r.db.ListKitchenQueue(ctx, datastore.KitchenQueueFilter{
		BatchID:  batchID,
		Statuses: []string{"running"},
		Limit:    100,
	})
	if err != nil {
		return
	}
	for _, item := range items {
		r.kitchenQueue.CancelItem(item.ID)
	}
}

// finalizeBatch determines the final status and updates the batch record.
func (r *Router) finalizeBatch(ctx context.Context, batchID string) {
	// Cancel any remaining pending batch instances.
	_, _ = r.db.CancelPendingBatchInstances(ctx, batchID)

	finalStatus := "completed"
	if ctx.Err() != nil {
		finalStatus = "cancelled"
	}
	_, casErr := r.db.UpdateKitchenBatchStatusIfCurrent(ctx, batchID, "running", finalStatus, time.Now().UTC())
	if casErr != nil {
		r.logf("WARN", "kitchen-batches: CAS running→%s for %s failed: %v", finalStatus, batchID, casErr)
	}

	// Broadcast completion.
	counts, _ := r.db.CountBatchInstancesByStatus(ctx, batchID)
	total := 0
	for _, n := range counts {
		total += n
	}
	r.hub.Broadcast(NewEvent(EventBatchComplete, map[string]any{
		"batch_id": batchID,
		"status":   finalStatus,
		"total":    total,
		"passed":   counts["passed"],
		"failed":   counts["failed"],
		"errored":  counts["errored"],
	}))
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

		exclusions, exclErr := r.loadInstanceExclusions(ctx, cb.Name)
		if exclErr != nil {
			r.logf("WARN", "kitchen-batches: skipping repo %q (exclusion error): %v", cb.Name, exclErr)
			continue
		}

		plan, err := gitkitchen.PlanRepo(*analysis, tkCfg.PlatformMap, exclusions...)
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
// Uses the first entry if multiple are specified. Falls back to the highest
// globally configured target version when filters don't specify one.
func (r *Router) batchTargetVersion(b datastore.KitchenBatch) string {
	if len(b.Filters.TargetChefVersions) > 0 {
		return b.Filters.TargetChefVersions[0]
	}
	// Fall back to the globally configured target version.
	return r.liveConfig().TargetChefVersion
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
