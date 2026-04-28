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
	_ = tkCfg // used in later Phase 6 execution wiring

	if !tkCfg.IsEnabled() {
		WriteError(w, http.StatusConflict, "conflict", "Test Kitchen is disabled.")
		return
	}

	// Determine target status based on dry_run flag.
	targetStatus := "running"
	if b.DryRun {
		targetStatus = "previewing"
	}

	now := time.Now().UTC()
	b, err = r.db.UpdateKitchenBatchStatus(ctx, id, targetStatus, now)
	if err != nil {
		r.logf("ERROR", "kitchen-batches: transitioning batch %s to %s: %v", id, targetStatus, err)
		WriteInternalError(w, "Failed to start kitchen batch.")
		return
	}

	// Resolve the batch to show what will run.
	estimate, resolveErr := r.resolveBatch(ctx, b.Filters, b.MaxCount)
	if resolveErr != nil {
		r.logf("ERROR", "kitchen-batches: resolving batch %s: %v", id, resolveErr)
	}

	// For dry_run batches, immediately transition to completed since
	// actual execution is not yet implemented (Phase 6).
	if b.DryRun {
		b, err = r.db.UpdateKitchenBatchStatus(ctx, id, "completed", time.Now().UTC())
		if err != nil {
			r.logf("ERROR", "kitchen-batches: completing dry-run batch %s: %v", id, err)
			WriteInternalError(w, "Failed to complete dry-run batch.")
			return
		}
	}

	resp := batchDetailResponse{
		KitchenBatch: b,
		Estimate:     estimate,
	}
	WriteJSON(w, http.StatusOK, resp)
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

	if b.Status != "running" && b.Status != "previewing" {
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
