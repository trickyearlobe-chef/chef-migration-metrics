// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
)

// ---------------------------------------------------------------------------
// PUT /api/v1/cookstyle/cops/:cop_name/classification
// ---------------------------------------------------------------------------

// classificationPutRequest is the request body for setting a cop
// classification. There is a single active target, so no target is carried.
type classificationPutRequest struct {
	Classification string `json:"classification"`
	Reason         string `json:"reason"`
}

// handleCookstyleCopClassification handles PUT/DELETE
// /api/v1/cookstyle/cops/<cop_name>/classification.
//
// Reclassifying a cop is a migration-policy decision (it changes verdicts,
// complexity, and node readiness across the estate), so it is restricted to
// admins even though the Cop Analysis view that hosts it is available to all
// authenticated users. The GET aggregation/drill-down routes stay open.
func (r *Router) handleCookstyleCopClassification(w http.ResponseWriter, req *http.Request) {
	if !requireAdminRole(w, req) {
		return
	}
	switch req.Method {
	case http.MethodPut:
		r.putCookstyleCopClassification(w, req)
	case http.MethodDelete:
		r.deleteCookstyleCopClassification(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports PUT and DELETE.")
	}
}

func (r *Router) putCookstyleCopClassification(w http.ResponseWriter, req *http.Request) {
	copName := extractCopNameFromClassificationPath(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing cop name in URL path.")
		return
	}

	var body classificationPutRequest
	if !decodeJSONBody(w, req, &body) {
		return
	}

	switch body.Classification {
	case "blocker", "review", "noise":
		// valid
	default:
		WriteBadRequest(w, "classification must be one of: blocker, review, noise")
		return
	}

	// The single active target drives the re-evaluation closure (it is not a
	// key for the override itself).
	target := r.defaultTargetVersion()
	if target == "" {
		WriteBadRequest(w, "No active target Chef version is configured.")
		return
	}

	ctx := req.Context()
	if err := r.db.UpsertCopClassification(ctx, copName, body.Classification, body.Reason, adminUsername(req)); err != nil {
		r.logf("ERROR", "upserting cop classification: %v", err)
		WriteInternalError(w, "Failed to save classification.")
		return
	}

	// Re-evaluation propagation (re-derive verdicts/compat/complexity + dependent
	// readiness) is O(repos) and slow at scale, so run it asynchronously via the
	// coalescing queue — the classification is already saved, the save returns
	// instantly, and the UI refreshes on the WebSocket recompute event.
	r.enqueueReclassification(copName, target)
	r.auditCookstyle(req, "cop_reclassified", copName, target, map[string]any{
		"classification": body.Classification,
		"reason":         body.Reason,
	})

	WriteJSON(w, http.StatusOK, map[string]any{
		"cop_name":       copName,
		"classification": body.Classification,
		"status":         "saved",
	})
}

func (r *Router) deleteCookstyleCopClassification(w http.ResponseWriter, req *http.Request) {
	copName := extractCopNameFromClassificationPath(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing cop name in URL path.")
		return
	}

	// The single active target drives the re-evaluation closure.
	target := r.defaultTargetVersion()
	if target == "" {
		WriteBadRequest(w, "No active target Chef version is configured.")
		return
	}

	ctx := req.Context()
	if err := r.db.DeleteCopClassification(ctx, copName); err != nil {
		r.logf("ERROR", "deleting cop classification: %v", err)
		WriteInternalError(w, "Failed to delete classification.")
		return
	}

	// Removing an override changes the resolved classification (falls back to
	// RemovedIn/curated/unclassified): re-evaluate the affected closure
	// asynchronously (see setCookstyleCopClassification) so delete returns fast.
	r.enqueueReclassification(copName, target)
	r.auditCookstyle(req, "cop_classification_removed", copName, target, nil)

	WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}
