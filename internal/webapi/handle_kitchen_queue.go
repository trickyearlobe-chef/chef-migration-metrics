// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// handleKitchenQueueList handles GET /api/v1/kitchen/queue.
// Query params: repo, type (git|node), status (comma-separated).
func (r *Router) handleKitchenQueueList(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	q := req.URL.Query()
	filter := datastore.KitchenQueueFilter{
		RepoName: q.Get("repo"),
		RunType:  q.Get("type"),
	}

	if statusParam := q.Get("status"); statusParam != "" {
		filter.Statuses = strings.Split(statusParam, ",")
	}

	items, err := r.db.ListKitchenQueue(req.Context(), filter)
	if err != nil {
		r.logf("ERROR", "kitchen queue list: %v", err)
		WriteInternalError(w, "Failed to list kitchen queue.")
		return
	}
	if items == nil {
		items = []datastore.KitchenQueueItem{}
	}

	// Include summary stats
	stats, statsErr := r.db.GetKitchenQueueStats(req.Context())
	if statsErr != nil {
		r.logf("ERROR", "kitchen queue stats: %v", statsErr)
		WriteInternalError(w, "Failed to get queue stats.")
		return
	}

	var workerCount int
	if r.kitchenQueue != nil {
		workerCount = r.kitchenQueue.RunningCount()
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"stats": map[string]any{
			"queued":         stats.Queued,
			"running":        stats.Running,
			"workers_active": workerCount,
		},
	})
}

// handleKitchenQueueGet handles GET /api/v1/kitchen/queue/{id}.
func (r *Router) handleKitchenQueueGet(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	id := extractPathParam(req.URL.Path, "/api/v1/kitchen/queue/")
	if id == "" {
		WriteBadRequest(w, "queue item ID is required")
		return
	}

	item, err := r.db.GetKitchenQueueItem(req.Context(), id)
	if err != nil {
		if err == datastore.ErrNotFound {
			WriteNotFound(w, "Queue item not found.")
			return
		}
		r.logf("ERROR", "kitchen queue get: %v", err)
		WriteInternalError(w, "Failed to get queue item.")
		return
	}

	WriteJSON(w, http.StatusOK, item)
}

// handleKitchenQueueCancel handles POST /api/v1/kitchen/queue/{id}/cancel.
func (r *Router) handleKitchenQueueCancel(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}

	// Extract ID from path: /api/v1/kitchen/queue/{id}/cancel
	path := req.URL.Path
	path = strings.TrimPrefix(path, "/api/v1/kitchen/queue/")
	id := strings.TrimSuffix(path, "/cancel")
	if id == "" {
		WriteBadRequest(w, "queue item ID is required")
		return
	}

	ctx := req.Context()

	// If the item is currently running, cancel the worker's context
	if r.kitchenQueue != nil {
		r.kitchenQueue.CancelItem(id)
	}

	// Mark as cancelled in DB (handles both queued and running)
	if err := r.db.CancelKitchenRun(ctx, id); err != nil {
		if err == datastore.ErrNotFound || strings.Contains(err.Error(), "not found") {
			WriteNotFound(w, "Queue item not found or not cancellable.")
			return
		}
		r.logf("ERROR", "kitchen queue cancel: %v", err)
		WriteInternalError(w, "Failed to cancel queue item.")
		return
	}

	item, _ := r.db.GetKitchenQueueItem(ctx, id)
	if item != nil {
		r.hub.Broadcast(NewEvent("kitchen_queue_update", item))
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Queue item cancelled",
		"id":      id,
	})
}

// handleKitchenQueueRetry handles POST /api/v1/kitchen/queue/{id}/retry.
func (r *Router) handleKitchenQueueRetry(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}

	path := req.URL.Path
	path = strings.TrimPrefix(path, "/api/v1/kitchen/queue/")
	id := strings.TrimSuffix(path, "/retry")
	if id == "" {
		WriteBadRequest(w, "queue item ID is required")
		return
	}

	item, err := r.db.RetryKitchenRun(req.Context(), id)
	if err != nil {
		if err == datastore.ErrNotFound {
			WriteNotFound(w, "Queue item not found or not retryable (must be failed/interrupted/cancelled).")
			return
		}
		if err == datastore.ErrAlreadyExists {
			WriteJSON(w, http.StatusConflict, map[string]string{
				"message": "Instance is already queued or running",
			})
			return
		}
		r.logf("ERROR", "kitchen queue retry: %v", err)
		WriteInternalError(w, "Failed to retry queue item.")
		return
	}

	r.hub.Broadcast(NewEvent("kitchen_queue_update", item))

	WriteJSON(w, http.StatusAccepted, map[string]any{
		"message":  "Item re-queued",
		"queue_id": item.ID,
		"status":   item.Status,
	})
}

// handleKitchenQueueStats handles GET /api/v1/kitchen/queue/stats.
func (r *Router) handleKitchenQueueStats(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}

	stats, err := r.db.GetKitchenQueueStats(req.Context())
	if err != nil {
		r.logf("ERROR", "kitchen queue stats: %v", err)
		WriteInternalError(w, "Failed to get queue stats.")
		return
	}

	var workersActive int
	if r.kitchenQueue != nil {
		workersActive = r.kitchenQueue.RunningCount()
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"queued":         stats.Queued,
		"running":        stats.Running,
		"workers_active": workersActive,
	})
}

// extractPathParam extracts the trailing path segment after a prefix.
func extractPathParam(path, prefix string) string {
	s := strings.TrimPrefix(path, prefix)
	if idx := strings.Index(s, "/"); idx != -1 {
		s = s[:idx]
	}
	return s
}

// handleKitchenQueueRouting dispatches /api/v1/kitchen/queue/{id}[/action] requests.
func (r *Router) handleKitchenQueueRouting(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/api/v1/kitchen/queue/")

	switch {
	case strings.HasSuffix(path, "/cancel"):
		r.handleKitchenQueueCancel(w, req)
	case strings.HasSuffix(path, "/retry"):
		r.handleKitchenQueueRetry(w, req)
	default:
		// GET /api/v1/kitchen/queue/{id}
		r.handleKitchenQueueGet(w, req)
	}
}
