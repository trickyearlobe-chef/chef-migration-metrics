// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/exports
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigExports(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{Exports: cfg.Exports}, configstore.KeyExports)
	case http.MethodPut:
		r.putAdminConfigExports(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigExports(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input config.ExportsConfig
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	if input.RetentionHours < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"exports.retention_hours must be >= 1.")
		return
	}
	if input.MaxRows < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"exports.max_rows must be >= 1.")
		return
	}
	if input.AsyncThreshold < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"exports.async_threshold must be >= 1.")
		return
	}

	r.storeAdminConfigSection(w, req, &config.Config{Exports: input}, configstore.KeyExports, false)
}
