// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// CRUD /api/v1/cookstyle/custom-cops
// ---------------------------------------------------------------------------

// handleCookstyleCustomCops handles GET/POST for the custom cops collection.
func (r *Router) handleCookstyleCustomCops(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.listCustomCops(w, req)
	case http.MethodPost:
		// A check written here moves verdicts across the whole estate, which is
		// the same power as reclassifying a shipped one — and that is admin.
		// Anyone with a session may still read them.
		if !requireAdminRole(w, req) {
			return
		}
		r.createCustomCop(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and POST.")
	}
}

// handleCookstyleCustomCop handles GET/PUT/DELETE for a single custom cop.
func (r *Router) handleCookstyleCustomCop(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.getCustomCop(w, req)
	case http.MethodPut:
		if !requireAdminRole(w, req) {
			return
		}
		r.updateCustomCop(w, req)
	case http.MethodDelete:
		if !requireAdminRole(w, req) {
			return
		}
		r.deleteCustomCop(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET, PUT, and DELETE.")
	}
}

func (r *Router) listCustomCops(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	cops, err := r.db.ListCustomCopDefinitions(ctx)
	if err != nil {
		r.logf("ERROR", "listing custom cops: %v", err)
		WriteInternalError(w, "Failed to list custom cops.")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": cops})
}

func (r *Router) createCustomCop(w http.ResponseWriter, req *http.Request) {
	var body datastore.CustomCopDefinition
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid JSON request body.")
		return
	}
	if err := validateCustomCop(body); err != nil {
		WriteBadRequest(w, err.Error())
		return
	}

	ctx := req.Context()
	id, err := r.db.CreateCustomCopDefinition(ctx, body)
	if err != nil {
		r.logf("ERROR", "creating custom cop: %v", err)
		WriteInternalError(w, "Failed to create custom cop.")
		return
	}

	body.ID = id
	r.propagateCustomCop(ctx, req, "custom_cop_created", body.CopName)
	WriteJSON(w, http.StatusCreated, body)
}

func (r *Router) getCustomCop(w http.ResponseWriter, req *http.Request) {
	copName := extractCustomCopName(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing custom cop name in URL path.")
		return
	}

	ctx := req.Context()
	cop, err := r.db.GetCustomCopDefinition(ctx, copName)
	if err != nil {
		r.logf("ERROR", "getting custom cop: %v", err)
		WriteInternalError(w, "Failed to get custom cop.")
		return
	}
	if cop == nil {
		WriteNotFound(w, "Custom cop not found.")
		return
	}
	WriteJSON(w, http.StatusOK, cop)
}

func (r *Router) updateCustomCop(w http.ResponseWriter, req *http.Request) {
	copName := extractCustomCopName(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing custom cop name in URL path.")
		return
	}

	var body datastore.CustomCopDefinition
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, "Invalid JSON request body.")
		return
	}
	body.CopName = copName
	if err := validateCustomCop(body); err != nil {
		WriteBadRequest(w, err.Error())
		return
	}

	ctx := req.Context()
	if err := r.db.UpdateCustomCopDefinition(ctx, &body); err != nil {
		r.logf("ERROR", "updating custom cop: %v", err)
		WriteInternalError(w, "Failed to update custom cop.")
		return
	}

	r.propagateCustomCop(ctx, req, "custom_cop_updated", body.CopName)
	WriteJSON(w, http.StatusOK, body)
}

func (r *Router) deleteCustomCop(w http.ResponseWriter, req *http.Request) {
	copName := extractCustomCopName(req.URL.Path)
	if copName == "" {
		WriteBadRequest(w, "Missing custom cop name in URL path.")
		return
	}

	ctx := req.Context()
	if err := r.db.DeleteCustomCopDefinition(ctx, copName); err != nil {
		r.logf("ERROR", "deleting custom cop: %v", err)
		WriteInternalError(w, "Failed to delete custom cop.")
		return
	}

	r.propagateCustomCop(ctx, req, "custom_cop_deleted", copName)
	WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
