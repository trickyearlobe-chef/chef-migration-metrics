// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// ---------------------------------------------------------------------------
// GET/PUT /api/v1/admin/config/backup
// ---------------------------------------------------------------------------

func (r *Router) handleAdminConfigBackup(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		cfg := r.liveConfig()
		r.writeAdminConfigSection(w, &config.Config{Backup: cfg.Backup}, configstore.KeyBackup)
	case http.MethodPut:
		r.putAdminConfigBackup(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and PUT.")
	}
}

func (r *Router) putAdminConfigBackup(w http.ResponseWriter, req *http.Request) {
	if r.configStore == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Config storage is not configured. Set CMM_CREDENTIAL_ENCRYPTION_KEY to enable.")
		return
	}

	var input config.BackupConfig
	if !decodeAdminConfigBody(w, req, &input) {
		return
	}

	if input.MaxGenerations < 1 {
		WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
			"backup.max_generations must be >= 1.")
		return
	}

	if input.Schedule != "" {
		if _, err := collector.ParseSchedule(input.Schedule); err != nil {
			WriteError(w, http.StatusUnprocessableEntity, ErrCodeValidationError,
				"backup.schedule: invalid cron expression: "+err.Error())
			return
		}
	}

	r.storeAdminConfigSection(w, req, &config.Config{Backup: input}, configstore.KeyBackup, false)
}
