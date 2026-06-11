// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/readiness
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigReadiness(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{Readiness: cfg.Readiness}, configstore.KeyReadiness)
	case http.MethodPut:
		r.putAdminConfigReadiness(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigReadiness(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input config.ReadinessConfig
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	if input.InstallSizeMBLinux < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"readiness.install_size_mb_linux must be >= 1.")
		return
	}
	if input.InstallSizeMBWindows < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"readiness.install_size_mb_windows must be >= 1.")
		return
	}
	if input.MinRemainingFreePercent < 0 || input.MinRemainingFreePercent > 99 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"readiness.min_remaining_free_percent must be between 0 and 99.")
		return
	}
	if input.InstallPathLinux == "" {
		input.InstallPathLinux = "/hab"
	}
	if input.InstallPathWindows == "" {
		input.InstallPathWindows = `C:\hab`
	}

	// Readiness thresholds are pulled per collector run and read live — applied.
	r.storeAdminConfigSection(w, req, &config.Config{Readiness: input}, configstore.KeyReadiness, appliedApplier)
}
